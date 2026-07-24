// Express server entry point - configures middleware, routes, and starts the Docklift backend
import './lib/loadEnv.js';
import express from 'express';
import cors from 'cors';
import rateLimit from 'express-rate-limit';
import crypto from 'crypto';
import fs from 'fs';
import path from 'path';
import { fileURLToPath } from 'url';

import { config } from './lib/config.js';
import { ensureNetwork } from './services/docker.js';
import { logBootstrapIfNeeded } from './lib/bootstrap.js';

import projectsRouter from './routes/projects.js';
import deploymentsRouter from './routes/deployments.js';
import filesRouter from './routes/files.js';
import portsRouter from './routes/ports.js';
import githubRouter from './routes/github.js';
import systemRouter from './routes/system.js';
import domainRouter from './routes/domains.js';
import backupRouter from './routes/backup.js';
import logsRouter from './routes/logs.js';
import authRouter from './routes/auth.js';
import { authMiddleware, sseAuthMiddleware } from './lib/authMiddleware.js';
import { setupTerminalWebSocket, cleanupAllSessions } from './services/terminal.js';
import { startCertRenewWatcher } from './services/certs.js';
import { reloadNginx, syncNginxConfigs } from './services/nginx.js';
import prisma from './lib/prisma.js';

const __filename = fileURLToPath(import.meta.url);
const __dirname = path.dirname(__filename);

const app = express();

// Rate limiting for auth endpoints (light protection for self-hosted)
const authLimiter = rateLimit({
  windowMs: 15 * 60 * 1000, // 15 minutes
  max: 100, // 100 attempts per window (flexible for self-hosted, still prevents automated attacks)
  message: { error: 'Too many attempts, please try again later' },
  standardHeaders: true,
  legacyHeaders: false,
  skip: (req) => {
    // Skip rate limiting for status check (doesn't need protection)
    return req.path === '/status';
  },
});

// Security headers middleware
app.use((req, res, next) => {
  res.setHeader('X-Content-Type-Options', 'nosniff');
  res.setHeader('X-Frame-Options', 'SAMEORIGIN');
  res.setHeader('X-XSS-Protection', '1; mode=block');
  res.setHeader('Referrer-Policy', 'strict-origin-when-cross-origin');
  res.setHeader('Permissions-Policy', 'camera=(), microphone=(), geolocation=()');
  // HSTS only when the request itself is HTTPS (terminate TLS at reverse proxy)
  const proto = (req.headers['x-forwarded-proto'] || req.protocol || '').toString();
  if (proto === 'https') {
    res.setHeader('Strict-Transport-Security', 'max-age=31536000; includeSubDomains');
  }
  next();
});

// CORS: explicit allowlist, else same-origin only (scheme + host + port)
function normalizeOrigin(value: string): string | null {
  try {
    const u = new URL(value);
    const port = u.port || (u.protocol === 'https:' ? '443' : u.protocol === 'http:' ? '80' : '');
    return `${u.protocol}//${u.hostname}${port ? `:${port}` : ''}`;
  } catch {
    return null;
  }
}

app.use((req, res, next) => {
  cors({
    origin: (origin, callback) => {
      if (!origin) return callback(null, true);

      if (process.env.CORS_ORIGIN) {
        const allowed = process.env.CORS_ORIGIN.split(',').map((s) => s.trim()).filter(Boolean);
        if (allowed.includes(origin)) return callback(null, true);
        return callback(new Error('CORS origin not allowed'));
      }

      try {
        const originNorm = normalizeOrigin(origin);
        if (!originNorm) return callback(new Error('CORS origin not allowed'));

        const proto = (req.headers['x-forwarded-proto'] || req.protocol || 'http')
          .toString()
          .split(',')[0]
          .trim();
        const host = (req.headers['x-forwarded-host'] || req.headers.host || '')
          .toString()
          .split(',')[0]
          .trim();
        if (host) {
          const reqNorm = normalizeOrigin(`${proto}://${host}`);
          if (reqNorm && originNorm === reqNorm) return callback(null, true);
        }
        if (config.frontendUrl) {
          const feNorm = normalizeOrigin(config.frontendUrl);
          if (feNorm && originNorm === feNorm) return callback(null, true);
        }
      } catch {
        /* fall through */
      }
      return callback(new Error('CORS origin not allowed'));
    },
    credentials: true,
  })(req, res, next);
});

