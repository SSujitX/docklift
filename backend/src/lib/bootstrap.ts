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

/** Atomically claim bootstrap secret for registration (exclusive rename). Returns claim path or null if lost. */
export function tryClaimBootstrapSecret(): string | null {
  if (!fs.existsSync(BOOTSTRAP_FILE)) return null;
  const claimName = `.claimed-${crypto.randomBytes(16).toString('hex')}`;
  const claimPath = path.join(config.dataPath, claimName);
  try {
    fs.renameSync(BOOTSTRAP_FILE, claimPath);
    return claimPath;
  } catch {
    return null;
  }
}

export function verifyBootstrapSecretAtPath(
  secretPath: string,
  provided: string | undefined | null
): boolean {
  if (!provided || typeof provided !== 'string') return false;
  try {
    if (!fs.existsSync(secretPath)) return false;
    const expected = fs.readFileSync(secretPath, 'utf8').trim();
    if (!expected) return false;
    const a = Buffer.from(provided);
    const b = Buffer.from(expected);
    if (a.length !== b.length) return false;
    return crypto.timingSafeEqual(a, b);
  } catch {
    return false;
  }
}

/** Drop claim file after successful registration (secret must not be restorable). */
export function finalizeBootstrapClaim(claimPath: string): void {
  try {
    if (fs.existsSync(claimPath)) fs.unlinkSync(claimPath);
  } catch {
    // ignore
  }
}

/** Restore bootstrap secret file if registration fails after claim. */
export function abortBootstrapClaim(claimPath: string): void {
  try {
    if (fs.existsSync(claimPath) && !fs.existsSync(BOOTSTRAP_FILE)) {
      fs.renameSync(claimPath, BOOTSTRAP_FILE);
    }
  } catch {
    // ignore
  }
}

/**
 * Crash recovery: if registration died after rename(secret → .claimed-*),
 * restore the newest claim file so setup is not permanently bricked.
 * Only when no admin users exist yet.
 */
export function recoverStaleBootstrapClaims(): void {
  try {
    if (!fs.existsSync(config.dataPath)) return;
    if (fs.existsSync(BOOTSTRAP_FILE)) return;
    const claims = fs
      .readdirSync(config.dataPath)
      .filter((name) => name.startsWith('.claimed-'))
      .map((name) => path.join(config.dataPath, name))
      .filter((p) => {
        try {
          return fs.statSync(p).isFile();
        } catch {
          return false;
        }
      })
      .sort((a, b) => fs.statSync(b).mtimeMs - fs.statSync(a).mtimeMs);
    if (claims.length === 0) return;
    const newest = claims[0];
    fs.renameSync(newest, BOOTSTRAP_FILE);
    for (const orphan of claims.slice(1)) {
      try {
        fs.unlinkSync(orphan);
      } catch {
        /* ignore */
      }
    }
    console.warn(`[bootstrap] Restored stale claim ${path.basename(newest)} → .bootstrap-secret`);
  } catch (err) {
    console.warn('[bootstrap] Failed to recover stale claims:', err);
  }
}

/** Log bootstrap instructions when no users exist yet. */
export async function logBootstrapIfNeeded(): Promise<void> {
  try {
    const userCount = await prisma.user.count();
    if (userCount > 0) {
      // Admin exists — drop leftover claim files from interrupted setup
      try {
        if (fs.existsSync(config.dataPath)) {
          for (const name of fs.readdirSync(config.dataPath)) {
            if (name.startsWith('.claimed-')) {
              fs.unlinkSync(path.join(config.dataPath, name));
            }
          }
        }
      } catch {
        /* ignore */
      }
      return;
    }
  } catch {
    // DB missing / unrestored — still treat as fresh setup
  }

  recoverStaleBootstrapClaims();
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
