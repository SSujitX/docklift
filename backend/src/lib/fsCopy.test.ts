import { afterEach, describe, expect, test } from 'bun:test';
import fs from 'fs';
import fsp from 'fs/promises';
import os from 'os';
import path from 'path';
import { replaceDirContents } from './fsCopy.js';

const tempRoots: string[] = [];

async function makeTempRoot(): Promise<string> {
  const root = await fsp.mkdtemp(path.join(os.tmpdir(), 'docklift-fscopy-'));
  tempRoots.push(root);
  return root;
}

afterEach(async () => {
  while (tempRoots.length) {
    const root = tempRoots.pop()!;
    await fsp.rm(root, { recursive: true, force: true }).catch(() => {});
  }
});

describe('replaceDirContents', () => {
  test('replaces target contents with source contents', async () => {
    const root = await makeTempRoot();
    const source = path.join(root, 'source');
    const target = path.join(root, 'target');
    await fsp.mkdir(path.join(source, 'app'), { recursive: true });
    await fsp.writeFile(path.join(source, 'app', 'new.txt'), 'new');
    await fsp.mkdir(path.join(target, 'old-app'), { recursive: true });
    await fsp.writeFile(path.join(target, 'old-app', 'old.txt'), 'old');

    await replaceDirContents(source, target);

    expect(await fsp.readFile(path.join(target, 'app', 'new.txt'), 'utf8')).toBe('new');
    expect(fs.existsSync(path.join(target, 'old-app'))).toBe(false);
    const leftovers = (await fsp.readdir(target)).filter((n) => n.startsWith('.docklift-restore-'));
    expect(leftovers).toEqual([]);
  });

  test('clears target when source is empty', async () => {
    const root = await makeTempRoot();
    const source = path.join(root, 'source');
    const target = path.join(root, 'target');
    await fsp.mkdir(source, { recursive: true });
    await fsp.mkdir(target, { recursive: true });
    await fsp.writeFile(path.join(target, 'gone.txt'), 'x');

    await replaceDirContents(source, target);

    expect(await fsp.readdir(target)).toEqual([]);
  });

  test('never renames the target directory itself (bind-mount safe)', async () => {
    const root = await makeTempRoot();
    const source = path.join(root, 'source');
    const target = path.join(root, 'target');
    await fsp.mkdir(source, { recursive: true });
    await fsp.writeFile(path.join(source, 'a.txt'), 'a');
    await fsp.mkdir(target, { recursive: true });
    await fsp.writeFile(path.join(target, 'b.txt'), 'b');

    const realRename = fsp.rename.bind(fsp);
    const targetRenames: string[] = [];
    // Same module object fsCopy uses — simulate Docker EBUSY on mount-root rename.
    (fsp as typeof fsp).rename = (async (from, to) => {
      if (path.resolve(String(from)) === path.resolve(target)) {
        targetRenames.push(`${from} -> ${to}`);
        throw Object.assign(new Error('resource busy or locked'), { code: 'EBUSY' });
      }
      return realRename(from, to);
    }) as typeof fsp.rename;

    try {
      await replaceDirContents(source, target);
      expect(targetRenames).toEqual([]);
      expect(await fsp.readFile(path.join(target, 'a.txt'), 'utf8')).toBe('a');
      expect(fs.existsSync(path.join(target, 'b.txt'))).toBe(false);
    } finally {
      (fsp as typeof fsp).rename = realRename;
    }
  });

  test('restores previous contents when promote fails', async () => {
    const root = await makeTempRoot();
    const source = path.join(root, 'source');
    const target = path.join(root, 'target');
    await fsp.mkdir(source, { recursive: true });
    await fsp.writeFile(path.join(source, 'new.txt'), 'new');
    await fsp.mkdir(target, { recursive: true });
    await fsp.writeFile(path.join(target, 'old.txt'), 'old');

    const realRename = fsp.rename.bind(fsp);
    (fsp as typeof fsp).rename = (async (from, to) => {
      if (String(from).includes('.docklift-restore-staging-')) {
        throw Object.assign(new Error('promote fail'), { code: 'EIO' });
      }
      return realRename(from, to);
    }) as typeof fsp.rename;

    try {
      await expect(replaceDirContents(source, target)).rejects.toThrow(/promote fail/);
      expect(await fsp.readFile(path.join(target, 'old.txt'), 'utf8')).toBe('old');
      expect(fs.existsSync(path.join(target, 'new.txt'))).toBe(false);
    } finally {
      (fsp as typeof fsp).rename = realRename;
    }
  });

  test('restores full tree when move-aside fails mid-loop', async () => {
    const root = await makeTempRoot();
    const source = path.join(root, 'source');
    const target = path.join(root, 'target');
    await fsp.mkdir(source, { recursive: true });
    await fsp.writeFile(path.join(source, 'new.txt'), 'new');
    await fsp.mkdir(target, { recursive: true });
    await fsp.writeFile(path.join(target, 'a.txt'), 'a');
    await fsp.writeFile(path.join(target, 'b.txt'), 'b');

    const realRename = fsp.rename.bind(fsp);
    let asideMoves = 0;
    (fsp as typeof fsp).rename = (async (from, to) => {
      if (String(to).includes('.docklift-restore-prev-')) {
        asideMoves += 1;
        if (asideMoves >= 2) {
          throw Object.assign(new Error('aside fail'), { code: 'EIO' });
        }
      }
      return realRename(from, to);
    }) as typeof fsp.rename;

    try {
      await expect(replaceDirContents(source, target)).rejects.toThrow(/aside fail/);
      expect(await fsp.readFile(path.join(target, 'a.txt'), 'utf8')).toBe('a');
      expect(await fsp.readFile(path.join(target, 'b.txt'), 'utf8')).toBe('b');
      expect(fs.existsSync(path.join(target, 'new.txt'))).toBe(false);
    } finally {
      (fsp as typeof fsp).rename = realRename;
    }
  });

  test('reclaims orphaned prev dirs from a prior crash on next success', async () => {
    const root = await makeTempRoot();
    const source = path.join(root, 'source');
    const target = path.join(root, 'target');
    await fsp.mkdir(source, { recursive: true });
    await fsp.writeFile(path.join(source, 'fresh.txt'), 'fresh');
    await fsp.mkdir(target, { recursive: true });
    await fsp.writeFile(path.join(target, 'live.txt'), 'live');
    const orphan = path.join(target, '.docklift-restore-prev-oldcrash');
    await fsp.mkdir(orphan, { recursive: true });
    await fsp.writeFile(path.join(orphan, 'stranded.txt'), 'stranded');

    await replaceDirContents(source, target);

    expect(await fsp.readFile(path.join(target, 'fresh.txt'), 'utf8')).toBe('fresh');
    expect(fs.existsSync(path.join(target, 'live.txt'))).toBe(false);
    const leftovers = (await fsp.readdir(target)).filter((n) => n.startsWith('.docklift-restore-'));
    expect(leftovers).toEqual([]);
  });

  test('succeeds when orphan prev and live share the same child name', async () => {
    const root = await makeTempRoot();
    const source = path.join(root, 'source');
    const target = path.join(root, 'target');
    await fsp.mkdir(source, { recursive: true });
    await fsp.writeFile(path.join(source, 'app.txt'), 'from-backup');
    await fsp.mkdir(target, { recursive: true });
    await fsp.writeFile(path.join(target, 'app.txt'), 'live');
    const orphan = path.join(target, '.docklift-restore-prev-midpromote');
    await fsp.mkdir(orphan, { recursive: true });
    await fsp.writeFile(path.join(orphan, 'app.txt'), 'old-aside');

    await replaceDirContents(source, target);

    expect(await fsp.readFile(path.join(target, 'app.txt'), 'utf8')).toBe('from-backup');
    const leftovers = (await fsp.readdir(target)).filter((n) => n.startsWith('.docklift-restore-'));
    expect(leftovers).toEqual([]);
  });

  test('throws when source is missing', async () => {
    const root = await makeTempRoot();
    const target = path.join(root, 'target');
    await fsp.mkdir(target, { recursive: true });
    await expect(replaceDirContents(path.join(root, 'missing'), target)).rejects.toThrow(
      /source missing/
    );
  });
});
