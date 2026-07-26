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
} from "lucide-react";

const ACTION =
  "h-10 gap-1.5 rounded-xl px-3 text-xs sm:h-9 sm:text-sm font-semibold shadow-none border transition-colors";

const ACTION_PRIMARY =
  `${ACTION} bg-brand text-brand-foreground border-transparent hover:bg-brand hover:brightness-110 hover:text-brand-foreground`;

const ACTION_SECONDARY =
  `${ACTION} bg-background text-foreground border-border/60 hover:bg-secondary hover:text-foreground`;

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
  /** Multi-service: aria/tooltip clarifies whole-stack scope. */
  multiService?: boolean;
  /** Header: cluster next to the project title (top-right on desktop). */
  placement?: "header" | "panel";
  /** Optional tooltip / title on the action group. */
  title?: string;
  className?: string;
}

export function ProjectActionBar({
  status,
  actionLoading,
  currentAction,
  onAction,
  multiService = false,
  placement = "panel",
  title,
  className,
}: ProjectActionBarProps) {
  const isLive = status === "running" || status === "degraded";
  const isHeader = placement === "header";
  const groupTitle =
    title ||
    (multiService
      ? "Affects every service in this project"
      : "Project actions");

  return (
    <div
      className={cn(
        isHeader
          ? "flex w-full flex-col items-stretch gap-2 sm:items-end md:max-w-md"
          : "flex items-center justify-center",
        className,
      )}
    >
      {isHeader && (
        <p className="text-left text-[11px] leading-snug text-muted-foreground sm:max-w-sm sm:text-right">
          {multiService ? (
            <>
              Redeploy, restart, and stop apply to{" "}
              <span className="font-medium text-foreground/80">every service</span>
              {" "}in this project — not only the one selected in Workspace.
            </>
          ) : (
            <>Redeploy, restart, and stop control this project’s containers.</>
          )}
        </p>
      )}
      <div
        role="group"
        title={groupTitle}
        aria-label={
          multiService
            ? "Actions for all services in this project"
            : "Project actions"
        }
        className={cn(
          isHeader
            ? "grid w-full grid-cols-2 gap-2 sm:flex sm:w-auto sm:flex-wrap sm:justify-end"
            : "flex w-full flex-wrap items-center justify-center gap-2",
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
              ACTION_DANGER_SOLID,
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
            className={cn(ACTION_PRIMARY, "w-full sm:w-auto")}
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
          className={cn(ACTION_DANGER, "w-full sm:w-auto")}
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
