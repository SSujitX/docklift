// Deployments routes - API endpoints for deploy, redeploy, stop, restart, logs
import { Router, Request, Response } from 'express';
import { spawnSync, ChildProcess } from 'child_process';
import path from 'path';
import fs from 'fs';
import prisma from '../lib/prisma.js';
import { config } from '../lib/config.js';
import * as dockerService from '../services/docker.js';
import {
  dockerfileMountsSecret,
  generateRuntimeCompose,
  validateDockerBuildArgs,
} from '../services/compose.js';
import { resolveProjectBuild } from '../services/buildResolver.js';
import { buildServiceImage } from '../services/buildRunner.js';
import { pullRepo, getLastCommitMessage } from '../services/git.js';
import { cleanupServiceDomain, updateServiceDomain } from '../services/nginx.js';
import {
  appendSslEvent,
  clearSslEvents,
  clearSslMeta,
  getCertificateStatus,
  getSslEvents,
  type CertificateStatus,
} from '../services/certs.js';
import { checkDomainsDns, getServerPublicIp } from '../services/dnsCheck.js';
import {
  composeProjectName,
  composeProjectAliases,
  dockerSlug,
  serviceContainerName,
  storageVolumeComposeKey,
} from '../lib/naming.js';
import { allocatePort } from '../lib/portAllocation.js';
import {
  assertHostnamesAvailable,
  formatDomainField,
  normalizeDomainList,
} from '../lib/domainOwnership.js';
import { isProjectDeploying, setProjectDeploying } from '../lib/deploymentState.js';
import { runCompose } from '../lib/runCompose.js';
import { syncProjectStatusFromContainers } from '../lib/projectStatusSync.js';

const router = Router();

function runtimeComposePath(projectId: string): string {
  return path.join(config.deploymentsPath, '.docklift', projectId, 'compose.yml');
}

function composeFileArgs(projectId: string): string[] {
  const composePath = runtimeComposePath(projectId);
  if (!fs.existsSync(composePath)) {
    throw new Error('DockLift runtime configuration is missing. Deploy the project first.');
  }
  return ['-f', composePath];
}

// Track in-flight compose builds so /cancel can actually stop them
interface ActiveBuild {
  process: ChildProcess;
  deploymentId: string;
  cancelled: boolean;
}
const activeBuilds = new Map<string, ActiveBuild>();
const cancelledDeployments = new Set<string>();

function killComposeProcess(proc: ChildProcess) {
  if (!proc.pid) return;
  try {
    if (process.platform === 'win32') {
      spawnSync('taskkill', ['/pid', String(proc.pid), '/T', '/F'], { stdio: 'ignore' });
    } else {
      // Kill the process group — docker compose is a CLI plugin with children
      try {
        process.kill(-proc.pid, 'SIGTERM');
      } catch {
        proc.kill('SIGTERM');
      }
      setTimeout(() => {
        try {
          if (proc.pid) {
            try {
              process.kill(-proc.pid, 'SIGKILL');
            } catch {
              proc.kill('SIGKILL');
            }
          }
        } catch {
          /* already dead */
        }
      }, 3000);
    }
  } catch {
    try {
      proc.kill('SIGKILL');
    } catch {
      /* ignore */
    }
  }
}

function registerBuild(projectId: string, child: ChildProcess, deploymentId: string) {
  const existing = activeBuilds.get(projectId);
  if (existing && existing.deploymentId !== deploymentId) {
    existing.cancelled = true;
    killComposeProcess(existing.process);
  }
  activeBuilds.set(projectId, { process: child, deploymentId, cancelled: false });
}

function clearBuild(projectId: string, deploymentId: string) {
  const entry = activeBuilds.get(projectId);
  if (entry && entry.deploymentId === deploymentId) {
    activeBuilds.delete(projectId);
  }
}

function wasBuildCancelled(projectId: string, deploymentId: string): boolean {
  if (cancelledDeployments.has(deploymentId)) return true;
  const entry = activeBuilds.get(projectId);
  if (!entry) return false;
  if (entry.deploymentId !== deploymentId) return true;
  return entry.cancelled;
}

async function isDeploymentCancelled(projectId: string, deploymentId: string): Promise<boolean> {
  if (wasBuildCancelled(projectId, deploymentId)) return true;
  const row = await prisma.deployment.findUnique({
    where: { id: deploymentId },
    select: { status: true },
  });
  return row?.status === 'cancelled';
}

async function failDeploymentState(
  projectId: string,
  deploymentId: string,
  logs?: string,
) {
  // Never overwrite an intentional cancel with failed
  const persisted = await prisma.deployment
    .updateMany({
      where: {
        id: deploymentId,
        status: { not: 'cancelled' },
      },
      data: {
        status: 'failed',
        finished_at: new Date(),
        ...(logs !== undefined ? { logs } : {}),
      },
    })
    .catch(() => ({ count: 0 }));
  if (!persisted || persisted.count === 0) return;

  await prisma.project
    .update({
      where: { id: projectId },
      data: { status: 'error' },
    })
    .catch(() => {});
  await prisma.service
    .updateMany({
      where: { project_id: projectId },
      data: { status: 'error' },
    })
    .catch(() => {});
}

async function activateServiceDomains(projectId: string) {
  const services = await prisma.service.findMany({ where: { project_id: projectId } });
  for (const svc of services) {
    if (svc.domain && svc.container_name) {
      await updateServiceDomain({ ...svc, status: 'running' });
    }
  }
}

// Strict domain validation helper - validates single domain or comma-separated domains
const DOMAIN_REGEX = /^[a-zA-Z0-9]([a-zA-Z0-9-]*[a-zA-Z0-9])?(\.[a-zA-Z0-9]([a-zA-Z0-9-]*[a-zA-Z0-9])?)+$/;
function isValidDomainList(domainStr: string): boolean {
  if (!domainStr) return true; // Empty is valid (optional field)
  const domains = domainStr.split(',').map(d => d.trim()).filter(Boolean);
  return domains.every(d => DOMAIN_REGEX.test(d));
}

/**
 * Post-deploy cleanup — intentionally a no-op for host Docker state.
 * Never auto-prune Docker images/system state: dangling layers may belong
 * to unrelated workloads on a shared host. Operators reclaim space via host tools.
 */
async function runPostDeploymentPurge(): Promise<{ success: boolean; message: string }> {
  return {
    success: true,
    message: '○ Skipped automatic Docker image cleanup (shared-host safe)',
  };
}

