// Cross-platform recursive directory copy + content-level replace helpers.
// Never shell out to `cp` — bare-metal Windows has no GNU cp.
//
// Restore targets (/deployments, /nginx-conf, /etc/letsencrypt) are Docker bind
// mounts. Renaming those directories (or renaming staging onto the mount path)
// fails with EBUSY/EXDEV. Replace contents inside the mount only.
import fs from 'fs';
import fsp from 'fs/promises';
import path from 'path';

const STAGING_PREFIX = '.docklift-restore-staging-';
const PREV_PREFIX = '.docklift-restore-prev-';

function isRestoreMetaName(name: string): boolean {
  return name.startsWith(STAGING_PREFIX) || name.startsWith(PREV_PREFIX);
}

/** Recursively copy directory contents from src into dest (dest is created). */
export async function copyDirContents(src: string, dest: string): Promise<void> {
  await fsp.mkdir(dest, { recursive: true });
  await fsp.cp(src, dest, { recursive: true, force: true, errorOnExist: false });
}

/**
 * Move a single filesystem entry. Prefer rename; fall back to copy+rm when
 * rename fails (EXDEV / Windows locks / busy files).
 */
async function moveEntry(from: string, to: string): Promise<void> {
  try {
    await fsp.rename(from, to);
    return;
  } catch (err: unknown) {
    const code = (err as NodeJS.ErrnoException)?.code;
    if (code !== 'EXDEV' && code !== 'EPERM' && code !== 'EBUSY' && code !== 'EACCES') {
      throw err;
    }
  }

  const st = await fsp.lstat(from);
  if (st.isDirectory()) {
    await fsp.cp(from, to, { recursive: true, force: true });
    await fsp.rm(from, { recursive: true, force: true });
    return;
  }
  await fsp.mkdir(path.dirname(to), { recursive: true });
  await fsp.copyFile(from, to);
  await fsp.rm(from, { force: true });
}

async function listContentNames(dir: string): Promise<string[]> {
  const names = await fsp.readdir(dir);
  return names.filter((name) => !isRestoreMetaName(name));
}

async function clearContentEntries(dir: string): Promise<void> {
  for (const name of await listContentNames(dir)) {
    await fsp.rm(path.join(dir, name), { recursive: true, force: true });
  }
}

/** After full move-aside + partial promote: wipe new content, put prev back. */
async function restoreAfterPromote(prev: string, targetDir: string): Promise<void> {
  if (!fs.existsSync(prev)) return;
  await clearContentEntries(targetDir);
  for (const name of await fsp.readdir(prev)) {
    await moveEntry(path.join(prev, name), path.join(targetDir, name));
  }
}

/**
 * After partial move-aside: put moved children back without deleting entries
 * that never left the target.
 */
async function restoreAfterPartialAside(prev: string, targetDir: string): Promise<void> {
  if (!fs.existsSync(prev)) return;
  for (const name of await fsp.readdir(prev)) {
    const dest = path.join(targetDir, name);
    if (fs.existsSync(dest)) {
      await fsp.rm(dest, { recursive: true, force: true });
    }
    await moveEntry(path.join(prev, name), dest);
  }
}

async function prevHasEntries(prev: string): Promise<boolean> {
  return fs.existsSync(prev) && (await fsp.readdir(prev)).length > 0;
}

/**
 * Replace the contents of `targetDir` with `sourceDir`.
 *
 * Staging and prev live *inside* `targetDir` so Docker bind mounts work.
 * Never renames `targetDir` itself and never renames a directory onto the
 * mount path. On failure after any children were moved into prev, restores
 * them before rethrowing.
 */
export async function replaceDirContents(sourceDir: string, targetDir: string): Promise<void> {
  if (!fs.existsSync(sourceDir)) {
    throw new Error(`replaceDirContents: source missing: ${sourceDir}`);
  }

  await fsp.mkdir(targetDir, { recursive: true });

  // Drop incomplete staging dirs from crashed runs (safe — not live data).
  for (const name of await fsp.readdir(targetDir)) {
    if (name.startsWith(STAGING_PREFIX)) {
      await fsp.rm(path.join(targetDir, name), { recursive: true, force: true });
    }
  }

  const stamp = `${Date.now()}-${Math.random().toString(16).slice(2, 8)}`;
  const staging = path.join(targetDir, `${STAGING_PREFIX}${stamp}`);
  const prev = path.join(targetDir, `${PREV_PREFIX}${stamp}`);
  let promotedAny = false;

  try {
    await fsp.mkdir(staging, { recursive: true });
    await fsp.cp(sourceDir, staging, { recursive: true, force: true });

    await fsp.mkdir(prev, { recursive: true });

    // Fold orphaned prev dirs from crashed runs into this prev so success can reclaim them.
    for (const name of await fsp.readdir(targetDir)) {
      if (!name.startsWith(PREV_PREFIX) || name === path.basename(prev)) continue;
      const orphan = path.join(targetDir, name);
      for (const child of await fsp.readdir(orphan)) {
        const dest = path.join(prev, child);
        if (fs.existsSync(dest)) {
          await fsp.rm(dest, { recursive: true, force: true });
        }
        await moveEntry(path.join(orphan, child), dest);
      }
      await fsp.rm(orphan, { recursive: true, force: true });
    }

    for (const name of await listContentNames(targetDir)) {
      const dest = path.join(prev, name);
      // Live wins over orphan-folded names (crash mid-promote can leave both).
      if (fs.existsSync(dest)) {
        await fsp.rm(dest, { recursive: true, force: true });
      }
      await moveEntry(path.join(targetDir, name), dest);
    }

    for (const name of await fsp.readdir(staging)) {
      await moveEntry(path.join(staging, name), path.join(targetDir, name));
      promotedAny = true;
    }

    await fsp.rm(staging, { recursive: true, force: true });
    await fsp.rm(prev, { recursive: true, force: true });
  } catch (err) {
    try {
      if (await prevHasEntries(prev)) {
        if (promotedAny) {
          await restoreAfterPromote(prev, targetDir);
        } else {
          await restoreAfterPartialAside(prev, targetDir);
        }
      }
    } catch {
      /* best-effort; original error is rethrown */
    }
    try {
      await fsp.rm(staging, { recursive: true, force: true });
    } catch {
      /* ignore */
    }
    try {
      if (fs.existsSync(prev) && (await fsp.readdir(prev)).length === 0) {
        await fsp.rm(prev, { recursive: true, force: true });
      }
    } catch {
      /* ignore */
    }
    throw err;
  }
}
