// Projects routes - API endpoints for project CRUD and environment variables
import { Router, Request, Response, NextFunction } from 'express';
import multer from 'multer';
import fs from 'fs';
import path from 'path';
import prisma from '../lib/prisma.js';
import { config } from '../lib/config.js';
import { cloneRepo, getCurrentBranch } from '../services/git.js';
import * as dockerService from '../services/docker.js';
import { getInstallationToken, getSetting, getInstallationIdForRepo } from './github.js';
import { cleanupServiceDomain } from '../services/nginx.js';
import { safeExtractZip } from '../lib/safeUnzip.js';
import crypto from 'crypto';
import {
  normalizeBuildType,
  resolveProjectBuild,
} from '../services/buildResolver.js';
import { composeProjectName, dockerSlug, shortProjectId } from '../lib/naming.js';
import { spawnSync } from 'child_process';

const router = Router();
const uploadDir = path.join(config.dataPath, 'uploads');
if (!fs.existsSync(uploadDir)) fs.mkdirSync(uploadDir, { recursive: true });
const MAX_UPLOAD_ZIP_BYTES = 100 * 1024 * 1024; // 100 MiB compressed
const MAX_EXTRACTED_BYTES = 512 * 1024 * 1024; // 512 MiB uncompressed
const upload = multer({
  dest: uploadDir,
  limits: { fileSize: MAX_UPLOAD_ZIP_BYTES, files: 1 },
});

// Strict domain validation helper - validates single domain or comma-separated domains
const DOMAIN_REGEX = /^[a-zA-Z0-9]([a-zA-Z0-9-]*[a-zA-Z0-9])?(\.[a-zA-Z0-9]([a-zA-Z0-9-]*[a-zA-Z0-9])?)+$/;
function isValidDomainList(domainStr: string): boolean {
  if (!domainStr) return true; // Empty is valid (optional field)
  const domains = domainStr.split(',').map(d => d.trim()).filter(Boolean);
  return domains.every(d => DOMAIN_REGEX.test(d));
}

// List all projects
router.get('/', async (req: Request, res: Response) => {
  try {
    const projects = await prisma.project.findMany({
      orderBy: { created_at: 'desc' },
    });
    
    // Sync status and branch with Git/Docker for each project
    for (const project of projects) {
      if (project.source_type === 'github' && !project.github_branch) {
        const projectPath = path.join(config.deploymentsPath, project.id);
        if (fs.existsSync(projectPath)) {
          const branch = await getCurrentBranch(projectPath);
          if (branch) {
            await prisma.project.update({
              where: { id: project.id },
              data: { github_branch: branch },
            });
            project.github_branch = branch;
          }
        }
      }

      let container_name = project.container_name;
      
      if (!container_name) {
        // Check services table
        const service = await prisma.service.findFirst({
          where: { project_id: project.id },
        });
        if (service) {
          container_name = service.container_name;
        }
      }
      
      if (container_name) {
        const status = await dockerService.getContainerStatus(container_name);

        // IMPORTANT: Skip auto-sync if project is currently building
        // The deployment process will handle status updates when it completes
        if (project.status === 'building') {
          // Don't auto-sync during active builds - let deployment handle it
          continue;
        }

        if (status.running && project.status !== 'running') {
          // Update both project AND all services to 'running' for consistency
          await prisma.project.update({
            where: { id: project.id },
            data: { status: 'running' },
          });
          await prisma.service.updateMany({
            where: { project_id: project.id },
            data: { status: 'running' },
          });
          project.status = 'running';

          // Also update IN_PROGRESS deployments to SUCCESS
          await prisma.deployment.updateMany({
            where: { project_id: project.id, status: 'in_progress' },
            data: { status: 'success', finished_at: new Date() },
          });
        } else if (!status.running && status.status !== 'not_found' && project.status === 'running') {
          // Update both project AND all services to 'stopped' for consistency
          await prisma.project.update({
            where: { id: project.id },
            data: { status: 'stopped' },
          });
          await prisma.service.updateMany({
            where: { project_id: project.id },
            data: { status: 'stopped' },
          });
          project.status = 'stopped';
        }
      }
    }
    
    res.json(projects);
  } catch (error) {
    console.error(error);
    res.status(500).json({ error: 'Failed to list projects' });
  }
});

