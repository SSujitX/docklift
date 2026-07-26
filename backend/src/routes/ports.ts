// Ports routes - API endpoints for port allocation management
import { Router, Request, Response } from 'express';
import prisma from '../lib/prisma.js';
import { config } from '../lib/config.js';
import * as dockerService from '../services/docker.js';

const router = Router();

// List all ports (+ projects running without a host port — private by default)
router.get('/', async (req: Request, res: Response) => {
  try {
    const dbPorts = await prisma.port.findMany({
      include: {
        project: {
          select: { id: true, name: true, status: true },
        },
      },
    });

    const portMap = new Map(dbPorts.map((p) => [p.port, p]));

    const ports = [];
    for (let p = config.portRangeStart; p <= config.portRangeEnd; p++) {
      if (portMap.has(p)) {
        ports.push(portMap.get(p));
      } else {
        ports.push({
          port: p,
          project_id: null,
          is_locked: false,
        });
      }
    }

    // Projects can be Running with zero host ports (domains / Docker DNS / DB linking)
    const allocatedProjectIds = new Set(
      dbPorts
        .filter((p) => p.is_locked && p.project_id)
        .map((p) => p.project_id as string),
    );

    const live = await prisma.project.findMany({
      where: {
        status: { in: ['running', 'degraded', 'building', 'pending'] },
      },
      select: {
        id: true,
        name: true,
        status: true,
        project_type: true,
        db_engine: true,
        publish_host_port: true,
        services: {
          select: { id: true, port: true, name: true },
        },
      },
      orderBy: { updated_at: 'desc' },
    });

    const private_running = live
      .filter((project) => {
        if (allocatedProjectIds.has(project.id)) return false;
        const hasHostPort = project.services.some(
          (s) => typeof s.port === 'number' && s.port > 0,
        );
        return !hasHostPort;
      })
      .map((project) => ({
        id: project.id,
        name: project.name,
        status: project.status,
        project_type: project.project_type,
        db_engine: project.db_engine,
        publish_host_port: project.publish_host_port === true,
        reason:
          project.project_type === 'database'
            ? 'Managed databases stay off the host — link them to apps over Docker DNS'
            : project.publish_host_port
              ? 'Publish host ports is on — redeploy to claim a pool port'
              : 'Private by default — add a domain or enable Publish host ports + redeploy',
      }));

    res.json({
      ports,
      private_running,
      pool: {
        start: config.portRangeStart,
        end: config.portRangeEnd,
      },
    });
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
