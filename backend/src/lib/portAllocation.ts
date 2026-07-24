import prisma from './prisma.js';
import { config } from './config.js';

function isUniqueViolation(error: unknown): boolean {
  return Boolean(error && typeof error === 'object' && (error as { code?: string }).code === 'P2002');
}

/** Allocate a host port for a project without stealing another project's assignment. */
export async function allocatePort(projectId: string): Promise<number> {
  const port = await prisma.$transaction(async (tx) => {
    const locked = await tx.port.findMany({
      where: { is_locked: true },
      select: { port: true },
    });
    const lockedSet = new Set(locked.map((row) => row.port));

    for (let candidate = config.portRangeStart; candidate <= config.portRangeEnd; candidate++) {
      if (lockedSet.has(candidate)) continue;

      const existing = await tx.port.findUnique({ where: { port: candidate } });
      if (existing) {
        if (existing.is_locked || existing.project_id != null) {
          continue;
        }
        const claimed = await tx.port.updateMany({
          where: { port: candidate, is_locked: { not: true }, project_id: null },
          data: { project_id: projectId, is_locked: true },
        });
        if (claimed.count === 1) {
          return candidate;
        }
        continue;
      }

      try {
        await tx.port.create({
          data: { port: candidate, project_id: projectId, is_locked: true },
        });
        return candidate;
      } catch (error) {
        if (isUniqueViolation(error)) {
          continue;
        }
        throw error;
      }
    }

    throw new Error(`No free ports in range ${config.portRangeStart}-${config.portRangeEnd}`);
  });

  await prisma.project
    .update({
      where: { id: projectId },
      data: { port },
    })
    .catch(() => {});

  return port;
}
