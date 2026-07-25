// Backup routes - API endpoints for backup and restore operations
import { Router, Request, Response } from 'express';
import { exec, spawnSync } from 'child_process';
import { promisify } from 'util';
import fs from 'fs';
import fsp from 'fs/promises';
import path from 'path';
import archiver from 'archiver';
import multer from 'multer';
import { config } from '../lib/config.js';
import prisma, { reconnectPrisma } from '../lib/prisma.js';
import { composeProjectName } from '../lib/naming.js';
import { safeExtractZip } from '../lib/safeUnzip.js';
import { replaceDirContents } from '../lib/fsCopy.js';
import { enterMaintenance, exitMaintenance } from '../lib/maintenance.js';
import {
  tryAcquireRestoreLock,
  releaseRestoreLock,
  restoreLockReason,
} from '../lib/restoreLock.js';
import { requireStepUpPassword } from '../lib/stepUpAuth.js';
import {
  consumeSetupRestoreSecrets,
  type SetupRestoreRequest,
} from '../lib/setupRestoreAuth.js';
import { decideRestoreCommit } from '../lib/restoreCommitPolicy.js';
import {
  clearRestoreCritical,
  enterRestoreCritical,
  isRestoreCritical,
  readRestoreCritical,
} from '../lib/restoreCritical.js';
import crypto from 'crypto';

const execAsync = promisify(exec);
const router = Router();

/**
 * Consistent SQLite snapshot via VACUUM INTO — never archive the live DB file
 * while writers are active (raw copy can mix pages across transactions).
 */
