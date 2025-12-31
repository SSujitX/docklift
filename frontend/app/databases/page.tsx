"use client";

import { useEffect, useState, useCallback } from "react";
import { Header } from "@/components/Header";
import { Footer } from "@/components/Footer";
import { ProjectCard } from "@/components/ProjectCard";
import { Button } from "@/components/ui/button";
import { useRouter } from "next/navigation";
import { Project } from "@/lib/types";
import { API_URL } from "@/lib/utils";
import { getAuthHeaders } from "@/lib/auth";
import { Plus, RefreshCw, Database, Sparkles } from "lucide-react";

export default function DatabasesPage() {
  const [projects, setProjects] = useState<Project[]>([]);
  const [loading, setLoading] = useState(true);
  const router = useRouter();

  const fetchProjects = useCallback(async () => {
    try {
      const res = await fetch(`${API_URL}/api/projects`, { headers: getAuthHeaders() });
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
    <div className="min-h-screen flex flex-col">
      <Header />

      <main className="flex-1 container max-w-7xl mx-auto px-4 md:px-6 py-8">
        <div className="flex flex-col sm:flex-row sm:items-center justify-between gap-4 mb-8">
          <div>
            <h1 className="text-3xl font-bold tracking-tight flex items-center gap-3">
              <Database className="h-8 w-8 text-blue-500" />
              Databases
            </h1>
            <p className="text-muted-foreground mt-1">
              {projects.length} database services · <span className="text-emerald-500">{runningCount} running</span> · {stoppedCount} stopped
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
              onClick={() => router.push("/projects/new")} 
              className="gap-2 bg-gradient-to-r from-blue-500 to-indigo-600 hover:from-blue-600 hover:to-indigo-700 text-white shadow-lg shadow-blue-500/25"
            >
              <Plus className="h-4 w-4" />
              New Database
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
              <div className="absolute inset-0 bg-gradient-to-br from-blue-500/30 to-indigo-500/30 rounded-3xl blur-2xl" />
              <div className="relative bg-gradient-to-br from-card to-secondary p-8 rounded-3xl border border-border shadow-xl">
                <Database className="h-12 w-12 text-blue-500 animate-float" />
              </div>
            </div>
            <h2 className="text-2xl font-bold mb-2">No databases yet</h2>
            <p className="text-muted-foreground text-center mb-8 max-w-md">
              Create a dedicated database service. We support PostgreSQL, MySQL, Redis, and more through standard Docker images.
            </p>
            <Button 
              onClick={() => router.push("/projects/new")} 
              size="lg" 
              className="gap-2 bg-gradient-to-r from-blue-500 to-indigo-600 hover:from-blue-600 hover:to-indigo-700 text-white shadow-lg shadow-blue-500/25"
            >
              <Sparkles className="h-5 w-5" />
              Launch Database Service
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
    </div>
  );
}
