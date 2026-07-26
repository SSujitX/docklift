/**
 * Managed database engines (Coolify/Dokploy-style).
 * Official images + preset volumes + connection URL builders.
 */

import { randomBytes } from 'crypto';
import { dockerSlug } from './naming.js';
import { matchesEngineTagPattern } from './dockerHubTags.js';

export type DatabaseEngineId =
  | 'postgres'
  | 'mysql'
  | 'mariadb'
  | 'redis'
  | 'mongodb';

export interface DatabaseCredentials {
  username: string;
  password: string;
  database: string;
}

export interface DatabaseEngineVersion {
  /** Docker tag (without repository), e.g. `16-alpine` */
  tag: string;
  /** Operator-facing label */
  label: string;
  /** Default selection in New Database UI */
  recommended?: boolean;
}

export interface DatabaseEngineDef {
  id: DatabaseEngineId;
  label: string;
  description: string;
  /** Docker Hub repository (no tag) */
  imageRepo: string;
  /**
   * Fallback tags when Docker Hub is unreachable.
   * Live catalog is filled from Hub via `listDatabaseEnginesWithLiveVersions`.
   * First `recommended: true` (else first entry) is the create default.
   */
  versions: DatabaseEngineVersion[];
  /** Default full image (`repo:tag`) — derived from recommended version */
  image: string;
  /** Container listen port */
  port: number;
  /**
   * Named-volume mount path inside the container.
   * For Postgres this is the ≤17 path; use `volumeMountForEngine` for the
   * selected image (18+ mounts `/var/lib/postgresql` per official image).
   */
  volumeMount: string;
  /** Env key injected into linked apps */
  defaultEnvKey: string;
  /** Docker Compose / Prisma service name */
  serviceName: string;
}

/**
 * Images for bare `[managed:engine]` markers created before version pinning.
 * Must stay on the majors that shipped with that marker — never "current recommended".
 */
export const LEGACY_MANAGED_IMAGES: Record<DatabaseEngineId, string> = {
  postgres: 'postgres:16-alpine',
  mysql: 'mysql:8.4',
  mariadb: 'mariadb:11',
  redis: 'redis:7-alpine',
  mongodb: 'mongo:7',
};

/** Parse leading major from a Docker tag (`18-alpine` → 18, `latest` → null). */
export function majorFromImageTag(imageOrTag: string): number | null {
  const tag = imageOrTag.includes(':')
    ? imageOrTag.slice(imageOrTag.lastIndexOf(':') + 1)
    : imageOrTag;
  if (!tag || tag === 'latest') return null;
  const m = tag.match(/^(\d+)/);
  if (!m) return null;
  const n = Number(m[1]);
  return Number.isFinite(n) ? n : null;
}

/**
 * Container path for the named data volume.
 * Postgres 18+ official images declare VOLUME at `/var/lib/postgresql` (PGDATA
 * under `$PG_MAJOR/docker`). Mounting `/var/lib/postgresql/data` leaves real
 * data on an anonymous volume that disappears on recreate.
 */
export function volumeMountForEngine(
  engine: DatabaseEngineDef,
  imageOrTag?: string | null,
): string {
  if (engine.id !== 'postgres') return engine.volumeMount;
  const ref = imageOrTag || engine.image;
  const tag = ref.includes(':') ? ref.slice(ref.lastIndexOf(':') + 1) : ref;
  // `latest` tracks current Hub default (18+); use the new mount.
  if (tag === 'latest') return '/var/lib/postgresql';
  const major = majorFromImageTag(tag);
  if (major != null && major >= 18) return '/var/lib/postgresql';
  return '/var/lib/postgresql/data';
}

function engine(
  partial: Omit<DatabaseEngineDef, 'image'> & { versions: DatabaseEngineVersion[] },
): DatabaseEngineDef {
  const recommended =
    partial.versions.find((v) => v.recommended) || partial.versions[0];
  if (!recommended) {
    throw new Error(`Engine ${partial.id} must declare at least one version`);
  }
  return {
    ...partial,
    image: `${partial.imageRepo}:${recommended.tag}`,
  };
}