// Get single project
router.get('/:id', async (req: Request, res: Response) => {
  try {
    const project = await prisma.project.findUnique({
      where: { id: req.params.id },
    });
    
    if (!project) {
      return res.status(404).json({ error: 'Project not found' });
    }
    
    // Sync status and branch with Git/Docker
    if (project.source_type === 'github' && !project.github_branch) {
      const projectPath = path.join(config.deploymentsPath, project.id);
      if (fs.existsSync(projectPath)) {
        const branch = await getCurrentBranch(projectPath);
        if (branch) {
          await prisma.project.update({
            where: { id: project.id },
            data: { github_branch: branch },
          });
          project.github_branch = branch;
        }
      }
    }

    let container_name = project.container_name;
    
    if (!container_name) {
      const service = await prisma.service.findFirst({
        where: { project_id: project.id },
      });
      if (service) {
        container_name = service.container_name;
      }
    }
    
    if (container_name) {
      const status = await dockerService.getContainerStatus(container_name);

      // IMPORTANT: Skip auto-sync if project is currently building
      // The deployment process will handle status updates when it completes
      if (project.status !== 'building') {
        if (status.running && project.status !== 'running') {
          // Update both project AND all services to 'running' for consistency
          await prisma.project.update({
            where: { id: project.id },
            data: { status: 'running' },
          });
          await prisma.service.updateMany({
            where: { project_id: project.id },
            data: { status: 'running' },
          });
          project.status = 'running';

          // Also update IN_PROGRESS deployments to SUCCESS
          await prisma.deployment.updateMany({
            where: { project_id: project.id, status: 'in_progress' },
            data: { status: 'success', finished_at: new Date() },
          });
        } else if (!status.running && status.status !== 'not_found' && project.status === 'running') {
          // Update both project AND all services to 'stopped' for consistency
          await prisma.project.update({
            where: { id: project.id },
            data: { status: 'stopped' },
          });
          await prisma.service.updateMany({
            where: { project_id: project.id },
            data: { status: 'stopped' },
          });
          project.status = 'stopped';
        }
      }
    }

    res.json(project);
  } catch (error) {
    console.error(error);
    res.status(500).json({ error: 'Failed to get project' });
  }
});

