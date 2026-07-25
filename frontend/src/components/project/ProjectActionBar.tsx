// Project-scoped lifecycle actions (whole compose stack — never a single service).

import { Button } from "@/components/ui/button";
import { cn } from "@/lib/utils";
import {
  Loader2,
  Play,
  RotateCw,
  Square,
  Trash2,
  XCircle,
  Layers,
} from "lucide-react";

const ACTION =
  "h-9 gap-1.5 rounded-lg px-3.5 text-xs sm:text-sm font-semibold shadow-none border transition-colors";

const ACTION_PRIMARY =
  `${ACTION} bg-brand text-brand-foreground border-transparent hover:bg-brand hover:brightness-110 hover:text-brand-foreground`;

const ACTION_SECONDARY =
  `${ACTION} bg-background text-foreground border-border hover:bg-secondary hover:text-foreground`;

/** Quiet danger (Delete) — visible border + tint, not ghost-grey. */
const ACTION_DANGER =
  `${ACTION} border-destructive/45 bg-destructive/10 text-destructive hover:bg-destructive/18 hover:border-destructive/60 hover:text-destructive`;

/** Solid danger (Cancel Build) — must read clearly while a build is running. */
const ACTION_DANGER_SOLID =
  `${ACTION} border-transparent bg-destructive text-destructive-foreground hover:bg-destructive/90 hover:text-destructive-foreground`;

export type ProjectLifecycleStatus =
  | "running"
  | "degraded"
  | "building"
  | "stopped"
  | "error"
  | string;

interface ProjectActionBarProps {
  status: ProjectLifecycleStatus;
  actionLoading: boolean;
  currentAction: string | null;
  onAction: (action: string) => void;
  /** Multi-service: make “all services” explicit so service workspace isn’t confused. */
  multiService?: boolean;
  /** Header: right-aligned cluster next to the title (single-service). */
  placement?: "header" | "panel";
  className?: string;
}

export function ProjectActionBar({
  status,
  actionLoading,
  currentAction,
  onAction,
  multiService = false,
  placement = "panel",
  className,
}: ProjectActionBarProps) {
  const isLive = status === "running" || status === "degraded";
  const isHeader = placement === "header";

  return (
    <div
      className={cn(
        multiService && !isHeader
          ? "flex flex-col gap-2 sm:flex-row sm:items-center sm:justify-between sm:gap-4"
          : "flex items-center justify-end",
        className,
      )}
    >
      {multiService && !isHeader && (
        <div className="min-w-0">
          <p className="flex items-center gap-1.5 text-[11px] font-medium uppercase tracking-[0.12em] text-muted-foreground">
            <Layers className="h-3 w-3" />
            All-services actions
          </p>
          <p className="mt-0.5 text-xs text-muted-foreground">
            Redeploy, restart, and stop hit every service in this project — not
            only the app selected in Workspace.
          </p>
        </div>
      )}

      <div
        role="group"
        aria-label={
          multiService
            ? "Actions for all services in this project"
            : "Project actions"
        }
        className={cn(
          "flex flex-wrap items-center gap-2",
          isHeader
            ? "w-full justify-stretch sm:w-auto sm:justify-end"
            : "w-full justify-end sm:w-auto",
        )}
      >
        {isLive ? (
          <>
            <Button
              type="button"
              variant="bare"
              size="sm"
              onClick={() => onAction("redeploy")}
              disabled={actionLoading}
              className={cn(
                ACTION_PRIMARY,
                isHeader && "min-w-0 flex-1 sm:flex-none",
              )}
            >
              {currentAction === "redeploy" ? (
                <Loader2 className="h-4 w-4 animate-spin" />
              ) : (
                <Play className="h-3.5 w-3.5 fill-current" />
              )}
              Redeploy
            </Button>
            <Button
              type="button"
              variant="bare"
              size="sm"
              onClick={() => onAction("restart")}
              disabled={actionLoading}
              className={cn(
                ACTION_SECONDARY,
                isHeader && "min-w-0 flex-1 sm:flex-none",
              )}
            >
              {currentAction === "restart" ? (
                <Loader2 className="h-4 w-4 animate-spin" />
              ) : (
                <RotateCw className="h-3.5 w-3.5" />
              )}
              Restart
            </Button>
            <Button
              type="button"
              variant="bare"
              size="sm"
              onClick={() => onAction("stop")}
              disabled={actionLoading}
              className={cn(
                ACTION_SECONDARY,
                isHeader && "min-w-0 flex-1 sm:flex-none",
              )}
            >
              {currentAction === "stop" ? (
                <Loader2 className="h-4 w-4 animate-spin" />
              ) : (
                <Square className="h-3.5 w-3.5 fill-current" />
              )}
              Stop
            </Button>
          </>
        ) : status === "building" ? (
          <Button
            type="button"
            variant="bare"
            size="sm"
            onClick={() => onAction("cancel")}
            disabled={currentAction === "cancel"}
            className={cn(
              ACTION_DANGER_SOLID,
              isHeader && "min-w-0 flex-1 sm:flex-none",
            )}
          >
            {currentAction === "cancel" ? (
              <Loader2 className="h-4 w-4 animate-spin" />
            ) : (
              <XCircle className="h-3.5 w-3.5" />
            )}
            Cancel Build
          </Button>
        ) : (
          <Button
            type="button"
            variant="bare"
            size="sm"
            onClick={() => onAction("deploy")}
            disabled={actionLoading}
            className={cn(
              ACTION_PRIMARY,
              isHeader && "min-w-0 flex-1 sm:flex-none",
            )}
          >
            {currentAction === "deploy" ? (
              <Loader2 className="h-4 w-4 animate-spin" />
            ) : (
              <Play className="h-3.5 w-3.5 fill-current" />
            )}
            Deploy Now
          </Button>
        )}

        <Button
          type="button"
          variant="bare"
          size="sm"
          onClick={() => onAction("delete")}
          disabled={actionLoading}
          className={cn(
            ACTION_DANGER,
            isHeader && "min-w-0 flex-1 sm:flex-none",
          )}
          title="Delete project"
        >
          {currentAction === "delete" ? (
            <Loader2 className="h-4 w-4 animate-spin" />
          ) : (
            <Trash2 className="h-3.5 w-3.5" />
          )}
          Delete
        </Button>
      </div>
    </div>
  );
}
