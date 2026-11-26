import { describe, expect, test } from 'bun:test';
import path from 'path';
import { isPathInside, touchesGitDir } from './pathSecurity.js';

const BASE = path.resolve('deployments', 'project-1');
const at = (...parts: string[]) => path.resolve(BASE, ...parts);

describe('touchesGitDir', () => {
  test('flags git metadata at any depth', () => {
    expect(touchesGitDir(BASE, at('.git'))).toBe(true);
    expect(touchesGitDir(BASE, at('.git', 'config'))).toBe(true);
    expect(touchesGitDir(BASE, at('.git', 'hooks', 'post-merge'))).toBe(true);
    expect(touchesGitDir(BASE, at('packages', 'api', '.git', 'config'))).toBe(true);
    expect(touchesGitDir(BASE, at('.GIT', 'config'))).toBe(true);
  });

  test('leaves ordinary project files alone', () => {
    expect(touchesGitDir(BASE, at('.gitignore'))).toBe(false);
    expect(touchesGitDir(BASE, at('.gitattributes'))).toBe(false);
    expect(touchesGitDir(BASE, at('.github', 'workflows', 'ci.yml'))).toBe(false);
    expect(touchesGitDir(BASE, at('src', 'git.ts'))).toBe(false);
    expect(touchesGitDir(BASE, at('docs', 'my.git.notes.md'))).toBe(false);
    expect(touchesGitDir(BASE, at('Dockerfile'))).toBe(false);
  });

  test('resolves dot segments before checking', () => {
    expect(touchesGitDir(BASE, at('src', '..', '.git', 'config'))).toBe(true);
  });
});

describe('isPathInside', () => {
  test('accepts the base and its children, rejects escapes', () => {
    expect(isPathInside(BASE, BASE)).toBe(true);
    expect(isPathInside(BASE, at('src', 'index.ts'))).toBe(true);
    expect(isPathInside(BASE, at('..', 'project-2', 'x'))).toBe(false);
    expect(isPathInside(BASE, `${BASE}-sibling`)).toBe(false);
  });
});