async function snapshotDatabase(liveDbPath: string): Promise<{ snapshotPath: string; cleanup: () => Promise<void> }> {
  const snapshotPath = path.join(
    path.dirname(liveDbPath),
    `.docklift-backup-${Date.now()}-${process.pid}.db`
  );
  await fsp.rm(snapshotPath, { force: true });
  // Absolute path, forward slashes for SQLite on Windows
  const sqlPath = snapshotPath.replace(/\\/g, '/').replace(/'/g, "''");
  await prisma.$executeRawUnsafe(`VACUUM INTO '${sqlPath}'`);
  return {
    snapshotPath,
    cleanup: async () => {
      try {
        await fsp.rm(snapshotPath, { force: true });
      } catch {
        /* ignore */
      }
    },
  };
}

async function restoreDatabaseFile(tempDbPath: string, currentDbPath: string, writeLog: (t: string) => void) {
  // Must drop open handles before replacing the SQLite file on disk.
  await prisma.$disconnect();
  const backupDbPath = `${currentDbPath}.pre-restore`;
  try {
    if (fs.existsSync(currentDbPath)) {
      await fsp.copyFile(currentDbPath, backupDbPath);
      writeLog(`      + Created backup of current database\n`);
    }
    await fsp.mkdir(path.dirname(currentDbPath), { recursive: true });
    await fsp.copyFile(tempDbPath, currentDbPath);
    writeLog(`      + Database restored\n`);
  } catch (err) {
    // Roll back DB file if the replace itself failed mid-way
    if (fs.existsSync(backupDbPath)) {
      try {
        await fsp.copyFile(backupDbPath, currentDbPath);
        writeLog(`      ! Database replace failed — rolled back to pre-restore copy\n`);
      } catch {
        writeLog(`      ! CRITICAL: database replace failed and rollback also failed\n`);
      }
    }
    await reconnectPrisma().catch(() => {});
    throw err;
  }
  await reconnectPrisma();
}

/**
 * Restore DB from `.pre-restore`. Returns true on success.
 * On failure: does NOT claim success — caller must enter critical recovery.
 */
async function rollbackDatabaseFromPreRestore(
  currentDbPath: string,
  writeLog: (t: string) => void
): Promise<boolean> {
  const backupDbPath = `${currentDbPath}.pre-restore`;
  if (!fs.existsSync(backupDbPath)) {
    writeLog(`      ! No pre-restore database at ${backupDbPath}\n`);
    return false;
  }
  try {
    await prisma.$disconnect();
    await fsp.copyFile(backupDbPath, currentDbPath);
    await reconnectPrisma();
    writeLog(`      + Rolled database back from .pre-restore\n`);
    writeLog(`      + Live DB: ${currentDbPath}\n`);
    writeLog(`      + Pre-restore copy retained: ${backupDbPath}\n`);
    return true;
  } catch (err: any) {
    writeLog(`      ! DB rollback FAILED: ${err?.message || err}\n`);
    writeLog(`      ! CRITICAL: preserve both files for manual recovery:\n`);
    writeLog(`          live: ${currentDbPath}\n`);
    writeLog(`          pre-restore: ${backupDbPath}\n`);
    await reconnectPrisma().catch(() => {});
    return false;
  }
}

export type RestoreCommitOutcome = 'committed' | 'aborted' | 'critical';

/**
 * After files + reconcile: commit (optionally consume setup secrets) or abort.
 * Setup-token restores roll the DB back when not fully committed so retry stays possible.
 * If rollback itself fails → `critical` (maintenance stays on; not retryable via setup UI).
 */
async function finishRestoreCommit(opts: {
  writeLog: (t: string) => void;
  reconcileOk: boolean;
  dbReplaced: boolean;
  currentDbPath: string;
  setupAuth: boolean;
}): Promise<RestoreCommitOutcome> {
  await reconnectPrisma().catch(() => {});
  const adminCount = await prisma.user
    .count({ where: { role: 'admin' } })
    .catch(() => 0);
  const decision = decideRestoreCommit({
    reconcileOk: opts.reconcileOk,
    adminCount,
    dbReplaced: opts.dbReplaced,
    setupAuth: opts.setupAuth,
  });

  if (decision.action === 'abort') {
    opts.writeLog(`\n[ROLLBACK] Restore not committed: ${decision.reason}\n`);
    if (decision.rollbackDb) {
      const rolled = await rollbackDatabaseFromPreRestore(opts.currentDbPath, opts.writeLog);
      if (!rolled) {
        opts.writeLog(
          `\n[CRITICAL] Database rollback failed — sealed; do not retry until files are repaired.\n`
        );
        opts.writeLog(
          `  Manual recovery paths:\n    ${opts.currentDbPath}\n    ${opts.currentDbPath}.pre-restore\n`
        );
        enterRestoreCritical({
          detail: decision.reason,
          liveDbPath: opts.currentDbPath,
        });
        return 'critical';
      }
    }
    if (opts.setupAuth) {
      opts.writeLog(`  [security] Setup credentials retained for retry\n`);
    }
    return 'aborted';
  }

  if (decision.consumeSetup) {
    consumeSetupRestoreSecrets();
    opts.writeLog(`\n  [security] Setup token + bootstrap secret consumed\n`);
  }
  return 'committed';
}

// Helper: Reconcile system state after restore
// 1. Redeploy all active projects
// 2. Reload Nginx
// 3. Restart Backend
/** Returns true when every attempted redeploy + nginx reload succeeded. */
async function reconcileSystem(writeLog: (text: string) => void): Promise<boolean> {
  writeLog(`\n${'='.repeat(50)}\n`);
  writeLog(`  RECONCILING SYSTEM STATE\n`);
  writeLog(`${'='.repeat(50)}\n\n`);

  let failures = 0;

  try {
    // Fresh client after DB file replacement
    await reconnectPrisma();
    writeLog(`[1/3] Reading restored database...\n`);

    const projects = await prisma.project.findMany();
    writeLog(`      + Found ${projects.length} projects in database\n`);

    // 3. Loop and redeploy
    writeLog(`\n[2/3] Auto-redeploying projects...\n`);
    for (const project of projects) {
      // Projects are stored at deployments/<id>/ (not deployments/<id>/source/)
      const projectPath = path.join(config.deploymentsPath, project.id);
      
      if (fs.existsSync(projectPath)) {
        writeLog(`      > Redeploying ${project.name} (${project.id})...\n`);
        try {
          const runtimeCompose = path.join(
            config.deploymentsPath,
            '.docklift',
            project.id,
            'compose.yml'
          );
          // Current backups include DockLift's generated runtime state. Older backups
          // fall back to their source-root compose file.
          const composeFile = fs.existsSync(runtimeCompose)
            ? runtimeCompose
            : path.join(projectPath, 'docker-compose.yml');
          if (fs.existsSync(composeFile)) {
             // -p must match deploy naming (dl-<slug>-<shortId>) so images/containers stay consistent
             const composeProject = composeProjectName(project.name, project.id);
             const volumes = await prisma.persistentVolume.findMany({
               where: { project_id: project.id },
             });
             for (const volume of volumes) {
               spawnSync('docker', ['volume', 'create', volume.name], {
                 shell: false,
                 stdio: 'ignore',
                 timeout: 30000,
               });
             }
             await execAsync(`docker compose -f "${composeFile}" -p ${composeProject} up -d`, {
               cwd: projectPath,
               env: { ...process.env, DOCKER_BUILDKIT: '1', COMPOSE_DOCKER_CLI_BUILD: '1' },
               maxBuffer: 50 * 1024 * 1024, // 50MB buffer for verbose build output
               timeout: 5 * 60 * 1000, // 5 minute timeout per project
             });
             writeLog(`        + Success (${composeProject})\n`);
          } else {
             writeLog(`        ! Skipped (No DockLift runtime compose; deploy manually)\n`);
          }
        } catch (e: any) {
             failures += 1;
             writeLog(`        [ERROR] Failed: ${e.message.split('\n')[0]}\n`);
        }
      } else {
        writeLog(`      - Skipped ${project.name} (No source code found)\n`);
      }
    }

    // 4. Reload Nginx Proxy
    writeLog(`\n[3/3] Reloading Nginx Proxy...\n`);
    try {
      await execAsync('docker exec docklift-nginx-proxy nginx -s reload');
      writeLog(`      + Nginx configuration reloaded\n`);
    } catch (e: any) {
      failures += 1;
      writeLog(`      [ERROR] Nginx reload failed: ${e.message.split('\n')[0]}\n`);
    }

    if (failures > 0) {
      writeLog(`\n[ERROR] Reconcile finished with ${failures} failure(s) — restore is incomplete\n`);
      return false;
    }
    return true;
  } catch (error: any) {
    writeLog(`\n[ERROR] Reconcile failed: ${error.message}\n`);
    console.error('Reconcile error:', error);
    return false;
  }
}


// Configure multer for backup uploads (saves to uploads subdirectory)
const uploadStorage = multer.diskStorage({
  destination: async (req, file, cb) => {
    const uploadDir = path.join(config.backupPath, 'uploads');
    await fsp.mkdir(uploadDir, { recursive: true });
    cb(null, uploadDir);
  },
  filename: (req, file, cb) => {
    // Server-chosen name — never trust client filename for storage path
    const ext = path.extname(file.originalname).toLowerCase() === '.zip' ? '.zip' : '.zip';
    cb(null, `restore-${Date.now()}-${crypto.randomBytes(8).toString('hex')}${ext}`);
  }
});

/** Acquire restore lock before Multer writes any bytes. */
function requireRestoreLock(why: string) {
  return (req: Request, res: Response, next: () => void) => {
    if (!tryAcquireRestoreLock(why)) {
      return res.status(409).json({ error: restoreLockReason(), locked: true });
    }
    next();
  };
}

const uploadBackup = multer({
  storage: uploadStorage,
  limits: { fileSize: 500 * 1024 * 1024 }, // 500MB — aligned with nginx client_max_body_size
  fileFilter: (req, file, cb) => {
    if (file.mimetype === 'application/zip' || file.originalname.endsWith('.zip')) {
      cb(null, true);
    } else {
      cb(new Error('Only ZIP files are allowed'));
    }
  }
});

// Configuration paths
const BACKUP_DIR = config.backupPath;
const UPLOADS_DIR = path.join(BACKUP_DIR, 'uploads'); // Separate directory for uploaded restore files
const DEPLOYMENTS_PATH = config.deploymentsPath;
const NGINX_CONF_PATH = config.nginxConfPath;
const LETSENCRYPT_PATH = config.letsencryptPath;
const GITHUB_KEY_PATH = config.githubPrivateKeyPath;

// Helper: Resolve database path (handles both relative and absolute paths)
function getDatabasePath(): string {
  const dbUrl = process.env.DATABASE_URL || 'file:/app/data/docklift.db';
  let dbPath = dbUrl.replace('file:', '');

  // If it's a relative path, resolve it from the backend directory
  if (dbPath.startsWith('./') || dbPath.startsWith('../') || !path.isAbsolute(dbPath)) {
    // Try multiple possible locations
    const possiblePaths = [
      path.resolve(process.cwd(), dbPath),
      path.resolve(process.cwd(), 'prisma', 'data', 'docklift.db'),
      path.resolve(process.cwd(), 'data', 'docklift.db'),
      path.resolve(__dirname, '..', '..', dbPath),
      path.resolve(__dirname, '..', '..', 'prisma', 'data', 'docklift.db'),
      path.resolve(__dirname, '..', '..', 'data', 'docklift.db'),
      '/app/data/docklift.db', // Docker production path
    ];

    for (const p of possiblePaths) {
      if (fs.existsSync(p)) {
        return p;
      }
    }
  }

  return dbPath;
}

// Get __dirname for ES modules
import { fileURLToPath } from 'url';
import { dirname } from 'path';
const __filename = fileURLToPath(import.meta.url);
const __dirname = dirname(__filename);

// Helper: Format bytes to human readable
function formatBytes(bytes: number): string {
  if (bytes === 0) return '0 B';
  const k = 1024;
  const sizes = ['B', 'KB', 'MB', 'GB', 'TB'];
  const i = Math.floor(Math.log(bytes) / Math.log(k));
  return parseFloat((bytes / Math.pow(k, i)).toFixed(2)) + ' ' + sizes[i];
}

// Helper: Validate filename for security (prevent path traversal)
function isValidBackupFilename(filename: string): boolean {
  // Only allow alphanumeric, dots, hyphens, underscores
  // Must end with .zip and not contain path traversal
  return /^[a-zA-Z0-9._-]+\.zip$/.test(filename) && !filename.includes('..');
}

// Helper: Get directory size recursively
async function getDirectorySize(dirPath: string): Promise<number> {
  try {
    const { stdout } = await execAsync(`du -sb "${dirPath}" 2>/dev/null || echo "0"`);
    return parseInt(stdout.split('\t')[0]) || 0;
  } catch {
    return 0;
  }
}

// GET /api/backup - List server-created backups only (excludes uploads directory)
router.get('/', async (req: Request, res: Response) => {
  try {
    // Ensure backup directory exists
    await fsp.mkdir(BACKUP_DIR, { recursive: true });

    const files = await fsp.readdir(BACKUP_DIR);
    const backups = [];

    for (const file of files) {
      // Skip the uploads subdirectory and only include .zip files
      if (file === 'uploads') continue;
      if (file.endsWith('.zip')) {
        try {
          const stats = await fsp.stat(path.join(BACKUP_DIR, file));
          backups.push({
            filename: file,
            size: stats.size,
            created_at: stats.mtime.toISOString(),
          });
        } catch {
          // Skip files that can't be read
        }
      }
    }

    // Sort by date descending (newest first)
    backups.sort((a, b) => new Date(b.created_at).getTime() - new Date(a.created_at).getTime());

    res.json(backups);
  } catch (error: any) {
    console.error('List backups error:', error);
    res.status(500).json({ error: 'Failed to list backups' });
  }
});

// GET /api/backup/uploads - List uploaded restore files
router.get('/uploads', async (req: Request, res: Response) => {
  try {
    await fsp.mkdir(UPLOADS_DIR, { recursive: true });

    const files = await fsp.readdir(UPLOADS_DIR);
    const uploads = [];

    for (const file of files) {
      if (file.endsWith('.zip')) {
        try {
          const stats = await fsp.stat(path.join(UPLOADS_DIR, file));
          uploads.push({
            filename: file,
            size: stats.size,
            created_at: stats.mtime.toISOString(),
          });
        } catch {
          // Skip files that can't be read
        }
      }
    }

    // Sort by date descending (newest first)
    uploads.sort((a, b) => new Date(b.created_at).getTime() - new Date(a.created_at).getTime());

    res.json(uploads);
  } catch (error: any) {
    console.error('List uploads error:', error);
    res.status(500).json({ error: 'Failed to list uploaded files' });
  }
});

// DELETE /api/backup/uploads/:filename - Delete an uploaded restore file
router.delete('/uploads/:filename', async (req: Request, res: Response) => {
  const { filename } = req.params;

  if (!isValidBackupFilename(filename)) {
    return res.status(400).json({ error: 'Invalid filename' });
  }

  const filePath = path.join(UPLOADS_DIR, filename);

  try {
    await fsp.access(filePath);
    await fsp.unlink(filePath);
    res.json({ success: true, message: 'File deleted successfully' });
  } catch (error: any) {
    if (error.code === 'ENOENT') {
      return res.status(404).json({ error: 'File not found' });
    }
    console.error('Delete upload error:', error);
    res.status(500).json({ error: 'Failed to delete file' });
  }
});

// POST /api/backup/create - Create new backup with streaming progress
router.post('/create', async (req: Request, res: Response) => {
  // Get optional custom name from request body
  const { name } = req.body || {};

  // Set streaming headers
  res.setHeader('Content-Type', 'text/plain; charset=utf-8');
  res.setHeader('X-Content-Type-Options', 'nosniff');
  res.setHeader('Cache-Control', 'no-cache, no-transform');
  res.setHeader('Connection', 'keep-alive');
  res.setHeader('Transfer-Encoding', 'chunked');

  const writeLog = (text: string) => {
    try {
      res.write(text);
    } catch {
      // Ignore write errors if client disconnected
    }
  };

  try {
    // Ensure backup directory exists
    await fsp.mkdir(BACKUP_DIR, { recursive: true });

    // Generate filename with timestamp
    const now = new Date();
    const timestamp = now.toISOString()
      .replace(/[:.]/g, '-')
      .replace('T', '_')
      .slice(0, 19);

    // Use custom name if provided, otherwise default to "docklift"
    const sanitizedName = name ? name.replace(/[^a-zA-Z0-9_-]/g, '-').slice(0, 50) : 'docklift';
    const backupFilename = `${sanitizedName}-backup-${timestamp}.zip`;
    const backupPath = path.join(BACKUP_DIR, backupFilename);

    writeLog(`\n${'='.repeat(50)}\n`);
    writeLog(`  CREATING BACKUP\n`);
    writeLog(`  ${now.toISOString()}\n`);
    writeLog(`${'='.repeat(50)}\n\n`);

    // Create archive stream
    const output = fs.createWriteStream(backupPath);
    const archive = archiver('zip', { zlib: { level: 9 } });

    // Handle archive errors
    archive.on('error', (err) => {
      writeLog(`\n[ERROR] Archive error: ${err.message}\n`);
    });

    archive.on('warning', (err) => {
      if (err.code !== 'ENOENT') {
        writeLog(`[WARN] ${err.message}\n`);
      }
    });

    archive.pipe(output);

    // 1. Add database (consistent snapshot — never the live file)
    writeLog(`[1/4] Adding database...\n`);
    const dbPath = getDatabasePath();
    let snapshotCleanup: (() => Promise<void>) | null = null;
    if (fs.existsSync(dbPath)) {
      const { snapshotPath, cleanup } = await snapshotDatabase(dbPath);
      snapshotCleanup = cleanup;
      const dbStats = await fsp.stat(snapshotPath);
      archive.file(snapshotPath, { name: 'database/docklift.db' });
      writeLog(`      + docklift.db (${formatBytes(dbStats.size)}) [consistent snapshot]\n`);
      archive.on('end', () => {
        void cleanup();
      });
      archive.on('error', () => {
        void cleanup();
      });
    } else {
      writeLog(`\n[ERROR] Database not found at ${dbPath} — refusing incomplete backup\n`);
      if (fs.existsSync(backupPath)) {
        await fsp.rm(backupPath, { force: true }).catch(() => {});
      }
      res.end();
      return;
    }

    // 2. Add deployments
    writeLog(`\n[2/4] Adding deployments...\n`);
    if (fs.existsSync(DEPLOYMENTS_PATH)) {
      const deploymentsSize = await getDirectorySize(DEPLOYMENTS_PATH);
      archive.directory(DEPLOYMENTS_PATH, 'deployments');
      writeLog(`      + /deployments/ (${formatBytes(deploymentsSize)})\n`);
    } else {
      writeLog(`      ! Deployments directory not found\n`);
    }

    // 3. Add nginx configs
    writeLog(`\n[3/5] Adding nginx configs...\n`);
    if (fs.existsSync(NGINX_CONF_PATH)) {
      const nginxSize = await getDirectorySize(NGINX_CONF_PATH);
      archive.directory(NGINX_CONF_PATH, 'nginx-conf');
      writeLog(`      + /nginx-conf/ (${formatBytes(nginxSize)})\n`);
    } else {
      writeLog(`      - Nginx config directory not found (skipped)\n`);
    }

    // 4. Add Let's Encrypt certificates
    writeLog(`\n[4/5] Adding TLS certificates...\n`);
    if (fs.existsSync(LETSENCRYPT_PATH)) {
      const leSize = await getDirectorySize(LETSENCRYPT_PATH);
      archive.directory(LETSENCRYPT_PATH, 'letsencrypt');
      writeLog(`      + /letsencrypt/ (${formatBytes(leSize)})\n`);
    } else {
      writeLog(`      - No Let's Encrypt data (skipped)\n`);
    }

    // 5. Add GitHub key if exists
    writeLog(`\n[5/5] Adding GitHub App key...\n`);
    if (fs.existsSync(GITHUB_KEY_PATH)) {
      archive.file(GITHUB_KEY_PATH, { name: 'github-app.pem' });
      writeLog(`      + github-app.pem\n`);
    } else {
      writeLog(`      - No GitHub App key configured (skipped)\n`);
    }

    // Finalize archive
    writeLog(`\nFinalizing backup...\n`);
    await archive.finalize();

    // Wait for output to finish writing
    await new Promise<void>((resolve, reject) => {
      output.on('close', resolve);
      output.on('error', reject);
    });

    // Get final backup size
    const finalStats = await fsp.stat(backupPath);

    writeLog(`\n${'='.repeat(50)}\n`);
    writeLog(`  BACKUP COMPLETE\n`);
    writeLog(`${'='.repeat(50)}\n`);
    writeLog(`  File: ${backupFilename}\n`);
    writeLog(`  Size: ${formatBytes(finalStats.size)}\n`);
    writeLog(`  Location: ${BACKUP_DIR}\n`);

    if (snapshotCleanup) await snapshotCleanup();
    res.end();
  } catch (error: any) {
    writeLog(`\n[ERROR] Backup failed: ${error.message}\n`);
    console.error('Backup creation error:', error);
    res.end();
  }
});

// POST /api/backup/restore/:filename - Restore from backup with streaming progress
router.post('/restore/:filename', requireRestoreLock('restore-from-backup'), async (req: Request, res: Response) => {
  if (!(await requireStepUpPassword(req, res))) {
    releaseRestoreLock();
    return;
  }

  const { filename } = req.params;

  // Security validation
  if (!isValidBackupFilename(filename)) {
    releaseRestoreLock();
    return res.status(400).json({ error: 'Invalid backup filename' });
  }

  const backupPath = path.join(BACKUP_DIR, filename);

  if (!fs.existsSync(backupPath)) {
    releaseRestoreLock();
    return res.status(404).json({ error: 'Backup file not found' });
  }

  enterMaintenance('Restoring from backup');

  // Set streaming headers
  res.setHeader('Content-Type', 'text/plain; charset=utf-8');
  res.setHeader('X-Content-Type-Options', 'nosniff');
  res.setHeader('Cache-Control', 'no-cache, no-transform');
  res.setHeader('Connection', 'keep-alive');
  res.setHeader('Transfer-Encoding', 'chunked');

  const writeLog = (text: string) => {
    try {
      res.write(text);
    } catch {
      // Ignore write errors if client disconnected
    }
  };

  let dbReplaced = false;
  const currentDbPath = getDatabasePath();

  try {
    const now = new Date();
    writeLog(`\n${'='.repeat(50)}\n`);
    writeLog(`  RESTORING BACKUP\n`);
    writeLog(`  ${now.toISOString()}\n`);
    writeLog(`${'='.repeat(50)}\n`);
    writeLog(`  Source: ${filename}\n\n`);

    // 1. Stop user containers (not Docklift system containers)
    writeLog(`[1/7] Stopping user containers...\n`);
    try {
      // Get all running containers that start with "dl_" (Docklift deployed projects)
      const { stdout: containers } = await execAsync(
        `docker ps -q --filter "name=dl_" 2>/dev/null || echo ""`
      );
      if (containers.trim()) {
        await execAsync(`docker stop ${containers.trim().split('\n').join(' ')}`);
        writeLog(`      + Stopped user containers\n`);
      } else {
        writeLog(`      - No user containers running\n`);
      }
    } catch (e: any) {
      writeLog(`      ! Container stop warning: ${e.message}\n`);
    }

    // 2. Extract backup to temp directory
    writeLog(`\n[2/7] Extracting backup...\n`);
    const tempDir = path.join(BACKUP_DIR, `temp-restore-${Date.now()}`);
    await fsp.rm(tempDir, { recursive: true, force: true });
    await fsp.mkdir(tempDir, { recursive: true });

    await safeExtractZip(backupPath, tempDir);
    writeLog(`      + Extraction complete\n`);

    // 3. Restore database
    writeLog(`\n[3/7] Restoring database...\n`);
    const tempDbPath = path.join(tempDir, 'database', 'docklift.db');
    if (fs.existsSync(tempDbPath)) {
      await restoreDatabaseFile(tempDbPath, currentDbPath, writeLog);
      dbReplaced = true;
    } else {
      writeLog(`      ! No database in backup\n`);
    }

    // 4. Restore deployments
    writeLog(`\n[4/7] Restoring deployments...\n`);
    const tempDeploymentsPath = path.join(tempDir, 'deployments');
    if (fs.existsSync(tempDeploymentsPath)) {
      await replaceDirContents(tempDeploymentsPath, DEPLOYMENTS_PATH);
      writeLog(`      + Deployments restored\n`);
    } else {
      writeLog(`      ! No deployments in backup\n`);
    }

    // 5. Restore nginx configs
    writeLog(`\n[5/8] Restoring nginx configs...\n`);
    const tempNginxPath = path.join(tempDir, 'nginx-conf');
    if (fs.existsSync(tempNginxPath)) {
      await fsp.mkdir(NGINX_CONF_PATH, { recursive: true });
      await replaceDirContents(tempNginxPath, NGINX_CONF_PATH);
      writeLog(`      + Nginx configs restored\n`);

      // Reload nginx proxy
      try {
        await execAsync('docker exec docklift-nginx-proxy nginx -s reload 2>/dev/null || true');
        writeLog(`      + Nginx proxy reloaded\n`);
      } catch {
        writeLog(`      - Nginx reload skipped\n`);
      }
    } else {
      writeLog(`      - No nginx configs in backup\n`);
    }

    // 6. Restore Let's Encrypt certificates
    writeLog(`\n[6/8] Restoring TLS certificates...\n`);
    const tempLePath = path.join(tempDir, 'letsencrypt');
    if (fs.existsSync(tempLePath)) {
      await fsp.mkdir(LETSENCRYPT_PATH, { recursive: true });
      await replaceDirContents(tempLePath, LETSENCRYPT_PATH);
      writeLog(`      + Let's Encrypt certificates restored\n`);
      try {
        await execAsync('docker exec docklift-nginx-proxy nginx -s reload 2>/dev/null || true');
        writeLog(`      + Nginx proxy reloaded\n`);
      } catch {
        writeLog(`      - Nginx reload skipped\n`);
      }
    } else {
      writeLog(`      - No certificates in backup\n`);
    }

    // 7. Restore GitHub key
    writeLog(`\n[7/8] Restoring GitHub App key...\n`);
    const tempGithubKeyPath = path.join(tempDir, 'github-app.pem');
    if (fs.existsSync(tempGithubKeyPath)) {
      await fsp.copyFile(tempGithubKeyPath, GITHUB_KEY_PATH);
      writeLog(`      + GitHub key restored\n`);
    } else {
      writeLog(`      - No GitHub key in backup\n`);
    }

    // Clean up temp directory
    await fsp.rm(tempDir, { recursive: true, force: true });

    // Keep other rollback archives; only note the archive that was just applied
    writeLog(`\n[8/8] Restore archive retained for rollback...\n`);
    writeLog(`      + Kept: ${filename}\n`);

    // Reconcile system state (auto-redeploy)
    const reconcileOk = await reconcileSystem(writeLog);
    const outcome = await finishRestoreCommit({
      writeLog,
      reconcileOk,
      dbReplaced,
      currentDbPath,
      setupAuth: false,
    });

    writeLog(`\n${'='.repeat(50)}\n`);
    if (outcome === 'critical') {
      writeLog(`  [CRITICAL] RESTORE FAILED — MANUAL RECOVERY REQUIRED\n`);
      writeLog(`  Seal: data/.restore-critical (lock held; restores blocked across restarts)\n`);
      writeLog(`${'='.repeat(50)}\n`);
      // Do NOT releaseRestoreLock — critical seal holds it
      res.end();
      return;
    }
    if (outcome === 'aborted') {
      writeLog(`  [ERROR] RESTORE ABORTED\n`);
      writeLog(`${'='.repeat(50)}\n`);
      exitMaintenance();
      releaseRestoreLock();
      res.end();
      return;
    }
    if (reconcileOk) {
      writeLog(`  RESTORE COMPLETE\n`);
    } else {
      writeLog(`  [ERROR] RESTORE INCOMPLETE — data restored but some projects failed to start\n`);
    }
    writeLog(`${'='.repeat(50)}\n`);
    writeLog(`\n  [!] Restarting backend service to apply changes...\n`);

    res.end();

    // Trigger restart
    setTimeout(() => {
      console.log('Restarting backend service after restore...');
      process.exit(0);
    }, 1000);
  } catch (error: any) {
    if (dbReplaced) {
      writeLog(`\n[ROLLBACK] Attempting database rollback after failed restore...\n`);
      const rolled = await rollbackDatabaseFromPreRestore(currentDbPath, writeLog);
      if (!rolled) {
        writeLog(`\n[CRITICAL] Rollback failed — sealed; do not retry\n`);
        writeLog(`  Paths: ${currentDbPath} | ${currentDbPath}.pre-restore\n`);
        enterRestoreCritical({
          detail: error?.message || 'restore failed after DB replace',
          liveDbPath: currentDbPath,
        });
        res.end();
        return;
      }
    }
    exitMaintenance();
    releaseRestoreLock();
    writeLog(`\n[ERROR] Restore failed: ${error.message}\n`);
    console.error('Restore error:', error);
    res.end();
  }
});

// DELETE /api/backup/:filename - Delete a backup
router.delete('/:filename', async (req: Request, res: Response) => {
  const { filename } = req.params;

  // Security validation
  if (!isValidBackupFilename(filename)) {
    return res.status(400).json({ error: 'Invalid backup filename' });
  }

  const backupPath = path.join(BACKUP_DIR, filename);

  try {
    await fsp.access(backupPath);
    await fsp.unlink(backupPath);
    res.json({ success: true, message: 'Backup deleted successfully' });
  } catch (error: any) {
    if (error.code === 'ENOENT') {
      return res.status(404).json({ error: 'Backup not found' });
    }
    console.error('Delete backup error:', error);
    res.status(500).json({ error: 'Failed to delete backup' });
  }
});

// GET /api/backup/download/:filename - Download a backup file
router.get('/download/:filename', async (req: Request, res: Response) => {
  const { filename } = req.params;

  // Security validation
  if (!isValidBackupFilename(filename)) {
    return res.status(400).json({ error: 'Invalid backup filename' });
  }

  const backupPath = path.join(BACKUP_DIR, filename);

  try {
    const stats = await fsp.stat(backupPath);

    res.setHeader('Content-Type', 'application/zip');
    res.setHeader('Content-Disposition', `attachment; filename="${filename}"`);
    res.setHeader('Content-Length', stats.size);

    const stream = fs.createReadStream(backupPath);
    stream.pipe(res);

    stream.on('error', (err) => {
      console.error('Download stream error:', err);
      if (!res.headersSent) {
        res.status(500).json({ error: 'Failed to download backup' });
      }
    });
  } catch (error: any) {
    if (error.code === 'ENOENT') {
      return res.status(404).json({ error: 'Backup not found' });
    }
    console.error('Download backup error:', error);
    res.status(500).json({ error: 'Failed to download backup' });
  }
});

// POST /api/backup/upload - Upload a backup file (just saves it)
router.post('/upload', uploadBackup.single('backup'), async (req: Request, res: Response) => {
  try {
    if (!req.file) {
      return res.status(400).json({ error: 'No backup file uploaded' });
    }

    res.json({
      success: true,
      filename: req.file.filename,
      size: req.file.size,
      message: 'Backup uploaded successfully'
    });
  } catch (error: any) {
    console.error('Upload backup error:', error);
    res.status(500).json({ error: error.message || 'Failed to upload backup' });
  }
});

// POST /api/backup/restore-upload - Upload and immediately restore from backup
router.post(
  '/restore-upload',
  requireRestoreLock('restore-upload'),
  (req: Request, res: Response, next) => {
    uploadBackup.single('backup')(req, res, (err: unknown) => {
      if (err) {
        releaseRestoreLock();
        const message = err instanceof Error ? err.message : 'Upload failed';
        return res.status(400).json({ error: message });
      }
      next();
    });
  },
  async (req: Request, res: Response) => {
  try {
    const setupAuth = (req as SetupRestoreRequest).setupTokenAuth === true;
    if (setupAuth) {
      // Fresh install: setup token already validated; no admin user / password yet.
      const adminCount = await prisma.user
        .count({ where: { role: 'admin' } })
        .catch(() => 0);
      if (adminCount > 0) {
        releaseRestoreLock();
        if (req.file?.path) await fsp.unlink(req.file.path).catch(() => {});
        return res.status(403).json({
          error: 'Setup-token restore is only allowed before the first admin exists',
        });
      }
    } else if (!(await requireStepUpPassword(req, res))) {
      releaseRestoreLock();
      if (req.file?.path) await fsp.unlink(req.file.path).catch(() => {});
      return;
    }
    if (!req.file) {
      releaseRestoreLock();
      return res.status(400).json({ error: 'No backup file uploaded' });
    }

    enterMaintenance('Restoring from uploaded backup');
    const backupPath = req.file.path;
    const filename = req.file.filename;

    // Set streaming headers
    res.setHeader('Content-Type', 'text/plain; charset=utf-8');
    res.setHeader('X-Content-Type-Options', 'nosniff');
    res.setHeader('Cache-Control', 'no-cache, no-transform');
    res.setHeader('Connection', 'keep-alive');
    res.setHeader('Transfer-Encoding', 'chunked');

    const writeLog = (text: string) => {
      try {
        res.write(text);
      } catch {
        // Ignore write errors if client disconnected
      }
    };

    let dbReplaced = false;
    const currentDbPath = getDatabasePath();

    try {
    const now = new Date();
    writeLog(`\n${'='.repeat(50)}\n`);
    writeLog(`  RESTORING FROM UPLOADED BACKUP\n`);
    writeLog(`  ${now.toISOString()}\n`);
    writeLog(`${'='.repeat(50)}\n`);
    writeLog(`  File: ${filename}\n`);
    writeLog(`  Size: ${formatBytes(req.file.size)}\n\n`);

    // 1. Stop user containers
    writeLog(`[1/7] Stopping user containers...\n`);
    try {
      const { stdout: containers } = await execAsync(
        `docker ps -q --filter "name=dl_" 2>/dev/null || echo ""`
      );
      if (containers.trim()) {
        await execAsync(`docker stop ${containers.trim().split('\n').join(' ')}`);
        writeLog(`      + Stopped user containers\n`);
      } else {
        writeLog(`      - No user containers running\n`);
      }
    } catch (e: any) {
      writeLog(`      ! Container stop warning: ${e.message}\n`);
    }

    // 2. Extract backup to temp directory
    writeLog(`\n[2/7] Extracting backup...\n`);
    const tempDir = path.join(BACKUP_DIR, `temp-restore-${Date.now()}`);
    await fsp.rm(tempDir, { recursive: true, force: true });
    await fsp.mkdir(tempDir, { recursive: true });

    await safeExtractZip(backupPath, tempDir);
    writeLog(`      + Extraction complete\n`);

    // 3. Restore database
    writeLog(`\n[3/7] Restoring database...\n`);
    const tempDbPath = path.join(tempDir, 'database', 'docklift.db');
    if (fs.existsSync(tempDbPath)) {
      await restoreDatabaseFile(tempDbPath, currentDbPath, writeLog);
      dbReplaced = true;
    } else {
      writeLog(`      ! No database in backup\n`);
    }

    // 4. Restore deployments
    writeLog(`\n[4/7] Restoring deployments...\n`);
    const tempDeploymentsPath = path.join(tempDir, 'deployments');
    if (fs.existsSync(tempDeploymentsPath)) {
      await replaceDirContents(tempDeploymentsPath, DEPLOYMENTS_PATH);
      writeLog(`      + Deployments restored\n`);
    } else {
      writeLog(`      ! No deployments in backup\n`);
    }

    // 5. Restore nginx configs
    writeLog(`\n[5/8] Restoring nginx configs...\n`);
    const tempNginxPath = path.join(tempDir, 'nginx-conf');
    if (fs.existsSync(tempNginxPath)) {
      await fsp.mkdir(NGINX_CONF_PATH, { recursive: true });
      await replaceDirContents(tempNginxPath, NGINX_CONF_PATH);
      writeLog(`      + Nginx configs restored\n`);

      try {
        await execAsync('docker exec docklift-nginx-proxy nginx -s reload 2>/dev/null || true');
        writeLog(`      + Nginx proxy reloaded\n`);
      } catch {
        writeLog(`      - Nginx reload skipped\n`);
      }
    } else {
      writeLog(`      - No nginx configs in backup\n`);
    }

    writeLog(`\n[6/8] Restoring TLS certificates...\n`);
    const tempLePath = path.join(tempDir, 'letsencrypt');
    if (fs.existsSync(tempLePath)) {
      await fsp.mkdir(LETSENCRYPT_PATH, { recursive: true });
      await replaceDirContents(tempLePath, LETSENCRYPT_PATH);
      writeLog(`      + Let's Encrypt certificates restored\n`);
      try {
        await execAsync('docker exec docklift-nginx-proxy nginx -s reload 2>/dev/null || true');
        writeLog(`      + Nginx proxy reloaded\n`);
      } catch {
        writeLog(`      - Nginx reload skipped\n`);
      }
    } else {
      writeLog(`      - No certificates in backup\n`);
    }

    // 7. Restore GitHub key
    writeLog(`\n[7/8] Restoring GitHub App key...\n`);
    const tempGithubKeyPath = path.join(tempDir, 'github-app.pem');
    if (fs.existsSync(tempGithubKeyPath)) {
      await fsp.copyFile(tempGithubKeyPath, GITHUB_KEY_PATH);
      writeLog(`      + GitHub key restored\n`);
    } else {
      writeLog(`      - No GitHub key in backup\n`);
    }

    // Clean up temp directory
    await fsp.rm(tempDir, { recursive: true, force: true });

    // Mark file as restored by renaming (keep file for manual deletion)
    writeLog(`\n[8/8] Marking file as restored...\n`);
    try {
      // Add restored timestamp to filename: file.zip -> file.restored-2024-01-08.zip
      const timestamp = new Date().toISOString().slice(0, 10);
      const baseName = filename.replace('.zip', '');
      const newFilename = `${baseName}.restored-${timestamp}.zip`;
      const newPath = path.join(UPLOADS_DIR, newFilename);

      // Only rename if not already marked as restored
      if (!filename.includes('.restored-')) {
        await fsp.rename(backupPath, newPath);
        writeLog(`      + Marked as restored: ${newFilename}\n`);
      } else {
        writeLog(`      - Already marked as restored\n`);
      }
    } catch (renameError: any) {
      writeLog(`      ! Could not mark file: ${renameError.message}\n`);
    }

    // Reconcile system state (auto-redeploy)
    const reconcileOk = await reconcileSystem(writeLog);
    const outcome = await finishRestoreCommit({
      writeLog,
      reconcileOk,
      dbReplaced,
      currentDbPath,
      setupAuth,
    });

    writeLog(`\n${'='.repeat(50)}\n`);
    if (outcome === 'critical') {
      writeLog(`  [CRITICAL] RESTORE FAILED — MANUAL RECOVERY REQUIRED\n`);
      writeLog(`  Seal: data/.restore-critical (lock held; restores blocked across restarts)\n`);
      writeLog(`${'='.repeat(50)}\n`);
      res.end();
      return;
    }
    if (outcome === 'aborted') {
      writeLog(`  [ERROR] RESTORE ABORTED\n`);
      writeLog(`${'='.repeat(50)}\n`);
      exitMaintenance();
      releaseRestoreLock();
      res.end();
      return;
    }
    if (reconcileOk) {
      writeLog(`  RESTORE COMPLETE\n`);
    } else {
      writeLog(`  [ERROR] RESTORE INCOMPLETE — data restored but some projects failed to start\n`);
    }
    writeLog(`${'='.repeat(50)}\n`);
    writeLog(`\n  [!] Restarting backend service to apply changes...\n`);
    writeLog(`\n  The uploaded file has been kept. You can delete it manually from Settings.\n`);

    res.end();

    // Trigger restart
    setTimeout(() => {
      console.log('Restarting backend service after restore...');
      process.exit(0);
    }, 1000);
    } catch (innerError: any) {
      if (dbReplaced) {
        writeLog(`\n[ROLLBACK] Attempting database rollback after failed restore...\n`);
        const rolled = await rollbackDatabaseFromPreRestore(currentDbPath, writeLog);
        if (!rolled) {
          writeLog(`\n[CRITICAL] Rollback failed — sealed; do not retry\n`);
          writeLog(`  Paths: ${currentDbPath} | ${currentDbPath}.pre-restore\n`);
          enterRestoreCritical({
            detail: innerError?.message || 'upload restore failed after DB replace',
            liveDbPath: currentDbPath,
          });
          res.end();
          return;
        }
      }
      throw innerError;
    }
  } catch (error: any) {
    if (isRestoreCritical()) {
      if (!res.writableEnded) res.end();
      return;
    }
    if (!res.writableEnded) {
      exitMaintenance();
      releaseRestoreLock();
    }
    // Keep setup token on failure so the operator can retry (when DB rolled back)
    console.error('Upload restore error:', error);
    if (!res.headersSent) {
      res.status(500).json({ error: error.message || 'Failed to restore from uploaded backup' });
    } else if (!res.writableEnded) {
      res.write(`\n[ERROR] Restore failed: ${error.message}\n`);
      res.end();
    }
  }
});

// POST /api/backup/restore-from-upload/:filename - Restore from an already uploaded file
router.post('/restore-from-upload/:filename', requireRestoreLock('restore-from-upload'), async (req: Request, res: Response) => {
  if (!(await requireStepUpPassword(req, res))) {
    releaseRestoreLock();
    return;
  }

  const { filename } = req.params;

  if (!isValidBackupFilename(filename)) {
    releaseRestoreLock();
    return res.status(400).json({ error: 'Invalid backup filename' });
  }

  const backupPath = path.join(UPLOADS_DIR, filename);

  if (!fs.existsSync(backupPath)) {
    releaseRestoreLock();
    return res.status(404).json({ error: 'Uploaded file not found' });
  }

  enterMaintenance('Restoring from uploaded backup');

  // Set streaming headers
  res.setHeader('Content-Type', 'text/plain; charset=utf-8');
  res.setHeader('X-Content-Type-Options', 'nosniff');
  res.setHeader('Cache-Control', 'no-cache, no-transform');
  res.setHeader('Connection', 'keep-alive');
  res.setHeader('Transfer-Encoding', 'chunked');

  const writeLog = (text: string) => {
    try {
      res.write(text);
    } catch {
      // Ignore write errors if client disconnected
    }
  };

  let dbReplaced = false;
  const currentDbPath = getDatabasePath();

  try {
    const stats = await fsp.stat(backupPath);
    const now = new Date();
    writeLog(`\n${'='.repeat(50)}\n`);
    writeLog(`  RESTORING FROM UPLOADED FILE\n`);
    writeLog(`  ${now.toISOString()}\n`);
    writeLog(`${'='.repeat(50)}\n`);
    writeLog(`  File: ${filename}\n`);
    writeLog(`  Size: ${formatBytes(stats.size)}\n\n`);

    // 1. Stop user containers
    writeLog(`[1/7] Stopping user containers...\n`);
    try {
      const { stdout: containers } = await execAsync(
        `docker ps -q --filter "name=dl_" 2>/dev/null || echo ""`
      );
      if (containers.trim()) {
        await execAsync(`docker stop ${containers.trim().split('\n').join(' ')}`);
        writeLog(`      + Stopped user containers\n`);
      } else {
        writeLog(`      - No user containers running\n`);
      }
    } catch (e: any) {
      writeLog(`      ! Container stop warning: ${e.message}\n`);
    }

    // 2. Extract backup to temp directory
    writeLog(`\n[2/7] Extracting backup...\n`);
    const tempDir = path.join(BACKUP_DIR, `temp-restore-${Date.now()}`);
    await fsp.rm(tempDir, { recursive: true, force: true });
    await fsp.mkdir(tempDir, { recursive: true });

    await safeExtractZip(backupPath, tempDir);
    writeLog(`      + Extraction complete\n`);

    // 3. Restore database
    writeLog(`\n[3/7] Restoring database...\n`);
    const tempDbPath = path.join(tempDir, 'database', 'docklift.db');
    if (fs.existsSync(tempDbPath)) {
      await restoreDatabaseFile(tempDbPath, currentDbPath, writeLog);
      dbReplaced = true;
    } else {
      writeLog(`      ! No database in backup\n`);
    }

    // 4. Restore deployments
    writeLog(`\n[4/7] Restoring deployments...\n`);
    const tempDeploymentsPath = path.join(tempDir, 'deployments');
    if (fs.existsSync(tempDeploymentsPath)) {
      await replaceDirContents(tempDeploymentsPath, DEPLOYMENTS_PATH);
      writeLog(`      + Deployments restored\n`);
    } else {
      writeLog(`      ! No deployments in backup\n`);
    }

    // 5. Restore nginx configs
    writeLog(`\n[5/8] Restoring nginx configs...\n`);
    const tempNginxPath = path.join(tempDir, 'nginx-conf');
    if (fs.existsSync(tempNginxPath)) {
      await fsp.mkdir(NGINX_CONF_PATH, { recursive: true });
      await replaceDirContents(tempNginxPath, NGINX_CONF_PATH);
      writeLog(`      + Nginx configs restored\n`);

      try {
        await execAsync('docker exec docklift-nginx-proxy nginx -s reload 2>/dev/null || true');
        writeLog(`      + Nginx proxy reloaded\n`);
      } catch {
        writeLog(`      - Nginx reload skipped\n`);
      }
    } else {
      writeLog(`      - No nginx configs in backup\n`);
    }

    writeLog(`\n[6/8] Restoring TLS certificates...\n`);
    const tempLePath = path.join(tempDir, 'letsencrypt');
    if (fs.existsSync(tempLePath)) {
      await fsp.mkdir(LETSENCRYPT_PATH, { recursive: true });
      await replaceDirContents(tempLePath, LETSENCRYPT_PATH);
      writeLog(`      + Let's Encrypt certificates restored\n`);
      try {
        await execAsync('docker exec docklift-nginx-proxy nginx -s reload 2>/dev/null || true');
        writeLog(`      + Nginx proxy reloaded\n`);
      } catch {
        writeLog(`      - Nginx reload skipped\n`);
      }
    } else {
      writeLog(`      - No certificates in backup\n`);
    }

    // 7. Restore GitHub key
    writeLog(`\n[7/8] Restoring GitHub App key...\n`);
    const tempGithubKeyPath = path.join(tempDir, 'github-app.pem');
    if (fs.existsSync(tempGithubKeyPath)) {
      await fsp.copyFile(tempGithubKeyPath, GITHUB_KEY_PATH);
      writeLog(`      + GitHub key restored\n`);
    } else {
      writeLog(`      - No GitHub key in backup\n`);
    }

    // Clean up temp directory
    await fsp.rm(tempDir, { recursive: true, force: true });

    // Mark file as restored by renaming (keep file for manual deletion)
    writeLog(`\n[8/8] Marking file as restored...\n`);
    try {
      // Add restored timestamp to filename: file.zip -> file.restored-2024-01-08.zip
      const timestamp = new Date().toISOString().slice(0, 10);
      const baseName = filename.replace('.zip', '');
      const newFilename = `${baseName}.restored-${timestamp}.zip`;
      const newPath = path.join(UPLOADS_DIR, newFilename);

      // Only rename if not already marked as restored
      if (!filename.includes('.restored-')) {
        await fsp.rename(backupPath, newPath);
        writeLog(`      + Marked as restored: ${newFilename}\n`);
      } else {
        writeLog(`      - Already marked as restored\n`);
      }
    } catch (renameError: any) {
      writeLog(`      ! Could not mark file: ${renameError.message}\n`);
    }

    // Reconcile system state (auto-redeploy)
    const reconcileOk = await reconcileSystem(writeLog);
    const outcome = await finishRestoreCommit({
      writeLog,
      reconcileOk,
      dbReplaced,
      currentDbPath,
      setupAuth: false,
    });

    writeLog(`\n${'='.repeat(50)}\n`);
    if (outcome === 'critical') {
      writeLog(`  [CRITICAL] RESTORE FAILED — MANUAL RECOVERY REQUIRED\n`);
      writeLog(`  Seal: data/.restore-critical (lock held; restores blocked across restarts)\n`);
      writeLog(`${'='.repeat(50)}\n`);
      res.end();
      return;
    }
    if (outcome === 'aborted') {
      writeLog(`  [ERROR] RESTORE ABORTED\n`);
      writeLog(`${'='.repeat(50)}\n`);
      exitMaintenance();
      releaseRestoreLock();
      res.end();
      return;
    }
    if (reconcileOk) {
      writeLog(`  RESTORE COMPLETE\n`);
    } else {
      writeLog(`  [ERROR] RESTORE INCOMPLETE — data restored but some projects failed to start\n`);
    }
    writeLog(`${'='.repeat(50)}\n`);
    writeLog(`\n  [!] Restarting backend service to apply changes...\n`);
    writeLog(`\n  The uploaded file has been kept. You can delete it manually from Settings.\n`);

    res.end();

    // Trigger restart
    setTimeout(() => {
      console.log('Restarting backend service after restore...');
      process.exit(0);
    }, 1000);
  } catch (error: any) {
    if (dbReplaced) {
      writeLog(`\n[ROLLBACK] Attempting database rollback after failed restore...\n`);
      const rolled = await rollbackDatabaseFromPreRestore(currentDbPath, writeLog);
      if (!rolled) {
        writeLog(`\n[CRITICAL] Rollback failed — sealed; do not retry\n`);
        writeLog(`  Paths: ${currentDbPath} | ${currentDbPath}.pre-restore\n`);
        enterRestoreCritical({
          detail: error?.message || 'restore-from-upload failed after DB replace',
          liveDbPath: currentDbPath,
        });
        res.end();
        return;
      }
    }
    exitMaintenance();
    releaseRestoreLock();
    console.error('Restore from upload error:', error);
    if (!res.headersSent) {
      res.status(500).json({ error: error.message || 'Failed to restore from uploaded file' });
    } else {
      res.write(`\n[ERROR] Restore failed: ${error.message}\n`);
      res.end();
    }
  }
});

/** POST /api/backup/clear-critical-restore — operator ack after manual DB repair */
router.post('/clear-critical-restore', async (req: Request, res: Response) => {
  try {
    if (!isRestoreCritical()) {
      return res.status(400).json({ error: 'No critical restore seal is active' });
    }
    if (!(await requireStepUpPassword(req, res))) return;
    const prev = clearRestoreCritical();
    res.json({
      success: true,
      message: 'Critical restore seal cleared',
      previous: prev,
    });
  } catch (error: any) {
    res.status(500).json({ error: error.message || 'Failed to clear critical seal' });
  }
});

router.get('/critical-status', async (_req: Request, res: Response) => {
  const payload = readRestoreCritical();
  res.json({
    critical: isRestoreCritical(),
    seal: payload,
  });
});

export default router;
