// In-memory active deploy/cancel guard — shared without importing route modules.
// Ownership is keyed by deploymentId (or a cancel sentinel) so:
// - a cancelled deploy's catch cannot clear a newer deploy's lock
// - a cancelled deploy's catch cannot clear a cancel teardown still in progress

const CANCEL_PREFIX = 'cancel:';

const activeDeploymentByProject = new Map<string, string>();

export function isProjectDeploying(projectId: string): boolean {
  return activeDeploymentByProject.has(projectId);
}

export function isProjectCancelling(projectId: string): boolean {
  const owner = activeDeploymentByProject.get(projectId);
  return typeof owner === 'string' && owner.startsWith(CANCEL_PREFIX);
}

/** Acquire the deploy lock for this deployment. False if another deploy/cancel owns it. */
export function acquireProjectDeploying(
  projectId: string,
  deploymentId: string,
): boolean {
  const current = activeDeploymentByProject.get(projectId);
  if (current && current !== deploymentId) return false;
  activeDeploymentByProject.set(projectId, deploymentId);
  return true;
}

/** Current lock owner (deployment id or `cancel:…`), if any. */
export function getProjectDeployOwner(projectId: string): string | undefined {
  return activeDeploymentByProject.get(projectId);
}

/**
 * Cancel takes ownership for the whole teardown window.
 * In-flight deploy release(deploymentId) will no-op while this sentinel is set.
 * Returns the previous owner (deployment id) when cancel steals from a deploy.
 */
export function beginProjectCancel(projectId: string): string | undefined {
  const previous = activeDeploymentByProject.get(projectId);
  activeDeploymentByProject.set(projectId, `${CANCEL_PREFIX}${Date.now()}`);
  if (previous && !previous.startsWith(CANCEL_PREFIX)) return previous;
  return undefined;
}

/** True only while this deployment id still holds the lock (not cancel/stolen). */
export function ownsProjectDeploying(
  projectId: string,
  deploymentId: string,
): boolean {
  return activeDeploymentByProject.get(projectId) === deploymentId;
}

/** Release only if this deployment still owns the lock. */
export function releaseProjectDeploying(
  projectId: string,
  deploymentId: string,
): void {
  if (activeDeploymentByProject.get(projectId) === deploymentId) {
    activeDeploymentByProject.delete(projectId);
  }
}

/** Cancel/teardown complete (or failed): drop the lock regardless of owner. */
export function clearProjectDeploying(projectId: string): void {
  activeDeploymentByProject.delete(projectId);
}
