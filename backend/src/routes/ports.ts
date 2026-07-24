// Ports routes - API endpoints for port allocation management
import { Router, Request, Response } from 'express';
import prisma from '../lib/prisma.js';
import { config } from '../lib/config.js';

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
    const port = parseInt(req.params.port);
    
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
