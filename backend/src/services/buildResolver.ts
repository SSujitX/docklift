import fs from 'fs';
import path from 'path';
import { scanDockerfiles } from './compose.js';

export type BuildType = 'auto' | 'dockerfile' | 'railpack';

export interface BuildSettings {
  buildType?: string | null;
  baseDirectory?: string | null;
  dockerfilePath?: string | null;
  internalPort?: number | null;
}

export interface ResolvedBuildService {
  name: string;
  builder: 'dockerfile' | 'railpack';
  contextPath: string;
  dockerfilePath: string | null;
  internalPort: number;
}

export interface ResolvedBuild {
  requestedType: BuildType;
  resolvedType: 'dockerfile' | 'railpack';
  baseDirectory: string;
  dockerfilePath: string | null;
  detected: string;
  manifests: string[];
  services: ResolvedBuildService[];
}

const MANIFESTS = [
  'package.json',
  'bun.lock',
  'bun.lockb',
  'pnpm-lock.yaml',
  'yarn.lock',
  'package-lock.json',
  'requirements.txt',
  'pyproject.toml',
  'Pipfile',
  'poetry.lock',
  'go.mod',
  'Cargo.toml',
  'composer.json',
  'Gemfile',
  'pom.xml',
  'build.gradle',
  'build.gradle.kts',
];

export function normalizeBuildType(value: unknown): BuildType {
  return value === 'dockerfile' || value === 'railpack' ? value : 'auto';
}

/** Resolve a user-controlled relative path without allowing deployment-root escape. */
export function resolveProjectPath(projectPath: string, relativeValue: string | null | undefined): string {
  const relative = (relativeValue || '.').trim().replace(/\\/g, '/');
  if (!relative || relative === '.') return path.resolve(projectPath);
  if (path.isAbsolute(relative) || relative.split('/').includes('..')) {
    throw new Error('Build paths must stay inside the project directory');
  }
  const resolved = path.resolve(projectPath, relative);
  const root = path.resolve(projectPath);
  if (resolved !== root && !resolved.startsWith(root + path.sep)) {
    throw new Error('Build paths must stay inside the project directory');
  }
  return resolved;
}

function relativeFromProject(projectPath: string, absolutePath: string): string {
  return path.relative(projectPath, absolutePath).replace(/\\/g, '/') || '.';
}

function assertRealPathInside(projectPath: string, targetPath: string): void {
  const root = fs.realpathSync(projectPath);
  const target = fs.realpathSync(targetPath);
  if (target !== root && !target.startsWith(root + path.sep)) {
    throw new Error('Build paths and symlinks must stay inside the project directory');
  }
}

export function detectManifests(basePath: string): string[] {
  return MANIFESTS.filter((file) => fs.existsSync(path.join(basePath, file)));
}

export function resolveProjectBuild(
  projectPath: string,
  settings: BuildSettings
): ResolvedBuild {
  const requestedType = normalizeBuildType(settings.buildType);
  const basePath = resolveProjectPath(projectPath, settings.baseDirectory);
  if (!fs.existsSync(basePath) || !fs.statSync(basePath).isDirectory()) {
    throw new Error(`Base directory not found: ${settings.baseDirectory || '.'}`);
  }
  assertRealPathInside(projectPath, basePath);
  const baseDirectory = relativeFromProject(projectPath, basePath);
  const internalPort =
    Number.isInteger(settings.internalPort) &&
    Number(settings.internalPort) > 0 &&
    Number(settings.internalPort) <= 65535
      ? Number(settings.internalPort)
      : 3000;
  const manifests = detectManifests(basePath);

  if (requestedType === 'dockerfile') {
    const configured = (settings.dockerfilePath || 'Dockerfile').trim();
    const dockerfile = resolveProjectPath(basePath, configured);
    if (!fs.existsSync(dockerfile) || !fs.statSync(dockerfile).isFile()) {
      throw new Error(`Dockerfile not found: ${relativeFromProject(projectPath, dockerfile)}`);
    }
    assertRealPathInside(projectPath, dockerfile);
    return {
      requestedType,
      resolvedType: 'dockerfile',
      baseDirectory,
      dockerfilePath: relativeFromProject(projectPath, dockerfile),
      detected: 'Configured Dockerfile',
      manifests,
      services: [{
        name: 'app',
        builder: 'dockerfile',
        contextPath: baseDirectory,
        dockerfilePath: relativeFromProject(projectPath, dockerfile),
        internalPort,
      }],
    };
  }

  if (requestedType === 'auto') {
    const dockerfiles = scanDockerfiles(basePath);
    if (dockerfiles.length > 0) {
      const services = dockerfiles.map((item) => {
        const contextAbsolute = path.resolve(basePath, item.context_path);
        const dockerfileAbsolute = path.resolve(basePath, item.dockerfile_path);
        return {
          name: item.name,
          builder: 'dockerfile' as const,
          contextPath: relativeFromProject(projectPath, contextAbsolute),
          dockerfilePath: relativeFromProject(projectPath, dockerfileAbsolute),
          internalPort: item.internal_port || internalPort,
        };
      });
      return {
        requestedType,
        resolvedType: 'dockerfile',
        baseDirectory,
        dockerfilePath: services[0].dockerfilePath,
        detected: `${services.length} repository Dockerfile${services.length === 1 ? '' : 's'}`,
        manifests,
        services,
      };
    }
  }

  return {
    requestedType,
    resolvedType: 'railpack',
    baseDirectory,
    dockerfilePath: null,
    detected: manifests.length
      ? `Railpack source (${manifests.join(', ')})`
      : 'Railpack source (framework detection runs during build)',
    manifests,
    services: [{
      name: 'app',
      builder: 'railpack',
      contextPath: baseDirectory,
      dockerfilePath: null,
      internalPort,
    }],
  };
}
