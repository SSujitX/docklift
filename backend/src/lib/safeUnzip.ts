// Safe ZIP extract: zip-slip rejection + file count / actual bytes-written quotas
import fs from 'fs';
import fsp from 'fs/promises';
import path from 'path';
import { Transform } from 'stream';
import unzipper from 'unzipper';
import { isPathInside } from './pathSecurity.js';

const DEFAULT_MAX_FILES = 50_000;
const DEFAULT_MAX_UNCOMPRESSED_BYTES = 2 * 1024 * 1024 * 1024; // 2 GiB

export async function safeExtractZip(
  zipPath: string,
  destDir: string,
  opts?: { maxFiles?: number; maxUncompressedBytes?: number }
): Promise<void> {
  const maxFiles = opts?.maxFiles ?? DEFAULT_MAX_FILES;
  const maxBytes = opts?.maxUncompressedBytes ?? DEFAULT_MAX_UNCOMPRESSED_BYTES;

  await fsp.mkdir(destDir, { recursive: true });

  const directory = await unzipper.Open.file(zipPath);
  let fileCount = 0;
  let totalBytes = 0;

  for (const entry of directory.files) {
    const type = (entry as { type?: string }).type;
    if (type === 'Directory') continue;

    fileCount += 1;
    if (fileCount > maxFiles) {
      throw new Error(`Archive contains too many files (max ${maxFiles})`);
    }

    // Fast-reject obviously oversized declared sizes (still enforce actual writes below)
    const declared = Number((entry as { uncompressedSize?: number }).uncompressedSize || 0);
    if (declared > maxBytes || totalBytes + declared > maxBytes) {
      throw new Error(`Archive uncompressed size exceeds limit (${maxBytes} bytes)`);
    }

    const rawName = entry.path.replace(/\\/g, '/');
    if (!rawName || rawName.startsWith('/') || rawName.includes('\0')) {
      throw new Error(`Unsafe path in archive: ${entry.path}`);
    }
    const segments = rawName.split('/').filter((s) => s.length > 0);
    if (segments.some((s) => s === '..')) {
      throw new Error(`Path traversal in archive: ${entry.path}`);
    }

    const destPath = path.resolve(destDir, ...segments);
    if (!isPathInside(destDir, destPath)) {
      throw new Error(`Zip-slip blocked: ${entry.path}`);
    }

    await fsp.mkdir(path.dirname(destPath), { recursive: true });

    await new Promise<void>((resolve, reject) => {
      const limiter = new Transform({
        transform(chunk, _enc, cb) {
          totalBytes += chunk.length;
          if (totalBytes > maxBytes) {
            cb(new Error(`Archive uncompressed size exceeds limit (${maxBytes} bytes)`));
            return;
          }
          cb(null, chunk);
        },
      });

      entry
        .stream()
        .pipe(limiter)
        .pipe(fs.createWriteStream(destPath))
        .on('finish', () => resolve())
        .on('error', reject);

      limiter.on('error', reject);
    });
  }
}
