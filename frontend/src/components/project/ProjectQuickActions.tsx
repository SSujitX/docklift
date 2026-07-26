// Shared deploy/stop/restart/delete controls for project list card and table rows.

import { useState, type MouseEvent } from "react";
import { Project } from "@/lib/types";
import { Button } from "@/components/ui/button";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import {
  Play,
  Square,
  RotateCw,
  Trash2,
  Loader2,
  XCircle,
  AlertTriangle,
} from "lucide-react";
import { API_URL, cn } from "@/lib/utils";
import { authFetch } from "@/lib/auth";
import { toast } from "sonner";

const ACTION =
  "h-8 gap-1.5 rounded-lg px-2.5 text-xs font-medium shadow-none border transition-colors";

const ACTION_PRIMARY = `${ACTION} bg-brand text-brand-foreground border-transparent hover:bg-brand hover:brightness-110 hover:text-brand-foreground`;
const ACTION_SOFT = `${ACTION} bg-brand/15 text-brand border-transparent hover:bg-brand/25`;
const ACTION_SECONDARY = `${ACTION} bg-background text-foreground border-border hover:bg-secondary hover:text-foreground`;
const ACTION_DANGER = `${ACTION} bg-background text-destructive border-border hover:bg-destructive/10 hover:text-destructive hover:border-destructive/35`;

interface ProjectQuickActionsProps {
  project: Project;
  onRefresh: () => void;
  /** Compact table row: Redeploy/Deploy, Stop, Delete. */
  compact?: boolean;
  className?: string;
}

