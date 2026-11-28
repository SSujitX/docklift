// Project detail page - overview, deployments, env vars, source files, and domain management

import { useEffect, useState, useCallback, useRef, type ReactNode } from "react";
import { useParams, useNavigate, useSearchParams } from "react-router-dom";
import { useBreadcrumbLeaf } from "@/components/shell/ShellContext";
import { StatusBadge } from "@/components/StatusBadge";
import { Terminal } from "@/components/Terminal";
import { LogViewer } from "@/components/LogViewer";
import { FileEditor } from "@/components/FileEditor";
import { FileTree } from "@/components/FileTree";
import { EnvVarsManager } from "@/components/EnvVarsManager";
import { DnsGuideCard } from "@/components/domains/DnsGuideCard";
import { ServiceDomainCard } from "@/components/domains/ServiceDomainCard";
import {
  ServiceSwitcher,
  type ProjectWorkspace,
} from "@/components/project/ServiceSwitcher";
import { ProjectActionBar } from "@/components/project/ProjectActionBar";
import { ManagedDatabasePanel } from "@/components/databases/ManagedDatabasePanel";
import { AttachDatabasePanel } from "@/components/databases/AttachDatabasePanel";
import { Button } from "@/components/ui/button";
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
import { apiCache, authFetch } from "@/lib/auth";
import { API, projectAPI } from "@/lib/prefetch";
import { consumeProgressStream } from "@/lib/streamProgress";
import {
  Loader2,
  ExternalLink,
  Trash2,
  RefreshCw,
  ChevronDown,
  ChevronLeft,
  ChevronRight,
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

function deploymentList(payload: unknown): Deployment[] {
  if (Array.isArray(payload)) return payload;
  const items = (payload as { items?: unknown } | undefined)?.items;
  return Array.isArray(items) ? items : [];
}

/** Last known data for a project (memory cache only); `project` is null when not cached. */
function readProjectCache(projectId: string) {
  const historyPayload = apiCache.peek<{ total?: number } | Deployment[]>(
    projectAPI.history(projectId, HISTORY_PAGE_SIZE, 0),
  );
  const deployments = deploymentList(historyPayload);
  const total = (historyPayload as { total?: number } | undefined)?.total;
  const latestSuccess = apiCache.peek(projectAPI.latestSuccess(projectId));
  return {
    project: apiCache.peek<Project>(projectAPI.project(projectId)) ?? null,
    files: apiCache.peek<ProjectFile[]>(projectAPI.files(projectId)),
    services: apiCache.peek<Service[]>(projectAPI.services(projectId)),
    deployments,
    deploymentTotal: typeof total === "number" ? total : deployments.length,
    latestSuccessId:
      latestSuccess === undefined ? null : deploymentList(latestSuccess)[0]?.id ?? null,
    autoDeploy: Boolean(
      apiCache.peek<{ auto_deploy?: boolean }>(projectAPI.autoDeploy(projectId))?.auto_deploy,
    ),
  };
}

const DEPLOY_STATUS: Record<string, { label: string; dot: string }> = {
  success: { label: "Succeeded", dot: "bg-ok" },
  failed: { label: "Failed", dot: "bg-danger" },
  cancelled: { label: "Cancelled", dot: "bg-subtle" },
  in_progress: {
    label: "In progress",
    dot: "bg-warn animate-pulse motion-reduce:animate-none",
  },
  queued: { label: "Queued", dot: "bg-info" },
  pending: { label: "Pending", dot: "bg-info" },
};

function deploymentTriggerLabel(trigger?: string) {
  switch (trigger) {
    case "webhook":
      return "Auto-deploy (GitHub)";
    case "restart":
      return "Restart";
    case "stop":
      return "Stop";
    case "redeploy":
      return "Manual redeploy";
    case "rollback":
      return "Restore previous";
    default:
      return "Manual deploy";
  }
}

function formatDuration(start: string, end: string | null) {
  if (!end) return null;
  const ms = new Date(end).getTime() - new Date(start).getTime();
  if (!Number.isFinite(ms) || ms < 0) return null;
  const total = Math.round(ms / 1000);
  const m = Math.floor(total / 60);
  const sec = total % 60;
  return m > 0 ? `${m}m ${sec}s` : `${sec}s`;
}

function repoPath(url: string | null) {
  if (!url) return null;
  return url
    .replace(/^https?:\/\/(www\.)?github\.com\//i, "")
    .replace(/\.git$/i, "");
}

/** Hairline-divided definition strip cell (2 columns on mobile, 4 on lg). */
const STRIP_CELL = [
  "",
  "border-l border-border",
  "border-t border-border lg:border-t-0 lg:border-l",
  "border-l border-t border-border lg:border-t-0",
];

const MAX_LOG_LINES = 10000;

// Real-time container logs panel
function ContainerLogsPanel({
  projectId,
  services,
  activeTab,
  preferredContainerName,
}: {
  projectId: string;
  services: Service[];
  activeTab: string;
  preferredContainerName?: string | null;
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

  // Follow the project service picker when present; else first container
  useEffect(() => {
    if (preferredContainerName) {
      setActiveContainer(preferredContainerName);
      return;
    }
    if (services.length > 0 && !activeContainer) {
      const firstWithContainer = services.find((s) => s.container_name);
      if (firstWithContainer) {
        setActiveContainer(firstWithContainer.container_name!);
      }
    }
  }, [services, activeContainer, preferredContainerName]);

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
      <div className="rounded-lg border border-border bg-card py-14 text-center">
        <p className="text-[15px] font-semibold">No containers found</p>
        <p className="mx-auto mt-1 max-w-sm text-[13.5px] text-muted-foreground">
          Deploy the project first to view container logs.
        </p>
      </div>
    );
  }

  return (
    <div className="space-y-3">
      {/* Container selector */}
      {containersWithNames.length > 1 && (
        <div className="scrollbar-none inline-flex max-w-full gap-0.5 overflow-x-auto rounded-md border border-input bg-card p-0.5">
          {containersWithNames.map((svc) => (
            <button
              key={svc.id}
              onClick={() => setActiveContainer(svc.container_name!)}
              className={cn(
                "inline-flex h-7 shrink-0 items-center gap-2 rounded-[4px] px-2.5 text-[13px] font-medium transition-colors duration-150 pointer-coarse:h-9",
                activeContainer === svc.container_name
                  ? "bg-secondary text-foreground"
                  : "text-muted-foreground hover:text-foreground",
              )}
            >
              <span
                className={cn(
                  "size-2 shrink-0 rounded-full",
                  connected[svc.container_name!] ? "bg-ok" : "bg-subtle",
                )}
              />
              {svc.name}
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
            key={containerName}
            logs={logLines}
            connected={!!connected[containerName]}
            title={svc.name}
            subtitle={containerName}
            onClear={() => clearLogs(containerName)}
            downloadFilename={`${containerName}-logs.txt`}
            height="h-[min(70vh,720px)]"
          />
        );
      })}
    </div>
  );
}


export default function ProjectDetail() {
  const params = useParams();
  const navigate = useNavigate();
  const [searchParams, setSearchParams] = useSearchParams();
  const projectId = params.id as string;

  // Last known data for this project draws at once on a revisit; the first poll refreshes it.
  const [initial] = useState(() => readProjectCache(projectId));
  const [project, setProject] = useState<Project | null>(initial.project);
  useBreadcrumbLeaf(project?.name);
  const [files, setFiles] = useState<ProjectFile[]>(initial.files ?? []);
  const [deployments, setDeployments] = useState<Deployment[]>(initial.deployments);
  const [services, setServices] = useState<Service[]>(initial.services ?? []);
  const [selectedServiceId, setSelectedServiceId] = useState<string | null>(
    null,
  );
  const [logs, setLogs] = useState("");
  const [loading, setLoading] = useState(initial.project === null);
  const [actionLoading, setActionLoading] = useState(false);
  const [currentAction, setCurrentAction] = useState<string | null>(null);
  const [editingFile, setEditingFile] = useState<{
    name: string;
    content: string;
  } | null>(null);
  const tabFromUrl = searchParams.get("tab");
  const [activeTab, setActiveTab] = useState(
    tabFromUrl && tabFromUrl.length > 0 ? tabFromUrl : "overview",
  );

  // Honor ?tab= from create/redeploy links (e.g. databases → deployments for pull logs)
  useEffect(() => {
    const tab = searchParams.get("tab");
    if (tab && tab !== activeTab) {
      setActiveTab(tab);
    }
    // Only react to URL tab changes — not every activeTab toggle
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [searchParams]);
  /** Source tab file browser — open by default; user can collapse via chevron */
  const [sourceFilesOpen, setSourceFilesOpen] = useState(true);
  const [serverIP, setServerIP] = useState<string>(() => {
    const cached = apiCache.peek<{ ip?: string }>(API.systemIP);
    return cached ? cached.ip || "N/A" : "...";
  });
  const [buildType, setBuildType] = useState<BuildType>("auto");
  const [baseDirectory, setBaseDirectory] = useState(".");
  const [dockerfilePath, setDockerfilePath] = useState("Dockerfile");
  const [internalPort, setInternalPort] = useState(3000);
  const [publishHostPort, setPublishHostPort] = useState(false);
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

  const multiService = services.length > 1;
  const serviceFromUrl = searchParams.get("service");
  // Multi-service: URL alone owns the service workspace. Never fall back to stale id.
  const selectedService = multiService
    ? services.find((s) => s.name === serviceFromUrl) || null
    : services[0] || null;

  const workspace: ProjectWorkspace =
    multiService && selectedService ? "service" : "project";
  const isDatabase = project?.project_type === "database";
  const showProjectTabs = !multiService || workspace === "project";
  const showServiceTabs = !multiService || workspace === "service";

  const PROJECT_ONLY_TABS = new Set([
    "deployments",
    "build",
    "source",
  ]);
  const SERVICE_ONLY_TABS = new Set(["domains", "storage", "logs"]);

  // Managed DB only has overview / deployments / logs
  useEffect(() => {
    if (!isDatabase) return;
    if (
      activeTab !== "overview" &&
      activeTab !== "deployments" &&
      activeTab !== "logs"
    ) {
      setActiveTab("overview");
    }
  }, [isDatabase, activeTab]);

  // Sync selection + keep URL clean; do not force a service onto multi projects
  useEffect(() => {
    if (services.length === 0) {
      if (selectedServiceId !== null) setSelectedServiceId(null);
      return;
    }
    if (!multiService) {
      if (services[0] && services[0].id !== selectedServiceId) {
        setSelectedServiceId(services[0].id);
      }
      if (serviceFromUrl) {
        const nextParams = new URLSearchParams(searchParams);
        nextParams.delete("service");
        setSearchParams(nextParams, { replace: true });
      }
      return;
    }
    if (serviceFromUrl) {
      const match = services.find((s) => s.name === serviceFromUrl);
      if (match) {
        if (match.id !== selectedServiceId) setSelectedServiceId(match.id);
      } else {
        setSelectedServiceId(null);
        const nextParams = new URLSearchParams(searchParams);
        nextParams.delete("service");
        setSearchParams(nextParams, { replace: true });
      }
    } else if (selectedServiceId !== null) {
      setSelectedServiceId(null);
    }
  }, [
    services,
    multiService,
    serviceFromUrl,
    selectedServiceId,
    searchParams,
    setSearchParams,
  ]);

  useEffect(() => {
    if (selectedService?.name) {
      setStorageService(selectedService.name);
    }
  }, [selectedService?.name]);

  // When workspace changes, land on a valid tab for that workspace
  useEffect(() => {
    if (!multiService) return;
    if (workspace === "project" && SERVICE_ONLY_TABS.has(activeTab)) {
      setActiveTab("overview");
    }
    if (workspace === "service" && PROJECT_ONLY_TABS.has(activeTab)) {
      setActiveTab("overview");
    }
  }, [workspace, multiService, activeTab]);

  const selectProjectWorkspace = () => {
    setSelectedServiceId(null);
    const nextParams = new URLSearchParams(searchParams);
    nextParams.delete("service");
    setSearchParams(nextParams, { replace: true });
    if (SERVICE_ONLY_TABS.has(activeTab)) setActiveTab("overview");
  };

  const selectService = (serviceId: string, tab = "overview") => {
    const svc = services.find((s) => s.id === serviceId);
    if (!svc) return;
    setSelectedServiceId(svc.id);
    const nextParams = new URLSearchParams(searchParams);
    nextParams.set("service", svc.name);
    setSearchParams(nextParams, { replace: true });
    setActiveTab(tab);
  };

  const goToProjectTab = useCallback(
    (tab: string) => {
      setSelectedServiceId(null);
      const nextParams = new URLSearchParams(searchParams);
      nextParams.delete("service");
      setSearchParams(nextParams, { replace: true });
      setActiveTab(tab);
    },
    [searchParams, setSearchParams],
  );

  // Confirmation Dialog State
  const [isConfirmOpen, setIsConfirmOpen] = useState(false);
  const [pendingAction, setPendingAction] = useState<string | null>(null);

  // Restore previous (rollback) — step-up password
  const [rollbackTarget, setRollbackTarget] = useState<Deployment | null>(null);
  const [rollbackPassword, setRollbackPassword] = useState("");
  const [rollbackLoading, setRollbackLoading] = useState(false);
  /** Global latest success id (not page-local) — Restore must never offer this row. */
  const [latestSuccessId, setLatestSuccessId] = useState<string | null>(initial.latestSuccessId);

  // Auto-deploy state
  const [autoDeploy, setAutoDeploy] = useState(initial.autoDeploy);
  const [autoDeployLoading, setAutoDeployLoading] = useState(false);

  // Deployment history pagination (6 per page)
  const [historyPage, setHistoryPage] = useState(0);
  const [deploymentTotal, setDeploymentTotal] = useState(initial.deploymentTotal);
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
        const res = await apiCache.fetch(API.systemIP);
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

      const [projectRes, filesRes, deploymentsRes, servicesRes, latestRes, latestSuccessRes] =
        await Promise.all([
          apiCache.fetch(projectAPI.project(projectId)),
          apiCache.fetch(projectAPI.files(projectId)),
          apiCache.fetch(
            projectAPI.history(projectId, HISTORY_PAGE_SIZE, historyOffset),
            { signal: deploymentsAbort.signal },
          ),
          apiCache.fetch(projectAPI.services(projectId)),
          // Always resolve the newest deployment for live log tracking when not on page 1
          historyPage > 0
            ? apiCache.fetch(
                `${API_URL}/api/deployments/${projectId}?limit=1&offset=0`,
              )
            : Promise.resolve(null),
          // Latest success for Restore eligibility on every history page
          apiCache.fetch(projectAPI.latestSuccess(projectId)),
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

      if (latestSuccessRes.ok) {
        const successPayload = await latestSuccessRes.json();
        const successList: Deployment[] = Array.isArray(successPayload)
          ? successPayload
          : Array.isArray(successPayload?.items)
            ? successPayload.items
            : [];
        setLatestSuccessId(successList[0]?.id ?? null);
      }

      let latestList = deps;
      if (latestRes) {
        const latestPayload = await latestRes.json();
        latestList = Array.isArray(latestPayload)
          ? latestPayload
          : Array.isArray(latestPayload?.items)
            ? latestPayload.items
            : [];
      }

      // Keep logs fresh for the active / currently viewed deploy (auto-select
      // of latest when none selected is handled in a separate effect below).
      if (latestList.length > 0 && !actionLoading && viewingDeploymentId) {
        const latestDeployment = latestList[0];
        const isBuilding =
          projectData.status === "building" || projectData.status === "pending";
        const isLatestInProgress = latestDeployment.status === "in_progress";

        const viewedDeployment =
          viewingDeploymentId === latestDeployment.id
            ? latestDeployment
            : deps.find((d: Deployment) => d.id === viewingDeploymentId) ||
              null;

        const shouldUpdateLogs =
          (isLatestInProgress && viewingDeploymentId !== latestDeployment.id) ||
          (isBuilding && viewingDeploymentId === latestDeployment.id) ||
          (isLatestInProgress &&
            viewingDeploymentId === latestDeployment.id) ||
          (viewedDeployment != null &&
            viewedDeployment.id === latestDeployment.id);

        if (shouldUpdateLogs) {
          const source =
            viewingDeploymentId === latestDeployment.id
              ? latestDeployment
              : viewedDeployment || latestDeployment;
          setLogs(
            source.logs ||
              (source.status === "in_progress"
                ? "🚀 Starting deployment...\n"
                : "No logs available for this deployment"),
          );
          if (isLatestInProgress) {
            setViewingDeploymentId(latestDeployment.id);
          }

          if (
            isLatestInProgress &&
            viewingDeploymentId !== latestDeployment.id &&
            historyPage !== 0
          ) {
            setHistoryPage(0);
          }

          if (
            isLatestInProgress &&
            activeTab !== "deployments" &&
            !serviceFromUrl
          ) {
            goToProjectTab("deployments");
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
    actionLoading,
    historyPage,
    viewingDeploymentId,
    activeTab,
    goToProjectTab,
    serviceFromUrl,
  ]);

  useEffect(() => {
    historyFetchGen.current += 1;
    // Start from this project's cached history (or empty); a cached project replaces the one
    // shown when the route switches projects without remounting.
    const cached = readProjectCache(projectId);
    setHistoryPage(0);
    setDeploymentTotal(cached.deploymentTotal);
    setDeployments(cached.deployments);
    setViewingDeploymentId(null);
    setLogs("");
    if (cached.project) {
      setProject(cached.project);
      if (cached.files) setFiles(cached.files);
      if (cached.services) setServices(cached.services);
      setLatestSuccessId(cached.latestSuccessId);
      setAutoDeploy(cached.autoDeploy);
    }
  }, [projectId]);

  // Apps + databases: when history arrives and nothing is selected, show latest logs
  useEffect(() => {
    if (actionLoading) return;
    if (viewingDeploymentId != null) return;
    if (deployments.length === 0) return;
    const latest = deployments[0];
    setViewingDeploymentId(latest.id);
    setLogs(
      latest.logs ||
        (latest.status === "in_progress"
          ? "🚀 Starting deployment...\n"
          : "No logs available for this deployment"),
    );
  }, [deployments, viewingDeploymentId, actionLoading]);

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
    setPublishHostPort(Boolean(project.publish_host_port));
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
          publish_host_port: publishHostPort,
        }),
      });
      const data = await res.json().catch(() => ({}));
      if (!res.ok) throw new Error(data.error || "Failed to save build settings");
      setProject((current) => current ? { ...current, ...data } : current);
      toast.success("Build settings saved — Deploy to apply networking changes");
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
        const res = await apiCache.fetch(projectAPI.autoDeploy(projectId));
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
          enabled ? "Auto-deploy enabled" : "Auto-deploy disabled",
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

  const getActionDetails = (): {
    title: string;
    description: string;
    confirmText: string;
    confirmVariant: "default" | "destructive";
  } => {
    switch (pendingAction) {
      case "delete":
        return {
          title: "Delete project",
          description:
            "This permanently deletes the project and removes all associated data, containers, and configuration. This cannot be undone.",
          confirmText: "Delete project",
          confirmVariant: "destructive",
        };
      case "stop":
        return {
          title: "Stop project",
          description:
            multiService
              ? "Stops every service in this project. Public endpoints will return errors until you deploy or restart again."
              : "Stop the running containers? The app will be unreachable until you deploy or restart again.",
          confirmText: "Stop project",
          confirmVariant: "default",
        };
      case "restart":
        return {
          title: "Restart project",
          description:
            multiService
              ? "Restarts every service in this project without rebuilding images. Expect brief downtime across all apps."
              : "Restart without rebuilding? Expect a brief period of downtime.",
          confirmText: "Restart project",
          confirmVariant: "default",
        };
      case "redeploy":
        return {
          title: "Redeploy project",
          description:
            multiService
              ? "Rebuilds from source and redeploys the whole project — every service, not only the one open in Workspace."
              : "Rebuild from source and deploy a new version. This may take a few minutes.",
          confirmText: "Redeploy",
          confirmVariant: "default",
        };
      case "cancel":
        return {
          title: "Cancel build",
          description:
            "Abort the current build? Partial artifacts may be left behind.",
          confirmText: "Cancel build",
          confirmVariant: "destructive",
        };
      case "deploy":
        return {
          title: "Deploy project",
          description:
            "Starts a new build and deployment for this application.",
          confirmText: "Deploy",
          confirmVariant: "default",
        };
      default:
        return {
          title: "Confirm action",
          description: "Are you sure you want to proceed?",
          confirmText: "Confirm",
          confirmVariant: "default",
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
      goToProjectTab("deployments");
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
          action === "deploy" || action === "redeploy"
            ? { "Content-Type": "application/json" }
            : undefined,
        body:
          action === "deploy" || action === "redeploy"
            ? JSON.stringify({
                trigger: action === "redeploy" ? "redeploy" : "manual",
              })
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
        `${action.charAt(0).toUpperCase() + action.slice(1)} completed`,
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

  const canRestoreDeployment = (d: Deployment) => {
    if (d.status !== "success") return false;
    if (
      !d.image_tags ||
      typeof d.image_tags !== "object" ||
      Object.keys(d.image_tags).length === 0
    ) {
      return false;
    }
    // Fail closed if latest-success fetch has not resolved yet
    if (!latestSuccessId) return false;
    return d.id !== latestSuccessId;
  };

  const handleRollback = async () => {
    if (!rollbackTarget || !projectId) return;
    if (!rollbackPassword.trim()) {
      toast.error("Enter your account password to confirm restore");
      return;
    }

    const target = rollbackTarget;
    setRollbackLoading(true);
    setActionLoading(true);
    setCurrentAction("rollback");
    setLogs(`$ Restoring deployment ${target.id.split("-")[0]}...\n`);
    goToProjectTab("deployments");

    try {
      const res = await authFetch(
        `${API_URL}/api/deployments/${projectId}/rollback`,
        {
          method: "POST",
          headers: { "Content-Type": "application/json" },
          body: JSON.stringify({
            deploymentId: target.id,
            password: rollbackPassword,
          }),
        },
      );

      if (!res.ok) {
        const errorData = await res.json().catch(() => null);
        const errMsg =
          errorData?.error || `Server returned ${res.status} ${res.statusText}`;
        setLogs((prev) => prev + `\n[ERROR] ${errMsg}\n`);
        toast.error(errMsg);
        return;
      }

      setRollbackTarget(null);
      setRollbackPassword("");

      const streamResult = await consumeProgressStream(res, (line) => {
        setLogs((prev) => prev + line + "\n");
      });

      setTimeout(fetchProject, 1000);
      await new Promise((resolve) => setTimeout(resolve, 1500));
      await fetchProject();

      const restored = streamResult.lines.some((l) =>
        /ROLLBACK SUCCESSFUL/i.test(l),
      );
      if (!streamResult.ok || !restored) {
        toast.error(
          streamResult.error ||
            "Restore failed — check deployment logs for details",
        );
        return;
      }

      toast.success("Previous deployment restored");
    } catch (error) {
      console.error(error);
      setLogs((prev) => prev + `\n❌ Connection error: ${error}\n`);
      toast.error("Connection error during restore");
    } finally {
      setRollbackLoading(false);
      setActionLoading(false);
      setCurrentAction(null);
    }
  };

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
      <div aria-hidden="true">
        <div className="h-6 w-48 rounded-md bg-secondary" />
        <div className="mt-2 h-4 w-72 rounded-md bg-secondary/70" />
        <div className="mt-8 h-10 border-b border-border" />
        <div className="mt-5 h-64 rounded-lg border border-border bg-card" />
      </div>
    );
  }

  if (!project) return null;

  const isDb = project.project_type === "database";
  const latestDeployment = historyPage === 0 ? deployments[0] : undefined;
  const latestSha = latestDeployment?.commit_sha?.slice(0, 7) || null;
  const manifestParts: string[] = [
    isDb ? project.db_engine || "database" : project.project_type,
    project.source_type === "github"
      ? repoPath(project.github_url) || "github"
      : project.source_type,
    ...(project.source_type === "github" && project.github_branch
      ? [project.github_branch]
      : []),
    ...(latestSha ? [latestSha] : []),
    `created ${new Date(project.created_at).toLocaleString([], {
      month: "short",
      day: "numeric",
      year: "numeric",
      hour: "2-digit",
      minute: "2-digit",
    })}`,
    project.id.split("-")[0],
  ];
  const scopedService =
    workspace === "service" && selectedService ? selectedService : null;
  const viewedDeployment = viewingDeploymentId
    ? deployments.find((d) => d.id === viewingDeploymentId) || null
    : null;

  return (
    <>
      <div>
        {/* Mobile only: the top bar shows just the current item, so keep a way back. */}
        <Link
          to={isDb ? "/databases" : "/"}
          className="-ml-1 mb-3 inline-flex h-8 items-center gap-1 rounded-md px-1 text-[13px] text-muted-foreground transition-colors hover:text-foreground sm:hidden"
        >
          <ChevronLeft className="size-4" />
          {isDb ? "Databases" : "Projects"}
        </Link>

        <header className="flex min-w-0 flex-col gap-4 md:flex-row md:items-start md:justify-between md:gap-6">
          <div className="min-w-0 flex-1">
            <div className="flex min-w-0 flex-wrap items-center gap-x-3 gap-y-1">
              <h1 className="max-w-full truncate text-xl font-semibold leading-7 tracking-[-0.01em]">
                {project.name}
              </h1>
              <StatusBadge status={project.status} />
            </div>
            <p className="mt-1 flex min-w-0 flex-wrap items-center gap-x-1.5 font-mono text-xs leading-5 text-muted-foreground">
              {manifestParts.map((part, i) => (
                <span key={i} className="inline-flex min-w-0 items-center gap-x-1.5">
                  <span className="min-w-0 truncate">{part}</span>
                  {i < manifestParts.length - 1 && (
                    <span aria-hidden="true" className="text-subtle">
                      ·
                    </span>
                  )}
                </span>
              ))}
            </p>
          </div>

          {/* Project-wide lifecycle actions — always header top-right (whole stack). */}
          <ProjectActionBar
            status={project.status}
            actionLoading={actionLoading}
            currentAction={currentAction}
            onAction={confirmAction}
            placement="header"
            multiService={multiService}
            className="shrink-0"
            title={
              multiService
                ? "Affects every service in this project"
                : undefined
            }
          />
        </header>

        <Tabs value={activeTab} onValueChange={setActiveTab} className="mt-6">
          {multiService && (
            <div className="mb-4 min-w-0">
              <ServiceSwitcher
                services={services}
                workspace={workspace}
                selectedId={selectedService?.id ?? null}
                onSelectProject={selectProjectWorkspace}
                onSelectService={(id) => selectService(id, "overview")}
              />
            </div>
          )}

          <div className="sticky top-12 z-10 min-w-0 bg-background">
            <TabsList>
              <TabsTrigger value="overview">
                {workspace === "service" && multiService
                  ? selectedService?.name || "Service"
                  : "Overview"}
              </TabsTrigger>

              {(!multiService || workspace === "project") && (
                <TabsTrigger value="deployments">Deployments</TabsTrigger>
              )}

              {!isDatabase && (
                <>
                  <TabsTrigger value="env">Environment</TabsTrigger>

                  {(!multiService || workspace === "project") && (
                    <TabsTrigger value="build">Build</TabsTrigger>
                  )}

                  {(!multiService || workspace === "service") && (
                    <TabsTrigger value="storage">Storage</TabsTrigger>
                  )}

                  {(!multiService || workspace === "project") && (
                    <TabsTrigger value="source">Source</TabsTrigger>
                  )}

                  {(!multiService || workspace === "service") && (
                    <TabsTrigger value="domains">Domains</TabsTrigger>
                  )}
                </>
              )}

              {(!multiService || workspace === "service") && (
                <TabsTrigger value="logs">Logs</TabsTrigger>
              )}
            </TabsList>
          </div>

          <TabsContent value="overview" className="space-y-5">
            {/* Summary strip */}
            <section className="grid grid-cols-2 rounded-lg border border-border bg-card lg:grid-cols-4">
              {(
                [
                  {
                    label: "Status",
                    value: (
                      <span className="capitalize">{project.status}</span>
                    ),
                    note: "Main service status",
                  },
                  {
                    label: "Deployments",
                    value: (
                      <span className="font-mono tabular-nums">
                        {deploymentTotal}
                      </span>
                    ),
                    note: "Total deployments",
                  },
                  {
                    label: "Source",
                    value:
                      project.source_type === "github" ? "GitHub" : "Upload",
                    note: "Source type",
                  },
                  {
                    label: "Network",
                    value: "Shared host",
                    note: "Same Docker host & bridge (not multi-tenant isolation)",
                  },
                ] as const
              ).map((cell, i) => (
                <div key={cell.label} className={cn("min-w-0 px-4 py-3", STRIP_CELL[i])}>
                  <p className="text-xs text-muted-foreground">{cell.label}</p>
                  <p className="mt-1 truncate text-[15px] font-semibold leading-6">
                    {cell.value}
                  </p>
                  <p className="mt-0.5 text-xs leading-4 text-muted-foreground">
                    {cell.note}
                  </p>
                </div>
              ))}
            </section>

            {/* Services & endpoints */}
            {services.length > 0 && (
              <section className="rounded-lg border border-border bg-card">
                <div className="flex items-start justify-between gap-3 border-b border-border px-4 py-3">
                  <div className="min-w-0">
                    <h2 className="truncate text-[15px] font-semibold leading-5">
                      {scopedService ? "Endpoints" : "Services"}
                    </h2>
                    <p className="mt-0.5 text-[13px] text-muted-foreground">
                      {scopedService
                        ? `${scopedService.dockerfile_path} · runtime endpoints for this service`
                        : multiService
                          ? "Open a service for env, domains, storage, and logs."
                          : "Live endpoints for this project."}
                    </p>
                  </div>
                  <span className="shrink-0 pt-0.5 text-xs text-muted-foreground">
                    {workspace === "service" ? (
                      "Service"
                    ) : multiService ? (
                      <>
                        <span className="font-mono">{services.length}</span>{" "}
                        services
                      </>
                    ) : (
                      "Live"
                    )}
                  </span>
                </div>
                <div className="divide-y divide-border">
                  {(scopedService ? [scopedService] : services).map((svc) => {
                    const domains = svc.domain
                      ? svc.domain
                          .split(",")
                          .map((d) => d.trim())
                          .filter(Boolean)
                      : [];

                    // Host publish is opt-in — only link when a real host port exists
                    const isLocal =
                      typeof window !== "undefined" &&
                      (window.location.hostname === "localhost" ||
                        window.location.hostname === "127.0.0.1");
                    const portHost = isLocal ? "localhost" : serverIP;
                    const hasHostPort =
                      typeof svc.port === "number" &&
                      Number.isFinite(svc.port) &&
                      svc.port > 0;
                    const hostPortReady =
                      hasHostPort &&
                      Boolean(portHost) &&
                      portHost !== "..." &&
                      portHost !== "N/A";

                    const domainLinks = domains.map((d) => ({
                      url: `https://${d}`,
                      label: d,
                    }));
                    const notPublicYet =
                      !hasHostPort && domainLinks.length === 0;
                    // Persisted flag only — never the unsaved Build checkbox
                    const awaitingHostPortPublish =
                      notPublicYet &&
                      Boolean(project?.publish_host_port);
                    const opensWorkspace =
                      multiService && workspace === "project";

                    return (
                      <div
                        key={svc.id}
                        className={cn(
                          "px-4 py-3 transition-colors duration-150",
                          opensWorkspace && "hover:bg-secondary/50",
                        )}
                      >
                        <div className="flex flex-col gap-2 sm:flex-row sm:items-center sm:justify-between sm:gap-6">
                          {/* Left: open service workspace (avoid nesting controls in a button) */}
                          <div
                            className={cn(
                              "min-w-0 flex-1",
                              opensWorkspace &&
                                "-m-1 cursor-pointer rounded-md p-1 focus-visible:outline-2 focus-visible:outline-ring",
                            )}
                            role={opensWorkspace ? "button" : undefined}
                            tabIndex={opensWorkspace ? 0 : undefined}
                            onClick={() => {
                              if (multiService && workspace === "project") {
                                selectService(svc.id);
                              }
                            }}
                            onKeyDown={(e) => {
                              if (!(multiService && workspace === "project")) {
                                return;
                              }
                              if (e.key === "Enter" || e.key === " ") {
                                e.preventDefault();
                                selectService(svc.id);
                              }
                            }}
                          >
                            <div className="flex min-w-0 flex-wrap items-center gap-x-3 gap-y-0.5">
                              <span className="truncate text-sm font-medium">
                                {svc.name}
                              </span>
                              <StatusBadge status={svc.status} />
                            </div>
                            <p className="mt-0.5 truncate font-mono text-xs text-muted-foreground">
                              port {svc.internal_port}
                              {svc.dockerfile_path && (
                                <>
                                  <span className="text-subtle"> · </span>
                                  {svc.dockerfile_path}
                                </>
                              )}
                            </p>
                          </div>

                          {/* Public endpoints when available */}
                          {(hostPortReady ||
                            (hasHostPort && !hostPortReady) ||
                            domainLinks.length > 0) && (
                            <div className="flex min-w-0 flex-wrap items-center gap-x-4 gap-y-1 sm:justify-end">
                              {hostPortReady && (
                                <a
                                  href={`http://${portHost}:${svc.port}`}
                                  target="_blank"
                                  rel="noopener noreferrer"
                                  onClick={(e) => e.stopPropagation()}
                                  className="inline-flex min-w-0 items-center gap-1 font-mono text-[13px] text-foreground underline-offset-4 hover:underline"
                                >
                                  <span className="truncate">
                                    {portHost}:{svc.port}
                                  </span>
                                  <ExternalLink className="size-3.5 shrink-0 text-subtle" />
                                </a>
                              )}
                              {hasHostPort && !hostPortReady && (
                                <span className="rounded-sm border border-border px-1.5 py-0.5 font-mono text-xs text-muted-foreground">
                                  Host port {svc.port}
                                </span>
                              )}
                              {domainLinks.map((link, idx) => (
                                <a
                                  key={idx}
                                  href={link.url}
                                  target="_blank"
                                  rel="noopener noreferrer"
                                  onClick={(e) => e.stopPropagation()}
                                  className="inline-flex min-w-0 items-center gap-1 font-mono text-[13px] text-foreground underline-offset-4 hover:underline"
                                >
                                  <span className="truncate">{link.label}</span>
                                  <ExternalLink className="size-3.5 shrink-0 text-subtle" />
                                </a>
                              ))}
                            </div>
                          )}
                        </div>

                        {notPublicYet && (
                          <div className="mt-3 rounded-md border border-info/40 bg-secondary/60 px-3 py-2.5 text-[13px]">
                            <p className="font-medium text-foreground">
                              Private by default
                            </p>
                            <p className="mt-1 max-w-2xl leading-5 text-muted-foreground">
                              {awaitingHostPortPublish ? (
                                <>
                                  Publish host ports is on — redeploy to open
                                  an <span className="font-mono text-foreground">IP:port</span>,
                                  or add a domain for HTTPS on{" "}
                                  <span className="font-mono text-foreground">:80/:443</span>.
                                  Prefer a domain: raw host ports advertise
                                  your server’s real IP and are easier to
                                  scan.
                                </>
                              ) : (
                                <>
                                  This service is not on the public internet
                                  yet. Add a domain for HTTPS — that is the
                                  safe path. Avoid sharing{" "}
                                  <span className="font-mono text-foreground">IP:port</span>:
                                  it reveals your origin server address and
                                  can expose apps on this host to anyone who
                                  finds the port.
                                </>
                              )}
                            </p>
                            <div className="mt-2.5 flex flex-wrap gap-2">
                              <Button
                                type="button"
                                variant="outline"
                                size="sm"
                                onClick={(e) => {
                                  e.stopPropagation();
                                  if (multiService) {
                                    selectService(svc.id, "domains");
                                  } else {
                                    setActiveTab("domains");
                                  }
                                }}
                              >
                                Add domain
                              </Button>
                              <Button
                                type="button"
                                variant="ghost"
                                size="sm"
                                className="text-muted-foreground"
                                onClick={(e) => {
                                  e.stopPropagation();
                                  if (awaitingHostPortPublish) {
                                    confirmAction("redeploy");
                                  } else {
                                    goToProjectTab("build");
                                  }
                                }}
                              >
                                {awaitingHostPortPublish
                                  ? "Redeploy"
                                  : "Build settings"}
                              </Button>
                            </div>
                          </div>
                        )}
                      </div>
                    );
                  })}
                </div>
              </section>
            )}

            {/* Source configuration (GitHub) */}
            {project.source_type === "github" && (
              <section className="rounded-lg border border-border bg-card">
                <div className="flex items-center justify-between gap-3 border-b border-border px-4 py-3">
                  <h2 className="text-[15px] font-semibold leading-5">Source</h2>
                  <a
                    href={project.github_url || "#"}
                    target="_blank"
                    rel="noopener noreferrer"
                    className="inline-flex items-center gap-1 text-[13px] text-brand underline-offset-4 hover:underline"
                  >
                    View repository
                    <ExternalLink className="size-3.5" />
                  </a>
                </div>
                <dl className="divide-y divide-border text-[13px]">
                  <div className="grid gap-1 px-4 py-3 sm:grid-cols-[160px_minmax(0,1fr)] sm:gap-4">
                    <dt className="text-muted-foreground">Repository URL</dt>
                    <dd className="min-w-0 truncate font-mono text-foreground">
                      {project.github_url}
                    </dd>
                  </div>
                  <div className="grid gap-1 px-4 py-3 sm:grid-cols-[160px_minmax(0,1fr)] sm:gap-4">
                    <dt className="text-muted-foreground">Branch</dt>
                    <dd className="min-w-0 truncate font-mono text-foreground">
                      {project.github_branch}
                    </dd>
                  </div>
                </dl>

                {/* Auto-deploy */}
                <div className="flex items-center justify-between gap-4 border-t border-border px-4 py-3">
                  <div className="min-w-0">
                    <p className="text-sm font-medium">Auto-deploy</p>
                    <p className="mt-0.5 text-[13px] text-muted-foreground">
                      {autoDeploy
                        ? "Pushes to this branch will trigger automatic deployment."
                        : "Enable to redeploy automatically when commits are pushed."}
                    </p>
                  </div>
                  <button
                    onClick={() => handleAutoDeployToggle(!autoDeploy)}
                    disabled={autoDeployLoading}
                    className={cn(
                      "relative h-5 w-9 shrink-0 rounded-full transition-colors duration-150 focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-ring disabled:opacity-50",
                      autoDeploy ? "bg-brand" : "bg-input",
                    )}
                  >
                    <span
                      className={cn(
                        "absolute left-0.5 top-0.5 size-4 rounded-full bg-white transition-transform duration-150",
                        autoDeploy ? "translate-x-4" : "translate-x-0",
                      )}
                    />
                  </button>
                </div>
              </section>
            )}

            {project.project_type === "database" && project.db_engine ? (
              <ManagedDatabasePanel
                databaseId={project.id}
                engineLabel={
                  (
                    {
                      postgres: "PostgreSQL",
                      mysql: "MySQL",
                      mariadb: "MariaDB",
                      redis: "Redis",
                      mongodb: "MongoDB",
                    } as Record<string, string>
                  )[project.db_engine] || project.db_engine
                }
              />
            ) : project.project_type !== "database" ? (
              <AttachDatabasePanel
                appProjectId={project.id}
                services={services.map((s) => ({ id: s.id, name: s.name }))}
                lockedServiceName={scopedService ? scopedService.name : null}
              />
            ) : null}
          </TabsContent>

          {showProjectTabs && (
          <TabsContent value="deployments">
            <div className="grid grid-cols-1 gap-5 xl:grid-cols-3">
              <section className="min-w-0 xl:col-span-2">
                <div className="mb-3 flex min-h-7 flex-wrap items-center justify-between gap-2">
                  <h2 className="flex min-w-0 items-baseline gap-2 text-[15px] font-semibold leading-5">
                    Deployment log
                    {viewedDeployment && (
                      <span className="truncate font-mono text-xs font-normal text-muted-foreground">
                        {viewedDeployment.commit_sha
                          ? viewedDeployment.commit_sha.slice(0, 7)
                          : `#${viewedDeployment.id.split("-")[0]}`}
                        {" · "}
                        {deploymentTriggerLabel(viewedDeployment.trigger)}
                      </span>
                    )}
                  </h2>
                  {project.status === "building" && (
                    <span className="inline-flex items-center gap-2 text-[13px] text-muted-foreground">
                      <Loader2 className="size-3.5 animate-spin text-warn" />
                      Building…
                    </span>
                  )}
                </div>
                <Terminal
                  logs={logs}
                  isBuilding={project?.status === "building"}
                  className="h-[min(55vh,28rem)] sm:h-[540px]"
                />
              </section>

              <section className="min-w-0">
                <div className="mb-3 flex min-h-7 flex-wrap items-center justify-between gap-2">
                  <h2 className="text-[15px] font-semibold leading-5">
                    History
                  </h2>
                  {deploymentTotal > 0 && (
                    <span className="font-mono text-xs tabular-nums text-muted-foreground">
                      {historyFrom}–{historyTo} of {deploymentTotal}
                    </span>
                  )}
                </div>
                <div
                  className={cn(
                    "overflow-hidden rounded-lg border border-border bg-card transition-opacity duration-150",
                    historyLoading && "opacity-60",
                  )}
                >
                  {deployments.length === 0 ? (
                    <div className="px-4 py-14 text-center">
                      <p className="text-[15px] font-semibold">
                        No deployments yet
                      </p>
                      <p className="mx-auto mt-1 max-w-sm text-[13.5px] text-muted-foreground">
                        Deploy the project to build it and see its history here.
                      </p>
                    </div>
                  ) : (
                    <>
                    <p className="border-b border-border px-4 py-2 text-xs text-muted-foreground">
                      Select a deployment to show its log.
                    </p>
                    <ul className="divide-y divide-border">
                    {deployments.map((deployment) => {
                      const isViewing = viewingDeploymentId === deployment.id;
                      const meta =
                        DEPLOY_STATUS[deployment.status] || DEPLOY_STATUS.pending;
                      const isLive = deployment.id === latestSuccessId;
                      const duration = formatDuration(
                        deployment.created_at,
                        deployment.finished_at,
                      );
                      const started = new Date(deployment.created_at);
                      return (
                        <li
                          key={deployment.id}
                          className={cn(
                            "relative grid min-w-0 cursor-pointer grid-cols-[auto_minmax(0,1fr)_auto] gap-x-3 px-4 py-3 transition-colors duration-150",
                            isViewing
                              ? "bg-secondary/70"
                              : "hover:bg-secondary/50",
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
                          <span
                            aria-hidden="true"
                            className={cn(
                              "mt-1.5 size-2 shrink-0 rounded-full",
                              meta.dot,
                            )}
                          />
                          <div className="min-w-0">
                            <div className="flex min-w-0 items-center gap-2">
                              <span className="truncate text-sm font-medium">
                                {deploymentTriggerLabel(deployment.trigger)}
                              </span>
                              {isLive && (
                                <span className="shrink-0 rounded-sm border border-border px-1.5 text-xs leading-5 text-muted-foreground">
                                  Live
                                </span>
                              )}
                            </div>
                            <p className="mt-0.5 truncate text-xs text-muted-foreground">
                              <span className="font-mono">
                                {deployment.commit_sha
                                  ? deployment.commit_sha.slice(0, 7)
                                  : `#${deployment.id.split("-")[0]}`}
                              </span>
                              <span className="text-subtle"> · </span>
                              {meta.label}
                              {duration && (
                                <>
                                  <span className="text-subtle"> · </span>
                                  <span className="font-mono">{duration}</span>
                                </>
                              )}
                            </p>
                            {deployment.commit_message && (
                              <p className="mt-0.5 truncate text-[13px] text-foreground/80">
                                {deployment.commit_message}
                              </p>
                            )}
                          </div>
                          <div className="flex shrink-0 flex-col items-end gap-0.5 text-right">
                            <span className="font-mono text-xs tabular-nums text-muted-foreground">
                              {started.toLocaleTimeString([], {
                                hour: "2-digit",
                                minute: "2-digit",
                              })}
                            </span>
                            <span className="whitespace-nowrap text-xs text-subtle">
                              {started.toLocaleDateString([], {
                                month: "short",
                                day: "numeric",
                                year: "numeric",
                              })}
                            </span>
                            {canRestoreDeployment(deployment) && (
                              <Button
                                type="button"
                                variant="ghost"
                                size="sm"
                                className="-mr-2 mt-1 text-muted-foreground"
                                disabled={
                                  actionLoading ||
                                  rollbackLoading ||
                                  project.status === "building"
                                }
                                onClick={(e) => {
                                  e.stopPropagation();
                                  setRollbackPassword("");
                                  setRollbackTarget(deployment);
                                }}
                              >
                                Restore
                              </Button>
                            )}
                          </div>
                        </li>
                      );
                    })}
                    </ul>
                    </>
                  )}

                  {deploymentTotal > HISTORY_PAGE_SIZE && (
                    <div className="flex items-center justify-between gap-2 border-t border-border px-2 py-1.5">
                      <Button
                        variant="ghost"
                        size="sm"
                        disabled={historyPage <= 0 || historyLoading}
                        onClick={(e) => {
                          e.stopPropagation();
                          goHistoryPage(historyPage - 1);
                        }}
                        className="text-muted-foreground"
                        aria-label="Previous history page"
                      >
                        <ChevronLeft className="size-4" />
                        Previous
                      </Button>
                      <span className="font-mono text-xs tabular-nums text-muted-foreground">
                        {historyPage + 1} / {historyPageCount}
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
                        className="text-muted-foreground"
                        aria-label="Next history page"
                      >
                        Next
                        <ChevronRight className="size-4" />
                      </Button>
                    </div>
                  )}
                </div>
              </section>
            </div>
          </TabsContent>
          )}

          {!isDatabase && (
          <TabsContent value="env" className="space-y-4">
            <SectionHeading
              title={
                scopedService
                  ? `Environment · ${scopedService.name}`
                  : multiService
                    ? "Shared environment"
                    : "Environment"
              }
              description={
                scopedService
                  ? "Only this service’s build and runtime. Overrides shared keys with the same name."
                  : multiService
                    ? "Injected into every service. Open a service for app-specific secrets."
                    : "Secrets and configuration for this project’s build and runtime."
              }
            />
            <EnvVarsManager
              key={`${projectId}:${
                workspace === "service" && selectedService
                  ? selectedService.name
                  : "__shared__"
              }`}
              projectId={projectId}
              multiService={multiService}
              serviceName={
                workspace === "service" && selectedService
                  ? selectedService.name
                  : ""
              }
            />
          </TabsContent>
          )}

          {!isDatabase && showProjectTabs && (
          <TabsContent value="build" className="space-y-5">
              <section className="rounded-lg border border-border bg-card">
                <div className="border-b border-border px-4 py-3">
                  <h2 className="text-[15px] font-semibold leading-5">
                    Build settings
                  </h2>
                  <p className="mt-0.5 text-[13px] text-muted-foreground">
                    Builder, source directory, and container port used by deployments.
                  </p>
                </div>

                <div className="space-y-5 p-4 sm:p-5">
                  <fieldset>
                    <legend className="mb-1.5 text-[13px] font-medium">
                      Builder
                    </legend>
                    <div className="grid grid-cols-1 gap-2 sm:grid-cols-3">
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
                            "rounded-lg border bg-card p-3 text-left transition-colors duration-150",
                            buildType === value
                              ? "border-brand ring-1 ring-brand"
                              : "border-input hover:bg-secondary/50",
                          )}
                        >
                          <span className="block text-sm font-medium text-foreground">
                            {label}
                          </span>
                          <span className="mt-0.5 block text-xs text-muted-foreground">
                            {description}
                          </span>
                        </button>
                      ))}
                    </div>
                  </fieldset>

                  <div className="grid max-w-2xl grid-cols-1 gap-4 sm:grid-cols-2">
                    <div className="space-y-1.5">
                      <label className="text-[13px] font-medium">Base directory</label>
                      <Input
                        value={baseDirectory}
                        onChange={(e) => setBaseDirectory(e.target.value)}
                        placeholder="."
                        className="font-mono"
                      />
                      <p className="text-xs text-muted-foreground">
                        Relative to the repository root, such as{" "}
                        <code className="font-mono">apps/web</code>.
                      </p>
                    </div>
                    <div className="space-y-1.5">
                      <label className="text-[13px] font-medium">Internal port</label>
                      <Input
                        type="number"
                        min={1}
                        max={65535}
                        value={internalPort}
                        onChange={(e) => setInternalPort(Number(e.target.value))}
                        className="font-mono"
                      />
                      <p className="text-xs text-muted-foreground">
                        Port the application listens on inside its container.
                      </p>
                    </div>

                    {buildType === "dockerfile" && (
                      <div className="space-y-1.5 sm:col-span-2">
                        <label className="text-[13px] font-medium">Dockerfile path</label>
                        <Input
                          value={dockerfilePath}
                          onChange={(e) => setDockerfilePath(e.target.value)}
                          placeholder="Dockerfile"
                          className="font-mono"
                        />
                        <p className="text-xs text-muted-foreground">
                          Path relative to the base directory.
                        </p>
                      </div>
                    )}
                  </div>

                  <label className="flex max-w-2xl cursor-pointer items-start gap-3">
                    <input
                      type="checkbox"
                      className="mt-0.5 size-4 shrink-0 accent-brand"
                      checked={publishHostPort}
                      onChange={(e) => setPublishHostPort(e.target.checked)}
                    />
                    <span>
                      <span className="block text-[13px] font-medium">
                        Publish host ports
                      </span>
                      <span className="mt-0.5 block text-xs leading-5 text-muted-foreground">
                        Off by default. Traffic should use your domain via nginx-proxy on an
                        isolated project network. Enable only if you need a raw host port from the pool.
                      </span>
                    </span>
                  </label>
                </div>

                <div className="flex justify-end border-t border-border px-4 py-3">
                  <Button onClick={handleSaveBuild} disabled={buildSaving}>
                    {buildSaving && <Loader2 className="animate-spin" />}
                    Save build settings
                  </Button>
                </div>
              </section>

              <section className="rounded-lg border border-border bg-card">
                <div className="flex items-start justify-between gap-4 border-b border-border px-4 py-3">
                  <div className="min-w-0">
                    <h2 className="text-[15px] font-semibold leading-5">
                      Detected build
                    </h2>
                    <p className="mt-0.5 text-[13px] text-muted-foreground">
                      How Docklift currently resolves this source.
                    </p>
                  </div>
                  <Button
                    variant="outline"
                    size="sm"
                    onClick={fetchBuildDetection}
                    disabled={buildDetecting}
                  >
                    <RefreshCw className={cn("size-3.5", buildDetecting && "animate-spin")} />
                    Detect again
                  </Button>
                </div>
                {buildDetection ? (
                  <dl className="divide-y divide-border text-[13px]">
                    {(
                      [
                        ["Requested", <span className="capitalize">{buildDetection.requestedType}</span>],
                        ["Resolved", <span className="font-medium capitalize">{buildDetection.resolvedType}</span>],
                        ["Base directory", <code className="font-mono">{buildDetection.baseDirectory}</code>],
                        ...(buildDetection.dockerfilePath
                          ? [["Dockerfile", <code className="font-mono">{buildDetection.dockerfilePath}</code>]]
                          : []),
                        ["Detected", <span className="text-muted-foreground">{buildDetection.detected}</span>],
                        ...(buildDetection.manifests.length > 0
                          ? [["Manifests", <span className="font-mono">{buildDetection.manifests.join(", ")}</span>]]
                          : []),
                      ] as Array<[string, ReactNode]>
                    ).map(([label, value]) => (
                      <div
                        key={label}
                        className="grid gap-1 px-4 py-2.5 sm:grid-cols-[160px_minmax(0,1fr)] sm:gap-4"
                      >
                        <dt className="text-muted-foreground">{label}</dt>
                        <dd className="min-w-0 break-words text-foreground">{value}</dd>
                      </div>
                    ))}
                  </dl>
                ) : (
                  <div className="flex items-center justify-center gap-2 px-4 py-10 text-[13px] text-muted-foreground">
                    {buildDetecting && <Loader2 className="size-4 animate-spin" />}
                    {buildDetecting ? "Detecting build configuration…" : "No detection result available."}
                  </div>
                )}
              </section>
          </TabsContent>
          )}

          {!isDatabase && showServiceTabs && (
          <TabsContent value="storage" className="space-y-5">
              <SectionHeading
                title={
                  scopedService
                    ? `Storage · ${scopedService.name}`
                    : "Persistent storage"
                }
                description={
                  <>
                    Attach Docker volumes to keep application data across redeployments.
                    Deploy after changing mounts.
                    {scopedService
                      ? ` Mounts below belong to ${scopedService.name} only.`
                      : ""}
                  </>
                }
              />

              <div className="space-y-1 rounded-md border border-info/40 bg-secondary/60 px-3 py-2.5 text-[13px] leading-5 text-muted-foreground">
                <p>
                  An external <code className="font-mono text-foreground">DATABASE_URL</code> points
                  to a separate database and is unaffected by these mounts.
                </p>
                <p>
                  SQLite files and user uploads need a mount at the directory where the app writes
                  them. Application migrations may still modify or remove stored data, so keep backups.
                </p>
              </div>

              <section className="rounded-lg border border-border bg-card">
                <div className="border-b border-border px-4 py-3">
                  <h2 className="text-[15px] font-semibold leading-5">Attach storage</h2>
                </div>
                <div className="grid grid-cols-1 gap-4 p-4 sm:grid-cols-3 sm:p-5">
                  <div className="space-y-1.5">
                    <label className="text-[13px] font-medium">Service</label>
                    <select
                      value={storageService}
                      onChange={(e) => setStorageService(e.target.value)}
                      disabled={
                        services.length === 0 ||
                        workspace === "service" ||
                        !multiService
                      }
                      className="h-9 w-full rounded-md border border-input bg-card px-3 text-sm transition-colors focus-visible:border-brand focus-visible:outline-hidden focus-visible:ring-3 focus-visible:ring-brand/20 disabled:opacity-60 pointer-coarse:h-10"
                    >
                      {services.length === 0 && <option value="">No services available</option>}
                      {services.map((service) => (
                        <option key={service.id} value={service.name}>{service.name}</option>
                      ))}
                    </select>
                  </div>
                  <div className="space-y-1.5">
                    <label className="text-[13px] font-medium">Label</label>
                    <Input
                      value={storageName}
                      onChange={(e) => setStorageName(e.target.value)}
                      placeholder="uploads"
                    />
                  </div>
                  <div className="space-y-1.5">
                    <label className="text-[13px] font-medium">Mount path</label>
                    <Input
                      value={storagePath}
                      onChange={(e) => setStoragePath(e.target.value)}
                      placeholder="/app/uploads"
                      className="font-mono"
                    />
                    <p className="text-xs text-muted-foreground">Absolute path inside the container.</p>
                  </div>
                </div>
                <div className="flex justify-end border-t border-border px-4 py-3">
                  <Button
                    onClick={handleAddStorage}
                    disabled={storageSaving || services.length === 0}
                  >
                    {storageSaving && <Loader2 className="animate-spin" />}
                    Attach storage
                  </Button>
                </div>
              </section>

              <section className="rounded-lg border border-border bg-card">
                <div className="border-b border-border px-4 py-3">
                  <h2 className="text-[15px] font-semibold leading-5">Attached mounts</h2>
                </div>
                {storageLoading ? (
                  <div className="flex justify-center py-10">
                    <Loader2 className="size-5 animate-spin text-subtle" />
                  </div>
                ) : (() => {
                  const visibleMounts =
                    workspace === "service" && selectedService
                      ? storageMounts.filter(
                          (m) => m.service_name === selectedService.name,
                        )
                      : storageMounts;
                  if (visibleMounts.length === 0) {
                    return (
                      <div className="px-4 py-12 text-center">
                        <p className="text-[15px] font-semibold">
                          No persistent storage attached
                          {workspace === "service" && selectedService
                            ? ` for ${selectedService.name}`
                            : ""}
                        </p>
                        <p className="mx-auto mt-1 max-w-sm text-[13.5px] text-muted-foreground">
                          Attach a volume above, then deploy to mount it.
                        </p>
                      </div>
                    );
                  }
                  return (
                    <ul className="divide-y divide-border">
                      {visibleMounts.map((mount) => (
                        <li
                          key={mount.id}
                          className="group flex min-h-11 items-center justify-between gap-4 px-4 py-2.5 transition-colors duration-150 hover:bg-secondary/50"
                        >
                          <div className="min-w-0">
                            <p className="truncate text-sm font-medium">
                              {mount.display_name || mount.name}
                            </p>
                            <p className="truncate font-mono text-xs text-muted-foreground">
                              {mount.service_name}
                              <span className="text-subtle"> · </span>
                              {mount.mount_path}
                            </p>
                          </div>
                          <Button
                            variant="ghost"
                            size="icon"
                            onClick={() => setStorageToDelete(mount)}
                            className="shrink-0 text-subtle opacity-60 hover:text-danger group-hover:opacity-100 focus-visible:opacity-100"
                            title="Delete storage"
                          >
                            <Trash2 />
                          </Button>
                        </li>
                      ))}
                    </ul>
                  );
                })()}
              </section>
          </TabsContent>
          )}

          {!isDatabase && showProjectTabs && (
          <TabsContent value="source">
            <section className="overflow-hidden rounded-lg border border-border bg-card">
              <button
                type="button"
                onClick={() => setSourceFilesOpen((open) => !open)}
                aria-expanded={sourceFilesOpen}
                className="flex w-full items-center justify-between gap-3 px-4 py-3 text-left transition-colors duration-150 hover:bg-secondary/50 focus-visible:outline-2 focus-visible:-outline-offset-2 focus-visible:outline-ring"
              >
                <span className="min-w-0">
                  <span className="block truncate text-[15px] font-semibold leading-5">
                    Project files
                  </span>
                  <span className="mt-0.5 block text-[13px] text-muted-foreground">
                    Source uploaded or cloned for this project.
                  </span>
                </span>
                <ChevronDown
                  aria-hidden
                  className={cn(
                    "size-4 shrink-0 text-subtle transition-transform duration-150",
                    !sourceFilesOpen && "-rotate-90",
                  )}
                />
              </button>
              {sourceFilesOpen && (
                <div className="border-t border-border">
                  <FileTree files={files} onFileEdit={openFileEditor} />
                </div>
              )}
            </section>
          </TabsContent>
          )}

          {!isDatabase && showServiceTabs && (
          <TabsContent value="domains" className="space-y-5">
            <DnsGuideCard serverIP={serverIP} />

            <div className="space-y-3">
              <SectionHeading
                title={
                  scopedService
                    ? `Domains · ${scopedService.name}`
                    : "Service domains"
                }
                description="Belongs to this service. Saves the hostname, wires the reverse proxy, and requests a certificate."
              />

              <div className="grid gap-4">
                {(scopedService ? [scopedService] : services).map((svc) => (
                  <ServiceDomainCard
                    key={svc.id}
                    service={svc}
                    projectId={projectId}
                    serverIP={serverIP}
                    onUpdate={fetchProject}
                  />
                ))}

                {services.length === 0 && (
                  <div className="rounded-lg border border-border bg-card py-14 text-center">
                    <p className="text-[15px] font-semibold">No services found</p>
                    <p className="mx-auto mt-1 max-w-sm text-[13.5px] text-muted-foreground">
                      Deploy the project to create its services, then add a domain.
                    </p>
                  </div>
                )}
              </div>
            </div>
          </TabsContent>
          )}

          {showServiceTabs && (
          <TabsContent value="logs" className="space-y-4">
            <SectionHeading
              title={
                scopedService
                  ? `Runtime logs · ${scopedService.name}`
                  : "Runtime logs"
              }
              description={
                workspace === "service"
                  ? "Live container output for this service only — not the whole project."
                  : "Live container output for this project’s service."
              }
            />
            <ContainerLogsPanel
              projectId={projectId}
              services={
                workspace === "service" && selectedService
                  ? [selectedService]
                  : services
              }
              activeTab={activeTab}
              preferredContainerName={
                workspace === "service"
                  ? selectedService?.container_name
                  : undefined
              }
            />
          </TabsContent>
          )}
        </Tabs>
      </div>

      {editingFile && (
        <FileEditor
          projectId={projectId}
          filename={editingFile.name}
          content={editingFile.content}
          onClose={() => setEditingFile(null)}
          onSave={() => {
            // Keep editor open; refresh tree/metadata in background
            fetchProject();
          }}
        />
      )}

      <Dialog open={isConfirmOpen} onOpenChange={setIsConfirmOpen}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>{actionDetails.title}</DialogTitle>
            <DialogDescription>{actionDetails.description}</DialogDescription>
          </DialogHeader>
          <DialogFooter>
            <Button variant="outline" onClick={() => setIsConfirmOpen(false)}>
              Cancel
            </Button>
            <Button
              variant={actionDetails.confirmVariant}
              onClick={executeAction}
              disabled={actionLoading}
            >
              {actionLoading && <Loader2 className="animate-spin" />}
              {actionDetails.confirmText}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
      <Dialog
        open={!!rollbackTarget}
        onOpenChange={(open) => {
          if (!open && !rollbackLoading) {
            setRollbackTarget(null);
            setRollbackPassword("");
          }
        }}
      >
        <DialogContent>
          <DialogHeader>
            <DialogTitle>Restore previous deployment</DialogTitle>
            <DialogDescription>
              Reverts runtime containers to images from deployment{" "}
              <span className="font-mono text-foreground">
                #{rollbackTarget?.id.split("-")[0]}
              </span>
              {rollbackTarget?.commit_sha ? (
                <>
                  {" "}and resets git to{" "}
                  <span className="font-mono text-foreground">
                    {rollbackTarget.commit_sha.slice(0, 12)}
                  </span>
                </>
              ) : null}
              . Does not rebuild. Requires your account password.
            </DialogDescription>
          </DialogHeader>
          <div className="space-y-1.5">
            <label className="text-[13px] font-medium">
              Account password
            </label>
            <Input
              type="password"
              value={rollbackPassword}
              onChange={(e) => setRollbackPassword(e.target.value)}
              placeholder="Your Docklift password"
              autoComplete="current-password"
              disabled={rollbackLoading}
              onKeyDown={(e) => {
                if (e.key === "Enter" && rollbackPassword.trim()) {
                  e.preventDefault();
                  void handleRollback();
                }
              }}
            />
          </div>
          <DialogFooter>
            <Button
              variant="outline"
              disabled={rollbackLoading}
              onClick={() => {
                setRollbackTarget(null);
                setRollbackPassword("");
              }}
            >
              Cancel
            </Button>
            <Button
              onClick={() => void handleRollback()}
              disabled={!rollbackPassword.trim() || rollbackLoading}
            >
              {rollbackLoading && <Loader2 className="animate-spin" />}
              Restore previous
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
      <Dialog
        open={!!storageToDelete}
        onOpenChange={(open) => !open && setStorageToDelete(null)}
      >
        <DialogContent>
          <DialogHeader>
            <DialogTitle>Delete persistent storage</DialogTitle>
            <DialogDescription>
              This permanently deletes the{" "}
              <span className="font-mono text-foreground">
                {storageToDelete?.display_name || storageToDelete?.name}
              </span>{" "}
              mount and its Docker volume. Stop the project first. Stored files cannot be recovered.
            </DialogDescription>
          </DialogHeader>
          <DialogFooter>
            <Button variant="outline" onClick={() => setStorageToDelete(null)}>
              Cancel
            </Button>
            <Button
              variant="destructive"
              onClick={handleDeleteStorage}
              disabled={storageSaving}
            >
              {storageSaving && <Loader2 className="animate-spin" />}
              Delete storage and data
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </>
  );
}

function SectionHeading({
  title,
  description,
}: {
  title: string;
  description?: ReactNode;
}) {
  return (
    <div className="min-w-0">
      <h2 className="truncate text-[15px] font-semibold leading-5">{title}</h2>
      {description && (
        <p className="mt-0.5 max-w-3xl text-[13px] leading-5 text-muted-foreground">
          {description}
        </p>
      )}
    </div>
  );
}
