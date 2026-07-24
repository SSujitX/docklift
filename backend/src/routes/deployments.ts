// Deployments routes - API endpoints for deploy, redeploy, stop, restart, logs
import { Router, Request, Response } from 'express';
import { spawn, spawnSync, ChildProcess } from 'child_process';
import path from 'path';
import fs from 'fs';
import prisma from '../lib/prisma.js';
import { config } from '../lib/config.js';
import * as dockerService from '../services/docker.js';
import { generateRuntimeCompose, validateDockerBuildArgs } from '../services/compose.js';
import { resolveProjectBuild } from '../services/buildResolver.js';
import { buildServiceImage } from '../services/buildRunner.js';
import { pullRepo, getLastCommitMessage } from '../services/git.js';
import { cleanupServiceDomain, updateServiceDomain } from '../services/nginx.js';
import {
  clearSslMeta,
  getCertificateStatus,
  type CertificateStatus,
} from '../services/certs.js';
import {
  composeProjectName,
  composeProjectAliases,
  dockerSlug,
  serviceContainerName,
} from '../lib/naming.js';

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
const activeDeploymentProjects = new Set<string>();
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

async function failDeploymentState(
  projectId: string,
  deploymentId: string,
  logs?: string,
) {
  await prisma.deployment
    .update({
      where: { id: deploymentId },
      data: {
        status: 'failed',
        finished_at: new Date(),
        ...(logs !== undefined ? { logs } : {}),
      },
    })
    .catch(() => {});
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

// Auto-purge helper function (runs after successful deployments)
async function runPostDeploymentPurge(): Promise<{ success: boolean; message: string }> {
  try {
    const { exec } = await import('child_process');
    const { promisify } = await import('util');
    const execAsync = promisify(exec);
    const si = await import('systeminformation');
    
    const results: string[] = [];

    // 1. Docker cleanup (remove build artifacts)
    try {
      await execAsync('docker system prune -af', { timeout: 30000 });
      results.push('✓ Docker cleanup');
    } catch (err) {
      results.push('○ Docker cleanup skipped');
    }

    // 2. Swap clearing (with safety check - only on Linux)
    if (process.platform === 'linux') {
      try {
        const memData = await si.default.mem();
        const freeMemoryPercent = ((memData.free + memData.available) / memData.total) * 100;
        
        if (freeMemoryPercent >= 30) {
          await execAsync('swapoff -a && swapon -a', { timeout: 30000 });
          results.push('✓ Swap cleared');
        } else {
          results.push('○ Swap skipped (low RAM)');
        }
      } catch (err) {
        results.push('○ Swap clearing skipped');
      }
    }

    return { 
      success: true, 
      message: results.join(' | ') 
    };
  } catch (error) {
    return { 
      success: false, 
      message: 'Auto-purge failed' 
    };
  }
}

// Allocate a port for the project
async function allocatePort(projectId: string): Promise<number> {
  const usedPorts = await prisma.port.findMany({
    where: { is_locked: true },
    select: { port: true },
  });
  const usedSet = new Set(usedPorts.map(p => p.port));

  let port = config.portRangeStart;
  while (usedSet.has(port)) {
    port++;
    if (port > config.portRangeEnd) {
      throw new Error(`No free ports in range ${config.portRangeStart}-${config.portRangeEnd}`);
    }
  }
  
  // Use upsert to handle if the port record already exists but is unlocked
  await prisma.port.upsert({
    where: { port },
    update: { project_id: projectId, is_locked: true },
    create: { port, project_id: projectId, is_locked: true },
  });
  
  // Also update the main project port if it's not set
  await prisma.project.update({
    where: { id: projectId },
    data: { port: port }
  }).catch(() => {}); // Ignore error if project doesn't exist or other issues
  
  return port;
}

// List deployments for a project
router.get('/:projectId', async (req: Request, res: Response) => {
  try {
    const limit = parseInt(req.query.limit as string) || 10;
    const offset = parseInt(req.query.offset as string) || 0;
    
    const deployments = await prisma.deployment.findMany({
      where: { project_id: req.params.projectId },
      orderBy: { created_at: 'desc' },
      take: limit,
      skip: offset,
    });
    res.json(deployments);
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

async function sslMapForDomainString(domainStr: string | null | undefined): Promise<Record<string, CertificateStatus>> {
  const domains = (domainStr || '')
    .split(',')
    .map((d) => d.trim().toLowerCase())
    .filter(Boolean);
  const ssl: Record<string, CertificateStatus> = {};
  for (const d of domains) {
    // Status is keyed by primary LE name (first domain in group); each listed domain checked individually
    ssl[d] = await getCertificateStatus(d);
  }
  return ssl;
}

// GET service SSL status for each configured domain
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
    res.json({ ssl });
  } catch (error) {
    console.error(error);
    res.status(500).json({ error: 'Failed to get SSL status' });
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

    const primaries = service.domain
      .split(',')
      .map((d) => d.trim().toLowerCase())
      .filter(Boolean);
    for (const d of primaries) {
      await clearSslMeta(d);
    }

    await updateServiceDomain(
      { ...service, status: service.status || 'running' },
      { issueSsl: true, forceSsl: true }
    );
    const ssl = await sslMapForDomainString(service.domain);
    res.json({ success: true, ssl });
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

    // Validate domain format to prevent Nginx config injection
    if (domain && !isValidDomainList(domain)) {
      return res.status(400).json({ error: 'Invalid domain format. Must be valid domain names (e.g., example.com, app.example.com).' });
    }

    const count = await prisma.service.updateMany({
      where: { id: serviceId, project_id: projectId },
      data: { domain },
    });

    if (count.count === 0) {
      return res.status(404).json({ error: 'Service not found or access denied' });
    }
    
    // Fetch updated service to generate Nginx config
    const service = await prisma.service.findUnique({
      where: { id: serviceId }
    });
    
    if (service && service.container_name) {
      await updateServiceDomain(service, { issueSsl: true });
    }

    const ssl = await sslMapForDomainString(service?.domain);
    res.json({ success: true, ssl });
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
    if (activeDeploymentProjects.has(projectId)) {
      return res.status(409).json({ error: 'A deployment is already running for this project' });
    }
    activeDeploymentProjects.add(projectId);
    
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
      try {
        const pullResWrapper = {
          write: (text: string) => writeLog(text),
          end: () => {},
          setHeader: () => {},
        } as any;
        await pullRepo(projectPath, pullResWrapper, project.github_branch || undefined);
      } finally {
        // SECURITY: Scrub token from remote URL after pull completes (or fails)
        if (gitTokenSet && gitInstance) {
          try {
            await gitInstance.remote(['set-url', 'origin', cleanUrl]);
          } catch { /* ignore cleanup errors */ }
        }
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
    
    for (const df of buildServices) {
      writeLog(`  ${df.builder === 'railpack' ? '🛤️' : '🐳'} ${df.name}: ${df.dockerfile_path}\n`);
      writeLog(`     Internal port: ${df.internal_port}\n`);
      
      let service = await prisma.service.findFirst({
        where: { project_id: projectId, name: df.name },
      });
      
      if (!service) {
        const port = await allocatePort(projectId);
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
        writeLog(`     Assigned new service: ${df.name} (Port: ${port})${shouldAssignProjectDomain ? ` with domain: ${project.domain}` : ''}\n`);
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
        // Ensure service has a port if it's missing
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
          writeLog(`     Assigned missing port to service: ${df.name} (Port: ${port})\n`);
        } else {
          await prisma.service.update({
            where: { id: service.id },
            data: {
              status: 'building',
              dockerfile_path: df.dockerfile_path,
              internal_port: df.internal_port,
            },
          });
          writeLog(`     Updating existing service: ${df.name} (Port: ${service.port})\n`);
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
    
    // Validate build args against Dockerfiles
    const buildArgKeys = envVars.filter(v => v.is_build_arg).map(v => v.key);
    if (buildArgKeys.length > 0) {
      for (const df of buildServices.filter((item) => item.builder === 'dockerfile')) {
         const missingArgs = validateDockerBuildArgs(path.join(projectPath, df.dockerfile_path), buildArgKeys);
         if (missingArgs.length > 0) {
           writeLog(`\n⚠️  WARNING: The following build arguments are configured but missing 'ARG' instructions in ${df.dockerfile_path}:\n`);
           missingArgs.forEach(arg => writeLog(`    - ${arg}\n`));
           writeLog(`    These variables will NOT be available during the build process! Please add "ARG ${missingArgs[0]}" to your Dockerfile.\n\n`);
         }
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
            key: `storage_${serviceData.name.replace(/[^a-zA-Z0-9_]/g, '_')}_${index}`,
            name: volume.name,
            mountPath: volume.mount_path,
          })),
      });
    }
    if (cancelledDeployments.has(deployment.id)) throw new Error('Deployment cancelled');
    generateRuntimeCompose(composePath, runtimeServices, envVars.map(v => ({
      key: v.key,
      value: v.value,
      is_build_arg: v.is_build_arg ?? false,
      is_runtime: v.is_runtime ?? true,
    })));
    writeLog(`✅ DockLift runtime compose created outside the repository\n\n`);
    
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
    
    // Run docker compose up — detached process group so cancel can signal plugin children
    const dockerProcess = spawn('docker', ['compose', ...composeFileArgs(projectId), '-p', composeProject, 'up', '-d', '--remove-orphans'], {
      cwd: projectPath,
      shell: false,
      detached: process.platform !== 'win32',
    });
    registerBuild(projectId, dockerProcess, deployment.id);
    
    // Throttled database update for logs
    let lastUpdate = Date.now();
    const syncLogsToDb = async (force = false) => {
      const now = Date.now();
      if (force || now - lastUpdate > 2000) { // Update every 2 seconds or if forced
        lastUpdate = now;
        await prisma.deployment.update({
          where: { id: deployment.id },
          data: { logs: logs.join('') },
        }).catch(err => console.error('Failed to sync logs to DB:', err));
      }
    };
    
    dockerProcess.stdout?.on('data', (data) => {
      const text = data.toString();
      logs.push(text);
      try { if (!res.writableEnded) res.write(text); } catch {}
      syncLogsToDb();
    });
    
    dockerProcess.stderr?.on('data', (data) => {
      const text = data.toString();
      logs.push(text);
      try { if (!res.writableEnded) res.write(text); } catch {}
      syncLogsToDb();
    });
    
    dockerProcess.on('close', async (code) => {
      activeDeploymentProjects.delete(projectId);
      const cancelled = wasBuildCancelled(projectId, deployment.id);
      clearBuild(projectId, deployment.id);

      await syncLogsToDb(true);

      if (cancelled) {
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
        if (!res.writableEnded) res.end();
        return;
      }

      success = code === 0;
      
      if (success) {
        // Use the request host (e.g., server IP) instead of localhost
        const host = req.headers.host?.split(':')[0] || 'localhost';
        
        writeLog(`\n${'━'.repeat(50)}\n`);
        writeLog(`✅ DEPLOY SUCCESSFUL!\n`);
        writeLog(`${'━'.repeat(50)}\n\n`);
        writeLog(`🌐 ENDPOINTS:\n`);
        for (const svc of servicesData) {
          if (svc.port) {
            writeLog(`  📍 ${svc.name}: http://${host}:${svc.port}\n`);
          }
        }
        
        // AUTO-PURGE: Clean up build artifacts and free memory
        writeLog(`\n🧹 Running auto-purge to free resources...\n`);
        const purgeResult = await runPostDeploymentPurge();
        writeLog(`   ${purgeResult.message}\n`);
      } else {
        writeLog(`\n${'━'.repeat(50)}\n`);
        writeLog(`❌ DEPLOY FAILED (Exit Code: ${code})\n`);
        writeLog(`${'━'.repeat(50)}\n`);
      }
      
      // Update logs in DB with final messages
      await prisma.deployment.update({
        where: { id: deployment.id },
        data: {
          status: success ? 'success' : 'failed',
          logs: logs.join(''),
          finished_at: new Date(),
        },
      });
      
      await prisma.project.update({
        where: { id: projectId },
        data: { status: success ? 'running' : 'error' },
      });

      // Update ALL services for this project (not just known ones) for consistency
      await prisma.service.updateMany({
        where: { project_id: projectId },
        data: { status: success ? 'running' : 'error' },
      });

      if (success) {
        try {
          await activateServiceDomains(projectId);
          writeLog(`🌐 Nginx domains activated\n`);
        } catch (e: any) {
          writeLog(`⚠️ Domain activation warning: ${e?.message || 'failed'}\n`);
        }
      }
      
      writeLog(`\n📊 Deployment complete! Status: ${success ? 'SUCCESS ✅' : 'FAILED ❌'}\n`);
      cancelledDeployments.delete(deployment.id);
      if (!res.writableEnded) res.end();
    });
    
    dockerProcess.on('error', async (err) => {
      activeDeploymentProjects.delete(projectId);
      clearBuild(projectId, deployment.id);
      writeLog(`\n❌ Docker execution error: ${err.message}\n`);
      await failDeploymentState(projectId, deployment.id, logs.join(''));
      cancelledDeployments.delete(deployment.id);
      if (!res.writableEnded) res.end();
    });
    
  } catch (error: any) {
    activeDeploymentProjects.delete(projectId);
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
    if (activeDeploymentProjects.has(projectId)) {
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

    // Also clear legacy UUID compose project if present
    for (const alias of composeProjectAliases(project.name, projectId)) {
      if (alias === composeProject) continue;
      spawnSync('docker', ['compose', '-p', alias, 'down'], {
        cwd: projectPath,
        stdio: 'ignore',
        shell: false,
        timeout: 60000,
      });
    }
    
    const dockerProcess = spawn('docker', ['compose', ...composeFileArgs(projectId), '-p', composeProject, 'down'], {
      cwd: projectPath,
      shell: false,
    });
    
    dockerProcess.stdout.on('data', (data) => {
      writeLog(data.toString());
    });
    
    dockerProcess.stderr.on('data', (data) => {
      writeLog(data.toString());
    });
    
    dockerProcess.on('close', async (code) => {
      const success = code === 0;
      
      if (success) {
        writeLog(`\n${'━'.repeat(50)}\n✅ STOP SUCCESSFUL!\n${'━'.repeat(50)}\n`);
      } else {
        writeLog(`\n${'━'.repeat(50)}\n❌ STOP FAILED (code ${code})\n${'━'.repeat(50)}\n`);
      }
      
      // Update deployment record with logs
      await prisma.deployment.update({
        where: { id: deployment.id },
        data: {
          status: success ? 'success' : 'failed',
          logs: logs.join(''),
          finished_at: new Date(),
        },
      });
      
      // Update service and project status
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
      
      res.end();
    });
    
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
    if (activeDeploymentProjects.has(projectId)) {
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
    
    const dockerProcess = spawn('docker', ['compose', ...composeFileArgs(projectId), '-p', composeProject, 'restart'], {
      cwd: projectPath,
      shell: false,
    });
    
    dockerProcess.stdout?.on('data', (data) => {
      writeLog(data.toString());
    });
    
    dockerProcess.stderr?.on('data', (data) => {
      writeLog(data.toString());
    });
    
    dockerProcess.on('close', async (code) => {
      const success = code === 0;
      
      if (success) {
        writeLog(`\n${'━'.repeat(50)}\n✅ RESTART SUCCESSFUL!\n${'━'.repeat(50)}\n`);
      } else {
        writeLog(`\n${'━'.repeat(50)}\n❌ RESTART FAILED (code ${code})\n${'━'.repeat(50)}\n`);
      }
      
      // Update deployment record with logs
      await prisma.deployment.update({
        where: { id: deployment.id },
        data: {
          status: success ? 'success' : 'failed',
          logs: logs.join(''),
          finished_at: new Date(),
        },
      });
      
      // Update project and services status
      await prisma.project.update({
        where: { id: projectId },
        data: { status: success ? 'running' : 'error' },
      });
      await prisma.service.updateMany({
        where: { project_id: projectId },
        data: { status: success ? 'running' : 'error' },
      });

      if (!res.writableEnded) res.end();
    });

    dockerProcess.on('error', async (err) => {
      writeLog(`\n❌ Docker execution error: ${err.message}\n`);
      await failDeploymentState(projectId, deployment.id, logs.join(''));
      if (!res.writableEnded) res.end();
    });

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
    const projectPath = path.join(config.deploymentsPath, projectId);
    const composeProject = project
      ? composeProjectName(project.name, projectId)
      : projectId;
    const hasRuntimeCompose = fs.existsSync(runtimeComposePath(projectId));
    
    res.setHeader('Content-Type', 'text/plain; charset=utf-8');
    res.setHeader('X-Content-Type-Options', 'nosniff');
    res.setHeader('Cache-Control', 'no-cache, no-transform');
    res.setHeader('Transfer-Encoding', 'chunked');
    
    res.write(`❌ Cancelling build...\n`);

    const active = activeBuilds.get(projectId);
    if (active) {
      active.cancelled = true;
      res.write(`🛑 Stopping in-flight docker compose process...\n`);
      killComposeProcess(active.process);
    } else {
      res.write(`ℹ️ No tracked build process — tearing down containers if present...\n`);
    }

    // Keep an in-memory marker so cancellation also interrupts repository pulls,
    // before the first Docker/Railpack child process exists.
    const inProgress = await prisma.deployment.findMany({
      where: { project_id: projectId, status: 'in_progress' },
      select: { id: true },
    });
    inProgress.forEach((deployment) => cancelledDeployments.add(deployment.id));

    // Mark any in-progress deployment rows cancelled immediately
    await prisma.deployment.updateMany({
      where: { project_id: projectId, status: 'in_progress' },
      data: { status: 'cancelled', finished_at: new Date() },
    });

    // Best-effort: kill services then down (argv arrays, no shell; timeout avoids blocking forever)
    if (fs.existsSync(projectPath) && hasRuntimeCompose) {
      spawnSync('docker', ['compose', ...composeFileArgs(projectId), '-p', composeProject, 'kill'], {
        cwd: projectPath,
        stdio: 'ignore',
        shell: false,
        timeout: 30000,
        killSignal: 'SIGKILL',
      });
    }

    const dockerArgs = hasRuntimeCompose
      ? ['compose', ...composeFileArgs(projectId), '-p', composeProject, 'down', '--remove-orphans']
      : ['version', '--format', '{{.Server.Version}}'];
    const dockerProcess = spawn('docker', dockerArgs, {
      cwd: fs.existsSync(projectPath) ? projectPath : process.cwd(),
      shell: false,
    });
    
    dockerProcess.stdout.on('data', (data) => {
      res.write(data.toString());
    });
    
    dockerProcess.stderr.on('data', (data) => {
      res.write(data.toString());
    });
    
    dockerProcess.on('close', async () => {
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
        await updateServiceDomain(svc);
      }
      
      res.write(`✅ Build cancelled and status reset\n`);
      res.end();
    });

    dockerProcess.on('error', async () => {
      await prisma.service.updateMany({
        where: { project_id: projectId },
        data: { status: 'stopped' },
      });
      await prisma.project.update({
        where: { id: projectId },
        data: { status: 'stopped' },
      });
      res.write(`✅ Status reset (compose down unavailable)\n`);
      res.end();
    });
    
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
