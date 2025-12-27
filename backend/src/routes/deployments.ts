import { Router, Request, Response } from 'express';
import { spawn } from 'child_process';
import path from 'path';
import fs from 'fs';
import prisma from '../lib/prisma.js';
import { config } from '../lib/config.js';
import * as dockerService from '../services/docker.js';
import { scanDockerfiles, generateCompose } from '../services/compose.js';
import { pullRepo } from '../services/git.js';
import { patchMiddlewareHosts, logMiddlewareBypassResult } from '../lib/middlewareBypass.js';

const router = Router();

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
    const deployments = await prisma.deployment.findMany({
      where: { project_id: req.params.projectId },
      orderBy: { created_at: 'desc' },
      take: 20,
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

// Update service domain
router.put('/:projectId/services/:serviceId', async (req: Request, res: Response) => {
  try {
    const { projectId, serviceId } = req.params;
    const { domain } = req.body;

    const count = await prisma.service.updateMany({
      where: { id: serviceId, project_id: projectId },
      data: { domain },
    });

    if (count.count === 0) {
      return res.status(404).json({ error: 'Service not found or access denied' });
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
    
    const deployment = await prisma.deployment.create({
      data: {
        project_id: projectId,
        status: 'in_progress',
      },
    });
    
    await prisma.project.update({
      where: { id: projectId },
      data: { status: 'building' },
    });
    
    // Set streaming headers
    res.setHeader('Content-Type', 'text/plain; charset=utf-8');
    res.setHeader('X-Content-Type-Options', 'nosniff');
    res.setHeader('Cache-Control', 'no-cache, no-transform');
    res.setHeader('Connection', 'keep-alive');
    res.setHeader('Transfer-Encoding', 'chunked');
    
    // Send initial chunk immediately to bypass any buffering
    res.write('🚀 Starting deployment...\n');
    
    const logs: string[] = [];
    let success = false;
    const servicesData: any[] = [];
    
    // Pull latest if GitHub project
    if (project.source_type === 'github' && project.github_url) {
      await pullRepo(projectPath, res);
    }
    
    res.write(`\n${'━'.repeat(50)}\n`);
    res.write(`📦 DETECTED ${dockerfiles.length} DOCKERFILE(S)\n`);
    res.write(`${'━'.repeat(50)}\n\n`);
    
    for (const df of dockerfiles) {
      res.write(`  🐳 ${df.name}: ${df.dockerfile_path}\n`);
      res.write(`     Internal port: ${df.internal_port}\n`);
      
      let service = await prisma.service.findFirst({
        where: { project_id: projectId, name: df.name },
      });
      
      if (!service) {
        const port = await allocatePort(projectId);
        service = await prisma.service.create({
          data: {
            project_id: projectId,
            name: df.name,
            dockerfile_path: df.dockerfile_path,
            container_name: `docklift_${projectId}_${df.name}`,
            internal_port: df.internal_port,
            port: port,
            status: 'building',
          },
        });
        res.write(`     Assigned new service: ${df.name} (Port: ${port})\n`);
      } else {
        // Ensure service has a port if it's missing
        if (!service.port) {
          const port = await allocatePort(projectId);
          service = await prisma.service.update({
            where: { id: service.id },
            data: { port, status: 'building' },
          });
          res.write(`     Assigned missing port to service: ${df.name} (Port: ${port})\n`);
        } else {
          await prisma.service.update({
            where: { id: service.id },
            data: { status: 'building' },
          });
          res.write(`     Updating existing service: ${df.name} (Port: ${service.port})\n`);
        }
      }
      
      servicesData.push({
        ...df,
        port: service.port,
        container_name: service.container_name,
      });
    }
    
    res.write(`\n${'─'.repeat(40)}\n`);
    res.write(`📝 Generating docker-compose.yml...\n`);
    
    const envVars = await prisma.envVariable.findMany({
      where: { project_id: projectId },
    });
    
    if (envVars.length > 0) {
      res.write(`   🔐 Including ${envVars.length} environment variable(s)\n`);
    }
    
    generateCompose(projectId, projectPath, servicesData, envVars.map(v => ({
      key: v.key,
      value: v.value,
      is_build_arg: v.is_build_arg ?? false,
      is_runtime: v.is_runtime ?? true,
    })), project.project_type ?? 'app');
    res.write(`✅ docker-compose.yml created with ${servicesData.length} service(s)\n\n`);
    
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
      logMiddlewareBypassResult(result, (msg) => res.write(msg));
    }
    
    res.write(`${'─'.repeat(40)}\n`);
    res.write(`🚀 Starting containers...\n`);
    res.write(`${'─'.repeat(40)}\n\n`);
    
    // Run docker compose up
    const dockerProcess = spawn('docker', ['compose', '-p', projectId, 'up', '-d', '--build'], {
      cwd: projectPath,
      shell: false,
    });
    
    dockerProcess.stdout.on('data', (data) => {
      const text = data.toString();
      logs.push(text);
      res.write(text);
    });
    
    dockerProcess.stderr.on('data', (data) => {
      const text = data.toString();
      logs.push(text);
      res.write(text);
    });
    
    dockerProcess.on('close', async (code) => {
      success = code === 0;
      
      // Update statuses
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
      
      for (const svc of servicesData) {
        await prisma.service.updateMany({
          where: { project_id: projectId, name: svc.name },
          data: { status: success ? 'running' : 'error' },
        });
      }
      
      if (success) {
        res.write(`\n${'━'.repeat(50)}\n`);
        res.write(`✅ DEPLOY SUCCESSFUL!\n`);
        res.write(`${'━'.repeat(50)}\n\n`);
        res.write(`🌐 ENDPOINTS:\n`);
        for (const svc of servicesData) {
          if (svc.port) {
            res.write(`  📍 ${svc.name}: http://localhost:${svc.port}\n`);
          }
        }
        logs.push(`\n${'━'.repeat(50)}\n✅ DEPLOY SUCCESSFUL!\n${'━'.repeat(50)}\n`);
      } else {
        res.write(`\n${'━'.repeat(50)}\n`);
        res.write(`❌ DEPLOY FAILED (Exit Code: ${code})\n`);
        res.write(`${'━'.repeat(50)}\n`);
        logs.push(`\n${'━'.repeat(50)}\n❌ DEPLOY FAILED (Exit Code: ${code})\n${'━'.repeat(50)}\n`);
      }
      
      // Update logs in DB with final messages
      await prisma.deployment.update({
        where: { id: deployment.id },
        data: {
          logs: logs.join(''),
        },
      });
      
      res.write(`\n📊 Deployment complete! Status: ${success ? 'SUCCESS ✅' : 'FAILED ❌'}\n`);
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
    const timestamp = new Date().toISOString();
    
    // Create deployment record for stop action
    const deployment = await prisma.deployment.create({
      data: {
        project_id: projectId,
        status: 'pending',
        logs: '',
      },
    });
    
    const header = `\n${'━'.repeat(50)}\n🛑 STOPPING PROJECT\n📅 ${timestamp}\n${'━'.repeat(50)}\n\n`;
    logs.push(header);
    res.write(header);
    
    const dockerProcess = spawn('docker', ['compose', '-p', projectId, 'down'], {
      cwd: projectPath,
      shell: false,
    });
    
    dockerProcess.stdout.on('data', (data) => {
      const text = data.toString();
      logs.push(text);
      res.write(text);
    });
    
    dockerProcess.stderr.on('data', (data) => {
      const text = data.toString();
      logs.push(text);
      res.write(text);
    });
    
    dockerProcess.on('close', async (code) => {
      const success = code === 0;
      
      if (success) {
        const msg = `\n${'━'.repeat(50)}\n✅ STOP SUCCESSFUL!\n${'━'.repeat(50)}\n`;
        logs.push(msg);
        res.write(msg);
      } else {
        const msg = `\n${'━'.repeat(50)}\n❌ STOP FAILED (code ${code})\n${'━'.repeat(50)}\n`;
        logs.push(msg);
        res.write(msg);
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
    const timestamp = new Date().toISOString();
    
    // Create deployment record for restart action
    const deployment = await prisma.deployment.create({
      data: {
        project_id: projectId,
        status: 'pending',
        logs: '',
      },
    });
    
    const header = `\n${'━'.repeat(50)}\n🔄 RESTARTING PROJECT\n📅 ${timestamp}\n${'━'.repeat(50)}\n\n`;
    logs.push(header);
    res.write(header);
    
    const dockerProcess = spawn('docker', ['compose', '-p', projectId, 'restart'], {
      cwd: projectPath,
      shell: false,
    });
    
    dockerProcess.stdout.on('data', (data) => {
      const text = data.toString();
      logs.push(text);
      res.write(text);
    });
    
    dockerProcess.stderr.on('data', (data) => {
      const text = data.toString();
      logs.push(text);
      res.write(text);
    });
    
    dockerProcess.on('close', async (code) => {
      const success = code === 0;
      
      if (success) {
        const msg = `\n${'━'.repeat(50)}\n✅ RESTART SUCCESSFUL!\n${'━'.repeat(50)}\n`;
        logs.push(msg);
        res.write(msg);
      } else {
        const msg = `\n${'━'.repeat(50)}\n❌ RESTART FAILED (code ${code})\n${'━'.repeat(50)}\n`;
        logs.push(msg);
        res.write(msg);
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
      
      // Update project status
      await prisma.project.update({
        where: { id: projectId },
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
    const timestamp = new Date().toISOString();
    
    // Create deployment record for redeploy action
    const deployment = await prisma.deployment.create({
      data: {
        project_id: projectId,
        status: 'pending',
        logs: '',
      },
    });
    
    const header = `\n${'━'.repeat(50)}\n🔄 REDEPLOYING CONTAINER\n📅 ${timestamp}\n${'━'.repeat(50)}\n\n📦 Rebuilding with --force-recreate...\n${'─'.repeat(40)}\n`;
    logs.push(header);
    res.write(header);
    
    const dockerProcess = spawn('docker', ['compose', '-p', projectId, 'up', '-d', '--build', '--force-recreate'], {
      cwd: projectPath,
      shell: true,
    });
    
    dockerProcess.stdout.on('data', (data) => {
      const text = data.toString();
      logs.push(text);
      res.write(text);
    });
    
    dockerProcess.stderr.on('data', (data) => {
      const text = data.toString();
      logs.push(text);
      res.write(text);
    });
    
    dockerProcess.on('close', async (code) => {
      const success = code === 0;
      
      res.write(`\n${'─'.repeat(40)}\n`);
      logs.push(`\n${'─'.repeat(40)}\n`);
      
      if (success) {
        const msg = `\n${'━'.repeat(50)}\n✅ REDEPLOY SUCCESSFUL!\n${'━'.repeat(50)}\n`;
        logs.push(msg);
        res.write(msg);
        
        await prisma.project.update({
          where: { id: projectId },
          data: { status: 'running' },
        });
        
        await prisma.service.updateMany({
          where: { project_id: projectId },
          data: { status: 'running' },
        });
      } else {
        const msg = `\n${'━'.repeat(50)}\n❌ REDEPLOY FAILED (code ${code})\n${'━'.repeat(50)}\n`;
        logs.push(msg);
        res.write(msg);
        
        await prisma.project.update({
          where: { id: projectId },
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
      shell: true,
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
