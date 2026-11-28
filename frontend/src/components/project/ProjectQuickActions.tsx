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
import { RotateCw, Trash2, Loader2 } from "lucide-react";
import { API_URL, cn } from "@/lib/utils";
import { authFetch } from "@/lib/auth";
import { toast } from "sonner";

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
            : "border-t border-border pt-3 md:border-t-0 md:pt-0",
          className,
        )}
        onClick={(e) => e.stopPropagation()}
        onKeyDown={(e) => e.stopPropagation()}
      >
        {isLive ? (
          <>
            <Button
              type="button"
              variant="outline"
              size="sm"
              onClick={(e) => handleAction("redeploy", e)}
              disabled={loading}
            >
              {loading && actionType === "redeploy" && (
                <Loader2 className="size-3.5 animate-spin" />
              )}
              Redeploy
            </Button>
            <Button
              type="button"
              variant={compact ? "ghost" : "outline"}
              size={compact ? "icon" : "sm"}
              className={compact ? "text-subtle hover:text-foreground" : undefined}
              onClick={(e) => handleAction("restart", e)}
              disabled={loading}
              title="Restart"
              aria-label="Restart"
            >
              {loading && actionType === "restart" ? (
                <Loader2 className="size-3.5 animate-spin" />
              ) : compact ? (
                <RotateCw />
              ) : null}
              {!compact && "Restart"}
            </Button>
            <Button
              type="button"
              variant={compact ? "ghost" : "outline"}
              size="sm"
              className={compact ? "text-muted-foreground" : undefined}
              onClick={(e) => handleAction("stop", e)}
              disabled={loading}
            >
              {loading && actionType === "stop" && (
                <Loader2 className="size-3.5 animate-spin" />
              )}
              Stop
            </Button>
          </>
        ) : project.status === "building" ? (
          <Button
            type="button"
            variant="outline"
            size="sm"
            className="text-danger"
            onClick={(e) => handleAction("cancel", e)}
            disabled={loading}
          >
            {loading && actionType === "cancel" && (
              <Loader2 className="size-3.5 animate-spin" />
            )}
            Cancel
          </Button>
        ) : (
          <Button
            type="button"
            variant="outline"
            size="sm"
            onClick={(e) => handleAction("deploy", e)}
            disabled={loading}
          >
            {loading && actionType === "deploy" && (
              <Loader2 className="size-3.5 animate-spin" />
            )}
            Deploy
          </Button>
        )}

        <Button
          type="button"
          variant={compact ? "ghost" : "outline"}
          size={compact ? "icon" : "sm"}
          className={
            compact ? "text-subtle hover:text-danger" : "text-danger"
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
          {compact ? <Trash2 /> : "Delete"}
        </Button>
      </div>

      <Dialog open={deleteDialogOpen} onOpenChange={setDeleteDialogOpen}>
        <DialogContent onClick={(e) => e.stopPropagation()}>
          <DialogHeader>
            <DialogTitle>Delete project</DialogTitle>
            <DialogDescription>
              Delete{" "}
              <span className="font-mono text-foreground">{project.name}</span>?
              This also removes all of its containers and cannot be undone.
            </DialogDescription>
          </DialogHeader>
          <DialogFooter>
            <Button
              variant="outline"
              onClick={(e) => {
                e.stopPropagation();
                setDeleteDialogOpen(false);
              }}
              disabled={deleting}
            >
              Cancel
            </Button>
            <Button
              variant="destructive"
              onClick={handleDeleteConfirm}
              disabled={deleting}
            >
              {deleting && <Loader2 className="animate-spin" />}
              Delete project
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </>
  );
}
