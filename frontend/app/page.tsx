// Dashboard page - displays all projects with status, actions, and navigation
"use client";

import { useEffect, useState, useCallback, useRef } from "react";
import { Header } from "@/components/Header";
import { Footer } from "@/components/Footer";
import { ProjectCard } from "@/components/ProjectCard";
import { Button } from "@/components/ui/button";
import { Project } from "@/lib/types";
import { API_URL } from "@/lib/utils";
import { getAuthHeaders } from "@/lib/auth";
import { Plus, RefreshCw, Container, Sparkles } from "lucide-react";
import { useRouter } from "next/navigation";

export default function Dashboard() {
  const [projects, setProjects] = useState<Project[]>([]);
  const [loading, setLoading] = useState(true);
  const router = useRouter();

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

  // Use a ref for buildingCount so the polling interval adapts without re-triggering the effect
  const buildingCountRef = useRef(buildingCount);
  buildingCountRef.current = buildingCount;

  useEffect(() => {
    fetchProjects();
    // Poll faster (3s) when building, slower (10s) otherwise — checked dynamically via ref
    const interval = setInterval(() => {
      fetchProjects();
    }, buildingCountRef.current > 0 ? 3000 : 10000);
    return () => clearInterval(interval);
  }, [fetchProjects]);

  return (
    <div className="min-h-screen flex flex-col">
      <Header />

      <main className="flex-1 container max-w-7xl mx-auto px-4 md:px-6 py-8 sm:py-12">
        <div className="flex flex-col md:flex-row md:items-end justify-between gap-6 mb-10">
          <div>
            <h1 className="text-3xl font-bold tracking-tight mb-4">Projects</h1>
            <div className="flex flex-wrap gap-3">
              <div className="px-4 py-2 rounded-xl bg-secondary/50 dark:bg-zinc-900/50 border border-border/50 dark:border-white/10 text-sm font-medium shadow-sm">
                <span className="text-muted-foreground mr-2">Total</span>
                <span className="font-bold text-foreground">{projects.length}</span>
              </div>
              <div className="px-4 py-2 rounded-xl bg-emerald-500/10 dark:bg-emerald-500/5 border border-emerald-500/20 dark:border-emerald-500/30 text-sm font-medium text-emerald-600 dark:text-emerald-400 shadow-sm shadow-emerald-500/5">
                <span className="mr-2 opacity-70">Running</span>
                <span className="font-bold">{runningCount}</span>
              </div>
              {buildingCount > 0 && (
                <div className="px-4 py-2 rounded-xl bg-amber-500/10 dark:bg-amber-500/5 border border-amber-500/20 dark:border-amber-500/30 text-sm font-medium text-amber-600 dark:text-amber-400 shadow-sm shadow-amber-500/5 animate-pulse">
                  <span className="mr-2 opacity-70">Building</span>
                  <span className="font-bold">{buildingCount}</span>
                </div>
              )}
              <div className="px-4 py-2 rounded-xl bg-secondary/50 dark:bg-zinc-900/50 border border-border/50 dark:border-white/10 text-sm font-medium shadow-sm">
                <span className="text-muted-foreground mr-2 opacity-70">Stopped</span>
                <span className="font-bold text-foreground">{stoppedCount}</span>
              </div>
            </div>
          </div>
          
          <div className="flex items-center gap-3">
            <Button 
              variant="outline" 
              size="icon" 
              onClick={fetchProjects}
              className="h-10 w-10 rounded-xl border-border/60 hover:bg-secondary/80 bg-background"
            >
              <RefreshCw className="h-4 w-4 text-muted-foreground" />
            </Button>
            <Button 
              onClick={() => router.push("/projects/new")} 
              className="h-10 px-6 rounded-xl bg-zinc-900 hover:bg-zinc-800 text-white shadow-lg shadow-zinc-900/10 font-medium"
            >
              <Plus className="h-4 w-4 mr-2" />
              New Project
            </Button>
          </div>
        </div>

        {loading ? (
          <div className="flex flex-col gap-4">
            {[1, 2, 3].map((i) => (
              <div key={i} className="h-28 rounded-2xl bg-secondary/20 border border-border/40 animate-pulse" />
            ))}
          </div>
        ) : projects.length === 0 ? (
          <div className="flex flex-col items-center justify-center py-20 px-4 text-center">
            <div className="h-24 w-24 rounded-[32px] bg-secondary/30 flex items-center justify-center mb-6">
              <Container className="h-10 w-10 text-muted-foreground/40" />
            </div>
            <h2 className="text-2xl font-bold mb-3">No projects found</h2>
            <p className="text-muted-foreground mb-8 max-w-sm leading-relaxed">
              Get started by creating your first project. We'll handle the build and deployment for you.
            </p>
            <Button 
              onClick={() => router.push("/projects/new")} 
              size="lg" 
              className="h-12 px-8 rounded-2xl bg-zinc-900 hover:bg-zinc-800 text-white shadow-xl shadow-zinc-900/10 font-bold"
            >
              <Sparkles className="h-4 w-4 mr-2" />
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
      </main>

      <Footer />
    </div>
  );
}