export const DATABASE_ENGINES: Record<DatabaseEngineId, DatabaseEngineDef> = {
  postgres: engine({
    id: 'postgres',
    label: 'PostgreSQL',
    description: 'Relational database — Prisma, Rails, Django, and most apps.',
    imageRepo: 'postgres',
    versions: [
      { tag: '18-alpine', label: '18 · Alpine', recommended: true },
      { tag: '17-alpine', label: '17 · Alpine' },
      { tag: '16-alpine', label: '16 · Alpine' },
      { tag: '15-alpine', label: '15 · Alpine' },
      { tag: '14-alpine', label: '14 · Alpine' },
      { tag: '18', label: '18' },
      { tag: '17', label: '17' },
      { tag: '16', label: '16' },
    ],
    port: 5432,
    volumeMount: '/var/lib/postgresql/data',
    defaultEnvKey: 'DATABASE_URL',
    serviceName: 'db',
  }),
  mysql: engine({
    id: 'mysql',
    label: 'MySQL',
    description: 'Popular relational database for LAMP and many frameworks.',
    imageRepo: 'mysql',
    versions: [
      { tag: '8.4', label: '8.4 LTS', recommended: true },
      { tag: '8.0', label: '8.0' },
      { tag: '8.4-oracle', label: '8.4 · Oracle Linux' },
      { tag: 'latest', label: 'latest' },
    ],
    port: 3306,
    volumeMount: '/var/lib/mysql',
    defaultEnvKey: 'DATABASE_URL',
    serviceName: 'db',
  }),
  mariadb: engine({
    id: 'mariadb',
    label: 'MariaDB',
    description: 'MySQL-compatible relational database.',
    imageRepo: 'mariadb',
    versions: [
      { tag: '11', label: '11', recommended: true },
      { tag: '11.4', label: '11.4 LTS' },
      { tag: '10.11', label: '10.11 LTS' },
      { tag: 'latest', label: 'latest' },
    ],
    port: 3306,
    volumeMount: '/var/lib/mysql',
    defaultEnvKey: 'DATABASE_URL',
    serviceName: 'db',
  }),
  redis: engine({
    id: 'redis',
    label: 'Redis',
    description: 'In-memory cache, queues, and sessions.',
    imageRepo: 'redis',
    versions: [
      { tag: '8-alpine', label: '8 · Alpine', recommended: true },
      { tag: '7-alpine', label: '7 · Alpine' },
      { tag: '7', label: '7' },
      { tag: '6-alpine', label: '6 · Alpine' },
      { tag: 'latest', label: 'latest' },
    ],
    port: 6379,
    volumeMount: '/data',
    defaultEnvKey: 'REDIS_URL',
    serviceName: 'db',
  }),
  mongodb: engine({
    id: 'mongodb',
    label: 'MongoDB',
    description: 'Document database for flexible JSON-like data.',
    imageRepo: 'mongo',
    versions: [
      { tag: '8', label: '8', recommended: true },
      { tag: '7', label: '7' },
      { tag: '6', label: '6' },
      { tag: 'latest', label: 'latest' },
    ],
    port: 27017,
    volumeMount: '/data/db',
    defaultEnvKey: 'MONGODB_URI',
    serviceName: 'db',
  }),
};

export const DATABASE_ENGINE_IDS = Object.keys(DATABASE_ENGINES) as DatabaseEngineId[];

export function isDatabaseEngineId(value: unknown): value is DatabaseEngineId {
  return typeof value === 'string' && value in DATABASE_ENGINES;
}

export function getDatabaseEngine(id: string): DatabaseEngineDef | null {
  if (!isDatabaseEngineId(id)) return null;
  return DATABASE_ENGINES[id];
}

export function generateDatabasePassword(): string {
  return randomBytes(24).toString('base64url');
}

export function defaultDatabaseCredentials(
  projectName: string,
  password = generateDatabasePassword(),
): DatabaseCredentials {
  const base = dockerSlug(projectName, 16).replace(/-/g, '_') || 'app';
  return {
    username: 'docklift',
    password,
    database: base,
  };
}

/** Runtime env vars for the database container itself. */
export function engineRuntimeEnv(
  engine: DatabaseEngineDef,
  creds: DatabaseCredentials,
): Record<string, string> {
  switch (engine.id) {
    case 'postgres':
      return {
        POSTGRES_USER: creds.username,
        POSTGRES_PASSWORD: creds.password,
        POSTGRES_DB: creds.database,
      };
    case 'mysql':
    case 'mariadb':
      return {
        MYSQL_USER: creds.username,
        MYSQL_PASSWORD: creds.password,
        MYSQL_DATABASE: creds.database,
        MYSQL_ROOT_PASSWORD: creds.password,
      };
    case 'redis':
      // requirepass via command is set in compose; env documents the secret
      return {
        REDIS_PASSWORD: creds.password,
      };
    case 'mongodb':
      return {
        MONGO_INITDB_ROOT_USERNAME: creds.username,
        MONGO_INITDB_ROOT_PASSWORD: creds.password,
        MONGO_INITDB_DATABASE: creds.database,
      };
    default:
      return {};
  }
}

/**
 * Connection URL for apps on a shared Docker network.
 * `host` must be the DB container_name (Docker DNS).
 */
export function buildConnectionUrl(
  engine: DatabaseEngineDef,
  host: string,
  creds: DatabaseCredentials,
): string {
  const user = encodeURIComponent(creds.username);
  const pass = encodeURIComponent(creds.password);
  const db = encodeURIComponent(creds.database);
  switch (engine.id) {
    case 'postgres':
      return `postgresql://${user}:${pass}@${host}:${engine.port}/${db}`;
    case 'mysql':
    case 'mariadb':
      return `mysql://${user}:${pass}@${host}:${engine.port}/${db}`;
    case 'redis':
      return `redis://:${pass}@${host}:${engine.port}`;
    case 'mongodb':
      return `mongodb://${user}:${pass}@${host}:${engine.port}/${db}?authSource=admin`;
    default:
      throw new Error(`Unsupported engine: ${(engine as DatabaseEngineDef).id}`);
  }
}

