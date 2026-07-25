// Project vs service scope rail for multi-Dockerfile projects.
// Single-service projects omit this — tabs stay flat.

import { cn } from "@/lib/utils";
import type { Service } from "@/lib/types";
import { StatusBadge } from "@/components/StatusBadge";
import { Layers } from "lucide-react";

export type ProjectWorkspace = "project" | "service";

interface ServiceSwitcherProps {
  services: Service[];
  workspace: ProjectWorkspace;
  selectedId: string | null;
  onSelectProject: () => void;
  onSelectService: (serviceId: string) => void;
  className?: string;
}

export function ServiceSwitcher({
  services,
  workspace,
  selectedId,
  onSelectProject,
  onSelectService,
  className,
}: ServiceSwitcherProps) {
  if (services.length <= 1) return null;

  return (
    <div className={cn("flex flex-col gap-3", className)}>
      <div className="min-w-0">
        <p className="text-[11px] font-medium uppercase tracking-[0.12em] text-muted-foreground">
          Workspace
        </p>
        <p className="mt-0.5 text-xs text-muted-foreground">
          All services covers deploy, build, and shared env. Open one service for
          that app’s env, domains, storage, and runtime logs.
        </p>
      </div>

      <div
        role="group"
        aria-label="All services or one service workspace"
        className="flex max-w-full gap-1 overflow-x-auto pb-0.5 [-ms-overflow-style:none] [scrollbar-width:none] [&::-webkit-scrollbar]:hidden"
      >
        <button
          type="button"
          aria-pressed={workspace === "project"}
          onClick={onSelectProject}
          className={cn(
            "inline-flex shrink-0 items-center gap-2 rounded-lg border px-3 py-2 text-left transition-colors",
            workspace === "project"
              ? "border-border bg-background text-foreground shadow-sm"
              : "border-transparent bg-secondary/50 text-muted-foreground hover:bg-secondary hover:text-foreground",
          )}
        >
          <Layers className="h-3.5 w-3.5 shrink-0" />
          <span className="text-xs font-medium">All services</span>
          <span className="text-[10px] text-muted-foreground">
            {services.length}
          </span>
        </button>

        {services.map((svc) => {
          const active =
            workspace === "service" && svc.id === selectedId;
          return (
            <button
              key={svc.id}
              type="button"
              aria-pressed={active}
              onClick={() => onSelectService(svc.id)}
              className={cn(
                "inline-flex shrink-0 items-center gap-2 rounded-lg border px-3 py-2 text-left transition-colors",
                active
                  ? "border-border bg-background text-foreground shadow-sm"
                  : "border-transparent bg-secondary/50 text-muted-foreground hover:bg-secondary hover:text-foreground",
              )}
            >
              <span className="max-w-[9rem] truncate text-xs font-medium sm:max-w-[12rem]">
                {svc.name}
              </span>
              <StatusBadge status={svc.status || "pending"} size="sm" />
            </button>
          );
        })}
      </div>
    </div>
  );
}
