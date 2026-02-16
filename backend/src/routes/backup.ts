// Backup routes - API endpoints for backup and restore operations
import { Router, Request, Response } from 'express';
import { exec } from 'child_process';
import { promisify } from 'util';
import fs from 'fs';
import fsp from 'fs/promises';
import path from 'path';
import archiver from 'archiver';
import unzipper from 'unzipper';
import multer from 'multer';
import { config } from '../lib/config.js';
import { PrismaClient } from '@prisma/client';

const execAsync = promisify(exec);
const router = Router();

// Helper: Reconcile system state after restore
// 1. Redeploy all active projects
// 2. Reload Nginx
// 3. Restart Backend
async function reconcileSystem(writeLog: (text: string) => void) {
  writeLog(`\n${'='.repeat(50)}\n`);
  writeLog(`  RECONCILING SYSTEM STATE\n`);
  writeLog(`${'='.repeat(50)}\n\n`);

  try {
    // 1. Initialize FRESH Prisma Client (to read the restored DB file)
    writeLog(`[1/3] Reading restored database...\n`);
    const prisma = new PrismaClient();
    
    // 2. Fetch all projects
    const projects = await prisma.project.findMany();
    writeLog(`      + Found ${projects.length} projects in database\n`);

    // 3. Loop and redeploy
    writeLog(`\n[2/3] Auto-redeploying projects...\n`);
    for (const project of projects) {
      const projectPath = path.join(config.deploymentsPath, project.id);
      const sourcePath = path.join(projectPath, 'source');
      
      // Only deploy if source code exists
      if (fs.existsSync(sourcePath)) {
        writeLog(`      > Redeploying ${project.name} (${project.id})...\n`);
        try {
          // Verify docker-compose.yml exists
          if (fs.existsSync(path.join(projectPath, 'docker-compose.yml'))) {
             // Run docker compose up -d --build
             // We use --build to ensure the image is recreated from the restored source
             await execAsync(`docker compose up -d --build`, { cwd: projectPath });
             writeLog(`        + Success\n`);
          } else {
             writeLog(`        ! Skipped (No docker-compose.yml)\n`);
          }
        } catch (e: any) {
             writeLog(`        ! Failed: ${e.message.split('\n')[0]}\n`);
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
      writeLog(`      ! Nginx reload failed: ${e.message.split('\n')[0]}\n`);
    }

    // Disconnect prisma
    await prisma.$disconnect();

  } catch (error: any) {
    writeLog(`\n[ERROR] Reconcile failed: ${error.message}\n`);
    console.error('Reconcile error:', error);
  }
}


// Configure multer for backup uploads (saves to uploads subdirectory)
const uploadStorage = multer.diskStorage({
  destination: async (req, file, cb) => {
    const uploadDir = path.join(process.env.BACKUP_PATH || '/data/backups', 'uploads');
    await fsp.mkdir(uploadDir, { recursive: true });
    cb(null, uploadDir);
  },
  filename: (req, file, cb) => {
    // Keep original filename but sanitize it
    const sanitized = file.originalname.replace(/[^a-zA-Z0-9._-]/g, '_');
    cb(null, sanitized);
  }
});

const uploadBackup = multer({
  storage: uploadStorage,
  limits: { fileSize: 10 * 1024 * 1024 * 1024 }, // 10GB max
  fileFilter: (req, file, cb) => {
    if (file.mimetype === 'application/zip' || file.originalname.endsWith('.zip')) {
      cb(null, true);
    } else {
      cb(new Error('Only ZIP files are allowed'));
    }
  }
});

// Configuration paths
const BACKUP_DIR = process.env.BACKUP_PATH || '/data/backups';
const UPLOADS_DIR = path.join(BACKUP_DIR, 'uploads'); // Separate directory for uploaded restore files
const DEPLOYMENTS_PATH = config.deploymentsPath;
const NGINX_CONF_PATH = config.nginxConfPath;
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

    // 1. Add database
    writeLog(`[1/4] Adding database...\n`);
    const dbPath = getDatabasePath();
    if (fs.existsSync(dbPath)) {
      const dbStats = await fsp.stat(dbPath);
      archive.file(dbPath, { name: 'database/docklift.db' });
      writeLog(`      + docklift.db (${formatBytes(dbStats.size)})\n`);
    } else {
      writeLog(`      ! Database not found at ${dbPath}\n`);
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
    writeLog(`\n[3/4] Adding nginx configs...\n`);
    if (fs.existsSync(NGINX_CONF_PATH)) {
      const nginxSize = await getDirectorySize(NGINX_CONF_PATH);
      archive.directory(NGINX_CONF_PATH, 'nginx-conf');
      writeLog(`      + /nginx-conf/ (${formatBytes(nginxSize)})\n`);
    } else {
      writeLog(`      - Nginx config directory not found (skipped)\n`);
    }

    // 4. Add GitHub key if exists
    writeLog(`\n[4/4] Adding GitHub App key...\n`);
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

    res.end();
  } catch (error: any) {
    writeLog(`\n[ERROR] Backup failed: ${error.message}\n`);
    console.error('Backup creation error:', error);
    res.end();
  }
});

