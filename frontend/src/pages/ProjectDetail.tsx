// Project detail page - overview, deployments, env vars, source files, and domain management

import { useEffect, useState, useCallback, useRef } from "react";
import { useParams, useNavigate } from "react-router-dom";
import { useBreadcrumbLeaf } from "@/components/shell/ShellContext";
import { StatusBadge } from "@/components/StatusBadge";
import { Terminal } from "@/components/Terminal";
import { LogViewer } from "@/components/LogViewer";
import { FileEditor } from "@/components/FileEditor";
import { FileTree } from "@/components/FileTree";
import { EnvVarsManager } from "@/components/EnvVarsManager";
import { DnsGuideCard } from "@/components/domains/DnsGuideCard";
import { ServiceDomainCard } from "@/components/domains/ServiceDomainCard";
import { Button } from "@/components/ui/button";
import { Card } from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs";
import {
  Project,
  ProjectFile,
  Deployment,
  Service,
  BuildType,
  BuildDetection,
  StorageMount,
} from "@/lib/types";
import { API_URL, cn } from "@/lib/utils";
import { authFetch } from "@/lib/auth";
import { consumeProgressStream } from "@/lib/streamProgress";
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
  Trash2,
  Info,
  AlertTriangle,
  Plus,
  Rocket,
  Calendar,
  RefreshCw,
  ScrollText,
  Download,
  Pause,
  ChevronDown,
  ChevronLeft,
  ChevronRight,
  Box,
  HardDrive,
} from "lucide-react";
import { Link } from "react-router-dom";
import { toast } from "sonner";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";

const HISTORY_PAGE_SIZE = 6;
const MAX_LOG_LINES = 10000;