// Create project
router.post('/', (req: Request, res: Response, next: NextFunction) => {
  upload.single('files')(req, res, (err: unknown) => {
    if (err) {
      const code = (err as { code?: string }).code;
      if (code === 'LIMIT_FILE_SIZE') {
        return res.status(400).json({ error: `Upload too large (max ${MAX_UPLOAD_ZIP_BYTES} bytes)` });
      }
      return res.status(400).json({ error: 'Upload failed' });
    }
    next();
  });
}, async (req: Request, res: Response) => {
  try {
    const {
      name,
      description,
      source_type,
      github_url,
      project_type,
      github_branch,
      domain,
      build_type,
      base_directory,
      dockerfile_path,
      internal_port,
    } = req.body;

    // Validate domain format to prevent Nginx config injection
    if (domain && !isValidDomainList(domain)) {
      return res.status(400).json({ error: 'Invalid domain format. Must be valid domain names (e.g., example.com, app.example.com).' });
    }
    
    // Create project record
    const project = await prisma.project.create({
      data: {
        name,
        description: description || null,
        source_type: source_type || 'upload',
        github_url: github_url || null,
        github_branch: github_branch || null,
        project_type: project_type || 'app',
        domain: domain || null,
        status: 'pending',
        auto_deploy: source_type === 'github',
        webhook_secret: source_type === 'github' ? generateWebhookSecret() : null,
        build_type: normalizeBuildType(build_type),
        base_directory: String(base_directory || '.').trim() || '.',
        dockerfile_path: dockerfile_path ? String(dockerfile_path).trim() : null,
        internal_port: Math.min(65535, Math.max(1, parseInt(internal_port, 10) || 3000)),
      },
    });
    
    const projectPath = path.join(config.deploymentsPath, project.id);
    


    // Handle file upload or git clone
    if (source_type === 'github' && github_url) {
      let authUrl = github_url;
      try {
        // Extract owner/repo from URL to get correct installation
        // Extract owner/repo from URL to get correct installation
        // MODIFIED: Regex updated to support dots in repo name (e.g. quickdlr.com) and stripped .git
        const match = github_url.match(/github\.com[/:]([^/]+)\/([^\/]+)/);
        if (match) {
          const [, owner, rawRepo] = match;
          const repo = rawRepo.endsWith('.git') ? rawRepo.slice(0, -4) : rawRepo;
          let installId: string | null = null;
          
          try {
            // Try to get installation ID for this specific repo
            installId = await getInstallationIdForRepo(owner, repo);
          } catch (err) {
            // Fallback to saved installation ID
            console.warn(`Dynamic installation lookup failed for ${owner}/${repo}, trying saved ID`);
            installId = await getSetting('github_installation_id');
          }
          
          if (installId) {
            const token = await getInstallationToken(installId);
            const urlObj = new URL(github_url);
            urlObj.username = 'x-access-token';
            urlObj.password = token;
            authUrl = urlObj.toString();
          }
        }
      } catch (err) {
        console.warn('Failed to inject GitHub token, trying public clone:', err);
      }
      
      await cloneRepo(authUrl, projectPath, github_branch || undefined);
      // SECURITY: Never leave installation tokens in .git/config (included in backups).
      // If scrub fails after an authenticated clone, delete the project — do not keep a credentialed repo.
      try {
        const { scrubOriginRemote } = await import('../services/git.js');
        await scrubOriginRemote(projectPath, github_url);
        // Verify origin no longer embeds credentials when we used a tokenized URL
        if (authUrl !== github_url) {
          const { simpleGit } = await import('simple-git');
          const remotes = await simpleGit(projectPath).getRemotes(true);
          const origin = remotes.find((r) => r.name === 'origin');
          const originUrl = origin?.refs?.fetch || origin?.refs?.push || '';
          if (
            originUrl.includes('x-access-token') ||
            /https?:\/\/[^/@]+:[^/@]+@/.test(originUrl)
          ) {
            throw new Error('Origin remote still contains credentials after scrub');
          }
        }
      } catch (scrubErr) {
        console.error('Failed to scrub git remote after clone — rolling back project:', scrubErr);
        try { fs.rmSync(projectPath, { recursive: true, force: true }); } catch {}
        try { await prisma.project.delete({ where: { id: project.id } }); } catch {}
        return res.status(500).json({ error: 'Failed to secure repository credentials after clone' });
      }
    } else if (req.file) {
      fs.mkdirSync(projectPath, { recursive: true });
      try {
        await safeExtractZip(req.file.path, projectPath, {
          maxFiles: 20_000,
          maxUncompressedBytes: MAX_EXTRACTED_BYTES,
        });
      } catch (extractErr: any) {
        try { fs.rmSync(projectPath, { recursive: true, force: true }); } catch {}
        try { await prisma.project.delete({ where: { id: project.id } }); } catch {}
        const msg = extractErr?.message || 'Failed to extract upload';
        if (String(msg).includes('exceeds limit') || String(msg).includes('too many files')) {
          return res.status(400).json({ error: msg });
        }
        return res.status(400).json({ error: 'Invalid or unsafe ZIP upload' });
      } finally {
        try { fs.unlinkSync(req.file.path); } catch {}
      }
    }
    
    res.status(201).json(project);
  } catch (error) {
    console.error(error);
    res.status(500).json({ error: 'Failed to create project' });
  }
});

// Update project
router.patch('/:id', async (req: Request, res: Response) => {
  try {
    const {
      name,
      description,
      github_url,
      project_type,
      domain,
      build_type,
      base_directory,
      dockerfile_path,
      internal_port,
    } = req.body;

    // Validate domain format if provided
    if (domain && !isValidDomainList(domain)) {
      return res.status(400).json({ error: 'Invalid domain format. Must be valid domain names (e.g., example.com, app.example.com).' });
    }
    
    const project = await prisma.project.update({
      where: { id: req.params.id },
      data: {
        ...(name && { name }),
        ...(description !== undefined && { description }),
        ...(github_url !== undefined && { github_url: github_url }),
        ...(project_type && { project_type: project_type }),
        ...(domain !== undefined && { domain: domain }),
        ...(build_type !== undefined && { build_type: normalizeBuildType(build_type) }),
        ...(base_directory !== undefined && {
          base_directory: String(base_directory || '.').trim() || '.',
        }),
        ...(dockerfile_path !== undefined && {
          dockerfile_path: dockerfile_path ? String(dockerfile_path).trim() : null,
        }),
        ...(internal_port !== undefined && {
          internal_port: Math.min(65535, Math.max(1, parseInt(internal_port, 10) || 3000)),
        }),
      },
    });
    
    res.json(project);
  } catch (error) {
    console.error(error);
    res.status(500).json({ error: 'Failed to update project' });
  }
});

