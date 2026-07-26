import { describe, expect, test } from 'bun:test';
import {
  LEGACY_MANAGED_IMAGES,
  buildConnectionUrl,
  credentialsFromEnvMap,
  defaultDatabaseCredentials,
  engineCommand,
  engineRuntimeEnv,
  getDatabaseEngine,
  imageForManagedService,
  isDatabaseEngineId,
  listDatabaseEngines,
  managedServiceMarker,
  parseManagedServiceMarker,
  resolveEngineImage,
  volumeMountForEngine,
} from './databaseEngines.js';

describe('databaseEngines', () => {
  test('lists five engines with versions', () => {
    expect(listDatabaseEngines()).toHaveLength(5);
    expect(isDatabaseEngineId('postgres')).toBe(true);
    expect(isDatabaseEngineId('sqlite')).toBe(false);
    const pg = getDatabaseEngine('postgres')!;
    expect(pg.versions.length).toBeGreaterThan(1);
    expect(pg.image).toMatch(/^postgres:\d/);
  });

  test('resolveEngineImage validates tags', () => {
    const pg = getDatabaseEngine('postgres')!;
    expect(resolveEngineImage(pg, '15-alpine')).toEqual({
      image: 'postgres:15-alpine',
      tag: '15-alpine',
    });
    expect(resolveEngineImage(pg, 'nope')).toHaveProperty('error');
  });

  test('managed service marker round-trip', () => {
    const marker = managedServiceMarker('redis', 'redis:7-alpine');
    expect(parseManagedServiceMarker(marker)).toEqual({
      engineId: 'redis',
      image: 'redis:7-alpine',
    });
    const engine = getDatabaseEngine('redis')!;
    expect(imageForManagedService(engine, marker)).toBe('redis:7-alpine');
    // Bare marker must pin legacy image — never silent major bump to engine.image
    expect(imageForManagedService(engine, '[managed:redis]')).toBe(
      LEGACY_MANAGED_IMAGES.redis,
    );
    expect(LEGACY_MANAGED_IMAGES.redis).not.toBe(engine.image);
  });

  test('postgres volume mount is version-aware (18+ vs ≤17)', () => {
    const pg = getDatabaseEngine('postgres')!;
    expect(volumeMountForEngine(pg, 'postgres:16-alpine')).toBe(
      '/var/lib/postgresql/data',
    );
    expect(volumeMountForEngine(pg, 'postgres:17')).toBe(
      '/var/lib/postgresql/data',
    );
    expect(volumeMountForEngine(pg, 'postgres:18-alpine')).toBe(
      '/var/lib/postgresql',
    );
    expect(volumeMountForEngine(pg, 'postgres:18')).toBe('/var/lib/postgresql');
    expect(volumeMountForEngine(pg, 'postgres:latest')).toBe(
      '/var/lib/postgresql',
    );
    expect(volumeMountForEngine(getDatabaseEngine('mysql')!, 'mysql:8.4')).toBe(
      '/var/lib/mysql',
    );
  });

  test('legacy bare markers pin pre-version defaults', () => {
    expect(
      imageForManagedService(getDatabaseEngine('postgres')!, '[managed:postgres]'),
    ).toBe('postgres:16-alpine');
    expect(
      imageForManagedService(getDatabaseEngine('mongodb')!, '[managed:mongodb]'),
    ).toBe('mongo:7');
  });

  test('builds postgres URL and runtime env', () => {
    const engine = getDatabaseEngine('postgres')!;
    const creds = { username: 'docklift', password: 's3cret!', database: 'myapp' };
    expect(engineRuntimeEnv(engine, creds)).toEqual({
      POSTGRES_USER: 'docklift',
      POSTGRES_PASSWORD: 's3cret!',
      POSTGRES_DB: 'myapp',
    });
    expect(buildConnectionUrl(engine, 'dl_myapp_aaaaaaaa_db', creds)).toBe(
      'postgresql://docklift:s3cret!@dl_myapp_aaaaaaaa_db:5432/myapp',
    );
  });

  test('redis command and URL', () => {
    const engine = getDatabaseEngine('redis')!;
    const creds = { username: '', password: 'p@ss', database: '0' };
    expect(engineCommand(engine, creds)).toEqual([
      'redis-server',
      '--requirepass',
      'p@ss',
      '--appendonly',
      'yes',
    ]);
    expect(buildConnectionUrl(engine, 'dbhost', creds)).toBe('redis://:p%40ss@dbhost:6379');
  });

  test('credentialsFromEnvMap round-trip', () => {
    const engine = getDatabaseEngine('mysql')!;
    const creds = defaultDatabaseCredentials('Shop App', 'pw');
    const env = engineRuntimeEnv(engine, creds);
    expect(credentialsFromEnvMap(engine, env)).toEqual({
      username: creds.username,
      password: creds.password,
      database: creds.database,
    });
  });
});
