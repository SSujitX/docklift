import prisma from './prisma.js';

const ENV_KEY_RE = /^[A-Za-z_][A-Za-z0-9_]*$/;

/** Empty string = shared across every service in the project. */
export const SHARED_ENV_SERVICE = '';

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

export function normalizeEnvServiceName(name: unknown): string {
  if (name == null) return SHARED_ENV_SERVICE;
  const trimmed = String(name).trim();
  return trimmed;
}

export type ScopedEnvVar = {
  key: string;
  value: string;
  service_name?: string | null;
  is_build_arg?: boolean | null;
  is_runtime?: boolean | null;
  is_secret?: boolean | null;
};

/**
 * Merge shared (service_name "") with service-specific vars.
 * Service keys override shared keys of the same name.
 */
export function envForService<T extends ScopedEnvVar>(
  all: T[],
  serviceName: string,
): T[] {
  const map = new Map<string, T>();
  for (const row of all) {
    const scope = row.service_name ?? SHARED_ENV_SERVICE;
    if (scope === SHARED_ENV_SERVICE) map.set(row.key, row);
  }
  for (const row of all) {
    const scope = row.service_name ?? SHARED_ENV_SERVICE;
    if (scope === serviceName) map.set(row.key, row);
  }
  return [...map.values()];
}

/** Keep newest row per (project_id, service_name, key). */
export async function dedupeEnvVariables(): Promise<number> {
  const all = await prisma.envVariable.findMany({
    orderBy: [
      { project_id: 'asc' },
      { service_name: 'asc' },
      { key: 'asc' },
      { created_at: 'desc' },
    ],
  });
  const seen = new Set<string>();
  const toDelete: string[] = [];
  for (const row of all) {
    const scope = row.service_name ?? SHARED_ENV_SERVICE;
    const k = `${row.project_id}\0${scope}\0${row.key}`;
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