export function ProjectQuickActions({
  project,
  onRefresh,
  compact = false,
  className,
}: ProjectQuickActionsProps) {
  const [loading, setLoading] = useState(false);
  const [actionType, setActionType] = useState<string | null>(null);
  const [deleteDialogOpen, setDeleteDialogOpen] = useState(false);
  const [deleting, setDeleting] = useState(false);

  const isLive = project.status === "running" || project.status === "degraded";

  const handleAction = async (
    action: "deploy" | "redeploy" | "stop" | "restart" | "cancel",
    e: MouseEvent,
  ) => {
    e.preventDefault();
    e.stopPropagation();
    setLoading(true);
    setActionType(action);
    try {
      const res = await authFetch(
        `${API_URL}/api/deployments/${project.id}/${action}`,
        {
          method: "POST",
          headers:
            action === "deploy" || action === "redeploy"
              ? { "Content-Type": "application/json" }
              : undefined,
          body:
            action === "deploy" || action === "redeploy"
              ? JSON.stringify({
                  trigger: action === "redeploy" ? "redeploy" : "manual",
                })
              : undefined,
        },
      );
      if (!res.ok) {
        const data = await res.json().catch(() => ({}));
        toast.error(
          (data as { error?: string }).error || `Failed to ${action} project`,
        );
        return;
      }
      toast.success(
        action === "redeploy"
          ? "Redeploy started"
          : action === "deploy"
            ? "Deploy started"
            : `${action.charAt(0).toUpperCase() + action.slice(1)} started`,
      );
      setTimeout(onRefresh, 2000);
    } catch (error) {
      console.error(error);
      toast.error(`Failed to ${action} project`);
    } finally {
      setLoading(false);
      setActionType(null);
    }
  };

  const handleDeleteConfirm = async (e?: MouseEvent) => {
    if (e) e.stopPropagation();
    setDeleting(true);
    try {
      const res = await authFetch(`${API_URL}/api/projects/${project.id}`, {
        method: "DELETE",
      });
      if (!res.ok) {
        const data = await res.json().catch(() => ({}));
        toast.error(
          (data as { error?: string }).error || "Failed to delete project",
        );
        return;
      }
      setDeleteDialogOpen(false);
      onRefresh();
    } catch (error) {
      console.error(error);
      toast.error("Failed to delete project");
    } finally {
      setDeleting(false);
    }
  };

  return (
    <>
      <div
        role="group"
        aria-label={`${project.name} actions`}
        className={cn(
          "flex flex-wrap items-center gap-1.5",
          compact
            ? "justify-start sm:justify-end"
            : "border-t border-border/40 pt-3 md:border-t-0 md:pt-0",
          className,
        )}
        onClick={(e) => e.stopPropagation()}
        onKeyDown={(e) => e.stopPropagation()}
      >
        {isLive ? (
          <>
            <Button
              type="button"
              variant="bare"
              size="sm"
              className={compact ? ACTION_SOFT : ACTION_PRIMARY}
              onClick={(e) => handleAction("redeploy", e)}
              disabled={loading}
            >
              {loading && actionType === "redeploy" ? (
                <Loader2 className="h-3.5 w-3.5 animate-spin" />
              ) : (
                <Play className="h-3 w-3 fill-current" />
              )}
              Redeploy
            </Button>
            <Button
              type="button"
              variant="bare"
              size={compact ? "icon" : "sm"}
              className={
                compact
                  ? "h-8 w-8 border border-border/60 bg-background"
                  : ACTION_SECONDARY
              }
              onClick={(e) => handleAction("restart", e)}
              disabled={loading}
              title="Restart"
              aria-label="Restart"
            >
              {loading && actionType === "restart" ? (
                <Loader2 className="h-3.5 w-3.5 animate-spin" />
              ) : (
                <RotateCw className="h-3.5 w-3.5" />
              )}
              {!compact && "Restart"}
            </Button>
            <Button
              type="button"
              variant="bare"
              size="sm"
              className={ACTION_SECONDARY}
              onClick={(e) => handleAction("stop", e)}
              disabled={loading}
            >
              {loading && actionType === "stop" ? (
                <Loader2 className="h-3.5 w-3.5 animate-spin" />
              ) : (
                <Square className="h-3 w-3 fill-current" />
              )}
              Stop
            </Button>
          </>
        ) : project.status === "building" ? (
          <Button
            type="button"
            variant="bare"
            size="sm"
            className={ACTION_DANGER}
            onClick={(e) => handleAction("cancel", e)}
            disabled={loading}
          >
            {loading && actionType === "cancel" ? (
              <Loader2 className="h-3.5 w-3.5 animate-spin" />
            ) : (
              <XCircle className="h-3.5 w-3.5" />
            )}
            Cancel
          </Button>
        ) : (
          <Button
            type="button"
            variant="bare"
            size="sm"
            className={ACTION_PRIMARY}
            onClick={(e) => handleAction("deploy", e)}
            disabled={loading}
          >
            {loading && actionType === "deploy" ? (
              <Loader2 className="h-3.5 w-3.5 animate-spin" />
            ) : (
              <Play className="h-3 w-3 fill-current" />
            )}
            Deploy
          </Button>
        )}

        <Button
          type="button"
          variant="bare"
          size={compact ? "icon" : "sm"}
          className={
            compact
              ? "h-8 w-8 border border-border/60 bg-background text-destructive hover:bg-destructive/10"
              : ACTION_DANGER
          }
          onClick={(e) => {
            e.preventDefault();
            e.stopPropagation();
            setDeleteDialogOpen(true);
          }}
          disabled={loading || deleting}
          title="Delete project"
          aria-label="Delete project"
        >
          <Trash2 className="h-3.5 w-3.5" />
          {!compact && "Delete"}
        </Button>
      </div>

      <Dialog open={deleteDialogOpen} onOpenChange={setDeleteDialogOpen}>
        <DialogContent
          className="sm:max-w-[400px]"
          onClick={(e) => e.stopPropagation()}
        >
          <DialogHeader>
            <div className="mx-auto mb-4 flex h-12 w-12 items-center justify-center rounded-full bg-red-500/10">
              <AlertTriangle className="h-6 w-6 text-red-500" />
            </div>
            <DialogTitle className="text-center">Delete Project</DialogTitle>
            <DialogDescription className="text-center">
              Are you sure you want to delete{" "}
              <span className="font-semibold text-foreground">{project.name}</span>
              ?
              <br />
              <span className="text-red-500">
                This will also remove all containers and cannot be undone.
              </span>
            </DialogDescription>
          </DialogHeader>
          <DialogFooter className="gap-2 sm:gap-0">
            <Button
              variant="outline"
              onClick={(e) => {
                e.stopPropagation();
                setDeleteDialogOpen(false);
              }}
              disabled={deleting}
              className="flex-1"
            >
              Cancel
            </Button>
            <Button
              variant="destructive"
              onClick={handleDeleteConfirm}
              disabled={deleting}
              className="flex-1 gap-2"
            >
              {deleting ? (
                <Loader2 className="h-4 w-4 animate-spin" />
              ) : (
                <Trash2 className="h-4 w-4" />
              )}
              Delete
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </>
  );
}
