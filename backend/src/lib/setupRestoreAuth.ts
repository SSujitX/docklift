import type { Request } from 'express';
import crypto from 'crypto';
import fs from 'fs';
import path from 'path';
import { config } from './config.js';
import { consumeBootstrapSecret } from './bootstrap.js';

export const SETUP_TOKEN_HEADER = 'x-setup-token';

export type SetupRestoreRequest = Request & {
  /** Validated setup-token auth for fresh-install restore (token not yet consumed). */
  setupTokenAuth?: boolean;
};

export function setupTokenPath(): string {
  return path.join(config.dataPath, '.setup-token');
}

/** Validate setup token without consuming it. */
export function validateSetupToken(provided: string | undefined): boolean {
  if (!provided || typeof provided !== 'string' || provided.length === 0) return false;
  try {
    const tokenPath = setupTokenPath();
    if (!fs.existsSync(tokenPath)) return false;
    const stored = fs.readFileSync(tokenPath, 'utf8').trim();
    if (!stored) return false;
    const a = Buffer.from(provided);
    const b = Buffer.from(stored);
    if (a.length !== b.length) return false;
    return crypto.timingSafeEqual(a, b);
  } catch {
    return false;
  }
}

/** Consume setup + bootstrap secrets only after a successful fresh-install restore. */
export function consumeSetupRestoreSecrets(): void {
  try {
    const tokenPath = setupTokenPath();
    if (fs.existsSync(tokenPath)) fs.unlinkSync(tokenPath);
  } catch {
    /* ignore */
  }
  consumeBootstrapSecret();
}
