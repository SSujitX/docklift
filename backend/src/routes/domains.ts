// Domains routes - API endpoints for custom domain configuration
import express, { Request, Response, Router } from 'express';
import fs from 'fs/promises';
import path from 'path';
import { exec } from 'child_process';
import util from 'util';

const execAsync = util.promisify(exec);
const router: Router = express.Router();

// Path to Nginx configuration directory (mounted volume)
// Backend sees this at /nginx-conf (mapped to ./nginx-proxy/conf.d on host)
const NGINX_CONF_PATH = process.env.NGINX_CONF_PATH || '/nginx-conf';

interface DomainConfig {
  domain: string;
  port: number;
}

// Helper: Reload Nginx Proxy
async function reloadNginxProxy() {
  try {
    // Execute reload command inside the docklift-nginx-proxy container
    // Requires permissions (backend runs as privileged)
    await execAsync('docker exec docklift-nginx-proxy nginx -s reload');
    console.log('Nginx proxy reloaded successfully');
  } catch (error) {
    console.error('Failed to reload Nginx proxy:', error);
    throw new Error('Failed to reload Nginx configuration');
  }
}

// GET /api/domains - List all configured domains
router.get('/', async (req: Request, res: Response) => {
  try {
    // Ensure directory exists
    await fs.mkdir(NGINX_CONF_PATH, { recursive: true });

    const files = await fs.readdir(NGINX_CONF_PATH);
    const configs: DomainConfig[] = [];

    for (const file of files) {
      if (file.endsWith('.conf') && file !== 'default.conf') {
        const content = await fs.readFile(path.join(NGINX_CONF_PATH, file), 'utf-8');
        
        // Simple regex to parse domain and port from the generated config
        // Matches: server_name example.com; ... proxy_pass http://host.docker.internal:3001;
        const domainMatch = content.match(/server_name\s+(.*?);/);
        const portMatch = content.match(/proxy_pass\s+http:\/\/host\.docker\.internal:(\d+);/);

        if (domainMatch && portMatch) {
          configs.push({
            domain: domainMatch[1],
            port: parseInt(portMatch[1])
          });
        }
      }
    }

    res.json(configs);
  } catch (error: any) {
    console.error('List domains error:', error);
    res.status(500).json({ error: 'Failed to list domains' });
  }
});

// POST /api/domains - Add a new domain mapping
router.post('/', async (req: Request, res: Response) => {
  const { domain, port } = req.body;

  if (!domain || !port || isNaN(parseInt(port))) {
    return res.status(400).json({ error: 'Invalid domain or port' });
  }

  // Strict domain validation - must be valid hostname format (same as DELETE endpoint)
  if (!/^[a-zA-Z0-9]([a-zA-Z0-9-]*[a-zA-Z0-9])?(\.[a-zA-Z0-9]([a-zA-Z0-9-]*[a-zA-Z0-9])?)+$/.test(domain)) {
    return res.status(400).json({ error: 'Invalid domain format' });
  }

  const configFilename = `${domain}.conf`;
  const configPath = path.join(NGINX_CONF_PATH, configFilename);

  // Nginx Configuration Template
  // Uses host.docker.internal to reach services running on the host machine
  const nginxConfig = `server {
    listen 80;
    server_name ${domain};

    location / {
        proxy_pass http://host.docker.internal:${port};
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto $scheme;
    }
}`;

  try {
    // Write config file
    await fs.writeFile(configPath, nginxConfig);
    console.log(`Created Nginx config for ${domain} -> Port ${port}`);

    // Reload Nginx
    await reloadNginxProxy();

    res.json({ success: true, domain, port });
  } catch (error: any) {
    console.error('Add domain error:', error);
    res.status(500).json({ error: error.message || 'Failed to add domain' });
  }
});

// DELETE /api/domains/:domain - Remove a domain mapping
router.delete('/:domain', async (req: Request, res: Response) => {
  const { domain } = req.params;

  // Strict domain validation - must be valid hostname format
  if (!domain || !/^[a-zA-Z0-9]([a-zA-Z0-9-]*[a-zA-Z0-9])?(\.[a-zA-Z0-9]([a-zA-Z0-9-]*[a-zA-Z0-9])?)+$/.test(domain)) {
    return res.status(400).json({ error: 'Invalid domain format' });
  }

  const configFilename = `${domain}.conf`;
  const configPath = path.join(NGINX_CONF_PATH, configFilename);

  try {
    // Check if file exists
    await fs.access(configPath);
    
    // Delete file
    await fs.unlink(configPath);
    console.log(`Deleted Nginx config for ${domain}`);

    // Reload Nginx
    await reloadNginxProxy();

    res.json({ success: true });
  } catch (error: any) {
    if (error.code === 'ENOENT') {
      return res.status(404).json({ error: 'Domain configuration not found' });
    }
    console.error('Delete domain error:', error);
    res.status(500).json({ error: 'Failed to delete domain' });
  }
});

export default router;