// Middleware
// SECURITY: Capture raw body for webhook signature verification (HMAC needs original bytes)
app.use(express.json({
  limit: '10mb',
  verify: (req: any, _res, buf) => {
    req.rawBody = buf;
  },
}));
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

// Generate a unique ID for this server instance at startup
const INSTANCE_ID = crypto.randomUUID();

// Health check (public)
app.get('/api/health', (req, res) => {
  let version = 'unknown';
  try {
    const pkgPath = path.resolve(__dirname, '..', 'package.json');
    const pkg = JSON.parse(fs.readFileSync(pkgPath, 'utf8'));
    version = pkg.version;
  } catch (e) {
    // Try one level up (for dist structure)
    try {
      const pkgPathDist = path.resolve(__dirname, '..', '..', 'package.json');
      const pkg = JSON.parse(fs.readFileSync(pkgPathDist, 'utf8'));
      version = pkg.version;
    } catch { /* ignore */ }
  }
  res.json({ 
    status: 'ok', 
    timestamp: new Date().toISOString(), 
    version,
    instanceId: INSTANCE_ID 
  });
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
app.use('/api/system', (req, res, next) => {
  // SSE log streams use short-lived query tokens; all other system APIs require Bearer session JWT
  if (req.method === 'GET' && req.path.startsWith('/logs/')) {
    return sseAuthMiddleware(req, res, next);
  }
  return authMiddleware(req, res, next);
}, systemRouter);
app.use('/api/domains', authMiddleware, domainRouter);
app.use('/api/logs', (req, res, next) => {
  // Only container log SSE uses query SSE tokens; container list stays Bearer-only
  if (req.method === 'GET' && /\/stream\//.test(req.path)) {
    return sseAuthMiddleware(req, res, next);
  }
  return authMiddleware(req, res, next);
}, logsRouter);
app.use('/api/backup', async (req, res, next) => {
  // Allow restore-upload without auth ONLY with valid setup token (for fresh install/restore)
  if (req.path === '/restore-upload' && req.method === 'POST') {
    // Check for setup token in header
    const setupToken = req.headers['x-setup-token'] as string | undefined;
    if (setupToken) {
      const tokenPath = path.join(dataDir, '.setup-token');
      try {
        if (fs.existsSync(tokenPath)) {
          const storedToken = fs.readFileSync(tokenPath, 'utf8').trim();
          if (setupToken === storedToken && storedToken.length > 0) {
            // Valid setup token - delete after use (one-time only)
            try { fs.unlinkSync(tokenPath); } catch {}
            console.log('[SECURITY] Restore-upload authorized via setup token (token consumed)');
            return next();
          }
        }
      } catch {
        // Token file read error - fall through to normal auth
      }
    }
    // No valid setup token = require normal JWT auth
    return authMiddleware(req, res, next);
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
    // Ensure Docker network exists (non-fatal so auth/API can run without Docker for local smoke)
    try {
      await ensureNetwork();
      console.log(`🐳 Docker network "${config.dockerNetwork}" ready`);
    } catch (dockerErr: any) {
      console.warn(`⚠️  Docker unavailable (${dockerErr?.code || dockerErr?.message || 'error'}) — API starting without Docker`);
    }
    
    const server = app.listen(config.port, () => {
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

    // Attach WebSocket terminal server to HTTP server
    setupTerminalWebSocket(server);

    // Fresh install: print bootstrap secret to logs (never via public API)
    await logBootstrapIfNeeded();

    // After certbot renew updates PEMs, reload nginx-proxy
    startCertRenewWatcher(() => reloadNginx());

    // Graceful shutdown: close connections cleanly on SIGTERM/SIGINT
    const shutdown = async (signal: string) => {
      console.log(`\n🛑 ${signal} received — shutting down gracefully...`);
      server.close(() => console.log('   HTTP server closed'));
      cleanupAllSessions();
      console.log('   Terminal sessions cleaned up');
      await prisma.$disconnect();
      console.log('   Database disconnected');
      process.exit(0);
    };
    process.on('SIGTERM', () => shutdown('SIGTERM'));
    process.on('SIGINT', () => shutdown('SIGINT'));
  } catch (error) {
    console.error('Failed to start server:', error);
    process.exit(1);
  }
}

main();