// List deployments for a project
// Default: JSON array (backward compatible).
// With ?meta=1: { items, total } for paginated UIs.
router.get('/:projectId', async (req: Request, res: Response) => {
  try {
    const limit = parseInt(req.query.limit as string) || 10;
    const offset = parseInt(req.query.offset as string) || 0;
    const withMeta = req.query.meta === '1' || req.query.meta === 'true';
    const where = { project_id: req.params.projectId };

    const [deployments, total] = await Promise.all([
      prisma.deployment.findMany({
        where,
        orderBy: { created_at: 'desc' },
        take: limit,
        skip: offset,
      }),
      withMeta
        ? prisma.deployment.count({ where })
        : Promise.resolve(undefined as number | undefined),
    ]);

    if (withMeta) {
      res.json({ items: deployments, total: total ?? 0 });
    } else {
      res.json(deployments);
    }
  } catch (error) {
    res.status(500).json({ error: 'Failed to list deployments' });
  }
});

// List services for a project
router.get('/:projectId/services', async (req: Request, res: Response) => {
  try {
    const services = await prisma.service.findMany({
      where: { project_id: req.params.projectId },
    });
    res.json(services);
  } catch (error) {
    res.status(500).json({ error: 'Failed to list services' });
  }
});

function domainList(domainStr: unknown): string[] {
  return String(domainStr ?? '')
    .split(',')
    .map((d) => d.trim().toLowerCase())
    .filter(Boolean);
}

async function sslMapForDomainString(domainStr: string | null | undefined): Promise<Record<string, CertificateStatus>> {
  const domains = domainList(domainStr);
  const ssl: Record<string, CertificateStatus> = {};
  for (const d of domains) {
    // Status is keyed by primary LE name (first domain in group); each listed domain checked individually
    ssl[d] = await getCertificateStatus(d);
  }
  return ssl;
}

// GET service SSL status + recent issuance activity for each configured domain
router.get('/:projectId/services/:serviceId/ssl', async (req: Request, res: Response) => {
  try {
    const { projectId, serviceId } = req.params;
    const service = await prisma.service.findFirst({
      where: { id: serviceId, project_id: projectId },
    });
    if (!service) {
      return res.status(404).json({ error: 'Service not found' });
    }
    const ssl = await sslMapForDomainString(service.domain);
    res.json({ ssl, events: getSslEvents(domainList(service.domain)) });
  } catch (error) {
    console.error(error);
    res.status(500).json({ error: 'Failed to get SSL status' });
  }
});

// POST DNS preflight — does each hostname point at this server?
router.post('/:projectId/services/:serviceId/dns-check', async (req: Request, res: Response) => {
  try {
    const { projectId, serviceId } = req.params;
    const service = await prisma.service.findFirst({
      where: { id: serviceId, project_id: projectId },
    });
    if (!service) {
      return res.status(404).json({ error: 'Service not found' });
    }

    const requested = Array.isArray(req.body?.domains) ? req.body.domains.slice(0, 10) : null;
    const hosts = (requested ? requested.map((d: unknown) => String(d)) : domainList(service.domain))
      .map((d: string) => d.trim().toLowerCase())
      .filter((d: string) => d && !d.includes(',') && isValidDomainList(d))
      .slice(0, 10);

    const [checks, serverIp] = await Promise.all([checkDomainsDns(hosts), getServerPublicIp()]);
    res.json({ checks, serverIp });
  } catch (error) {
    console.error(error);
    res.status(500).json({ error: 'DNS check failed' });
  }
});

// Retry Let's Encrypt for all domains on a service
router.post('/:projectId/services/:serviceId/ssl/retry', async (req: Request, res: Response) => {
  try {
    const { projectId, serviceId } = req.params;
    const service = await prisma.service.findFirst({
      where: { id: serviceId, project_id: projectId },
    });
    if (!service) {
      return res.status(404).json({ error: 'Service not found' });
    }
    if (!service.domain || !service.container_name) {
      return res.status(400).json({ error: 'Service has no domain or is not deployed' });
    }

    const primaries = domainList(service.domain);
    for (const d of primaries) {
      await clearSslMeta(d);
    }
    clearSslEvents(primaries);
    appendSslEvent(primaries, 'info', 'Manual SSL retry requested.');

    await updateServiceDomain(
      { ...service, status: service.status || 'running' },
      { issueSsl: true, forceSsl: true }
    );
    const ssl = await sslMapForDomainString(service.domain);
    res.json({ success: true, ssl, events: getSslEvents(primaries) });
  } catch (error) {
    console.error(error);
    res.status(500).json({ error: 'SSL retry failed' });
  }
});

// Update service domain
router.put('/:projectId/services/:serviceId', async (req: Request, res: Response) => {
  try {
    const { projectId, serviceId } = req.params;
    const { domain } = req.body || {};

    if (domain !== undefined && domain !== null && typeof domain !== 'string') {
      return res.status(400).json({ error: 'Domain must be a comma-separated string' });
    }

    // Validate domain format to prevent Nginx config injection
    if (domain && !isValidDomainList(domain)) {
      return res.status(400).json({ error: 'Invalid domain format. Must be valid domain names (e.g., example.com, app.example.com).' });
    }

    const existing = await prisma.service.findFirst({
      where: { id: serviceId, project_id: projectId },
    });
    if (!existing) {
      return res.status(404).json({ error: 'Service not found or access denied' });
    }

    const previous = domainList(existing.domain);
    const next = normalizeDomainList(domain);

    try {
      await assertHostnamesAvailable(next, { excludeServiceId: serviceId });
    } catch (conflict: any) {
      return res.status(409).json({ error: conflict.message || 'Domain already in use' });
    }

    const domainField = formatDomainField(next);
    const previousField = existing.domain;

    await prisma.service.update({
      where: { id: serviceId },
      data: { domain: domainField },
    });

    // Drop activity for hostnames that are no longer mapped here
    clearSslEvents(previous.filter((d) => !next.includes(d)));

    const added = next.filter((d) => !previous.includes(d));
    if (added.length > 0) {
      appendSslEvent(next, 'info', `Domain added: ${added.join(', ')}`);
    }

    // Fetch updated service to generate Nginx config
    const service = await prisma.service.findUnique({
      where: { id: serviceId }
    });

    try {
      if (service && service.container_name) {
        await updateServiceDomain(service, { issueSsl: true });
      } else if (next.length > 0) {
        appendSslEvent(
          next,
          'warn',
          'Service is not deployed yet — routing and HTTPS are set up on the next deploy.'
        );
      }
    } catch (nginxErr) {
      // Compensate: keep DB aligned with live nginx when reload/write fails
      await prisma.service.update({
        where: { id: serviceId },
        data: { domain: previousField },
      }).catch(() => {});
      throw nginxErr;
    }

    const ssl = await sslMapForDomainString(service?.domain);
    res.json({ success: true, ssl, events: getSslEvents(next) });
  } catch (error) {
    console.error(error);
    res.status(500).json({ error: 'Failed to update service' });
  }
});

