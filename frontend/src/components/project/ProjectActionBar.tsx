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
  "h-9 gap-1.5 rounded-lg px-3 text-xs sm:text-sm font-medium shadow-none border transition-colors";

const ACTION_PRIMARY =
  `${ACTION} bg-brand text-brand-foreground border-transparent hover:bg-brand hover:brightness-110 hover:text-brand-foreground`;

const ACTION_SECONDARY =
  `${ACTION} bg-background text-foreground border-border hover:bg-secondary hover:text-foreground`;

const ACTION_DANGER =
  `${ACTION} bg-background text-destructive border-border hover:bg-destructive/10 hover:text-destructive hover:border-destructive/35`;

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
  className?: string;
}

export function ProjectActionBar({
  status,
  actionLoading,
  currentAction,
  onAction,
  multiService = false,
  className,
}: ProjectActionBarProps) {
  const isLive = status === "running" || status === "degraded";

  return (
    <div
      className={cn(
        "flex flex-col gap-2 sm:flex-row sm:items-center sm:justify-between sm:gap-4",
        className,
      )}
    >
      {multiService && (
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
        className="grid w-full grid-cols-2 gap-2 sm:flex sm:w-auto sm:flex-wrap sm:items-center sm:justify-end"
      >
        {isLive ? (
          <>
            <Button
              type="button"
              variant="bare"
              size="sm"
              onClick={() => onAction("redeploy")}
              disabled={actionLoading}
              className={cn(ACTION_PRIMARY, "w-full sm:w-auto")}
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
              className={cn(ACTION_SECONDARY, "w-full sm:w-auto")}
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
              className={cn(ACTION_SECONDARY, "w-full sm:w-auto")}
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
              ACTION_DANGER,
              "col-span-2 w-full sm:col-span-1 sm:w-auto",
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
              "col-span-2 w-full sm:col-span-1 sm:w-auto",
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
            isLive
              ? "w-full sm:w-auto"
              : "col-span-2 w-full sm:col-span-1 sm:w-auto",
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
