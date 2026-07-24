// Dashboard page - displays all projects with status, actions, and navigation

import { useEffect, useState, useCallback } from "react";
import { PageHeader, StatChip } from "@/components/shell/PageHeader";
import { ProjectCard } from "@/components/ProjectCard";
import { Button } from "@/components/ui/button";
import { Project } from "@/lib/types";
import { API_URL } from "@/lib/utils";
import { getAuthHeaders } from "@/lib/auth";
import { Plus, RefreshCw, Container, LayoutGrid, Sparkles } from "lucide-react";
import { useNavigate } from "react-router-dom";

export default function Dashboard() {
  const [projects, setProjects] = useState<Project[]>([]);
  const [loading, setLoading] = useState(true);
  const navigate = useNavigate();

  const fetchProjects = useCallback(async () => {
    try {
      const res = await fetch(`${API_URL}/api/projects`, {
        headers: getAuthHeaders(),
      });
      
      if (res.status === 401) {
        // Not authenticated, AuthProvider will handle redirect
        return;
      }
      
      const data = await res.json();
      // Ensure we always have an array
      setProjects(Array.isArray(data) ? data : []);
    } catch (error) {
      console.error("Failed to fetch projects:", error);
    } finally {
      setLoading(false);
    }
  }, []);

  const runningCount = projects.filter((p) => p.status === "running").length;
  const buildingCount = projects.filter((p) => p.status === "building").length;
  const stoppedCount = projects.filter((p) => p.status === "stopped").length;

  // Poll faster while any project is building; recreate interval when that changes
  const isBuilding = buildingCount > 0;

  useEffect(() => {
    fetchProjects();
    const interval = setInterval(() => {
      fetchProjects();
    }, isBuilding ? 3000 : 10000);
    return () => clearInterval(interval);
  }, [fetchProjects, isBuilding]);

  return (
    <>
      <PageHeader
        eyebrow="Deploy"
        title="Projects"
        description="Every application and database Docklift builds and runs on this server."
        icon={LayoutGrid}
        meta={
          <>
            <StatChip label="Total" value={projects.length} />
            <StatChip label="Running" value={runningCount} tone="success" />
            {buildingCount > 0 && (
              <StatChip label="Building" value={buildingCount} tone="warning" pulse />
            )}
            <StatChip label="Stopped" value={stoppedCount} />
          </>
        }
        actions={
          <>
            <Button
              variant="outline"
              size="icon"
              onClick={fetchProjects}
              title="Refresh projects"
              className="h-10 w-10 border-border/60 bg-background hover:bg-secondary/80"
            >
              <RefreshCw className="h-4 w-4 text-muted-foreground" />
            </Button>
            <Button
              onClick={() => navigate("/projects/new")}
              className="h-10 bg-brand px-5 font-semibold text-brand-foreground shadow-lg shadow-brand/20 hover:brightness-110"
            >
              <Plus className="h-4 w-4" />
              New Project
            </Button>
          </>
        }
      />

      {loading ? (
        <div className="flex flex-col gap-4">
          {[1, 2, 3].map((i) => (
            <div key={i} className="h-28 rounded-2xl bg-secondary/20 border border-border/40 animate-pulse" />
          ))}
        </div>
      ) : projects.length === 0 ? (
        <div className="flex flex-col items-center justify-center rounded-3xl border border-dashed border-border/60 px-4 py-20 text-center">
          <div className="mb-6 flex h-20 w-20 items-center justify-center rounded-[28px] border border-brand/20 bg-brand/10">
            <Container className="h-9 w-9 text-brand" />
          </div>
          <h2 className="mb-3 text-2xl font-bold">No projects yet</h2>
          <p className="mb-8 max-w-sm leading-relaxed text-muted-foreground">
            Connect a repository or upload your source. Docklift detects the build,
            creates the image, and keeps it running.
          </p>
          <Button
            onClick={() => navigate("/projects/new")}
            size="lg"
            className="h-12 bg-brand px-8 font-semibold text-brand-foreground shadow-xl shadow-brand/20 hover:brightness-110"
          >
            <Sparkles className="h-4 w-4" />
            Create Project
          </Button>
        </div>
      ) : (
        <div className="grid grid-cols-1 gap-4">
          {projects.map((project) => (
            <ProjectCard key={project.id} project={project} onRefresh={fetchProjects} />
          ))}
        </div>
      )}
    </>
  );
}
