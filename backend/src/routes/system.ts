// System routes - API endpoints for monitoring, server controls, and command execution
import { Router, Request, Response } from 'express';
import si from 'systeminformation';
import { exec } from 'child_process';
import { promisify } from 'util';
import os from 'os';
import { readFileSync } from 'fs';
import { join, dirname } from 'path';
import { fileURLToPath } from 'url';

const __filenameSystem = fileURLToPath(import.meta.url);
const __dirnameSystem = dirname(__filenameSystem);

const execAsync = promisify(exec);

const router = Router();

// Cache for reducing system calls
let cachedStats: SystemStats | null = null;
let lastFetch = 0;
const CACHE_TTL = 3000; // 3 seconds cache for fast response

// Background cache for slow data (connections count)
let cachedConnections = 0;
let lastConnectionsFetch = 0;
const CONNECTIONS_CACHE_TTL = 30000; // 30 seconds for slow data

// Cache for public IP and location (fetched once, cached for 5 minutes)
let cachedPublicIP = 'N/A';
let cachedLocation = 'N/A';
let lastIPFetch = 0;
const IP_CACHE_TTL = 300000; // 5 minutes

// Fetch public IP and location in background
async function fetchPublicIPInfo() {
  try {
    const response = await fetch('http://ip-api.com/json/?fields=query,city,country');
    if (response.ok) {
      const data = await response.json() as { query?: string; city?: string; country?: string };
      cachedPublicIP = data.query || 'N/A';
      cachedLocation = data.city && data.country ? `${data.city}, ${data.country}` : 'N/A';
    }
  } catch {
    // Fallback to local IP if external API fails
  }
  lastIPFetch = Date.now();
}

interface SystemStats {
  cpu: {
    usage: number;
    cores: number;
    model: string;
    speed: number;
    temperature: number | null;
  };
  memory: {
    total: number;
    used: number;
    free: number;
    usedPercent: number;
  };
  gpu: {
    available: boolean;
    model: string | null;
    memoryTotal: number | null;
    memoryUsed: number | null;
    temperature: number | null;
    utilization: number | null;
  };
  disk: Array<{
    mount: string;
    type: string;
    total: number;
    used: number;
    usedPercent: number;
  }>;
  network: {
    bytesReceived: number;
    bytesSent: number;
    rxSpeed: number;
    txSpeed: number;
  };
  server: {
    hostname: string;
    platform: string;
    distro: string;
    kernel: string;
    arch: string;
    uptime: number;
    uptimeFormatted: string;
    serverTime: string;
    cpuModel: string;
    cpuCores: string;
    loadAvg: {
      load1: number;
      load5: number;
      load15: number;
    };
    swap: {
      total: number;
      used: number;
    };
    ipAddress: string;
    location: string;
    activeConnections: number;
  };
  processes: Array<{
    pid: number;
    name: string;
    cpu: number;
    mem: number;
    user: string;
  }>;
  timestamp: string;
}

// Format bytes to human readable
function formatBytes(bytes: number): string {
  if (bytes === 0) return '0 B';
  const k = 1024;
  const sizes = ['B', 'KB', 'MB', 'GB', 'TB'];
  const i = Math.floor(Math.log(bytes) / Math.log(k));
  return parseFloat((bytes / Math.pow(k, i)).toFixed(2)) + ' ' + sizes[i];
}

// Format uptime to human readable
function formatUptime(seconds: number): string {
  const days = Math.floor(seconds / 86400);
  const hours = Math.floor((seconds % 86400) / 3600);
  const minutes = Math.floor((seconds % 3600) / 60);
  
  const parts = [];
  if (days > 0) parts.push(`${days} Day${days > 1 ? 's' : ''}`);
  if (hours > 0) parts.push(`${hours} Hour${hours > 1 ? 's' : ''}`);
  if (minutes > 0) parts.push(`${minutes} Min${minutes > 1 ? 's' : ''}`);
  
  return parts.length > 0 ? parts.join(', ') : '< 1 Min';
}

