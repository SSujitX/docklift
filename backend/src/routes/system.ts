import { Router, Request, Response } from 'express';
import si from 'systeminformation';

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

// Get comprehensive system stats
async function getSystemStats(): Promise<SystemStats> {
  const now = Date.now();
  
  // Return cached data if still fresh
  if (cachedStats && (now - lastFetch) < CACHE_TTL) {
    return cachedStats;
  }

  try {
    // Fetch connections in background (slow call - cache for 30s)
    // Only run on Linux - Windows netstat causes errors
    if (process.platform !== 'win32' && now - lastConnectionsFetch > CONNECTIONS_CACHE_TTL) {
      si.networkConnections().then(conns => {
        cachedConnections = Array.isArray(conns) ? conns.filter((c: { state?: string }) => c.state === 'ESTABLISHED').length : 0;
        lastConnectionsFetch = Date.now();
      }).catch(() => {});
    }

    // Fetch public IP and location in background (cache for 5 min)
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
      networkInterfaces
    ] = await Promise.all([
      si.cpu(),
      si.currentLoad(),
      // Skip cpuTemperature on Windows (causes errors)
      isWindows ? Promise.resolve({ main: null }) : si.cpuTemperature().catch(() => ({ main: null })),
      si.mem(),
      si.fsSize(),
      si.networkStats(),
      si.osInfo(),
      // Skip graphics on Windows (causes errors)
      isWindows ? Promise.resolve({ controllers: [] }) : si.graphics().catch(() => ({ controllers: [] })),
      si.time(),
      si.networkInterfaces().catch(() => [])
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

    // Process disk data (filter out small/system partitions)
    const diskInfo = diskData
      .filter(d => d.size > 1024 * 1024 * 100) // > 100MB
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
        hostname: osInfo.hostname,
        platform: osInfo.platform,
        distro: osInfo.distro,
        kernel: osInfo.kernel,
        arch: osInfo.arch,
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
      timestamp: new Date().toISOString()
    };

    // Update cache
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

export default router;
