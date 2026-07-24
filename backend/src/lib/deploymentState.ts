// In-memory active deploy guard — shared without importing route modules
const activeDeploymentProjects = new Set<string>();

export function isProjectDeploying(projectId: string): boolean {
  return activeDeploymentProjects.has(projectId);
}

export function setProjectDeploying(projectId: string, deploying: boolean): void {
  if (deploying) {
    activeDeploymentProjects.add(projectId);
  } else {
    activeDeploymentProjects.delete(projectId);
  }
}