// Helper to read host file content safely
async function readHostFile(path: string): Promise<string | null> {
  try {
    const fs = require('fs/promises');
    const content = await fs.readFile(path, 'utf8');
    return content.trim();
  } catch (e) {
    return null;
  }
}

// Helper to parse os-release file
async function readHostOSInfo() {
  const content = await readHostFile('/host/os-release');
  if (!content) return null;
  
  const info: any = {};
  content.split('\n').forEach((line: string) => {
    const [key, value] = line.split('=');
    if (key && value) {
      info[key] = value.replace(/"/g, '');
    }
  });
  
  return {
    platform: 'linux',
    distro: info.PRETTY_NAME || info.NAME || 'Linux',
    release: info.VERSION_ID || ''
  };
}

// Helper to getting processes using 'top' on Linux (more accurate in Docker)
async function getLinuxTopProcesses() {
  try {
    // Run top in batch mode, 1 iteration, sorted by CPU
    // -b: batch mode, -n 1: 1 iteration, -w 512: wide output to prevent truncation
    // -o %CPU: sort by CPU
    const { stdout } = await execAsync('top -b -n 1 -w 512 -o %CPU | head -n 20');    
    const lines = stdout.split('\n');
    // Find header line to identify columns
    const headerIndex = lines.findIndex(l => l.includes('PID') && l.includes('COMMAND'));
    
    if (headerIndex === -1) return null;
    
    const processes = [];
    
    // Skip header and process rows
    for (let i = headerIndex + 1; i < lines.length; i++) {
      const line = lines[i].trim();
      if (!line) continue;
      
      const parts = line.split(/\s+/);
      // Top output format usually: PID USER PR NI VIRT RES SHR S %CPU %MEM TIME+ COMMAND
      // But can vary. We rely on standard column positions for PID, USER, CPU, MEM, COMMAND
      
      const pid = parseInt(parts[0]);
      if (isNaN(pid)) continue;
      
      const user = parts[1];
      const cpu = parseFloat(parts[8]); // 9th column is usually %CPU
      const mem = parseFloat(parts[9]); // 10th column is usually %MEM
      const name = parts.slice(11).join(' '); // Command starts at 12th column
      
      if (name !== 'top') {
        processes.push({ pid, name, cpu, mem, user });
      }
    }
    
    return processes;
  } catch (error) {
    console.error('Error running top:', error);
    return null;
  }
}

// Get comprehensive system stats
async function getSystemStats(): Promise<SystemStats> {
  const now = Date.now();
  
  // Return cached data if still fresh
  if (cachedStats && (now - lastFetch) < CACHE_TTL) {
    return cachedStats;
  }

  try {
    // Fetch connections in background (slow call - cache for 30s)
    if (process.platform !== 'win32' && now - lastConnectionsFetch > CONNECTIONS_CACHE_TTL) {
      si.networkConnections().then(conns => {
        cachedConnections = Array.isArray(conns) ? conns.filter((c: { state?: string }) => c.state === 'ESTABLISHED').length : 0;
        lastConnectionsFetch = Date.now();
      }).catch(() => {});
    }

    if (now - lastIPFetch > IP_CACHE_TTL) {
      fetchPublicIPInfo();
    }

    const isWindows = process.platform === 'win32';

    // Fetch all FAST data in parallel
    const [
      cpuData,
      cpuLoad,
      cpuTemp,
      memData,
      diskData,
      networkStats,
      osInfo,
      graphics,
      time,
      networkInterfaces,
      hostHostname,
      hostOS,
      linuxProcesses
    ] = await Promise.all([
      si.cpu(),
      si.currentLoad(),
      isWindows ? Promise.resolve({ main: null }) : si.cpuTemperature().catch(() => ({ main: null })),
      si.mem(),
      si.fsSize(),
      si.networkStats(),
      si.osInfo(),
      isWindows ? Promise.resolve({ controllers: [] }) : si.graphics().catch(() => ({ controllers: [] })),
      si.time(),
      si.networkInterfaces().catch(() => []),
      readHostFile('/host/hostname'),
      readHostOSInfo(),
      // Try to get Linux processes via top if on Linux
      (process.platform === 'linux') ? getLinuxTopProcesses() : Promise.resolve(null)
    ]);

    // Process GPU data
    const gpuController = graphics.controllers?.[0];
    const gpuInfo = {
      available: !!gpuController,
      model: gpuController?.model || null,
      memoryTotal: gpuController?.vram || null,
      memoryUsed: gpuController?.memoryUsed || null,
      temperature: gpuController?.temperatureGpu || null,
      utilization: gpuController?.utilizationGpu || null
    };

    // Process disk data
    const diskInfo = diskData
      .filter(d => d.size > 1024 * 1024 * 100)
      .map(d => ({
        mount: d.mount,
        type: d.type,
        total: d.size,
        used: d.used,
        usedPercent: parseFloat(d.use?.toFixed(1) || '0')
      }));

    // Calculate network speeds
    const primaryNet = networkStats.find(n => n.operstate === 'up') || networkStats[0];
    const networkInfo = {
      bytesReceived: primaryNet?.rx_bytes || 0,
      bytesSent: primaryNet?.tx_bytes || 0,
      rxSpeed: primaryNet?.rx_sec || 0,
      txSpeed: primaryNet?.tx_sec || 0
    };

    // Process List Logic: Prefer 'top' output on Linux, fallback to 'si' elsewhere
    let processList = [];
    if (linuxProcesses && linuxProcesses.length > 0) {
      processList = linuxProcesses;
    } else {
      processList = (await si.processes()).list
        .filter(p => p.name !== 'System Idle Process' && p.name !== 'idle')
        .sort((a, b) => b.cpu - a.cpu)
        .slice(0, 10)
        .map(p => ({
          pid: p.pid,
          name: p.name,
          cpu: parseFloat(p.cpu.toFixed(1)),
          mem: parseFloat(p.mem.toFixed(1)),
          user: p.user
        }));
    }

    const stats: SystemStats = {
      cpu: {
        usage: parseFloat(cpuLoad.currentLoad?.toFixed(1) || '0'),
        cores: cpuData.cores,
        model: cpuData.brand,
        speed: cpuData.speed,
        temperature: cpuTemp?.main || null
      },
      memory: {
        total: memData.total,
        used: memData.used,
        free: memData.free,
        usedPercent: parseFloat(((memData.used / memData.total) * 100).toFixed(1))
      },
      gpu: gpuInfo,
      disk: diskInfo,
      network: networkInfo,
      server: {
        hostname: hostHostname || osInfo.hostname,
        platform: hostOS ? hostOS.platform : osInfo.platform,
        distro: hostOS ? hostOS.distro : osInfo.distro,
        kernel: osInfo.kernel, // Kernel is shared with host
        arch: osInfo.arch,     // Architecture is shared with host
        uptime: time.uptime,
        uptimeFormatted: formatUptime(time.uptime),
        serverTime: new Date().toLocaleString(),
        cpuModel: cpuData.brand,
        cpuCores: `${cpuData.cores} Cores @ ${cpuData.speed} GHz`,
        loadAvg: {
          load1: parseFloat(cpuLoad.avgLoad?.toFixed(2) || '0'),
          load5: parseFloat((cpuLoad.avgLoad * 0.8)?.toFixed(2) || '0'),
          load15: parseFloat((cpuLoad.avgLoad * 0.6)?.toFixed(2) || '0')
        },
        swap: {
          total: memData.swaptotal,
          used: memData.swapused
        },
        ipAddress: cachedPublicIP !== 'N/A' ? cachedPublicIP : ((Array.isArray(networkInterfaces) ? networkInterfaces.find((n: { ip4?: string; internal?: boolean }) => n.ip4 && !n.internal)?.ip4 : null) || 'N/A'),
        location: cachedLocation,
        activeConnections: cachedConnections
      },
      processes: processList.slice(0, 10),
      timestamp: new Date().toISOString()
    };

    cachedStats = stats;
    lastFetch = now;

    return stats;
  } catch (error) {
    console.error('Error fetching system stats:', error);
    throw error;
  }
}

// GET /api/system/stats - Get all system statistics
router.get('/stats', async (req: Request, res: Response) => {
  try {
    const stats = await getSystemStats();
    res.json(stats);
  } catch (error) {
    console.error('Failed to get system stats:', error);
    res.status(500).json({ error: 'Failed to fetch system statistics' });
  }
});

// GET /api/system/quick - Get only CPU and memory (for header widget)
router.get('/quick', async (req: Request, res: Response) => {
  try {
    const [cpuLoad, memData] = await Promise.all([
      si.currentLoad(),
      si.mem()
    ]);

    res.json({
      cpu: parseFloat(cpuLoad.currentLoad?.toFixed(1) || '0'),
      memory: parseFloat(((memData.used / memData.total) * 100).toFixed(1)),
      timestamp: new Date().toISOString()
    });
  } catch (error) {
    res.status(500).json({ error: 'Failed to fetch quick stats' });
  }
});

// GET /api/system/ip - Get server's public IP address
router.get('/ip', async (req: Request, res: Response) => {
  try {
    // Ensure IP is fetched
    if (cachedPublicIP === 'N/A' || Date.now() - lastIPFetch > IP_CACHE_TTL) {
      await fetchPublicIPInfo();
    }
    res.json({ ip: cachedPublicIP });
  } catch (error) {
    res.status(500).json({ error: 'Failed to fetch server IP' });
  }
});

// POST /api/system/purge - Clean up system resources and free memory
router.post('/purge', async (req: Request, res: Response) => {
  try {
    const results: string[] = [];
    
    // 1. Docker Cleanup
    try {
      const { stdout: dockerOutput } = await execAsync('docker system prune -f');
      results.push('Docker cleanup successful');
      // console.log('Docker Prune:', dockerOutput);
    } catch (err: any) {
      console.error('Docker prune failed:', err);
      results.push('Docker cleanup skipped or failed');
    }

    // 2. Clear Linux Memory Caches (Drop PageCache, dentries and inodes)
    // Only works on Linux and requires root
    if (process.platform === 'linux') {
      try {
        await execAsync('sync && echo 3 > /proc/sys/vm/drop_caches');
        results.push('System memory caches cleared');
        
        // Reset system stats cache to reflect changes immediately
        cachedStats = null;
        lastFetch = 0;
      } catch (err: any) {
        console.error('Failed to clear memory caches:', err);
        results.push('Memory cache clearing failed (requires root)');
      }
    } else {
      results.push('Memory cache clearing only supported on Linux');
    }

    res.json({ 
      message: 'Purge operation completed', 
      details: results 
    });
  } catch (error) {
    console.error('Purge error:', error);
    res.status(500).json({ error: 'Failed to complete purge operation' });
  }
});

// POST /api/system/reboot - Reboot the server
router.post('/reboot', async (req: Request, res: Response) => {
  try {
    const isWindows = os.platform() === "win32";
    const isMac = os.platform() === "darwin";

    if (isWindows || isMac) {
      // Dev environment stimulation
      console.log('Reboot requested (Dev Mode: Simulation)');
      await new Promise(resolve => setTimeout(resolve, 2000)); // Simulate delay
      return res.json({ message: 'Dev Mode: Simulated Server Reboot successful.' });
    }

    // Production Linux Reboot
    // Execute asynchronously with a small delay to allow response to be sent
    // Use -f to force reboot (skips shutdown scripts/stuck processes)
    setTimeout(() => {
      // Without sudo because we are root inside the container
      // and have privileged: true to control the host
      exec('reboot -f', (error, stdout, stderr) => {
        if (error) {
          console.error('CRITICAL: Reboot command failed locally!');
          console.error('Error:', error.message);
          console.error('STDERR:', stderr); 
        } else {
          console.log('Reboot command executed successfully. System should go down.');
        }
      });
    }, 1000);
    
    res.json({ message: 'System is rebooting now...' });
  } catch (error: any) {
    console.error('Reboot error:', error);
    res.status(500).json({ 
      error: 'Failed to initiate reboot', 
      details: error.message || error.stderr || 'Unknown error' 
    });
  }
});

// POST /api/system/reset - Reset Docklift services (OS aware)
router.post('/reset', async (req: Request, res: Response) => {
  try {
    const isWindows = os.platform() === "win32";
    
    if (isWindows) {
      console.log('Reset Service requested (Dev Mode: Simulation)');
      await new Promise(resolve => setTimeout(resolve, 1500));
      return res.json({ message: 'Dev Mode: Simulated Service Reset complete.' });
    }

    // Production: Attempt to restart docker containers
    // Use detached execution to allow response to complete
    const containers = 'docklift-backend docklift-frontend docklift-nginx docklift-nginx-proxy';
    const command = `sleep 1 && docker restart ${containers}`;
    
    console.log(`Reset triggered. Restarting: ${containers}`);
    
    exec(command, (error) => {
      if (error) console.error('Reset execution ended:', error);
    });

    res.json({ message: 'Services reset command sent successfully (Restarting in 1s)' });
  } catch (error: any) {
    console.error('Reset error:', error);
    res.status(500).json({ 
      error: 'Failed to reset services', 
      details: error.message || error.stderr || 'Unknown error'
    });
  }
});

// POST /api/system/execute - Execute a command and return output
router.post('/execute', async (req: Request, res: Response) => {
  const { command } = req.body;
  
  if (!command) {
    return res.status(400).json({ error: 'No command provided' });
  }

  try {
    // Basic security: avoid obviously destructive commands if possible
    const forbidden = ['rm -rf /', ':(){ :|:& };:', 'mv /dev/null'];
    if (forbidden.some(b => command.includes(b))) {
      return res.status(403).json({ error: 'Command forbidden for safety' });
    }

    const { stdout, stderr } = await execAsync(command, { timeout: 30000 });
    res.json({ 
      output: stdout,
      error: stderr || null
    });
  } catch (error: any) {
    res.status(500).json({ 
      error: error.message,
      output: error.stdout || '',
      stderr: error.stderr || ''
    });
  }
});

// ========================================
// Version Check & Upgrade
// ========================================

// Cache for version check (1 hour)
let cachedVersionInfo: { current: string; latest: string; updateAvailable: boolean } | null = null;
let lastVersionCheck = 0;
const VERSION_CACHE_TTL = 3600000; // 1 hour

// Read current version from package.json dynamically
function getCurrentVersion(): string {
  try {
    const packagePath = join(__dirnameSystem, '../../package.json');
    const pkg = JSON.parse(readFileSync(packagePath, 'utf8'));
    return pkg.version || '1.0.0';
  } catch {
    return '1.0.0';
  }
}

// GET /api/system/version - Check for updates
router.get('/version', async (req: Request, res: Response) => {
  try {
    const now = Date.now();
    
    // Return cached result if still valid
    if (cachedVersionInfo && (now - lastVersionCheck) < VERSION_CACHE_TTL) {
      return res.json(cachedVersionInfo);
    }

    // Fetch latest release from GitHub
    const currentVersion = getCurrentVersion();
    let latestVersion = currentVersion;
    try {
      const response = await fetch('https://api.github.com/repos/SSujitX/docklift/releases/latest', {
        headers: { 'User-Agent': 'Docklift' }
      });
      if (response.ok) {
        const data = await response.json() as { tag_name?: string };
        // Remove 'v' prefix if present
        latestVersion = data.tag_name?.replace(/^v/, '') || currentVersion;
      }
    } catch {
      // If GitHub API fails, assume no updates
    }

    // Compare versions
    const currentParts = currentVersion.split('.').map(Number);
    const latestParts = latestVersion.split('.').map(Number);
    
    let updateAvailable = false;
    for (let i = 0; i < 3; i++) {
      if ((latestParts[i] || 0) > (currentParts[i] || 0)) {
        updateAvailable = true;
        break;
      } else if ((latestParts[i] || 0) < (currentParts[i] || 0)) {
        break;
      }
    }

    cachedVersionInfo = {
      current: currentVersion,
      latest: latestVersion,
      updateAvailable
    };
    lastVersionCheck = now;

    res.json(cachedVersionInfo);
  } catch (error: any) {
    res.status(500).json({ error: error.message });
  }
});

// POST /api/system/update-system - Run apt update and upgrade on HOST
router.post('/update-system', async (req: Request, res: Response) => {
  try {
    const isWindows = os.platform() === "win32";
    const isMac = os.platform() === "darwin";

    if (isWindows || isMac) {
      // Dev environment simulation
      console.log('System update requested (Dev Mode: Simulation)');
      await new Promise(resolve => setTimeout(resolve, 2000));
      return res.json({ message: 'Dev Mode: Simulated system update complete.' });
    }

    // Production: Run apt update/upgrade on HOST via nsenter
    // nsenter --target 1 lets us escape the container and run on the host
    const command = 'nsenter --target 1 --mount --uts --ipc --net --pid -- sh -c "apt update -y && DEBIAN_FRONTEND=noninteractive apt upgrade -y && apt autoremove -y"';
    
    console.log('System update initiated on host');
    
    exec(command, { timeout: 900000 }, (error, stdout, stderr) => {
      if (error) {
        console.error('System update error:', error.message);
        console.error('STDERR:', stderr);
      } else {
        console.log('System update completed successfully');
      }
    });

    res.json({ message: 'System update started on host. This may take several minutes.' });
  } catch (error: any) {
    console.error('System update error:', error);
    res.status(500).json({ 
      error: 'Failed to start system update', 
      details: error.message 
    });
  }
});

// POST /api/system/upgrade - Run upgrade script on HOST
router.post('/upgrade', async (req: Request, res: Response) => {
  try {
    const isWindows = os.platform() === "win32";
    const isMac = os.platform() === "darwin";

    if (isWindows || isMac) {
      // Dev environment simulation
      console.log('Docklift upgrade requested (Dev Mode: Simulation)');
      await new Promise(resolve => setTimeout(resolve, 2000));
      return res.json({ message: 'Dev Mode: Simulated upgrade complete. No changes made to system.' });
    }

    // Production: Run upgrade.sh on HOST via nsenter
    // upgrade.sh is at /opt/docklift/upgrade.sh on the host
    // We remove --cgroup as it's not supported by all nsenter versions (e.g. busybox)
    // We use systemd-run to escape the container's cgroup to prevent being killed during restart
    const command = 'nsenter --target 1 --mount --uts --ipc --net --pid -- sh -c "cd /opt/docklift && (systemd-run --unit=docklift-upgrade-$(date +%s) --scope bash upgrade.sh > /dev/null 2>&1 || nohup bash upgrade.sh > /dev/null 2>&1) &"';
    
    console.log('Docklift upgrade initiated on host');
    
    // Set a short timeout because we expect the command to detach immediately
    exec(command, { timeout: 5000 }, (error, stdout, stderr) => {
      if (error && !error.killed) { // Ignore timeout/killed errors as they might happen on quick detach
        console.error('Upgrade initiation error (might be benign if process detached):', error.message);
      } else {
        console.log('Docklift upgrade initiated successfully');
      }
    });

    res.json({ message: 'Docklift upgrade started. The application will restart shortly.' });
  } catch (error: any) {
    console.error('Upgrade error:', error);
    res.status(500).json({ 
      error: 'Failed to start upgrade', 
      details: error.message 
    });
  }
});

export default router;
