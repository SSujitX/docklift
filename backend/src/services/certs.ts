// Let's Encrypt via certbot sidecar (webroot). Backend triggers issue; PEMs are source of truth.
import fs from 'fs';
import path from 'path';
import { spawn } from 'child_process';
import { X509Certificate } from 'crypto';
import { config } from '../lib/config.js';
import prisma from '../lib/prisma.js';
import { checkDomainDns } from './dnsCheck.js';

export type SslStatus =
  | 'missing'
  | 'pending'
  | 'active'
  | 'expiring'
  | 'expired'
  | 'failed';

export interface CertificateStatus {
  status: SslStatus;
  domain: string;
  expiresAt: string | null;
  error: string | null;
  diagnosticCommand: string | null;
  certPath: string | null;
}

const EXPIRING_DAYS = 21;
const STATUS_KEY_PREFIX = 'ssl_meta_';

export type SslEventLevel = 'info' | 'success' | 'warn' | 'error';

export interface SslEvent {
  at: string;
  level: SslEventLevel;
  message: string;
}

// Recent issuance activity per hostname, so the UI can show what certbot is doing.
// In-memory on purpose: it is progress narration, not state — PEMs remain the source of truth.
const MAX_EVENTS_PER_DOMAIN = 40;
const eventLog = new Map<string, SslEvent[]>();

export function appendSslEvent(
  hostnames: string | string[],
  level: SslEventLevel,
  message: string
): void {
  const event: SslEvent = { at: new Date().toISOString(), level, message };
  const hosts = (Array.isArray(hostnames) ? hostnames : [hostnames])
    .map((h) => h.trim().toLowerCase())
    .filter(Boolean);

  for (const host of hosts) {
    const events = eventLog.get(host) || [];
    events.push(event);
    eventLog.set(host, events.slice(-MAX_EVENTS_PER_DOMAIN));
  }
}

/** Merged, de-duplicated, oldest-first activity for a set of hostnames. */
export function getSslEvents(hostnames: string[]): SslEvent[] {
  const seen = new Set<string>();
  const merged: SslEvent[] = [];

  for (const host of hostnames) {
    for (const event of eventLog.get(host.trim().toLowerCase()) || []) {
      const key = `${event.at}|${event.message}`;
      if (seen.has(key)) continue;
      seen.add(key);
      merged.push(event);
    }
  }

  return merged.sort((a, b) => a.at.localeCompare(b.at)).slice(-MAX_EVENTS_PER_DOMAIN);
}

export function clearSslEvents(hostnames: string[]): void {
  for (const host of hostnames) {
    eventLog.delete(host.trim().toLowerCase());
  }
}

function certbotLogCommand(): string {
  return `sudo docker exec ${config.certbotContainer} tail -n 100 /var/log/letsencrypt/letsencrypt.log`;
}