// POST /api/backup/restore/:filename - Restore from backup with streaming progress
router.post('/restore/:filename', async (req: Request, res: Response) => {
  const { filename } = req.params;

  // Security validation
  if (!isValidBackupFilename(filename)) {
    return res.status(400).json({ error: 'Invalid backup filename' });
  }

  const backupPath = path.join(BACKUP_DIR, filename);

  if (!fs.existsSync(backupPath)) {
    return res.status(404).json({ error: 'Backup file not found' });
  }

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

    await new Promise<void>((resolve, reject) => {
      fs.createReadStream(backupPath)
        .pipe(unzipper.Extract({ path: tempDir }))
        .on('close', resolve)
        .on('error', reject);
    });
    writeLog(`      + Extraction complete\n`);

    // 3. Restore database
    writeLog(`\n[3/7] Restoring database...\n`);
    const tempDbPath = path.join(tempDir, 'database', 'docklift.db');
    const currentDbPath = getDatabasePath();
    if (fs.existsSync(tempDbPath)) {
      // Create backup of current database
      if (fs.existsSync(currentDbPath)) {
        const backupDbPath = `${currentDbPath}.pre-restore`;
        await fsp.copyFile(currentDbPath, backupDbPath);
        writeLog(`      + Created backup of current database\n`);
      }
      // Ensure target directory exists
      await fsp.mkdir(path.dirname(currentDbPath), { recursive: true });
      await fsp.copyFile(tempDbPath, currentDbPath);
      writeLog(`      + Database restored\n`);
    } else {
      writeLog(`      ! No database in backup\n`);
    }

    // 4. Restore deployments
    writeLog(`\n[4/7] Restoring deployments...\n`);
    const tempDeploymentsPath = path.join(tempDir, 'deployments');
    if (fs.existsSync(tempDeploymentsPath)) {
      // Clear existing deployments contents (don't remove the mount point itself)
      try {
        const existingItems = await fsp.readdir(DEPLOYMENTS_PATH);
        for (const item of existingItems) {
          await fsp.rm(path.join(DEPLOYMENTS_PATH, item), { recursive: true, force: true });
        }
      } catch {
        // Directory might not exist yet
      }
      await fsp.mkdir(DEPLOYMENTS_PATH, { recursive: true });
      await execAsync(`cp -r "${tempDeploymentsPath}/." "${DEPLOYMENTS_PATH}/"`);
      writeLog(`      + Deployments restored\n`);
    } else {
      writeLog(`      ! No deployments in backup\n`);
    }

    // 5. Restore nginx configs
    writeLog(`\n[5/7] Restoring nginx configs...\n`);
    const tempNginxPath = path.join(tempDir, 'nginx-conf');
    if (fs.existsSync(tempNginxPath)) {
      await fsp.mkdir(NGINX_CONF_PATH, { recursive: true });
      await execAsync(`cp -r "${tempNginxPath}/." "${NGINX_CONF_PATH}/"`);
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

    // 6. Restore GitHub key
    writeLog(`\n[6/7] Restoring GitHub App key...\n`);
    const tempGithubKeyPath = path.join(tempDir, 'github-app.pem');
    if (fs.existsSync(tempGithubKeyPath)) {
      await fsp.copyFile(tempGithubKeyPath, GITHUB_KEY_PATH);
      writeLog(`      + GitHub key restored\n`);
    } else {
      writeLog(`      - No GitHub key in backup\n`);
    }

    // Clean up temp directory
    await fsp.rm(tempDir, { recursive: true, force: true });

    // Delete all backup files from the server
    writeLog(`\n[7/7] Cleaning up backup files...\n`);
    try {
      const backupFiles = await fsp.readdir(BACKUP_DIR);
      let deletedCount = 0;
      for (const file of backupFiles) {
        if (file.endsWith('.zip')) {
          const filePath = path.join(BACKUP_DIR, file);
          await fsp.unlink(filePath);
          writeLog(`      - Removed: ${file}\n`);
          deletedCount++;
        }
      }
      if (deletedCount === 0) {
        writeLog(`      - No backup files to remove\n`);
      } else {
        writeLog(`      + Removed ${deletedCount} backup file(s)\n`);
      }
    } catch (cleanupError: any) {
      writeLog(`      ! Failed to clean backup files: ${cleanupError.message}\n`);
    }

    // Reconcile system state (auto-redeploy)
    await reconcileSystem(writeLog);

    writeLog(`\n${'='.repeat(50)}\n`);
    writeLog(`  RESTORE COMPLETE\n`);
    writeLog(`${'='.repeat(50)}\n`);
    writeLog(`\n  [!] Restarting backend service to apply changes...\n`);

    res.end();

    // Trigger restart
    setTimeout(() => {
      console.log('Restarting backend service after restore...');
      process.exit(0);
    }, 1000);
  } catch (error: any) {
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
router.post('/restore-upload', uploadBackup.single('backup'), async (req: Request, res: Response) => {
  try {
    if (!req.file) {
      return res.status(400).json({ error: 'No backup file uploaded' });
    }

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

    await new Promise<void>((resolve, reject) => {
      fs.createReadStream(backupPath)
        .pipe(unzipper.Extract({ path: tempDir }))
        .on('close', resolve)
        .on('error', reject);
    });
    writeLog(`      + Extraction complete\n`);

    // 3. Restore database
    writeLog(`\n[3/7] Restoring database...\n`);
    const tempDbPath = path.join(tempDir, 'database', 'docklift.db');
    const currentDbPath = getDatabasePath();
    if (fs.existsSync(tempDbPath)) {
      if (fs.existsSync(currentDbPath)) {
        const backupDbPath = `${currentDbPath}.pre-restore`;
        await fsp.copyFile(currentDbPath, backupDbPath);
        writeLog(`      + Created backup of current database\n`);
      }
      // Ensure target directory exists
      await fsp.mkdir(path.dirname(currentDbPath), { recursive: true });
      await fsp.copyFile(tempDbPath, currentDbPath);
      writeLog(`      + Database restored\n`);
    } else {
      writeLog(`      ! No database in backup\n`);
    }

    // 4. Restore deployments
    writeLog(`\n[4/7] Restoring deployments...\n`);
    const tempDeploymentsPath = path.join(tempDir, 'deployments');
    if (fs.existsSync(tempDeploymentsPath)) {
      // Clear existing deployments contents (don't remove the mount point itself)
      try {
        const existingItems = await fsp.readdir(DEPLOYMENTS_PATH);
        for (const item of existingItems) {
          await fsp.rm(path.join(DEPLOYMENTS_PATH, item), { recursive: true, force: true });
        }
      } catch {
        // Directory might not exist yet
      }
      await fsp.mkdir(DEPLOYMENTS_PATH, { recursive: true });
      await execAsync(`cp -r "${tempDeploymentsPath}/." "${DEPLOYMENTS_PATH}/"`);
      writeLog(`      + Deployments restored\n`);
    } else {
      writeLog(`      ! No deployments in backup\n`);
    }

    // 5. Restore nginx configs
    writeLog(`\n[5/7] Restoring nginx configs...\n`);
    const tempNginxPath = path.join(tempDir, 'nginx-conf');
    if (fs.existsSync(tempNginxPath)) {
      await fsp.mkdir(NGINX_CONF_PATH, { recursive: true });
      await execAsync(`cp -r "${tempNginxPath}/." "${NGINX_CONF_PATH}/"`);
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

    // 6. Restore GitHub key
    writeLog(`\n[6/7] Restoring GitHub App key...\n`);
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
    writeLog(`\n[7/7] Marking file as restored...\n`);
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
    await reconcileSystem(writeLog);

    writeLog(`\n${'='.repeat(50)}\n`);
    writeLog(`  RESTORE COMPLETE\n`);
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
    console.error('Upload restore error:', error);
    if (!res.headersSent) {
      res.status(500).json({ error: error.message || 'Failed to restore from uploaded backup' });
    } else {
      res.write(`\n[ERROR] Restore failed: ${error.message}\n`);
      res.end();
    }
  }
});

// POST /api/backup/restore-from-upload/:filename - Restore from an already uploaded file
router.post('/restore-from-upload/:filename', async (req: Request, res: Response) => {
  const { filename } = req.params;

  if (!isValidBackupFilename(filename)) {
    return res.status(400).json({ error: 'Invalid backup filename' });
  }

  const backupPath = path.join(UPLOADS_DIR, filename);

  if (!fs.existsSync(backupPath)) {
    return res.status(404).json({ error: 'Uploaded file not found' });
  }

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

    await new Promise<void>((resolve, reject) => {
      fs.createReadStream(backupPath)
        .pipe(unzipper.Extract({ path: tempDir }))
        .on('close', resolve)
        .on('error', reject);
    });
    writeLog(`      + Extraction complete\n`);

    // 3. Restore database
    writeLog(`\n[3/7] Restoring database...\n`);
    const tempDbPath = path.join(tempDir, 'database', 'docklift.db');
    const currentDbPath = getDatabasePath();
    if (fs.existsSync(tempDbPath)) {
      if (fs.existsSync(currentDbPath)) {
        const backupDbPath = `${currentDbPath}.pre-restore`;
        await fsp.copyFile(currentDbPath, backupDbPath);
        writeLog(`      + Created backup of current database\n`);
      }
      await fsp.mkdir(path.dirname(currentDbPath), { recursive: true });
      await fsp.copyFile(tempDbPath, currentDbPath);
      writeLog(`      + Database restored\n`);
    } else {
      writeLog(`      ! No database in backup\n`);
    }

    // 4. Restore deployments
    writeLog(`\n[4/7] Restoring deployments...\n`);
    const tempDeploymentsPath = path.join(tempDir, 'deployments');
    if (fs.existsSync(tempDeploymentsPath)) {
      try {
        const existingItems = await fsp.readdir(DEPLOYMENTS_PATH);
        for (const item of existingItems) {
          await fsp.rm(path.join(DEPLOYMENTS_PATH, item), { recursive: true, force: true });
        }
      } catch {
        // Directory might not exist yet
      }
      await fsp.mkdir(DEPLOYMENTS_PATH, { recursive: true });
      await execAsync(`cp -r "${tempDeploymentsPath}/." "${DEPLOYMENTS_PATH}/"`);
      writeLog(`      + Deployments restored\n`);
    } else {
      writeLog(`      ! No deployments in backup\n`);
    }

    // 5. Restore nginx configs
    writeLog(`\n[5/7] Restoring nginx configs...\n`);
    const tempNginxPath = path.join(tempDir, 'nginx-conf');
    if (fs.existsSync(tempNginxPath)) {
      await fsp.mkdir(NGINX_CONF_PATH, { recursive: true });
      await execAsync(`cp -r "${tempNginxPath}/." "${NGINX_CONF_PATH}/"`);
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

    // 6. Restore GitHub key
    writeLog(`\n[6/7] Restoring GitHub App key...\n`);
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
    writeLog(`\n[7/7] Marking file as restored...\n`);
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
    await reconcileSystem(writeLog);

    writeLog(`\n${'='.repeat(50)}\n`);
    writeLog(`  RESTORE COMPLETE\n`);
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
    console.error('Restore from upload error:', error);
    if (!res.headersSent) {
      res.status(500).json({ error: error.message || 'Failed to restore from uploaded file' });
    } else {
      res.write(`\n[ERROR] Restore failed: ${error.message}\n`);
      res.end();
    }
  }
});

export default router;
