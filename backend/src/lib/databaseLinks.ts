/**
 * Helpers for managed DB ↔ app linking (network join + scoped env inject).
 */

import prisma from './prisma.js';
import {
  buildConnectionUrl,
  credentialsFromEnvMap,
  getDatabaseEngine,
  type DatabaseEngineDef,
  type DatabaseCredentials,
} from './databaseEngines.js';
import {
  connectContainerToProjectNetwork,
  disconnectContainerFromProjectNetwork,
} from '../services/docker.js';

export async function loadDatabaseCredentials(
  databaseProjectId: string,
): Promise<{ engine: DatabaseEngineDef; creds: DatabaseCredentials; host: string } | null> {
  const project = await prisma.project.findUnique({
    where: { id: databaseProjectId },
    include: {
      env_variables: true,
      services: { take: 1, orderBy: { created_at: 'asc' } },
    },
  });
  if (!project || project.project_type !== 'database' || !project.db_engine) return null;
  const engine = getDatabaseEngine(project.db_engine);
  if (!engine) return null;
  const envMap = Object.fromEntries(
    project.env_variables.map((v) => [v.key, v.value]),
  );
  const creds = credentialsFromEnvMap(engine, envMap);
  if (!creds) return null;
  const host = project.services[0]?.container_name;
  if (!host) return null;
  return { engine, creds, host };
}

export function connectionUrlFor(
  engine: DatabaseEngineDef,
  host: string,
  creds: DatabaseCredentials,
): string {
  return buildConnectionUrl(engine, host, creds);
}

/** Ensure DB container is attached to the app project network. */
export async function ensureDbOnAppNetwork(
  databaseProjectId: string,
  appProjectId: string,
): Promise<void> {
  const svc = await prisma.service.findFirst({
    where: { project_id: databaseProjectId },
    orderBy: { created_at: 'asc' },
  });
  if (!svc?.container_name) {
    throw new Error('Database container is not ready yet — deploy the database first');
  }
  await connectContainerToProjectNetwork(appProjectId, svc.container_name);
}

/** Disconnect DB from app network if no remaining links to that app. */
export async function maybeDisconnectDbFromAppNetwork(
  databaseProjectId: string,
  appProjectId: string,
): Promise<void> {
  const remaining = await prisma.databaseLink.count({
    where: {
      database_project_id: databaseProjectId,
      app_project_id: appProjectId,
    },
  });
  if (remaining > 0) return;
  const svc = await prisma.service.findFirst({
    where: { project_id: databaseProjectId },
    orderBy: { created_at: 'asc' },
  });
  if (!svc?.container_name) return;
  await disconnectContainerFromProjectNetwork(appProjectId, svc.container_name);
}

/**
 * One link owns each (app, service_name, env_key). Reject if another DB already owns it,
 * or if a non-linked env value already exists (unless overwrite).
 */
export async function assertEnvKeyAvailable(opts: {
  appProjectId: string;
  serviceName: string;
  envKey: string;
  databaseProjectId: string;
  overwrite?: boolean;
}): Promise<void> {
  const other = await prisma.databaseLink.findFirst({
    where: {
      app_project_id: opts.appProjectId,
      service_name: opts.serviceName,
      env_key: opts.envKey,
      NOT: { database_project_id: opts.databaseProjectId },
    },
    include: { database_project: { select: { name: true } } },
  });
  if (other) {
    throw Object.assign(
      new Error(
        `"${opts.envKey}" on this project/service is already linked to database "${other.database_project.name}". Unlink that first.`,
      ),
      { statusCode: 409 },
    );
  }

  const existingEnv = await prisma.envVariable.findFirst({
    where: {
      project_id: opts.appProjectId,
      service_name: opts.serviceName,
      key: opts.envKey,
    },
  });
  if (!existingEnv) return;

  const ownedByThisDb = await prisma.databaseLink.findFirst({
    where: {
      database_project_id: opts.databaseProjectId,
      app_project_id: opts.appProjectId,
      service_name: opts.serviceName,
      env_key: opts.envKey,
    },
  });
  if (ownedByThisDb) return;

  if (!opts.overwrite) {
    throw Object.assign(
      new Error(
        `"${opts.envKey}" already exists on this project/service. Pass overwrite=true to replace it with the managed database URL.`,
      ),
      { statusCode: 409 },
    );
  }
}

/** Disconnect every linked DB container from an app network (before compose down). */
export async function disconnectLinkedDatabasesFromApp(
  appProjectId: string,
): Promise<void> {
  const links = await prisma.databaseLink.findMany({
    where: { app_project_id: appProjectId },
    select: { database_project_id: true },
  });
  const dbIds = [...new Set(links.map((l) => l.database_project_id))];
  for (const dbId of dbIds) {
    const svc = await prisma.service.findFirst({
      where: { project_id: dbId },
      orderBy: { created_at: 'asc' },
    });
    if (!svc?.container_name) continue;
    await disconnectContainerFromProjectNetwork(appProjectId, svc.container_name);
  }
}

