// Compact CPU / RAM meters for the projects table (live container stats).

import { cn } from "@/lib/utils";

export interface ProjectResourceStats {
  cpuPercent: number;
  memoryMb: number;
  memoryPercent: number;
}

function Meter({
  label,
  value,
  display,
  tone = "brand",
}: {
  label: string;
  value: number | null;
  display: string;
  tone?: "brand" | "muted" | "danger";
}) {
  const width = value == null ? 0 : Math.min(100, Math.max(0, value));
  return (
    <div className="min-w-[7.5rem]">
      <div className="mb-0.5 flex items-center justify-between gap-2 text-[10px] font-medium tabular-nums">
        <span className="text-muted-foreground">{label}</span>
        <span
          className={cn(
            value == null ? "text-muted-foreground/60" : "text-foreground",
          )}
        >
          {display}
        </span>
      </div>
      <div className="h-1.5 overflow-hidden rounded-full bg-secondary">
        <div
          className={cn(
            "h-full rounded-full transition-[width] duration-300",
            tone === "danger"
              ? "bg-red-500/70"
              : tone === "muted"
                ? "bg-zinc-400/50"
                : "bg-brand",
          )}
          style={{ width: `${width}%` }}
        />
      </div>
    </div>
  );
}

function formatRam(mb: number): string {
  if (mb >= 1024) return `${(mb / 1024).toFixed(1)}G`;
  return `${Math.round(mb)}M`;
}

export function ProjectResourceCell({
  stats,
  live,
}: {
  /** undefined = loading, null = unavailable, object = live reading */
  stats?: ProjectResourceStats | null;
  live: boolean;
}) {
  if (!live) {
    return (
      <div className="space-y-1.5">
        <Meter label="CPU" value={null} display="—" tone="muted" />
        <Meter label="RAM" value={null} display="—" tone="muted" />
      </div>
    );
  }

  if (stats === undefined) {
    return (
      <div className="space-y-1.5">
        <Meter label="CPU" value={null} display="…" tone="muted" />
        <Meter label="RAM" value={null} display="…" tone="muted" />
      </div>
    );
  }

  if (stats === null) {
    return (
      <div className="space-y-1.5">
        <Meter label="CPU" value={null} display="n/a" tone="muted" />
        <Meter label="RAM" value={null} display="n/a" tone="muted" />
      </div>
    );
  }

  const cpu = Number.isFinite(stats.cpuPercent) ? stats.cpuPercent : 0;
  const ramPct = Number.isFinite(stats.memoryPercent) ? stats.memoryPercent : 0;
  const ramMb = Number.isFinite(stats.memoryMb) ? stats.memoryMb : 0;
  const cpuTone = cpu >= 90 ? "danger" : "brand";
  const ramTone = ramPct >= 90 ? "danger" : "brand";

  return (
    <div className="space-y-1.5">
      <Meter
        label="CPU"
        value={cpu}
        display={`${Math.round(cpu)}%`}
        tone={cpuTone}
      />
      <Meter
        label="RAM"
        value={ramPct}
        display={formatRam(ramMb)}
        tone={ramTone}
      />
    </div>
  );
}

/** Aggregate multi-service stats from GET /api/deployments/:id/stats. */
export function aggregateProjectStats(
  payload: Record<string, unknown> | null | undefined,
): ProjectResourceStats | null {
  if (!payload || typeof payload !== "object") return null;

  let cpu = 0;
  let memoryMb = 0;
  let memoryPercent = 0;
  let count = 0;

  for (const value of Object.values(payload)) {
    if (!value || typeof value !== "object") continue;
    const row = value as {
      cpu_percent?: string | number;
      memory_usage?: string;
      memory_percent?: string | number;
    };
    const cpuVal = Number(row.cpu_percent);
    const memPct = Number(row.memory_percent);
    const memMatch = String(row.memory_usage ?? "").match(/([\d.]+)/);
    const memMb = memMatch ? Number(memMatch[1]) : NaN;
    if (!Number.isFinite(cpuVal) && !Number.isFinite(memMb)) continue;
    cpu += Number.isFinite(cpuVal) ? cpuVal : 0;
    memoryMb += Number.isFinite(memMb) ? memMb : 0;
    memoryPercent += Number.isFinite(memPct) ? memPct : 0;
    count += 1;
  }

  if (count === 0) return null;
  return {
    cpuPercent: cpu,
    memoryMb,
    memoryPercent: memoryPercent / count,
  };
}
