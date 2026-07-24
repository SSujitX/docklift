// Fresh-install bootstrap secret — printed to server logs, never returned by a public API.
import fs from 'fs';
import path from 'path';
import crypto from 'crypto';
import { config } from './config.js';
import prisma from './prisma.js';

const BOOTSTRAP_FILE = path.join(config.dataPath, '.bootstrap-secret');

export function getBootstrapSecretPath(): string {
  return BOOTSTRAP_FILE;
}

/** Ensure a bootstrap secret exists when setup is incomplete. Returns the secret (for logging). */
export function ensureBootstrapSecret(): string {
  if (!fs.existsSync(config.dataPath)) {
    fs.mkdirSync(config.dataPath, { recursive: true });
  }
  if (fs.existsSync(BOOTSTRAP_FILE)) {
    return fs.readFileSync(BOOTSTRAP_FILE, 'utf8').trim();
  }
  const secret = crypto.randomBytes(32).toString('hex');
  fs.writeFileSync(BOOTSTRAP_FILE, secret, { mode: 0o600 });
  return secret;
}

export function readBootstrapSecret(): string | null {
  try {
    if (!fs.existsSync(BOOTSTRAP_FILE)) return null;
    const value = fs.readFileSync(BOOTSTRAP_FILE, 'utf8').trim();
    return value.length > 0 ? value : null;
  } catch {
    return null;
  }
}

export function verifyBootstrapSecret(provided: string | undefined | null): boolean {
  if (!provided || typeof provided !== 'string') return false;
  const expected = readBootstrapSecret();
  if (!expected) return false;
  const a = Buffer.from(provided);
  const b = Buffer.from(expected);
  if (a.length !== b.length) return false;
  return crypto.timingSafeEqual(a, b);
}

/** Delete bootstrap secret after first admin is created (closes claim window). */
export function consumeBootstrapSecret(): void {
  try {
    if (fs.existsSync(BOOTSTRAP_FILE)) fs.unlinkSync(BOOTSTRAP_FILE);
  } catch {
    // ignore
  }
}

/** Log bootstrap instructions when no users exist yet. */
export async function logBootstrapIfNeeded(): Promise<void> {
  try {
    const userCount = await prisma.user.count();
    if (userCount > 0) return;
  } catch {
    // DB missing / unrestored — still treat as fresh setup
  }

  const secret = ensureBootstrapSecret();
  console.log(`
╔══════════════════════════════════════════════════════════════════╗
║  SECURITY: Fresh install — bootstrap secret required             ║
║                                                                  ║
║  Paste this into the Setup page (Register / Restore).            ║
║  It is NOT available via any public API.                         ║
║                                                                  ║
║  ${secret}
║                                                                  ║
║  Also on host: data/.bootstrap-secret                            ║
╚══════════════════════════════════════════════════════════════════╝
`);
}
