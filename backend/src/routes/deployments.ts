// Deployments routes - API endpoints for deploy, redeploy, stop, restart, logs
import { Router, Request, Response } from 'express';
import { spawn } from 'child_process';
import path from 'path';
import fs from 'fs';
import prisma from '../lib/prisma.js';
import { config } from '../lib/config.js';
import * as dockerService from '../services/docker.js';
import { scanDockerfiles, generateCompose, validateDockerBuildArgs } from '../services/compose.js';
import { pullRepo, getLastCommitMessage } from '../services/git.js';
import { patchMiddlewareHosts, logMiddlewareBypassResult } from '../lib/middlewareBypass.js';

const router = Router();

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
  // Find next available port starting from 3001
  const usedPorts = await prisma.port.findMany({
    where: { is_locked: true },
    select: { port: true },
  });
  const usedSet = new Set(usedPorts.map(p => p.port));
  
  let port = 3001;
  while (usedSet.has(port)) {
    port++;
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

import { updateServiceDomain } from '../services/nginx.js';

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
      await updateServiceDomain(service);
    }

    res.json({ success: true });
  } catch (error) {
    console.error(error);
    res.status(500).json({ error: 'Failed to update service' });
  }
});

// Stream deployment logs
router.post('/:projectId/deploy', async (req: Request, res: Response) => {
  try {
    const { projectId } = req.params;
    const project = await prisma.project.findUnique({
      where: { id: projectId },
    });
    
    if (!project) {
      return res.status(404).json({ error: 'Project not found' });
    }
    
    const projectPath = path.join(config.deploymentsPath, projectId);
    
    if (!fs.existsSync(projectPath)) {
      return res.status(400).json({ error: 'Project files not found' });
    }
    
    // Scan for all Dockerfiles in the project
    const dockerfiles = scanDockerfiles(projectPath);
    
    if (dockerfiles.length === 0) {
      return res.status(400).json({ error: 'No Dockerfile found in project. Docklift is based on Docker and requires a Dockerfile to automatically build and run your application.' });
    }
    
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
    
    const logs: string[] = [];
    
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
    
    writeLog(`\n${'━'.repeat(50)}\n`);
    writeLog(`📦 DETECTED ${dockerfiles.length} DOCKERFILE(S)\n`);
    writeLog(`${'━'.repeat(50)}\n\n`);
    
    for (const df of dockerfiles) {
      writeLog(`  🐳 ${df.name}: ${df.dockerfile_path}\n`);
      writeLog(`     Internal port: ${df.internal_port}\n`);
      
      let service = await prisma.service.findFirst({
        where: { project_id: projectId, name: df.name },
      });
      
      if (!service) {
        const port = await allocatePort(projectId);
        const shortId = projectId.substring(0, 8);
        // Truncate service name if it's too long to stay under 64 char DNS limit
        const sanitizedName = df.name.substring(0, 50); 
        const containerName = `dl_${shortId}_${sanitizedName}`;
        
        // If project has a domain and this is the first service being created, assign it
        const shouldAssignProjectDomain = project.domain && !service && dockerfiles.indexOf(df) === 0;

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
      } else {
        // Migration: If existing service has long container name, shorten it
        const shortId = projectId.substring(0, 8);
        const sanitizedName = df.name.substring(0, 50);
        const targetName = `dl_${shortId}_${sanitizedName}`;
        
        if (service.container_name !== targetName) {
           writeLog(`     🛠️ Migrating container name to shorter format...\n`);
           
           // Force remove the old container name to free up ports
           // SECURITY: Use spawnSync with argument array to prevent command injection
           const { spawnSync } = await import('child_process');
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
            data: { port, status: 'building' },
          });
          writeLog(`     Assigned missing port to service: ${df.name} (Port: ${port})\n`);
        } else {
          await prisma.service.update({
            where: { id: service.id },
            data: { status: 'building' },
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
      for (const df of dockerfiles) {
         const missingArgs = validateDockerBuildArgs(path.join(projectPath, df.dockerfile_path), buildArgKeys);
         if (missingArgs.length > 0) {
           writeLog(`\n⚠️  WARNING: The following build arguments are configured but missing 'ARG' instructions in ${df.dockerfile_path}:\n`);
           missingArgs.forEach(arg => writeLog(`    - ${arg}\n`));
           writeLog(`    These variables will NOT be available during the build process! Please add "ARG ${missingArgs[0]}" to your Dockerfile.\n\n`);
         }
      }
    }
    
    generateCompose(projectId, projectPath, servicesData, envVars.map(v => ({
      key: v.key,
      value: v.value,
      is_build_arg: v.is_build_arg ?? false,
      is_runtime: v.is_runtime ?? true,
    })), project.project_type ?? 'app');
    writeLog(`✅ docker-compose.yml created with ${servicesData.length} service(s)\n\n`);
    
    // Patch middleware host restrictions for local access
    const allDomains: string[] = [];
    const allPorts: number[] = [];
    for (const svc of servicesData) {
      if (svc.port) allPorts.push(svc.port);
      // Get domains from service table
      const svcRecord = await prisma.service.findFirst({
        where: { project_id: projectId, name: svc.name },
        select: { domain: true },
      });
      if (svcRecord?.domain) {
        allDomains.push(...svcRecord.domain.split(',').map((d: string) => d.trim()).filter(Boolean));
      }
    }
    
    // Scan each service directory for middleware
    for (const svc of servicesData) {
      if (!svc.dockerfile_path) continue; // Skip if no dockerfile_path
      const svcPath = path.join(projectPath, path.dirname(svc.dockerfile_path));
      const result = await patchMiddlewareHosts({
        projectPath: svcPath,
        port: svc.port || 3000,
        domains: allDomains,
      });
      logMiddlewareBypassResult(result, (msg) => writeLog(msg));
    }
    
    writeLog(`${'─'.repeat(40)}\n`);
    writeLog(`🚀 Starting containers...\n`);
    writeLog(`${'─'.repeat(40)}\n\n`);
    
    // Run docker compose up
    const dockerProcess = spawn('docker', ['compose', '-p', projectId, 'up', '-d', '--build'], {
      cwd: projectPath,
      shell: false,
    });
    
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
    
    dockerProcess.stdout.on('data', (data) => {
      const text = data.toString();
      logs.push(text);
      res.write(text);
      syncLogsToDb();
    });
    
    dockerProcess.stderr.on('data', (data) => {
      const text = data.toString();
      logs.push(text);
      res.write(text);
      syncLogsToDb();
    });
    
    dockerProcess.on('close', async (code) => {
      success = code === 0;
      
      // Force final sync
      await syncLogsToDb(true);
      
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
      
      writeLog(`\n📊 Deployment complete! Status: ${success ? 'SUCCESS ✅' : 'FAILED ❌'}\n`);
      res.end();
    });
    
    dockerProcess.on('error', (err) => {
      res.write(`\n❌ Docker execution error: ${err.message}\n`);
      res.end();
    });
    
  } catch (error: any) {
    console.error(error);
    res.write(`\n❌ Error: ${error.message}\n`);
    if (!res.writableEnded) res.end();
  }
});

// Stop project (STREAMING)
router.post('/:projectId/stop', async (req: Request, res: Response) => {
  try {
    const { projectId } = req.params;
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
    
    const dockerProcess = spawn('docker', ['compose', '-p', projectId, 'down'], {
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
    
    const dockerProcess = spawn('docker', ['compose', '-p', projectId, 'restart'], {
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

      res.end();
    });

  } catch (error: any) {
    console.error(error);
    res.write(`\n❌ Error: ${error.message}\n`);
    if (!res.writableEnded) res.end();
  }
});

// Redeploy project (STREAMING)
router.post('/:projectId/redeploy', async (req: Request, res: Response) => {
  try {
    const { projectId } = req.params;
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
    
    // Create deployment record for redeploy action
    const deployment = await prisma.deployment.create({
      data: {
        project_id: projectId,
        status: 'in_progress',
        trigger: 'redeploy',
        logs: '🔄 Starting redeploy...\n', // Initialize with starting message for real-time polling
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

    writeLog(`\n${'━'.repeat(50)}\n🔄 REDEPLOYING CONTAINER\n📅 ${timestamp}\n${'━'.repeat(50)}\n\n📦 Rebuilding with --force-recreate...\n${'─'.repeat(40)}\n`);
    
    const dockerProcess = spawn('docker', ['compose', '-p', projectId, 'up', '-d', '--build', '--force-recreate'], {
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
      
      writeLog(`\n${'─'.repeat(40)}\n`);
      
      if (success) {
        writeLog(`\n${'━'.repeat(50)}\n✅ REDEPLOY SUCCESSFUL!\n${'━'.repeat(50)}\n`);
        
        await prisma.project.update({
          where: { id: projectId },
          data: { status: 'running' },
        });
        
        await prisma.service.updateMany({
          where: { project_id: projectId },
          data: { status: 'running' },
        });
        
        // AUTO-PURGE: Clean up build artifacts and free memory
        writeLog(`\n🧹 Running auto-purge to free resources...\n`);
        const purgeResult = await runPostDeploymentPurge();
        writeLog(`   ${purgeResult.message}\n`);
      } else {
        writeLog(`\n${'━'.repeat(50)}\n❌ REDEPLOY FAILED (code ${code})\n${'━'.repeat(50)}\n`);

        // Update both project AND all services to 'error' for consistency
        await prisma.project.update({
          where: { id: projectId },
          data: { status: 'error' },
        });
        await prisma.service.updateMany({
          where: { project_id: projectId },
          data: { status: 'error' },
        });
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
      
      res.end();
    });
    
  } catch (error: any) {
    console.error(error);
    res.write(`\n❌ Error: ${error.message}\n`);
    if (!res.writableEnded) res.end();
  }
});

// Cancel build
router.post('/:projectId/cancel', async (req: Request, res: Response) => {
  try {
    const { projectId } = req.params;
    const projectPath = path.join(config.deploymentsPath, projectId);
    
    res.setHeader('Content-Type', 'text/plain; charset=utf-8');
    res.setHeader('X-Content-Type-Options', 'nosniff');
    res.setHeader('Cache-Control', 'no-cache, no-transform');
    res.setHeader('Transfer-Encoding', 'chunked');
    
    res.write(`❌ Cancelling build...\n`);
    
    const dockerProcess = spawn('docker', ['compose', '-p', projectId, 'down'], {
      cwd: projectPath,
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
      
      res.write(`✅ Build cancelled and status reset\n`);
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
