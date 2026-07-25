import type { Request, Response } from 'express';
import bcrypt from 'bcrypt';
import prisma from './prisma.js';
import type { AuthenticatedRequest } from './authMiddleware.js';

/**
 * Re-verify the admin password for dangerous operations (restore, purge, etc.).
 * JWT alone is not enough for these control-plane actions.
 */
export async function requireStepUpPassword(
  req: Request,
  res: Response
): Promise<boolean> {
  const password = (req.body as { password?: unknown })?.password;
  if (!password || typeof password !== 'string') {
    res.status(403).json({
      error: 'Password confirmation required for this operation',
      requirePassword: true,
    });
    return false;
  }

  const userId = (req as AuthenticatedRequest).user?.userId;
  if (!userId || userId === 'internal') {
    res.status(401).json({ error: 'Unauthorized' });
    return false;
  }

  const user = await prisma.user.findUnique({ where: { id: userId } });
  if (!user?.password) {
    res.status(403).json({ error: 'Unable to verify password', requirePassword: true });
    return false;
  }

  const ok = await bcrypt.compare(password, user.password);
  if (!ok) {
    res.status(403).json({ error: 'Invalid password', requirePassword: true });
    return false;
  }
  return true;
}