// Resolve Auto/Dockerfile/Railpack without starting a build.
router.get('/:id/build/detect', async (req: Request, res: Response) => {
  try {
    const project = await prisma.project.findUnique({ where: { id: req.params.id } });
    if (!project) return res.status(404).json({ error: 'Project not found' });
    const projectPath = path.join(config.deploymentsPath, project.id);
    if (!fs.existsSync(projectPath)) {
      return res.status(400).json({ error: 'Project files not found' });
    }
    const resolved = resolveProjectBuild(projectPath, {
      buildType: project.build_type,
      baseDirectory: project.base_directory,
      dockerfilePath: project.dockerfile_path,
      internalPort: project.internal_port,
    });
    res.json({
      requestedType: resolved.requestedType,
      resolvedType: resolved.resolvedType,
      baseDirectory: resolved.baseDirectory,
      dockerfilePath: resolved.dockerfilePath,
      detected: resolved.detected,
      manifests: resolved.manifests,
    });
  } catch (error: any) {
    res.status(400).json({ error: error.message || 'Failed to detect build type' });
  }
});

const STORAGE_NAME_REGEX = /^[a-zA-Z0-9][a-zA-Z0-9_-]{0,31}$/;
const BLOCKED_MOUNT_PATHS = ['/proc', '/sys', '/dev', '/etc', '/var/run', '/var/run/docker.sock'];

function validateMountPath(value: unknown): string {
  const mountPath = String(value || '').trim().replace(/\/+$/, '') || '/';
  if (!mountPath.startsWith('/') || mountPath.includes(':') || mountPath.includes('\0')) {
    throw new Error('Mount path must be an absolute container path');
  }
  if (
    mountPath === '/' ||
    BLOCKED_MOUNT_PATHS.some((blocked) => mountPath === blocked || mountPath.startsWith(`${blocked}/`))
  ) {
    throw new Error('This system mount path is not allowed');
  }
  return mountPath;
}

router.get('/:id/storage', async (req: Request, res: Response) => {
  try {
    const storage = await prisma.persistentVolume.findMany({
      where: { project_id: req.params.id },
      orderBy: { created_at: 'asc' },
    });
    res.json(storage);
  } catch {
    res.status(500).json({ error: 'Failed to list persistent storage' });
  }
});

router.post('/:id/storage', async (req: Request, res: Response) => {
  try {
    const project = await prisma.project.findUnique({ where: { id: req.params.id } });
    if (!project) return res.status(404).json({ error: 'Project not found' });
    const serviceName = String(req.body?.service_name || '').trim();
    const label = String(req.body?.name || '').trim();
    if (!STORAGE_NAME_REGEX.test(label)) {
      return res.status(400).json({ error: 'Storage name must use 1-32 letters, numbers, _ or -' });
    }
    const service = await prisma.service.findFirst({
      where: { project_id: project.id, name: serviceName },
    });
    if (!service) return res.status(400).json({ error: 'Select a valid project service' });
    const mountPath = validateMountPath(req.body?.mount_path);
    const volumeName = `dl-${shortProjectId(project.id)}-${dockerSlug(label, 32)}`;
    const created = await prisma.persistentVolume.create({
      data: {
        project_id: project.id,
        service_name: serviceName,
        name: volumeName,
        display_name: label,
        mount_path: mountPath,
      },
    });
    const result = spawnSync(
      'docker',
      [
        'volume',
        'create',
        '--label',
        `com.docker.compose.project=${composeProjectName(project.name, project.id)}`,
        '--label',
        `com.docklift.project=${project.id}`,
        volumeName,
      ],
      { encoding: 'utf8', shell: false, timeout: 30000 }
    );
    if (result.status !== 0) {
      await prisma.persistentVolume.delete({ where: { id: created.id } }).catch(() => {});
      return res.status(500).json({ error: result.stderr?.trim() || 'Failed to create Docker volume' });
    }
    res.status(201).json(created);
  } catch (error: any) {
    if (error?.code === 'P2002') {
      return res.status(409).json({ error: 'That storage name or mount path already exists' });
    }
    res.status(400).json({ error: error.message || 'Failed to create persistent storage' });
  }
});

