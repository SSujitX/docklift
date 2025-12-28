// Nginx service - manages reverse proxy configurations for custom domains
import fs from 'fs';
import path from 'path';
import { spawn } from 'child_process';
import { config } from '../lib/config.js';
import * as dockerService from './docker.js';

export async function updateServiceDomain(service: any) {
  // Ensure config directory exists
  if (!fs.existsSync(config.nginxConfPath)) {
    fs.mkdirSync(config.nginxConfPath, { recursive: true });
  }

  const confPath = path.join(config.nginxConfPath, `service-${service.id}.conf`);
  
  // Only generate config if service is running and has a domain
  const shouldExist = service.domain && service.container_name && (service.status === 'running' || service.status === 'starting');
  
  if (!shouldExist) {
    if (fs.existsSync(confPath)) {
      fs.unlinkSync(confPath);
      await reloadNginx();
    }
    return;
  }
  
  // Verify container actually exists in Docker to prevent Nginx crash
  try {
    const containerStatus = await dockerService.getContainerStatus(service.container_name);
    if (!containerStatus.running && service.status !== 'starting') {
       console.warn(`Container ${service.container_name} not running, skipping Nginx config to prevent crash.`);
       if (fs.existsSync(confPath)) {
          fs.unlinkSync(confPath);
          await reloadNginx();
       }
       return;
    }
  } catch (e) {
    // If error checking container, err on side of caution
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
  
  // Parse domains and generate www variants for redirect
  const domainsArray = service.domain.split(',').map((d: string) => d.trim()).filter(Boolean);
  
  // Separate main domains from www domains
  const mainDomains: string[] = [];
  const wwwRedirects: string[] = [];
  
  for (const domain of domainsArray) {
    if (domain.startsWith('www.')) {
      // If user explicitly adds www, add it to main domains
      mainDomains.push(domain);
    } else {
      // For non-www domains, add to main and create www redirect
      mainDomains.push(domain);
      // Only add www redirect for actual domains (not IPs or localhost)
      if (!domain.match(/^[\d.]+$/) && !domain.includes('localhost')) {
        wwwRedirects.push(`www.${domain}`);
      }
    }
  }
  
  const mainDomainsStr = mainDomains.join(' ');
  const wwwDomainsStr = wwwRedirects.join(' ');
  
  // Use a variable for proxy_pass and add a resolver. 
  // This prevents Nginx from failing to start/reload if the container is not yet in DNS.
  let content = `
# Main server block for ${service.name}
server {
    listen 80;
    server_name ${mainDomainsStr};

    # Docker internal DNS resolver
    resolver 127.0.0.11 valid=30s ipv6=off;
    
    location / {
        set $target_${service.id.replace(/-/g, '_')} ${service.container_name};
        proxy_pass http://$target_${service.id.replace(/-/g, '_')}:${service.internal_port};
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

  // Add www redirect block if there are www domains to redirect
  if (wwwRedirects.length > 0) {
    content += `
# WWW to non-WWW redirect for ${service.name}
server {
    listen 80;
    server_name ${wwwDomainsStr};
    
    # 301 permanent redirect to non-www
    return 301 http://$host$request_uri;
}
`;
    // Fix: return 301 should redirect to the non-www version
    // We need to extract the domain without www
    content = content.replace(
      'return 301 http://$host$request_uri;',
      `return 301 $scheme://${mainDomains[0]}$request_uri;`
    );
  }


  try {
    fs.writeFileSync(confPath, content);
    console.log(`Updated Nginx config for ${service.name} (${mainDomainsStr}${wwwRedirects.length > 0 ? ` + www redirect` : ''})`);
    await reloadNginx();
  } catch (error) {
    console.error('Failed to write Nginx config:', error);
  }
}

export async function syncNginxConfigs() {
  try {
    if (!fs.existsSync(config.nginxConfPath)) return;
    
    // 1. Get all existing conf files
    const files = fs.readdirSync(config.nginxConfPath).filter(f => f.startsWith('service-') && f.endsWith('.conf'));
    const fileServiceIds = new Set(files.map(f => {
      const match = f.match(/^service-(.+)\.conf$/);
      return match ? match[1] : null;
    }).filter(Boolean) as string[]);
    
    // 2. Get all Services from DB that SHOULD have config
    // (Running, have domain, have container_name)
    const { default: prisma } = await import('../lib/prisma.js');
    const allServices = await prisma.service.findMany({
        where: {
            domain: { not: null },
            status: 'running' // Only running services!
        }
    });

    let changeMade = false;
    const activeServiceIds = new Set();

    // 3. Update/Create valid configs
    for (const service of allServices) {
        if (!service.container_name) continue;
        
        // Verify container exists to be safe
        try {
            const status = await dockerService.getContainerStatus(service.container_name);
            if (!status.running) continue;
        } catch (e) { continue; }

        activeServiceIds.add(service.id);
        
        // If file missing or we just want to ensure it's correct (simplified: just regenerate if missing)
        // Actually, let's regenerate all valid ones to be sure content is correct
        // But optimization: check if file exists.
        const confPath = path.join(config.nginxConfPath, `service-${service.id}.conf`);
        
        // If file doesn't exist, Create it
        if (!fileServiceIds.has(service.id)) {
             console.log(`Restoring missing Nginx config for ${service.name}`);
             await updateServiceDomain(service); // This uses reloadNginx inside, so we might reload multiple times. That's fine for startup.
             // Note: updateServiceDomain handles check for domain string validity
        }
    }

    // 4. Delete orphans (Files that exist but are NOT in the active bucket)
    // This catches: Deleted services, Stopped services, Dead containers
    const filesToDelete = [...fileServiceIds].filter(id => !activeServiceIds.has(id));
    
    for (const id of filesToDelete) {
        const file = `service-${id}.conf`;
        console.log(`Removing invalid/orphaned Nginx config: ${file}`);
        fs.unlinkSync(path.join(config.nginxConfPath, file));
        changeMade = true;
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
    
    let stderrBuffer = '';

    child.stdout.on('data', (data) => console.log(`Nginx stdout: ${data}`));
    child.stderr.on('data', (data) => {
        const str = data.toString();
        stderrBuffer += str;
        console.error(`Nginx stderr: ${str}`);
    });

    child.on('close', (code) => {
      if (code === 0) {
        console.log('Nginx proxy reloaded successfully');
        resolve();
      } else {
        console.error(`Nginx reload failed with code ${code}`);
        
        // Self-Healing: Check for "host not found" error
        const match = stderrBuffer.match(/host not found in upstream .* in (.*\/service-[a-zA-Z0-9-]+\.conf):/);
        if (match && match[1]) {
            const badConfPath = match[1];
            // The path reported by Nginx is inside the container (/etc/nginx/conf.d/...)
            // We need to map it to our local path
            const filename = path.basename(badConfPath);
            const localPath = path.join(config.nginxConfPath, filename);
            
            if (fs.existsSync(localPath)) {
                console.log(`Self-Healing: Removing bad Nginx config causing crash: ${filename}`);
                try {
                    fs.unlinkSync(localPath);
                    console.log('Bad config removed. Nginx should stabilize on next reload.');
                    // Optional: Trigger another reload immediately? 
                    // Let's just resolve to avoid infinite loops, next action will fix it.
                } catch (err) {
                    console.error('Failed to remove bad config:', err);
                }
            }
        }
        
        resolve();
      }
    });
    
    child.on('error', (err) => {
      console.error('Failed to spawn docker exec:', err);
      resolve();
    });
  });
}
