// Project detail page - overview, deployments, env vars, source files, and domain management
"use client";

import { useEffect, useState, useCallback } from "react";
import { useParams, useRouter } from "next/navigation";
import { Header } from "@/components/Header";
import { Footer } from "@/components/Footer";
import { StatusBadge } from "@/components/StatusBadge";
import { Terminal } from "@/components/Terminal";
import { FileEditor } from "@/components/FileEditor";
import { FileTree } from "@/components/FileTree";
import { EnvVarsManager } from "@/components/EnvVarsManager";
import { Button } from "@/components/ui/button";
import { Card } from "@/components/ui/card";
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs";
import { Project, ProjectFile, Deployment, Service } from "@/lib/types";
import { API_URL, cn, copyToClipboard } from "@/lib/utils";
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
  Server,
  Key,
  LayoutDashboard,
  Settings,
  Shield,
  Activity,
  Cpu,
  Database,
  Cloud,
  Clock,
  Check,
  Trash2,
  Copy,
  Info,
  AlertTriangle,
  ArrowRight,
  Plus,
  Lock,
  Globe2,
  Rocket,
  Calendar,
} from "lucide-react";
import Link from "next/link";
import { toast } from "sonner";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";

function ServiceDomainManager({ service, projectId, onUpdate }: { service: Service; projectId: string; onUpdate: () => void }) {
  const [domains, setDomains] = useState<string[]>(service.domain ? service.domain.split(',').map(d => d.trim()).filter(Boolean) : []);
  const [loading, setLoading] = useState(false);

  // Sync with prop if it changes externally
  useEffect(() => {
    setDomains(service.domain ? service.domain.split(',').map(d => d.trim()).filter(Boolean) : []);
  }, [service.domain]);

  const handleAddRow = () => {
    setDomains([...domains, ""]);
  };

  const handleRemoveRow = (index: number) => {
    const newDomains = [...domains];
    newDomains.splice(index, 1);
    setDomains(newDomains);
  };

  const handleChange = (index: number, value: string) => {
    const newDomains = [...domains];
    newDomains[index] = value;
    setDomains(newDomains);
  };

  const handleSave = async () => {
    setLoading(true);
    const cleanDomains = domains.map(d => d.trim()).filter(Boolean);
    const domainString = cleanDomains.join(',');

    try {
      const res = await fetch(`${API_URL}/api/deployments/${projectId}/services/${service.id}`, {
        method: "PUT",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ domain: domainString }),
      });
      
      if (!res.ok) throw new Error("Failed to update domains");
      
      toast.success("Domains updated successfully");
      onUpdate();
    } catch (err) {
      console.error(err);
      toast.error("Failed to save domains");
    } finally {
      setLoading(false);
    }
  };

  return (
    <Card className="p-6 border-border/40 hover:border-purple-500/30 transition-all bg-purple-500/5">
      <div className="flex flex-col md:flex-row gap-6">
        {/* Service Info */}
        <div className="md:w-1/3 flex items-start gap-4">
          <div className="h-12 w-12 rounded-xl bg-purple-500/10 flex items-center justify-center shrink-0">
            <Server className="h-6 w-6 text-purple-500" />
          </div>
          <div>
            <h4 className="font-bold text-lg">{service.name}</h4>
            <div className="flex items-center gap-2 mt-1">
              <span className="text-xs font-mono bg-secondary px-2 py-0.5 rounded">Port: {service.port}</span>
              <span className="text-xs text-muted-foreground">Internal: {service.internal_port}</span>
            </div>
            <p className="text-xs text-muted-foreground mt-2 leading-relaxed">
              Map one or more custom domains to this service. traffic.
            </p>
          </div>
        </div>

        {/* Domain Editor */}
        <div className="flex-1 space-y-4">
           <div className="flex items-center justify-between">
             <label className="text-sm font-medium text-purple-500 font-bold uppercase tracking-wider">
               Active Domains
             </label>
             <Button variant="ghost" size="sm" onClick={handleAddRow} className="h-7 text-xs gap-1 text-purple-500 hover:text-purple-600 hover:bg-purple-500/10">
               <Plus className="h-3 w-3" /> Add Domain
             </Button>
           </div>
           
           <div className="space-y-2">
             {domains.length === 0 && (
               <div className="text-sm text-muted-foreground italic p-3 border border-dashed rounded-lg text-center bg-background/50">
                 No domains configured. Click "Add Domain" to start.
               </div>
             )}
             
             {domains.map((domain, idx) => (
               <div key={idx} className="flex gap-2 animate-in fade-in slide-in-from-left-2 duration-300">
                 <div className="relative flex-1 group">
                    <div className="absolute inset-y-0 left-3 flex items-center pointer-events-none">
                      <span className="text-muted-foreground text-sm font-medium group-focus-within:text-foreground">https://</span>
                    </div>
                   <input
                     type="text"
                     value={domain}
                     onChange={(e) => handleChange(idx, e.target.value)}
                     placeholder="app.example.com"
                     className="flex h-10 w-full rounded-lg border border-input bg-background pl-[4.5rem] pr-3 py-2 text-sm ring-offset-background file:border-0 file:bg-transparent file:text-sm file:font-medium placeholder:text-muted-foreground focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring focus-visible:ring-purple-500 focus-visible:ring-offset-2 disabled:cursor-not-allowed disabled:opacity-50 transition-all font-mono"
                   />
                 </div>
                 <Button 
                   variant="ghost" 
                   size="icon"
                   onClick={() => handleRemoveRow(idx)}
                   className="h-10 w-10 text-muted-foreground hover:text-red-500 hover:bg-red-500/10 rounded-lg shrink-0"
                 >
                   <Trash2 className="h-4 w-4" />
                 </Button>
               </div>
             ))}
           </div>

           <div className="flex justify-end pt-2">
             <Button 
                onClick={handleSave} 
                disabled={loading}
                className="bg-purple-600 hover:bg-purple-700 text-white shadow-lg shadow-purple-500/20 font-bold"
             >
               {loading && <Loader2 className="h-4 w-4 animate-spin mr-2" />}
               Save Changes
             </Button>
           </div>
        </div>
      </div>
    </Card>
  );
}

