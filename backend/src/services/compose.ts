import fs from 'fs';
import path from 'path';
import yaml from 'js-yaml';
import { config } from '../lib/config.js';

interface ServiceConfig {
  name: string;
  dockerfile_path: string;
  context_path: string;
  internal_port: number;
  port?: number;
  container_name?: string;
}

interface EnvVar {
  key: string;
  value: string;
  is_build_arg: boolean;
  is_runtime: boolean;
}

// Directories to ignore when scanning
const IGNORE_DIRS = new Set([
  'node_modules', '.git', '.github', 'vendor', '__pycache__',
  '.venv', 'venv', '.next', 'dist', 'build', '.cache'
]);

// Detect port from Dockerfile
function detectPortFromDockerfile(dockerfilePath: string): number {
  try {
    const content = fs.readFileSync(dockerfilePath, 'utf-8');
    
    // Check EXPOSE directive
    const exposeMatch = content.match(/EXPOSE\s+(\d+)/i);
    if (exposeMatch) {
      return parseInt(exposeMatch[1]);
    }
    
    // Fallback based on common patterns
    const contentLower = content.toLowerCase();
    if (contentLower.includes('next') || contentLower.includes('react')) return 3000;
    if (contentLower.includes('uvicorn') || contentLower.includes('fastapi')) return 8000;
    if (contentLower.includes('flask')) return 5000;
    if (contentLower.includes('django')) return 8000;
    if (contentLower.includes('express') || contentLower.includes('node')) return 3000;
    
    return 3000; // Default
  } catch {
    return 3000;
  }
}

// Scan for Dockerfiles in project
export function scanDockerfiles(projectPath: string, maxDepth = 2): ServiceConfig[] {
  const services: ServiceConfig[] = [];
  
  function scanDir(dirPath: string, depth: number) {
    if (depth > maxDepth) return;
    
    try {
      const items = fs.readdirSync(dirPath);
      
      for (const item of items) {
        const itemPath = path.join(dirPath, item);
        const stat = fs.statSync(itemPath);
        
        if (stat.isDirectory()) {
          if (!IGNORE_DIRS.has(item)) {
            scanDir(itemPath, depth + 1);
          }
        } else if (item === 'Dockerfile') {
          const relPath = path.relative(projectPath, itemPath);
          const parentDir = path.relative(projectPath, dirPath);
          
          const name = parentDir === '' ? 'app' : parentDir.replace(/[/\\]/g, '-');
          const internalPort = detectPortFromDockerfile(itemPath);
          
          services.push({
            name,
            dockerfile_path: relPath,
            context_path: parentDir || '.',
            internal_port: internalPort,
          });
        }
      }
    } catch {
      // Permission denied or other error
    }
  }
  
  scanDir(projectPath, 0);
  return services;
}

// Generate docker-compose.yml
export function generateCompose(
  projectId: string,
  projectPath: string,
  services: ServiceConfig[],
  envVars: EnvVar[] = [],
  projectType = 'app'
): void {
  const composeConfig: Record<string, unknown> = {
    services: {},
    networks: {
      default: {
        external: true,
        name: config.dockerNetwork,
      },
    },
  };
  
  for (const svc of services) {
    const serviceDef: Record<string, unknown> = {
      build: {
        context: svc.context_path,
        dockerfile: path.basename(svc.dockerfile_path),
      },
      container_name: svc.container_name || `docklift_${projectId}_${svc.name}`,
      ports: [`${svc.port}:${svc.internal_port}`],
      restart: 'unless-stopped',
    };
    
    // Add runtime environment variables
    const runtimeVars = envVars.filter(v => v.is_runtime);
    if (runtimeVars.length > 0) {
      serviceDef.environment = runtimeVars.reduce((acc, v) => {
        acc[v.key] = v.value;
        return acc;
      }, {} as Record<string, string>);
    }
    
    // Add build args
    const buildArgs = envVars.filter(v => v.is_build_arg);
    if (buildArgs.length > 0) {
      (serviceDef.build as Record<string, unknown>).args = buildArgs.reduce((acc, v) => {
        acc[v.key] = v.value;
        return acc;
      }, {} as Record<string, string>);
    }
    
    (composeConfig.services as Record<string, unknown>)[svc.name] = serviceDef;
  }
  
  const yamlContent = yaml.dump(composeConfig, { lineWidth: -1 });
  fs.writeFileSync(path.join(projectPath, 'docker-compose.yml'), yamlContent);
}

// Check if compose file exists
export function checkComposeExists(projectPath: string): boolean {
  return fs.existsSync(path.join(projectPath, 'docker-compose.yml'));
}
