import test from 'node:test';
import assert from 'node:assert/strict';
import fs from 'fs';
import path from 'path';
import {
  enterRestoreCritical,
  clearRestoreCritical,
  isRestoreCritical,
  readRestoreCritical,
  restoreCriticalMarkerPath,
} from './restoreCritical.js';
import {
  tryAcquireRestoreLock,
  isRestoreLocked,
  releaseRestoreLock,
} from './restoreLock.js';
import { isMaintenanceMode } from './maintenance.js';
import { config } from './config.js';

test('enterRestoreCritical persists marker and blocks new restore locks', () => {
  fs.mkdirSync(config.dataPath, { recursive: true });
  const marker = restoreCriticalMarkerPath();
  const prev = fs.existsSync(marker) ? fs.readFileSync(marker, 'utf8') : null;
  const live = path.join(config.dataPath, 'docklift.db');
  try {
    enterRestoreCritical({ detail: 'rollback failed', liveDbPath: live });
    assert.equal(isRestoreCritical(), true);
    const payload = readRestoreCritical();
    assert.ok(payload);
    assert.equal(payload!.liveDbPath, live);
    assert.equal(payload!.preRestorePath, `${live}.pre-restore`);
    assert.equal(tryAcquireRestoreLock('another-restore'), false);
    assert.equal(isRestoreLocked(), true);
    releaseRestoreLock();
    assert.equal(isRestoreCritical(), true);
    assert.equal(tryAcquireRestoreLock('another-restore'), false);
  } finally {
    try {
      clearRestoreCritical();
    } catch {
      /* may already be cleared / invalid */
    }
    if (fs.existsSync(marker)) {
      try {
        fs.rmSync(marker, { recursive: true, force: true });
      } catch {
        /* ignore */
      }
    }
    if (prev != null) fs.writeFileSync(marker, prev, { mode: 0o600 });
  }
});

test('clearRestoreCritical removes seal and allows locks again', () => {
  fs.mkdirSync(config.dataPath, { recursive: true });
  const live = path.join(config.dataPath, 'docklift.db');
  enterRestoreCritical({ detail: 'test', liveDbPath: live });
  clearRestoreCritical();
  assert.equal(isRestoreCritical(), false);
  assert.equal(isMaintenanceMode(), false);
  assert.equal(tryAcquireRestoreLock('ok'), true);
  releaseRestoreLock();
});

test('clearRestoreCritical fails closed when marker cannot be deleted', () => {
  fs.mkdirSync(config.dataPath, { recursive: true });
  const marker = restoreCriticalMarkerPath();
  const live = path.join(config.dataPath, 'docklift.db');
  enterRestoreCritical({ detail: 'deletion-failure test', liveDbPath: live });

  // Replace the seal file with a directory so unlinkSync fails (EISDIR / EPERM)
  fs.unlinkSync(marker);
  fs.mkdirSync(marker);

  assert.throws(
    () => clearRestoreCritical(),
    (err: unknown) =>
      err instanceof Error &&
      /Failed to delete critical seal|still present after delete/i.test(err.message)
  );

  // Marker path still exists → still critical; maintenance must remain on
  assert.equal(isRestoreCritical(), true);
  assert.equal(isMaintenanceMode(), true);
  assert.equal(tryAcquireRestoreLock('must-fail'), false);

  // Cleanup for other tests
  fs.rmSync(marker, { recursive: true, force: true });
  // Re-enter then clear cleanly so we don't leave maintenance on
  enterRestoreCritical({ detail: 'cleanup', liveDbPath: live });
  clearRestoreCritical();
});
