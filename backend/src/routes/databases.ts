/**
 * Managed databases API — Coolify/Dokploy-style engine catalog + Dokku-style links.
 */
import { Router, type Request, type Response } from 'express';
import fs from 'fs';
import path from 'path';
import { spawnSync } from 'child_process';
import prisma from '../lib/prisma.js';
import { config } from '../lib/config.js';
import {
  composeProjectName,
  dockerSlug,
  serviceContainerName,
  shortPathHash,
  shortProjectId,
} from '../lib/naming.js';
import {
  defaultDatabaseCredentials,
  engineRuntimeEnv,
  getDatabaseEngine,
  isDatabaseEngineId,
  listDatabaseEngines,
} from '../lib/databaseEngines.js';
import {
  assertEnvKeyAvailable,
  connectionUrlFor,
  ensureDbOnAppNetwork,
  loadDatabaseCredentials,
  unlinkDatabaseLink,
  upsertLinkedEnv,
} from '../lib/databaseLinks.js';

const router = Router();

router.get('/engines', (_req: Request, res: Response) => {
  res.json(listDatabaseEngines());
});

router.get('/', async (_req: Request, res: Response) => {
  try {
    const projects = await prisma.project.findMany({
      where: { project_type: 'database' },
      include: {
        services: true,
        databaseLinksAsDb: {
          include: {
            app_project: { select: { id: true, name: true, status: true } },
          },
        },
      },
      orderBy: { created_at: 'desc' },
    });
    res.json(projects);
  } catch (error) {
    console.error(error);
    res.status(500).json({ error: 'Failed to list databases' });
  }
});

/** Links targeting an app project (for Project detail UI). Must be before /:id. */
router.get('/links/by-app/:appProjectId', async (req: Request, res: Response) => {
  try {
    const links = await prisma.databaseLink.findMany({
      where: { app_project_id: req.params.appProjectId },
      include: {
        database_project: {
          select: {
            id: true,
            name: true,
            status: true,
            db_engine: true,
          },
        },
      },
      orderBy: { created_at: 'desc' },
    });
    res.json(links);
  } catch (error) {
    console.error(error);
    res.status(500).json({ error: 'Failed to list app database links' });
  }
});

router.post('/', async (req: Request, res: Response) => {
  try {
    const nameTrim = typeof req.body?.name === 'string' ? req.body.name.trim() : '';
    if (!nameTrim || nameTrim.length > 120) {
      return res.status(400).json({ error: 'Name is required (max 120 characters)' });
    }
    const engineId = req.body?.engine;
    if (!isDatabaseEngineId(engineId)) {
      return res.status(400).json({
        error: `Invalid engine. Use one of: ${listDatabaseEngines().map((e) => e.id).join(', ')}`,
      });
    }
    const engine = getDatabaseEngine(engineId)!;
    const creds = defaultDatabaseCredentials(nameTrim);
    const runtimeEnv = engineRuntimeEnv(engine, creds);

    const project = await prisma.project.create({
      data: {
        name: nameTrim,
        description: `${engine.label} managed by DockLift`,
        source_type: 'managed',
        project_type: 'database',
        db_engine: engine.id,
        status: 'pending',
        auto_deploy: false,
        build_type: 'dockerfile',
        base_directory: '.',
        dockerfile_path: null,
        internal_port: engine.port,
        publish_host_port: false,
      },
    });

    const projectPath = path.join(config.deploymentsPath, project.id);
    fs.mkdirSync(projectPath, { recursive: true });
    fs.writeFileSync(
      path.join(projectPath, 'README.docklift-db.md'),
      `# ${nameTrim}\n\nManaged ${engine.label} (${engine.image}).\nDo not replace with application source.\n`,
      'utf8',
    );

    const containerName = serviceContainerName(project.name, project.id, engine.serviceName);
    const service = await prisma.service.create({
      data: {
        project_id: project.id,
        name: engine.serviceName,
        dockerfile_path: `[managed:${engine.id}]`,
        container_name: containerName,
        internal_port: engine.port,
        port: null,
        status: 'pending',
      },
    });

    for (const [key, value] of Object.entries(runtimeEnv)) {
      await prisma.envVariable.create({
        data: {
          project_id: project.id,
          service_name: '',
          key,
          value,
          is_runtime: true,
          is_build_arg: false,
          is_secret: key.toLowerCase().includes('password'),
        },
      });
    }

    const volumeLabel = 'data';
    const volumeName = `dl-${shortProjectId(project.id)}-${dockerSlug(volumeLabel, 28)}-${shortPathHash(volumeLabel)}`;
    const volume = await prisma.persistentVolume.create({
      data: {
        project_id: project.id,
        service_name: engine.serviceName,
        name: volumeName,
        display_name: volumeLabel,
        mount_path: engine.volumeMount,
      },
    });
    const volResult = spawnSync(
      'docker',
      [
        'volume',
        'create',
        '--label',
        `com.docker.compose.project=${composeProjectName(project.name, project.id)}`,
        '--label',
        `com.docklift.project=${project.id}`,
        '--label',
        'com.docklift.role=managed-database',
        volumeName,
      ],
      { encoding: 'utf8', shell: false, timeout: 30000 },
    );
    if (volResult.status !== 0) {
      await prisma.project.delete({ where: { id: project.id } }).catch(() => {});
      try {
        fs.rmSync(projectPath, { recursive: true, force: true });
      } catch {
        /* ignore */
      }
      return res.status(500).json({
        error: volResult.stderr?.trim() || 'Failed to create Docker volume for database',
      });
    }

    const connection_url = connectionUrlFor(engine, containerName, creds);

    res.status(201).json({
      ...project,
      services: [service],
      persistent_volumes: [volume],
      engine,
      connection_url,
      credentials: {
        username: creds.username || null,
        database: creds.database,
        // password returned once at create so the operator can copy it
        password: creds.password,
      },
    });
  } catch (error) {
    console.error(error);
    res.status(500).json({ error: 'Failed to create database' });
  }
});

