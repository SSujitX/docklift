"use client";

import { useEffect, useState, useCallback } from "react";
import { Header } from "@/components/Header";
import { Footer } from "@/components/Footer";
import { ProjectCard } from "@/components/ProjectCard";
import { CreateProjectModal } from "@/components/CreateProjectModal";
import { Button } from "@/components/ui/button";
import { Project } from "@/lib/types";
import { API_URL } from "@/lib/utils";
import { Plus, RefreshCw, Container, Sparkles } from "lucide-react";

export default function Dashboard() {
  const [projects, setProjects] = useState<Project[]>([]);
  const [loading, setLoading] = useState(true);
  const [createModalOpen, setCreateModalOpen] = useState(false);

  const fetchProjects = useCallback(async () => {
    try {
      const res = await fetch(`${API_URL}/api/projects`);
      const data = await res.json();
      setProjects(data);
    } catch (error) {
      console.error("Failed to fetch projects:", error);
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
    <div className="min-h-screen flex flex-col">
      <Header />

      <main className="flex-1 container max-w-7xl mx-auto px-4 md:px-6 py-8">
        <div className="flex flex-col sm:flex-row sm:items-center justify-between gap-4 mb-8">
          <div>
            <h1 className="text-3xl font-bold tracking-tight">Projects</h1>
            <p className="text-muted-foreground mt-1">
              {projects.length} total · <span className="text-emerald-500">{runningCount} running</span> · {stoppedCount} stopped
            </p>
          </div>
          
          <div className="flex items-center gap-3">
            <Button 
              variant="outline" 
              size="icon" 
              onClick={fetchProjects}
              className="hover:bg-secondary"
            >
              <RefreshCw className="h-4 w-4" />
            </Button>
            <Button 
              onClick={() => setCreateModalOpen(true)} 
              className="gap-2 bg-gradient-to-r from-cyan-500 to-blue-600 hover:from-cyan-600 hover:to-blue-700 text-white shadow-lg shadow-cyan-500/25"
            >
              <Plus className="h-4 w-4" />
              New Project
            </Button>
          </div>
        </div>

        {loading ? (
          <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 gap-5">
            {[1, 2, 3].map((i) => (
              <div key={i} className="h-44 rounded-2xl bg-card border border-border shimmer" />
            ))}
          </div>
        ) : projects.length === 0 ? (
          <div className="flex flex-col items-center justify-center py-24">
            <div className="relative mb-6">
              <div className="absolute inset-0 bg-gradient-to-br from-cyan-500/30 to-blue-500/30 rounded-3xl blur-2xl" />
              <div className="relative bg-gradient-to-br from-card to-secondary p-8 rounded-3xl border border-border shadow-xl">
                <Container className="h-12 w-12 text-cyan-500 animate-float" />
              </div>
            </div>
            <h2 className="text-2xl font-bold mb-2">No projects yet</h2>
            <p className="text-muted-foreground text-center mb-8 max-w-md">
              Deploy your first app in seconds. Just upload your project with a Dockerfile and we handle the rest.
            </p>
            <Button 
              onClick={() => setCreateModalOpen(true)} 
              size="lg" 
              className="gap-2 bg-gradient-to-r from-cyan-500 to-blue-600 hover:from-cyan-600 hover:to-blue-700 text-white shadow-lg shadow-cyan-500/25"
            >
              <Sparkles className="h-5 w-5" />
              Create Your First Project
            </Button>
          </div>
        ) : (
          <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 gap-5">
            {projects.map((project) => (
              <ProjectCard key={project.id} project={project} onRefresh={fetchProjects} />
            ))}
          </div>
        )}
      </main>

      <Footer />

      <CreateProjectModal
        open={createModalOpen}
        onOpenChange={setCreateModalOpen}
        onSuccess={fetchProjects}
      />
    </div>
  );
}
