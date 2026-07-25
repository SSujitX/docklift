/**
 * Managed database engines (Coolify/Dokploy-style).
 * Official images + preset volumes + connection URL builders.
 */

import { randomBytes } from 'crypto';
import { dockerSlug } from './naming.js';

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

export interface DatabaseEngineDef {
  id: DatabaseEngineId;
  label: string;
  description: string;
  /** Official Docker Hub image tag */
  image: string;
  /** Container listen port */
  port: number;
  /** Named-volume mount path inside the container */
  volumeMount: string;
  /** Env key injected into linked apps */
  defaultEnvKey: string;
  /** Docker Compose / Prisma service name */
  serviceName: string;
}

export const DATABASE_ENGINES: Record<DatabaseEngineId, DatabaseEngineDef> = {
  postgres: {
    id: 'postgres',
    label: 'PostgreSQL',
    description: 'Relational database — Prisma, Rails, Django, and most apps.',
    image: 'postgres:16-alpine',
    port: 5432,
    volumeMount: '/var/lib/postgresql/data',
    defaultEnvKey: 'DATABASE_URL',
    serviceName: 'db',
  },
  mysql: {
    id: 'mysql',
    label: 'MySQL',
    description: 'Popular relational database for LAMP and many frameworks.',
    image: 'mysql:8.4',
    port: 3306,
    volumeMount: '/var/lib/mysql',
    defaultEnvKey: 'DATABASE_URL',
    serviceName: 'db',
  },
  mariadb: {
    id: 'mariadb',
    label: 'MariaDB',
    description: 'MySQL-compatible relational database.',
    image: 'mariadb:11',
    port: 3306,
    volumeMount: '/var/lib/mysql',
    defaultEnvKey: 'DATABASE_URL',
    serviceName: 'db',
  },
  redis: {
    id: 'redis',
    label: 'Redis',
    description: 'In-memory cache, queues, and sessions.',
    image: 'redis:7-alpine',
    port: 6379,
    volumeMount: '/data',
    defaultEnvKey: 'REDIS_URL',
    serviceName: 'db',
  },
  mongodb: {
    id: 'mongodb',
    label: 'MongoDB',
    description: 'Document database for flexible JSON-like data.',
    image: 'mongo:7',
    port: 27017,
    volumeMount: '/data/db',
    defaultEnvKey: 'MONGODB_URI',
    serviceName: 'db',
  },
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
