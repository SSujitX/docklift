// SystemOverview component - displays CPU, memory, disk, network, GPU stats with live updates
"use client";

import { useEffect, useState, useCallback } from "react";
import {
  Cpu,
  HardDrive,
  Wifi,
  Server,
  Activity,
  RefreshCw,
  Gauge,
  CheckCircle2,
  CircuitBoard,
  ArrowDownToLine,
  ArrowUpFromLine,
  Clock,
  Monitor,
  Globe,
  Zap,
  Calendar,
  Network,
  MapPin,
  Trash2,
} from "lucide-react";
import { toast } from "sonner";
import { API_URL, cn } from "@/lib/utils";
import { getAuthHeaders } from "@/lib/auth";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { Button } from "@/components/ui/button";

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

function formatBytes(bytes: number): string {
  if (bytes === 0) return "0 B";
  const k = 1024;
  const sizes = ["B", "KB", "MB", "GB", "TB"];
  const i = Math.floor(Math.log(bytes) / Math.log(k));
  return parseFloat((bytes / Math.pow(k, i)).toFixed(1)) + " " + sizes[i];
}

function formatSpeed(bytesPerSec: number): string {
  if (bytesPerSec === 0) return "0 B/s";
  const k = 1024;
  const sizes = ["B/s", "KB/s", "MB/s", "GB/s"];
  const i = Math.floor(Math.log(bytesPerSec) / Math.log(k));
  return parseFloat((bytesPerSec / Math.pow(k, i)).toFixed(1)) + " " + sizes[i];
}

// Circular Progress Component
function CircularProgress({
  value,
  size = 120,
  strokeWidth = 10,
  label,
  sublabel,
  color = "cyan",
}: {
  value: number;
  size?: number;
  strokeWidth?: number;
  label: string;
  sublabel?: string;
  color?: "cyan" | "purple" | "amber" | "emerald" | "rose";
}) {
  const radius = (size - strokeWidth) / 2;
  const circumference = radius * 2 * Math.PI;
  const offset = circumference - (value / 100) * circumference;

  const colorClasses = {
    cyan: "stroke-cyan-500",
    purple: "stroke-purple-500",
    amber: "stroke-amber-500",
    emerald: "stroke-emerald-500",
    rose: "stroke-rose-500",
  };

  const gradientIds = {
    cyan: "gradient-cyan",
    purple: "gradient-purple",
    amber: "gradient-amber",
    emerald: "gradient-emerald",
    rose: "gradient-rose",
  };

  return (
    <div className="relative flex flex-col items-center">
      <svg width={size} height={size} className="transform -rotate-90">
        <defs>
          <linearGradient
            id="gradient-cyan"
            x1="0%"
            y1="0%"
            x2="100%"
            y2="100%"
          >
            <stop offset="0%" stopColor="#06b6d4" />
            <stop offset="100%" stopColor="#3b82f6" />
          </linearGradient>
          <linearGradient
            id="gradient-purple"
            x1="0%"
            y1="0%"
            x2="100%"
            y2="100%"
          >
            <stop offset="0%" stopColor="#a855f7" />
            <stop offset="100%" stopColor="#6366f1" />
          </linearGradient>
          <linearGradient
            id="gradient-amber"
            x1="0%"
            y1="0%"
            x2="100%"
            y2="100%"
          >
            <stop offset="0%" stopColor="#f59e0b" />
            <stop offset="100%" stopColor="#ef4444" />
          </linearGradient>
          <linearGradient
            id="gradient-emerald"
            x1="0%"
            y1="0%"
            x2="100%"
            y2="100%"
          >
            <stop offset="0%" stopColor="#10b981" />
            <stop offset="100%" stopColor="#14b8a6" />
          </linearGradient>
          <linearGradient
            id="gradient-rose"
            x1="0%"
            y1="0%"
            x2="100%"
            y2="100%"
          >
            <stop offset="0%" stopColor="#f43f5e" />
            <stop offset="100%" stopColor="#ec4899" />
          </linearGradient>
        </defs>
        {/* Background circle */}
        <circle
          cx={size / 2}
          cy={size / 2}
          r={radius}
          fill="none"
          stroke="currentColor"
          strokeWidth={strokeWidth}
          className="text-secondary"
        />
        {/* Progress circle */}
        <circle
          cx={size / 2}
          cy={size / 2}
          r={radius}
          fill="none"
          stroke={`url(#${gradientIds[color]})`}
          strokeWidth={strokeWidth}
          strokeLinecap="round"
          strokeDasharray={circumference}
          strokeDashoffset={offset}
          className="transition-all duration-500 ease-out"
        />
      </svg>
      <div className="absolute inset-0 flex flex-col items-center justify-center">
        <span className="text-2xl font-bold">{value.toFixed(1)}%</span>
        <span className="text-xs text-muted-foreground font-medium">
          {label}
        </span>
        {sublabel && (
          <span className="text-[10px] text-muted-foreground/70">
            {sublabel}
          </span>
        )}
      </div>
    </div>
  );
}

