import prisma from './prisma.js';
import { getContainerStatus } from '../services/docker.js';
import { syncProjectStatusFromContainers } from './projectStatusSync.js';

const INTERRUPTED = 'interrupted by restart';

export async function recoverDeploymentStateOnBoot(): Promise<void> {
  const inProgress = await prisma.deployment.findMany({
    where: { status: 'in_progress' },
    select: { id: true, logs: true },
  });
  for (const deployment of inProgress) {
    await prisma.deployment.update({
      where: { id: deployment.id },
      data: {
        status: 'failed',
        finished_at: new Date(),
        logs: `${deployment.logs || ''}\n❌ ${INTERRUPTED}\n`,
      },
    });
  }

  const buildingProjects = await prisma.project.findMany({
    where: { status: 'building' },
    select: { id: true },
  });
  for (const project of buildingProjects) {
    const services = await prisma.service.findMany({
      where: { project_id: project.id },
      select: { container_name: true },
    });
    let anyRunning = false;
    for (const svc of services) {
      if (!svc.container_name) continue;
      const cs = await getContainerStatus(svc.container_name);
      if (cs.running) anyRunning = true;
    }
    if (anyRunning) {
      await syncProjectStatusFromContainers(project.id);
    } else {
      await prisma.project.update({
        where: { id: project.id },
        data: { status: 'error' },
      });
      await prisma.service.updateMany({
        where: { project_id: project.id },
        data: { status: 'error' },
      });
    }
  }
}
