/**
 * Persistent seal after a failed DB rollback during restore.
 * Survives backend restarts; blocks further restores until an operator clears it.
 */
import fs from 'fs';
import path from 'path';
import { config } from './config.js';
import { enterMaintenance, exitMaintenance } from './maintenance.js';
import { forceRestoreLock, releaseRestoreLock } from './restoreLock.js';

export type RestoreCriticalPayload = {
  at: string;
  detail: string;
  liveDbPath: string;
  preRestorePath: string;
};

export function restoreCriticalMarkerPath(): string {
  return path.join(config.dataPath, '.restore-critical');
}

export function isRestoreCritical(): boolean {
  try {
    return fs.existsSync(restoreCriticalMarkerPath());
  } catch {
    return false;
  }
}

export function readRestoreCritical(): RestoreCriticalPayload | null {
  try {
    const p = restoreCriticalMarkerPath();
    if (!fs.existsSync(p)) return null;
    // Only regular files are valid seals (a leftover directory is still "critical")
    const st = fs.statSync(p);
    if (!st.isFile()) return { at: '', detail: 'invalid seal path (not a file)', liveDbPath: '', preRestorePath: '' };
    const raw = fs.readFileSync(p, 'utf8');
    return JSON.parse(raw) as RestoreCriticalPayload;
  } catch {
    return null;
  }
}

/** Enter critical recovery: persist marker, hold restore lock, keep maintenance on. */
export function enterRestoreCritical(input: {
  detail: string;
  liveDbPath: string;
}): void {
  const payload: RestoreCriticalPayload = {
    at: new Date().toISOString(),
    detail: input.detail,
    liveDbPath: input.liveDbPath,
    preRestorePath: `${input.liveDbPath}.pre-restore`,
  };
  if (!fs.existsSync(config.dataPath)) {
    fs.mkdirSync(config.dataPath, { recursive: true });
  }
  fs.writeFileSync(restoreCriticalMarkerPath(), JSON.stringify(payload, null, 2), {
    mode: 0o600,
  });
  enterMaintenance(
    `CRITICAL restore recovery required — do not retry restore. See ${restoreCriticalMarkerPath()}`
  );
  forceRestoreLock('restore-critical');
  console.error('[CRITICAL] Restore sealed:', payload);
}

/** Boot: re-apply maintenance + lock if a previous process left a critical marker. */
export function loadRestoreCriticalOnBoot(): void {
  if (!isRestoreCritical()) return;
  const payload = readRestoreCritical();
  enterMaintenance(
    `CRITICAL restore recovery required — do not retry restore. See ${restoreCriticalMarkerPath()}`
  );
  forceRestoreLock('restore-critical');
  console.error('[CRITICAL] Loaded .restore-critical from disk:', payload);
}

/**
 * Explicit operator clear after manual DB repair.
 * Throws if the marker cannot be deleted or still exists afterward —
 * maintenance/lock stay active in that case (fail closed).
 */
export function clearRestoreCritical(): RestoreCriticalPayload | null {
  const prev = readRestoreCritical();
  const p = restoreCriticalMarkerPath();

  if (fs.existsSync(p)) {
    try {
      fs.unlinkSync(p);
    } catch (err: unknown) {
      const msg = err instanceof Error ? err.message : String(err);
      throw new Error(
        `Failed to delete critical seal at ${p}: ${msg}. Maintenance remains active.`
      );
    }
  }

  if (fs.existsSync(p)) {
    throw new Error(
      `Critical seal still present after delete: ${p}. Maintenance remains active.`
    );
  }

  exitMaintenance();
  releaseRestoreLock();
  console.warn('[CRITICAL] Restore critical seal cleared by operator');
  return prev;
}
