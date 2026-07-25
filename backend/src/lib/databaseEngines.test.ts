import { describe, expect, test } from 'bun:test';
import {
  buildConnectionUrl,
  credentialsFromEnvMap,
  defaultDatabaseCredentials,
  engineCommand,
  engineRuntimeEnv,
  getDatabaseEngine,
  isDatabaseEngineId,
  listDatabaseEngines,
} from './databaseEngines.js';

describe('databaseEngines', () => {
  test('lists five engines', () => {
    expect(listDatabaseEngines()).toHaveLength(5);
    expect(isDatabaseEngineId('postgres')).toBe(true);
    expect(isDatabaseEngineId('sqlite')).toBe(false);
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