// Stream deployment logs
async function deployProject(req: Request, res: Response) {
  const { projectId } = req.params;
  let deploymentId: string | null = null;
  const logs: string[] = [];

  try {
    const project = await prisma.project.findUnique({
      where: { id: projectId },
    });
    
    if (!project) {
      return res.status(404).json({ error: 'Project not found' });
    }

    const composeProject = composeProjectName(project.name, projectId);
    
    const projectPath = path.join(config.deploymentsPath, projectId);
    
    if (!fs.existsSync(projectPath)) {
      return res.status(400).json({ error: 'Project files not found' });
    }
    if (isProjectDeploying(projectId)) {
      return res.status(409).json({ error: 'A deployment is already running for this project' });
    }
    const busyDeploy = await prisma.deployment.findFirst({
      where: { project_id: projectId, status: 'in_progress' },
      select: { id: true },
    });
    if (busyDeploy) {
      return res.status(409).json({ error: 'A deployment is already running for this project' });
    }
    setProjectDeploying(projectId, true);
    
    const { trigger, commit_message } = req.body || {};

    // Auto-fetch commit message if not provided (manual deploy)
    let finalCommitMessage = commit_message;
    if (!finalCommitMessage && (project.source_type === 'github' || project.source_type === 'public')) {
      finalCommitMessage = await getLastCommitMessage(projectPath);
    }

    const deployment = await prisma.deployment.create({
      data: {
        project_id: projectId,
        status: 'in_progress',
        trigger: trigger || 'manual',
        commit_message: finalCommitMessage,
        logs: '🚀 Starting deployment...\n', // Initialize with starting message for real-time polling
      },
    });
    deploymentId = deployment.id;

    // Set project and all services to 'building' immediately
    await prisma.project.update({
      where: { id: projectId },
      data: { status: 'building' },
    });
    await prisma.service.updateMany({
      where: { project_id: projectId },
      data: { status: 'building' },
    });

    // Set streaming headers
    res.setHeader('Content-Type', 'text/plain; charset=utf-8');
    res.setHeader('X-Content-Type-Options', 'nosniff');
    res.setHeader('Cache-Control', 'no-cache, no-transform');
    res.setHeader('Connection', 'keep-alive');
    res.setHeader('Transfer-Encoding', 'chunked');
    
    // Helper to write logs to both response and DB logs array
    const writeLog = (text: string) => {
      try { if (!res.writableEnded) res.write(text); } catch {}
      logs.push(text);
    };

    let success = false;
    const servicesData: any[] = [];
    
    // Send initial chunk
    writeLog('🚀 Starting deployment...\n');
    
    // Pull latest if GitHub project
    if (project.source_type === 'github' && project.github_url) {
      // SECURITY: Token is set just-in-time and scrubbed after pull completes
      let gitTokenSet = false;
      let cleanUrl = project.github_url;
      let gitInstance: any = null;
      
      // Refresh the remote URL with a new token (tokens expire after 1 hour)
      try {
        const { getInstallationIdForRepo, getInstallationToken } = await import('./github.js');
        const match = project.github_url.match(/github\.com[/:]([^/]+)\/([^\/]+)/);
        if (match) {
          const [, owner, rawRepo] = match;
          const repo = rawRepo.endsWith('.git') ? rawRepo.slice(0, -4) : rawRepo;
          const installId = await getInstallationIdForRepo(owner, repo);
          const token = await getInstallationToken(installId);
          const urlObj = new URL(project.github_url);
          urlObj.username = 'x-access-token';
          urlObj.password = token;
          const { simpleGit } = await import('simple-git');
          gitInstance = simpleGit(projectPath);
          await gitInstance.remote(['set-url', 'origin', urlObj.toString()]);
          gitTokenSet = true;
          writeLog(`🔑 Refreshed GitHub access token\n`);
        }
      } catch (err: any) {
        writeLog(`⚠️ Token refresh warning: ${err.message}\n`);
      }
      
      // Pull latest code (uses authenticated URL if token was set above)
      let scrubFailed = false;
      try {
        const pullResWrapper = {
          write: (text: string) => writeLog(text),
          end: () => {},
          setHeader: () => {},
        } as any;
        await pullRepo(projectPath, pullResWrapper, project.github_branch || undefined);
      } finally {
        // SECURITY: Scrub token from remote URL after pull — fail deploy if creds remain
        if (gitTokenSet && gitInstance) {
          try {
            const { scrubOriginRemote } = await import('../services/git.js');
            await scrubOriginRemote(projectPath, cleanUrl);
            const remotes = await gitInstance.getRemotes(true);
            const origin = remotes.find((r: { name: string }) => r.name === 'origin');
            const originUrl = origin?.refs?.fetch || origin?.refs?.push || '';
            if (
              originUrl.includes('x-access-token') ||
              /https?:\/\/[^/@]+:[^/@]+@/.test(originUrl)
            ) {
              throw new Error('Origin remote still contains credentials after scrub');
            }
          } catch (scrubErr: any) {
            scrubFailed = true;
            writeLog(`❌ Failed to scrub Git credentials: ${scrubErr?.message || scrubErr}\n`);
            try {
              await gitInstance.remote(['set-url', 'origin', cleanUrl]);
            } catch {
              /* ignore */
            }
          }
        }
      }
      if (scrubFailed) {
        throw new Error('Failed to scrub Git credentials after pull');
      }
    }

    if (cancelledDeployments.has(deployment.id)) throw new Error('Deployment cancelled');
    const resolvedBuild = resolveProjectBuild(projectPath, {
      buildType: project.build_type,
      baseDirectory: project.base_directory,
      dockerfilePath: project.dockerfile_path,
      internalPort: project.internal_port,
    });
    const buildServices = resolvedBuild.services.map((service) => ({
      ...service,
      dockerfile_path: service.dockerfilePath || '[railpack]',
      context_path: service.contextPath,
      internal_port: service.internalPort,
    }));
    
    writeLog(`\n${'━'.repeat(50)}\n`);
    writeLog(`📦 BUILD: ${resolvedBuild.detected}\n`);
    writeLog(`${'━'.repeat(50)}\n\n`);

    const activeServiceNames = new Set(buildServices.map((service) => service.name));
    const staleServices = await prisma.service.findMany({
      where: { project_id: projectId, name: { notIn: [...activeServiceNames] } },
    });
    for (const stale of staleServices) {
      writeLog(`  🧹 Removing stale service: ${stale.name}\n`);
      await cleanupServiceDomain(stale.id);
      if (stale.container_name) {
        spawnSync('docker', ['rm', '-f', stale.container_name], { stdio: 'ignore', shell: false });
      }
      if (stale.port != null) {
        await prisma.port.updateMany({
          where: { project_id: projectId, port: stale.port },
          data: { project_id: null, is_locked: false },
        });
      }
      await prisma.service.delete({ where: { id: stale.id } });
    }
    
    const publishHostPort = (project as { publish_host_port?: boolean }).publish_host_port === true;
    if (publishHostPort) {
      writeLog(`  🔓 Host port publish enabled for this project\n`);
    } else {
      writeLog(`  🔒 Host ports off — reach services via domain / nginx-proxy (opt-in publish_host_port)\n`);
    }

    for (const df of buildServices) {
      writeLog(`  ${df.builder === 'railpack' ? '🛤️' : '🐳'} ${df.name}: ${df.dockerfile_path}\n`);
      writeLog(`     Internal port: ${df.internal_port}\n`);
      
      let service = await prisma.service.findFirst({
        where: { project_id: projectId, name: df.name },
      });
      
      if (!service) {
        const port = publishHostPort ? await allocatePort(projectId) : null;
        const containerName = serviceContainerName(project.name, projectId, df.name);
        
        // If project has a domain and this is the first service being created, assign it
        const shouldAssignProjectDomain = project.domain && !service && buildServices.indexOf(df) === 0;

        service = await prisma.service.create({
          data: {
            project_id: projectId,
            name: df.name,
            dockerfile_path: df.dockerfile_path,
            container_name: containerName,
            internal_port: df.internal_port,
            port: port,
            domain: shouldAssignProjectDomain ? project.domain : null,
            status: 'building',
          },
        });
        writeLog(
          `     Assigned new service: ${df.name}${port != null ? ` (Host port: ${port})` : ' (no host port)'}${shouldAssignProjectDomain ? ` with domain: ${project.domain}` : ''}\n`
        );
        writeLog(`     Container: ${containerName}\n`);
      } else {
        // Migration: rename to slug-based container if needed
        const targetName = serviceContainerName(project.name, projectId, df.name);
        
        if (service.container_name !== targetName) {
           writeLog(`     🛠️ Migrating container name → ${targetName}\n`);
           
           // Force remove the old container name to free up ports
           // SECURITY: Use spawnSync with argument array to prevent command injection
           try {
              writeLog(`     🛑 Removing old container: ${service.container_name}\n`);
              if (service.container_name) spawnSync('docker', ['rm', '-f', service.container_name], { stdio: 'ignore' });
           } catch (e) {
              // Ignore if container doesn't exist
           }

           service = await prisma.service.update({
             where: { id: service.id },
             data: { container_name: targetName }
           });
        }

        if (publishHostPort) {
          if (!service.port) {
            const port = await allocatePort(projectId);
            service = await prisma.service.update({
              where: { id: service.id },
              data: {
                port,
                status: 'building',
                dockerfile_path: df.dockerfile_path,
                internal_port: df.internal_port,
              },
            });
            writeLog(`     Assigned host port to service: ${df.name} (Port: ${port})\n`);
          } else {
            await prisma.service.update({
              where: { id: service.id },
              data: {
                status: 'building',
                dockerfile_path: df.dockerfile_path,
                internal_port: df.internal_port,
              },
            });
            writeLog(`     Updating existing service: ${df.name} (Host port: ${service.port})\n`);
          }
        } else {
          // Release any previously allocated host ports when publish is off
          if (service.port != null) {
            await prisma.port.updateMany({
              where: { project_id: projectId, port: service.port },
              data: { project_id: null, is_locked: false },
            });
            service = await prisma.service.update({
              where: { id: service.id },
              data: {
                port: null,
                status: 'building',
                dockerfile_path: df.dockerfile_path,
                internal_port: df.internal_port,
              },
            });
            writeLog(`     Updating existing service: ${df.name} (host port released)\n`);
          } else {
            await prisma.service.update({
              where: { id: service.id },
              data: {
                status: 'building',
                dockerfile_path: df.dockerfile_path,
                internal_port: df.internal_port,
              },
            });
            writeLog(`     Updating existing service: ${df.name}\n`);
          }
        }
      }
      
      servicesData.push({
        ...df,
        port: service.port,
        container_name: service.container_name,
      });
    }
    
    writeLog(`\n${'─'.repeat(40)}\n`);
    writeLog(`📝 Generating docker-compose.yml...\n`);
    
    const envVars = await prisma.envVariable.findMany({
      where: { project_id: projectId },
    });
    
    if (envVars.length > 0) {
      writeLog(`   🔐 Including ${envVars.length} environment variable(s)\n`);
    }
    
    // Validate public build args against Dockerfiles; secrets need BuildKit mounts
    const publicBuildArgKeys = envVars
      .filter((v) => v.is_build_arg && !(v as { is_secret?: boolean }).is_secret)
      .map((v) => v.key);
    const secretBuildKeys = envVars
      .filter((v) => v.is_build_arg && (v as { is_secret?: boolean }).is_secret)
      .map((v) => v.key);
    if (publicBuildArgKeys.length > 0) {
      for (const df of buildServices.filter((item) => item.builder === 'dockerfile')) {
         const missingArgs = validateDockerBuildArgs(path.join(projectPath, df.dockerfile_path), publicBuildArgKeys);
         if (missingArgs.length > 0) {
           writeLog(`\n⚠️  WARNING: The following build arguments are configured but missing 'ARG' instructions in ${df.dockerfile_path}:\n`);
           missingArgs.forEach(arg => writeLog(`    - ${arg}\n`));
           writeLog(`    These variables will NOT be available during the build process! Please add "ARG ${missingArgs[0]}" to your Dockerfile.\n\n`);
         }
      }
    }
    if (secretBuildKeys.length > 0) {
      const missingSecrets: string[] = [];
      for (const df of buildServices.filter((item) => item.builder === 'dockerfile')) {
        const dfPath = path.join(projectPath, df.dockerfile_path);
        for (const key of secretBuildKeys) {
          if (!dockerfileMountsSecret(dfPath, key)) {
            missingSecrets.push(`${key} (in ${df.dockerfile_path})`);
          }
        }
      }
      if (missingSecrets.length > 0) {
        writeLog(`\n❌ PREFLIGHT FAILED: secret build vars need Dockerfile mounts:\n`);
        for (const item of missingSecrets) {
          writeLog(`    - ${item}\n`);
        }
        writeLog(`    Add: RUN --mount=type=secret,id=<KEY> …\n`);
        throw new Error(
          `Secret build vars missing Dockerfile mounts: ${missingSecrets.join(', ')}`
        );
      }
    }
    
    const statePath = path.join(config.deploymentsPath, '.docklift', projectId);
    const composePath = path.join(statePath, 'compose.yml');
    const persistentVolumes = await prisma.persistentVolume.findMany({
      where: { project_id: projectId },
      orderBy: { created_at: 'asc' },
    });
    const runtimeServices: Array<{
      name: string;
      image: string;
      internal_port: number;
      port: number;
      container_name: string;
      volumes?: Array<{ key: string; name: string; mountPath: string }>;
    }> = [];
    for (const serviceData of servicesData) {
      if (cancelledDeployments.has(deployment.id)) throw new Error('Deployment cancelled');
      const buildService = resolvedBuild.services.find((item) => item.name === serviceData.name);
      if (!buildService) throw new Error(`Build plan missing service ${serviceData.name}`);
      const imageTag = `docklift-${projectId.slice(0, 8)}-${dockerSlug(serviceData.name)}:${deployment.id.slice(0, 8)}`;
      writeLog(`\n${'─'.repeat(40)}\n`);
      await buildServiceImage({
        projectPath,
        statePath,
        service: buildService,
        imageTag,
        envVars,
        writeLog,
        onProcess: (child) => registerBuild(projectId, child, deployment.id),
      });
      clearBuild(projectId, deployment.id);
      runtimeServices.push({
        name: serviceData.name,
        image: imageTag,
        internal_port: serviceData.internal_port,
        port: serviceData.port,
        container_name: serviceData.container_name,
        volumes: persistentVolumes
          .filter((volume) => volume.service_name === serviceData.name)
          .map((volume, index) => ({
            key: storageVolumeComposeKey(serviceData.name, index, volume.name),
            name: volume.name,
            mountPath: volume.mount_path,
          })),
      });
    }
    if (cancelledDeployments.has(deployment.id)) throw new Error('Deployment cancelled');
    generateRuntimeCompose(
      composePath,
      runtimeServices,
      envVars.map((v) => ({
        key: v.key,
        value: v.value,
        is_build_arg: v.is_build_arg ?? false,
        is_runtime: v.is_runtime ?? true,
      })),
      { projectId, publishHostPort }
    );
    writeLog(`✅ DockLift runtime compose created outside the repository\n`);
    writeLog(`   Network: dl-net-${projectId.replace(/-/g, '').slice(0, 8)} (proxy attached after up)\n\n`);
    
    writeLog(`${'─'.repeat(40)}\n`);
    writeLog(`🚀 Starting containers...\n`);
    writeLog(`   Compose project: ${composeProject}\n`);
    writeLog(`${'─'.repeat(40)}\n\n`);

    // Tear down legacy UUID-named compose projects once (pre-slug naming)
    for (const alias of composeProjectAliases(project.name, projectId)) {
      if (alias === composeProject) continue;
      spawnSync('docker', ['compose', '-p', alias, 'down', '--remove-orphans'], {
        cwd: projectPath,
        stdio: 'ignore',
        shell: false,
        timeout: 60000,
      });
    }
    
    let lastUpdate = Date.now();
    const syncLogsToDb = async (force = false) => {
      const now = Date.now();
      if (force || now - lastUpdate > 2000) {
        lastUpdate = now;
        await prisma.deployment.update({
          where: { id: deployment.id },
          data: { logs: logs.join('') },
        }).catch(err => console.error('Failed to sync logs to DB:', err));
      }
    };

    // Run docker compose up — detached process group so cancel can signal plugin children
    const dockerProcess = runCompose(
      ['compose', ...composeFileArgs(projectId), '-p', composeProject, 'up', '-d', '--remove-orphans'],
      {
        cwd: projectPath,
        detached: process.platform !== 'win32',
      },
      {
        onStdout: (data) => {
          const text = data.toString();
          logs.push(text);
          try { if (!res.writableEnded) res.write(text); } catch {}
          syncLogsToDb();
        },
        onStderr: (data) => {
          const text = data.toString();
          logs.push(text);
          try { if (!res.writableEnded) res.write(text); } catch {}
          syncLogsToDb();
        },
        onClose: async (code) => {
      // Hold deploy lock through status write + nginx/SSL so cancel/delete/redeploy stay serialized
      const cancelled = await isDeploymentCancelled(projectId, deployment.id);
      clearBuild(projectId, deployment.id);

      await syncLogsToDb(true);

      const markCancelledAndRelease = async () => {
        writeLog(`\n${'━'.repeat(50)}\n❌ DEPLOY CANCELLED\n${'━'.repeat(50)}\n`);
        await prisma.deployment.update({
          where: { id: deployment.id },
          data: {
            status: 'cancelled',
            logs: logs.join(''),
            finished_at: new Date(),
          },
        }).catch(() => {});
        cancelledDeployments.delete(deployment.id);
        setProjectDeploying(projectId, false);
        if (!res.writableEnded) res.end();
      };

      if (cancelled) {
        await markCancelledAndRelease();
        return;
      }

      success = code === 0;
      
      if (success) {
        try {
          await dockerService.connectProxyToProjectNetwork(projectId);
          writeLog(`🔗 Edge proxy attached to project network\n`);
        } catch (netErr: any) {
          success = false;
          writeLog(
            `\n❌ Edge proxy attach FAILED: ${netErr?.message || 'failed'}\n` +
              `   Domains will NOT be activated (containers may still be running).\n`
          );
        }
      }

      if (success) {
        // Use the request host (e.g., server IP) instead of localhost
        const host = req.headers.host?.split(':')[0] || 'localhost';
        
        writeLog(`\n${'━'.repeat(50)}\n`);
        writeLog(`✅ DEPLOY SUCCESSFUL!\n`);
        writeLog(`${'━'.repeat(50)}\n\n`);
        writeLog(`🌐 ENDPOINTS:\n`);
        let anyHost = false;
        for (const svc of servicesData) {
          if (svc.port) {
            anyHost = true;
            writeLog(`  📍 ${svc.name}: http://${host}:${svc.port}\n`);
          }
        }
        if (!anyHost) {
          writeLog(`  📍 Host ports disabled — use your custom domain (nginx-proxy → container DNS)\n`);
        }

        writeLog(`\n🧹 Post-deploy cleanup...\n`);
        const purgeResult = await runPostDeploymentPurge();
        writeLog(`   ${purgeResult.message}\n`);

        if (await isDeploymentCancelled(projectId, deployment.id)) {
          await markCancelledAndRelease();
          return;
        }
      } else if (code !== 0) {
        writeLog(`\n${'━'.repeat(50)}\n`);
        writeLog(`❌ DEPLOY FAILED (Exit Code: ${code})\n`);
        writeLog(`${'━'.repeat(50)}\n`);
      } else {
        writeLog(`\n${'━'.repeat(50)}\n`);
        writeLog(`❌ DEPLOY FAILED (edge proxy not attached)\n`);
        writeLog(`${'━'.repeat(50)}\n`);
      }

      // Final cancel gate — cancel during purge/success logging must not flip to success
      if (await isDeploymentCancelled(projectId, deployment.id)) {
        await markCancelledAndRelease();
        return;
      }
      
      // Update logs in DB with final messages (never overwrite cancelled)
      const finalStatus = success ? 'success' : 'failed';
      const persisted = await prisma.deployment.updateMany({
        where: {
          id: deployment.id,
          status: { not: 'cancelled' },
        },
        data: {
          status: finalStatus,
          logs: logs.join(''),
          finished_at: new Date(),
        },
      });
      if (persisted.count === 0) {
        cancelledDeployments.delete(deployment.id);
        setProjectDeploying(projectId, false);
        if (!res.writableEnded) res.end();
        return;
      }
      
      if (success) {
        // Cancel can still arrive during nginx activation — honor it by skipping further work
        if (await isDeploymentCancelled(projectId, deployment.id)) {
          await markCancelledAndRelease();
          return;
        }
        await syncProjectStatusFromContainers(projectId);
        try {
          await activateServiceDomains(projectId);
          writeLog(`🌐 Nginx domains activated\n`);
        } catch (e: any) {
          writeLog(`⚠️ Domain activation warning: ${e?.message || 'failed'}\n`);
        }
        if (await isDeploymentCancelled(projectId, deployment.id)) {
          await markCancelledAndRelease();
          return;
        }
      } else {
        await prisma.project.update({
          where: { id: projectId },
          data: { status: 'error' },
        });
        await prisma.service.updateMany({
          where: { project_id: projectId },
          data: { status: 'error' },
        });
      }
      setProjectDeploying(projectId, false);
      
      writeLog(`\n📊 Deployment complete! Status: ${success ? 'SUCCESS ✅' : 'FAILED ❌'}\n`);
      cancelledDeployments.delete(deployment.id);
      if (!res.writableEnded) res.end();
        },
        onError: async (err) => {
      clearBuild(projectId, deployment.id);
      writeLog(`\n❌ Docker execution error: ${err.message}\n`);
      if (await isDeploymentCancelled(projectId, deployment.id)) {
        await prisma.deployment.update({
          where: { id: deployment.id },
          data: { status: 'cancelled', logs: logs.join(''), finished_at: new Date() },
        }).catch(() => {});
      } else {
        await failDeploymentState(projectId, deployment.id, logs.join(''));
      }
      cancelledDeployments.delete(deployment.id);
      setProjectDeploying(projectId, false);
      if (!res.writableEnded) res.end();
        },
      },
    );
    registerBuild(projectId, dockerProcess, deployment.id);
    
  } catch (error: any) {
    setProjectDeploying(projectId, false);
    console.error(error);
    if (deploymentId) {
      const cancelled = wasBuildCancelled(projectId, deploymentId);
      clearBuild(projectId, deploymentId);
      if (cancelled) {
        logs.push(`\n❌ Deployment cancelled\n`);
        await prisma.deployment.update({
          where: { id: deploymentId },
          data: { status: 'cancelled', logs: logs.join(''), finished_at: new Date() },
        }).catch(() => {});
      } else {
        logs.push(`\n❌ Error: ${error.message}\n`);
        await failDeploymentState(projectId, deploymentId, logs.join(''));
      }
      cancelledDeployments.delete(deploymentId);
    }
    try {
      if (!res.headersSent) {
        res.status(500).json({ error: 'Deployment failed' });
      } else {
        try { if (!res.writableEnded) res.write(`\n❌ Error: ${error.message}\n`); } catch {}
        if (!res.writableEnded) res.end();
      }
    } catch {
      /* ignore */
    }
  }
}

