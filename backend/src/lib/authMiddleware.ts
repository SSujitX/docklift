// Auth middleware - verifies JWT token and attaches user to request
import { Request, Response, NextFunction } from 'express';
import jwt from 'jsonwebtoken';
import crypto from 'crypto';
import fs from 'fs';
import path from 'path';
import { config } from './config.js';
import prisma from './prisma.js';

// Generate secure random secret
const generateSecureSecret = () => crypto.randomBytes(64).toString('hex');

// Secrets file path in data directory
const SECRETS_FILE = path.join(config.dataPath, '.secrets');

// Load or generate secrets with auto-persistence
function loadOrCreateSecrets(): { jwtSecret: string; internalApiSecret: string } {
  // Priority 1: Environment variables (explicit configuration)
  if (process.env.JWT_SECRET && process.env.INTERNAL_API_SECRET) {
    console.log('🔐 Using secrets from environment variables');
    return {
      jwtSecret: process.env.JWT_SECRET,
      internalApiSecret: process.env.INTERNAL_API_SECRET,
    };
  }

  // Priority 2: Load from persisted secrets file
  try {
    if (fs.existsSync(SECRETS_FILE)) {
      const data = JSON.parse(fs.readFileSync(SECRETS_FILE, 'utf-8'));
      if (data.jwtSecret && data.internalApiSecret) {
        console.log('🔐 Loaded persisted secrets');
        return data;
      }
    }
  } catch (error) {
    console.warn('⚠️  Failed to load secrets file, generating new ones');
  }

  // Priority 3: Generate new secrets and persist them
  const secrets = {
    jwtSecret: process.env.JWT_SECRET || generateSecureSecret(),
    internalApiSecret: process.env.INTERNAL_API_SECRET || generateSecureSecret(),
  };

  // Ensure data directory exists
  if (!fs.existsSync(config.dataPath)) {
    fs.mkdirSync(config.dataPath, { recursive: true });
  }

  // Save secrets to file (mode 0o600 = owner read/write only)
  try {
    fs.writeFileSync(SECRETS_FILE, JSON.stringify(secrets, null, 2), { mode: 0o600 });
    console.log('🔐 Generated and saved new secrets');
  } catch (error) {
    console.warn('⚠️  Could not persist secrets (sessions will reset on restart)');
  }

  return secrets;
}

const secrets = loadOrCreateSecrets();
const JWT_SECRET = secrets.jwtSecret;
const INTERNAL_API_SECRET = secrets.internalApiSecret;

export interface AuthenticatedRequest extends Request {
  user?: {
    userId: string;
    email: string;
    role: string;
    purpose?: string;
  };
}

// Export for use in auth.ts and github.ts
export { JWT_SECRET, INTERNAL_API_SECRET };

export type JwtPayload = {
  userId: string;
  email: string;
  role: string;
  purpose?: string;
  pwdv?: number;
  iat?: number;
  exp?: number;
};

function authError(res: Response, error: any) {
  if (error?.name === 'JsonWebTokenError') {
    return res.status(401).json({ error: 'Invalid token' });
  }
  if (error?.name === 'TokenExpiredError') {
    return res.status(401).json({ error: 'Token expired' });
  }
  return res.status(500).json({ error: 'Authentication failed' });
}

/** Reject session JWTs issued before the user's last password change. */
export async function assertPasswordStillValid(decoded: JwtPayload): Promise<string | null> {
  if (!decoded.userId || decoded.userId === 'internal') return null;
  try {
    const user = await prisma.user.findUnique({
      where: { id: decoded.userId },
      select: { passwordChangedAt: true },
    });
    if (!user) return 'User not found';
    const expectedPwdv = user.passwordChangedAt?.getTime() ?? 0;
    if (typeof decoded.pwdv !== 'number' || decoded.pwdv !== expectedPwdv) {
      return 'Session expired. Please log in again.';
    }
  } catch {
    // Fail closed: DB/schema errors must not let revoked sessions through
    return 'Authentication temporarily unavailable';
  }
  return null;
}

/**
 * Default API auth: Authorization Bearer session JWT only.
 * Does NOT accept query tokens (prevents SSE/terminal tokens from authorizing DELETE/reboot/etc).
 */
export const authMiddleware = async (req: AuthenticatedRequest, res: Response, next: NextFunction) => {
  try {
    // Allow internal API calls with shared secret (for webhook auto-deploy)
    const internalSecret = req.headers['x-internal-secret'];
    if (internalSecret && internalSecret === INTERNAL_API_SECRET) {
      req.user = { userId: 'internal', email: 'internal@docklift', role: 'admin' };
      return next();
    }

    const authHeader = req.headers.authorization;
    if (!authHeader || !authHeader.startsWith('Bearer ')) {
      return res.status(401).json({ error: 'Authentication required' });
    }

    const token = authHeader.split(' ')[1];
    if (!token) {
      return res.status(401).json({ error: 'Authentication required' });
    }

    const decoded = jwt.verify(token, JWT_SECRET) as JwtPayload;

    // Short-lived purpose tokens must not authenticate normal API calls
    if (decoded.purpose === 'sse' || decoded.purpose === 'terminal') {
      return res.status(401).json({ error: 'Invalid token for this endpoint' });
    }

    const pwdErr = await assertPasswordStillValid(decoded);
    if (pwdErr) {
      return res.status(401).json({ error: pwdErr });
    }

    req.user = {
      userId: decoded.userId,
      email: decoded.email,
      role: decoded.role,
    };
    next();
  } catch (error: any) {
    return authError(res, error);
  }
};

/**
 * SSE-only auth: query ?token= with purpose === 'sse'.
 * Mount exclusively on log stream routes (EventSource cannot set Authorization headers).
 */
export const sseAuthMiddleware = async (req: AuthenticatedRequest, res: Response, next: NextFunction) => {
  try {
    const queryToken = req.query.token as string | undefined;
    if (!queryToken) {
      return res.status(401).json({ error: 'SSE token required' });
    }

    const decoded = jwt.verify(queryToken, JWT_SECRET) as JwtPayload;
    if (decoded.purpose !== 'sse') {
      return res.status(401).json({ error: 'SSE token required' });
    }

    const pwdErr = await assertPasswordStillValid(decoded);
    if (pwdErr) {
      return res.status(401).json({ error: pwdErr });
    }

    req.user = {
      userId: decoded.userId,
      email: decoded.email,
      role: decoded.role,
      purpose: 'sse',
    };
    next();
  } catch (error: any) {
    return authError(res, error);
  }
};

export default authMiddleware;
