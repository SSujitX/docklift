import { Router, Request, Response } from 'express';
import multer from 'multer';
import fs from 'fs';
import path from 'path';
import unzipper from 'unzipper';
import prisma from '../lib/prisma.js';
import { config } from '../lib/config.js';
import { cloneRepo, getCurrentBranch } from '../services/git.js';
import * as dockerService from '../services/docker.js';

const router = Router();
const upload = multer({ dest: 'uploads/' });

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
        if (status.running && project.status !== 'running') {
          await prisma.project.update({
            where: { id: project.id },
            data: { status: 'running' },
          });
          project.status = 'running';
          
          // Also update IN_PROGRESS deployments to SUCCESS
          await prisma.deployment.updateMany({
            where: { project_id: project.id, status: 'in_progress' },
            data: { status: 'success', finished_at: new Date() },
          });
        } else if (!status.running && status.status !== 'not_found' && project.status === 'running') {
          await prisma.project.update({
            where: { id: project.id },
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
      if (status.running && project.status !== 'running') {
        await prisma.project.update({
          where: { id: project.id },
          data: { status: 'running' },
        });
        project.status = 'running';
        
        // Also update IN_PROGRESS deployments to SUCCESS
        await prisma.deployment.updateMany({
          where: { project_id: project.id, status: 'in_progress' },
          data: { status: 'success', finished_at: new Date() },
        });
      } else if (!status.running && status.status !== 'not_found' && project.status === 'running') {
        await prisma.project.update({
          where: { id: project.id },
          data: { status: 'stopped' },
        });
        project.status = 'stopped';
      }
    }
    
    res.json(project);
  } catch (error) {
    console.error(error);
    res.status(500).json({ error: 'Failed to get project' });
  }
});

// Create project
router.post('/', upload.single('files'), async (req: Request, res: Response) => {
  try {
    const { name, description, source_type, github_url, project_type, github_branch } = req.body;
    
    // Create project record
    const project = await prisma.project.create({
      data: {
        name,
        description: description || null,
        source_type: source_type || 'upload',
        github_url: github_url || null,
        github_branch: github_branch || null,
        project_type: project_type || 'app',
        status: 'pending',
      },
    });
    
    const projectPath = path.join(config.deploymentsPath, project.id);
    
import { getInstallationToken, getSetting } from './github.js';

// ... (existing imports)

    // Handle file upload or git clone
    if (source_type === 'github' && github_url) {
      let authUrl = github_url;
      try {
        const installId = await getSetting('github_installation_id');
        if (installId) {
           const token = await getInstallationToken(installId);
           const urlObj = new URL(github_url);
           urlObj.username = 'x-access-token';
           urlObj.password = token;
           authUrl = urlObj.toString();
        }
      } catch (err) {
        console.warn('Failed to inject GitHub token, trying public clone:', err);
      }
      
      await cloneRepo(authUrl, projectPath, github_branch || undefined);
    } else if (req.file) {
      // Extract zip file
      fs.mkdirSync(projectPath, { recursive: true });
      await fs.createReadStream(req.file.path)
        .pipe(unzipper.Extract({ path: projectPath }))
        .promise();
      fs.unlinkSync(req.file.path);
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
    const { name, description, github_url, project_type } = req.body;
    
    const project = await prisma.project.update({
      where: { id: req.params.id },
      data: {
        ...(name && { name }),
        ...(description !== undefined && { description }),
        ...(github_url !== undefined && { github_url: github_url }),
        ...(project_type && { project_type: project_type }),
      },
    });
    
    res.json(project);
  } catch (error) {
    console.error(error);
    res.status(500).json({ error: 'Failed to update project' });
  }
});

// Delete project
router.delete('/:id', async (req: Request, res: Response) => {
  try {
    const projectId = req.params.id;
    const projectPath = path.join(config.deploymentsPath, projectId);
    
    // Attempt to stop and remove Docker containers first
    if (fs.existsSync(projectPath)) {
      try {
        const { spawnSync } = await import('child_process');
        // Stop and remove containers, volumes, and images associated with the project
        spawnSync('docker', ['compose', '-p', `${projectId}`, 'down', '--volumes', '--remove-orphans', '--rmi', 'all'], { 
          cwd: projectPath,
          timeout: 60000, // Increased to 60 seconds for image removal
          shell: false
        });
      } catch (dockerError) {
        console.warn(`Docker cleanup warned for project ${projectId}:`, dockerError);
      }
      
      // Remove project files
      try {
        fs.rmSync(projectPath, { recursive: true, force: true });
      } catch (fileError) {
        console.warn(`File cleanup warned for project ${projectId}:`, fileError);
      }
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
        value,
        is_build_arg: is_build_arg ?? false,
        is_runtime: is_runtime ?? true,
      },
    });
    res.status(201).json(envVar);
  } catch (error) {
    res.status(500).json({ error: 'Failed to create environment variable' });
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

export default router;