router.post('/:projectId/deploy', deployProject);

// Stop project (STREAMING)
router.post('/:projectId/stop', async (req: Request, res: Response) => {
  try {
    const { projectId } = req.params;
    if (isProjectDeploying(projectId)) {
      return res.status(409).json({ error: 'Cannot stop while a deployment is running; cancel it first' });
    }
    const project = await prisma.project.findUnique({ where: { id: projectId } });
    if (!project) return res.status(404).json({ error: 'Project not found' });
    composeFileArgs(projectId);
    const composeProject = composeProjectName(project.name, projectId);
    const projectPath = path.join(config.deploymentsPath, projectId);
    
    res.setHeader('Content-Type', 'text/plain; charset=utf-8');
    res.setHeader('X-Content-Type-Options', 'nosniff');
    res.setHeader('Cache-Control', 'no-cache, no-transform');
    res.setHeader('Transfer-Encoding', 'chunked');
    
    const logs: string[] = [];
    const writeLog = (text: string) => {
      try { if (!res.writableEnded) res.write(text); } catch {}
      logs.push(text);
    };

    const timestamp = new Date().toISOString();
    
    // Create deployment record for stop action
    const deployment = await prisma.deployment.create({
      data: {
        project_id: projectId,
        status: 'in_progress',
        trigger: 'stop',
        logs: '🛑 Stopping project...\n', // Initialize with starting message for real-time polling
      },
    });

    writeLog(`\n${'━'.repeat(50)}\n🛑 STOPPING PROJECT\n📅 ${timestamp}\n${'━'.repeat(50)}\n\n`);

    // Proxy stays attached across deploys; disconnect so compose can remove the network
    writeLog(`🔌 Disconnecting edge proxy from project network...\n`);
    await dockerService.disconnectProxyFromProjectNetwork(projectId);

    const { isComposeTeardownOk } = await import('../lib/composeTeardown.js');
    const aliases = composeProjectAliases(project.name, projectId);
    let success = true;

    // Tear down every alias (current + legacy UUID-era) and verify exact-label postconditions
    for (const alias of aliases) {
      const args =
        alias === composeProject
          ? ['compose', ...composeFileArgs(projectId), '-p', alias, 'down']
          : ['compose', '-p', alias, 'down'];
      writeLog(`🛑 compose down -p ${alias}\n`);
      let down = spawnSync('docker', args, {
        cwd: projectPath,
        encoding: 'utf8',
        shell: false,
        timeout: 60000,
      });
      if (down.stdout) writeLog(String(down.stdout));
      if (down.stderr) writeLog(String(down.stderr));

      if (!isComposeTeardownOk(down, alias)) {
        await dockerService.disconnectProxyFromProjectNetwork(projectId);
        down = spawnSync('docker', args, {
          cwd: projectPath,
          encoding: 'utf8',
          shell: false,
          timeout: 60000,
        });
        if (down.stdout) writeLog(String(down.stdout));
        if (down.stderr) writeLog(String(down.stderr));
      }

      if (!isComposeTeardownOk(down, alias)) {
        success = false;
        writeLog(`❌ Teardown incomplete for "${alias}" — owned containers/networks still present\n`);
      } else {
        writeLog(`✅ Teardown verified for "${alias}"\n`);
      }
    }

    // Containers may still be running — reattach proxy so domains keep working
    if (!success) {
      try {
        await dockerService.connectProxyToProjectNetwork(projectId);
        writeLog(`🔗 Reconnected edge proxy after failed stop (app may still be running)\n`);
      } catch (reErr: any) {
        writeLog(`⚠️ Could not reconnect edge proxy: ${reErr?.message || reErr}\n`);
      }
    }

    if (success) {
      writeLog(`\n${'━'.repeat(50)}\n✅ STOP SUCCESSFUL!\n${'━'.repeat(50)}\n`);
    } else {
      writeLog(`\n${'━'.repeat(50)}\n❌ STOP FAILED\n${'━'.repeat(50)}\n`);
    }

    await prisma.deployment.update({
      where: { id: deployment.id },
      data: {
        status: success ? 'success' : 'failed',
        logs: logs.join(''),
        finished_at: new Date(),
      },
    });

    if (success) {
      await prisma.service.updateMany({
        where: { project_id: projectId },
        data: { status: 'stopped' },
      });

      await prisma.project.update({
        where: { id: projectId },
        data: { status: 'stopped' },
      });

      const stoppedServices = await prisma.service.findMany({ where: { project_id: projectId } });
      for (const svc of stoppedServices) {
        await updateServiceDomain(svc);
      }
    }

    res.end();
    
  } catch (error: any) {
    console.error(error);
    res.write(`\n❌ Error: ${error.message}\n`);
    if (!res.writableEnded) res.end();
  }
});

