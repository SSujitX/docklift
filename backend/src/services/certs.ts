// Let's Encrypt via certbot sidecar (webroot). Backend triggers issue; PEMs are source of truth.
import fs from 'fs';
import path from 'path';
import { spawn } from 'child_process';
import { X509Certificate } from 'crypto';
import { config } from '../lib/config.js';
import prisma from '../lib/prisma.js';

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
  certPath: string | null;
}

const EXPIRING_DAYS = 21;
const STATUS_KEY_PREFIX = 'ssl_meta_';

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
        certPath: fullchainPath(certName),
      };
    }
    if (msLeft < EXPIRING_DAYS * 24 * 60 * 60 * 1000) {
      return {
        status: 'expiring',
        domain,
        expiresAt: expires.toISOString(),
        error: null,
        certPath: fullchainPath(certName),
      };
    }
    return {
      status: 'active',
      domain,
      expiresAt: expires.toISOString(),
      error: null,
      certPath: fullchainPath(certName),
    };
  }

  if (meta?.status === 'pending') {
    return {
      status: 'pending',
      domain,
      expiresAt: null,
      error: null,
      certPath: null,
    };
  }

  if (meta?.status === 'failed') {
    return {
      status: 'failed',
      domain,
      expiresAt: null,
      error: meta.error,
      certPath: null,
    };
  }

  return {
    status: 'missing',
    domain,
    expiresAt: null,
    error: null,
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
      return status;
    }
  }

  await setMeta(primary, { status: 'pending', error: null });

  let email: string;
  try {
    email = await getAcmeEmail();
  } catch (e: any) {
    await setMeta(primary, { status: 'failed', error: e.message });
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
  const result = await runDockerExec(args);

  if (result.code !== 0 || !certificateFilesExist(primary)) {
    const err =
      (result.stderr || result.stdout || 'certbot failed').trim().slice(0, 800) ||
      'Certificate issuance failed';
    console.error(`[SSL] Issue failed for ${primary}:`, err);
    await setMeta(primary, { status: 'failed', error: err });
    return getCertificateStatus(primary);
  }

  await setMeta(primary, { status: 'active', error: null });
  console.log(`[SSL] Certificate active for ${primary}`);
  return getCertificateStatus(primary);
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
