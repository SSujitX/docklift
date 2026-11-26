import path from 'path';

/**
 * True if `candidate` is exactly `base` or a path inside it.
 * Uses path.relative — rejects `..` and absolute escapes (sibling-prefix safe).
 */
export function isPathInside(base: string, candidate: string): boolean {
  const root = path.resolve(base);
  const target = path.resolve(candidate);
  if (target === root) return true;
  const rel = path.relative(root, target);
  return rel !== '' && !rel.startsWith('..') && !path.isAbsolute(rel);
}

/**
 * True if `candidate` is, or sits under, a `.git` entry below `base`.
 * Git metadata (config, hooks) is executed by git itself on the next pull,
 * so the file editor must never read or write it.
 */
export function touchesGitDir(base: string, candidate: string): boolean {
  const rel = path.relative(path.resolve(base), path.resolve(candidate));
  return rel.split(/[\\/]+/).some((segment) => segment.toLowerCase() === '.git');
}
