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
