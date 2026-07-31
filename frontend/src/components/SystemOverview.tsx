// SystemOverview component - displays CPU, memory, disk, network, GPU stats with live updates

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
import { authFetch } from "@/lib/auth";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
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
      <div className="absolute inset-0 flex flex-col items-center justify-center px-1">
        <span
          className={cn(
            "font-bold tabular-nums",
            size < 110 ? "text-lg" : "text-2xl"
          )}
        >
          {value.toFixed(1)}%
        </span>
        <span className="text-[10px] font-medium text-muted-foreground sm:text-xs">
          {label}
        </span>
        {sublabel && (
          <span className="max-w-full truncate text-[9px] text-muted-foreground/70 sm:text-[10px]">
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
  const [purgePassword, setPurgePassword] = useState("");
  const [processSortBy, setProcessSortBy] = useState<'cpu' | 'mem'>('cpu');

  const fetchStats = useCallback(async () => {
    try {
      const res = await authFetch(`${API_URL}/api/system/stats`);
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
    if (!purgePassword.trim()) {
      toast.error("Enter your account password to confirm purge");
      return;
    }
    setShowPurgeDialog(false);
    setPurging(true);
    try {
      const res = await authFetch(`${API_URL}/api/system/purge`, {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ password: purgePassword }),
      });
      const data = await res.json();
      setPurgePassword("");

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
      <div className="space-y-4 sm:space-y-5">
        <div className="flex items-center gap-2.5 sm:gap-3">
          <div className="h-9 w-9 shrink-0 rounded-xl bg-card border border-border shimmer sm:h-10 sm:w-10 sm:rounded-2xl" />
          <div className="min-w-0 flex-1 space-y-2">
            <div className="h-5 w-28 rounded-md bg-card border border-border shimmer" />
            <div className="h-3 w-48 max-w-full rounded-md bg-card border border-border shimmer" />
          </div>
        </div>
        <div className="grid grid-cols-2 gap-2.5 sm:gap-3 lg:grid-cols-4">
          {[1, 2, 3, 4].map((i) => (
            <div
              key={i}
              className="h-44 rounded-xl bg-card border border-border shimmer sm:h-52 sm:rounded-2xl"
            />
          ))}
        </div>
        <div className="h-64 rounded-xl bg-card border border-border shimmer sm:rounded-2xl" />
      </div>
    );
  }

  if (error || !stats) {
    return (
      <div className="flex flex-col items-center justify-center py-12 sm:py-16">
        <div className="mb-4 rounded-2xl border border-destructive/20 bg-destructive/10 p-4">
          <Server className="h-8 w-8 text-destructive" />
        </div>
        <h3 className="mb-2 text-lg font-semibold">
          Unable to fetch system stats
        </h3>
        <p className="mb-4 text-sm text-muted-foreground">{error}</p>
        <button
          onClick={fetchStats}
          className="flex items-center gap-2 rounded-lg bg-secondary px-4 py-2 text-sm font-medium transition-colors hover:bg-secondary/80"
        >
          <RefreshCw className="h-4 w-4" />
          Retry
        </button>
      </div>
    );
  }

  return (
    <div className="space-y-4 sm:space-y-5">
      {/* Compact title row — matches Operate pages; page still scrolls */}
      <div className="flex items-start justify-between gap-3 sm:items-center">
        <div className="flex min-w-0 items-center gap-2.5 sm:gap-3">
          <span className="flex h-9 w-9 shrink-0 items-center justify-center rounded-xl border border-brand/20 bg-brand/10 sm:h-10 sm:w-10 sm:rounded-2xl">
            <Gauge className="h-4 w-4 text-brand sm:h-[1.125rem] sm:w-[1.125rem]" />
          </span>
          <div className="min-w-0">
            <h1 className="truncate text-lg font-bold tracking-tight sm:text-xl">
              System
            </h1>
            <p className="truncate text-[11px] text-muted-foreground sm:text-xs">
              Live host metrics · Updated{" "}
              {lastUpdate ? lastUpdate.toLocaleTimeString() : "now"}
            </p>
          </div>
        </div>
        <div className="flex shrink-0 items-center gap-1.5 sm:gap-2">
          <button
            onClick={() => setShowPurgeDialog(true)}
            disabled={purging}
            title="Remove unused Docklift images + clear BuildKit cache"
            className={cn(
              "inline-flex h-9 items-center gap-1.5 rounded-xl border px-2.5 text-xs font-bold transition-all active:scale-95 sm:h-10 sm:gap-2 sm:px-3.5",
              purging
                ? "cursor-not-allowed border-rose-500/20 bg-rose-500/10 text-rose-500/30"
                : "border-rose-500/20 bg-background text-rose-500 hover:border-rose-500/40 hover:bg-rose-500/5"
            )}
          >
            {purging ? (
              <RefreshCw className="h-3.5 w-3.5 animate-spin" />
            ) : (
              <Trash2 className="h-3.5 w-3.5" />
            )}
            <span className="hidden sm:inline">Purge</span>
          </button>
          <Button
            variant="outline"
            size="icon"
            onClick={fetchStats}
            title="Refresh stats"
            className="h-9 w-9 shrink-0 border-border/60 bg-background hover:bg-secondary/80 sm:h-10 sm:w-10"
          >
            <RefreshCw className="h-4 w-4 text-muted-foreground" />
          </Button>
        </div>
      </div>

      {/* Main Stats Grid */}
      <div className="grid grid-cols-2 gap-2.5 sm:gap-3 lg:grid-cols-4">
        {/* CPU Card */}
        <div className="rounded-xl border border-border/50 bg-card p-3 shadow-sm sm:rounded-2xl sm:p-5">
          <div className="mb-2.5 flex items-center gap-2 sm:mb-3">
            <div className="rounded-lg bg-cyan-500/10 p-1.5 sm:p-2">
              <Cpu className="h-3.5 w-3.5 text-cyan-500 sm:h-4 sm:w-4" />
            </div>
            <h3 className="text-sm font-semibold sm:text-base">CPU</h3>
          </div>
          <div className="flex justify-center">
            <CircularProgress
              value={stats.cpu.usage}
              label="Usage"
              sublabel={`${stats.cpu.cores} cores`}
              color="cyan"
              size={108}
            />
          </div>
          <div className="mt-3 space-y-1 border-t border-border/50 pt-3 sm:mt-4 sm:space-y-1.5 sm:pt-4">
            <p
              className="truncate text-[10px] text-muted-foreground sm:text-xs"
              title={stats.cpu.model}
            >
              {stats.cpu.model}
            </p>
            <div className="flex justify-between text-[10px] sm:text-xs">
              <span className="text-muted-foreground">Speed</span>
              <span className="font-medium">{stats.cpu.speed} GHz</span>
            </div>
            {stats.cpu.temperature && (
              <div className="flex justify-between text-[10px] sm:text-xs">
                <span className="text-muted-foreground">Temp</span>
                <span className="font-medium">{stats.cpu.temperature}°C</span>
              </div>
            )}
          </div>
        </div>

        {/* Memory Card */}
        <div className="rounded-xl border border-border/50 bg-card p-3 shadow-sm sm:rounded-2xl sm:p-5">
          <div className="mb-2.5 flex items-center gap-2 sm:mb-3">
            <div className="rounded-lg bg-purple-500/10 p-1.5 sm:p-2">
              <CircuitBoard className="h-3.5 w-3.5 text-purple-500 sm:h-4 sm:w-4" />
            </div>
            <h3 className="text-sm font-semibold sm:text-base">Memory</h3>
          </div>
          <div className="flex justify-center">
            <CircularProgress
              value={stats.memory.usedPercent}
              label="Used"
              sublabel={`${formatBytes(stats.memory.used)} / ${formatBytes(
                stats.memory.total
              )}`}
              color="purple"
              size={108}
            />
          </div>
          <div className="mt-3 space-y-1 border-t border-border/50 pt-3 sm:mt-4 sm:space-y-1.5 sm:pt-4">
            <div className="flex justify-between text-[10px] sm:text-xs">
              <span className="text-muted-foreground">Total</span>
              <span className="font-medium">
                {formatBytes(stats.memory.total)}
              </span>
            </div>
            <div className="flex justify-between text-[10px] sm:text-xs">
              <span className="text-muted-foreground">Free</span>
              <span className="font-medium text-emerald-500">
                {formatBytes(stats.memory.free)}
              </span>
            </div>
          </div>
        </div>

        {/* Primary Disk Card */}
        <div className="rounded-xl border border-border/50 bg-card p-3 shadow-sm sm:rounded-2xl sm:p-5">
          <div className="mb-2.5 flex items-center gap-2 sm:mb-3">
            <div className="rounded-lg bg-amber-500/10 p-1.5 sm:p-2">
              <HardDrive className="h-3.5 w-3.5 text-amber-500 sm:h-4 sm:w-4" />
            </div>
            <h3 className="text-sm font-semibold sm:text-base">Storage</h3>
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
                  size={108}
                />
              </div>
              <div className="mt-3 space-y-1 border-t border-border/50 pt-3 sm:mt-4 sm:space-y-1.5 sm:pt-4">
                <div className="flex justify-between text-[10px] sm:text-xs">
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
        <div className="rounded-xl border border-border/50 bg-card p-3 shadow-sm sm:rounded-2xl sm:p-5">
          <div className="mb-2.5 flex items-center gap-2 sm:mb-3">
            <div className="rounded-lg bg-emerald-500/10 p-1.5 sm:p-2">
              <Wifi className="h-3.5 w-3.5 text-emerald-500 sm:h-4 sm:w-4" />
            </div>
            <h3 className="text-sm font-semibold sm:text-base">Network</h3>
          </div>
          <div className="flex justify-center">
            <div className="flex h-[108px] w-[108px] items-center justify-center rounded-full border-4 border-secondary bg-secondary/50">
              <Activity className="h-8 w-8 animate-pulse text-emerald-500 sm:h-9 sm:w-9" />
            </div>
          </div>
          <div className="mt-3 space-y-1.5 border-t border-border/50 pt-3 sm:mt-4 sm:space-y-2 sm:pt-4">
            <div className="flex items-center justify-between text-[10px] sm:text-xs">
              <div className="flex items-center gap-1.5 text-muted-foreground">
                <ArrowDownToLine className="h-3 w-3 text-emerald-500" />
                Down
              </div>
              <span className="font-medium tabular-nums">
                {formatSpeed(stats.network.rxSpeed)}
              </span>
            </div>
            <div className="flex items-center justify-between text-[10px] sm:text-xs">
              <div className="flex items-center gap-1.5 text-muted-foreground">
                <ArrowUpFromLine className="h-3 w-3 text-cyan-500" />
                Up
              </div>
              <span className="font-medium tabular-nums">
                {formatSpeed(stats.network.txSpeed)}
              </span>
            </div>
          </div>
        </div>
      </div>

      {/* Server Details - Full Width Two Column Layout */}
      <div className="rounded-xl border border-border/50 bg-card p-3 shadow-sm sm:rounded-2xl sm:p-5">
        <div className="mb-3 flex flex-wrap items-center justify-between gap-2 sm:mb-4">
          <div className="flex items-center gap-2">
            <div className="rounded-lg bg-rose-500/10 p-1.5 sm:p-2">
              <Server className="h-3.5 w-3.5 text-rose-500 sm:h-4 sm:w-4" />
            </div>
            <h3 className="text-sm font-semibold sm:text-base">Server Details</h3>
          </div>
          <div className="inline-flex items-center gap-1.5 rounded-lg border border-emerald-500/20 bg-gradient-to-r from-emerald-500/10 to-cyan-500/10 px-2.5 py-1 sm:rounded-full sm:px-3 sm:py-1.5">
            <Clock className="h-3.5 w-3.5 text-emerald-600 dark:text-emerald-400" />
            <span className="text-[11px] font-semibold text-emerald-600 dark:text-emerald-400 sm:text-xs">
              {stats.server.uptimeFormatted}
            </span>
          </div>
        </div>
        <div className="grid grid-cols-1 gap-x-8 gap-y-0 text-xs sm:text-sm md:grid-cols-2">
          {/* Left Column */}
          <div className="space-y-0">
            <div className="flex items-center justify-between gap-3 border-b border-border/30 py-2">
              <span className="flex shrink-0 items-center gap-2 text-muted-foreground">
                <Monitor className="h-3.5 w-3.5" /> Hostname
              </span>
              <span className="min-w-0 truncate text-right font-medium">
                {stats.server.hostname}
              </span>
            </div>
            <div className="flex items-center justify-between gap-3 border-b border-border/30 py-2">
              <span className="flex shrink-0 items-center gap-2 text-muted-foreground">
                <Server className="h-3.5 w-3.5" /> OS
              </span>
              <span className="min-w-0 truncate text-right text-[11px] font-medium sm:text-xs">
                {stats.server.distro || stats.server.platform}
              </span>
            </div>
            <div className="flex items-center justify-between gap-3 border-b border-border/30 py-2">
              <span className="flex shrink-0 items-center gap-2 text-muted-foreground">
                <Zap className="h-3.5 w-3.5" /> Kernel
              </span>
              <span className="min-w-0 truncate text-right text-[11px] font-medium sm:text-xs">
                {stats.server.kernel}
              </span>
            </div>
            <div className="flex items-center justify-between gap-3 border-b border-border/30 py-2">
              <span className="flex shrink-0 items-center gap-2 text-muted-foreground">
                <Gauge className="h-3.5 w-3.5" /> Arch
              </span>
              <span className="font-medium">{stats.server.arch}</span>
            </div>
            <div className="flex items-center justify-between gap-3 border-b border-border/30 py-2">
              <span className="flex shrink-0 items-center gap-2 text-muted-foreground">
                <Calendar className="h-3.5 w-3.5" /> Time
              </span>
              <span className="min-w-0 truncate text-right text-[11px] font-medium sm:text-xs">
                {stats.server.serverTime}
              </span>
            </div>
            <div className="flex items-center justify-between gap-3 border-b border-border/30 py-2 md:border-b-0">
              <span className="flex shrink-0 items-center gap-2 text-muted-foreground">
                <Network className="h-3.5 w-3.5" /> Connections
              </span>
              <span className="font-medium tabular-nums">
                {stats.server.activeConnections}
              </span>
            </div>
          </div>

          {/* Right Column */}
          <div className="space-y-0">
            <div className="flex items-center justify-between gap-3 border-b border-border/30 py-2">
              <span className="flex shrink-0 items-center gap-2 text-muted-foreground">
                <Cpu className="h-3.5 w-3.5" /> CPU
              </span>
              <span
                className="min-w-0 truncate text-right text-[11px] font-medium sm:max-w-[200px] sm:text-xs"
                title={stats.server.cpuModel}
              >
                {stats.server.cpuCores}
              </span>
            </div>
            <div className="flex items-center justify-between gap-3 border-b border-border/30 py-2">
              <span className="flex shrink-0 items-center gap-2 text-muted-foreground">
                <Activity className="h-3.5 w-3.5" /> Load
              </span>
              <span className="font-medium tabular-nums">
                <span className="text-cyan-500">
                  {stats.server.loadAvg?.load1 || 0}
                </span>
                <span className="mx-1 text-muted-foreground">|</span>
                <span className="text-amber-500">
                  {stats.server.loadAvg?.load5 || 0}
                </span>
                <span className="mx-1 text-muted-foreground">|</span>
                <span className="text-emerald-500">
                  {stats.server.loadAvg?.load15 || 0}
                </span>
              </span>
            </div>
            {stats.gpu.available && (
              <div className="flex items-center justify-between gap-3 border-b border-border/30 py-2">
                <span className="flex shrink-0 items-center gap-2 text-muted-foreground">
                  <Gauge className="h-3.5 w-3.5" /> GPU
                </span>
                <span
                  className="min-w-0 truncate text-right text-[11px] font-medium sm:max-w-[200px] sm:text-xs"
                  title={stats.gpu.model || ""}
                >
                  {stats.gpu.model}
                </span>
              </div>
            )}
            <div className="flex items-center justify-between gap-3 border-b border-border/30 py-2">
              <span className="flex shrink-0 items-center gap-2 text-muted-foreground">
                <CircuitBoard className="h-3.5 w-3.5" /> Swap
              </span>
              <span className="text-right font-medium tabular-nums">
                {formatBytes(stats.server.swap?.used || 0)} /{" "}
                {formatBytes(stats.server.swap?.total || 0)}
              </span>
            </div>
            <div className="flex items-center justify-between gap-3 border-b border-border/30 py-2">
              <span className="flex shrink-0 items-center gap-2 text-muted-foreground">
                <Globe className="h-3.5 w-3.5" /> IP
              </span>
              <span className="font-medium text-cyan-500 tabular-nums">
                {stats.server.ipAddress}
              </span>
            </div>
            {stats.server.location && stats.server.location !== "N/A" ? (
              <div className="flex items-center justify-between gap-3 py-2">
                <span className="flex shrink-0 items-center gap-2 text-muted-foreground">
                  <MapPin className="h-3.5 w-3.5" /> Location
                </span>
                <span className="min-w-0 truncate text-right font-medium">
                  {stats.server.location}
                </span>
              </div>
            ) : (
              <div className="flex items-center justify-between gap-3 py-2">
                <span className="flex shrink-0 items-center gap-2 text-muted-foreground">
                  <Globe className="h-3.5 w-3.5" /> IP
                </span>
                <span className="font-medium text-cyan-500 tabular-nums">
                  {stats.server.ipAddress}
                </span>
              </div>
            )}
          </div>
        </div>

        {/* Top Processes - Full Width Below */}
        {stats.processes && stats.processes.length > 0 && (
          <div className="mt-3 border-t border-border/50 pt-3 sm:mt-4 sm:pt-4">
            <div className="mb-2.5 flex items-center justify-between gap-2 sm:mb-3">
              <div className="flex items-center gap-2">
                <Activity className="h-3.5 w-3.5 text-blue-500 sm:h-4 sm:w-4" />
                <span className="text-sm font-medium">Top Processes</span>
              </div>
              <div className="flex items-center gap-0.5 rounded-lg bg-secondary/50 p-0.5">
                <button
                  onClick={() => setProcessSortBy("cpu")}
                  className={cn(
                    "rounded-md px-2 py-1 text-[11px] font-medium transition-all sm:text-xs",
                    processSortBy === "cpu"
                      ? "bg-cyan-500/20 text-cyan-500"
                      : "text-muted-foreground hover:text-foreground"
                  )}
                >
                  CPU
                </button>
                <button
                  onClick={() => setProcessSortBy("mem")}
                  className={cn(
                    "rounded-md px-2 py-1 text-[11px] font-medium transition-all sm:text-xs",
                    processSortBy === "mem"
                      ? "bg-purple-500/20 text-purple-500"
                      : "text-muted-foreground hover:text-foreground"
                  )}
                >
                  Mem
                </button>
              </div>
            </div>

            <div className="overflow-x-auto rounded-xl border border-border/50 [-webkit-overflow-scrolling:touch]">
              <table className="w-full min-w-[480px] text-left text-xs sm:min-w-[560px]">
                <thead className="bg-secondary/50 font-medium text-muted-foreground">
                  <tr>
                    <th className="px-2.5 py-2 sm:px-3">PID</th>
                    <th className="px-2.5 py-2 sm:px-3">Process</th>
                    <th className="px-2.5 py-2 sm:px-3">User</th>
                    <th className="px-2.5 py-2 text-right sm:px-3">CPU</th>
                    <th className="px-2.5 py-2 text-right sm:px-3">Memory</th>
                  </tr>
                </thead>
                <tbody className="divide-y divide-border/30 bg-card">
                  {[...stats.processes]
                    .sort((a, b) =>
                      processSortBy === "cpu" ? b.cpu - a.cpu : b.mem - a.mem
                    )
                    .map((proc) => (
                      <tr
                        key={proc.pid}
                        className="transition-colors hover:bg-secondary/30"
                      >
                        <td className="px-2.5 py-2 font-mono text-muted-foreground tabular-nums sm:px-3">
                          {proc.pid}
                        </td>
                        <td
                          className="max-w-[120px] truncate px-2.5 py-2 font-medium sm:max-w-[180px] sm:px-3"
                          title={proc.name}
                        >
                          {proc.name}
                        </td>
                        <td className="px-2.5 py-2 text-muted-foreground sm:px-3">
                          {proc.user}
                        </td>
                        <td className="px-2.5 py-2 text-right font-medium tabular-nums text-cyan-500 sm:px-3">
                          {proc.cpu.toFixed(1)}%
                        </td>
                        <td className="px-2.5 py-2 text-right tabular-nums text-purple-500 sm:px-3">
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
              Purge unused Docklift images
            </DialogTitle>
            <DialogDescription className="text-center text-muted-foreground text-sm">
              Removes unused Docklift images outside each project&apos;s keep-2 set and clears all BuildKit cache. Does not modify the host OS or other containers.
            </DialogDescription>
          </DialogHeader>

          <div className="space-y-2.5 my-4">
            <div className="p-3 rounded-xl bg-secondary/30 border border-border flex items-center gap-3">
              <CheckCircle2 className="h-4 w-4 text-emerald-500" />
              <p className="text-xs font-semibold">
                Unused Docklift images + full BuildKit wipe
              </p>
            </div>
            <div className="p-3 rounded-xl bg-secondary/30 border border-border flex items-center gap-3">
              <CheckCircle2 className="h-4 w-4 text-muted-foreground" />
              <p className="text-xs font-semibold text-muted-foreground">
                Keeps current + previous successful tags per project
              </p>
            </div>
            <div className="p-3 rounded-xl bg-secondary/30 border border-border flex items-center gap-3">
              <CheckCircle2 className="h-4 w-4 text-muted-foreground" />
              <p className="text-xs font-semibold text-muted-foreground">
                No host prune, journal, apt, or /tmp wipe
              </p>
            </div>
          </div>

          <div className="space-y-2 mb-2">
            <label className="text-xs font-semibold text-muted-foreground">Confirm with account password</label>
            <Input
              type="password"
              value={purgePassword}
              onChange={(e) => setPurgePassword(e.target.value)}
              placeholder="Your DockLift password"
              autoComplete="current-password"
              className="rounded-xl"
            />
          </div>

          <DialogFooter className="flex flex-col sm:flex-row gap-2 pt-2">
            <Button
              variant="ghost"
              onClick={() => {
                setShowPurgeDialog(false);
                setPurgePassword("");
              }}
              className="flex-1 rounded-xl font-bold"
            >
              Cancel
            </Button>
            <Button
              onClick={handlePurge}
              disabled={!purgePassword.trim()}
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