/** Extra compose command args (e.g. Redis requirepass). */
export function engineCommand(engine: DatabaseEngineDef, creds: DatabaseCredentials): string[] | null {
  if (engine.id === 'redis') {
    return ['redis-server', '--requirepass', creds.password, '--appendonly', 'yes'];
  }
  return null;
}

export function credentialsFromEnvMap(
  engine: DatabaseEngineDef,
  env: Record<string, string>,
): DatabaseCredentials | null {
  switch (engine.id) {
    case 'postgres': {
      const username = env.POSTGRES_USER;
      const password = env.POSTGRES_PASSWORD;
      const database = env.POSTGRES_DB;
      if (!username || !password || !database) return null;
      return { username, password, database };
    }
    case 'mysql':
    case 'mariadb': {
      const username = env.MYSQL_USER;
      const password = env.MYSQL_PASSWORD;
      const database = env.MYSQL_DATABASE;
      if (!username || !password || !database) return null;
      return { username, password, database };
    }
    case 'redis': {
      const password = env.REDIS_PASSWORD;
      if (!password) return null;
      return { username: '', password, database: '0' };
    }
    case 'mongodb': {
      const username = env.MONGO_INITDB_ROOT_USERNAME;
      const password = env.MONGO_INITDB_ROOT_PASSWORD;
      const database = env.MONGO_INITDB_DATABASE || 'admin';
      if (!username || !password) return null;
      return { username, password, database };
    }
    default:
      return null;
  }
}

export function listDatabaseEngines(): DatabaseEngineDef[] {
  return DATABASE_ENGINE_IDS.map((id) => DATABASE_ENGINES[id]);
}

/** Catalog with versions refreshed from Docker Hub (cached). */
export async function listDatabaseEnginesWithLiveVersions(): Promise<
  DatabaseEngineDef[]
> {
  const { fetchLiveEngineVersions } = await import('./dockerHubTags.js');
  return Promise.all(
    DATABASE_ENGINE_IDS.map(async (id) => {
      const engine = DATABASE_ENGINES[id];
      const versions = await fetchLiveEngineVersions(
        id,
        engine.imageRepo,
        engine.versions,
      );
      const recommended = versions.find((v) => v.recommended) || versions[0];
      return {
        ...engine,
        versions,
        image: recommended
          ? `${engine.imageRepo}:${recommended.tag}`
          : engine.image,
      };
    }),
  );
}

/**
 * Resolve a full image from an engine + optional tag.
 * Accepts catalog tags, or any tag matching the engine's Hub pattern
 * (so create works even if the live list is briefly stale).
 */
export function resolveEngineImage(
  engine: DatabaseEngineDef,
  tag?: string | null,
  options?: { allowedTags?: string[] },
): { image: string; tag: string } | { error: string } {
  const catalog = options?.allowedTags?.length
    ? options.allowedTags
    : engine.versions.map((v) => v.tag);
  const recommendedTag =
    engine.versions.find((v) => v.recommended)?.tag ||
    catalog[0] ||
    engine.versions[0]?.tag;
  if (!recommendedTag) {
    return { error: `No versions configured for ${engine.id}` };
  }
  if (tag == null || tag === '') {
    return {
      image: `${engine.imageRepo}:${recommendedTag}`,
      tag: recommendedTag,
    };
  }
  const normalized = String(tag).trim();
  // Accept bare tag or full `repo:tag` for the same repo
  const bare = normalized.startsWith(`${engine.imageRepo}:`)
    ? normalized.slice(engine.imageRepo.length + 1)
    : normalized;

  const inCatalog = catalog.includes(bare);
  const patternOk = matchesEngineTagPattern(engine.id, bare);

  if (!inCatalog && !patternOk) {
    const sample = catalog.slice(0, 12).join(', ');
    return {
      error: `Invalid version for ${engine.label}. Try one of: ${sample}${catalog.length > 12 ? '…' : ''}`,
    };
  }
  return { image: `${engine.imageRepo}:${bare}`, tag: bare };
}

/** Persist selection on the service row: `[managed:postgres|postgres:16-alpine]` */
export function managedServiceMarker(engineId: DatabaseEngineId, image: string): string {
  return `[managed:${engineId}|${image}]`;
}

export function parseManagedServiceMarker(
  dockerfilePath: string | null | undefined,
): { engineId: string; image: string | null } | null {
  if (!dockerfilePath) return null;
  const m = dockerfilePath.match(/^\[managed:([^\]|]+)(?:\|([^\]]+))?\]$/);
  if (!m) return null;
  return { engineId: m[1], image: m[2] || null };
}

/**
 * Prefer image stored on the service.
 * Bare `[managed:engine]` (pre-pin) → LEGACY_MANAGED_IMAGES, never the live recommended major.
 */
export function imageForManagedService(
  engine: DatabaseEngineDef,
  dockerfilePath: string | null | undefined,
): string {
  const parsed = parseManagedServiceMarker(dockerfilePath);
  if (parsed?.image) return parsed.image;
  return LEGACY_MANAGED_IMAGES[engine.id] || engine.image;
}
