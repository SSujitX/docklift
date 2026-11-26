import { describe, expect, test } from 'bun:test';
import crypto from 'crypto';
import path from 'path';
import { isSafeSetupStateToken, setupStateFilePath } from './githubSetupState.js';
import { isPathInside } from './pathSecurity.js';

const DIR = path.resolve('data', 'github-setup');

describe('setupStateFilePath', () => {
  test('accepts a generated state and stays inside the setup dir', () => {
    const state = crypto.randomBytes(24).toString('hex');
    const file = setupStateFilePath(DIR, state);
    expect(file).toBe(path.join(DIR, `${state}.json`));
    expect(isPathInside(DIR, file as string)).toBe(true);
  });

  test('rejects traversal and separators', () => {
    for (const state of [
      '../../package',
      '..\\..\\package',
      '..',
      '.',
      'a/b',
      'a\\b',
      '/etc/passwd',
      'C:\\x',
      'state.json',
      'a b',
      'a\0b',
      '%2e%2e%2fpackage',
    ]) {
      expect(setupStateFilePath(DIR, state)).toBeNull();
    }
  });

  test('rejects empty, oversized and non-string values', () => {
    expect(setupStateFilePath(DIR, '')).toBeNull();
    expect(setupStateFilePath(DIR, 'a'.repeat(129))).toBeNull();
    expect(setupStateFilePath(DIR, undefined)).toBeNull();
    expect(setupStateFilePath(DIR, ['abc'])).toBeNull();
    expect(isSafeSetupStateToken(123)).toBe(false);
  });
});
