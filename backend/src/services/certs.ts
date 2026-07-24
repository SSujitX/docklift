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

/** Resolve LE live/ directory name that covers this hostname (primary or SAN). */
export function resolveCertName(hostname: string): string | null {
  const host = hostname.trim().toLowerCase();
  if (!host) return null;
  if (certificateFilesExist(host)) return host;

  const liveRoot = path.join(config.letsencryptPath, 'live');
  if (!fs.existsSync(liveRoot)) return null;

  for (const name of fs.readdirSync(liveRoot)) {
    if (name === 'README') continue;
    try {
      const pem = fs.readFileSync(fullchainPath(name), 'utf8');
      const cert = new X509Certificate(pem);
      const san = cert.subjectAltName || '';
      const names = san
        .split(',')
        .map((p) => p.trim().replace(/^DNS:/i, '').toLowerCase())
        .filter(Boolean);
      if (names.includes(host)) return name;
      const cn = /CN=([^,\n/]+)/i.exec(cert.subject)?.[1]?.trim().toLowerCase();
      if (cn === host) return name;
    } catch {
      /* skip */
    }
  }
  return null;
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

/** True if an existing cert named `certName` covers every hostname in `hosts`. */
function certCoversAllHosts(certName: string, hosts: string[]): boolean {
  if (!certificateFilesExist(certName)) return false;
  try {
    const pem = fs.readFileSync(fullchainPath(certName), 'utf8');
    const cert = new X509Certificate(pem);
    const san = cert.subjectAltName || '';
    const names = new Set(
      san
        .split(',')
        .map((p) => p.trim().replace(/^DNS:/i, '').toLowerCase())
        .filter(Boolean)
    );
    const cn = /CN=([^,\n/]+)/i.exec(cert.subject)?.[1]?.trim().toLowerCase();
    if (cn) names.add(cn);
    return hosts.every((h) => names.has(h));
  } catch {
    return false;
  }
}

/**
 * Issue (or reuse) a certificate for the given hostnames.
 * Primary domain = first entry (LE live/ directory name).
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

  // Reuse only when PEMs exist, not forced, and every requested host is on the cert (SAN expand)
  if (!opts?.force && certificateFilesExist(primary) && certCoversAllHosts(primary, cleaned)) {
    const status = await getCertificateStatus(primary);
    if (status.status === 'active' || status.status === 'expiring') {
      appendSslEvent(cleaned, 'info', 'Existing certificate already covers these hostnames — reusing it.');
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
    return getCertificateStatus(primary);
  }

  // Preflight DNS. Let's Encrypt fails the whole order when any single hostname does
  // not resolve, and failed orders count against the account rate limit — so stop here
  // with the real reason instead of calling certbot.
  const unresolved: string[] = [];
  for (const domain of cleaned) {
    const check = await checkDomainDns(domain);
    appendSslEvent(
      cleaned,
      check.status === 'ok' ? 'info' : check.status === 'missing' ? 'error' : 'warn',
      check.message
    );
    if (check.status === 'missing') unresolved.push(domain);
  }

  if (unresolved.length > 0) {
    const err = `DNS record missing for ${unresolved.join(', ')} — create an A record pointing at this server, then retry SSL.`;
    appendSslEvent(cleaned, 'error', 'Skipped Let\u2019s Encrypt: DNS is not ready yet.');
    await setMeta(primary, { status: 'failed', error: err });
    return getCertificateStatus(primary);
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
    primary,
    ...(config.certbotStaging ? ['--staging'] : []),
    // Expand SANs or explicit retry: force renewal so certbot replaces the line
    ...(!opts?.force && certificateFilesExist(primary) && !certCoversAllHosts(primary, cleaned)
      ? ['--expand', '--force-renewal']
      : []),
    ...(opts?.force ? ['--force-renewal'] : []),
  ];
  for (const d of cleaned) {
    args.push('-d', d);
  }

  console.log(`[SSL] Issuing certificate for ${cleaned.join(', ')} (staging=${config.certbotStaging})`);
  appendSslEvent(cleaned, 'info', `Running certbot HTTP-01 challenge in ${config.certbotContainer}…`);
  const result = await runDockerExec(args);

  if (result.code !== 0 || !certificateFilesExist(primary)) {
    const err = summarizeCertbotError(result.stdout, result.stderr);
    console.error(`[SSL] Issue failed for ${primary}:`, err);
    appendSslEvent(cleaned, 'error', err);
    await setMeta(primary, { status: 'failed', error: err });
    return getCertificateStatus(primary);
  }

  await setMeta(primary, { status: 'active', error: null });
  console.log(`[SSL] Certificate active for ${primary}`);
  const issued = await getCertificateStatus(primary);
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

/** Watch cert mtimes and reload nginx when certbot renew updates files. */
export function startCertRenewWatcher(reloadFn: () => Promise<void>): void {
  let lastStamp = scanCertMtimes();
  setInterval(async () => {
    try {
      const next = scanCertMtimes();
      if (next > lastStamp && lastStamp > 0) {
        console.log('[SSL] Certificate files changed — reloading nginx-proxy');
        await reloadFn();
      }
      lastStamp = next;
    } catch (e) {
      console.warn('[SSL] Renew watcher error:', e);
    }
  }, 5 * 60 * 1000);
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
