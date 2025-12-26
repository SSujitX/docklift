"use client";

import { useEffect, useState, useCallback } from "react";
import { useParams, useRouter } from "next/navigation";
import { Header } from "@/components/Header";
import { Footer } from "@/components/Footer";
import { StatusBadge } from "@/components/StatusBadge";
import { Terminal } from "@/components/Terminal";
import { FileEditor } from "@/components/FileEditor";
import { FileTree } from "@/components/FileTree";
import { Button } from "@/components/ui/button";
import { Card } from "@/components/ui/card";
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs";
import { Project, ProjectFile, Deployment } from "@/lib/types";
import { API_URL } from "@/lib/utils";
import {
  ArrowLeft,
  Play,
  Square,
  RotateCw,
  FileCode,
  Terminal as TerminalIcon,
  History,
  Globe,
  GitBranch,
  Loader2,
  ExternalLink,
  XCircle,
} from "lucide-react";
import Link from "next/link";

export default function ProjectDetail() {
  const params = useParams();
  const router = useRouter();
  const projectId = params.id as string;

  const [project, setProject] = useState<Project | null>(null);
  const [files, setFiles] = useState<ProjectFile[]>([]);
  const [deployments, setDeployments] = useState<Deployment[]>([]);
  const [logs, setLogs] = useState("");
  const [loading, setLoading] = useState(true);
  const [actionLoading, setActionLoading] = useState(false);
  const [editingFile, setEditingFile] = useState<{ name: string; content: string } | null>(null);
  const [activeTab, setActiveTab] = useState("terminal");

  const fetchProject = useCallback(async () => {
    try {
      const [projectRes, filesRes, deploymentsRes] = await Promise.all([
        fetch(`${API_URL}/api/projects/${projectId}`),
        fetch(`${API_URL}/api/files/${projectId}`),
        fetch(`${API_URL}/api/deployments/${projectId}`),
      ]);

      if (!projectRes.ok) {
        router.push("/");
        return;
      }

      setProject(await projectRes.json());
      setFiles(await filesRes.json());
      const deps = await deploymentsRes.json();
      setDeployments(deps);
      
      // Load logs from last deployment if available and no current logs
      if (deps.length > 0 && !logs) {
        setLogs(deps[0].logs || "");
      }
    } catch (error) {
      console.error(error);
    } finally {
      setLoading(false);
    }
  }, [projectId, router, logs]);

  useEffect(() => {
    fetchProject();
    const interval = setInterval(fetchProject, 5000);
    return () => clearInterval(interval);
  }, [fetchProject]);

  const handleAction = async (action: "deploy" | "stop" | "restart" | "cancel" | "redeploy") => {
    setActionLoading(true);
    setLogs(`$ Starting ${action}...\n`);
    setActiveTab("terminal");

    try {
      const res = await fetch(`${API_URL}/api/deployments/${projectId}/${action}`, { method: "POST" });
      
      if (!res.ok) {
        setLogs((prev) => prev + `\n❌ Error: Server returned ${res.status} ${res.statusText}\n`);
        return;
      }
      
      const reader = res.body?.getReader();
      const decoder = new TextDecoder();

      if (reader) {
        while (true) {
          const { done, value } = await reader.read();
          if (done) break;
          setLogs((prev) => prev + decoder.decode(value));
        }
      } else {
        setLogs((prev) => prev + "\n⚠️ No stream data received\n");
      }
      setTimeout(fetchProject, 1000);
    } catch (error) {
      console.error(error);
      setLogs((prev) => prev + `\n❌ Connection error: ${error}\n`);
    } finally {
      setActionLoading(false);
    }
  };

  const openFileEditor = async (filePath: string) => {
    try {
      const res = await fetch(`${API_URL}/api/files/${projectId}/${encodeURIComponent(filePath)}`);
      const data = await res.json();
      setEditingFile({ name: filePath, content: data.content });
    } catch (error) {
      console.error(error);
    }
  };

  if (loading) {
    return (
      <div className="min-h-screen">
        <Header />
        <main className="container max-w-6xl mx-auto px-4 md:px-6 py-8">
          <div className="shimmer h-10 w-48 bg-secondary rounded-xl mb-6" />
          <div className="shimmer h-64 bg-secondary rounded-2xl" />
        </main>
      </div>
    );
  }

  if (!project) return null;

  return (
    <div className="min-h-screen flex flex-col">
      <Header />

      <main className="flex-1 container max-w-6xl mx-auto px-4 md:px-6 py-8">
        <Link href="/" className="inline-flex items-center gap-2 text-muted-foreground hover:text-foreground mb-6 transition-colors">
          <ArrowLeft className="h-4 w-4" />
          Back to Projects
        </Link>

        <div className="flex flex-col lg:flex-row lg:items-center justify-between gap-4 mb-8">
          <div className="flex items-center gap-4">
            <h1 className="text-3xl font-bold tracking-tight">{project.name}</h1>
            <StatusBadge status={project.status} />
          </div>

          <div className="flex items-center gap-3">
            {project.status === "running" ? (
              <>
                <Button variant="success" onClick={() => handleAction("redeploy")} disabled={actionLoading}>
                  {actionLoading ? <Loader2 className="h-4 w-4 animate-spin" /> : <Play className="h-4 w-4" />}
                  Redeploy
                </Button>
                <Button variant="secondary" onClick={() => handleAction("restart")} disabled={actionLoading}>
                  {actionLoading ? <Loader2 className="h-4 w-4 animate-spin" /> : <RotateCw className="h-4 w-4" />}
                  Restart
                </Button>
                <Button variant="outline" onClick={() => handleAction("stop")} disabled={actionLoading}>
                  <Square className="h-4 w-4" />
                  Stop
                </Button>
              </>
            ) : project.status === "building" ? (
              <Button variant="destructive" onClick={() => handleAction("cancel")} disabled={actionLoading} className="gap-2">
                {actionLoading ? <Loader2 className="h-4 w-4 animate-spin" /> : <XCircle className="h-4 w-4" />}
                Cancel Build
              </Button>
            ) : (
              <Button 
                onClick={() => handleAction("deploy")} 
                disabled={actionLoading} 
                className="gap-2 bg-gradient-to-r from-cyan-500 to-blue-600 hover:from-cyan-600 hover:to-blue-700 text-white shadow-lg shadow-cyan-500/25"
              >
                {actionLoading ? <Loader2 className="h-4 w-4 animate-spin" /> : <Play className="h-4 w-4" />}
                Deploy
              </Button>
            )}
          </div>
        </div>

        <div className="grid grid-cols-2 md:grid-cols-4 gap-4 mb-8">
          <Card className="p-4">
            <div className="flex items-center gap-2 text-muted-foreground mb-2">
              <Globe className="h-4 w-4" />
              <span className="text-sm">Endpoint</span>
            </div>
            {project.domain ? (
              <a href={`http://${project.domain}`} target="_blank" rel="noopener noreferrer" className="font-medium text-primary flex items-center gap-1 hover:underline">
                {project.domain} <ExternalLink className="h-3.5 w-3.5" />
              </a>
            ) : project.port ? (
              <a href={`http://${typeof window !== 'undefined' ? window.location.hostname : 'localhost'}:${project.port}`} target="_blank" rel="noopener noreferrer" className="font-mono font-medium text-primary flex items-center gap-1 hover:underline">
                {typeof window !== 'undefined' ? window.location.hostname : 'localhost'}:{project.port} <ExternalLink className="h-3.5 w-3.5" />
              </a>
            ) : (
              <span className="text-muted-foreground text-sm">Not assigned</span>
            )}
          </Card>
          <Card className="p-4">
            <div className="flex items-center gap-2 text-muted-foreground mb-2">
              <GitBranch className="h-4 w-4" />
              <span className="text-sm">Source</span>
            </div>
            <span className="font-medium capitalize">{project.source_type === "github" ? project.github_branch : "Upload"}</span>
          </Card>
          <Card className="p-4">
            <div className="flex items-center gap-2 text-muted-foreground mb-2">
              <FileCode className="h-4 w-4" />
              <span className="text-sm">Files</span>
            </div>
            <span className="font-medium">{files.length}</span>
          </Card>
          <Card className="p-4">
            <div className="flex items-center gap-2 text-muted-foreground mb-2">
              <History className="h-4 w-4" />
              <span className="text-sm">Deployments</span>
            </div>
            <span className="font-medium">{deployments.length}</span>
          </Card>
        </div>

        <Tabs value={activeTab} onValueChange={setActiveTab}>
          <TabsList>
            <TabsTrigger value="terminal" className="gap-2">
              <TerminalIcon className="h-4 w-4" />
              Terminal
            </TabsTrigger>
            <TabsTrigger value="files" className="gap-2">
              <FileCode className="h-4 w-4" />
              Files
            </TabsTrigger>
            <TabsTrigger value="history" className="gap-2">
              <History className="h-4 w-4" />
              History
            </TabsTrigger>
          </TabsList>

          <TabsContent value="terminal">
            <Terminal logs={logs} className="h-[400px]" />
          </TabsContent>

          <TabsContent value="files">
            <Card>
              <FileTree files={files} onFileEdit={openFileEditor} />
            </Card>
          </TabsContent>

          <TabsContent value="history">
            <Card className="divide-y divide-border">
              {deployments.length === 0 ? (
                <div className="text-muted-foreground text-sm py-12 text-center">No deployments yet</div>
              ) : (
                deployments.map((deployment) => (
                  <div 
                    key={deployment.id} 
                    className="flex items-center justify-between px-5 py-4 hover:bg-secondary/50 cursor-pointer transition-colors"
                    onClick={() => {
                      setLogs(deployment.logs || "No logs available for this deployment");
                      setActiveTab("terminal");
                    }}
                  >
                    <div className="flex items-center gap-3">
                      <span className={`h-2.5 w-2.5 rounded-full ${
                        deployment.status === "success" ? "bg-emerald-500" :
                        deployment.status === "failed" ? "bg-red-500" : "bg-amber-500 animate-pulse"
                      }`} />
                      <span className="text-sm font-medium capitalize">{deployment.status}</span>
                    </div>
                    <div className="flex items-center gap-3">
                      <span className="text-xs text-muted-foreground">Click to view logs</span>
                      <span className="text-sm text-muted-foreground">{new Date(deployment.created_at).toLocaleString()}</span>
                    </div>
                  </div>
                ))
              )}
            </Card>
          </TabsContent>
        </Tabs>
      </main>

      {editingFile && (
        <FileEditor
          projectId={projectId}
          filename={editingFile.name}
          content={editingFile.content}
          onClose={() => setEditingFile(null)}
          onSave={() => { setEditingFile(null); fetchProject(); }}
        />
      )}

      <Footer />
    </div>
  );
}
