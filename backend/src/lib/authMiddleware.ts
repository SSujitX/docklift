// Auth middleware - verifies JWT token and attaches user to request
import { Request, Response, NextFunction } from 'express';
import jwt from 'jsonwebtoken';
import crypto from 'crypto';
import fs from 'fs';
import path from 'path';
import { config } from './config.js';

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
  };
}

// Export for use in auth.ts and github.ts
export { JWT_SECRET, INTERNAL_API_SECRET };

export const authMiddleware = (req: AuthenticatedRequest, res: Response, next: NextFunction) => {
  try {
    // Allow internal API calls with shared secret (for webhook auto-deploy)
    const internalSecret = req.headers['x-internal-secret'];
    if (internalSecret && internalSecret === INTERNAL_API_SECRET) {
      req.user = { userId: 'internal', email: 'internal@docklift', role: 'admin' };
      return next();
    }

    const authHeader = req.headers.authorization;
    const queryToken = req.query.token as string | undefined;

    let token: string | undefined;

    if (authHeader && authHeader.startsWith('Bearer ')) {
      token = authHeader.split(' ')[1];
    } else if (queryToken) {
      // Allow token in query params for download endpoints
      token = queryToken;
    }

    if (!token) {
      return res.status(401).json({ error: 'Authentication required' });
    }
    const decoded = jwt.verify(token, JWT_SECRET) as { userId: string; email: string; role: string };

    req.user = decoded;
    next();
  } catch (error: any) {
    if (error.name === 'JsonWebTokenError') {
      return res.status(401).json({ error: 'Invalid token' });
    }
    if (error.name === 'TokenExpiredError') {
      return res.status(401).json({ error: 'Token expired' });
    }
    return res.status(500).json({ error: 'Authentication failed' });
  }
};

export default authMiddleware;
