// Cross-platform recursive directory copy + atomic-ish swap helpers.
// Never shell out to `cp` — bare-metal Windows has no GNU cp.
import fs from 'fs';
import fsp from 'fs/promises';
import path from 'path';

/** Recursively copy directory contents from src into dest (dest is created). */
export async function copyDirContents(src: string, dest: string): Promise<void> {
  await fsp.mkdir(dest, { recursive: true });
  await fsp.cp(src, dest, { recursive: true, force: true, errorOnExist: false });
}

/**
 * Replace the contents of `targetDir` with `sourceDir`.
 * Copies into a sibling temp directory first, then swaps, so a failed copy
 * never leaves an empty target.
 */
export async function replaceDirContents(sourceDir: string, targetDir: string): Promise<void> {
  const parent = path.dirname(targetDir);
  const base = path.basename(targetDir);
  const staging = path.join(parent, `.${base}.staging-${Date.now()}`);
  const backup = path.join(parent, `.${base}.prev-${Date.now()}`);

  await fsp.rm(staging, { recursive: true, force: true });
  await fsp.mkdir(staging, { recursive: true });
  await fsp.cp(sourceDir, staging, { recursive: true, force: true });

  await fsp.mkdir(targetDir, { recursive: true });

  // Move current aside (if present), then promote staging.
  let movedAside = false;
  try {
    if (fs.existsSync(targetDir)) {
      await fsp.rename(targetDir, backup);
      movedAside = true;
    }
    await fsp.rename(staging, targetDir);
  } catch (err) {
    // Best-effort rollback
    try {
      await fsp.rm(staging, { recursive: true, force: true });
    } catch {
      /* ignore */
    }
    if (movedAside && fs.existsSync(backup) && !fs.existsSync(targetDir)) {
      try {
        await fsp.rename(backup, targetDir);
      } catch {
        /* ignore */
      }
    }
    throw err;
  }

  await fsp.rm(backup, { recursive: true, force: true });
}