// Real-time container logs panel
function ContainerLogsPanel({
  projectId,
  services,
  activeTab,
}: {
  projectId: string;
  services: Service[];
  activeTab: string;
}) {
  const [containerLogs, setContainerLogs] = useState<Record<string, string[]>>(
    {}
  );
  const [activeContainer, setActiveContainer] = useState<string | null>(null);
  const [connected, setConnected] = useState<Record<string, boolean>>({});
  const eventSourcesRef = useRef<Record<string, EventSource>>({});

  // Stable list of container names to avoid re-connecting on every poll
  const containerNamesKey = services
    .map((s) => s.container_name)
    .filter(Boolean)
    .sort()
    .join(",");

  // Set default active container when services load
  useEffect(() => {
    if (services.length > 0 && !activeContainer) {
      const firstWithContainer = services.find((s) => s.container_name);
      if (firstWithContainer) {
        setActiveContainer(firstWithContainer.container_name!);
      }
    }
  }, [services, activeContainer]);

  // SSE connection management using native EventSource — clean close, no abort errors
  useEffect(() => {
    if (activeTab !== "logs" || !containerNamesKey) {
      Object.values(eventSourcesRef.current).forEach((es) => {
        try { es.close(); } catch { /* ignore */ }
      });
      eventSourcesRef.current = {};
      setConnected({});
      return;
    }

    let cancelled = false;

    // Use async IIFE to fetch SSE token (useEffect callbacks can't be async)
    (async () => {
      // Fetch short-lived SSE token — never fall back to session JWT in the URL
      let sseToken = "";
      try {
        const tokenRes = await authFetch(`${API_URL || ""}/api/auth/sse-token`, {
          method: "POST",
          headers: { "Content-Type": "application/json" },
        });
        if (tokenRes.ok) {
          const data = await tokenRes.json();
          sseToken = data.token || "";
        }
      } catch {
        /* no session JWT fallback */
      }

      if (cancelled || !sseToken) {
        return;
      }

      // EventSource: in DEV hit the API host directly so the Vite proxy cannot buffer SSE.
      // In PROD, Nginx routes /api correctly, so relative paths are fine when API_URL is unset.
      const isDev = import.meta.env.DEV;
      const sseBase = API_URL || (isDev && typeof window !== "undefined"
        ? `${window.location.protocol}//${window.location.hostname}:8000`
        : "");
      const containerNames = containerNamesKey.split(",");

      containerNames.forEach((containerName) => {
        if (cancelled || eventSourcesRef.current[containerName]) return;

        const url = `${sseBase}/api/logs/${projectId}/stream/${encodeURIComponent(containerName)}?token=${encodeURIComponent(sseToken)}&tail=5000`;
        const es = new EventSource(url);

        es.onopen = () => {
          if (cancelled) {
            es.close();
            return;
          }
          setConnected((prev) => ({ ...prev, [containerName]: true }));
        };

        es.onmessage = (event) => {
          if (cancelled) return;
          try {
            const data = JSON.parse(event.data);
            if (data.type === "log") {
              setContainerLogs((prev) => {
                const existing = prev[containerName] || [];
                const updated = [...existing, data.message];
                return {
                  ...prev,
                  [containerName]: updated.length > MAX_LOG_LINES
                    ? updated.slice(-MAX_LOG_LINES)
                    : updated,
                };
              });
            } else if (data.type === "status") {
              setContainerLogs((prev) => ({
                ...prev,
                [containerName]: [`⚠️ ${data.message}`],
              }));
              setConnected((prev) => ({
                ...prev,
                [containerName]: false,
              }));
            } else if (data.type === "error") {
              setContainerLogs((prev) => ({
                ...prev,
                [containerName]: [
                  ...(prev[containerName] || []),
                  `❌ ${data.message}`,
                ],
              }));
              setConnected((prev) => ({
                ...prev,
                [containerName]: false,
              }));
            }
          } catch {
            // Ignore parse errors
          }
        };

        es.onerror = () => {
          es.close();
          if (!cancelled) {
            setConnected((prev) => ({ ...prev, [containerName]: false }));
          }
        };

        eventSourcesRef.current[containerName] = es;
      });
    })();

    return () => {
      cancelled = true;
      Object.values(eventSourcesRef.current).forEach((es) => {
        try { es.close(); } catch { /* ignore */ }
      });
      eventSourcesRef.current = {};
      setConnected({});
    };
  // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [activeTab, containerNamesKey, projectId]);

  const clearLogs = (containerName: string) => {
    setContainerLogs((prev) => ({ ...prev, [containerName]: [] }));
  };

  const downloadLogs = (containerName: string) => {
    const logContent = (containerLogs[containerName] || []).join("\n");
    const blob = new Blob([logContent], { type: "text/plain" });
    const url = URL.createObjectURL(blob);
    const a = document.createElement("a");
    a.href = url;
    a.download = `${containerName}-logs.txt`;
    a.click();
    URL.revokeObjectURL(url);
  };

  const containersWithNames = services.filter((s) => s.container_name);

  if (containersWithNames.length === 0) {
    return (
      <div className="text-center py-16 text-muted-foreground">
        <ScrollText className="h-12 w-12 mx-auto mb-4 opacity-30" />
        <p className="text-lg font-semibold">No containers found</p>
        <p className="text-sm mt-1">
          Deploy your project first to view container logs.
        </p>
      </div>
    );
  }

  return (
    <div className="space-y-4">
      <div className="flex items-center justify-between">
        <h3 className="text-xl font-bold flex items-center gap-2">
          <ScrollText className="h-5 w-5 text-emerald-500" />
          Container Logs
        </h3>
        <div className="text-[10px] font-bold px-2 py-0.5 rounded-full bg-emerald-500/10 text-emerald-500 border border-emerald-500/20 uppercase tracking-widest">
          Real-time
        </div>
      </div>

      {/* Container selector tabs */}
      {containersWithNames.length > 1 && (
        <div className="flex flex-wrap gap-2">
          {containersWithNames.map((svc) => (
            <button
              key={svc.id}
              onClick={() => setActiveContainer(svc.container_name!)}
              className={cn(
                "flex items-center gap-2 px-4 py-2 rounded-xl text-sm font-bold transition-all duration-300 border",
                activeContainer === svc.container_name
                  ? "bg-emerald-500/15 text-emerald-500 border-emerald-500/30 shadow-sm"
                  : "bg-secondary/50 text-muted-foreground border-border/50 hover:bg-secondary hover:text-foreground"
              )}
            >
              <Server className="h-3.5 w-3.5" />
              {svc.name}
              <span
                className={cn(
                  "h-2 w-2 rounded-full",
                  connected[svc.container_name!]
                    ? "bg-emerald-500 animate-pulse"
                    : "bg-muted-foreground/30"
                )}
              />
            </button>
          ))}
        </div>
      )}

      {/* Log panels */}
      {containersWithNames.map((svc) => {
        const containerName = svc.container_name!;
        const isActive =
          containersWithNames.length === 1 ||
          activeContainer === containerName;
        if (!isActive) return null;

        const logLines = containerLogs[containerName] || [];

        return (
          <LogViewer
            key={svc.id}
            logs={logLines}
            connected={!!connected[containerName]}
            title={svc.name}
            subtitle={containerName}
            onClear={() => clearLogs(containerName)}
            downloadFilename={`${containerName}-logs.txt`}
            height="h-[550px]"
          />
        );
      })}
    </div>
  );
}


export default function ProjectDetail() {
  const params = useParams();
  const navigate = useNavigate();
  const projectId = params.id as string;

  const [project, setProject] = useState<Project | null>(null);
  useBreadcrumbLeaf(project?.name);
  const [files, setFiles] = useState<ProjectFile[]>([]);
  const [deployments, setDeployments] = useState<Deployment[]>([]);
  const [services, setServices] = useState<Service[]>([]);
  const [logs, setLogs] = useState("");
  const [loading, setLoading] = useState(true);
  const [actionLoading, setActionLoading] = useState(false);
  const [currentAction, setCurrentAction] = useState<string | null>(null);
  const [editingFile, setEditingFile] = useState<{
    name: string;
    content: string;
  } | null>(null);
  const [activeTab, setActiveTab] = useState("overview");
  const [serverIP, setServerIP] = useState<string>("...");
  const [buildType, setBuildType] = useState<BuildType>("auto");
  const [baseDirectory, setBaseDirectory] = useState(".");
  const [dockerfilePath, setDockerfilePath] = useState("Dockerfile");
  const [internalPort, setInternalPort] = useState(3000);
  const [buildInitialized, setBuildInitialized] = useState(false);
  const [buildSaving, setBuildSaving] = useState(false);
  const [buildDetecting, setBuildDetecting] = useState(false);
  const [buildDetection, setBuildDetection] = useState<BuildDetection | null>(null);
  const [storageMounts, setStorageMounts] = useState<StorageMount[]>([]);
  const [storageLoading, setStorageLoading] = useState(false);
  const [storageSaving, setStorageSaving] = useState(false);
  const [storageService, setStorageService] = useState("");
  const [storageName, setStorageName] = useState("");
  const [storagePath, setStoragePath] = useState("");
  const [storageToDelete, setStorageToDelete] = useState<StorageMount | null>(null);

  // Confirmation Dialog State
  const [isConfirmOpen, setIsConfirmOpen] = useState(false);
  const [pendingAction, setPendingAction] = useState<string | null>(null);

  // Auto-deploy state
  const [autoDeploy, setAutoDeploy] = useState(false);
  const [autoDeployLoading, setAutoDeployLoading] = useState(false);

  // Deployment history pagination (6 per page)
  const [historyPage, setHistoryPage] = useState(0);
  const [deploymentTotal, setDeploymentTotal] = useState(0);
  const [historyLoading, setHistoryLoading] = useState(false);
  const historyFetchGen = useRef(0);
  const deploymentsAbortRef = useRef<AbortController | null>(null);

  // Track currently viewed deployment for auto-deploy real-time logs
  const [viewingDeploymentId, setViewingDeploymentId] = useState<string | null>(
    null,
  );

  // Fetch server IP on mount
  useEffect(() => {
    const fetchServerIP = async () => {
      try {
        const res = await authFetch(`${API_URL}/api/system/ip`);
        if (res.ok) {
          const data = await res.json();
          setServerIP(data.ip || "N/A");
        }
      } catch {
        setServerIP("N/A");
      }
    };
    fetchServerIP();
  }, []);

  const fetchProject = useCallback(async () => {
    try {
      const historyOffset = historyPage * HISTORY_PAGE_SIZE;
      const pageForHistory = historyPage;
      const historyGenAtStart = historyFetchGen.current;

      deploymentsAbortRef.current?.abort();
      const deploymentsAbort = new AbortController();
      deploymentsAbortRef.current = deploymentsAbort;

      const [projectRes, filesRes, deploymentsRes, servicesRes, latestRes] =
        await Promise.all([
          authFetch(`${API_URL}/api/projects/${projectId}`),
          authFetch(`${API_URL}/api/files/${projectId}`),
          authFetch(
            `${API_URL}/api/deployments/${projectId}?limit=${HISTORY_PAGE_SIZE}&offset=${historyOffset}&meta=1`,
            { signal: deploymentsAbort.signal },
          ),
          authFetch(`${API_URL}/api/deployments/${projectId}/services`),
          // Always resolve the newest deployment for live log tracking when not on page 1
          historyPage > 0
            ? authFetch(
                `${API_URL}/api/deployments/${projectId}?limit=1&offset=0`,
              )
            : Promise.resolve(null),
        ]);

      if (!projectRes.ok) {
        // Don't redirect if an action is in progress (e.g., during deployment)
        if (!actionLoading) {
          navigate("/");
        }
        return;
      }

      const projectData = await projectRes.json();
      setProject(projectData);
      setFiles(await filesRes.json());

      const depsPayload = await deploymentsRes.json();
      const historyStale =
        historyGenAtStart !== historyFetchGen.current ||
        pageForHistory !== historyPage;

      const deps: Deployment[] = Array.isArray(depsPayload)
        ? depsPayload
        : (depsPayload.items ?? []);

      if (!historyStale) {
        const total =
          typeof depsPayload?.total === "number"
            ? depsPayload.total
            : deps.length;
        setDeployments(deps);
        setDeploymentTotal(total);

        const maxPage = Math.max(0, Math.ceil(total / HISTORY_PAGE_SIZE) - 1);
        if (historyPage > maxPage) {
          setHistoryPage(maxPage);
        }
      }

      if (servicesRes.ok) {
        setServices(await servicesRes.json());
      }

      let latestList = deps;
      if (latestRes) {
        const latestPayload = await latestRes.json();
        latestList = Array.isArray(latestPayload) ? latestPayload : [];
      }

      // Handle real-time logs for deployments (including auto-deploy via webhooks)
      // Skip log updates during manual actions (they stream logs directly)
      if (latestList.length > 0 && !actionLoading) {
        const latestDeployment = latestList[0];
        const isBuilding =
          projectData.status === "building" || projectData.status === "pending";
        const isLatestInProgress = latestDeployment.status === "in_progress";

        // Find the deployment we're currently viewing (may be on another page)
        const viewedDeployment =
          viewingDeploymentId === latestDeployment.id
            ? latestDeployment
            : deps.find((d: Deployment) => d.id === viewingDeploymentId) ||
              null;

        // Detect when to update logs:
        // 1. Initial page load
        // 2. A new deployment started (different ID from what we're viewing)
        // 3. Currently viewing a building deployment (keep updating logs)
        // 4. The deployment we're viewing just finished (get final logs)
        const shouldUpdateLogs =
          loading || // Initial load
          (isLatestInProgress && viewingDeploymentId !== latestDeployment.id) || // New deployment started
          (isBuilding && viewingDeploymentId === latestDeployment.id) || // Current deployment still building
          (viewedDeployment &&
            viewedDeployment.id === latestDeployment.id &&
            latestDeployment.logs); // Refresh viewed deployment logs

        if (shouldUpdateLogs) {
          // Update logs (use empty string fallback to handle initial null state)
          setLogs(latestDeployment.logs || "🚀 Starting deployment...\n");
          setViewingDeploymentId(latestDeployment.id);

          // Jump back to the newest page so the active deploy is visible
          if (
            isLatestInProgress &&
            viewingDeploymentId !== latestDeployment.id &&
            historyPage !== 0
          ) {
            setHistoryPage(0);
          }

          // Auto-switch to deployments tab when a new build starts
          if (isLatestInProgress && activeTab !== "deployments") {
            setActiveTab("deployments");
          }
        }
      }
    } catch (error) {
      if ((error as DOMException)?.name === "AbortError") return;
      console.error(error);
    } finally {
      setLoading(false);
      setHistoryLoading(false);
    }
  }, [
    projectId,
    navigate,
    loading,
    actionLoading,
    historyPage,
    viewingDeploymentId,
    activeTab,
  ]);

  useEffect(() => {
    historyFetchGen.current += 1;
    setHistoryPage(0);
    setDeploymentTotal(0);
  }, [projectId]);

  const historyPageCount = Math.max(
    1,
    Math.ceil(deploymentTotal / HISTORY_PAGE_SIZE),
  );
  const historyFrom =
    deploymentTotal === 0 ? 0 : historyPage * HISTORY_PAGE_SIZE + 1;
  const historyTo = Math.min(
    (historyPage + 1) * HISTORY_PAGE_SIZE,
    deploymentTotal,
  );

  const goHistoryPage = (page: number) => {
    const next = Math.max(0, Math.min(page, historyPageCount - 1));
    if (next === historyPage) return;
    historyFetchGen.current += 1;
    setHistoryLoading(true);
    setHistoryPage(next);
  };

  useEffect(() => {
    fetchProject();
    // Poll faster (2s) when building, slower (5s) otherwise
    const pollInterval = project?.status === "building" ? 2000 : 5000;
    const interval = setInterval(fetchProject, pollInterval);
    return () => clearInterval(interval);
  }, [fetchProject, project?.status]);

  useEffect(() => {
    if (!project || buildInitialized) return;
    setBuildType(project.build_type || "auto");
    setBaseDirectory(project.base_directory || ".");
    setDockerfilePath(project.dockerfile_path || "Dockerfile");
    setInternalPort(project.internal_port || 3000);
    setBuildInitialized(true);
  }, [project, buildInitialized]);

  useEffect(() => {
    if (!storageService && services.length > 0) {
      setStorageService(services[0].name);
    }
  }, [services, storageService]);

  const fetchBuildDetection = useCallback(async () => {
    setBuildDetecting(true);
    try {
      const res = await authFetch(`${API_URL}/api/projects/${projectId}/build/detect`);
      const data = await res.json().catch(() => ({}));
      if (!res.ok) throw new Error(data.error || "Failed to detect build");
      setBuildDetection(data);
    } catch (error) {
      toast.error(error instanceof Error ? error.message : "Failed to detect build");
    } finally {
      setBuildDetecting(false);
    }
  }, [projectId]);

  const fetchStorage = useCallback(async () => {
    setStorageLoading(true);
    try {
      const res = await authFetch(`${API_URL}/api/projects/${projectId}/storage`);
      const data = await res.json().catch(() => []);
      if (!res.ok) throw new Error(data.error || "Failed to load storage");
      setStorageMounts(data);
    } catch (error) {
      toast.error(error instanceof Error ? error.message : "Failed to load storage");
    } finally {
      setStorageLoading(false);
    }
  }, [projectId]);

  useEffect(() => {
    if (activeTab === "build") fetchBuildDetection();
    if (activeTab === "storage") fetchStorage();
  }, [activeTab, fetchBuildDetection, fetchStorage]);

  const handleSaveBuild = async () => {
    if (!baseDirectory.trim()) return toast.error("Base directory is required");
    if (buildType === "dockerfile" && !dockerfilePath.trim()) {
      return toast.error("Dockerfile path is required");
    }
    if (!Number.isInteger(internalPort) || internalPort < 1 || internalPort > 65535) {
      return toast.error("Internal port must be between 1 and 65535");
    }

    setBuildSaving(true);
    try {
      const res = await authFetch(`${API_URL}/api/projects/${projectId}`, {
        method: "PATCH",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({
          build_type: buildType,
          base_directory: baseDirectory.trim(),
          dockerfile_path: buildType === "dockerfile" ? dockerfilePath.trim() : null,
          internal_port: internalPort,
        }),
      });
      const data = await res.json().catch(() => ({}));
      if (!res.ok) throw new Error(data.error || "Failed to save build settings");
      setProject((current) => current ? { ...current, ...data } : current);
      toast.success("Build settings saved — Deploy to build with them");
      await fetchBuildDetection();
    } catch (error) {
      toast.error(error instanceof Error ? error.message : "Failed to save build settings");
    } finally {
      setBuildSaving(false);
    }
  };

  const handleAddStorage = async () => {
    if (!storageService) return toast.error("Select a service");
    if (!storageName.trim()) return toast.error("Storage label is required");
    if (!storagePath.startsWith("/")) return toast.error("Mount path must be absolute");

    setStorageSaving(true);
    try {
      const res = await authFetch(`${API_URL}/api/projects/${projectId}/storage`, {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({
          service_name: storageService,
          name: storageName.trim(),
          mount_path: storagePath.trim(),
        }),
      });
      const data = await res.json().catch(() => ({}));
      if (!res.ok) throw new Error(data.error || "Failed to create storage");
      setStorageName("");
      setStoragePath("");
      toast.success("Persistent storage attached — Deploy to apply it");
      await fetchStorage();
    } catch (error) {
      toast.error(error instanceof Error ? error.message : "Failed to create storage");
    } finally {
      setStorageSaving(false);
    }
  };

  const handleDeleteStorage = async () => {
    if (!storageToDelete) return;
    setStorageSaving(true);
    try {
      const res = await authFetch(
        `${API_URL}/api/projects/${projectId}/storage/${storageToDelete.id}?removeVolume=true`,
        { method: "DELETE" },
      );
      const data = await res.json().catch(() => ({}));
      if (!res.ok) throw new Error(data.error || "Failed to delete storage");
      toast.success("Storage and volume deleted");
      setStorageToDelete(null);
      await fetchStorage();
    } catch (error) {
      toast.error(error instanceof Error ? error.message : "Failed to delete storage");
    } finally {
      setStorageSaving(false);
    }
  };

  // Fetch auto-deploy status
  useEffect(() => {
    if (!projectId) return;

    const fetchAutoDeploy = async () => {
      try {
        const res = await authFetch(
          `${API_URL}/api/projects/${projectId}/auto-deploy`,
        );
        if (res.ok) {
          const data = await res.json();
          setAutoDeploy(data.auto_deploy || false);
        }
      } catch (error) {
        console.error("Failed to fetch auto-deploy status:", error);
      }
    };

    fetchAutoDeploy();
  }, [projectId]);

  // Toggle auto-deploy handler
  const handleAutoDeployToggle = async (enabled: boolean) => {
    setAutoDeployLoading(true);
    try {
      const res = await authFetch(
        `${API_URL}/api/projects/${projectId}/auto-deploy`,
        {
          method: "PATCH",
          headers: { "Content-Type": "application/json" },
          body: JSON.stringify({ enabled }),
        },
      );

      if (res.ok) {
        const data = await res.json();
        setAutoDeploy(data.auto_deploy);
        toast.success(
          enabled ? "Auto-deploy enabled!" : "Auto-deploy disabled",
        );
      } else {
        toast.error("Failed to update auto-deploy");
      }
    } catch (error) {
      toast.error("Failed to update auto-deploy");
    } finally {
      setAutoDeployLoading(false);
    }
  };

  const confirmAction = (action: string) => {
    setPendingAction(action);
    setIsConfirmOpen(true);
  };

  const getActionDetails = () => {
    switch (pendingAction) {
      case "delete":
        return {
          title: "Delete Project",
          description:
            "Are you sure you want to delete this project? This action cannot be undone and will explicitly remove all associated data, containers, and configurations.",
          confirmText: "Yes, Delete Project",
          confirmVariant: "bg-red-600 hover:bg-red-700 text-white border-0",
          icon: <Trash2 className="h-6 w-6 text-red-600" />,
        };
      case "stop":
        return {
          title: "Stop Application",
          description:
            "Are you sure you want to stop the running container? The application will generate 503 errors until started again.",
          confirmText: "Stop Application",
          confirmVariant: "bg-red-500 hover:bg-red-600 text-white border-0",
          icon: <Square className="h-6 w-6 text-red-500" />,
        };
      case "restart":
        return {
          title: "Restart Application",
          description:
            "Are you sure you want to restart the application? This may cause a brief period of downtime for your users.",
          confirmText: "Restart Now",
          confirmVariant: "bg-amber-500 hover:bg-amber-600 text-white border-0",
          icon: <RotateCw className="h-6 w-6 text-amber-500" />,
        };
      case "redeploy":
        return {
          title: "Redeploy Application",
          description:
            "This will rebuild your application from the source and deploy a new version. This process may take a few minutes.",
          confirmText: "Start Redeploy",
          confirmVariant:
            "bg-emerald-600 hover:bg-emerald-700 text-white border-0",
          icon: <Play className="h-6 w-6 text-emerald-600" />,
        };
      case "cancel":
        return {
          title: "Cancel Build",
          description:
            "Are you sure you want to abort the current build process? Partial artifacts may be left behind.",
          confirmText: "Abort Build",
          confirmVariant: "bg-red-600 hover:bg-red-700 text-white border-0",
          icon: <XCircle className="h-6 w-6 text-red-600" />,
        };
      case "deploy":
        return {
          title: "Deploy Application",
          description:
            "Ready to launch? This will start a new build and deployment process for your application.",
          confirmText: "Deploy Now",
          confirmVariant: "bg-cyan-600 hover:bg-cyan-700 text-white border-0",
          icon: <Play className="h-6 w-6 text-cyan-600" />,
        };
      default:
        return {
          title: "Confirm Action",
          description: "Are you sure you want to proceed?",
          confirmText: "Confirm",
          confirmVariant: "default",
          icon: <AlertTriangle className="h-6 w-6 text-amber-500" />,
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

      const res = await authFetch(url, {
        method,
        headers:
          action === "deploy"
            ? { "Content-Type": "application/json" }
            : undefined,
        body:
          action === "deploy"
            ? JSON.stringify({ trigger: "manual" })
            : undefined,
      });

      if (!res.ok) {
        if (action !== "delete") {
          const errorData = await res.json().catch(() => null);
          const errMsg =
            errorData?.error ||
            `Server returned ${res.status} ${res.statusText}`;
          setLogs((prev) => prev + `\n[ERROR] ${errMsg}\n`);
        } else {
          toast.error("Failed to delete project");
        }
        return;
      }

      if (action === "delete") {
        toast.success("Project deleted successfully");
        navigate("/");
        return;
      }

      const streamResult = await consumeProgressStream(res, (line) => {
        setLogs((prev) => prev + line + "\n");
      });

      setTimeout(fetchProject, 1000);
      await new Promise((resolve) => setTimeout(resolve, 1500));
      await fetchProject();

      if (!streamResult.ok) {
        if (streamResult.error) {
          toast.error(streamResult.error);
        }
        return;
      }

      toast.success(
        `${action.charAt(0).toUpperCase() + action.slice(1)} completed!`,
      );
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
      const res = await authFetch(
        `${API_URL}/api/files/${projectId}/content?path=${encodeURIComponent(filePath)}`,
      );
      if (!res.ok) throw new Error("Failed to fetch file content");
      const data = await res.json();
      setEditingFile({ name: filePath, content: data.content });
    } catch (error) {
      console.error(error);
    }
  };

  if (loading) {
    return (
      <div>
        <div className="shimmer h-10 w-48 bg-secondary rounded-xl mb-6" />
        <div className="shimmer h-64 bg-secondary rounded-2xl" />
      </div>
    );
  }

  if (!project) return null;

  return (
    <>
      <div>
        <Link
          to="/"
          className="inline-flex items-center gap-2 text-muted-foreground hover:text-foreground mb-6 transition-colors"
        >
          <ArrowLeft className="h-4 w-4" />
          Back to Projects
        </Link>

        <div className="flex flex-col lg:flex-row lg:items-center lg:justify-between gap-4 sm:gap-6 mb-8 sm:mb-10">
          <div className="flex items-start sm:items-center gap-3 sm:gap-5">
            <div className="h-12 w-12 sm:h-14 sm:w-14 rounded-2xl bg-gradient-to-br from-cyan-500/10 to-blue-600/10 border border-cyan-500/20 flex items-center justify-center shrink-0">
              {project.project_type === "database" ? (
                <Database className="h-6 w-6 sm:h-7 sm:w-7 text-blue-500" />
              ) : (
                <Cloud className="h-6 w-6 sm:h-7 sm:w-7 text-cyan-500" />
              )}
            </div>
            <div className="min-w-0 flex-1">
              <div className="flex flex-wrap items-center gap-2 sm:gap-3">
                <h1 className="text-2xl sm:text-3xl font-bold tracking-tight truncate">
                  {project.name}
                </h1>
                <StatusBadge status={project.status} />
              </div>
              <p className="text-xs sm:text-sm text-muted-foreground mt-1 flex flex-wrap items-center gap-x-3 sm:gap-x-4 gap-y-1">
                <span className="capitalize font-bold text-foreground/80">
                  {project.project_type}
                </span>
                <span className="flex items-center gap-1.5 opacity-70">
                  <Calendar className="h-3 sm:h-3.5 w-3 sm:w-3.5" />
                  <span className="hidden sm:inline">Created </span>
                  {new Date(project.created_at).toLocaleString([], {
                    month: "short",
                    day: "numeric",
                    year: "numeric",
                    hour: "2-digit",
                    minute: "2-digit",
                  })}
                </span>
                <span className="font-mono text-[9px] sm:text-[10px] bg-secondary px-1.5 py-0.5 rounded uppercase tracking-wider">
                  {project.id.split("-")[0]}
                </span>
              </p>
            </div>
          </div>

          <div className="flex flex-wrap items-center gap-2 bg-background/50 backdrop-blur-sm p-1.5 rounded-2xl border border-border/50 shadow-sm w-full lg:w-auto shrink-0">
            {project.status === "running" ? (
              <>
                <Button
                  variant="ghost"
                  size="sm"
                  onClick={() => confirmAction("redeploy")}
                  disabled={actionLoading}
                  className="gap-1 sm:gap-2 h-9 px-3 sm:px-4 text-xs sm:text-sm bg-gradient-to-r from-emerald-500 to-teal-600 hover:from-emerald-600 hover:to-teal-700 text-white shadow-lg shadow-emerald-500/20 border-0 rounded-xl transition-all font-bold flex-1 lg:flex-none"
                >
                  {currentAction === "redeploy" ? (
                    <Loader2 className="h-4 w-4 animate-spin text-white" />
                  ) : (
                    <Play className="h-4 w-4 fill-current" />
                  )}
                  Redeploy
                </Button>

                <Button
                  variant="ghost"
                  size="sm"
                  onClick={() => confirmAction("restart")}
                  disabled={actionLoading}
                  className="gap-2 h-9 px-4 bg-gradient-to-r from-cyan-500 to-blue-600 hover:from-cyan-600 hover:to-blue-700 text-white shadow-lg shadow-cyan-500/20 border-0 rounded-xl transition-all font-bold"
                >
                  {currentAction === "restart" ? (
                    <Loader2 className="h-4 w-4 animate-spin text-white" />
                  ) : (
                    <RotateCw className="h-4 w-4" />
                  )}
                  Restart
                </Button>

                <Button
                  variant="ghost"
                  size="sm"
                  onClick={() => confirmAction("stop")}
                  disabled={actionLoading}
                  className="gap-2 h-9 px-4 bg-gradient-to-r from-amber-500 to-orange-600 hover:from-amber-600 hover:to-orange-700 text-white shadow-lg shadow-amber-500/20 border-0 rounded-xl transition-all font-bold"
                >
                  {currentAction === "stop" ? (
                    <Loader2 className="h-4 w-4 animate-spin text-white" />
                  ) : (
                    <Square className="h-4 w-4 fill-current" />
                  )}
                  Stop
                </Button>
              </>
            ) : project.status === "building" ? (
              <Button
                variant="destructive"
                onClick={() => confirmAction("cancel")}
                disabled={currentAction === "cancel"}
                className="gap-2 h-9 px-4 rounded-xl shadow-red-500/20 font-bold bg-red-600 hover:bg-red-700"
              >
                {currentAction === "cancel" ? (
                  <Loader2 className="h-4 w-4 animate-spin" />
                ) : (
                  <XCircle className="h-4 w-4" />
                )}
                Cancel Build
              </Button>
            ) : (
              <Button
                onClick={() => confirmAction("deploy")}
                disabled={actionLoading}
                className="gap-2 h-9 px-6 bg-gradient-to-r from-cyan-500 to-blue-600 hover:from-cyan-600 hover:to-blue-700 text-white shadow-lg shadow-cyan-500/25 rounded-xl transition-all border-none font-bold"
              >
                {currentAction === "deploy" ? (
                  <Loader2 className="h-4 w-4 animate-spin" />
                ) : (
                  <Play className="h-4 w-4 fill-current" />
                )}
                Deploy Now
              </Button>
            )}

            <div className="w-px h-6 bg-border mx-1" />

            <Button
              variant="ghost"
              size="sm"
              onClick={() => confirmAction("delete")}
              disabled={actionLoading}
              className="gap-2 h-9 px-4 flex-1 lg:flex-none bg-gradient-to-r from-red-500 to-rose-600 hover:from-red-600 hover:to-rose-700 text-white shadow-lg shadow-red-500/25 rounded-xl transition-all border-none font-bold"
              title="Delete Project"
            >
              {currentAction === "delete" ? (
                <Loader2 className="h-4 w-4 animate-spin" />
              ) : (
                <Trash2 className="h-4 w-4" />
              )}
              Delete
            </Button>
          </div>
        </div>

        {/* Stats Grid and Services were removed here as they are now in Overview tab */}

        {/* Stats Grid removed - moved to Overview */}

        <Tabs
          value={activeTab}
          onValueChange={setActiveTab}
          className="space-y-6"
        >
          <div className="sticky top-14 z-10 -mx-4 px-4 md:mx-0 md:px-0 overflow-x-auto md:overflow-visible md:flex md:justify-center">
            <TabsList className="w-max inline-flex justify-start md:justify-center bg-secondary/50 dark:bg-zinc-900 backdrop-blur-xl border border-border/40 dark:border-white/10 p-1 rounded-full shadow-sm items-center gap-1">
              <TabsTrigger
                value="overview"
                className="gap-1.5 sm:gap-2 rounded-full px-3 sm:px-5 py-2 sm:py-2.5 text-xs sm:text-sm font-bold transition-all duration-300 data-[state=active]:bg-white data-[state=active]:dark:bg-cyan-500/20 data-[state=active]:text-cyan-600 data-[state=active]:dark:text-cyan-400 data-[state=active]:shadow-sm data-[state=active]:border-cyan-200/50 data-[state=active]:dark:border-cyan-500/30 border border-transparent hover:bg-secondary/50 whitespace-nowrap"
              >
                <LayoutDashboard className="h-3.5 w-3.5 sm:h-4 sm:w-4" />
                <span className="hidden xs:inline">Overview</span>
                <span className="xs:hidden">View</span>
              </TabsTrigger>
              <TabsTrigger
                value="deployments"
                className="gap-1.5 sm:gap-2 rounded-full px-3 sm:px-5 py-2 sm:py-2.5 text-xs sm:text-sm font-bold transition-all duration-300 data-[state=active]:bg-white data-[state=active]:dark:bg-cyan-500/20 data-[state=active]:text-cyan-600 data-[state=active]:dark:text-cyan-400 data-[state=active]:shadow-sm data-[state=active]:border-cyan-200/50 data-[state=active]:dark:border-cyan-500/30 border border-transparent hover:bg-secondary/50 whitespace-nowrap"
              >
                <Activity className="h-3.5 w-3.5 sm:h-4 sm:w-4" />
                <span className="hidden xs:inline">Deployments</span>
                <span className="xs:hidden">Deploy</span>
              </TabsTrigger>
              <TabsTrigger
                value="env"
                className="gap-1.5 sm:gap-2 rounded-full px-3 sm:px-5 py-2 sm:py-2.5 text-xs sm:text-sm font-bold transition-all duration-300 data-[state=active]:bg-white data-[state=active]:dark:bg-cyan-500/20 data-[state=active]:text-cyan-600 data-[state=active]:dark:text-cyan-400 data-[state=active]:shadow-sm data-[state=active]:border-cyan-200/50 data-[state=active]:dark:border-cyan-500/30 border border-transparent hover:bg-secondary/50 whitespace-nowrap"
              >
                <Key className="h-3.5 w-3.5 sm:h-4 sm:w-4" />
                <span className="hidden xs:inline">Environment</span>
                <span className="xs:hidden">Env</span>
              </TabsTrigger>
              <TabsTrigger
                value="build"
                className="gap-1.5 sm:gap-2 rounded-full px-3 sm:px-5 py-2 sm:py-2.5 text-xs sm:text-sm font-bold transition-all duration-300 data-[state=active]:bg-white data-[state=active]:dark:bg-cyan-500/20 data-[state=active]:text-cyan-600 data-[state=active]:dark:text-cyan-400 data-[state=active]:shadow-sm data-[state=active]:border-cyan-200/50 data-[state=active]:dark:border-cyan-500/30 border border-transparent hover:bg-secondary/50 whitespace-nowrap"
              >
                <Box className="h-3.5 w-3.5 sm:h-4 sm:w-4" />
                Build
              </TabsTrigger>
              <TabsTrigger
                value="storage"
                className="gap-1.5 sm:gap-2 rounded-full px-3 sm:px-5 py-2 sm:py-2.5 text-xs sm:text-sm font-bold transition-all duration-300 data-[state=active]:bg-white data-[state=active]:dark:bg-cyan-500/20 data-[state=active]:text-cyan-600 data-[state=active]:dark:text-cyan-400 data-[state=active]:shadow-sm data-[state=active]:border-cyan-200/50 data-[state=active]:dark:border-cyan-500/30 border border-transparent hover:bg-secondary/50 whitespace-nowrap"
              >
                <HardDrive className="h-3.5 w-3.5 sm:h-4 sm:w-4" />
                Storage
              </TabsTrigger>
              <TabsTrigger
                value="source"
                className="gap-1.5 sm:gap-2 rounded-full px-3 sm:px-5 py-2 sm:py-2.5 text-xs sm:text-sm font-bold transition-all duration-300 data-[state=active]:bg-white data-[state=active]:dark:bg-cyan-500/20 data-[state=active]:text-cyan-600 data-[state=active]:dark:text-cyan-400 data-[state=active]:shadow-sm data-[state=active]:border-cyan-200/50 data-[state=active]:dark:border-cyan-500/30 border border-transparent hover:bg-secondary/50 whitespace-nowrap"
              >
                <FileCode className="h-3.5 w-3.5 sm:h-4 sm:w-4" />
                Source
              </TabsTrigger>
              <TabsTrigger
                value="domains"
                className="gap-1.5 sm:gap-2 rounded-full px-3 sm:px-5 py-2 sm:py-2.5 text-xs sm:text-sm font-bold transition-all duration-300 data-[state=active]:bg-white data-[state=active]:dark:bg-cyan-500/20 data-[state=active]:text-cyan-600 data-[state=active]:dark:text-cyan-400 data-[state=active]:shadow-sm data-[state=active]:border-cyan-200/50 data-[state=active]:dark:border-cyan-500/30 border border-transparent hover:bg-secondary/50 whitespace-nowrap"
              >
                <Globe className="h-3.5 w-3.5 sm:h-4 sm:w-4" />
                Domains
              </TabsTrigger>
              <TabsTrigger
                value="logs"
                className="gap-1.5 sm:gap-2 rounded-full px-3 sm:px-5 py-2 sm:py-2.5 text-xs sm:text-sm font-bold transition-all duration-300 data-[state=active]:bg-white data-[state=active]:dark:bg-cyan-500/20 data-[state=active]:text-cyan-600 data-[state=active]:dark:text-cyan-400 data-[state=active]:shadow-sm data-[state=active]:border-cyan-200/50 data-[state=active]:dark:border-cyan-500/30 border border-transparent hover:bg-secondary/50 whitespace-nowrap"
              >
                <ScrollText className="h-3.5 w-3.5 sm:h-4 sm:w-4" />
                Logs
              </TabsTrigger>
            </TabsList>
          </div>

          <TabsContent
            value="overview"
            className="space-y-8 animate-in fade-in slide-in-from-bottom-2 duration-400"
          >
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
                    const domains = svc.domain
                      ? svc.domain
                          .split(",")
                          .map((d) => d.trim())
                          .filter(Boolean)
                      : [];

                    // Use localhost when in development (browser is on localhost), server IP otherwise
                    const isLocal =
                      typeof window !== "undefined" &&
                      (window.location.hostname === "localhost" ||
                        window.location.hostname === "127.0.0.1");
                    const portHost = isLocal ? "localhost" : serverIP;

                    const portEndpoint = {
                      url: `http://${portHost}:${svc.port}`,
                      label: `${portHost}:${svc.port}`,
                      isPort: true,
                    };

                    // Always show port endpoint + any configured domains
                    const domainLinks = domains.map((d) => ({
                      url: `https://${d}`,
                      label: d,
                      isPort: false,
                    }));

                    return (
                      <Card
                        key={svc.id}
                        className="group relative overflow-hidden p-0 border-border/40 hover:border-cyan-500/50 transition-all duration-300 shadow-sm hover:shadow-cyan-500/5"
                      >
                        <div className="absolute inset-y-0 left-0 w-1 bg-cyan-500 opacity-0 group-hover:opacity-100 transition-opacity" />
                        <div className="flex flex-col sm:flex-row sm:items-center sm:justify-between gap-3 sm:gap-5 p-4 sm:p-5">
                          {/* Left: Icon + Name + Status */}
                          <div className="flex items-start sm:items-center gap-3 sm:gap-5 min-w-0">
                            <div className="h-10 w-10 sm:h-12 sm:w-12 rounded-xl sm:rounded-2xl bg-secondary/50 flex items-center justify-center group-hover:bg-cyan-500/10 transition-colors shrink-0">
                              <Server className="h-5 w-5 sm:h-6 sm:w-6 text-muted-foreground group-hover:text-cyan-500 transition-colors" />
                            </div>
                            <div className="min-w-0 flex-1">
                              <div className="flex flex-wrap items-center gap-2 sm:gap-3">
                                <span className="font-bold text-base sm:text-lg truncate">
                                  {svc.name}
                                </span>
                                <StatusBadge status={svc.status} />
                              </div>
                              <p className="text-[10px] sm:text-xs text-muted-foreground mt-0.5 sm:mt-1 flex flex-wrap items-center gap-1.5 sm:gap-2">
                                <span className="font-mono bg-secondary/80 px-1 sm:px-1.5 py-0.5 rounded text-[9px] sm:text-xs">
                                  Port {svc.internal_port}
                                </span>
                                <span className="text-muted-foreground/30 hidden sm:inline">
                                  /
                                </span>
                                <span className="flex items-center gap-1 truncate">
                                  <FileCode className="h-2.5 w-2.5 sm:h-3 sm:w-3 shrink-0" />
                                  <span className="truncate">
                                    {svc.dockerfile_path}
                                  </span>
                                </span>
                              </p>
                            </div>
                          </div>

                          {/* Right: Endpoint links */}
                          <div className="flex flex-wrap gap-2 sm:flex-col sm:gap-2 sm:items-end ml-0 sm:ml-auto shrink-0">
                            {/* Always show IP:Port access */}
                            <a
                              href={portEndpoint.url}
                              target="_blank"
                              rel="noopener noreferrer"
                              className="h-7 sm:h-9 px-2.5 sm:px-4 flex items-center gap-1.5 sm:gap-2 rounded-lg sm:rounded-xl bg-secondary/50 hover:bg-cyan-500 text-muted-foreground hover:text-white font-mono text-[10px] sm:text-xs font-bold transition-all duration-300 border border-border/50"
                            >
                              <span className="truncate max-w-[120px] sm:max-w-none">
                                {portEndpoint.label}
                              </span>
                              <ExternalLink className="h-2.5 w-2.5 sm:h-3 sm:w-3 shrink-0" />
                            </a>
                            {/* Show domain links if configured */}
                            {domainLinks.map((link, idx) => (
                              <a
                                key={idx}
                                href={link.url}
                                target="_blank"
                                rel="noopener noreferrer"
                                className="h-7 sm:h-9 px-2.5 sm:px-4 flex items-center gap-1.5 sm:gap-2 rounded-lg sm:rounded-xl bg-cyan-500/10 hover:bg-cyan-500 text-cyan-600 hover:text-white font-mono text-[10px] sm:text-xs font-bold transition-all duration-300"
                              >
                                <span className="truncate max-w-[120px] sm:max-w-none">
                                  {link.label}
                                </span>
                                <ExternalLink className="h-2.5 w-2.5 sm:h-3 sm:w-3 shrink-0" />
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
            <div className="grid grid-cols-2 sm:grid-cols-2 lg:grid-cols-4 gap-3 sm:gap-5">
              <Card className="p-4 sm:p-6 border-border/40 hover:border-border transition-colors">
                <div className="flex items-center justify-between mb-2 sm:mb-4">
                  <div className="p-2 sm:p-2.5 rounded-lg sm:rounded-xl bg-orange-500/10">
                    <Activity className="h-4 w-4 sm:h-5 sm:w-5 text-orange-500" />
                  </div>
                  <span className="text-[10px] font-bold text-muted-foreground uppercase tracking-widest">
                    Health
                  </span>
                </div>
                <div className="space-y-0.5 sm:space-y-1">
                  <p className="text-lg sm:text-2xl font-bold tracking-tight capitalize">
                    {project.status}
                  </p>
                  <p className="text-[10px] sm:text-xs text-muted-foreground">
                    Main service status
                  </p>
                </div>
              </Card>

              <Card className="p-4 sm:p-6 border-border/40 hover:border-border transition-colors">
                <div className="flex items-center justify-between mb-2 sm:mb-4">
                  <div className="p-2 sm:p-2.5 rounded-lg sm:rounded-xl bg-purple-500/10">
                    <History className="h-4 w-4 sm:h-5 sm:w-5 text-purple-500" />
                  </div>
                  <span className="text-[10px] font-bold text-muted-foreground uppercase tracking-widest">
                    Activity
                  </span>
                </div>
                <div className="space-y-0.5 sm:space-y-1">
                  <p className="text-lg sm:text-2xl font-bold tracking-tight">
                    {deploymentTotal}
                  </p>
                  <p className="text-[10px] sm:text-xs text-muted-foreground">
                    Total deployments
                  </p>
                </div>
              </Card>

              <Card className="p-4 sm:p-6 border-border/40 hover:border-border transition-colors">
                <div className="flex items-center justify-between mb-2 sm:mb-4">
                  <div className="p-2 sm:p-2.5 rounded-lg sm:rounded-xl bg-blue-500/10">
                    <Cpu className="h-4 w-4 sm:h-5 sm:w-5 text-blue-500" />
                  </div>
                  <span className="text-[10px] font-bold text-muted-foreground uppercase tracking-widest">
                    Resource
                  </span>
                </div>
                <div className="space-y-0.5 sm:space-y-1">
                  <p className="text-lg sm:text-2xl font-bold tracking-tight">
                    {project.source_type === "github" ? "GitHub" : "Upload"}
                  </p>
                  <p className="text-[10px] sm:text-xs text-muted-foreground">
                    Source type
                  </p>
                </div>
              </Card>

              <Card className="p-4 sm:p-6 border-border/40 hover:border-border transition-colors">
                <div className="flex items-center justify-between mb-2 sm:mb-4">
                  <div className="p-2 sm:p-2.5 rounded-lg sm:rounded-xl bg-cyan-500/10">
                    <Shield className="h-4 w-4 sm:h-5 sm:w-5 text-cyan-500" />
                  </div>
                  <span className="text-[10px] font-bold text-muted-foreground uppercase tracking-widest">
                    Privacy
                  </span>
                </div>
                <div className="space-y-0.5 sm:space-y-1">
                  <p className="text-lg sm:text-2xl font-bold tracking-tight">
                    Isolated
                  </p>
                  <p className="text-[10px] sm:text-xs text-muted-foreground">
                    Network namespace
                  </p>
                </div>
              </Card>
            </div>

            {/* GitHub Info if applicable */}
            {project.source_type === "github" && (
              <Card className="p-0 border-border/40 overflow-hidden">
                <div className="bg-secondary/30 px-6 py-4 border-b flex items-center justify-between">
                  <div className="flex items-center gap-2">
                    <GitBranch className="h-4 w-4 text-muted-foreground" />
                    <span className="text-sm font-semibold italic text-muted-foreground uppercase tracking-widest">
                      Source Configuration
                    </span>
                  </div>
                  <a
                    href={project.github_url || "#"}
                    target="_blank"
                    rel="noopener noreferrer"
                    className="text-xs font-bold text-cyan-500 hover:text-cyan-400 flex items-center gap-1.5 transition-colors"
                  >
                    VIEW REPO <ExternalLink className="h-3 w-3" />
                  </a>
                </div>
                <div className="p-6 grid grid-cols-1 md:grid-cols-2 gap-8">
                  <div className="space-y-4">
                    <div className="flex flex-col gap-1.5">
                      <span className="text-[10px] font-bold text-muted-foreground uppercase tracking-widest">
                        Repository URL
                      </span>
                      <p className="text-sm font-mono bg-secondary/50 p-3 rounded-xl border border-border/50 truncate">
                        {project.github_url}
                      </p>
                    </div>
                  </div>
                  <div className="flex items-center gap-6">
                    <div className="flex flex-col gap-1.5">
                      <span className="text-[10px] font-bold text-muted-foreground uppercase tracking-widest">
                        Selected Branch
                      </span>
                      <div className="flex items-center gap-2 px-3 py-1.5 bg-cyan-500/10 text-cyan-500 rounded-lg border border-cyan-500/20 font-mono text-xs font-bold">
                        <GitBranch className="h-3.5 w-3.5" />
                        {project.github_branch}
                      </div>
                    </div>
                  </div>
                </div>

                {/* Auto-Deploy Section */}
                <div className="border-t border-border/40 p-4 sm:p-6">
                  <div className="flex flex-col sm:flex-row sm:items-center sm:justify-between gap-3">
                    <div className="flex items-center gap-3 min-w-0">
                      <div className="p-2 rounded-lg bg-emerald-500/10 shrink-0">
                        <Rocket className="h-4 w-4 text-emerald-500" />
                      </div>
                      <div className="min-w-0">
                        <span className="font-semibold text-sm sm:text-base">
                          Auto-Deploy
                        </span>
                        <p className="text-[10px] sm:text-xs text-muted-foreground">
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
                        "relative h-6 w-11 rounded-full transition-colors duration-200 focus:outline-none focus:ring-2 focus:ring-offset-2 focus:ring-emerald-500 shrink-0 self-end sm:self-auto",
                        autoDeploy ? "bg-emerald-500" : "bg-secondary",
                      )}
                    >
                      <span
                        className={cn(
                          "absolute top-0.5 left-0.5 h-5 w-5 rounded-full bg-white shadow-sm transition-transform duration-200",
                          autoDeploy ? "translate-x-5" : "translate-x-0",
                        )}
                      />
                    </button>
                  </div>
                </div>
              </Card>
            )}
          </TabsContent>

          <TabsContent
            value="deployments"
            className="animate-in fade-in slide-in-from-bottom-2 duration-400"
          >
            <div className="grid grid-cols-1 gap-6 xl:grid-cols-3">
              <div className="min-w-0 space-y-4 sm:space-y-6 xl:col-span-2">
                <div className="flex flex-wrap items-center justify-between gap-2">
                  <h3 className="text-lg font-bold flex items-center gap-2 sm:text-xl">
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
                <Terminal
                  logs={logs}
                  isBuilding={project?.status === "building"}
                  className="h-[min(55vh,28rem)] sm:h-[550px]"
                />
              </div>

              <div className="min-w-0 space-y-4 sm:space-y-6">
                <div className="flex flex-wrap items-end justify-between gap-2">
                  <h3 className="text-lg font-bold flex items-center gap-2 sm:text-xl">
                    <History className="h-5 w-5 text-muted-foreground" />
                    Recent History
                  </h3>
                  {deploymentTotal > 0 && (
                    <span className="text-[11px] tabular-nums text-muted-foreground">
                      {historyFrom}–{historyTo} of {deploymentTotal}
                    </span>
                  )}
                </div>
                <Card
                  className={cn(
                    "divide-y divide-border/40 border-border/40 overflow-hidden relative transition-opacity",
                    historyLoading && "opacity-60",
                  )}
                >
                  {project.status === "building" && (
                    <div className="absolute top-0 left-0 w-full h-1 bg-secondary overflow-hidden z-10">
                      <div className="absolute top-0 h-full bg-gradient-to-r from-cyan-500 via-blue-500 to-cyan-500 animate-progress-scan" />
                    </div>
                  )}
                  {deployments.length === 0 ? (
                    <div className="text-muted-foreground text-sm py-16 sm:py-20 text-center flex flex-col items-center gap-3 px-4">
                      <div className="p-4 rounded-full bg-secondary/50">
                        <History className="h-8 w-8 text-muted-foreground/30" />
                      </div>
                      No deployments found
                    </div>
                  ) : (
                    deployments.map((deployment) => {
                      const isViewing = viewingDeploymentId === deployment.id;
                      const isInProgress = deployment.status === "in_progress";
                      return (
                        <div
                          key={deployment.id}
                          className={cn(
                            "flex flex-col gap-2 px-3 py-3 sm:px-5 sm:py-4 cursor-pointer transition-all duration-200 border-l-4 min-w-0",
                            deployment.status === "success"
                              ? "border-l-emerald-500 lg:hover:bg-emerald-500/5"
                              : deployment.status === "failed"
                                ? "border-l-red-500 lg:hover:bg-red-500/5"
                                : deployment.status === "cancelled"
                                  ? "border-l-slate-500 lg:hover:bg-slate-500/5"
                                : "border-l-amber-500",
                            isViewing &&
                              "bg-secondary/30 ring-1 ring-inset ring-border/50",
                            isInProgress &&
                              isViewing &&
                              "animate-pulse bg-amber-500/5",
                          )}
                          onClick={() => {
                            setLogs(
                              deployment.logs ||
                                "No logs available for this deployment",
                            );
                            setViewingDeploymentId(deployment.id); // Track which deployment user is viewing
                            const terminalElement =
                              document.getElementById("terminal-wrapper");
                            terminalElement?.scrollIntoView({
                              behavior: "smooth",
                              block: "center",
                            });
                          }}
                        >
                          <div className="flex items-start justify-between gap-2 min-w-0">
                            <div className="flex flex-col gap-1 min-w-0">
                              <span className="font-bold text-sm tracking-tight truncate">
                                {deployment.trigger === "webhook"
                                  ? "Auto-Deploy (GitHub)"
                                  : deployment.trigger === "restart"
                                    ? "Restart Action"
                                    : deployment.trigger === "stop"
                                      ? "Stop Action"
                                      : deployment.trigger === "redeploy"
                                        ? "Manual Redeploy"
                                        : "Manual Deployment"}
                              </span>
                              <div className="flex flex-wrap items-center gap-1.5">
                                <span
                                  className={cn(
                                    "text-[10px] font-bold px-1.5 py-0.5 rounded-md uppercase tracking-wider",
                                    deployment.status === "success"
                                      ? "bg-emerald-500/10 text-emerald-500"
                                      : deployment.status === "failed"
                                        ? "bg-red-500/10 text-red-500"
                                        : deployment.status === "cancelled"
                                          ? "bg-slate-500/10 text-slate-500"
                                        : "bg-amber-500/10 text-amber-500",
                                  )}
                                >
                                  {deployment.status}
                                </span>
                                <span className="text-[10px] font-mono text-muted-foreground opacity-60">
                                  # {deployment.id.split("-")[0]}
                                </span>
                              </div>
                            </div>
                            <div className="flex shrink-0 flex-col items-end gap-1">
                              <span className="text-[10px] font-medium text-muted-foreground flex items-center gap-1 sm:gap-1.5 bg-secondary/30 px-1.5 sm:px-2 py-0.5 rounded-full border border-border/20 whitespace-nowrap">
                                <Clock className="h-3 w-3" />
                                {new Date(
                                  deployment.created_at,
                                ).toLocaleTimeString([], {
                                  hour: "2-digit",
                                  minute: "2-digit",
                                })}
                              </span>
                              <span className="text-[10px] text-muted-foreground/60 flex items-center gap-1 sm:gap-1.5 whitespace-nowrap">
                                <Calendar className="h-3 w-3" />
                                {new Date(
                                  deployment.created_at,
                                ).toLocaleDateString([], {
                                  month: "short",
                                  day: "numeric",
                                  year: "numeric",
                                })}
                              </span>
                            </div>
                          </div>
                          <p className="text-[11px] text-muted-foreground line-clamp-1 italic hidden sm:block">
                            Click to restore log output to terminal
                          </p>

                          {deployment.commit_message && (
                            <div className="mt-1 sm:mt-2 flex items-center gap-2 px-2.5 py-1.5 sm:px-3 sm:py-2 bg-secondary/30 rounded-lg border border-border/20 min-w-0">
                              <GitBranch className="h-3 w-3 shrink-0 text-cyan-500" />
                              <span className="text-[10px] font-medium text-foreground/80 line-clamp-1 min-w-0">
                                {deployment.commit_message}
                              </span>
                            </div>
                          )}
                        </div>
                      );
                    })
                  )}

                  {deploymentTotal > HISTORY_PAGE_SIZE && (
                    <div className="p-3 sm:p-4 bg-secondary/20 flex flex-wrap items-center justify-between gap-2 border-t border-border/40">
                      <Button
                        variant="ghost"
                        size="sm"
                        disabled={historyPage <= 0 || historyLoading}
                        onClick={(e) => {
                          e.stopPropagation();
                          goHistoryPage(historyPage - 1);
                        }}
                        className="text-xs font-bold gap-1.5 hover:bg-cyan-500/10 hover:text-cyan-500 transition-all"
                        aria-label="Previous history page"
                      >
                        <ChevronLeft className="h-3.5 w-3.5" />
                        Prev
                      </Button>
                      <span className="text-[11px] font-medium tabular-nums text-muted-foreground order-first w-full text-center sm:order-none sm:w-auto">
                        Page {historyPage + 1} of {historyPageCount}
                      </span>
                      <Button
                        variant="ghost"
                        size="sm"
                        disabled={
                          historyPage >= historyPageCount - 1 || historyLoading
                        }
                        onClick={(e) => {
                          e.stopPropagation();
                          goHistoryPage(historyPage + 1);
                        }}
                        className="text-xs font-bold gap-1.5 hover:bg-cyan-500/10 hover:text-cyan-500 transition-all ml-auto sm:ml-0"
                        aria-label="Next history page"
                      >
                        Next
                        <ChevronRight className="h-3.5 w-3.5" />
                      </Button>
                    </div>
                  )}
                </Card>
              </div>
            </div>
          </TabsContent>

          <TabsContent
            value="env"
            className="animate-in fade-in slide-in-from-bottom-2 duration-400"
          >
            <div className="max-w-4xl mx-auto space-y-6">
              <div className="flex items-center justify-between">
                <div>
                  <h3 className="text-xl font-bold flex items-center gap-2">
                    <Key className="h-5 w-5 text-cyan-500" />
                    Environment Variables
                  </h3>
                  <p className="text-sm text-muted-foreground mt-1">
                    Manage secret values and configuration for your deployment
                    environment.
                  </p>
                </div>
              </div>
              <EnvVarsManager projectId={projectId} />
            </div>
          </TabsContent>

          <TabsContent
            value="build"
            className="animate-in fade-in slide-in-from-bottom-2 duration-400"
          >
            <div className="max-w-4xl mx-auto space-y-6">
              <div>
                <h3 className="text-xl font-bold flex items-center gap-2">
                  <Box className="h-5 w-5 text-orange-500" />
                  Build Settings
                </h3>
                <p className="text-sm text-muted-foreground mt-1">
                  Configure the builder, source directory, and container port used by deployments.
                </p>
              </div>

              <Card className="p-6 space-y-6 border-border/40">
                <div className="grid grid-cols-1 sm:grid-cols-3 gap-3">
                  {([
                    ["auto", "Auto", "Detect Dockerfile or app manifests"],
                    ["dockerfile", "Dockerfile", "Build with a specific Dockerfile"],
                    ["railpack", "Railpack", "Generate an image from manifests"],
                  ] as const).map(([value, label, description]) => (
                    <button
                      key={value}
                      type="button"
                      onClick={() => setBuildType(value)}
                      className={cn(
                        "p-4 rounded-xl border text-left transition-all",
                        buildType === value
                          ? "border-orange-500/40 bg-orange-500/5"
                          : "border-border/40 hover:bg-secondary/30",
                      )}
                    >
                      <span className="font-bold text-sm">{label}</span>
                      <span className="block text-[11px] text-muted-foreground mt-1">
                        {description}
                      </span>
                    </button>
                  ))}
                </div>

                <div className="grid grid-cols-1 sm:grid-cols-2 gap-5">
                  <div className="space-y-2">
                    <label className="text-sm font-medium">Base Directory</label>
                    <Input
                      value={baseDirectory}
                      onChange={(e) => setBaseDirectory(e.target.value)}
                      placeholder="."
                      className="font-mono bg-secondary/20"
                    />
                    <p className="text-[11px] text-muted-foreground">
                      Relative to the repository root, such as <code>apps/web</code>.
                    </p>
                  </div>
                  <div className="space-y-2">
                    <label className="text-sm font-medium">Internal Port</label>
                    <Input
                      type="number"
                      min={1}
                      max={65535}
                      value={internalPort}
                      onChange={(e) => setInternalPort(Number(e.target.value))}
                      className="font-mono bg-secondary/20"
                    />
                    <p className="text-[11px] text-muted-foreground">
                      Port exposed by the application inside its container.
                    </p>
                  </div>
                </div>

                {buildType === "dockerfile" && (
                  <div className="space-y-2">
                    <label className="text-sm font-medium">Dockerfile Path</label>
                    <Input
                      value={dockerfilePath}
                      onChange={(e) => setDockerfilePath(e.target.value)}
                      placeholder="Dockerfile"
                      className="font-mono bg-secondary/20"
                    />
                    <p className="text-[11px] text-muted-foreground">
                      Path relative to the base directory.
                    </p>
                  </div>
                )}

                <div className="flex justify-end">
                  <Button onClick={handleSaveBuild} disabled={buildSaving}>
                    {buildSaving && <Loader2 className="h-4 w-4 animate-spin mr-2" />}
                    Save Build Settings
                  </Button>
                </div>
              </Card>

              <Card className="p-6 border-border/40">
                <div className="flex items-center justify-between gap-4 mb-5">
                  <div>
                    <h4 className="font-bold">Detected Build</h4>
                    <p className="text-xs text-muted-foreground mt-1">
                      DockLift&apos;s current resolution for this source.
                    </p>
                  </div>
                  <Button
                    variant="outline"
                    size="sm"
                    onClick={fetchBuildDetection}
                    disabled={buildDetecting}
                  >
                    <RefreshCw className={cn("h-3.5 w-3.5 mr-2", buildDetecting && "animate-spin")} />
                    Detect Again
                  </Button>
                </div>
                {buildDetection ? (
                  <div className="grid grid-cols-1 sm:grid-cols-2 gap-4">
                    <div className="p-4 rounded-xl bg-secondary/30 border border-border/40">
                      <span className="text-[10px] font-bold uppercase tracking-widest text-muted-foreground">
                        Requested
                      </span>
                      <p className="font-bold capitalize mt-1">{buildDetection.requestedType}</p>
                    </div>
                    <div className="p-4 rounded-xl bg-orange-500/5 border border-orange-500/20">
                      <span className="text-[10px] font-bold uppercase tracking-widest text-orange-500">
                        Resolved
                      </span>
                      <p className="font-bold capitalize mt-1">{buildDetection.resolvedType}</p>
                    </div>
                    <div className="sm:col-span-2 text-xs text-muted-foreground space-y-2">
                      <p>
                        Base: <code>{buildDetection.baseDirectory}</code>
                        {buildDetection.dockerfilePath && (
                          <> · Dockerfile: <code>{buildDetection.dockerfilePath}</code></>
                        )}
                      </p>
                      <p>
                        {buildDetection.detected}
                        {buildDetection.manifests.length > 0 && (
                          <> Manifests: {buildDetection.manifests.join(", ")}</>
                        )}
                      </p>
                    </div>
                  </div>
                ) : (
                  <div className="py-8 text-center text-sm text-muted-foreground">
                    {buildDetecting ? "Detecting build configuration…" : "No detection result available."}
                  </div>
                )}
              </Card>
            </div>
          </TabsContent>

          <TabsContent
            value="storage"
            className="animate-in fade-in slide-in-from-bottom-2 duration-400"
          >
            <div className="max-w-4xl mx-auto space-y-6">
              <div>
                <h3 className="text-xl font-bold flex items-center gap-2">
                  <HardDrive className="h-5 w-5 text-blue-500" />
                  Persistent Storage
                </h3>
                <p className="text-sm text-muted-foreground mt-1">
                  Attach Docker volumes to keep application data across redeployments. Deploy after changing mounts.
                </p>
              </div>

              <Card className="p-5 border-blue-500/20 bg-blue-500/5">
                <div className="flex gap-3">
                  <Info className="h-5 w-5 text-blue-500 shrink-0 mt-0.5" />
                  <div className="text-xs text-muted-foreground leading-relaxed space-y-1">
                    <p>An external <code>DATABASE_URL</code> points to a separate database and is unaffected by these mounts.</p>
                    <p>SQLite files and user uploads need a mount at the directory where the app writes them. Application migrations may still modify or remove stored data, so keep backups.</p>
                  </div>
                </div>
              </Card>

              <Card className="p-6 space-y-5 border-border/40">
                <h4 className="font-bold">Attach Storage</h4>
                <div className="grid grid-cols-1 sm:grid-cols-3 gap-4">
                  <div className="space-y-2">
                    <label className="text-sm font-medium">Service</label>
                    <select
                      value={storageService}
                      onChange={(e) => setStorageService(e.target.value)}
                      disabled={services.length === 0}
                      className="flex h-10 w-full rounded-md border border-input bg-background px-3 py-2 text-sm focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring disabled:opacity-50"
                    >
                      {services.length === 0 && <option value="">No services available</option>}
                      {services.map((service) => (
                        <option key={service.id} value={service.name}>{service.name}</option>
                      ))}
                    </select>
                  </div>
                  <div className="space-y-2">
                    <label className="text-sm font-medium">Label</label>
                    <Input
                      value={storageName}
                      onChange={(e) => setStorageName(e.target.value)}
                      placeholder="uploads"
                    />
                  </div>
                  <div className="space-y-2">
                    <label className="text-sm font-medium">Absolute Mount Path</label>
                    <Input
                      value={storagePath}
                      onChange={(e) => setStoragePath(e.target.value)}
                      placeholder="/app/uploads"
                      className="font-mono"
                    />
                  </div>
                </div>
                <div className="flex justify-end">
                  <Button
                    onClick={handleAddStorage}
                    disabled={storageSaving || services.length === 0}
                  >
                    {storageSaving ? <Loader2 className="h-4 w-4 animate-spin mr-2" /> : <Plus className="h-4 w-4 mr-2" />}
                    Attach Storage
                  </Button>
                </div>
              </Card>

              <div className="space-y-3">
                <h4 className="font-bold">Attached Mounts</h4>
                {storageLoading ? (
                  <div className="py-10 flex justify-center"><Loader2 className="h-5 w-5 animate-spin text-muted-foreground" /></div>
                ) : storageMounts.length === 0 ? (
                  <Card className="p-10 text-center text-sm text-muted-foreground border-dashed">
                    No persistent storage attached.
                  </Card>
                ) : storageMounts.map((mount) => (
                  <Card key={mount.id} className="p-4 border-border/40">
                    <div className="flex items-center justify-between gap-4">
                      <div className="flex items-center gap-3 min-w-0">
                        <div className="p-2.5 rounded-xl bg-blue-500/10">
                          <HardDrive className="h-5 w-5 text-blue-500" />
                        </div>
                        <div className="min-w-0">
                          <p className="font-bold truncate">{mount.display_name || mount.name}</p>
                          <p className="text-xs text-muted-foreground truncate">
                            {mount.service_name} · <code>{mount.mount_path}</code>
                          </p>
                        </div>
                      </div>
                      <Button
                        variant="ghost"
                        size="icon"
                        onClick={() => setStorageToDelete(mount)}
                        className="text-muted-foreground hover:text-red-500 hover:bg-red-500/10 shrink-0"
                        title="Delete storage"
                      >
                        <Trash2 className="h-4 w-4" />
                      </Button>
                    </div>
                  </Card>
                ))}
              </div>
            </div>
          </TabsContent>

          <TabsContent
            value="source"
            className="animate-in fade-in slide-in-from-bottom-2 duration-400"
          >
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

          <TabsContent
            value="domains"
            className="animate-in fade-in slide-in-from-bottom-2 duration-400"
          >
            <div className="mx-auto max-w-4xl space-y-6">
              <DnsGuideCard serverIP={serverIP} />

              <div className="space-y-3">
                <div>
                  <h3 className="flex items-center gap-2 text-lg font-bold sm:text-xl">
                    <Globe className="h-5 w-5 text-brand" />
                    Service domains
                  </h3>
                  <p className="mt-1 text-xs text-muted-foreground sm:text-sm">
                    Adding a domain saves it immediately, wires up the reverse proxy and
                    requests a certificate. Progress and failures show up per domain.
                  </p>
                </div>

                <div className="grid gap-4">
                  {services.map((svc) => (
                    <ServiceDomainCard
                      key={svc.id}
                      service={svc}
                      projectId={projectId}
                      serverIP={serverIP}
                      onUpdate={fetchProject}
                    />
                  ))}

                  {services.length === 0 && (
                    <div className="rounded-xl border border-dashed border-border bg-secondary/20 py-12 text-center text-muted-foreground">
                      No services found for this project.
                    </div>
                  )}
                </div>
              </div>
            </div>
          </TabsContent>

          <TabsContent
            value="logs"
            className="animate-in fade-in slide-in-from-bottom-2 duration-400"
          >
            <ContainerLogsPanel
              projectId={projectId}
              services={services}
              activeTab={activeTab}
            />
          </TabsContent>
        </Tabs>
      </div>

      {editingFile && (
        <FileEditor
          projectId={projectId}
          filename={editingFile.name}
          content={editingFile.content}
          onClose={() => setEditingFile(null)}
          onSave={() => {
            setEditingFile(null);
            fetchProject();
          }}
        />
      )}

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
              {actionLoading ? (
                <Loader2 className="h-4 w-4 animate-spin mr-2" />
              ) : null}
              {actionDetails.confirmText}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
      <Dialog
        open={!!storageToDelete}
        onOpenChange={(open) => !open && setStorageToDelete(null)}
      >
        <DialogContent className="sm:max-w-md">
          <DialogHeader>
            <DialogTitle className="flex items-center gap-2">
              <Trash2 className="h-6 w-6 text-red-600" />
              Delete Persistent Storage
            </DialogTitle>
            <DialogDescription className="pt-2">
              This permanently deletes the <strong>{storageToDelete?.display_name || storageToDelete?.name}</strong> mount and its Docker volume. Stop the project first. Stored files cannot be recovered.
            </DialogDescription>
          </DialogHeader>
          <DialogFooter className="gap-2 sm:gap-0">
            <Button variant="outline" onClick={() => setStorageToDelete(null)}>
              Cancel
            </Button>
            <Button
              onClick={handleDeleteStorage}
              disabled={storageSaving}
              className="bg-red-600 hover:bg-red-700 text-white"
            >
              {storageSaving && <Loader2 className="h-4 w-4 animate-spin mr-2" />}
              Delete Storage and Data
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </>
  );
}