/** Turn Certbot's verbose output into the first useful, user-facing cause. */
export function summarizeCertbotError(stdout: string, stderr: string): string {
  const output = [stderr, stdout]
    .filter(Boolean)
    .join('\n')
    .replace(/\u001b\[[0-9;]*m/g, '')
    .replace(/\r/g, '');
  const lines = output.split('\n').map((line) => line.trim());

  // HTTP-01 failures are emitted as Domain / Type / Detail blocks. Prefer the
  // CA's detail (NXDOMAIN, wrong IP, timeout, invalid response) over Certbot's
  // generic "Some challenges have failed" footer.
  for (let i = 0; i < lines.length; i += 1) {
    const domain = /^Domain:\s*(.+)$/i.exec(lines[i])?.[1]?.trim();
    if (!domain) continue;
    for (let j = i + 1; j < Math.min(lines.length, i + 8); j += 1) {
      const detail = /^Detail:\s*(.+)$/i.exec(lines[j])?.[1]?.trim();
      if (detail) return `${domain}: ${detail}`.slice(0, 700);
      if (/^Domain:/i.test(lines[j])) break;
    }
  }

  const boilerplate = /^(saving debug log|ask for help|search for solutions|see the logfile|some challenges have failed|certbot failed to authenticate|the certificate authority reported|an unexpected error occurred|hint:)/i;
  const useful = lines.find(
    (line) =>
      line &&
      !boilerplate.test(line) &&
      /(error|failed|invalid|unauthorized|forbidden|nxdomain|timeout|timed out|connection|refused|rate limit|no such|not found|permission denied)/i.test(line)
  );
  if (useful) return useful.slice(0, 700);

  return lines.find((line) => line && !boilerplate.test(line))?.slice(0, 700)
    || 'Certificate issuance failed for an unknown reason';
}

function liveDir(primaryDomain: string): string {
  return path.join(config.letsencryptPath, 'live', primaryDomain);
}

function fullchainPath(primaryDomain: string): string {
  return path.join(liveDir(primaryDomain), 'fullchain.pem');
}

function privkeyPath(primaryDomain: string): string {
  return path.join(liveDir(primaryDomain), 'privkey.pem');
}

/** Paths as seen inside nginx-proxy / certbot containers */
export function nginxCertPaths(primaryDomain: string): { fullchain: string; privkey: string } {
  return {
    fullchain: `/etc/letsencrypt/live/${primaryDomain}/fullchain.pem`,
    privkey: `/etc/letsencrypt/live/${primaryDomain}/privkey.pem`,
  };
}

export function certificateFilesExist(primaryDomain: string): boolean {
  return fs.existsSync(fullchainPath(primaryDomain)) && fs.existsSync(privkeyPath(primaryDomain));
}

/** DNS names present on an existing cert lineage (DNS SANs + CN only — never IP/email/URI). */
export function listCertDnsNames(certName: string): string[] {
  if (!certificateFilesExist(certName)) return [];
  try {
    const pem = fs.readFileSync(fullchainPath(certName), 'utf8');
    const cert = new X509Certificate(pem);
    const names = new Set<string>();
    const san = cert.subjectAltName || '';
    for (const part of san.split(',')) {
      const dns = /^DNS:(.+)$/i.exec(part.trim())?.[1]?.trim().toLowerCase();
      if (dns) names.add(dns);
    }
    const cn = /CN=([^,\n/]+)/i.exec(cert.subject)?.[1]?.trim().toLowerCase();
    if (cn) names.add(cn);
    return [...names];
  } catch {
    return [];
  }
}

function liveCertDirNames(): string[] {
  const liveRoot = path.join(config.letsencryptPath, 'live');
  if (!fs.existsSync(liveRoot)) return [];
  return fs.readdirSync(liveRoot).filter((name) => name !== 'README' && certificateFilesExist(name));
}

/**
 * Higher is better. When overlap ties, covering the UI primary must beat a newer orphan
 * (preferName/+1 used to lose to expiry ~1e12 and HSTS-brick the established host).
 */
function scoreCertLineage(certName: string, hosts: string[]): number {
  const primary = hosts[0];
  const sans = new Set(listCertDnsNames(certName));
  const exp = parseExpiry(certName)?.getTime() ?? 0;
  const valid = exp > Date.now() ? 1 : 0;
  const coversPrimary = primary && sans.has(primary) ? 1 : 0;
  const sanCount = sans.size;
  const nameIsPrimary = primary && certName === primary ? 1 : 0;
  return (
    coversPrimary * 1e16 +
    valid * 1e15 +
    sanCount * 1e10 +
    nameIsPrimary * 1e9 +
    exp
  );
}

/** True if an existing cert named `certName` covers every hostname in `hosts`. */
export function certCoversAllHosts(certName: string, hosts: string[]): boolean {
  const names = new Set(listCertDnsNames(certName));
  if (names.size === 0) return false;
  return hosts.every((h) => names.has(h));
}

/**
 * LE `live/<name>/` that covers every hostname. Prefers healthy (non-expired) lineages
 * over same-named orphans — folder name alone never wins.
 */
export function findCoveringCertName(hosts: string[]): string | null {
  const cleaned = [...new Set(hosts.map((h) => h.trim().toLowerCase()).filter(Boolean))];
  if (cleaned.length === 0) return null;

  let best: string | null = null;
  let bestScore = -1;
  for (const name of liveCertDirNames()) {
    if (!certCoversAllHosts(name, cleaned)) continue;
    const score = scoreCertLineage(name, cleaned);
    if (score > bestScore) {
      bestScore = score;
      best = name;
    }
  }
  return best;
}

/**
 * Best existing lineage to expand/replace within this host set.
 * Only considers `live/<name>/` where `name` is one of the requested hosts — never
 * mutates another service's leftover lineage after a domain move.
 */
export function findBestExpandableCertName(hosts: string[]): string | null {
  const cleaned = [...new Set(hosts.map((h) => h.trim().toLowerCase()).filter(Boolean))];
  if (cleaned.length === 0) return null;

  const allowed = new Set(cleaned);
  let best: string | null = null;
  let bestOverlap = 0;
  let bestScore = -1;
  for (const name of liveCertDirNames()) {
    if (!allowed.has(name)) continue;
    const sans = new Set(listCertDnsNames(name));
    const overlap = cleaned.reduce((n, h) => n + (sans.has(h) ? 1 : 0), 0);
    if (overlap === 0) continue;
    const score = scoreCertLineage(name, cleaned);
    if (overlap > bestOverlap || (overlap === bestOverlap && score > bestScore)) {
      bestOverlap = overlap;
      bestScore = score;
      best = name;
    }
  }
  return best;
}

/** Resolve LE live/ directory for a hostname — health-aware, not same-folder-first. */
export function resolveCertName(hostname: string): string | null {
  const host = hostname.trim().toLowerCase();
  if (!host) return null;
  return findCoveringCertName([host]);
}

/** Choose certbot `--cert-name` + whether this is a pure SAN expand (superset). */
export function selectCertLineage(hosts: string[]): {
  certName: string;
  covering: string | null;
  needsExpand: boolean;
} {
  const cleaned = [...new Set(hosts.map((h) => h.trim().toLowerCase()).filter(Boolean))];
  const primary = cleaned[0] || 'unknown';
  const covering = findCoveringCertName(cleaned);
  const expandable = findBestExpandableCertName(cleaned);
  const certName = covering || expandable || primary;
  const existing = listCertDnsNames(certName);
  // Pure expand only when every existing name stays and at least one new name is added
  const needsExpand =
    existing.length > 0 &&
    existing.every((n) => cleaned.includes(n)) &&
    cleaned.some((n) => !existing.includes(n));
  return { certName, covering, needsExpand };
}

async function getSetting(key: string): Promise<string | null> {
  const row = await prisma.settings.findUnique({ where: { key } });
  return row?.value ?? null;
}

async function setSetting(key: string, value: string): Promise<void> {
  await prisma.settings.upsert({
    where: { key },
    update: { value },
    create: { key, value },
  });
}

async function deleteSetting(key: string): Promise<void> {
  await prisma.settings.deleteMany({ where: { key } });
}

export async function getAcmeEmail(): Promise<string> {
  const fromSettings = await getSetting('ssl_acme_email');
  if (fromSettings?.includes('@')) return fromSettings.trim();
  if (config.certbotEmail?.includes('@')) return config.certbotEmail.trim();
  const admin = await prisma.user.findFirst({
    orderBy: { created_at: 'asc' },
    select: { email: true },
  });
  if (admin?.email) return admin.email;
  throw new Error('No ACME email configured. Set SSL email in Settings or CERTBOT_EMAIL.');
}

export async function setAcmeEmail(email: string): Promise<void> {
  if (!email || !email.includes('@')) {
    throw new Error('Invalid email');
  }
  await setSetting('ssl_acme_email', email.trim().toLowerCase());
}

async function setMeta(
  primaryDomain: string,
  meta: { status: SslStatus; error?: string | null }
): Promise<void> {
  await setSetting(
    STATUS_KEY_PREFIX + primaryDomain,
    JSON.stringify({
      status: meta.status,
      error: meta.error ?? null,
      updatedAt: new Date().toISOString(),
    })
  );
}

async function getMeta(
  primaryDomain: string
): Promise<{ status: SslStatus; error: string | null } | null> {
  const raw = await getSetting(STATUS_KEY_PREFIX + primaryDomain);
  if (!raw) return null;
  try {
    const parsed = JSON.parse(raw);
    return { status: parsed.status, error: parsed.error ?? null };
  } catch {
    return null;
  }
}

function parseExpiry(primaryDomain: string): Date | null {
  try {
    const pem = fs.readFileSync(fullchainPath(primaryDomain), 'utf8');
    const cert = new X509Certificate(pem);
    return new Date(cert.validTo);
  } catch {
    return null;
  }
}

export async function getCertificateStatus(hostname: string): Promise<CertificateStatus> {
  const domain = hostname.trim().toLowerCase();
  const certName = resolveCertName(domain) || domain;
  const meta = (await getMeta(domain)) || (await getMeta(certName));
  const exists = certificateFilesExist(certName);

  if (exists) {
    const expires = parseExpiry(certName);
    if (!expires) {
      return {
        status: 'failed',
        domain,
        expiresAt: null,
        error: meta?.error || 'Certificate file unreadable',
        diagnosticCommand: null,
        certPath: fullchainPath(certName),
      };
    }
    const msLeft = expires.getTime() - Date.now();
    if (msLeft <= 0) {
      return {
        status: 'expired',
        domain,
        expiresAt: expires.toISOString(),
        error: null,
        diagnosticCommand: null,
        certPath: fullchainPath(certName),
      };
    }
    if (msLeft < EXPIRING_DAYS * 24 * 60 * 60 * 1000) {
      return {
        status: 'expiring',
        domain,
        expiresAt: expires.toISOString(),
        error: null,
        diagnosticCommand: null,
        certPath: fullchainPath(certName),
      };
    }
    return {
      status: 'active',
      domain,
      expiresAt: expires.toISOString(),
      error: null,
      diagnosticCommand: null,
      certPath: fullchainPath(certName),
    };
  }

  if (meta?.status === 'pending') {
    return {
      status: 'pending',
      domain,
      expiresAt: null,
      error: null,
      diagnosticCommand: null,
      certPath: null,
    };
  }

  if (meta?.status === 'failed') {
    return {
      status: 'failed',
      domain,
      expiresAt: null,
      error: meta.error,
      diagnosticCommand: certbotLogCommand(),
      certPath: null,
    };
  }

  return {
    status: 'missing',
    domain,
    expiresAt: null,
    error: null,
    diagnosticCommand: null,
    certPath: null,
  };
}

function runDockerExec(args: string[]): Promise<{ code: number; stdout: string; stderr: string }> {
  return new Promise((resolve) => {
    const child = spawn('docker', ['exec', ...args], { windowsHide: true });
    let stdout = '';
    let stderr = '';
    child.stdout.on('data', (d) => {
      stdout += d.toString();
    });
    child.stderr.on('data', (d) => {
      stderr += d.toString();
    });
    child.on('close', (code) => {
      resolve({ code: code ?? 1, stdout, stderr });
    });
    child.on('error', (err) => {
      resolve({ code: 1, stdout, stderr: err.message });
    });
  });
}

/** Status for a specific live/ lineage, labeled with the UI primary hostname. */
function statusForLineage(certName: string, displayDomain: string): CertificateStatus {
  const domain = displayDomain.trim().toLowerCase();
  if (!certificateFilesExist(certName)) {
    return {
      status: 'missing',
      domain,
      expiresAt: null,
      error: null,
      diagnosticCommand: null,
      certPath: null,
    };
  }
  const expires = parseExpiry(certName);
  if (!expires) {
    return {
      status: 'failed',
      domain,
      expiresAt: null,
      error: 'Certificate file unreadable',
      diagnosticCommand: null,
      certPath: fullchainPath(certName),
    };
  }
  const msLeft = expires.getTime() - Date.now();
  if (msLeft <= 0) {
    return {
      status: 'expired',
      domain,
      expiresAt: expires.toISOString(),
      error: null,
      diagnosticCommand: null,
      certPath: fullchainPath(certName),
    };
  }
  if (msLeft < EXPIRING_DAYS * 24 * 60 * 60 * 1000) {
    return {
      status: 'expiring',
      domain,
      expiresAt: expires.toISOString(),
      error: null,
      diagnosticCommand: null,
      certPath: fullchainPath(certName),
    };
  }
  return {
    status: 'active',
    domain,
    expiresAt: expires.toISOString(),
    error: null,
    diagnosticCommand: null,
    certPath: fullchainPath(certName),
  };
}

/**
 * Issue (or reuse) a certificate for the given hostnames.
 * First hostname is the UI/meta primary; PEM directory follows covering/expand lineage.
 */
export async function issueCertificate(
  domains: string[],
  opts?: { force?: boolean }
): Promise<CertificateStatus> {
  const cleaned = [...new Set(domains.map((d) => d.trim().toLowerCase()).filter(Boolean))];
  if (cleaned.length === 0) {
    throw new Error('No domains provided');
  }
  const primary = cleaned[0];

  const { certName: preferredName, covering } = selectCertLineage(cleaned);
  if (!opts?.force && covering) {
    const status = statusForLineage(covering, primary);
    if (status.status === 'active' || status.status === 'expiring') {
      appendSslEvent(
        cleaned,
        'info',
        covering === primary
          ? 'Existing certificate already covers these hostnames — reusing it.'
          : `Existing certificate (${covering}) already covers these hostnames — reusing it.`
      );
      return status;
    }
  }

  await setMeta(primary, { status: 'pending', error: null });
  appendSslEvent(
    cleaned,
    'info',
    `Requesting certificate for ${cleaned.join(', ')}${config.certbotStaging ? ' (staging CA)' : ''}`
  );

  let email: string;
  try {
    email = await getAcmeEmail();
  } catch (e: any) {
    appendSslEvent(cleaned, 'error', e.message);
    await setMeta(primary, { status: 'failed', error: e.message });
    return statusForLineage(preferredName, primary);
  }

  const certName = preferredName;
  const existingSans = listCertDnsNames(certName);

  // Build ACME -d list: required hosts + extras still on the lineage.
  // Drop extras only when DNS is definitively missing (same bar as cleaned hosts for "missing").
  // mismatch/unknown stay — dropping them would make a non-superset --expand (fork -0001).
  const orderHosts = [...cleaned];
  for (const extra of existingSans) {
    if (orderHosts.includes(extra)) continue;
    const check = await checkDomainDns(extra);
    if (check.status === 'missing') {
      appendSslEvent(
        cleaned,
        'warn',
        `Leaving ${extra} off this order — DNS record missing (certificate set will shrink).`
      );
    } else {
      orderHosts.push(extra);
    }
  }

  // Pure expand only when the final -d list is a strict superset of the existing lineage
  const needsExpand =
    existingSans.length > 0 &&
    existingSans.every((n) => orderHosts.includes(n)) &&
    orderHosts.some((n) => !existingSans.includes(n));
  // Shrink / replace / force: renew without --expand so certbot does not fork -0001
  // Shrink/replace needs force-renewal; pure --expand does not (avoids burning LE quota)
  const isShrinkOrReplace =
    existingSans.length > 0 &&
    (existingSans.length !== orderHosts.length ||
      !existingSans.every((n) => orderHosts.includes(n))) &&
    !needsExpand;
  const forceRenewal = opts?.force === true || isShrinkOrReplace;

  // Preflight DNS for every hostname that will be on the ACME order
  const unresolved: string[] = [];
  for (const domain of orderHosts) {
    const check = await checkDomainDns(domain);
    appendSslEvent(
      cleaned,
      check.status === 'ok' ? 'info' : check.status === 'missing' ? 'error' : 'warn',
      check.message
    );
    if (check.status === 'missing' && cleaned.includes(domain)) unresolved.push(domain);
  }

  if (unresolved.length > 0) {
    const err = `DNS record missing for ${unresolved.join(', ')} — create an A record pointing at this server, then retry SSL.`;
    appendSslEvent(cleaned, 'error', 'Skipped Let\u2019s Encrypt: DNS is not ready yet.');
    await setMeta(primary, { status: 'failed', error: err });
    return statusForLineage(certName, primary);
  }

  const args = [
    config.certbotContainer,
    'certbot',
    'certonly',
    '--webroot',
    '-w',
    '/var/www/certbot',
    '--non-interactive',
    '--agree-tos',
    '--email',
    email,
    '--cert-name',
    certName,
    ...(config.certbotStaging ? ['--staging'] : []),
    ...(needsExpand ? ['--expand'] : []),
    ...(forceRenewal ? ['--force-renewal'] : []),
  ];
  for (const d of orderHosts) {
    args.push('-d', d);
  }

  console.log(
    `[SSL] Issuing certificate for ${orderHosts.join(', ')} cert-name=${certName} expand=${needsExpand} force=${forceRenewal} (staging=${config.certbotStaging})`
  );
  appendSslEvent(cleaned, 'info', `Running certbot HTTP-01 challenge in ${config.certbotContainer}…`);
  const result = await runDockerExec(args);

  // Success = PEMs exist AND cover every requested hostname (not merely old folder present)
  const coversRequested =
    certificateFilesExist(certName) && certCoversAllHosts(certName, cleaned);
  if (result.code !== 0 || !coversRequested) {
    const err =
      result.code !== 0
        ? summarizeCertbotError(result.stdout, result.stderr)
        : `Certificate at live/${certName} does not cover ${cleaned.join(', ')}`;
    console.error(`[SSL] Issue failed for ${primary}:`, err);
    appendSslEvent(cleaned, 'error', err);
    await setMeta(primary, { status: 'failed', error: err });
    return statusForLineage(certName, primary);
  }

  await setMeta(primary, { status: 'active', error: null });
  console.log(`[SSL] Certificate active for ${primary} (live/${certName})`);
  const issued = statusForLineage(certName, primary);
  appendSslEvent(
    cleaned,
    'success',
    issued.expiresAt
      ? `Certificate issued — valid until ${new Date(issued.expiresAt).toUTCString()}`
      : 'Certificate issued'
  );
  return issued;
}

export async function clearSslMeta(primaryDomain: string): Promise<void> {
  await deleteSetting(STATUS_KEY_PREFIX + primaryDomain);
}

function certMtimeStatePath(): string {
  return path.join(config.dataPath, 'ssl-cert-mtime');
}

function readPersistedCertMtime(): number {
  try {
    const raw = fs.readFileSync(certMtimeStatePath(), 'utf8').trim();
    const n = Number(raw);
    return Number.isFinite(n) && n > 0 ? n : 0;
  } catch {
    return 0;
  }
}

function writePersistedCertMtime(stamp: number): void {
  try {
    fs.mkdirSync(config.dataPath, { recursive: true });
    if (stamp <= 0) {
      fs.rmSync(certMtimeStatePath(), { force: true });
      return;
    }
    fs.writeFileSync(certMtimeStatePath(), String(stamp));
  } catch (e) {
    console.warn('[SSL] Could not persist cert mtime:', e);
  }
}

/**
 * Watch cert mtimes and reload nginx when certbot renew updates files.
 * Persists the last-seen mtime so a renew while the backend was down still triggers reload on start.
 */
export function startCertRenewWatcher(reloadFn: () => Promise<void>): void {
  let lastStamp = readPersistedCertMtime();
  let ticking = false;
  let pending = false;
  /** Require consecutive empty scans before clearing — live/ may be briefly unreadable on boot. */
  let emptyScans = 0;
  const EMPTY_SCANS_BEFORE_CLEAR = 2;

  const tick = async (reason: string) => {
    if (ticking) {
      pending = true;
      return;
    }
    ticking = true;
    try {
      do {
        pending = false;
        try {
          const next = scanCertMtimes();
          if (next === 0) {
            emptyScans += 1;
            if (lastStamp !== 0 && emptyScans >= EMPTY_SCANS_BEFORE_CLEAR) {
              lastStamp = 0;
              writePersistedCertMtime(0);
            }
            continue;
          }
          emptyScans = 0;
          // Any mtime change (renew, restore) must reload — including renews while backend was down
          if (next !== lastStamp) {
            console.log(`[SSL] Certificate files changed (${reason}) — reloading nginx-proxy`);
            await reloadFn();
            lastStamp = next;
            writePersistedCertMtime(lastStamp);
          }
        } catch (e) {
          // Keep pending so a failed reload still retries on the coalesced follow-up
          pending = true;
          console.warn('[SSL] Renew watcher error:', e);
          break;
        }
      } while (pending);
    } finally {
      ticking = false;
      if (pending) {
        pending = false;
        setTimeout(() => void tick('retry'), 15_000);
      }
    }
  };

  void tick('startup');
  setInterval(() => void tick('poll'), 5 * 60 * 1000);
}

function scanCertMtimes(): number {
  const liveRoot = path.join(config.letsencryptPath, 'live');
  if (!fs.existsSync(liveRoot)) return 0;
  let max = 0;
  for (const name of fs.readdirSync(liveRoot)) {
    if (name === 'README') continue;
    const p = path.join(liveRoot, name, 'fullchain.pem');
    try {
      const st = fs.statSync(p);
      if (st.mtimeMs > max) max = st.mtimeMs;
    } catch {
      /* skip */
    }
  }
  return max;
}