router.get('/:id/connection', async (req: Request, res: Response) => {
  try {
    const project = await prisma.project.findFirst({
      where: { id: req.params.id, project_type: 'database' },
      include: { services: { take: 1 } },
    });
    const loaded = await loadDatabaseCredentials(req.params.id);
    if (!loaded || !project) {
      return res.status(404).json({ error: 'Managed database not found or credentials missing' });
    }
    const { engine, creds, host } = loaded;
    const publishHost = project.publish_host_port === true;
    const hostPort = project.services[0]?.port;
    const exposed = publishHost && typeof hostPort === 'number' && hostPort > 0;
    res.json({
      engine,
      host,
      port: engine.port,
      username: creds.username || null,
      database: creds.database,
      password: creds.password,
      connection_url: connectionUrlFor(engine, host, creds),
      internal_only: !exposed,
      publish_host_port: publishHost,
      host_port: exposed ? hostPort : null,
      // Product treats credentials as recreate-to-rotate (env edits blocked).
      credentials_init_only: true,
      note: exposed
        ? `Host port ${hostPort} is published — this database may be reachable on the server IP. Prefer linking apps over sharing IP:port.`
        : 'Reachable from linked apps on the app Docker network via the host above. Prefer linking over publishing host ports.',
    });
  } catch (error) {
    console.error(error);
    res.status(500).json({ error: 'Failed to load connection info' });
  }
});

router.get('/:id/links', async (req: Request, res: Response) => {
  try {
    const project = await prisma.project.findFirst({
      where: { id: req.params.id, project_type: 'database' },
    });
    if (!project) return res.status(404).json({ error: 'Database not found' });
    const links = await prisma.databaseLink.findMany({
      where: { database_project_id: project.id },
      include: {
        app_project: {
          select: {
            id: true,
            name: true,
            status: true,
            services: { select: { id: true, name: true, status: true } },
          },
        },
      },
      orderBy: { created_at: 'desc' },
    });
    res.json(links);
  } catch (error) {
    console.error(error);
    res.status(500).json({ error: 'Failed to list links' });
  }
});

