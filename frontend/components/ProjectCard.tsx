// ProjectCard component - displays project info with deploy/stop/restart/delete actions
"use client";

import { Project } from "@/lib/types";
import { Card } from "./ui/card";
import { StatusBadge } from "./StatusBadge";
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
  Upload, 
  Play, 
  Square, 
  RotateCw, 
  Trash2, 
  ExternalLink, 
  Loader2, 
  XCircle,
  Globe,
  Server,
  AlertTriangle,
  Calendar,
  Clock
} from "lucide-react";
import { API_URL } from "@/lib/utils";
import { getAuthHeaders } from "@/lib/auth";
import { useState } from "react";
import { useRouter } from "next/navigation";

interface ProjectCardProps {
  project: Project;
  onRefresh: () => void;
}

export function ProjectCard({ project, onRefresh }: ProjectCardProps) {
  const [loading, setLoading] = useState(false);
  const [actionType, setActionType] = useState<string | null>(null);
  const [deleteDialogOpen, setDeleteDialogOpen] = useState(false);
  const [deleting, setDeleting] = useState(false);
  const router = useRouter();

  const handleAction = async (action: "deploy" | "stop" | "restart" | "cancel", e: React.MouseEvent) => {
    e.preventDefault();
    e.stopPropagation();
    setLoading(true);
    setActionType(action);
    try {
      await fetch(`${API_URL}/api/deployments/${project.id}/${action}`, { method: "POST", headers: getAuthHeaders() });
      setTimeout(onRefresh, 2000);
    } catch (error) {
      console.error(error);
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
      await fetch(`${API_URL}/api/projects/${project.id}`, { method: "DELETE", headers: getAuthHeaders() });
      setDeleteDialogOpen(false);
      onRefresh();
    } catch (error) {
      console.error(error);
    } finally {
      setDeleting(false);
    }
  };

  const handleCardClick = () => {
    router.push(`/projects/${project.id}`);
  };

  const hostname = typeof window !== 'undefined' ? window.location.hostname : 'localhost';

  return (
    <>
      <div 
        onClick={handleCardClick}
        role="link"
        tabIndex={0}
        onKeyDown={(e) => {
          if (e.key === 'Enter' || e.key === ' ') {
            e.preventDefault();
            handleCardClick();
          }
        }}
        className="group relative flex flex-col md:flex-row md:items-center justify-between p-4 gap-4 bg-card hover:bg-secondary/40 border border-border/60 dark:border-white/5 hover:border-foreground/20 dark:hover:border-white/20 rounded-xl transition-all duration-200 cursor-pointer shadow-sm hover:shadow-md"
      >
        {/* Main Info Section */}
        <div className="flex items-center gap-4 flex-1 min-w-0">
          <div className={`h-2 w-2 rounded-full shrink-0 shadow-sm ${
             project.status === 'running' ? 'bg-emerald-500 shadow-emerald-500/50' :
             project.status === 'building' ? 'bg-amber-500 shadow-amber-500/50 animate-pulse' :
             project.status === 'error' ? 'bg-red-500 shadow-red-500/50' :
             'bg-zinc-400'
          }`} />

          <div className="flex-1 min-w-0 grid gap-1">
             <div className="flex items-center gap-3">
               <h3 className="text-base font-bold tracking-tight text-foreground/90 group-hover:text-foreground transition-colors">
                 {project.name}
               </h3>
               
               {project.status === "running" ? (
                  <span className="text-[10px] font-bold px-2 py-0.5 rounded-md bg-emerald-500/10 text-emerald-600 dark:text-emerald-400 border border-emerald-500/20 uppercase tracking-wide">
                    Running
                  </span>
               ) : project.status === "building" ? (
                  <span className="text-[10px] font-bold px-2 py-0.5 rounded-md bg-amber-500/10 text-amber-600 dark:text-amber-400 border border-amber-500/20 uppercase tracking-wide">
                    Building
                  </span>
               ) : project.status === "error" ? (
                  <span className="text-[10px] font-bold px-2 py-0.5 rounded-md bg-red-500/10 text-red-600 dark:text-red-400 border border-red-500/20 uppercase tracking-wide">
                    Error
                  </span>
               ) : (
                  <span className="text-[10px] font-bold px-2 py-0.5 rounded-md bg-zinc-500/10 text-zinc-500 dark:text-zinc-400 border border-zinc-500/20 uppercase tracking-wide">
                    Stopped
                  </span>
               )}
             </div>

             <div className="flex flex-wrap items-center gap-3 text-xs text-muted-foreground font-medium">
               <div className="flex items-center gap-1.5 text-foreground/70">
                 <GitBranch className="h-3.5 w-3.5" />
                 <span>{project.github_branch}</span>
               </div>
               
               {project.port && (
                 <div className="flex items-center gap-1.5 text-foreground/70">
                   <Server className="h-3.5 w-3.5" />
                   <span>:{project.port}</span>
                 </div>
               )}
               
               <span className="text-border mx-1 hidden sm:inline">|</span>
               
               <div className="flex flex-wrap items-center gap-3 text-[11px] text-muted-foreground">
                 <span title={new Date(project.created_at).toLocaleString()}>Created {new Date(project.created_at).toLocaleDateString()}</span>
                 <span className="hidden sm:inline">•</span>
                 <span title={new Date(project.updated_at).toLocaleString()}>Updated {new Date(project.updated_at).toLocaleDateString()}</span>
               </div>
             </div>
          </div>
        </div>

        {/* Actions Section */}
        <div className="flex items-center gap-2 pl-6 md:pl-0 border-t md:border-t-0 border-border/40 pt-3 md:pt-0 mt-1 md:mt-0" onClick={(e) => e.stopPropagation()}>
            {project.status === "running" ? (
              <>
                 <Button
                   size="icon"
                   variant="ghost"
                   className="h-8 w-8 rounded-lg bg-secondary/70 text-muted-foreground hover:text-cyan-500 hover:bg-cyan-500/10 dark:hover:text-cyan-400 dark:hover:bg-cyan-500/20 border border-border/30 hover:border-cyan-500/20 transition-all hover:scale-105"
                   onClick={(e) => handleAction("restart", e)}
                   disabled={loading}
                   title="Restart"
                 >
                   {loading && actionType === "restart" ? <Loader2 className="h-4 w-4 animate-spin" /> : <RotateCw className="h-4 w-4" />}
                 </Button>
                 <Button
                   size="icon"
                   variant="ghost"
                   className="h-8 w-8 rounded-lg bg-secondary/70 text-muted-foreground hover:text-red-600 hover:bg-red-500/20 dark:hover:text-red-400 dark:hover:bg-red-500/30 border border-border/30 hover:border-red-500/20 transition-all hover:scale-105"
                   onClick={(e) => handleAction("stop", e)}
                   disabled={loading}
                   title="Stop"
                 >
                   {loading && actionType === "stop" ? <Loader2 className="h-4 w-4 animate-spin" /> : <Square className="h-3.5 w-3.5 fill-current" />}
                 </Button>
              </>
            ) : project.status === "building" ? (
               <Button
                 size="sm"
                 variant="outline"
                 className="rounded-lg h-8 px-3 text-xs font-semibold gap-1.5 border-red-500/20 bg-red-500/5 text-red-600 hover:bg-red-500/10 transition-colors"
                 onClick={(e) => handleAction("cancel", e)}
                 disabled={loading}
               >
                 <XCircle className="h-3.5 w-3.5" />
                 Cancel
               </Button>
            ) : (
                <Button
                 size="sm"
                 className="rounded-lg h-8 px-4 text-xs font-semibold gap-1.5 bg-zinc-900 hover:bg-zinc-800 hover:scale-105 active:scale-95 text-white shadow-sm hover:shadow-lg transition-all duration-200"
                 onClick={(e) => handleAction("deploy", e)}
                 disabled={loading}
               >
                 {loading && actionType === "deploy" ? <Loader2 className="h-3 w-3 animate-spin" /> : <Play className="h-3 w-3 fill-current" />}
                 Deploy
               </Button>
            )}

            <div className="w-px h-5 bg-border/40 mx-1 hidden md:block" />

            <Button
               size="icon"
               variant="ghost"
               className="h-8 w-8 rounded-lg bg-secondary/40 text-muted-foreground/70 hover:text-red-600 hover:bg-red-500/20 dark:hover:text-red-400 dark:hover:bg-red-500/30 border border-transparent hover:border-red-500/20 transition-all duration-200 hover:scale-110"
               onClick={handleDeleteClick}
               title="Delete"
             >
               <Trash2 className="h-3.5 w-3.5" />
             </Button>
        </div>
      </div>

      {/* Delete Confirmation Dialog */}
      <Dialog open={deleteDialogOpen} onOpenChange={setDeleteDialogOpen}>
        <DialogContent className="sm:max-w-[400px]" onClick={(e) => e.stopPropagation()}>
          <DialogHeader>
            <div className="mx-auto mb-4 h-12 w-12 rounded-full bg-red-500/10 flex items-center justify-center">
              <AlertTriangle className="h-6 w-6 text-red-500" />
            </div>
            <DialogTitle className="text-center">Delete Project</DialogTitle>
            <DialogDescription className="text-center">
              Are you sure you want to delete <span className="font-semibold text-foreground">{project.name}</span>?
              <br />
              <span className="text-red-500">This will also remove all containers and cannot be undone.</span>
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
