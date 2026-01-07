// Express server entry point - configures middleware, routes, and starts the Docklift backend
import 'dotenv/config';
import express from 'express';
import cors from 'cors';
import rateLimit from 'express-rate-limit';
import fs from 'fs';
import path from 'path';
import { fileURLToPath } from 'url';

import { config } from './lib/config.js';
import { ensureNetwork } from './services/docker.js';
import { syncNginxConfigs } from './services/nginx.js';

import projectsRouter from './routes/projects.js';
import deploymentsRouter from './routes/deployments.js';
import filesRouter from './routes/files.js';
import portsRouter from './routes/ports.js';
import githubRouter from './routes/github.js';
import systemRouter from './routes/system.js';
import domainRouter from './routes/domains.js';
import backupRouter from './routes/backup.js';
import authRouter from './routes/auth.js';
import { authMiddleware } from './lib/authMiddleware.js';

const __filename = fileURLToPath(import.meta.url);
const __dirname = path.dirname(__filename);

const app = express();

// Rate limiting for auth endpoints (prevent brute force)
const authLimiter = rateLimit({
  windowMs: 15 * 60 * 1000, // 15 minutes
  max: 5, // 5 attempts per window (stricter brute force protection)
  message: { error: 'Too many attempts, please try again later' },
  standardHeaders: true,
  legacyHeaders: false,
});

// Security headers middleware
app.use((req, res, next) => {
  res.setHeader('X-Content-Type-Options', 'nosniff');
  res.setHeader('X-Frame-Options', 'SAMEORIGIN');
  res.setHeader('X-XSS-Protection', '1; mode=block');
  res.setHeader('Referrer-Policy', 'strict-origin-when-cross-origin');
  next();
});

// CORS - allow requests from frontend (same origin in production, localhost in dev)
app.use(cors({
  origin: process.env.CORS_ORIGIN || true, // true allows same-origin, set CORS_ORIGIN for specific domain
  credentials: true,
}));

// Middleware
app.use(express.json({ limit: '10mb' })); // Limit body size
app.use(express.urlencoded({ extended: true, limit: '10mb' }));
// Trust only the first proxy (nginx) - prevents IP spoofing for rate limiting
app.set('trust proxy', 1);

// Ensure directories exist
const dataDir = path.resolve('./data');
const deploymentsDir = config.deploymentsPath;

if (!fs.existsSync(dataDir)) {
  fs.mkdirSync(dataDir, { recursive: true });
}
if (!fs.existsSync(deploymentsDir)) {
  fs.mkdirSync(deploymentsDir, { recursive: true });
}

// Health check (public)
app.get('/api/health', (req, res) => {
  res.json({ status: 'ok', timestamp: new Date().toISOString() });
});

// Auth routes (public, rate limited)
app.use('/api/auth', authLimiter, authRouter);

// Protected routes - apply auth middleware
app.use('/api/projects', authMiddleware, projectsRouter);
app.use('/api/deployments', authMiddleware, deploymentsRouter);
app.use('/api/files', authMiddleware, filesRouter);
app.use('/api/ports', authMiddleware, portsRouter);
app.use('/api/github', (req, res, next) => {
  // Allow public access for webhooks and callbacks
  const publicPaths = ['/webhook', '/callback', '/manifest/callback', '/setup'];
  if (publicPaths.some(p => req.path.startsWith(p))) {
    return next();
  }
  return authMiddleware(req, res, next);
}, githubRouter);
app.use('/api/system', authMiddleware, systemRouter);
app.use('/api/domains', authMiddleware, domainRouter);
app.use('/api/backup', async (req, res, next) => {
  // Allow restore-upload without auth if no users exist (fresh install/restore scenario)
  if (req.path === '/restore-upload' && req.method === 'POST') {
    // Apply rate limiting to unauthenticated restore endpoint
    return authLimiter(req, res, async () => {
      try {
        const { PrismaClient } = await import('@prisma/client');
        const prisma = new PrismaClient();
        const userCount = await prisma.user.count();
        await prisma.$disconnect();

        if (userCount === 0) {
          // No users exist - allow restore without auth (fresh install)
          return next();
        }
      } catch (error) {
        // Database doesn't exist or error - allow restore without auth
        return next();
      }
      // Users exist - require auth
      return authMiddleware(req, res, next);
    });
  }
  // All other backup endpoints require auth
  return authMiddleware(req, res, next);
}, backupRouter);

// Error handler
app.use((err: Error, req: express.Request, res: express.Response, next: express.NextFunction) => {
  console.error(err.stack);
  res.status(500).json({ error: 'Something went wrong!' });
});

// Start server
async function main() {
  try {
    // Ensure Docker network exists
    await ensureNetwork();
    console.log(`🐳 Docker network "${config.dockerNetwork}" ready`);
    
    app.listen(config.port, () => {
      console.log(`
╔════════════════════════════════════════════════════════════╗
║                                                            ║
║   🚀 Docklift Backend (Node.js)                           ║
║                                                            ║
║   Server running at: http://localhost:${config.port}                ║
║   Deployments path:  ${config.deploymentsPath}                      
║                                                            ║
╚════════════════════════════════════════════════════════════╝
      `);
      
      // Clean up orphaned Nginx configs on startup
      syncNginxConfigs().catch(console.error);
    });
  } catch (error) {
    console.error('Failed to start server:', error);
    process.exit(1);
  }
}

main();
