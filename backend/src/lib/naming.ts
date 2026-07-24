/**
 * Readable, unique Docker names for deployed projects.
 *
 * Compose project (-p):  dl-<slug>-<8charId>   → images like dl-python-smoke-53b01966-app
 * Container:             dl_<slug>_<8charId>_<svc> → dl_python-smoke_53b01966_app
 */

const SLUG_MAX = 24;
const SERVICE_MAX = 20;

/** Sanitize a project or service label for Docker names */
export function dockerSlug(input: string, maxLen = SLUG_MAX): string {
  const slug = (input || 'app')
    .toLowerCase()
    .replace(/[^a-z0-9]+/g, '-')
    .replace(/^-+|-+$/g, '')
    .slice(0, maxLen)
    .replace(/-+$/g, '');
  return slug || 'app';
}

export function shortProjectId(projectId: string): string {
  return projectId.replace(/-/g, '').slice(0, 8);
}

/** Docker Compose project name (`docker compose -p …`) — also prefixes built image names */
export function composeProjectName(projectName: string, projectId: string): string {
  return `dl-${dockerSlug(projectName)}-${shortProjectId(projectId)}`;
}

/** Explicit container_name in generated compose files */
export function serviceContainerName(
  projectName: string,
  projectId: string,
  serviceName: string,
): string {
  const slug = dockerSlug(projectName);
  const id = shortProjectId(projectId);
  const svc = dockerSlug(serviceName, SERVICE_MAX).replace(/-/g, '_');
  // Docker container name max practical length ~63
  return `dl_${slug}_${id}_${svc}`.slice(0, 63);
}

/** All compose project names that may exist for this project (current + legacy UUID) */
export function composeProjectAliases(projectName: string, projectId: string): string[] {
  const current = composeProjectName(projectName, projectId);
  const aliases = [current, projectId];
  return [...new Set(aliases)];
}