router.delete('/:id/storage/:storageId', async (req: Request, res: Response) => {
  try {
    if (req.query.removeVolume !== 'true') {
      return res.status(400).json({ error: 'Confirm permanent data deletion with removeVolume=true' });
    }
    const volume = await prisma.persistentVolume.findFirst({
      where: { id: req.params.storageId, project_id: req.params.id },
    });
    if (!volume) return res.status(404).json({ error: 'Persistent storage not found' });
    const result = spawnSync('docker', ['volume', 'rm', '-f', volume.name], {
      encoding: 'utf8',
      shell: false,
    });
    if (result.status !== 0) {
      return res.status(409).json({
        error: result.stderr?.trim() || 'Stop the project before deleting storage',
      });
    }
    await prisma.persistentVolume.delete({ where: { id: volume.id } });
    res.json({ status: 'deleted' });
  } catch (error: any) {
    res.status(500).json({ error: error.message || 'Failed to delete persistent storage' });
  }
});

// Delete project
router.delete('/:id', async (req: Request, res: Response) => {
  try {
    const projectId = req.params.id;
    const project = await prisma.project.findUnique({ where: { id: projectId } });
    const projectPath = path.join(config.deploymentsPath, projectId);
    const statePath = path.join(config.deploymentsPath, '.docklift', projectId);
    const runtimeCompose = path.join(statePath, 'compose.yml');
    const persistentVolumes = await prisma.persistentVolume.findMany({
      where: { project_id: projectId },
    });
    
    // Attempt to stop and remove Docker containers first
    if (project) {
      try {
        const { composeProjectAliases } = await import('../lib/naming.js');
        const aliases = composeProjectAliases(project.name, projectId);
        // Stop current + legacy compose project names (UUID-era)
        for (const alias of aliases) {
          const composeArgs =
            alias === composeProjectName(project.name, projectId) && fs.existsSync(runtimeCompose)
              ? ['compose', '-f', runtimeCompose, '-p', alias]
              : ['compose', '-p', alias];
          spawnSync('docker', [...composeArgs, 'down', '--remove-orphans', '--rmi', 'all'], {
            cwd: projectPath,
            timeout: 60000,
            shell: false,
          });
        }
      } catch (dockerError) {
        console.warn(`Docker cleanup warned for project ${projectId}:`, dockerError);
      }

      for (const volume of persistentVolumes) {
        const removed = spawnSync('docker', ['volume', 'rm', '-f', volume.name], {
          timeout: 30000,
          shell: false,
          encoding: 'utf8',
        });
        const output = `${removed.stdout || ''}\n${removed.stderr || ''}`;
        if (removed.error || (removed.status !== 0 && !/no such volume/i.test(output))) {
          return res.status(409).json({
            error: `Could not remove persistent volume "${volume.display_name}". Detach any containers using it, then retry project deletion.`,
          });
        }
        await prisma.persistentVolume.delete({ where: { id: volume.id } });
      }
      
      // Remove project files
      try {
        fs.rmSync(projectPath, { recursive: true, force: true });
        fs.rmSync(statePath, { recursive: true, force: true });
      } catch (fileError) {
        console.warn(`File cleanup warned for project ${projectId}:`, fileError);
      }
    }
    
    // Cleanup Nginx configs for all services
    const services = await prisma.service.findMany({
      where: { project_id: projectId },
      select: { id: true }
    });
    
    for (const svc of services) {
      await cleanupServiceDomain(svc.id);
    }
    
    // Delete from database (Prisma handles cascading deletes for deployments, services, env_variables, and ports)
    await prisma.project.delete({
      where: { id: projectId },
    });
    
    res.json({ status: 'deleted' });
  } catch (error) {
    console.error(`Failed to delete project ${req.params.id}:`, error);
    res.status(500).json({ error: 'Failed to delete project' });
  }
});

// Environment variables endpoints
router.get('/:id/env', async (req: Request, res: Response) => {
  try {
    const envVars = await prisma.envVariable.findMany({
      where: { project_id: req.params.id },
    });
    res.json(envVars);
  } catch (error) {
    res.status(500).json({ error: 'Failed to get environment variables' });
  }
});

router.post('/:id/env', async (req: Request, res: Response) => {
  try {
    const { key, value, is_build_arg, is_runtime } = req.body;
    const envVar = await prisma.envVariable.create({
      data: {
        project_id: req.params.id,
        key,
        value: (() => {
          let v = value.trim();
          if (v.length >= 2) {
            const first = v.charAt(0);
            const last = v.charAt(v.length - 1);
            if ((first === '"' && last === '"') || (first === "'" && last === "'")) {
              v = v.substring(1, v.length - 1);
            }
          }
          return v;
        })(),
        is_build_arg: is_build_arg ?? false,
        is_runtime: is_runtime ?? true,
      },
    });
    res.status(201).json(envVar);
  } catch (error) {
    res.status(500).json({ error: 'Failed to create environment variable' });
  }
});

