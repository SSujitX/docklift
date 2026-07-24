// Ports routes - API endpoints for port allocation management
import { Router, Request, Response } from 'express';
import prisma from '../lib/prisma.js';
import { config } from '../lib/config.js';
import * as dockerService from '../services/docker.js';

const router = Router();

// List all ports
router.get('/', async (req: Request, res: Response) => {
  try {
    const dbPorts = await prisma.port.findMany({
      include: {
        project: {
          select: { id: true, name: true, status: true }
        }
      },
    });
    
    // Create a map for easy lookup
    const portMap = new Map(dbPorts.map(p => [p.port, p]));
    
    const allPorts = [];
    for (let p = config.portRangeStart; p <= config.portRangeEnd; p++) {
      if (portMap.has(p)) {
        allPorts.push(portMap.get(p));
      } else {
        allPorts.push({
          port: p,
          project_id: null,
          is_locked: false,
        });
      }
    }
    
    res.json(allPorts);
  } catch (error) {
    console.error(error);
    res.status(500).json({ error: 'Failed to list ports' });
  }
});

// Delete port allocation
router.delete('/:port', async (req: Request, res: Response) => {
  try {
    const port = parseInt(req.params.port, 10);
    if (!Number.isInteger(port)) {
      return res.status(400).json({ error: 'Invalid port' });
    }

    const servicesOnPort = await prisma.service.findMany({
      where: { port },
      select: { container_name: true },
    });
    const projectRef = await prisma.project.findFirst({ where: { port } });

    for (const svc of servicesOnPort) {
      if (!svc.container_name) continue;
      try {
        const status = await dockerService.getContainerStatus(svc.container_name);
        if (status.running) {
          return res.status(409).json({ error: 'Port is in use by a running container' });
        }
      } catch {
        // ignore inspect errors
      }
    }

    if (servicesOnPort.length > 0 || projectRef) {
      return res.status(409).json({
        error: 'Port is still assigned to a service or project',
      });
    }

    await prisma.port.deleteMany({
      where: { port },
    });
    
    res.json({ status: 'deleted' });
  } catch (error) {
    console.error(error);
    res.status(500).json({ error: 'Failed to delete port' });
  }
});

export default router;
