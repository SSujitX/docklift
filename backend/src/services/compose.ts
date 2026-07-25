// Docker Compose service - scans for Dockerfiles and generates docker-compose.yml
import fs from 'fs';
import path from 'path';
import yaml from 'js-yaml';
import { projectNetworkName, shortPathHash } from '../lib/naming.js';
import { envForService } from '../lib/envVariables.js';

interface ServiceConfig {
  name: string;
  dockerfile_path: string;
  context_path: string;
  internal_port: number;
}

interface EnvVar {
  key: string;
  value: string;
  is_build_arg: boolean;
  is_runtime: boolean;
  /** Empty/undefined = shared; otherwise Docker service name */
  service_name?: string;
}

export interface RuntimeServiceConfig {
  name: string;
  image: string;
  internal_port: number;
  /** Host port when publish_host_port is enabled; omit/null = no host publish */
  port?: number | null;
  container_name: string;
  volumes?: Array<{ key: string; name: string; mountPath: string }>;
}

export interface RuntimeComposeOptions {
  projectId: string;
  publishHostPort?: boolean;
  /** Soft defaults — apps can still OOM if they ignore cgroup limits */
  memLimit?: string;
  cpus?: number;
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
        const stat = fs.lstatSync(itemPath);
        // Never traverse repository symlinks: build discovery must stay inside the
        // checked-out source tree.
        if (stat.isSymbolicLink()) continue;
        
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
  return dedupeScannedServices(services);
}

/** Resolve Dockerfile scan name collisions using a short hash of each service path. */
export function dedupeScannedServices(services: ServiceConfig[]): ServiceConfig[] {
  if (services.length === 0) return services;

  const usedNames = new Set<string>();
  const deduped: ServiceConfig[] = [];

  for (const svc of services) {
    let name = svc.name;
    if (usedNames.has(name)) {
      name = `${svc.name}-${shortPathHash(svc.dockerfile_path)}`;
    }
    if (usedNames.has(name)) {
      throw new Error(
        `Duplicate service name "${name}" after scan deduplication — adjust Dockerfile layout or paths.`,
      );
    }
    usedNames.add(name);
    deduped.push(name === svc.name ? svc : { ...svc, name });
  }

  return deduped;
}

/**
 * Write DockLift-owned runtime state outside the source checkout. Repository
 * Dockerfiles and docker-compose.yml files are never modified.
 *
 * Isolation: each project gets its own bridge network. The edge proxy is
 * attached to that network after `compose up` (see docker.connectProxyToProjectNetwork).
 * Control-plane services stay on docklift_network only.
 */
export function generateRuntimeCompose(
  composePath: string,
  services: RuntimeServiceConfig[],
  envVars: EnvVar[] = [],
  options?: RuntimeComposeOptions
): void {
  const projectId = options?.projectId || 'unknown';
  const netName = projectNetworkName(projectId);
  const publishHost = options?.publishHostPort === true;
  // Soft defaults: enough for Node/Python and common DB images. Override via options.
  // Do NOT cap_drop ALL by default — Postgres/MySQL/Redis often need extra caps to init.
  const memLimit = options?.memLimit; // unset = no compose mem_limit
  const cpus = options?.cpus; // unset = no compose cpus limit

  const composeConfig: Record<string, any> = {
    services: {},
    networks: {
      project: {
        name: netName,
        labels: {
          'com.docklift.managed': 'true',
          'com.docklift.project': projectId,
        },
      },
    },
  };
  const topLevelVolumes: Record<string, { name: string; external: true }> = {};

  for (const service of services) {
    const runtimeEnv = envForService(envVars, service.name)
      .filter((item) => item.is_runtime)
      .reduce<Record<string, string>>((acc, item) => {
        acc[item.key] = item.value;
        return acc;
      }, {});
    const labels: Record<string, string> = {
      'com.docklift.managed': 'true',
      'com.docklift.project': projectId,
      'com.docklift.service': service.name,
    };
    const definition: Record<string, unknown> = {
      image: service.image,
      container_name: service.container_name,
      restart: 'unless-stopped',
      networks: ['project'],
      labels,
      environment: {
        ...runtimeEnv,
        PORT: String(service.internal_port),
      },
      security_opt: ['no-new-privileges:true'],
    };
    if (memLimit) definition.mem_limit = memLimit;
    if (cpus != null) definition.cpus = String(cpus);

    if (publishHost && service.port != null) {
      definition.ports = [`${service.port}:${service.internal_port}`];
    }

    if (service.volumes?.length) {
      definition.volumes = service.volumes.map((volume) => {
        topLevelVolumes[volume.key] = { name: volume.name, external: true };
        return `${volume.key}:${volume.mountPath}`;
      });
    }
    composeConfig.services[service.name] = definition;
  }
  if (Object.keys(topLevelVolumes).length) {
    composeConfig.volumes = topLevelVolumes;
  }

  fs.mkdirSync(path.dirname(composePath), { recursive: true });
  fs.writeFileSync(composePath, yaml.dump(composeConfig, { lineWidth: -1, noRefs: true }));
}

// Validate that build args are declared in Dockerfile
export function validateDockerBuildArgs(dockerfilePath: string, buildArgs: string[]): string[] {
  try {
    if (!fs.existsSync(dockerfilePath)) return [];
    
    const content = fs.readFileSync(dockerfilePath, 'utf-8');
    const missingArgs: string[] = [];
    
    // Simple regex to find ARG instructions
    // Matches: ARG variable_name or ARG variable_name=default
    const argRegex = /^\s*ARG\s+([a-zA-Z0-9_]+)/gm;
    const declaredArgs = new Set<string>();
    
    let match;
    while ((match = argRegex.exec(content)) !== null) {
      declaredArgs.add(match[1]);
    }
    
    for (const arg of buildArgs) {
      if (!declaredArgs.has(arg)) {
        missingArgs.push(arg);
      }
    }
    
    return missingArgs;
  } catch (error) {
    console.warn('Failed to validate Dockerfile args:', error);
    return [];
  }
}

/** True when Dockerfile mounts a BuildKit secret id (RUN --mount=type=secret,id=…). */
export function dockerfileMountsSecret(dockerfilePath: string, secretId: string): boolean {
  try {
    if (!fs.existsSync(dockerfilePath)) return false;
    const content = fs.readFileSync(dockerfilePath, 'utf-8');
    const re = new RegExp(
      `type\\s*=\\s*secret[^\\n]*id\\s*=\\s*${secretId.replace(/[.*+?^${}()|[\]\\]/g, '\\$&')}`,
      'i'
    );
    return re.test(content);
  } catch {
    return false;
  }
}