router.post('/:id/env/bulk', async (req: Request, res: Response) => {
  try {
    const { content, is_build_arg, is_runtime } = req.body;
    
    if (!content || typeof content !== 'string') {
      return res.status(400).json({ error: 'Content is required' });
    }
    
    const lines = content.split('\n').filter((line: string) => line.trim());
    const envVars: { key: string; value: string }[] = [];
    
    for (const line of lines) {
      const trimmed = line.trim();
      if (!trimmed || trimmed.startsWith('#')) continue;
      
      const eqIndex = trimmed.indexOf('=');
      if (eqIndex === -1) continue;
      
      const key = trimmed.substring(0, eqIndex).trim();
      let value = trimmed.substring(eqIndex + 1).trim();
      
      // Remove surrounding quotes if present
      if (value.length >= 2) {
        const first = value.charAt(0);
        const last = value.charAt(value.length - 1);
        if ((first === '"' && last === '"') || (first === "'" && last === "'")) {
          value = value.substring(1, value.length - 1);
        }
      }
      
      if (key) {
        envVars.push({ key, value });
      }
    }
    
    if (envVars.length === 0) {
      return res.status(400).json({ error: 'No valid KEY=VALUE pairs found' });
    }
    
    const created = await prisma.envVariable.createMany({
      data: envVars.map(({ key, value }) => ({
        project_id: req.params.id,
        key,
        value,
        is_build_arg: is_build_arg ?? true,
        is_runtime: is_runtime ?? true,
      })),
    });
    
    res.status(201).json({ count: created.count, message: `Added ${created.count} environment variable(s)` });
  } catch (error) {
    console.error(error);
    res.status(500).json({ error: 'Failed to bulk import environment variables' });
  }
});

router.delete('/:id/env/:envId', async (req: Request, res: Response) => {
  try {
    await prisma.envVariable.delete({
      where: { id: req.params.envId },
    });
    res.json({ status: 'deleted' });
  } catch (error) {
    res.status(500).json({ error: 'Failed to delete environment variable' });
  }
});

// ========================================
// Auto-Deploy Management
// ========================================

// Helper: Generate random webhook secret
function generateWebhookSecret(): string {
  return crypto.randomBytes(32).toString('hex');
}

// PATCH /:id/auto-deploy - Toggle auto-deploy for a project
router.patch('/:id/auto-deploy', async (req: Request, res: Response) => {
  try {
    const { id } = req.params;
    const { enabled } = req.body;
    
    const project = await prisma.project.findUnique({
      where: { id },
    });
    
    if (!project) {
      return res.status(404).json({ error: 'Project not found' });
    }
    
    // Check if project is from GitHub
    if (project.source_type !== 'github') {
      return res.status(400).json({ error: 'Auto-deploy is only available for GitHub projects' });
    }
    
    // Generate webhook secret if enabling and not already set
    let webhookSecret = project.webhook_secret;
    if (enabled && !webhookSecret) {
      webhookSecret = generateWebhookSecret();
    }
    
    const updated = await prisma.project.update({
      where: { id },
      data: {
        auto_deploy: enabled,
        webhook_secret: webhookSecret,
      },
    });
    
    res.json({
      auto_deploy: updated.auto_deploy,
      webhook_secret: updated.webhook_secret,
      webhook_url: enabled ? `/api/github/webhook/${id}` : null,
    });
  } catch (error) {
    console.error(error);
    res.status(500).json({ error: 'Failed to update auto-deploy settings' });
  }
});

// GET /:id/auto-deploy - Get auto-deploy status
router.get('/:id/auto-deploy', async (req: Request, res: Response) => {
  try {
    const { id } = req.params;
    
    const project = await prisma.project.findUnique({
      where: { id },
      select: {
        auto_deploy: true,
        webhook_secret: true,
        source_type: true,
      },
    });
    
    if (!project) {
      return res.status(404).json({ error: 'Project not found' });
    }
    
    res.json({
      auto_deploy: project.auto_deploy || false,
      webhook_secret: project.auto_deploy ? project.webhook_secret : null,
      webhook_url: project.auto_deploy ? `/api/github/webhook/${id}` : null,
      available: project.source_type === 'github',
    });
  } catch (error) {
    res.status(500).json({ error: 'Failed to get auto-deploy status' });
  }
});

export default router;

