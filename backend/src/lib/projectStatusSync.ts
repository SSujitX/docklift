import prisma from './prisma.js';
import { getContainerStatus } from '../services/docker.js';

export type ServiceRuntimeStatus = 'running' | 'stopped' | 'error';

export function dockerStatusToServiceStatus(cs: {
  status: string;
  running: boolean;
}): ServiceRuntimeStatus {
  if (cs.running) return 'running';
  if (cs.status === 'not_found') return 'stopped';
  if (cs.status === 'dead') return 'error';
  return 'stopped';
}

export type ProjectAggregateStatus = 'running' | 'stopped' | 'error' | 'degraded';

export function aggregateProjectStatus(
  serviceStatuses: ServiceRuntimeStatus[],
): ProjectAggregateStatus {
  if (serviceStatuses.length === 0) return 'stopped';
  if (serviceStatuses.every((s) => s === 'running')) return 'running';
  if (serviceStatuses.some((s) => s === 'error')) return 'error';
  // Partial: some running, some stopped — never report full "running"
  if (serviceStatuses.some((s) => s === 'running')) return 'degraded';
  return 'stopped';
}

/** Inspect every service container and sync DB project + service rows. */
export async function syncProjectStatusFromContainers(
  projectId: string,
): Promise<ProjectAggregateStatus> {
  const services = await prisma.service.findMany({ where: { project_id: projectId } });
  const statuses: ServiceRuntimeStatus[] = [];

  for (const svc of services) {
    let svcStatus: ServiceRuntimeStatus = 'stopped';
    if (svc.container_name) {
      const cs = await getContainerStatus(svc.container_name);
      svcStatus = dockerStatusToServiceStatus(cs);
    }
    if (svc.status !== svcStatus) {
      await prisma.service.update({ where: { id: svc.id }, data: { status: svcStatus } });
    }
    statuses.push(svcStatus);
  }

  const projectStatus = aggregateProjectStatus(statuses);
  await prisma.project.update({
    where: { id: projectId },
    data: { status: projectStatus },
  });
  return projectStatus;
}
