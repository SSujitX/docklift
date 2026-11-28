// Project-scoped lifecycle actions (whole compose stack — never a single service).

import { Button } from "@/components/ui/button";
import { cn } from "@/lib/utils";
import { Loader2 } from "lucide-react";

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

function Spinner() {
  return <Loader2 className="animate-spin" />;
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
          ? "flex flex-col items-start gap-1.5 md:items-end"
          : "flex items-center justify-center",
        className,
      )}
    >
      <div
        role="group"
        title={groupTitle}
        aria-label={
          multiService
            ? "Actions for all services in this project"
            : "Project actions"
        }
        className={cn(
          "flex flex-wrap items-center gap-2",
          isHeader ? "justify-start md:justify-end" : "justify-center",
        )}
      >
        {isLive ? (
          <>
            <Button
              type="button"
              onClick={() => onAction("redeploy")}
              disabled={actionLoading}
            >
              {currentAction === "redeploy" && <Spinner />}
              Redeploy
            </Button>
            <Button
              type="button"
              variant="outline"
              onClick={() => onAction("restart")}
              disabled={actionLoading}
            >
              {currentAction === "restart" && <Spinner />}
              Restart
            </Button>
            <Button
              type="button"
              variant="outline"
              onClick={() => onAction("stop")}
              disabled={actionLoading}
            >
              {currentAction === "stop" && <Spinner />}
              Stop
            </Button>
          </>
        ) : status === "building" ? (
          <Button
            type="button"
            variant="outline"
            className="text-danger"
            onClick={() => onAction("cancel")}
            disabled={currentAction === "cancel"}
          >
            {currentAction === "cancel" && <Spinner />}
            Cancel build
          </Button>
        ) : (
          <Button
            type="button"
            onClick={() => onAction("deploy")}
            disabled={actionLoading}
          >
            {currentAction === "deploy" && <Spinner />}
            Deploy
          </Button>
        )}

        <Button
          type="button"
          variant="outline"
          className="text-danger"
          onClick={() => onAction("delete")}
          disabled={actionLoading}
          title="Delete project"
        >
          {currentAction === "delete" && <Spinner />}
          Delete
        </Button>
      </div>
      {isHeader && multiService && (
        <p className="text-xs text-muted-foreground">
          Applies to every service in this project.
        </p>
      )}
    </div>
  );
}
