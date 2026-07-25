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
import { isTrustedOrigin } from './lib/originCheck.js';
import { isMaintenanceMode, maintenanceReason } from './lib/maintenance.js';
import {
  isRestoreCritical,
  loadRestoreCriticalOnBoot,
  restoreCriticalMarkerPath,
} from './lib/restoreCritical.js';
import { dedupeEnvVariables } from './lib/envVariables.js';

import projectsRouter from './routes/projects.js';
import deploymentsRouter from './routes/deployments.js';
import filesRouter from './routes/files.js';
import portsRouter from './routes/ports.js';
import githubRouter from './routes/github.js';
import systemRouter from './routes/system.js';
import domainRouter from './routes/domains.js';
import backupRouter from './routes/backup.js';
import logsRouter from './routes/logs.js';
import databasesRouter from './routes/databases.js';
import authRouter from './routes/auth.js';
import { authMiddleware, sseAuthMiddleware } from './lib/authMiddleware.js';
import { setupTerminalWebSocket, cleanupAllSessions } from './services/terminal.js';
import { startCertRenewWatcher } from './services/certs.js';
import { reloadNginx, syncNginxConfigs } from './services/nginx.js';
import prisma from './lib/prisma.js';
import { recoverDeploymentStateOnBoot } from './lib/deploymentRecovery.js';

const __filename = fileURLToPath(import.meta.url);
const __dirname = path.dirname(__filename);

const app = express();

// General auth limiter (login, password change, etc.)
const authLimiter = rateLimit({
  windowMs: 15 * 60 * 1000,
  max: 100,
  message: { error: 'Too many attempts, please try again later' },
  standardHeaders: true,
  legacyHeaders: false,
  skip: (req) => req.path === '/status',
});

// Stricter limiter for first-account / bootstrap claim surface (public panel by design)
const setupLimiter = rateLimit({
  windowMs: 15 * 60 * 1000,
  max: 10,
  message: { error: 'Too many setup attempts, please try again later' },
  standardHeaders: true,
  legacyHeaders: false,
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

// CORS: explicit allowlist, else same origin (scheme + host + port) as this request
app.use((req, res, next) => {
  cors({
    origin: (origin, callback) => {
      // Non-browser clients omit Origin; the JWT is still the gate
      if (!origin) return callback(null, true);
      const allowed = isTrustedOrigin(origin, req.headers, {
        fallbackProto: req.protocol,
        allow: [config.frontendUrl],
      });
      return callback(null, allowed);
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

// Ensure directories exist — always use configured DATA_PATH (not a hardcoded ./data)
const dataDir = config.dataPath;
const deploymentsDir = config.deploymentsPath;

if (!fs.existsSync(dataDir)) {
  fs.mkdirSync(dataDir, { recursive: true });
}
if (!fs.existsSync(deploymentsDir)) {
  fs.mkdirSync(deploymentsDir, { recursive: true });
}
if (!fs.existsSync(config.backupPath)) {
  fs.mkdirSync(config.backupPath, { recursive: true });
}

// Generate a unique ID for this server instance at startup
const INSTANCE_ID = crypto.randomUUID();

// During restore / critical recovery: gate API traffic
app.use((req, res, next) => {
  if (isRestoreCritical()) {
    if (req.path === '/api/health') return next();
    // Explicit operator recovery only — never another normal restore
    if (
      req.path === '/api/backup/clear-critical-restore' ||
      req.path === '/api/backup/critical-status'
    ) {
      return next();
    }
    return res.status(503).json({
      error: maintenanceReason(),
      maintenance: true,
      critical: true,
      sealPath: restoreCriticalMarkerPath(),
    });
  }
  if (!isMaintenanceMode()) return next();
  if (req.path === '/api/health') return next();
  // Active restore stream only (blocked when critical above)
  if (req.path.startsWith('/api/backup/restore')) return next();
  return res.status(503).json({ error: maintenanceReason(), maintenance: true });
});

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
app.use('/api/auth/register', setupLimiter);
app.use('/api/auth/setup-token', setupLimiter);
app.use('/api/auth', authLimiter, authRouter);

// Protected routes - apply auth middleware
app.use('/api/projects', authMiddleware, projectsRouter);
app.use('/api/deployments', authMiddleware, deploymentsRouter);
app.use('/api/databases', authMiddleware, databasesRouter);
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
  // Fresh-install restore: validate setup token but do NOT consume yet.
  // Token + bootstrap secret are consumed only after a successful restore
  // (see backup.ts → consumeSetupRestoreSecrets).
  if (req.path === '/restore-upload' && req.method === 'POST') {
    const setupToken = req.headers['x-setup-token'] as string | undefined;
    const { validateSetupToken } = await import('./lib/setupRestoreAuth.js');
    if (validateSetupToken(setupToken)) {
      (req as import('./lib/setupRestoreAuth.js').SetupRestoreRequest).setupTokenAuth = true;
      console.log('[SECURITY] Restore-upload authorized via setup token (deferred consume)');
      return next();
    }
    return authMiddleware(req, res, next);
  }
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
    // Re-seal critical restore state from disk before accepting traffic
    loadRestoreCriticalOnBoot();

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

    await recoverDeploymentStateOnBoot();

    try {
      const removed = await dedupeEnvVariables();
      if (removed > 0) {
        console.log(`[startup] Removed ${removed} duplicate env_variables row(s)`);
      }
    } catch (envErr) {
      console.warn('[startup] Env dedupe skipped:', envErr);
    }

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