// Stat Card Component
function StatCard({
  icon: Icon,
  label,
  value,
  sublabel,
  iconColor = "text-cyan-500",
}: {
  icon: React.ElementType;
  label: string;
  value: string;
  sublabel?: string;
  iconColor?: string;
}) {
  return (
    <div className="flex items-center gap-3 p-3 rounded-xl bg-secondary/30 border border-border/50">
      <div className={`p-2 rounded-lg bg-secondary ${iconColor}`}>
        <Icon className="h-4 w-4" />
      </div>
      <div className="flex-1 min-w-0">
        <p className="text-xs text-muted-foreground font-medium">{label}</p>
        <p className="text-sm font-semibold truncate">{value}</p>
        {sublabel && (
          <p className="text-[10px] text-muted-foreground/70">{sublabel}</p>
        )}
      </div>
    </div>
  );
}

export function SystemOverview() {
  const [stats, setStats] = useState<SystemStats | null>(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [lastUpdate, setLastUpdate] = useState<Date | null>(null);
  const [purging, setPurging] = useState(false);
  const [showPurgeDialog, setShowPurgeDialog] = useState(false);

  const fetchStats = useCallback(async () => {
    try {
      const res = await fetch(`${API_URL}/api/system/stats`, {
        headers: getAuthHeaders(),
      });
      const data = await res.json();
      setStats(data);
      setLastUpdate(new Date());
    } catch (err) {
      setError("Failed to fetch system statistics");
      console.error(err);
    } finally {
      setLoading(false);
    }
  }, []);

  const handlePurge = async () => {
    setShowPurgeDialog(false);
    setPurging(true);
    try {
      const res = await fetch(`${API_URL}/api/system/purge`, {
        method: "POST",
        headers: getAuthHeaders(),
      });
      const data = await res.json();

      if (!res.ok) throw new Error(data.error || "Purge failed");

      // Show detailed success message with memory savings if available
      let message = "Server resources purged successfully!";
      if (data.memorySaved && data.memorySaved !== "0%") {
        message += ` Memory reduced: ${data.memoryBefore}% → ${data.memoryAfter}% (${data.memorySaved} saved)`;
      }
      
      toast.success(message, {
        description: data.details?.join(" • ") || undefined,
        duration: 5000,
      });
      
      fetchStats();
    } catch (err: any) {
      toast.error(err.message || "Failed to purge server");
      console.error(err);
    } finally {
      setPurging(false);
    }
  };

  useEffect(() => {
    fetchStats();
    const interval = setInterval(fetchStats, 3000);
    return () => clearInterval(interval);
  }, [fetchStats]);

  if (loading) {
    return (
      <div className="space-y-6">
        <div className="grid grid-cols-1 md:grid-cols-3 gap-6">
          {[1, 2, 3].map((i) => (
            <div
              key={i}
              className="h-52 rounded-2xl bg-card border border-border shimmer"
            />
          ))}
        </div>
      </div>
    );
  }

  if (error || !stats) {
    return (
      <div className="flex flex-col items-center justify-center py-16">
        <div className="p-4 rounded-2xl bg-destructive/10 border border-destructive/20 mb-4">
          <Server className="h-8 w-8 text-destructive" />
        </div>
        <h3 className="text-lg font-semibold mb-2">
          Unable to fetch system stats
        </h3>
        <p className="text-muted-foreground text-sm mb-4">{error}</p>
        <button
          onClick={fetchStats}
          className="px-4 py-2 rounded-lg bg-secondary hover:bg-secondary/80 text-sm font-medium flex items-center gap-2 transition-colors"
        >
          <RefreshCw className="h-4 w-4" />
          Retry
        </button>
      </div>
    );
  }

  return (
    <div className="space-y-6">
      {/* Header */}
      <div className="flex flex-col sm:flex-row sm:items-center justify-between gap-4">
        <div>
          <h2 className="text-2xl font-bold tracking-tight">System Overview</h2>
          <p className="text-muted-foreground text-sm mt-1">
            Real-time server statistics · Updated{" "}
            {lastUpdate ? lastUpdate.toLocaleTimeString() : "now"}
          </p>
        </div>
        <div className="flex items-center gap-3 self-start sm:self-auto">
            <button
              onClick={() => setShowPurgeDialog(true)}
              disabled={purging}
              className={cn(
                "flex items-center gap-2 px-4 py-2 rounded-xl border text-xs font-bold transition-all active:scale-95 shadow-sm",
                purging
                  ? "bg-rose-500/10 text-rose-500/30 border-rose-500/20 cursor-not-allowed"
                  : "bg-background hover:bg-rose-500/5 text-rose-500 border-rose-500/20 hover:border-rose-500/40"
              )}
            >
              {purging ? (
                <RefreshCw className="h-3.5 w-3.5 animate-spin" />
              ) : (
                <Trash2 className="h-3.5 w-3.5" />
              )}
              Purge Server
            </button>

            <button
              onClick={fetchStats}
              className="p-2.5 rounded-xl bg-secondary/50 hover:bg-secondary border border-border/50 transition-all active:scale-95 text-muted-foreground hover:text-foreground"
            >
              <RefreshCw className="h-4 w-4" />
            </button>
        </div>
      </div>

      {/* Main Stats Grid */}
      <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-4 gap-4">
        {/* CPU Card */}
        <div className="p-6 rounded-2xl bg-card border border-border/50 shadow-sm">
          <div className="flex items-center gap-2 mb-4">
            <div className="p-2 rounded-lg bg-cyan-500/10">
              <Cpu className="h-4 w-4 text-cyan-500" />
            </div>
            <h3 className="font-semibold">CPU</h3>
          </div>
          <div className="flex justify-center">
            <CircularProgress
              value={stats.cpu.usage}
              label="Usage"
              sublabel={`${stats.cpu.cores} cores`}
              color="cyan"
            />
          </div>
          <div className="mt-4 pt-4 border-t border-border/50 space-y-1.5">
            <p
              className="text-xs text-muted-foreground truncate"
              title={stats.cpu.model}
            >
              {stats.cpu.model}
            </p>
            <div className="flex justify-between text-xs">
              <span className="text-muted-foreground">Speed</span>
              <span className="font-medium">{stats.cpu.speed} GHz</span>
            </div>
            {stats.cpu.temperature && (
              <div className="flex justify-between text-xs">
                <span className="text-muted-foreground">Temp</span>
                <span className="font-medium">{stats.cpu.temperature}°C</span>
              </div>
            )}
          </div>
        </div>

        {/* Memory Card */}
        <div className="p-6 rounded-2xl bg-card border border-border/50 shadow-sm">
          <div className="flex items-center gap-2 mb-4">
            <div className="p-2 rounded-lg bg-purple-500/10">
              <CircuitBoard className="h-4 w-4 text-purple-500" />
            </div>
            <h3 className="font-semibold">Memory</h3>
          </div>
          <div className="flex justify-center">
            <CircularProgress
              value={stats.memory.usedPercent}
              label="Used"
              sublabel={`${formatBytes(stats.memory.used)} / ${formatBytes(
                stats.memory.total
              )}`}
              color="purple"
            />
          </div>
          <div className="mt-4 pt-4 border-t border-border/50 space-y-1.5">
            <div className="flex justify-between text-xs">
              <span className="text-muted-foreground">Total</span>
              <span className="font-medium">
                {formatBytes(stats.memory.total)}
              </span>
            </div>
            <div className="flex justify-between text-xs">
              <span className="text-muted-foreground">Free</span>
              <span className="font-medium text-emerald-500">
                {formatBytes(stats.memory.free)}
              </span>
            </div>
          </div>
        </div>

        {/* Primary Disk Card */}
        <div className="p-6 rounded-2xl bg-card border border-border/50 shadow-sm">
          <div className="flex items-center gap-2 mb-4">
            <div className="p-2 rounded-lg bg-amber-500/10">
              <HardDrive className="h-4 w-4 text-amber-500" />
            </div>
            <h3 className="font-semibold">Storage</h3>
          </div>
          {stats.disk[0] && (
            <>
              <div className="flex justify-center">
                <CircularProgress
                  value={stats.disk[0].usedPercent}
                  label={stats.disk[0].mount}
                  sublabel={`${formatBytes(stats.disk[0].used)} / ${formatBytes(
                    stats.disk[0].total
                  )}`}
                  color="amber"
                />
              </div>
              <div className="mt-4 pt-4 border-t border-border/50 space-y-1.5">
                <div className="flex justify-between text-xs">
                  <span className="text-muted-foreground">Type</span>
                  <span className="font-medium">
                    {stats.disk[0].type || "Unknown"}
                  </span>
                </div>
                {stats.disk.length > 1 && (
                  <p className="text-[10px] text-muted-foreground">
                    +{stats.disk.length - 1} more disk(s)
                  </p>
                )}
              </div>
            </>
          )}
        </div>

        {/* Network Card */}
        <div className="p-6 rounded-2xl bg-card border border-border/50 shadow-sm">
          <div className="flex items-center gap-2 mb-4">
            <div className="p-2 rounded-lg bg-emerald-500/10">
              <Wifi className="h-4 w-4 text-emerald-500" />
            </div>
            <h3 className="font-semibold">Network</h3>
          </div>
          <div className="flex justify-center">
            <div className="w-[120px] h-[120px] rounded-full bg-secondary/50 border-4 border-secondary flex items-center justify-center">
              <Activity className="h-10 w-10 text-emerald-500 animate-pulse" />
            </div>
          </div>
          <div className="mt-4 pt-4 border-t border-border/50 space-y-2">
            <div className="flex items-center justify-between text-xs">
              <div className="flex items-center gap-1.5 text-muted-foreground">
                <ArrowDownToLine className="h-3 w-3 text-emerald-500" />
                Download
              </div>
              <span className="font-medium">
                {formatSpeed(stats.network.rxSpeed)}
              </span>
            </div>
            <div className="flex items-center justify-between text-xs">
              <div className="flex items-center gap-1.5 text-muted-foreground">
                <ArrowUpFromLine className="h-3 w-3 text-cyan-500" />
                Upload
              </div>
              <span className="font-medium">
                {formatSpeed(stats.network.txSpeed)}
              </span>
            </div>
          </div>
        </div>
      </div>

      {/* Server Details - Full Width Two Column Layout */}
      <div className="p-6 rounded-2xl bg-card border border-border/50 shadow-sm">
        <div className="flex items-center justify-between mb-4">
          <div className="flex items-center gap-2">
            <div className="p-2 rounded-lg bg-rose-500/10">
              <Server className="h-4 w-4 text-rose-500" />
            </div>
            <h3 className="font-semibold">Server Details</h3>
          </div>
          <div className="flex items-center gap-2 px-3 py-1.5 rounded-full bg-gradient-to-r from-emerald-500/10 to-cyan-500/10 border border-emerald-500/20">
            <Clock className="h-3.5 w-3.5" />
            <span className="text-xs font-semibold text-emerald-600 dark:text-emerald-400">
              Uptime: {stats.server.uptimeFormatted}
            </span>
          </div>
        </div>
        <div className="grid grid-cols-1 md:grid-cols-2 gap-x-8 gap-y-0 text-sm">
          {/* Left Column */}
          <div className="space-y-0">
            <div className="flex justify-between py-2 border-b border-border/30">
              <span className="text-muted-foreground flex items-center gap-2">
                <Monitor className="h-3.5 w-3.5" /> Hostname
              </span>
              <span className="font-medium">{stats.server.hostname}</span>
            </div>
            <div className="flex justify-between py-2 border-b border-border/30">
              <span className="text-muted-foreground flex items-center gap-2">
                <Server className="h-3.5 w-3.5" /> Operating System
              </span>
              <span className="font-medium text-xs">
                {stats.server.distro || stats.server.platform}
              </span>
            </div>
            <div className="flex justify-between py-2 border-b border-border/30">
              <span className="text-muted-foreground flex items-center gap-2">
                <Zap className="h-3.5 w-3.5" /> Kernel
              </span>
              <span className="font-medium text-xs">{stats.server.kernel}</span>
            </div>
            <div className="flex justify-between py-2 border-b border-border/30">
              <span className="text-muted-foreground flex items-center gap-2">
                <Gauge className="h-3.5 w-3.5" /> Architecture
              </span>
              <span className="font-medium">{stats.server.arch}</span>
            </div>
            <div className="flex justify-between py-2 border-b border-border/30">
              <span className="text-muted-foreground flex items-center gap-2">
                <Calendar className="h-3.5 w-3.5" /> Server Time
              </span>
              <span className="font-medium text-xs">
                {stats.server.serverTime}
              </span>
            </div>
            <div className="flex justify-between py-2 border-b border-border/30 md:border-b-0">
              <span className="text-muted-foreground flex items-center gap-2">
                <Network className="h-3.5 w-3.5" /> Active Connections
              </span>
              <span className="font-medium">
                {stats.server.activeConnections}
              </span>
            </div>
          </div>

          {/* Right Column */}
          <div className="space-y-0">
            <div className="flex justify-between py-2 border-b border-border/30">
              <span className="text-muted-foreground flex items-center gap-2">
                <Cpu className="h-3.5 w-3.5" /> CPU
              </span>
              <span
                className="font-medium text-xs truncate max-w-[200px]"
                title={stats.server.cpuModel}
              >
                {stats.server.cpuCores}
              </span>
            </div>
            <div className="flex justify-between py-2 border-b border-border/30">
              <span className="text-muted-foreground flex items-center gap-2">
                <Activity className="h-3.5 w-3.5" /> Load
              </span>
              <span className="font-medium">
                <span className="text-cyan-500">
                  {stats.server.loadAvg?.load1 || 0}
                </span>
                <span className="text-muted-foreground mx-1">|</span>
                <span className="text-amber-500">
                  {stats.server.loadAvg?.load5 || 0}
                </span>
                <span className="text-muted-foreground mx-1">|</span>
                <span className="text-emerald-500">
                  {stats.server.loadAvg?.load15 || 0}
                </span>
              </span>
            </div>
            {stats.gpu.available && (
              <div className="flex justify-between py-2 border-b border-border/30">
                <span className="text-muted-foreground flex items-center gap-2">
                  <Gauge className="h-3.5 w-3.5" /> GPU
                </span>
                <span
                  className="font-medium text-xs truncate max-w-[200px]"
                  title={stats.gpu.model || ""}
                >
                  {stats.gpu.model}
                </span>
              </div>
            )}
            <div className="flex justify-between py-2 border-b border-border/30">
              <span className="text-muted-foreground flex items-center gap-2">
                <CircuitBoard className="h-3.5 w-3.5" /> Swap
              </span>
              <span className="font-medium">
                {formatBytes(stats.server.swap?.used || 0)} /{" "}
                {formatBytes(stats.server.swap?.total || 0)}
              </span>
            </div>
            <div className="flex justify-between py-2 border-b border-border/30">
              <span className="text-muted-foreground flex items-center gap-2">
                <Globe className="h-3.5 w-3.5" /> IP Address
              </span>
              <span className="font-medium text-cyan-500">
                {stats.server.ipAddress}
              </span>
            </div>
            {stats.server.location && stats.server.location !== "N/A" ? (
              <div className="flex justify-between py-2">
                <span className="text-muted-foreground flex items-center gap-2">
                  <MapPin className="h-3.5 w-3.5" /> Location
                </span>
                <span className="font-medium">{stats.server.location}</span>
              </div>
            ) : (
              <div className="flex justify-between py-2">
                <span className="text-muted-foreground flex items-center gap-2">
                  <Globe className="h-3.5 w-3.5" /> IP Address
                </span>
                <span className="font-medium text-cyan-500">
                  {stats.server.ipAddress}
                </span>
              </div>
            )}
          </div>
        </div>

        {/* Top Processes - Full Width Below */}
        {stats.processes && stats.processes.length > 0 && (
          <div className="mt-4 pt-4 border-t border-border/50">
            <div className="flex items-center gap-2 mb-3">
              <Activity className="h-4 w-4 text-blue-500" />
              <span className="font-medium text-sm">
                Top Processes (by CPU)
              </span>
            </div>

            <div className="overflow-x-auto rounded-xl border border-border/50">
              <table className="w-full text-xs text-left min-w-[600px]">
                <thead className="bg-secondary/50 text-muted-foreground font-medium">
                  <tr>
                    <th className="px-3 py-2">PID</th>
                    <th className="px-3 py-2">Process</th>
                    <th className="px-3 py-2">User</th>
                    <th className="px-3 py-2 text-right">CPU</th>
                    <th className="px-3 py-2 text-right">Memory</th>
                  </tr>
                </thead>
                <tbody className="divide-y divide-border/30 bg-card">
                  {stats.processes.map((proc) => (
                    <tr
                      key={proc.pid}
                      className="hover:bg-secondary/30 transition-colors"
                    >
                      <td className="px-3 py-2 font-mono text-muted-foreground">
                        {proc.pid}
                      </td>
                      <td
                        className="px-3 py-2 font-medium truncate max-w-[150px]"
                        title={proc.name}
                      >
                        {proc.name}
                      </td>
                      <td className="px-3 py-2 text-muted-foreground">
                        {proc.user}
                      </td>
                      <td className="px-3 py-2 text-right font-medium text-cyan-500">
                        {proc.cpu.toFixed(1)}%
                      </td>
                      <td className="px-3 py-2 text-right text-purple-500">
                        {proc.mem.toFixed(1)}%
                      </td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>
          </div>
        )}
      </div>

      {/* Purge Confirmation Dialog */}
      <Dialog open={showPurgeDialog} onOpenChange={setShowPurgeDialog}>
        <DialogContent className="sm:max-w-md bg-background border-border shadow-2xl">
          <DialogHeader className="space-y-3">
            <div className="mx-auto w-12 h-12 rounded-2xl bg-rose-500/10 flex items-center justify-center mb-2">
              <Trash2 className="h-6 w-6 text-rose-500" />
            </div>
            <DialogTitle className="text-center text-xl font-bold tracking-tight">
              Purge Server Resources
            </DialogTitle>
            <DialogDescription className="text-center text-muted-foreground text-sm">
              Aggressive cleanup to truly free CPU and RAM. User containers will restart briefly.
            </DialogDescription>
          </DialogHeader>

          <div className="space-y-2.5 my-4">
            <div className="p-3 rounded-xl bg-secondary/30 border border-border flex items-center gap-3">
              <CheckCircle2 className="h-4 w-4 text-emerald-500" />
              <p className="text-xs font-semibold">
                Aggressive Docker cleanup (unused images, networks)
              </p>
            </div>
            <div className="p-3 rounded-xl bg-secondary/30 border border-border flex items-center gap-3">
              <CheckCircle2 className="h-4 w-4 text-emerald-500" />
              <p className="text-xs font-semibold">
                Restart user containers to free memory (Docklift safe)
              </p>
            </div>
            <div className="p-3 rounded-xl bg-secondary/30 border border-border flex items-center gap-3">
              <CheckCircle2 className="h-4 w-4 text-emerald-500" />
              <p className="text-xs font-semibold">
                Clear swap if safe (30%+ RAM free required)
              </p>
            </div>
          </div>

          <DialogFooter className="flex flex-col sm:flex-row gap-2 pt-2">
            <Button
              variant="ghost"
              onClick={() => setShowPurgeDialog(false)}
              className="flex-1 rounded-xl font-bold"
            >
              Cancel
            </Button>
            <Button
              onClick={handlePurge}
              className="flex-1 rounded-xl bg-rose-500 hover:bg-rose-600 text-white font-bold shadow-lg shadow-rose-500/20"
            >
              Start Purge
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </div>
  );
}