/** Upsert the connection URL env on the app project (scoped or shared). */
export async function upsertLinkedEnv(opts: {
  appProjectId: string;
  serviceName: string;
  envKey: string;
  value: string;
}): Promise<void> {
  const existing = await prisma.envVariable.findFirst({
    where: {
      project_id: opts.appProjectId,
      service_name: opts.serviceName,
      key: opts.envKey,
    },
  });
  if (existing) {
    await prisma.envVariable.update({
      where: { id: existing.id },
      data: {
        value: opts.value,
        is_runtime: true,
        is_build_arg: false,
        is_secret: true,
      },
    });
    return;
  }
  await prisma.envVariable.create({
    data: {
      project_id: opts.appProjectId,
      service_name: opts.serviceName,
      key: opts.envKey,
      value: opts.value,
      is_runtime: true,
      is_build_arg: false,
      is_secret: true,
    },
  });
}

/**
 * Remove injected env only when no remaining DatabaseLink owns that key
 * (any database) on the same app/service scope.
 */
export async function deleteLinkedEnvIfUnowned(opts: {
  appProjectId: string;
  serviceName: string;
  envKey: string;
}): Promise<boolean> {
  const remaining = await prisma.databaseLink.count({
    where: {
      app_project_id: opts.appProjectId,
      service_name: opts.serviceName,
      env_key: opts.envKey,
    },
  });
  if (remaining > 0) return false;
  await prisma.envVariable.deleteMany({
    where: {
      project_id: opts.appProjectId,
      service_name: opts.serviceName,
      key: opts.envKey,
    },
  });
  return true;
}

/** Unlink one row: delete link, maybe env, maybe network. */
export async function unlinkDatabaseLink(linkId: string): Promise<void> {
  const link = await prisma.databaseLink.findUnique({ where: { id: linkId } });
  if (!link) return;
  await prisma.databaseLink.delete({ where: { id: link.id } });
  await deleteLinkedEnvIfUnowned({
    appProjectId: link.app_project_id,
    serviceName: link.service_name,
    envKey: link.env_key,
  });
  await maybeDisconnectDbFromAppNetwork(link.database_project_id, link.app_project_id);
}

/** Before deleting a managed DB project — clean peer app envs + networks. */
export async function cleanupLinksForDatabaseProject(databaseProjectId: string): Promise<void> {
  const links = await prisma.databaseLink.findMany({
    where: { database_project_id: databaseProjectId },
  });
  for (const link of links) {
    await prisma.databaseLink.delete({ where: { id: link.id } });
    await deleteLinkedEnvIfUnowned({
      appProjectId: link.app_project_id,
      serviceName: link.service_name,
      envKey: link.env_key,
    });
    await maybeDisconnectDbFromAppNetwork(databaseProjectId, link.app_project_id);
  }
}

/** Refresh connection URLs on all apps linked to this DB (after DB redeploy). */
export async function refreshLinkedEnvForDatabase(databaseProjectId: string): Promise<void> {
  const loaded = await loadDatabaseCredentials(databaseProjectId);
  if (!loaded) return;
  const url = connectionUrlFor(loaded.engine, loaded.host, loaded.creds);
  const links = await prisma.databaseLink.findMany({
    where: { database_project_id: databaseProjectId },
  });
  for (const link of links) {
    await upsertLinkedEnv({
      appProjectId: link.app_project_id,
      serviceName: link.service_name,
      envKey: link.env_key,
      value: url,
    });
  }
}

/** After DB deploy: re-attach networks + refresh injected URLs. */
export async function reapplyDatabaseLinksForDatabase(databaseProjectId: string): Promise<void> {
  await refreshLinkedEnvForDatabase(databaseProjectId);
  const links = await prisma.databaseLink.findMany({
    where: { database_project_id: databaseProjectId },
    select: { app_project_id: true },
  });
  const apps = [...new Set(links.map((l) => l.app_project_id))];
  for (const appId of apps) {
    try {
      await ensureDbOnAppNetwork(databaseProjectId, appId);
    } catch (err) {
      console.warn(
        `[databaseLinks] Failed to attach DB ${databaseProjectId} to app ${appId}:`,
        err,
      );
    }
  }
}

/** Re-attach linked DBs to an app network. Returns attach counts for honest logging. */
export async function reapplyDatabaseLinksForApp(
  appProjectId: string,
): Promise<{ ok: number; failed: number }> {
  const links = await prisma.databaseLink.findMany({
    where: { app_project_id: appProjectId },
    select: { database_project_id: true },
  });
  const dbs = [...new Set(links.map((l) => l.database_project_id))];
  let ok = 0;
  let failed = 0;
  for (const dbId of dbs) {
    try {
      await ensureDbOnAppNetwork(dbId, appProjectId);
      ok += 1;
    } catch (err) {
      failed += 1;
      console.warn(
        `[databaseLinks] Failed to attach DB ${dbId} to app ${appProjectId}:`,
        err,
      );
    }
  }
  return { ok, failed };
}

/** Credential env keys that official images only honor on first volume init. */
export function managedCredentialEnvKeys(engineId: string): string[] {
  switch (engineId) {
    case 'postgres':
      return ['POSTGRES_USER', 'POSTGRES_PASSWORD', 'POSTGRES_DB'];
    case 'mysql':
    case 'mariadb':
      return ['MYSQL_USER', 'MYSQL_PASSWORD', 'MYSQL_DATABASE', 'MYSQL_ROOT_PASSWORD'];
    case 'mongodb':
      return [
        'MONGO_INITDB_ROOT_USERNAME',
        'MONGO_INITDB_ROOT_PASSWORD',
        'MONGO_INITDB_DATABASE',
      ];
    case 'redis':
      return ['REDIS_PASSWORD'];
    default:
      return [];
  }
}