// Restart project (STREAMING)
router.post('/:projectId/restart', async (req: Request, res: Response) => {
  try {
    const { projectId } = req.params;
    if (isProjectDeploying(projectId)) {
      return res.status(409).json({ error: 'Cannot restart while a deployment is running' });
    }
    const project = await prisma.project.findUnique({ where: { id: projectId } });
    if (!project) return res.status(404).json({ error: 'Project not found' });
    composeFileArgs(projectId);
    const composeProject = composeProjectName(project.name, projectId);
    const projectPath = path.join(config.deploymentsPath, projectId);
    
    res.setHeader('Content-Type', 'text/plain; charset=utf-8');
    res.setHeader('X-Content-Type-Options', 'nosniff');
    res.setHeader('Cache-Control', 'no-cache, no-transform');
    res.setHeader('Transfer-Encoding', 'chunked');
    
    const logs: string[] = [];
    const writeLog = (text: string) => {
      try { if (!res.writableEnded) res.write(text); } catch {}
      logs.push(text);
    };

    const timestamp = new Date().toISOString();
    
    // Create deployment record for restart action
    const deployment = await prisma.deployment.create({
      data: {
        project_id: projectId,
        status: 'in_progress',
        trigger: 'restart',
        logs: '🔄 Starting restart...\n', // Initialize with starting message for real-time polling
      },
    });

    // Set project and all services to 'building' immediately
    await prisma.project.update({
      where: { id: projectId },
      data: { status: 'building' },
    });
    await prisma.service.updateMany({
      where: { project_id: projectId },
      data: { status: 'building' },
    });

    writeLog(`\n${'━'.repeat(50)}\n🔄 RESTARTING PROJECT\n📅 ${timestamp}\n${'━'.repeat(50)}\n\n`);
    
    runCompose(
      ['compose', ...composeFileArgs(projectId), '-p', composeProject, 'restart'],
      { cwd: projectPath },
      {
        onStdout: (data) => writeLog(data.toString()),
        onStderr: (data) => writeLog(data.toString()),
        onClose: async (code) => {
      const success = code === 0;
      
      if (success) {
        writeLog(`\n${'━'.repeat(50)}\n✅ RESTART SUCCESSFUL!\n${'━'.repeat(50)}\n`);
      } else {
        writeLog(`\n${'━'.repeat(50)}\n❌ RESTART FAILED (code ${code})\n${'━'.repeat(50)}\n`);
      }
      
      await prisma.deployment.update({
        where: { id: deployment.id },
        data: {
          status: success ? 'success' : 'failed',
          logs: logs.join(''),
          finished_at: new Date(),
        },
      });
      
      if (success) {
        await syncProjectStatusFromContainers(projectId);
      } else {
        await syncProjectStatusFromContainers(projectId);
      }

      if (!res.writableEnded) res.end();
        },
        onError: async (err) => {
      writeLog(`\n❌ Docker execution error: ${err.message}\n`);
      await prisma.deployment.update({
        where: { id: deployment.id },
        data: { status: 'failed', logs: logs.join(''), finished_at: new Date() },
      }).catch(() => {});
      await syncProjectStatusFromContainers(projectId);
      if (!res.writableEnded) res.end();
        },
      },
    );

  } catch (error: any) {
    console.error(error);
    // Restart sets building before spawn — clear stuck status on unexpected failure
    try {
      const { projectId } = req.params;
      await prisma.project.update({
        where: { id: projectId },
        data: { status: 'error' },
      }).catch(() => {});
      await prisma.service.updateMany({
        where: { project_id: projectId },
        data: { status: 'error' },
      }).catch(() => {});
      await prisma.deployment.updateMany({
        where: { project_id: projectId, status: 'in_progress', trigger: 'restart' },
        data: { status: 'failed', finished_at: new Date() },
      }).catch(() => {});
    } catch { /* ignore */ }
    try {
      if (!res.headersSent) {
        res.status(500).json({ error: 'Restart failed' });
      } else {
        try { if (!res.writableEnded) res.write(`\n❌ Error: ${error.message}\n`); } catch {}
        if (!res.writableEnded) res.end();
      }
    } catch { /* ignore */ }
  }
});