router.post('/:id/links', async (req: Request, res: Response) => {
  try {
    const dbProject = await prisma.project.findFirst({
      where: { id: req.params.id, project_type: 'database' },
    });
    if (!dbProject || !dbProject.db_engine) {
      return res.status(404).json({ error: 'Database not found' });
    }
    const engine = getDatabaseEngine(dbProject.db_engine);
    if (!engine) return res.status(400).json({ error: 'Unknown database engine' });

    const appProjectId = typeof req.body?.app_project_id === 'string' ? req.body.app_project_id.trim() : '';
    if (!appProjectId) {
      return res.status(400).json({ error: 'app_project_id is required' });
    }
    const app = await prisma.project.findFirst({
      where: { id: appProjectId, project_type: { not: 'database' } },
      include: { services: true },
    });
    if (!app) {
      return res.status(404).json({ error: 'App project not found' });
    }

    let serviceName =
      typeof req.body?.service_name === 'string' ? req.body.service_name.trim() : '';
    if (serviceName) {
      const ok = app.services.some((s) => s.name === serviceName);
      if (!ok) {
        return res.status(400).json({ error: `Service "${serviceName}" not found on that project` });
      }
    } else {
      serviceName = '';
    }

    const envKey =
      typeof req.body?.env_key === 'string' && req.body.env_key.trim()
        ? req.body.env_key.trim()
        : engine.defaultEnvKey;
    if (!/^[A-Za-z_][A-Za-z0-9_]*$/.test(envKey) || envKey.length > 64) {
      return res.status(400).json({ error: 'Invalid env_key' });
    }

    const loaded = await loadDatabaseCredentials(dbProject.id);
    if (!loaded) {
      return res.status(400).json({
        error: 'Database credentials missing — deploy the database first',
      });
    }

    if (dbProject.status !== 'running' && dbProject.status !== 'degraded') {
      return res.status(409).json({
        error: 'Database must be running before linking. Deploy it first.',
      });
    }

    const overwrite = req.body?.overwrite === true;
    try {
      await assertEnvKeyAvailable({
        appProjectId: app.id,
        serviceName,
        envKey,
        databaseProjectId: dbProject.id,
        overwrite,
      });
    } catch (err) {
      const status = (err as { statusCode?: number }).statusCode || 400;
      return res.status(status).json({
        error: err instanceof Error ? err.message : 'Env key conflict',
      });
    }

    const url = connectionUrlFor(loaded.engine, loaded.host, loaded.creds);

    // Link row first (ownership), then env — avoids orphan env if create fails mid-way.
    const link = await prisma.databaseLink.upsert({
      where: {
        database_project_id_app_project_id_service_name_env_key: {
          database_project_id: dbProject.id,
          app_project_id: app.id,
          service_name: serviceName,
          env_key: envKey,
        },
      },
      create: {
        database_project_id: dbProject.id,
        app_project_id: app.id,
        service_name: serviceName,
        env_key: envKey,
      },
      update: {},
    });

    await upsertLinkedEnv({
      appProjectId: app.id,
      serviceName,
      envKey,
      value: url,
    });

    let networkAttached = true;
    let networkError: string | null = null;
    try {
      await ensureDbOnAppNetwork(dbProject.id, app.id);
    } catch (err) {
      networkAttached = false;
      networkError =
        err instanceof Error
          ? err.message
          : 'Could not attach database to the app network yet';
    }

    res.status(201).json({
      link,
      env_key: envKey,
      service_name: serviceName || null,
      network_attached: networkAttached,
      network_error: networkError,
      note: networkAttached
        ? 'Connection URL injected as a runtime secret. Redeploy the app for containers to pick up the new env.'
        : `Connection URL saved. Network attach pending (${networkError}). Redeploy the app (and ensure the database is running) to join networks.`,
    });
  } catch (error: unknown) {
    console.error(error);
    const code = (error as { code?: string })?.code;
    if (code === 'P2002') {
      return res.status(409).json({ error: 'This link already exists' });
    }
    res.status(500).json({ error: 'Failed to link database' });
  }
});

router.delete('/:id/links/:linkId', async (req: Request, res: Response) => {
  try {
    const link = await prisma.databaseLink.findFirst({
      where: {
        id: req.params.linkId,
        database_project_id: req.params.id,
      },
    });
    if (!link) return res.status(404).json({ error: 'Link not found' });

    await unlinkDatabaseLink(link.id);

    res.json({
      status: 'unlinked',
      note: 'Env removed if no other database owns that key. Redeploy the app to drop it from running containers.',
    });
  } catch (error) {
    console.error(error);
    res.status(500).json({ error: 'Failed to unlink database' });
  }
});

export default router;
