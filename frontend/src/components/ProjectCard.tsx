// ProjectCard component - displays project info with deploy/stop/restart/delete actions

import { Project } from "@/lib/types";
import { Button } from "./ui/button";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "./ui/dialog";
import {
  GitBranch,
  Play,
  Square,
  RotateCw,
  Trash2,
  Loader2,
  XCircle,
  Server,
  AlertTriangle,
} from "lucide-react";
import { API_URL } from "@/lib/utils";
import { authFetch } from "@/lib/auth";
import { useState } from "react";
import { useNavigate } from "react-router-dom";
import { toast } from "sonner";

interface ProjectCardProps {
  project: Project;
  onRefresh: () => void;
}

const CARD_ACTION =
  "h-8 gap-1.5 rounded-lg px-2.5 text-xs font-medium shadow-none border transition-colors";

const CARD_ACTION_PRIMARY =
  `${CARD_ACTION} bg-brand text-brand-foreground border-transparent hover:bg-brand hover:brightness-110 hover:text-brand-foreground`;

const CARD_ACTION_SECONDARY =
  `${CARD_ACTION} bg-background text-foreground border-border hover:bg-secondary hover:text-foreground`;

const CARD_ACTION_DANGER =
  `${CARD_ACTION} bg-background text-destructive border-border hover:bg-destructive/10 hover:text-destructive hover:border-destructive/35`;

export function ProjectCard({ project, onRefresh }: ProjectCardProps) {
  const [loading, setLoading] = useState(false);
  const [actionType, setActionType] = useState<string | null>(null);
  const [deleteDialogOpen, setDeleteDialogOpen] = useState(false);
  const [deleting, setDeleting] = useState(false);
  const navigate = useNavigate();

  const handleAction = async (
    action: "deploy" | "redeploy" | "stop" | "restart" | "cancel",
    e: React.MouseEvent,
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

  const handleDeleteClick = (e: React.MouseEvent) => {
    e.preventDefault();
    e.stopPropagation();
    setDeleteDialogOpen(true);
  };

  const handleDeleteConfirm = async (e?: React.MouseEvent) => {
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

  const handleCardClick = () => {
    navigate(`/projects/${project.id}`);
  };

  const isLive =
    project.status === "running" || project.status === "degraded";

  return (
    <>
      <div
        onClick={handleCardClick}
        role="link"
        tabIndex={0}
        onKeyDown={(e) => {
          if (e.key === "Enter" || e.key === " ") {
            e.preventDefault();
            handleCardClick();
          }
        }}
        className="group relative flex flex-col gap-4 rounded-xl border border-border/60 bg-card p-4 shadow-sm transition-all duration-200 hover:border-foreground/20 hover:bg-secondary/40 hover:shadow-md dark:border-white/5 dark:hover:border-white/20 md:flex-row md:items-center md:justify-between"
      >
        <div className="flex min-w-0 flex-1 items-center gap-4">
          <div
            className={`h-2 w-2 shrink-0 rounded-full shadow-sm ${
              project.status === "running"
                ? "bg-emerald-500 shadow-emerald-500/50"
                : project.status === "building"
                  ? "animate-pulse bg-amber-500 shadow-amber-500/50"
                  : project.status === "error"
                    ? "bg-red-500 shadow-red-500/50"
                    : project.status === "degraded"
                      ? "bg-orange-500 shadow-orange-500/50"
                      : "bg-zinc-400"
            }`}
          />

          <div className="grid min-w-0 flex-1 gap-1">
            <div className="flex items-center gap-3">
              <h3 className="truncate text-base font-bold tracking-tight text-foreground/90 transition-colors group-hover:text-foreground">
                {project.name}
              </h3>

              {project.status === "running" ? (
                <span className="rounded-md border border-emerald-500/20 bg-emerald-500/10 px-2 py-0.5 text-[10px] font-bold uppercase tracking-wide text-emerald-600 dark:text-emerald-400">
                  Running
                </span>
              ) : project.status === "building" ? (
                <span className="rounded-md border border-amber-500/20 bg-amber-500/10 px-2 py-0.5 text-[10px] font-bold uppercase tracking-wide text-amber-600 dark:text-amber-400">
                  Building
                </span>
              ) : project.status === "error" ? (
                <span className="rounded-md border border-red-500/20 bg-red-500/10 px-2 py-0.5 text-[10px] font-bold uppercase tracking-wide text-red-600 dark:text-red-400">
                  Error
                </span>
              ) : project.status === "degraded" ? (
                <span className="rounded-md border border-orange-500/20 bg-orange-500/10 px-2 py-0.5 text-[10px] font-bold uppercase tracking-wide text-orange-600 dark:text-orange-400">
                  Degraded
                </span>
              ) : (
                <span className="rounded-md border border-zinc-500/20 bg-zinc-500/10 px-2 py-0.5 text-[10px] font-bold uppercase tracking-wide text-zinc-500 dark:text-zinc-400">
                  Stopped
                </span>
              )}
            </div>

            <div className="flex flex-wrap items-center gap-3 text-xs font-medium text-muted-foreground">
              <div className="flex items-center gap-1.5 text-foreground/70">
                <GitBranch className="h-3.5 w-3.5" />
                <span>{project.github_branch}</span>
              </div>

              {project.port ? (
                <div className="flex items-center gap-1.5 text-foreground/70">
                  <Server className="h-3.5 w-3.5" />
                  <span>:{project.port}</span>
                </div>
              ) : null}

              <span className="mx-1 hidden text-border sm:inline">|</span>

              <div className="flex flex-wrap items-center gap-3 text-[11px] text-muted-foreground">
                <span title={new Date(project.created_at).toLocaleString()}>
                  Created {new Date(project.created_at).toLocaleDateString()}
                </span>
                <span className="hidden sm:inline">•</span>
                <span title={new Date(project.updated_at).toLocaleString()}>
                  Updated {new Date(project.updated_at).toLocaleDateString()}
                </span>
              </div>
            </div>
          </div>
        </div>

        <div
          role="group"
          aria-label={`${project.name} actions`}
          className="flex flex-wrap items-center gap-1.5 border-t border-border/40 pt-3 md:border-t-0 md:pt-0"
          onClick={(e) => e.stopPropagation()}
        >
          {isLive ? (
            <>
              <Button
                type="button"
                variant="bare"
                size="sm"
                className={CARD_ACTION_PRIMARY}
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
                size="sm"
                className={CARD_ACTION_SECONDARY}
                onClick={(e) => handleAction("restart", e)}
                disabled={loading}
              >
                {loading && actionType === "restart" ? (
                  <Loader2 className="h-3.5 w-3.5 animate-spin" />
                ) : (
                  <RotateCw className="h-3.5 w-3.5" />
                )}
                Restart
              </Button>
              <Button
                type="button"
                variant="bare"
                size="sm"
                className={CARD_ACTION_SECONDARY}
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
              className={CARD_ACTION_DANGER}
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
              className={CARD_ACTION_PRIMARY}
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

          <div className="mx-0.5 hidden h-5 w-px bg-border/40 md:block" />

          <Button
            type="button"
            variant="bare"
            size="sm"
            className={CARD_ACTION_DANGER}
            onClick={handleDeleteClick}
            disabled={loading || deleting}
            title="Delete project"
          >
            <Trash2 className="h-3.5 w-3.5" />
            Delete
          </Button>
        </div>
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
