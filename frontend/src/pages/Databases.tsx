// Databases page - project list filtered to managed data services

import { useEffect, useState, useCallback } from "react";
import { PageHeader, StatChip } from "@/components/shell/PageHeader";
import { ProjectCard } from "@/components/ProjectCard";
import { Button } from "@/components/ui/button";
import { useNavigate } from "react-router-dom";
import { Project } from "@/lib/types";
import { API_URL } from "@/lib/utils";
import { authFetch } from "@/lib/auth";
import { Plus, RefreshCw, Database, Sparkles } from "lucide-react";

export default function DatabasesPage() {
  const [projects, setProjects] = useState<Project[]>([]);
  const [loading, setLoading] = useState(true);
  const navigate = useNavigate();

  const fetchProjects = useCallback(async () => {
    try {
      const res = await authFetch(`${API_URL}/api/projects`);
      const data = await res.json();
      // Filter only database projects
      const databaseProjects = Array.isArray(data) ? data.filter((p: Project) => p.project_type === "database") : [];
      setProjects(databaseProjects);
    } catch (error) {
      console.error("Failed to fetch database services:", error);
    } finally {
      setLoading(false);
    }
  }, []);

  useEffect(() => {
    fetchProjects();
    const interval = setInterval(fetchProjects, 10000);
    return () => clearInterval(interval);
  }, [fetchProjects]);

  const runningCount = projects.filter((p) => p.status === "running").length;
  const stoppedCount = projects.filter((p) => p.status === "stopped").length;

  return (
    <>
      <PageHeader
        eyebrow="Deploy"
        title="Databases"
        description="Dedicated data services running alongside your applications."
        icon={Database}
        meta={
          <>
            <StatChip label="Services" value={projects.length} />
            <StatChip label="Running" value={runningCount} tone="success" />
            <StatChip label="Stopped" value={stoppedCount} />
          </>
        }
        actions={
          <>
            <Button
              variant="outline"
              size="icon"
              onClick={fetchProjects}
              title="Refresh databases"
              className="h-10 w-10 border-border/60 bg-background hover:bg-secondary/80"
            >
              <RefreshCw className="h-4 w-4 text-muted-foreground" />
            </Button>
            <Button
              onClick={() => navigate("/projects/new")}
              className="h-10 bg-brand px-5 font-semibold text-brand-foreground shadow-lg shadow-brand/20 hover:brightness-110"
            >
              <Plus className="h-4 w-4" />
              New Database
            </Button>
          </>
        }
      />

      {loading ? (
        <div className="grid grid-cols-1 gap-5 md:grid-cols-2 lg:grid-cols-3">
          {[1, 2, 3].map((i) => (
            <div key={i} className="h-44 rounded-2xl bg-card border border-border shimmer" />
          ))}
        </div>
      ) : projects.length === 0 ? (
        <div className="flex flex-col items-center justify-center rounded-3xl border border-dashed border-border/60 px-4 py-20 text-center">
          <div className="mb-6 flex h-20 w-20 items-center justify-center rounded-[28px] border border-blue-500/20 bg-blue-500/10">
            <Database className="h-9 w-9 text-blue-500" />
          </div>
          <h2 className="mb-3 text-2xl font-bold">No databases yet</h2>
          <p className="mb-8 max-w-md leading-relaxed text-muted-foreground">
            Create a dedicated database service. PostgreSQL, MySQL, Redis and more run
            from standard Docker images.
          </p>
          <Button
            onClick={() => navigate("/projects/new")}
            size="lg"
            className="h-12 bg-gradient-to-r from-blue-500 to-indigo-600 px-8 font-semibold text-white shadow-xl shadow-blue-500/20 hover:brightness-110"
          >
            <Sparkles className="h-4 w-4" />
            Launch Database Service
          </Button>
        </div>
      ) : (
        <div className="grid grid-cols-1 gap-5 md:grid-cols-2 lg:grid-cols-3">
          {projects.map((project) => (
            <ProjectCard key={project.id} project={project} onRefresh={fetchProjects} />
          ))}
        </div>
      )}
    </>
  );
}