export default function ProjectDetail() {
  const params = useParams();
  const router = useRouter();
  const projectId = params.id as string;

  const [project, setProject] = useState<Project | null>(null);
  const [files, setFiles] = useState<ProjectFile[]>([]);
  const [deployments, setDeployments] = useState<Deployment[]>([]);
  const [services, setServices] = useState<Service[]>([]);
  const [logs, setLogs] = useState("");
  const [loading, setLoading] = useState(true);
  const [actionLoading, setActionLoading] = useState(false);
  const [currentAction, setCurrentAction] = useState<string | null>(null);
  const [editingFile, setEditingFile] = useState<{ name: string; content: string } | null>(null);
  const [activeTab, setActiveTab] = useState("overview");
  const [serverIP, setServerIP] = useState<string>('...');

  // Confirmation Dialog State
  const [isConfirmOpen, setIsConfirmOpen] = useState(false);
  const [pendingAction, setPendingAction] = useState<string | null>(null);

  // Auto-deploy state
  const [autoDeploy, setAutoDeploy] = useState(false);
  const [autoDeployLoading, setAutoDeployLoading] = useState(false);

  // Fetch server IP on mount
  useEffect(() => {
    const fetchServerIP = async () => {
      try {
        const res = await fetch(`${API_URL}/api/system/ip`);
        if (res.ok) {
          const data = await res.json();
          setServerIP(data.ip || 'N/A');
        }
      } catch {
        setServerIP('N/A');
      }
    };
    fetchServerIP();
  }, []);

  const fetchProject = useCallback(async () => {
    try {
      const [projectRes, filesRes, deploymentsRes, servicesRes] = await Promise.all([
        fetch(`${API_URL}/api/projects/${projectId}`),
        fetch(`${API_URL}/api/files/${projectId}`),
        fetch(`${API_URL}/api/deployments/${projectId}`),
        fetch(`${API_URL}/api/deployments/${projectId}/services`),
      ]);

      if (!projectRes.ok) {
        // Don't redirect if an action is in progress (e.g., during deployment)
        if (!actionLoading) {
          router.push("/");
        }
        return;
      }

      const projectData = await projectRes.json();
      setProject(projectData);
      setFiles(await filesRes.json());
      const deps = await deploymentsRes.json();
      setDeployments(deps);
      
      if (servicesRes.ok) {
        setServices(await servicesRes.json());
      }
      
      // Load logs from last deployment if available
      // - On initial load (loading is true)
      // - Or if project is building/pending (to show live progress)
      if (deps.length > 0 && deps[0].logs) {
        if (loading || projectData.status === "building" || projectData.status === "pending") {
          setLogs(deps[0].logs);
        }
      }
    } catch (error) {
      console.error(error);
    } finally {
      setLoading(false);
    }
  }, [projectId, router, loading, actionLoading]);

  useEffect(() => {
    fetchProject();
    // Poll faster (2s) when building, slower (5s) otherwise
    const pollInterval = project?.status === "building" ? 2000 : 5000;
    const interval = setInterval(fetchProject, pollInterval);
    return () => clearInterval(interval);
  }, [fetchProject, project?.status]);

  // Fetch auto-deploy status
  useEffect(() => {
    if (!projectId) return;
    
    const fetchAutoDeploy = async () => {
      try {
        const res = await fetch(`${API_URL}/api/projects/${projectId}/auto-deploy`);
        if (res.ok) {
          const data = await res.json();
          setAutoDeploy(data.auto_deploy || false);
        }
      } catch (error) {
        console.error('Failed to fetch auto-deploy status:', error);
      }
    };
    
    fetchAutoDeploy();
  }, [projectId]);

  // Toggle auto-deploy handler
  const handleAutoDeployToggle = async (enabled: boolean) => {
    setAutoDeployLoading(true);
    try {
      const res = await fetch(`${API_URL}/api/projects/${projectId}/auto-deploy`, {
        method: 'PATCH',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ enabled }),
      });
      
      if (res.ok) {
        const data = await res.json();
        setAutoDeploy(data.auto_deploy);
        toast.success(enabled ? 'Auto-deploy enabled!' : 'Auto-deploy disabled');
      } else {
        toast.error('Failed to update auto-deploy');
      }
    } catch (error) {
      toast.error('Failed to update auto-deploy');
    } finally {
      setAutoDeployLoading(false);
    }
  };

  const confirmAction = (action: string) => {
    setPendingAction(action);
    setIsConfirmOpen(true);
  };

  const getActionDetails = () => {
    switch(pendingAction) {
      case "delete":
        return {
          title: "Delete Project",
          description: "Are you sure you want to delete this project? This action cannot be undone and will explicitly remove all associated data, containers, and configurations.",
          confirmText: "Yes, Delete Project",
          confirmVariant: "bg-red-600 hover:bg-red-700 text-white border-0",
          icon: <Trash2 className="h-6 w-6 text-red-600" />
        };
      case "stop":
        return {
          title: "Stop Application",
          description: "Are you sure you want to stop the running container? The application will generate 503 errors until started again.",
          confirmText: "Stop Application",
          confirmVariant: "bg-red-500 hover:bg-red-600 text-white border-0",
          icon: <Square className="h-6 w-6 text-red-500" />
        };
      case "restart":
        return {
          title: "Restart Application",
          description: "Are you sure you want to restart the application? This may cause a brief period of downtime for your users.",
          confirmText: "Restart Now",
          confirmVariant: "bg-amber-500 hover:bg-amber-600 text-white border-0",
          icon: <RotateCw className="h-6 w-6 text-amber-500" />
        };
      case "redeploy":
        return {
          title: "Redeploy Application",
          description: "This will rebuild your application from the source and deploy a new version. This process may take a few minutes.",
          confirmText: "Start Redeploy",
          confirmVariant: "bg-emerald-600 hover:bg-emerald-700 text-white border-0",
          icon: <Play className="h-6 w-6 text-emerald-600" />
        };
      case "cancel":
        return {
          title: "Cancel Build",
          description: "Are you sure you want to abort the current build process? Partial artifacts may be left behind.",
          confirmText: "Abort Build",
          confirmVariant: "bg-red-600 hover:bg-red-700 text-white border-0",
          icon: <XCircle className="h-6 w-6 text-red-600" />
        };
       case "deploy":
        return {
          title: "Deploy Application",
          description: "Ready to launch? This will start a new build and deployment process for your application.",
          confirmText: "Deploy Now",
          confirmVariant: "bg-cyan-600 hover:bg-cyan-700 text-white border-0",
          icon: <Play className="h-6 w-6 text-cyan-600" />
        };
      default:
        return {
          title: "Confirm Action",
          description: "Are you sure you want to proceed?",
          confirmText: "Confirm",
          confirmVariant: "default",
          icon: <AlertTriangle className="h-6 w-6 text-amber-500" />
        };
    }
  };

  const executeAction = async () => {
    if (!pendingAction) return;
    const action = pendingAction;
    setIsConfirmOpen(false);

    setActionLoading(true);
    setCurrentAction(action);
    if (action !== "delete") {
      setLogs(`$ Starting ${action}...\n`);
      setActiveTab("deployments");
    }

    try {
      let url = `${API_URL}/api/deployments/${projectId}/${action}`;
      let method = "POST";

      if (action === "delete") {
        url = `${API_URL}/api/projects/${projectId}`;
        method = "DELETE";
      }

      const res = await fetch(url, { 
        method,
        headers: action === "deploy" ? { "Content-Type": "application/json" } : undefined,
        body: action === "deploy" ? JSON.stringify({ trigger: "manual" }) : undefined
      });
      
      if (!res.ok) {
        if (action !== "delete") {
          const errorData = await res.json().catch(() => null);
          const errMsg = errorData?.error || `Server returned ${res.status} ${res.statusText}`;
          setLogs((prev) => prev + `\n[ERROR] ${errMsg}\n`);
        } else {
          toast.error("Failed to delete project");
        }
        return;
      }

      if (action === "delete") {
        toast.success("Project deleted successfully");
        router.push("/");
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
        setLogs((prev) => prev + "\nâš ï¸ No stream data received\n");
      }
      setTimeout(fetchProject, 1000);
      // Wait for state to refresh before resetting actionLoading
      await new Promise(resolve => setTimeout(resolve, 1500));
      await fetchProject();
      toast.success(`${action.charAt(0).toUpperCase() + action.slice(1)} completed!`);
    } catch (error) {
      console.error(error);
      if (action !== "delete") {
        setLogs((prev) => prev + `\nâŒ Connection error: ${error}\n`);
      } else {
        toast.error("Connection error during deletion");
      }
    } finally {
      setActionLoading(false);
      setCurrentAction(null);
    }
  };

  const actionDetails = getActionDetails();

  const openFileEditor = async (filePath: string) => {
    try {
      const res = await fetch(`${API_URL}/api/files/${projectId}/content?path=${encodeURIComponent(filePath)}`);
      if (!res.ok) throw new Error("Failed to fetch file content");
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

        <div className="flex flex-col lg:flex-row lg:items-center justify-between gap-6 mb-10">
          <div className="flex items-center gap-5">
            <div className="h-14 w-14 rounded-2xl bg-gradient-to-br from-cyan-500/10 to-blue-600/10 border border-cyan-500/20 flex items-center justify-center">
              {project.project_type === "database" ? (
                <Database className="h-7 w-7 text-blue-500" />
              ) : (
                <Cloud className="h-7 w-7 text-cyan-500" />
              )}
            </div>
            <div>
              <div className="flex items-center gap-3">
                <h1 className="text-3xl font-bold tracking-tight">{project.name}</h1>
                <StatusBadge status={project.status} />
              </div>
              <p className="text-sm text-muted-foreground mt-1 flex items-center gap-2">
                <span className="capitalize">{project.project_type}</span>
                <span className="h-1 w-1 rounded-full bg-muted-foreground/30" />
                <span className="font-mono text-[10px] bg-secondary px-1.5 py-0.5 rounded uppercase tracking-wider">{project.id.split('-')[0]}</span>
              </p>
            </div>
          </div>

          <div className="flex items-center gap-2 bg-background/50 backdrop-blur-sm p-1.5 rounded-2xl border border-border/50 shadow-sm">
            {project.status === "running" ? (
              <>
                <Button 
                    variant="ghost" 
                    size="sm" 
                    onClick={() => confirmAction("redeploy")} 
                    disabled={actionLoading} 
                    className="gap-2 h-9 px-4 bg-gradient-to-r from-emerald-500 to-teal-600 hover:from-emerald-600 hover:to-teal-700 text-white shadow-lg shadow-emerald-500/20 border-0 rounded-xl transition-all font-bold"
                >
                  {currentAction === "redeploy" ? <Loader2 className="h-4 w-4 animate-spin text-white" /> : <Play className="h-4 w-4 fill-current" />}
                  Redeploy
                </Button>
                
                <Button 
                    variant="ghost" 
                    size="sm" 
                    onClick={() => confirmAction("restart")} 
                    disabled={actionLoading} 
                    className="gap-2 h-9 px-4 bg-gradient-to-r from-amber-500 to-orange-500 hover:from-amber-600 hover:to-orange-600 text-white shadow-lg shadow-amber-500/20 border-0 rounded-xl transition-all font-bold"
                >
                  {currentAction === "restart" ? <Loader2 className="h-4 w-4 animate-spin text-white" /> : <RotateCw className="h-4 w-4" />}
                  Restart
                </Button>
                
                <Button 
                    variant="ghost" 
                    size="sm" 
                    onClick={() => confirmAction("stop")} 
                    disabled={actionLoading} 
                    className="gap-2 h-9 px-4 bg-gradient-to-r from-red-500 to-rose-600 hover:from-red-600 hover:to-rose-700 text-white shadow-lg shadow-red-500/20 border-0 rounded-xl transition-all font-bold"
                >
                  {currentAction === "stop" ? <Loader2 className="h-4 w-4 animate-spin text-white" /> : <Square className="h-4 w-4 fill-current" />}
                  Stop
                </Button>
              </>
            ) : project.status === "building" ? (
              <Button variant="destructive" onClick={() => confirmAction("cancel")} disabled={currentAction === "cancel"} className="gap-2 h-9 px-4 rounded-xl shadow-red-500/20 font-bold bg-red-600 hover:bg-red-700">
                {currentAction === "cancel" ? <Loader2 className="h-4 w-4 animate-spin" /> : <XCircle className="h-4 w-4" />}
                Cancel Build
              </Button>
            ) : (
              <Button 
                onClick={() => confirmAction("deploy")} 
                disabled={actionLoading} 
                className="gap-2 h-9 px-6 bg-gradient-to-r from-cyan-500 to-blue-600 hover:from-cyan-600 hover:to-blue-700 text-white shadow-lg shadow-cyan-500/25 rounded-xl transition-all border-none font-bold"
              >
                {currentAction === "deploy" ? <Loader2 className="h-4 w-4 animate-spin" /> : <Play className="h-4 w-4 fill-current" />}
                Deploy Now
              </Button>
            )}

            <div className="w-px h-6 bg-border mx-1" />

            <Button 
                variant="ghost" 
                size="icon" 
                onClick={() => confirmAction("delete")} 
                disabled={actionLoading} 
                className="h-9 w-9 bg-secondary hover:bg-red-500 hover:text-white text-muted-foreground rounded-xl transition-all shadow-sm"
                title="Delete Project"
            >
                {currentAction === "delete" ? <Loader2 className="h-4 w-4 animate-spin" /> : <Trash2 className="h-4 w-4" />}
            </Button>
          </div>
        </div>

        {/* Stats Grid and Services were removed here as they are now in Overview tab */}

        {/* Stats Grid removed - moved to Overview */}

        <Tabs value={activeTab} onValueChange={setActiveTab} className="space-y-6">
          <TabsList className="sticky top-[72px] z-10 w-full md:w-auto flex overflow-x-auto no-scrollbar justify-start bg-background/60 backdrop-blur-xl border border-white/10 p-1.5 rounded-full shadow-2xl inline-flex items-center gap-2">
            <TabsTrigger 
              value="overview" 
              className="gap-2 rounded-full px-5 py-2.5 text-sm font-bold transition-all duration-300 data-[state=active]:bg-white data-[state=active]:dark:bg-cyan-500/20 data-[state=active]:text-cyan-600 data-[state=active]:dark:text-cyan-400 data-[state=active]:shadow-md data-[state=active]:border-cyan-200 data-[state=active]:dark:border-cyan-500/30 border border-transparent hover:bg-white/5"
            >
              <LayoutDashboard className="h-4 w-4" />
              Overview
            </TabsTrigger>
            <TabsTrigger 
              value="deployments" 
              className="gap-2 rounded-full px-5 py-2.5 text-sm font-bold transition-all duration-300 data-[state=active]:bg-white data-[state=active]:dark:bg-cyan-500/20 data-[state=active]:text-cyan-600 data-[state=active]:dark:text-cyan-400 data-[state=active]:shadow-md data-[state=active]:border-cyan-200 data-[state=active]:dark:border-cyan-500/30 border border-transparent hover:bg-white/5"
            >
              <Activity className="h-4 w-4" />
              Deployments
            </TabsTrigger>
            <TabsTrigger 
              value="env" 
              className="gap-2 rounded-full px-5 py-2.5 text-sm font-bold transition-all duration-300 data-[state=active]:bg-white data-[state=active]:dark:bg-cyan-500/20 data-[state=active]:text-cyan-600 data-[state=active]:dark:text-cyan-400 data-[state=active]:shadow-md data-[state=active]:border-cyan-200 data-[state=active]:dark:border-cyan-500/30 border border-transparent hover:bg-white/5"
            >
              <Key className="h-4 w-4" />
              Environment
            </TabsTrigger>
            <TabsTrigger 
              value="source" 
              className="gap-2 rounded-full px-5 py-2.5 text-sm font-bold transition-all duration-300 data-[state=active]:bg-white data-[state=active]:dark:bg-cyan-500/20 data-[state=active]:text-cyan-600 data-[state=active]:dark:text-cyan-400 data-[state=active]:shadow-md data-[state=active]:border-cyan-200 data-[state=active]:dark:border-cyan-500/30 border border-transparent hover:bg-white/5"
            >
              <FileCode className="h-4 w-4" />
              Source
            </TabsTrigger>
            <TabsTrigger 
              value="domains" 
              className="gap-2 rounded-full px-5 py-2.5 text-sm font-bold transition-all duration-300 data-[state=active]:bg-white data-[state=active]:dark:bg-cyan-500/20 data-[state=active]:text-cyan-600 data-[state=active]:dark:text-cyan-400 data-[state=active]:shadow-md data-[state=active]:border-cyan-200 data-[state=active]:dark:border-cyan-500/30 border border-transparent hover:bg-white/5"
            >
              <Globe className="h-4 w-4" />
              Domains
            </TabsTrigger>
          </TabsList>

          <TabsContent value="overview" className="space-y-8 animate-in fade-in slide-in-from-bottom-2 duration-400">
            {/* Real Services Card */}
            {services.length > 0 && (
              <div className="grid gap-6">
                <div className="flex items-center justify-between">
                  <h3 className="text-xl font-bold flex items-center gap-2">
                    <Cloud className="h-5 w-5 text-cyan-500" />
                    Services & Endpoints
                  </h3>
                  <div className="text-[10px] font-bold px-2 py-0.5 rounded-full bg-cyan-500/10 text-cyan-500 border border-cyan-500/20 uppercase tracking-widest">
                    Live Status
                  </div>
                </div>
                <div className="grid gap-4">
                  {services.map((svc) => {
                    const domains = svc.domain ? svc.domain.split(',').map(d => d.trim()).filter(Boolean) : [];
                    
                    // Use localhost when in development (browser is on localhost), server IP otherwise
                    const isLocal = typeof window !== 'undefined' && 
                      (window.location.hostname === 'localhost' || window.location.hostname === '127.0.0.1');
                    const portHost = isLocal ? 'localhost' : serverIP;
                    
                    const portEndpoint = { 
                      url: `http://${portHost}:${svc.port}`, 
                      label: `${portHost}:${svc.port}`,
                      isPort: true
                    };

                    // Always show port endpoint + any configured domains
                    const domainLinks = domains.map(d => ({ 
                      url: `https://${d}`, 
                      label: d,
                      isPort: false 
                    }));

                    return (
                      <Card 
                        key={svc.id} 
                        className="group relative overflow-hidden p-0 border-border/40 hover:border-cyan-500/50 transition-all duration-300 shadow-sm hover:shadow-cyan-500/5"
                      >
                        <div className="absolute inset-y-0 left-0 w-1 bg-cyan-500 opacity-0 group-hover:opacity-100 transition-opacity" />
                        <div className="flex items-center justify-between p-5">
                          <div className="flex items-center gap-5">
                            <div className="h-12 w-12 rounded-2xl bg-secondary/50 flex items-center justify-center group-hover:bg-cyan-500/10 transition-colors">
                              <Server className="h-6 w-6 text-muted-foreground group-hover:text-cyan-500 transition-colors" />
                            </div>
                            <div>
                              <div className="flex items-center gap-3">
                                <span className="font-bold text-lg">{svc.name}</span>
                                <StatusBadge status={svc.status} />
                              </div>
                              <p className="text-xs text-muted-foreground mt-1 flex items-center gap-2">
                                <span className="font-mono bg-secondary/80 px-1.5 py-0.5 rounded">Port {svc.internal_port}</span>
                                <span className="text-muted-foreground/30">/</span>
                                <span className="flex items-center gap-1">
                                  <FileCode className="h-3 w-3" />
                                  {svc.dockerfile_path}
                                </span>
                              </p>
                            </div>
                          </div>
                          <div className="flex flex-col gap-2 items-end">
                            {/* Always show IP:Port access */}
                            <a
                              href={portEndpoint.url}
                              target="_blank"
                              rel="noopener noreferrer"
                              className="h-9 px-4 flex items-center gap-2 rounded-xl bg-secondary/50 hover:bg-cyan-500 text-muted-foreground hover:text-white font-mono text-xs font-bold transition-all duration-300 border border-border/50"
                            >
                              {portEndpoint.label}
                              <ExternalLink className="h-3 w-3" />
                            </a>
                            {/* Show domain links if configured */}
                            {domainLinks.map((link, idx) => (
                              <a
                                key={idx}
                                href={link.url}
                                target="_blank"
                                rel="noopener noreferrer"
                                className="h-9 px-4 flex items-center gap-2 rounded-xl bg-cyan-500/10 hover:bg-cyan-500 text-cyan-600 hover:text-white font-mono text-xs font-bold transition-all duration-300"
                              >
                                {link.label}
                                <ExternalLink className="h-3 w-3" />
                              </a>
                            ))}
                          </div>
                        </div>
                      </Card>
                    );
                  })}
                </div>
              </div>
            )}

            {/* Quick Stats Grid */}
            <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-4 gap-5">
              <Card className="p-6 border-border/40 hover:border-border transition-colors">
                <div className="flex items-center justify-between mb-4">
                  <div className="p-2.5 rounded-xl bg-orange-500/10">
                    <Activity className="h-5 w-5 text-orange-500" />
                  </div>
                  <span className="text-[10px] font-bold text-muted-foreground uppercase tracking-widest">Health</span>
                </div>
                <div className="space-y-1">
                  <p className="text-2xl font-bold tracking-tight capitalize">{project.status}</p>
                  <p className="text-xs text-muted-foreground">Main service status</p>
                </div>
              </Card>

              <Card className="p-6 border-border/40 hover:border-border transition-colors">
                <div className="flex items-center justify-between mb-4">
                  <div className="p-2.5 rounded-xl bg-purple-500/10">
                    <History className="h-5 w-5 text-purple-500" />
                  </div>
                  <span className="text-[10px] font-bold text-muted-foreground uppercase tracking-widest">Activity</span>
                </div>
                <div className="space-y-1">
                  <p className="text-2xl font-bold tracking-tight">{deployments.length}</p>
                  <p className="text-xs text-muted-foreground">Total deployments</p>
                </div>
              </Card>

              <Card className="p-6 border-border/40 hover:border-border transition-colors">
                <div className="flex items-center justify-between mb-4">
                  <div className="p-2.5 rounded-xl bg-blue-500/10">
                    <Cpu className="h-5 w-5 text-blue-500" />
                  </div>
                  <span className="text-[10px] font-bold text-muted-foreground uppercase tracking-widest">Resource</span>
                </div>
                <div className="space-y-1">
                  <p className="text-2xl font-bold tracking-tight">{project.source_type === "github" ? "GitHub" : "Upload"}</p>
                  <p className="text-xs text-muted-foreground">Source type</p>
                </div>
              </Card>

              <Card className="p-6 border-border/40 hover:border-border transition-colors">
                <div className="flex items-center justify-between mb-4">
                  <div className="p-2.5 rounded-xl bg-cyan-500/10">
                    <Shield className="h-5 w-5 text-cyan-500" />
                  </div>
                  <span className="text-[10px] font-bold text-muted-foreground uppercase tracking-widest">Privacy</span>
                </div>
                <div className="space-y-1">
                  <p className="text-2xl font-bold tracking-tight">Isolated</p>
                  <p className="text-xs text-muted-foreground">Network namespace</p>
                </div>
              </Card>
            </div>

            {/* GitHub Info if applicable */}
            {project.source_type === "github" && (
              <Card className="p-0 border-border/40 overflow-hidden">
                <div className="bg-secondary/30 px-6 py-4 border-b flex items-center justify-between">
                  <div className="flex items-center gap-2">
                    <GitBranch className="h-4 w-4 text-muted-foreground" />
                    <span className="text-sm font-semibold italic text-muted-foreground uppercase tracking-widest">Source Configuration</span>
                  </div>
                  <a href={project.github_url || "#"} target="_blank" rel="noopener noreferrer" className="text-xs font-bold text-cyan-500 hover:text-cyan-400 flex items-center gap-1.5 transition-colors">
                    VIEW REPO <ExternalLink className="h-3 w-3" />
                  </a>
                </div>
                <div className="p-6 grid grid-cols-1 md:grid-cols-2 gap-8">
                  <div className="space-y-4">
                    <div className="flex flex-col gap-1.5">
                      <span className="text-[10px] font-bold text-muted-foreground uppercase tracking-widest">Repository URL</span>
                      <p className="text-sm font-mono bg-secondary/50 p-3 rounded-xl border border-border/50 truncate">
                        {project.github_url}
                      </p>
                    </div>
                  </div>
                  <div className="flex items-center gap-6">
                    <div className="flex flex-col gap-1.5">
                      <span className="text-[10px] font-bold text-muted-foreground uppercase tracking-widest">Selected Branch</span>
                      <div className="flex items-center gap-2 px-3 py-1.5 bg-cyan-500/10 text-cyan-500 rounded-lg border border-cyan-500/20 font-mono text-xs font-bold">
                        <GitBranch className="h-3.5 w-3.5" />
                        {project.github_branch}
                      </div>
                    </div>
                  </div>
                </div>

                {/* Auto-Deploy Section */}
                <div className="border-t border-border/40 p-6">
                  <div className="flex items-center justify-between">
                    <div className="flex items-center gap-3">
                      <div className="p-2 rounded-lg bg-emerald-500/10">
                        <Rocket className="h-4 w-4 text-emerald-500" />
                      </div>
                      <div>
                        <span className="font-semibold">Auto-Deploy</span>
                        <p className="text-xs text-muted-foreground">
                          {autoDeploy 
                            ? "Pushes to this branch will trigger automatic deployment" 
                            : "Enable to auto-redeploy when commits are pushed"}
                        </p>
                      </div>
                    </div>
                    <button
                      onClick={() => handleAutoDeployToggle(!autoDeploy)}
                      disabled={autoDeployLoading}
                      className={cn(
                        "relative h-6 w-11 rounded-full transition-colors duration-200 focus:outline-none focus:ring-2 focus:ring-offset-2 focus:ring-emerald-500",
                        autoDeploy ? "bg-emerald-500" : "bg-secondary"
                      )}
                    >
                      <span
                        className={cn(
                          "absolute top-0.5 left-0.5 h-5 w-5 rounded-full bg-white shadow-sm transition-transform duration-200",
                          autoDeploy ? "translate-x-5" : "translate-x-0"
                        )}
                      />
                    </button>
                  </div>
                </div>
              </Card>
            )}
          </TabsContent>

          <TabsContent value="deployments" className="animate-in fade-in slide-in-from-bottom-2 duration-400">
            <div className="grid grid-cols-1 lg:grid-cols-3 gap-6">
              <div className="lg:col-span-2 space-y-6">
                <div className="flex items-center justify-between">
                   <h3 className="text-xl font-bold flex items-center gap-2">
                    <TerminalIcon className="h-5 w-5 text-amber-500" />
                    Live Terminal Output
                  </h3>
                  {project.status === "building" && (
                    <div className="flex items-center gap-2 text-xs font-bold text-amber-500 animate-pulse">
                      <Loader2 className="h-3 w-3 animate-spin" />
                      BUILDING IN PROGRESS
                    </div>
                  )}
                </div>
                <Terminal logs={logs} isBuilding={project?.status === "building"} className="h-[550px]" />
              </div>

              <div className="space-y-6">
                <h3 className="text-xl font-bold flex items-center gap-2">
                  <History className="h-5 w-5 text-muted-foreground" />
                  Recent History
                </h3>
                <Card className="divide-y divide-border/40 border-border/40 overflow-hidden relative">
                  {project.status === "building" && (
                    <div className="absolute top-0 left-0 w-full h-1 bg-secondary overflow-hidden z-10">
                      <div className="absolute top-0 h-full bg-gradient-to-r from-cyan-500 via-blue-500 to-cyan-500 animate-progress-scan" />
                    </div>
                  )}
                  {deployments.length === 0 ? (
                    <div className="text-muted-foreground text-sm py-20 text-center flex flex-col items-center gap-3">
                      <div className="p-4 rounded-full bg-secondary/50">
                        <History className="h-8 w-8 text-muted-foreground/30" />
                      </div>
                      No deployments found
                    </div>
                  ) : (
                    deployments.map((deployment) => (
                      <div 
                        key={deployment.id} 
                        className={cn(
                          "flex flex-col gap-2 px-5 py-4 cursor-pointer transition-all duration-200 border-l-4",
                          deployment.status === "success" ? "border-l-emerald-500 lg:hover:bg-emerald-500/5" :
                          deployment.status === "failed" ? "border-l-red-500 lg:hover:bg-red-500/5" : "border-l-amber-500"
                        )}
                        onClick={() => {
                          setLogs(deployment.logs || "No logs available for this deployment");
                          const terminalElement = document.getElementById('terminal-wrapper');
                          terminalElement?.scrollIntoView({ behavior: 'smooth', block: 'center' });
                        }}
                      >
                        <div className="flex items-start justify-between">
                          <div className="flex flex-col gap-1">
                             <span className="font-bold text-sm tracking-tight">
                              {deployment.trigger === 'webhook' ? 'Auto-Deploy (GitHub)' :
                               deployment.trigger === 'restart' ? 'Restart Action' :
                               deployment.trigger === 'stop' ? 'Stop Action' :
                               deployment.trigger === 'redeploy' ? 'Manual Redeploy' : 'Manual Deployment'}
                            </span>
                            <div className="flex items-center gap-2">
                              <span className={cn(
                                "text-[10px] font-bold px-1.5 py-0.5 rounded-md uppercase tracking-wider",
                                deployment.status === "success" ? "bg-emerald-500/10 text-emerald-500" :
                                deployment.status === "failed" ? "bg-red-500/10 text-red-500" : "bg-amber-500/10 text-amber-500"
                              )}>
                                {deployment.status}
                              </span>
                              <span className="text-[10px] font-mono text-muted-foreground opacity-60"># {deployment.id.split('-')[0]}</span>
                            </div>
                          </div>
                          <div className="flex flex-col items-end gap-1">
                            <span className="text-[10px] font-medium text-muted-foreground flex items-center gap-1.5 bg-secondary/30 px-2 py-0.5 rounded-full border border-border/20">
                              <Clock className="h-3 w-3" />
                              {new Date(deployment.created_at).toLocaleTimeString([], { hour: '2-digit', minute: '2-digit' })}
                            </span>
                             <span className="text-[10px] text-muted-foreground/60 flex items-center gap-1.5 mr-1">
                              <Calendar className="h-3 w-3" />
                              {new Date(deployment.created_at).toLocaleDateString([], { month: 'short', day: 'numeric', year: 'numeric' })}
                            </span>
                          </div>
                        </div>
                        <p className="text-[11px] text-muted-foreground line-clamp-1 italic">
                          Click to restore log output to terminal
                        </p>
                      </div>
                    ))
                  )}
                </Card>
              </div>
            </div>
          </TabsContent>

          <TabsContent value="env" className="animate-in fade-in slide-in-from-bottom-2 duration-400">
            <div className="max-w-4xl mx-auto space-y-6">
              <div className="flex items-center justify-between">
                <div>
                  <h3 className="text-xl font-bold flex items-center gap-2">
                    <Key className="h-5 w-5 text-cyan-500" />
                    Environment Variables
                  </h3>
                  <p className="text-sm text-muted-foreground mt-1">
                    Manage secret values and configuration for your deployment environment.
                  </p>
                </div>
              </div>
              <EnvVarsManager projectId={projectId} />
            </div>
          </TabsContent>

          <TabsContent value="source" className="animate-in fade-in slide-in-from-bottom-2 duration-400">
            <div className="space-y-6">
              <div className="flex items-center justify-between">
                <h3 className="text-xl font-bold flex items-center gap-2">
                  <FileCode className="h-5 w-5 text-muted-foreground" />
                  Project Files & Source
                </h3>
              </div>
              <Card className="border-border/40 overflow-hidden">
                <FileTree files={files} onFileEdit={openFileEditor} />
              </Card>
            </div>
          </TabsContent>

          <TabsContent value="domains" className="animate-in fade-in slide-in-from-bottom-2 duration-400">
             <div className="max-w-4xl mx-auto space-y-8">
               

               {/* DNS Guide Section */}
               <Card className="p-6 border-cyan-500/20 bg-cyan-500/5">
                 <div className="flex items-start gap-4">
                   <div className="p-2 bg-cyan-500/10 rounded-lg shrink-0">
                     <Info className="h-5 w-5 text-cyan-500" />
                   </div>
                   <div className="space-y-4 flex-1">
                     <div>
                       <h3 className="text-lg font-bold text-foreground">DNS Configuration Guide</h3>
                       <p className="text-sm text-muted-foreground mt-1">
                         Configure your DNS records to point to your deployment server. Support for Cloudflare (Full/Strict) SSL is recommended.
                       </p>
                     </div>

                     <div className="grid md:grid-cols-2 gap-6">
                       <div className="bg-background/50 p-4 rounded-xl border border-border/50">
                         <div className="flex items-center justify-between mb-2">
                           <span className="text-xs font-bold uppercase tracking-wider text-muted-foreground">Server IP (A Record)</span>
                           <Button 
                             variant="ghost" 
                             size="icon" 
                             className="h-6 w-6 hover:bg-background" 
                             onClick={() => {
                               copyToClipboard(serverIP);
                               toast.success("IP copied to clipboard");
                             }}
                           >
                             <Copy className="h-3 w-3" />
                           </Button>
                         </div>
                         <code className="text-xl font-mono font-bold text-cyan-500 block">
                           {serverIP}
                         </code>
                         <p className="text-[10px] text-muted-foreground mt-2">
                           Target for all <strong>A Records</strong>.
                         </p>
                       </div>

                        <div className="space-y-2 text-sm">
                          <div className="grid grid-cols-[80px_1fr_auto_1fr] gap-2 items-center p-2 rounded-lg hover:bg-white/5 transition-colors group">
                            <span className="font-bold text-muted-foreground group-hover:text-cyan-400 transition-colors">Root</span>
                            <span className="font-mono">example.com</span>
                            <span><ArrowRight className="h-4 w-4 text-muted-foreground/50" /></span>
                            <span className="font-mono bg-secondary px-2 py-1 rounded-md text-xs text-center border border-white/5">A Record (@)</span>
                          </div>
                          <div className="grid grid-cols-[80px_1fr_auto_1fr] gap-2 items-center p-2 rounded-lg hover:bg-white/5 transition-colors group">
                            <span className="font-bold text-muted-foreground group-hover:text-cyan-400 transition-colors">Subdomain</span>
                            <span className="font-mono">app.example.com</span>
                            <span><ArrowRight className="h-4 w-4 text-muted-foreground/50" /></span>
                            <span className="font-mono bg-secondary px-2 py-1 rounded-md text-xs text-center border border-white/5">A Record (app)</span>
                          </div>
                          <div className="grid grid-cols-[80px_1fr_auto_1fr] gap-2 items-center p-2 rounded-lg hover:bg-white/5 transition-colors group">
                            <span className="font-bold text-muted-foreground group-hover:text-cyan-400 transition-colors">WWW</span>
                            <span className="font-mono">www.example.com</span>
                            <span><ArrowRight className="h-4 w-4 text-muted-foreground/50" /></span>
                            <span className="font-mono bg-secondary px-2 py-1 rounded-md text-xs text-center border border-white/5">CNAME (@)</span>
                          </div>
                          <div className="flex items-center gap-2 mt-2 pt-2 border-t border-border/30 text-xs text-emerald-500 font-medium">
                            <Lock className="h-3 w-3" />
                            ssl/https: Enable <strong>Full (Strict)</strong> mode in Cloudflare/Provider.
                          </div>
                        </div>
                     </div>
                   </div>
                 </div>
               </Card>

               <div className="space-y-4">
                 <div className="flex items-center justify-between">
                   <h3 className="text-xl font-bold flex items-center gap-2">
                     <Globe className="h-5 w-5 text-purple-500" />
                     Service Domains
                   </h3>
                 </div>

                 <div className="grid gap-4">
                   {services.map((svc) => (
                     <ServiceDomainManager key={svc.id} service={svc} projectId={projectId} onUpdate={fetchProject} />
                   ))}
                   
                   {services.length === 0 && (
                     <div className="text-center py-12 text-muted-foreground bg-secondary/20 rounded-xl border border-dashed border-border">
                       No services found for this project.
                     </div>
                   )}
                 </div>
               </div>
             </div>
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
      <Dialog open={isConfirmOpen} onOpenChange={setIsConfirmOpen}>
        <DialogContent className="sm:max-w-md">
          <DialogHeader>
            <DialogTitle className="flex items-center gap-2">
              {actionDetails.icon}
              {actionDetails.title}
            </DialogTitle>
            <DialogDescription className="pt-2">
              {actionDetails.description}
            </DialogDescription>
          </DialogHeader>
          <DialogFooter className="gap-2 sm:gap-0">
            <Button variant="outline" onClick={() => setIsConfirmOpen(false)}>
              Cancel
            </Button>
            <Button 
              className={actionDetails.confirmVariant} 
              onClick={executeAction}
              disabled={actionLoading}
            >
              {actionLoading ? <Loader2 className="h-4 w-4 animate-spin mr-2" /> : null}
              {actionDetails.confirmText}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </div>
  );
}