// Redeploy runs the same source pull, build, and rollout pipeline as deploy.
router.post('/:projectId/redeploy', deployProject);

// Cancel build — kill tracked compose process + tear down project containers
router.post('/:projectId/cancel', async (req: Request, res: Response) => {
  try {
    const { projectId } = req.params;
    const project = await prisma.project.findUnique({ where: { id: projectId } });
    if (!project) {
      return res.status(404).json({ error: 'Project not found' });
    }
    const projectPath = path.join(config.deploymentsPath, projectId);
    const composeProject = composeProjectName(project.name, projectId);
    const hasRuntimeCompose = fs.existsSync(runtimeComposePath(projectId));
    
    res.setHeader('Content-Type', 'text/plain; charset=utf-8');
    res.setHeader('X-Content-Type-Options', 'nosniff');
    res.setHeader('Cache-Control', 'no-cache, no-transform');
    res.setHeader('Transfer-Encoding', 'chunked');
    
    res.write(`❌ Cancelling — tearing down so you can start fresh...\n`);

    const active = activeBuilds.get(projectId);
    if (active) {
      active.cancelled = true;
      res.write(`🛑 Stopping in-flight docker compose process...\n`);
      killComposeProcess(active.process);
    } else {
      res.write(`ℹ️ No tracked build process — tearing down containers if present...\n`);
    }

    // Cancel history: only mark rows that were actually in progress.
    // Idle cancel (tear down for a fresh start) must NOT rewrite past success/failed history.
    const inProgress = await prisma.deployment.findMany({
      where: { project_id: projectId, status: 'in_progress' },
      select: { id: true },
    });
    inProgress.forEach((deployment) => cancelledDeployments.add(deployment.id));

    await prisma.deployment.updateMany({
      where: { project_id: projectId, status: 'in_progress' },
      data: { status: 'cancelled', finished_at: new Date() },
    });

    if (inProgress.length === 0 && !active) {
      res.write(`ℹ️ No active deploy row — tearing down containers; history left unchanged.\n`);
    }

    // Disconnect proxy before down — otherwise Docker cannot remove the project network
    await dockerService.disconnectProxyFromProjectNetwork(projectId);

    // Kill primary compose project when runtime file exists (best-effort)
    if (fs.existsSync(projectPath) && hasRuntimeCompose) {
      spawnSync('docker', ['compose', ...composeFileArgs(projectId), '-p', composeProject, 'kill'], {
        cwd: projectPath,
        stdio: 'ignore',
        shell: false,
        timeout: 30000,
        killSignal: 'SIGKILL',
      });
    }

    for (const deployment of inProgress) {
      clearBuild(projectId, deployment.id);
    }

    // Always tear down + label-verify every alias — even when runtime compose.yml is missing
    // (legacy projects may still have labeled containers/networks).
    const { isComposeTeardownOk } = await import('../lib/composeTeardown.js');
    const cwd = fs.existsSync(projectPath) ? projectPath : process.cwd();
    let success = true;
    for (const alias of composeProjectAliases(project.name, projectId)) {
      const args =
        alias === composeProject && hasRuntimeCompose
          ? ['compose', ...composeFileArgs(projectId), '-p', alias, 'down', '--remove-orphans']
          : ['compose', '-p', alias, 'down', '--remove-orphans'];
      res.write(`🛑 compose down -p ${alias}\n`);
      let down = spawnSync('docker', args, {
        cwd,
        encoding: 'utf8',
        shell: false,
        timeout: 60000,
      });
      if (down.stdout) res.write(String(down.stdout));
      if (down.stderr) res.write(String(down.stderr));
      if (!isComposeTeardownOk(down, alias)) {
        await dockerService.disconnectProxyFromProjectNetwork(projectId);
        down = spawnSync('docker', args, {
          cwd,
          encoding: 'utf8',
          shell: false,
          timeout: 60000,
        });
      }
      if (!isComposeTeardownOk(down, alias)) {
        success = false;
        res.write(`❌ Cancel teardown incomplete for "${alias}"\n`);
      } else {
        res.write(`✅ Teardown verified for "${alias}"\n`);
      }
    }

    if (!success) {
      try {
        await dockerService.connectProxyToProjectNetwork(projectId);
        res.write(`🔗 Reconnected edge proxy after failed cancel teardown\n`);
      } catch (reErr: any) {
        res.write(`⚠️ Could not reconnect edge proxy: ${reErr?.message || reErr}\n`);
      }
    }

    setProjectDeploying(projectId, false);
    if (success) {
      await prisma.service.updateMany({
        where: { project_id: projectId },
        data: { status: 'stopped' },
      });

      await prisma.project.update({
        where: { id: projectId },
        data: { status: 'stopped' },
      });

      const cancelledServices = await prisma.service.findMany({ where: { project_id: projectId } });
      for (const svc of cancelledServices) {
        try {
          await updateServiceDomain(svc);
        } catch (e) {
          console.error('Cancel nginx cleanup failed:', e);
        }
      }
      res.write(`✅ Cancelled — containers down. Ready for a fresh deploy.\n`);
    } else {
      res.write(`❌ Compose teardown failed; project status unchanged\n`);
    }
    res.end();
  } catch (error) {
    res.status(500).json({ error: 'Failed to cancel build' });
  }
});

// Get logs
router.get('/:projectId/logs', async (req: Request, res: Response) => {
  try {
    const { projectId } = req.params;
    
    const services = await prisma.service.findMany({
      where: { project_id: projectId },
    });
    
    const logs: Record<string, string> = {};
    
    for (const svc of services) {
      if (svc.container_name) {
        logs[svc.name] = await dockerService.getContainerLogs(svc.container_name);
      }
    }
    
    res.json({ logs });
  } catch (error) {
    res.status(500).json({ error: 'Failed to get logs' });
  }
});

// Get stats
router.get('/:projectId/stats', async (req: Request, res: Response) => {
  try {
    const { projectId } = req.params;
    
    const services = await prisma.service.findMany({
      where: { project_id: projectId },
    });
    
    const stats: Record<string, unknown> = {};
    
    for (const svc of services) {
      if (svc.container_name) {
        stats[svc.name] = await dockerService.getContainerStats(svc.container_name);
      }
    }
    
    res.json(stats);
  } catch (error) {
    res.status(500).json({ error: 'Failed to get stats' });
  }
});

export default router;
