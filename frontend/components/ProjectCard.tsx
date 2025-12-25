"use client";

import { Project } from "@/lib/types";
import { Card } from "./ui/card";
import { StatusBadge } from "./StatusBadge";
import { Button } from "./ui/button";
import { GitBranch, Upload, Play, Square, RotateCw, Trash2, ArrowUpRight, Loader2, XCircle } from "lucide-react";
import Link from "next/link";
import { API_URL } from "@/lib/utils";
import { useState } from "react";

interface ProjectCardProps {
  project: Project;
  onRefresh: () => void;
}

export function ProjectCard({ project, onRefresh }: ProjectCardProps) {
  const [loading, setLoading] = useState(false);

  const handleAction = async (action: "deploy" | "stop" | "restart" | "cancel", e: React.MouseEvent) => {
    e.preventDefault();
    e.stopPropagation();
    setLoading(true);
    try {
      await fetch(`${API_URL}/api/deployments/${project.id}/${action}`, { method: "POST" });
      setTimeout(onRefresh, 2000);
    } catch (error) {
      console.error(error);
    } finally {
      setLoading(false);
    }
  };

  const handleDelete = async (e: React.MouseEvent) => {
    e.preventDefault();
    e.stopPropagation();
    if (!confirm(`Delete "${project.name}"?`)) return;
    
    try {
      await fetch(`${API_URL}/api/projects/${project.id}`, { method: "DELETE" });
      onRefresh();
    } catch (error) {
      console.error(error);
    }
  };

  return (
    <Link href={`/projects/${project.id}`}>
      <Card className="group relative overflow-hidden hover:border-primary/50 cursor-pointer">
        <div className="absolute inset-0 bg-gradient-to-br from-violet-500/5 to-purple-500/5 opacity-0 group-hover:opacity-100 transition-opacity" />
        
        <div className="relative p-5">
          <div className="flex items-start justify-between gap-3 mb-4">
            <div className="min-w-0 flex-1">
              <div className="flex items-center gap-2 mb-1">
                <h3 className="font-semibold text-base truncate group-hover:text-primary transition-colors">
                  {project.name}
                </h3>
                <ArrowUpRight className="h-4 w-4 text-muted-foreground opacity-0 group-hover:opacity-100 transition-opacity" />
              </div>
              {project.description && (
                <p className="text-sm text-muted-foreground line-clamp-1">{project.description}</p>
              )}
            </div>
            <StatusBadge status={project.status} size="sm" />
          </div>

          <div className="flex items-center gap-3 text-xs text-muted-foreground mb-4">
            <span className="inline-flex items-center gap-1.5 bg-secondary/50 px-2.5 py-1 rounded-lg">
              {project.source_type === "github" ? (
                <>
                  <GitBranch className="h-3.5 w-3.5" />
                  {project.github_branch}
                </>
              ) : (
                <>
                  <Upload className="h-3.5 w-3.5" />
                  Upload
                </>
              )}
            </span>
            <span className="inline-flex items-center gap-1.5 font-mono text-xs bg-gradient-to-r from-violet-500/10 to-purple-500/10 text-violet-600 dark:text-violet-400 px-2.5 py-1 rounded-lg border border-violet-500/20">
              <span className="w-1.5 h-1.5 rounded-full bg-violet-500 animate-pulse" />
              Port {project.port}
            </span>
          </div>

          <div className="flex items-center gap-2">
            {project.status === "running" ? (
              <>
                <Button
                  size="sm"
                  variant="secondary"
                  className="flex-1"
                  onClick={(e) => handleAction("restart", e)}
                  disabled={loading}
                >
                  {loading ? <Loader2 className="h-4 w-4 animate-spin" /> : <RotateCw className="h-4 w-4" />}
                  Restart
                </Button>
                <Button
                  size="sm"
                  variant="outline"
                  className="flex-1"
                  onClick={(e) => handleAction("stop", e)}
                  disabled={loading}
                >
                  <Square className="h-4 w-4" />
                  Stop
                </Button>
              </>
            ) : project.status === "building" ? (
              <Button
                size="sm"
                variant="destructive"
                className="flex-1"
                onClick={(e) => handleAction("cancel", e)}
                disabled={loading}
              >
                {loading ? <Loader2 className="h-4 w-4 animate-spin" /> : <XCircle className="h-4 w-4" />}
                Cancel
              </Button>
            ) : (
              <Button
                size="sm"
                variant="success"
                className="flex-1"
                onClick={(e) => handleAction("deploy", e)}
                disabled={loading}
              >
                {loading ? <Loader2 className="h-4 w-4 animate-spin" /> : <Play className="h-4 w-4" />}
                Deploy
              </Button>
            )}
            
            <Button
              size="sm"
              variant="ghost"
              className="text-muted-foreground hover:text-destructive opacity-0 group-hover:opacity-100 transition-opacity"
              onClick={handleDelete}
            >
              <Trash2 className="h-4 w-4" />
            </Button>
          </div>
        </div>
      </Card>
    </Link>
  );
}
