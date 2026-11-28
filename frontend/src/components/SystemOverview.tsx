// SystemOverview component - displays CPU, memory, disk, network, GPU stats with live updates

import { useEffect, useState, useCallback, type ReactNode } from "react";
import { ArrowDown, ArrowUp, RefreshCw, Trash2 } from "lucide-react";
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
import { Card, CardTitle } from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import { PageHeader } from "@/components/shell/PageHeader";
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

// ─── Presentation helpers ────────────────────────────────────────────────────

/** Meters are ink until they cross a threshold: warn at 75%, danger at 90%. */
function meterFill(percent: number) {
  if (percent >= 90) return "bg-danger";
  if (percent >= 75) return "bg-warn";
  return "bg-foreground/70";
}

function Meter({ value, className }: { value: number; className?: string }) {
  const pct = Math.max(0, Math.min(100, Number.isFinite(value) ? value : 0));
  return (
    <div
      aria-hidden="true"
      className={cn("h-1 overflow-hidden rounded-full bg-secondary", className)}
    >
      <div
        className={cn(
          "h-full rounded-full transition-[width,background-color] duration-300 ease-out motion-reduce:transition-none",
          meterFill(pct),
        )}
        style={{ width: `${pct}%` }}
      />
    </div>
  );
}

/** "184.3 KB/s" -> ["184.3", "KB/s"] so the number can carry the figure size. */
function splitUnit(formatted: string): [string, string] {
  const at = formatted.lastIndexOf(" ");
  return at === -1 ? [formatted, ""] : [formatted.slice(0, at), formatted.slice(at + 1)];
}

/** One cell of the Host strip. */
function Metric({
  label,
  aside,
  asideTitle,
  children,
}: {
  label: string;
  aside?: ReactNode;
  asideTitle?: string;
  children: ReactNode;
}) {
  return (
    <section className="flex min-w-0 flex-col bg-card p-4 sm:p-5">
      <div className="flex items-baseline justify-between gap-3">
        <h3 className="text-xs leading-4 text-muted-foreground">{label}</h3>
        {aside != null && (
          <span
            className="min-w-0 truncate font-mono text-xs leading-4 text-subtle"
            title={asideTitle}
          >
            {aside}
          </span>
        )}
      </div>
      {children}
    </section>
  );
}

function Figure({ value, unit, className }: { value: string; unit?: string; className?: string }) {
  return (
    <p
      className={cn(
        "font-mono text-[28px] font-medium leading-8 text-foreground tabular-nums",
        className,
      )}
    >
      {value}
      {unit && (
        <span className="ml-0.5 text-[15px] font-normal text-muted-foreground">{unit}</span>
      )}
    </p>
  );
}

/** `label … value` lines under a figure. */
function Line({
  label,
  value,
  title,
  mono = true,
}: {
  label: string;
  value: ReactNode;
  title?: string;
  mono?: boolean;
}) {
  return (
    <div className="flex items-baseline justify-between gap-3">
      <dt className="shrink-0 text-muted-foreground">{label}</dt>
      <dd
        className={cn("min-w-0 truncate text-right text-foreground", mono && "font-mono")}
        title={title}
      >
        {value}
      </dd>
    </div>
  );
}

function Lines({ children }: { children: ReactNode }) {
  return <dl className="mt-4 space-y-1.5 text-[13px] leading-5">{children}</dl>;
}

/** One row of the server definition list. */
function Detail({
  label,
  title,
  mono = true,
  children,
}: {
  label: string;
  title?: string;
  mono?: boolean;
  children: ReactNode;
}) {
  return (
    <div className="grid grid-cols-[7.5rem_minmax(0,1fr)] items-baseline gap-3 py-2 text-[13px] leading-5 sm:grid-cols-[8.5rem_minmax(0,1fr)]">
      <dt className="text-muted-foreground">{label}</dt>
      <dd className={cn("min-w-0 break-words text-foreground", mono && "font-mono")} title={title}>
        {children}
      </dd>
    </div>
  );
}

