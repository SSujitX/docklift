// StatusBadge component - status dot + label (running, building, etc.). No fill, no pill.
import { cn } from "@/lib/utils";

interface StatusBadgeProps {
  status: string;
  size?: "sm" | "default";
}

const statusConfig: Record<string, { label: string; text: string; dot: string }> = {
  running: {
    label: "Running",
    text: "text-foreground",
    dot: "bg-ok",
  },
  building: {
    label: "Building",
    text: "text-foreground",
    dot: "bg-warn animate-pulse motion-reduce:animate-none",
  },
  stopped: {
    label: "Stopped",
    text: "text-muted-foreground",
    dot: "bg-subtle",
  },
  pending: {
    label: "Pending",
    text: "text-foreground",
    dot: "bg-info",
  },
  error: {
    label: "Error",
    text: "text-foreground",
    dot: "bg-danger",
  },
  degraded: {
    label: "Degraded",
    text: "text-foreground",
    dot: "bg-warn",
  },
};

export function StatusBadge({ status, size = "default" }: StatusBadgeProps) {
  const config = statusConfig[status] || statusConfig.pending;

  return (
    <span
      className={cn(
        "inline-flex items-center whitespace-nowrap font-medium",
        config.text,
        size === "sm" ? "gap-1.5 text-xs" : "gap-2 text-[13px] leading-5",
      )}
    >
      <span
        aria-hidden="true"
        className={cn("shrink-0 rounded-full", config.dot, size === "sm" ? "size-1.5" : "size-2")}
      />
      {config.label}
    </span>
  );
}
