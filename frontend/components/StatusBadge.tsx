import { cn } from "@/lib/utils";

interface StatusBadgeProps {
  status: string;
  size?: "sm" | "default";
}

const statusConfig: Record<string, { label: string; colors: string; dot: string }> = {
  running: {
    label: "Running",
    colors: "bg-emerald-500/10 text-emerald-600 dark:text-emerald-400 ring-emerald-500/20",
    dot: "bg-emerald-500",
  },
  building: {
    label: "Building",
    colors: "bg-amber-500/10 text-amber-600 dark:text-amber-400 ring-amber-500/20",
    dot: "bg-amber-500 animate-pulse",
  },
  stopped: {
    label: "Stopped",
    colors: "bg-zinc-500/10 text-zinc-600 dark:text-zinc-400 ring-zinc-500/20",
    dot: "bg-zinc-400",
  },
  pending: {
    label: "Pending",
    colors: "bg-blue-500/10 text-blue-600 dark:text-blue-400 ring-blue-500/20",
    dot: "bg-blue-500",
  },
  error: {
    label: "Error",
    colors: "bg-red-500/10 text-red-600 dark:text-red-400 ring-red-500/20",
    dot: "bg-red-500",
  },
};

export function StatusBadge({ status, size = "default" }: StatusBadgeProps) {
  const config = statusConfig[status] || statusConfig.pending;

  return (
    <span
      className={cn(
        "inline-flex items-center gap-2 rounded-full ring-1 font-medium",
        config.colors,
        size === "sm" ? "px-2.5 py-1 text-[11px]" : "px-3 py-1.5 text-xs"
      )}
    >
      <span className={cn("rounded-full", config.dot, size === "sm" ? "h-1.5 w-1.5" : "h-2 w-2")} />
      {config.label}
    </span>
  );
}