const SEGMENT =
  "inline-flex h-7 items-center rounded-[4px] px-2.5 text-[13px] font-medium transition-colors duration-150 pointer-coarse:h-9";

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
      let message = "Server resources purged successfully.";
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

  const description = "Live metrics for this host, refreshed every 3 seconds.";

  if (loading) {
    return (
      <div>
        <PageHeader title="System" description={description} />
        <Card className="overflow-hidden">
          <div className="h-12 border-b border-border" />
          <div className="grid grid-cols-2 gap-px bg-border lg:grid-cols-4">
            {[1, 2, 3, 4].map((i) => (
              <div key={i} className="bg-card p-4 sm:p-5">
                <div className="h-3 w-12 rounded-sm bg-secondary" />
                <div className="mt-3 h-7 w-20 rounded-sm bg-secondary" />
                <div className="mt-4 h-1 rounded-full bg-secondary" />
                <div className="mt-5 space-y-2.5">
                  <div className="h-3 rounded-sm bg-secondary" />
                  <div className="h-3 w-4/5 rounded-sm bg-secondary" />
                  <div className="h-3 w-3/5 rounded-sm bg-secondary" />
                </div>
              </div>
            ))}
          </div>
        </Card>
      </div>
    );
  }

  if (error || !stats) {
    return (
      <div>
        <PageHeader title="System" description={description} />
        <Card>
          <div className="px-4 py-14 text-center">
            <h3 className="text-[15px] font-semibold">Unable to fetch system stats</h3>
            <p className="mx-auto mt-1 max-w-sm text-[13.5px] text-muted-foreground">
              {error}
            </p>
            <Button variant="outline" onClick={fetchStats} className="mt-5">
              <RefreshCw />
              Retry
            </Button>
          </div>
        </Card>
      </div>
    );
  }

  const primaryDisk = stats.disk[0];
  const [rxValue, rxUnit] = splitUnit(formatSpeed(stats.network.rxSpeed));
  const [txValue, txUnit] = splitUnit(formatSpeed(stats.network.txSpeed));
  const cores = stats.cpu.cores || 0;
  // Load average is a run-queue length: at or above the core count the CPU is
  // saturated (warn); at twice the core count work is queueing badly (danger).
  const loadTone = (load: number) =>
    cores > 0 && load >= cores * 2
      ? "text-danger"
      : cores > 0 && load >= cores
        ? "text-warn"
        : undefined;
  const loads = [
    stats.server.loadAvg?.load1 || 0,
    stats.server.loadAvg?.load5 || 0,
    stats.server.loadAvg?.load15 || 0,
  ];
  const hasLocation = Boolean(stats.server.location && stats.server.location !== "N/A");

  return (
    <div className="space-y-4 sm:space-y-6">
      <PageHeader
        title="System"
        description={description}
        className="mb-0 sm:mb-0"
        actions={
          <>
            <Button
              variant="outline"
              size="icon"
              onClick={fetchStats}
              title="Refresh stats"
            >
              <RefreshCw className="text-muted-foreground" />
            </Button>
            <Button
              variant="outline"
              onClick={() => setShowPurgeDialog(true)}
              disabled={purging}
              title="Remove unused Docklift images + clear BuildKit cache"
              className="text-danger hover:text-danger"
            >
              {purging ? <RefreshCw className="animate-spin" /> : <Trash2 />}
              Purge
            </Button>
          </>
        }
      />

      {/* Host: one panel, four metrics divided by hairlines */}
      <Card className="overflow-hidden">
        <div className="flex items-center justify-between gap-3 border-b border-border px-4 py-3 sm:px-5">
          <CardTitle>Host</CardTitle>
          <span className="font-mono text-xs text-subtle">
            Updated {lastUpdate ? lastUpdate.toLocaleTimeString() : "now"}
          </span>
        </div>

        <div className="grid grid-cols-2 gap-px bg-border lg:grid-cols-4">
          <Metric label="CPU" aside={`${stats.cpu.cores} cores`}>
            <Figure value={stats.cpu.usage.toFixed(1)} unit="%" className="mt-2" />
            <Meter value={stats.cpu.usage} className="mt-3" />
            <Lines>
              <Line label="Model" value={stats.cpu.model} title={stats.cpu.model} mono={false} />
              <Line label="Speed" value={`${stats.cpu.speed} GHz`} />
              {stats.cpu.temperature && (
                <Line label="Temp" value={`${stats.cpu.temperature}°C`} />
              )}
            </Lines>
          </Metric>

          <Metric label="Memory" aside={formatBytes(stats.memory.total)}>
            <Figure value={stats.memory.usedPercent.toFixed(1)} unit="%" className="mt-2" />
            <Meter value={stats.memory.usedPercent} className="mt-3" />
            <Lines>
              <Line label="Used" value={formatBytes(stats.memory.used)} />
              <Line label="Free" value={formatBytes(stats.memory.free)} />
              <Line label="Total" value={formatBytes(stats.memory.total)} />
            </Lines>
          </Metric>

          <Metric
            label="Storage"
            aside={primaryDisk?.mount}
            asideTitle={primaryDisk ? `Mount point ${primaryDisk.mount}` : undefined}
          >
            {primaryDisk ? (
              <>
                <Figure value={primaryDisk.usedPercent.toFixed(1)} unit="%" className="mt-2" />
                <Meter value={primaryDisk.usedPercent} className="mt-3" />
                <Lines>
                  <Line label="Used" value={formatBytes(primaryDisk.used)} />
                  <Line label="Total" value={formatBytes(primaryDisk.total)} />
                  <Line label="Type" value={primaryDisk.type || "Unknown"} />
                </Lines>
              </>
            ) : (
              <p className="mt-2 text-[13px] text-muted-foreground">No disks reported.</p>
            )}
          </Metric>

          <Metric label="Network">
            <div className="mt-2 flex flex-wrap items-baseline gap-x-4">
              <div className="flex items-baseline">
                <ArrowDown className="size-3.5 shrink-0 self-center text-subtle" />
                <span className="sr-only">Down</span>
                <Figure value={rxValue} unit={rxUnit} className="ml-1" />
              </div>
              <div className="flex items-baseline">
                <ArrowUp className="size-3.5 shrink-0 self-center text-subtle" />
                <span className="sr-only">Up</span>
                <Figure value={txValue} unit={txUnit} className="ml-1" />
              </div>
            </div>
            {/* Keeps the lines level with the metered cells */}
            <div aria-hidden="true" className="mt-3 h-1" />
            <Lines>
              <Line label="Received" value={formatBytes(stats.network.bytesReceived)} />
              <Line label="Sent" value={formatBytes(stats.network.bytesSent)} />
            </Lines>
          </Metric>
        </div>

        {stats.disk.length > 1 && (
          <div className="border-t border-border">
            <h3 className="px-4 pb-1 pt-3 text-xs text-muted-foreground sm:px-5">
              Disks
            </h3>
            <ul className="divide-y divide-border">
              {stats.disk.map((d) => (
                <li key={d.mount} className="px-4 py-2.5 sm:px-5">
                  <div className="flex items-center gap-4">
                    <div className="flex min-w-0 flex-1 items-baseline gap-2 sm:w-52 sm:flex-none">
                      <span className="truncate font-mono text-[13px] text-foreground">
                        {d.mount}
                      </span>
                      <span className="shrink-0 text-xs text-subtle">
                        {d.type || "Unknown"}
                      </span>
                    </div>
                    <Meter value={d.usedPercent} className="hidden flex-1 sm:block" />
                    <span className="hidden w-40 shrink-0 text-right font-mono text-xs text-muted-foreground sm:inline">
                      {formatBytes(d.used)} of {formatBytes(d.total)}
                    </span>
                    <span className="w-12 shrink-0 text-right font-mono text-[13px] text-foreground">
                      {d.usedPercent.toFixed(1)}%
                    </span>
                  </div>
                  <div className="mt-2 flex items-center gap-3 sm:hidden">
                    <Meter value={d.usedPercent} className="flex-1" />
                    <span className="shrink-0 font-mono text-xs text-muted-foreground">
                      {formatBytes(d.used)} of {formatBytes(d.total)}
                    </span>
                  </div>
                </li>
              ))}
            </ul>
          </div>
        )}
      </Card>

      {/* Server details: two-column definition list */}
      <Card>
        <div className="border-b border-border px-4 py-3 sm:px-5">
          <CardTitle>Server details</CardTitle>
        </div>
        <div className="grid px-4 sm:px-5 md:grid-cols-2 md:gap-x-10">
          <dl className="divide-y divide-border">
            <Detail label="Hostname">{stats.server.hostname}</Detail>
            <Detail label="OS" mono={false}>{stats.server.distro || stats.server.platform}</Detail>
            <Detail label="Kernel">{stats.server.kernel}</Detail>
            <Detail label="Architecture">{stats.server.arch}</Detail>
            <Detail label="Uptime">{stats.server.uptimeFormatted}</Detail>
            <Detail label="Server time">{stats.server.serverTime}</Detail>
          </dl>
          <dl className="divide-y divide-border border-t border-border md:border-t-0">
            <Detail label="CPU" title={stats.server.cpuModel}>
              {stats.server.cpuCores}
            </Detail>
            <Detail label="Load average">
              <span className="inline-flex flex-wrap items-baseline gap-x-3">
                {loads.map((load, i) => (
                  <span key={i} className={loadTone(load)}>
                    {load}
                  </span>
                ))}
                <span className="font-sans text-xs text-subtle">1, 5, 15 min</span>
              </span>
            </Detail>
            {stats.gpu.available && (
              <Detail label="GPU" title={stats.gpu.model || ""}>
                {stats.gpu.model}
              </Detail>
            )}
            <Detail label="Swap">
              {formatBytes(stats.server.swap?.used || 0)} of{" "}
              {formatBytes(stats.server.swap?.total || 0)}
            </Detail>
            <Detail label="IP address">{stats.server.ipAddress}</Detail>
            {hasLocation && <Detail label="Location" mono={false}>{stats.server.location}</Detail>}
            <Detail label="Connections">{stats.server.activeConnections}</Detail>
          </dl>
        </div>
      </Card>

      {/* Top processes */}
      {stats.processes && stats.processes.length > 0 && (
        <Card className="overflow-hidden">
          <div className="flex items-center justify-between gap-3 border-b border-border px-4 py-2.5 sm:px-5">
            <CardTitle>Top processes</CardTitle>
            <div className="inline-flex rounded-md border border-input p-0.5">
              <button
                onClick={() => setProcessSortBy("cpu")}
                className={cn(
                  SEGMENT,
                  processSortBy === "cpu"
                    ? "bg-secondary text-foreground"
                    : "text-muted-foreground hover:text-foreground"
                )}
              >
                CPU
              </button>
              <button
                onClick={() => setProcessSortBy("mem")}
                className={cn(
                  SEGMENT,
                  processSortBy === "mem"
                    ? "bg-secondary text-foreground"
                    : "text-muted-foreground hover:text-foreground"
                )}
              >
                Mem
              </button>
            </div>
          </div>

          <table className="w-full text-left text-[13px]">
            <thead>
              <tr className="h-9 text-xs text-muted-foreground">
                <th className="hidden w-24 pl-5 pr-3 font-medium sm:table-cell">PID</th>
                <th className="pl-4 pr-3 font-medium sm:px-3">Process</th>
                <th className="hidden px-3 font-medium sm:table-cell">User</th>
                <th
                  className={cn(
                    "w-20 px-3 text-right font-medium sm:w-28",
                    processSortBy === "cpu" && "text-foreground",
                  )}
                >
                  CPU
                </th>
                <th
                  className={cn(
                    "w-20 pl-3 pr-4 text-right font-medium sm:w-28 sm:pr-5",
                    processSortBy === "mem" && "text-foreground",
                  )}
                >
                  Memory
                </th>
              </tr>
            </thead>
            <tbody>
              {[...stats.processes]
                .sort((a, b) =>
                  processSortBy === "cpu" ? b.cpu - a.cpu : b.mem - a.mem
                )
                .map((proc) => (
                  <tr
                    key={proc.pid}
                    className="h-11 border-t border-border transition-colors duration-150 hover:bg-secondary/50"
                  >
                    <td className="hidden pl-5 pr-3 font-mono text-muted-foreground sm:table-cell">
                      {proc.pid}
                    </td>
                    <td className="max-w-0 py-2 pl-4 pr-3 sm:px-3">
                      <div className="truncate font-medium text-foreground" title={proc.name}>
                        {proc.name}
                      </div>
                      <div className="truncate font-mono text-xs text-muted-foreground sm:hidden">
                        {proc.pid} · {proc.user}
                      </div>
                    </td>
                    <td className="hidden px-3 font-mono text-muted-foreground sm:table-cell">
                      {proc.user}
                    </td>
                    <td className="px-3 text-right font-mono text-foreground">
                      {proc.cpu.toFixed(1)}%
                    </td>
                    <td className="pl-3 pr-4 text-right font-mono text-foreground sm:pr-5">
                      {proc.mem.toFixed(1)}%
                    </td>
                  </tr>
                ))}
            </tbody>
          </table>
        </Card>
      )}

      {/* Purge Confirmation Dialog */}
      <Dialog open={showPurgeDialog} onOpenChange={setShowPurgeDialog}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>Purge unused Docklift images</DialogTitle>
            <DialogDescription>
              Removes unused Docklift images outside each project&apos;s keep-2 set and clears all BuildKit cache. Does not modify the host OS or other containers.
            </DialogDescription>
          </DialogHeader>

          <dl className="divide-y divide-border border-y border-border text-[13px] leading-5">
            <div className="grid grid-cols-[5.5rem_minmax(0,1fr)] gap-3 py-2">
              <dt className="text-muted-foreground">Removes</dt>
              <dd className="text-foreground">Unused Docklift images + full BuildKit wipe</dd>
            </div>
            <div className="grid grid-cols-[5.5rem_minmax(0,1fr)] gap-3 py-2">
              <dt className="text-muted-foreground">Keeps</dt>
              <dd className="text-foreground">Current + previous successful tags per project</dd>
            </div>
            <div className="grid grid-cols-[5.5rem_minmax(0,1fr)] gap-3 py-2">
              <dt className="text-muted-foreground">Leaves alone</dt>
              <dd className="text-foreground">
                No host prune, journal, apt, or <span className="font-mono">/tmp</span> wipe
              </dd>
            </div>
          </dl>

          <label className="block space-y-1.5">
            <span className="block text-[13px] font-medium text-foreground">
              Confirm with your account password
            </span>
            <Input
              type="password"
              value={purgePassword}
              onChange={(e) => setPurgePassword(e.target.value)}
              placeholder="Account password"
              autoComplete="current-password"
            />
          </label>

          <DialogFooter className="pt-1">
            <Button
              variant="outline"
              onClick={() => {
                setShowPurgeDialog(false);
                setPurgePassword("");
              }}
            >
              Cancel
            </Button>
            <Button
              variant="destructive"
              onClick={handlePurge}
              disabled={!purgePassword.trim()}
            >
              Purge images
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </div>
  );
}
