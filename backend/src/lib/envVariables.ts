import prisma from './prisma.js';

const ENV_KEY_RE = /^[A-Za-z_][A-Za-z0-9_]*$/;

export function isValidEnvKey(key: unknown): key is string {
  return typeof key === 'string' && ENV_KEY_RE.test(key) && key.length <= 128;
}

export function normalizeEnvValue(value: unknown): string {
  if (value == null) return '';
  let v = String(value);
  // Never call .trim on non-string before stringify
  v = v.trim();
  if (v.length >= 2) {
    const first = v.charAt(0);
    const last = v.charAt(v.length - 1);
    if ((first === '"' && last === '"') || (first === "'" && last === "'")) {
      v = v.substring(1, v.length - 1);
    }
  }
  return v;
}

/** Keep newest row per (project_id, key); delete older duplicates before unique constraint. */
export async function dedupeEnvVariables(): Promise<number> {
  const all = await prisma.envVariable.findMany({
    orderBy: [{ project_id: 'asc' }, { key: 'asc' }, { created_at: 'desc' }],
  });
  const seen = new Set<string>();
  const toDelete: string[] = [];
  for (const row of all) {
    const k = `${row.project_id}\0${row.key}`;
    if (seen.has(k)) {
      toDelete.push(row.id);
    } else {
      seen.add(k);
    }
  }
  if (toDelete.length === 0) return 0;
  await prisma.envVariable.deleteMany({ where: { id: { in: toDelete } } });
  return toDelete.length;
}
