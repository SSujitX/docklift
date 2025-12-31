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
        className="group relative flex flex-col sm:flex-row sm:items-center justify-between py-4 sm:py-5 px-4 sm:px-6 gap-4 sm:gap-6 bg-card hover:bg-secondary/20 border border-border/50 hover:border-cyan-500/30 rounded-xl transition-all duration-200 cursor-pointer overflow-hidden shadow-sm hover:shadow-cyan-500/5"
      >
        {/* Left Gradient Accent Border */}
        <div className="absolute left-0 top-0 bottom-0 w-1.5 bg-gradient-to-b from-cyan-400 to-purple-500" />

        {/* Top Row: Status + Name */}
        <div className="flex items-start sm:items-center gap-3 flex-1 min-w-0 pl-2">
          {/* Status Indicator (Dot) */}
          <div className={`h-3 w-3 rounded-full shrink-0 shadow-[0_0_8px_currentColor] mt-1.5 sm:mt-0 ${
             project.status === 'running' ? 'bg-emerald-500 text-emerald-500/50' :
             project.status === 'building' ? 'bg-amber-500 text-amber-500/50 animate-pulse' :
             project.status === 'error' ? 'bg-red-500 text-red-500/50' :
             'bg-slate-400 text-slate-400/50'
          }`} />

          {/* Identity Section */}
          <div className="flex-1 min-w-0 flex flex-col justify-center gap-1">
             <div className="flex flex-wrap items-center gap-2 sm:gap-3">
               <h3 className="text-lg sm:text-xl font-bold text-cyan-600 dark:text-cyan-400 truncate tracking-tight">
                 {project.name}
               </h3>
               
               {project.status === "running" && (
                  <span className="text-[10px] font-extrabold px-2 py-0.5 rounded bg-emerald-100 dark:bg-emerald-500/20 text-emerald-600 dark:text-emerald-400 uppercase tracking-widest shadow-sm">
                    Running
                  </span>
               )}
             </div>

             <div className="flex flex-wrap items-center gap-x-3 sm:gap-x-4 gap-y-1 text-[9px] sm:text-[10px] text-muted-foreground/60 font-medium tracking-tight">
               <div className="flex items-center gap-1.5">
                 <Calendar className="h-3 w-3" />
                 <span>Created: {new Date(project.created_at).toLocaleString([], { month: 'short', day: 'numeric', year: 'numeric', hour: '2-digit', minute: '2-digit' })}</span>
               </div>
               <div className="flex items-center gap-1.5">
                 <Clock className="h-3 w-3" />
                 <span>Updated: {new Date(project.updated_at).toLocaleString([], { month: 'short', day: 'numeric', year: 'numeric', hour: '2-digit', minute: '2-digit' })}</span>
               </div>
             </div>
             
             {project.domain && (
               <div className="flex items-center gap-1.5 text-xs text-muted-foreground/80 hover:text-cyan-500 transition-colors mt-1">
                 <Globe className="h-3 w-3" />
                 <span className="truncate max-w-[200px]">{project.domain}</span>
               </div>
             )}
          </div>
        </div>

        {/* Meta Info Section */}
        <div className="flex flex-wrap items-center gap-2 sm:gap-3 text-sm text-muted-foreground pl-2 sm:pl-0">
           <div className="flex items-center gap-2 px-3 py-1.5 rounded-lg bg-secondary/40 border border-border/50">
             <GitBranch className="h-3.5 w-3.5 opacity-70" />
             <span className="font-mono text-xs opacity-90">{project.github_branch}</span>
           </div>
           
           {project.port && (
             <div className="flex items-center gap-2 px-3 py-1.5 rounded-lg bg-secondary/40 border border-border/50 font-mono text-xs">
               <Server className="h-3.5 w-3.5 opacity-70" />
               <span>:{project.port}</span>
             </div>
           )}
        </div>

        {/* Actions Section */}
        <div className="flex items-center gap-2 sm:gap-3 pl-2 sm:pl-0" onClick={(e) => e.stopPropagation()}>
            {project.status === "running" ? (
              <>
                 <Button
                   size="icon"
                   variant="secondary"
                   className="h-11 w-11 rounded-2xl !bg-slate-100 dark:!bg-slate-800 text-slate-600 dark:text-slate-300 border border-slate-200 dark:border-slate-700 shadow-lg shadow-slate-300/30 dark:shadow-black/30 hover:!bg-gradient-to-br hover:!from-indigo-500 hover:!to-purple-600 hover:text-white hover:border-transparent hover:shadow-indigo-500/40 transition-all duration-300"
                   onClick={(e) => handleAction("restart", e)}
                   disabled={loading}
                   title="Restart"
                 >
                   {loading && actionType === "restart" ? <Loader2 className="h-5 w-5 animate-spin" /> : <RotateCw className="h-5 w-5" />}
                 </Button>
                 <Button
                   size="icon"
                   variant="secondary"
                   className="h-11 w-11 rounded-2xl !bg-slate-100 dark:!bg-slate-800 text-slate-600 dark:text-slate-300 border border-slate-200 dark:border-slate-700 shadow-lg shadow-slate-300/30 dark:shadow-black/30 hover:!bg-gradient-to-br hover:!from-indigo-500 hover:!to-purple-600 hover:text-white hover:border-transparent hover:shadow-indigo-500/40 transition-all duration-300"
                   onClick={(e) => handleAction("stop", e)}
                   disabled={loading}
                   title="Stop"
                 >
                   {loading && actionType === "stop" ? <Loader2 className="h-5 w-5 animate-spin" /> : <Square className="h-4 w-4 fill-current" />}
                 </Button>
              </>
            ) : project.status === "building" ? (
               <Button
                 size="sm"
                 variant="secondary"
                 className="rounded-2xl h-10 px-5 text-xs gap-2 !bg-slate-100 dark:!bg-slate-800 text-red-500 border border-slate-200 dark:border-slate-700 shadow-lg hover:!bg-red-500 hover:text-white hover:border-transparent transition-all duration-300"
                 onClick={(e) => handleAction("cancel", e)}
                 disabled={loading}
               >
                 <XCircle className="h-4 w-4" />
                 Cancel
               </Button>
            ) : (
                <Button
                 size="sm"
                 variant="secondary"
                 className="rounded-2xl h-11 px-6 text-sm font-bold gap-2 !bg-slate-100 dark:!bg-slate-800 text-cyan-600 dark:text-cyan-400 border border-slate-200 dark:border-slate-700 shadow-lg shadow-slate-300/30 dark:shadow-black/30 hover:!bg-gradient-to-br hover:!from-cyan-500 hover:!to-blue-600 hover:text-white hover:border-transparent hover:shadow-cyan-500/40 transition-all duration-300"
                 onClick={(e) => handleAction("deploy", e)}
                 disabled={loading}
               >
                 {loading && actionType === "deploy" ? <Loader2 className="h-4 w-4 animate-spin" /> : <Play className="h-4 w-4 fill-current" />}
                 Deploy
               </Button>
            )}

            <div className="w-px h-8 bg-border/30 mx-1 hidden sm:block" />

            <Button
               size="icon"
               variant="secondary"
               className="h-11 w-11 rounded-2xl !bg-slate-100 dark:!bg-slate-800 text-slate-400 dark:text-slate-500 border border-slate-200 dark:border-slate-700 shadow-lg shadow-slate-300/30 dark:shadow-black/30 hover:!bg-gradient-to-br hover:!from-red-500 hover:!to-rose-600 hover:text-white hover:border-transparent hover:shadow-red-500/40 transition-all duration-300"
               onClick={handleDeleteClick}
               title="Delete"
             >
               <Trash2 className="h-5 w-5" />
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
