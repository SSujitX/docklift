// Compact CPU / RAM meters for the projects table (live container stats).

import { cn } from "@/lib/utils";

export interface ProjectResourceStats {
  cpuPercent: number;
  memoryMb: number;
  memoryPercent: number;
}

/** Fill colour for a 0–100 reading: ink until it crosses a threshold. */
export function meterFill(percent: number | null): string {
  if (percent == null) return "bg-transparent";
  if (percent >= 90) return "bg-danger";
  if (percent >= 75) return "bg-warn";
  return "bg-foreground/70";
}

/** Value text colour for a 0–100 reading. */
export function meterText(percent: number | null): string {
  if (percent == null) return "text-subtle";
  if (percent >= 90) return "text-danger";
  if (percent >= 75) return "text-warn";
  return "text-foreground";
}

/**
 * One-line meter: label, 4px track, mono value. `percent` drives the fill and
 * threshold colour; `display` is the text shown (may differ, e.g. "301M").
 */
export function ResourceMeter({
  label,
  percent,
  display,
  className,
  trackClassName,
}: {
  label: string;
  percent: number | null;
  display: string;
  className?: string;
  trackClassName?: string;
}) {
  const width = percent == null ? 0 : Math.min(100, Math.max(0, percent));
  return (
    <div className={cn("flex items-center gap-2 text-xs leading-4", className)}>
      <span className="w-8 shrink-0 text-muted-foreground">{label}</span>
      <span
        aria-hidden="true"
        className={cn(
          "h-1 min-w-10 flex-1 overflow-hidden rounded-full bg-secondary",
          trackClassName,
        )}
      >
        <span
          className={cn(
            "block h-full rounded-full transition-[width] duration-300",
            meterFill(percent),
          )}
          style={{ width: `${width}%` }}
        />
      </span>
      <span
        className={cn(
          "w-10 shrink-0 text-right font-mono tabular-nums",
          meterText(percent),
        )}
      >
        {display}
      </span>
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
  const wrap = "w-full min-w-36 max-w-48 space-y-1";

  if (!live || stats === undefined || stats === null) {
    const display = !live ? "—" : stats === undefined ? "…" : "n/a";
    return (
      <div className={wrap}>
        <ResourceMeter label="CPU" percent={null} display={display} />
        <ResourceMeter label="RAM" percent={null} display={display} />
      </div>
    );
  }

  const cpu = Number.isFinite(stats.cpuPercent) ? stats.cpuPercent : 0;
  const ramPct = Number.isFinite(stats.memoryPercent) ? stats.memoryPercent : 0;
  const ramMb = Number.isFinite(stats.memoryMb) ? stats.memoryMb : 0;

  return (
    <div className={wrap}>
      <ResourceMeter label="CPU" percent={cpu} display={`${Math.round(cpu)}%`} />
      <ResourceMeter label="RAM" percent={ramPct} display={formatRam(ramMb)} />
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
