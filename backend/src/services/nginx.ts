import fs from 'fs';
import path from 'path';
import { spawn } from 'child_process';
import { config } from '../lib/config.js';

export async function updateServiceDomain(service: any) {
  // Ensure config directory exists
  if (!fs.existsSync(config.nginxConfPath)) {
    fs.mkdirSync(config.nginxConfPath, { recursive: true });
  }

  const confPath = path.join(config.nginxConfPath, `service-${service.id}.conf`);
  
  if (!service.domain) {
    if (fs.existsSync(confPath)) {
      fs.unlinkSync(confPath);
      await reloadNginx();
    }
    return;
  }
  
  const domains = service.domain.split(',').map((d: string) => d.trim()).filter(Boolean).join(' ');
  
  if (!domains) {
    if (fs.existsSync(confPath)) {
      fs.unlinkSync(confPath);
      await reloadNginx();
    }
    return;
  }

  // Use container name and internal port for routing within the docker network
  const upstream = `${service.container_name}:${service.internal_port}`;
  
  const content = `
server {
    listen 80;
    server_name ${domains};
    
    location / {
        proxy_pass http://${upstream};
        proxy_http_version 1.1;
        proxy_set_header Upgrade $http_upgrade;
        proxy_set_header Connection "upgrade";
        proxy_set_header Host $http_host;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto $scheme;
    }
}
`;

  try {
    fs.writeFileSync(confPath, content);
    console.log(`Updated Nginx config for ${service.name} (${domains})`);
    await reloadNginx();
  } catch (error) {
    console.error('Failed to write Nginx config:', error);
  }

}

export async function syncNginxConfigs() {
  try {
    if (!fs.existsSync(config.nginxConfPath)) return;
    
    // Get all conf files
    const files = fs.readdirSync(config.nginxConfPath).filter(f => f.startsWith('service-') && f.endsWith('.conf'));
    
    // Extract service IDs from filenames
    const fileServiceIds = files.map(f => {
      const match = f.match(/^service-(.+)\.conf$/);
      return match ? match[1] : null;
    }).filter(Boolean) as string[];
    
    if (fileServiceIds.length === 0) return;
    
    console.log(`Found ${fileServiceIds.length} Nginx config files. Verifying against database...`);
    
    // Check which ones exist in DB
    const { default: prisma } = await import('../lib/prisma.js');
    const dbServices = await prisma.service.findMany({
      where: {
        id: { in: fileServiceIds }
      },
      select: { id: true }
    });
    
    const dbServiceIds = new Set(dbServices.map(s => s.id));
    let changeMade = false;
    
    // Delete orphans
    for (const file of files) {
        const match = file.match(/^service-(.+)\.conf$/);
        const id = match ? match[1] : null;
        
        if (id && !dbServiceIds.has(id)) {
            console.log(`Removing orphaned Nginx config: ${file}`);
            fs.unlinkSync(path.join(config.nginxConfPath, file));
            changeMade = true;
        }
    }
    
    if (changeMade) {
        await reloadNginx();
    } else {
        console.log('Nginx configs are in sync.');
    }
    
  } catch (error) {
    console.error('Failed to sync Nginx configs:', error);
  }
}

export async function cleanupServiceDomain(serviceId: string) {
  const confPath = path.join(config.nginxConfPath, `service-${serviceId}.conf`);
  
  if (fs.existsSync(confPath)) {
    try {
      fs.unlinkSync(confPath);
      console.log(`Removed Nginx config for service ${serviceId}`);
      await reloadNginx();
    } catch (error) {
      console.error(`Failed to remove Nginx config for service ${serviceId}:`, error);
    }
  }
}

async function reloadNginx() {
  return new Promise<void>((resolve, reject) => {
    console.log('Reloading Nginx proxy...');
    const child = spawn('docker', ['exec', 'docklift-nginx-proxy', 'nginx', '-s', 'reload']);
    
    child.on('close', (code) => {
      if (code === 0) {
        console.log('Nginx proxy reloaded successfully');
        resolve();
      } else {
        console.error(`Nginx reload failed with code ${code}`);
        // resolve anyway to avoid breaking the request flow
        resolve();
      }
    });
    
    child.on('error', (err) => {
      console.error('Failed to spawn docker exec:', err);
      resolve();
    });
  });
}
