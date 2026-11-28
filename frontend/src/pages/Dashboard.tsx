// Projects page — filterable table with host resource summary and pagination.

import {
  useEffect,
  useMemo,
  useState,
  useCallback,
  useRef,
  type KeyboardEvent,
} from "react";
import { PageHeader } from "@/components/shell/PageHeader";
import { ProjectQuickActions } from "@/components/project/ProjectQuickActions";
import {
  ProjectResourceCell,
  ResourceMeter,
  aggregateProjectStats,
  type ProjectResourceStats,
} from "@/components/project/ProjectResourceCell";
import { StatusBadge } from "@/components/StatusBadge";
import { Button } from "@/components/ui/button";
import { Project } from "@/lib/types";
import { API_URL, cn } from "@/lib/utils";
import { apiCache, authFetch } from "@/lib/auth";
import { API } from "@/lib/prefetch";
import { Plus, RefreshCw, ChevronLeft, ChevronRight } from "lucide-react";
import { useNavigate } from "react-router-dom";

type StatusFilter = "all" | "running" | "stopped" | "building" | "error" | "degraded";

const PAGE_SIZE = 8;

interface HostResources {
  cpuPercent: number;
  memoryPercent: number;
}

function formatDateTime(iso: string): { date: string; time: string; full: string } {
  const d = new Date(iso);
  if (Number.isNaN(d.getTime())) {
    return { date: "—", time: "", full: "—" };
  }
  return {
    date: d.toLocaleDateString(undefined, {
      year: "numeric",
      month: "short",
      day: "numeric",
    }),
    time: d.toLocaleTimeString(undefined, {
      hour: "2-digit",
      minute: "2-digit",
    }),
    full: d.toLocaleString(),
  };
}

function matchesFilter(project: Project, filter: StatusFilter): boolean {
  if (filter === "all") return true;
  if (filter === "running") {
    return project.status === "running" || project.status === "degraded";
  }
  if (filter === "stopped") {
    return project.status === "stopped" || project.status === "pending";
  }
  if (filter === "error") {
    return project.status === "error";
  }
  return project.status === filter;
}

/** `/api/projects` lists databases too; this page shows applications only. */
function appProjects(data: unknown): Project[] {
  const list = Array.isArray(data) ? (data as Project[]) : [];
  return list.filter((p) => p.project_type !== "database");
}

function hostResources(
  data: { cpu?: unknown; memory?: unknown } | null | undefined,
): HostResources | null {
  if (data === undefined) return null;
  return {
    cpuPercent: Number(data?.cpu ?? 0),
    memoryPercent: Number(data?.memory ?? 0),
  };
}

function openProjectFromKeyboard(event: KeyboardEvent, open: () => void) {
  // Ignore when focus is on a nested control (Redeploy, Delete, etc.).
  if (event.target !== event.currentTarget) return;
  if (event.key === "Enter" || event.key === " ") {
    event.preventDefault();
    open();
  }
}

export default function Dashboard() {
  // Last known data draws at once on a revisit; the mount fetch below refreshes it.
  const [projects, setProjects] = useState<Project[]>(() => appProjects(apiCache.peek(API.projects)));
  const [loading, setLoading] = useState(() => apiCache.peek(API.projects) === undefined);
  const [filter, setFilter] = useState<StatusFilter>("all");
  const [page, setPage] = useState(0);
  const [resources, setResources] = useState<HostResources | null>(() =>
    hostResources(apiCache.peek(API.systemQuick)),
  );
  const [projectStats, setProjectStats] = useState<
    Record<string, ProjectResourceStats | null | undefined>
  >({});
  const pageItemsRef = useRef<Project[]>([]);
  const statsInFlightRef = useRef(false);
  const navigate = useNavigate();

  const fetchProjects = useCallback(async () => {
    try {
      const res = await apiCache.fetch(API.projects);
      if (!res.ok) return;
      setProjects(appProjects(await res.json()));
    } catch (error) {
      console.error("Failed to fetch projects:", error);
    } finally {
      setLoading(false);
    }
  }, []);

  const fetchResources = useCallback(async () => {
    try {
      const res = await apiCache.fetch(API.systemQuick);
      if (!res.ok) return;
      setResources(hostResources(await res.json()));
    } catch {
      // Host stats are optional on this page.
    }
  }, []);

  const runningCount = projects.filter(
    (p) => p.status === "running" || p.status === "degraded",
  ).length;
  const buildingCount = projects.filter((p) => p.status === "building").length;
  const stoppedCount = projects.filter(
    (p) => p.status === "stopped" || p.status === "pending",
  ).length;
  const errorCount = projects.filter((p) => p.status === "error").length;
  const isBuilding = buildingCount > 0;

  useEffect(() => {
    fetchProjects();
    const interval = setInterval(fetchProjects, isBuilding ? 3000 : 10000);
    return () => clearInterval(interval);
  }, [fetchProjects, isBuilding]);

  useEffect(() => {
    fetchResources();
    const interval = setInterval(fetchResources, 15000);
    return () => clearInterval(interval);
  }, [fetchResources]);

  const filtered = useMemo(
    () => projects.filter((p) => matchesFilter(p, filter)),
    [projects, filter],
  );

  const pageCount = Math.max(1, Math.ceil(filtered.length / PAGE_SIZE) || 1);
  const safePage = Math.min(page, pageCount - 1);
  const pageItems = filtered.slice(safePage * PAGE_SIZE, safePage * PAGE_SIZE + PAGE_SIZE);
  pageItemsRef.current = pageItems;
  // Restart the poller when visible row identity or status changes.
  const pageLiveKey = pageItems
    .map((p) => `${p.id}:${p.status}`)
    .join("|");

  useEffect(() => {
    setPage(0);
  }, [filter]);

  useEffect(() => {
    if (page > pageCount - 1) setPage(Math.max(0, pageCount - 1));
  }, [page, pageCount]);

  // Live container CPU/RAM for visible running projects (ref avoids stale status).
  useEffect(() => {
    let cancelled = false;

    const loadStats = async () => {
      if (statsInFlightRef.current) return;
      const rows = pageItemsRef.current;
      const live = rows.filter(
        (p) => p.status === "running" || p.status === "degraded",
      );

      if (live.length === 0) {
        if (!cancelled) setProjectStats({});
        return;
      }

      statsInFlightRef.current = true;
      // Mark live rows as loading until the first successful poll lands.
      setProjectStats((prev) => {
        const next: Record<string, ProjectResourceStats | null | undefined> = {};
        for (const project of live) {
          next[project.id] =
            project.id in prev ? prev[project.id] : undefined;
        }
        return next;
      });

      try {
        const entries = await Promise.all(
          live.map(async (project) => {
            try {
              const res = await authFetch(
                `${API_URL}/api/deployments/${project.id}/stats`,
              );
              if (!res.ok) return [project.id, null] as const;
              const data = await res.json();
              return [project.id, aggregateProjectStats(data)] as const;
            } catch {
              return [project.id, null] as const;
            }
          }),
        );

        if (cancelled) return;
        const next: Record<string, ProjectResourceStats | null> = {};
        for (const [id, stats] of entries) next[id] = stats;
        setProjectStats(next);
      } finally {
        statsInFlightRef.current = false;
      }
    };

    void loadStats();
    const interval = setInterval(loadStats, 5000);
    return () => {
      cancelled = true;
      clearInterval(interval);
    };
  }, [pageLiveKey]);

  const statusTabs: Array<{
    id: StatusFilter;
    label: string;
    count: number;
    tone?: "danger";
  }> = [
    { id: "all", label: "All", count: projects.length },
    { id: "running", label: "Running", count: runningCount },
    { id: "stopped", label: "Stopped", count: stoppedCount },
    { id: "building", label: "Building", count: buildingCount },
    { id: "error", label: "Failed", count: errorCount, tone: "danger" },
  ];

  const rangeLabel =
    filtered.length === 0
      ? "0"
      : `${safePage * PAGE_SIZE + 1}–${Math.min(
          (safePage + 1) * PAGE_SIZE,
          filtered.length,
        )}`;

  return (
    <>
      <PageHeader
        title="Projects"
        description="Applications deployed on this server."
        actions={
          <>
            <Button
              variant="outline"
              size="icon"
              onClick={() => {
                fetchProjects();
                fetchResources();
              }}
              title="Refresh projects"
            >
              <RefreshCw className="text-subtle" />
            </Button>
            <Button
              onClick={() => navigate("/projects/new")}
              className="flex-1 sm:flex-none"
            >
              <Plus />
              New project
            </Button>
          </>
        }
      />

      <div
        className="hairline-b mb-4 flex gap-5 overflow-x-auto scrollbar-none"
        role="group"
        aria-label="Filter projects by status"
      >
        {statusTabs.map((tab) => {
          const active = filter === tab.id;
          return (
            <button
              key={tab.id}
              type="button"
              aria-pressed={active}
              onClick={() => setFilter(tab.id)}
              className={cn(
                "inline-flex h-10 shrink-0 items-center gap-1.5 border-b-2 text-[13.5px] transition-colors duration-150 focus-visible:-outline-offset-2",
                active
                  ? "border-brand font-medium text-foreground"
                  : "border-transparent text-muted-foreground hover:text-foreground",
              )}
            >
              {tab.label}
              <span
                className={cn(
                  "font-mono text-[13px] tabular-nums",
                  tab.tone === "danger" && tab.count > 0
                    ? "text-danger"
                    : "text-subtle",
                )}
              >
                {tab.count}
              </span>
            </button>
          );
        })}
      </div>

      <div className="overflow-hidden rounded-lg border border-border bg-card">
        {loading ? (
          <div className="divide-y divide-border" aria-hidden="true">
            {[1, 2, 3, 4].map((i) => (
              <div key={i} className="flex h-14 items-center gap-6 px-4">
                <div className="space-y-2">
                  <div className="h-2.5 w-36 rounded-sm bg-secondary" />
                  <div className="h-2 w-52 rounded-sm bg-secondary/70" />
                </div>
              </div>
            ))}
          </div>
        ) : projects.length === 0 ? (
          <div className="px-4 py-14 text-center">
            <h2 className="text-[15px] font-semibold">No projects yet</h2>
            <p className="mx-auto mt-1 max-w-sm text-[13.5px] leading-5 text-muted-foreground">
              Connect a repository or upload your source. Docklift detects the
              build, creates the image, and keeps it running.
            </p>
            <Button
              variant="outline"
              onClick={() => navigate("/projects/new")}
              className="mt-5"
            >
              <Plus />
              Create project
            </Button>
          </div>
        ) : filtered.length === 0 ? (
          <div className="px-4 py-14 text-center">
            <p className="text-[13.5px] text-muted-foreground">
              No projects match this filter.
            </p>
            <Button
              type="button"
              variant="link"
              onClick={() => setFilter("all")}
              className="mt-1 h-auto px-0 text-[13.5px]"
            >
              Show all projects
            </Button>
          </div>
        ) : (
          <>
            {/* Mobile / tablet: stacked rows inside the same panel */}
            <div className="divide-y divide-border md:hidden">
              {pageItems.map((project) => {
                const created = formatDateTime(project.created_at);
                const updated = formatDateTime(project.updated_at);
                const live =
                  project.status === "running" || project.status === "degraded";
                return (
                  <article
                    key={project.id}
                    role="link"
                    tabIndex={0}
                    aria-label={`Open ${project.name}`}
                    onClick={() => navigate(`/projects/${project.id}`)}
                    onKeyDown={(e) =>
                      openProjectFromKeyboard(e, () =>
                        navigate(`/projects/${project.id}`),
                      )
                    }
                    className="cursor-pointer px-4 py-3.5 transition-colors duration-150 hover:bg-secondary/50 focus-visible:bg-secondary/50 focus-visible:-outline-offset-2"
                  >
                    <div className="flex items-start justify-between gap-3">
                      <div className="min-w-0">
                        <p className="truncate text-sm font-medium leading-5">
                          {project.name}
                        </p>
                        <Manifest parts={projectManifest(project)} />
                      </div>
                      <span className="pt-0.5">
                        <StatusBadge status={project.status} />
                      </span>
                    </div>

                    <div className="mt-2.5 flex flex-wrap gap-x-4 gap-y-1 text-xs leading-4 text-muted-foreground">
                      <span title={created.full}>
                        Created{" "}
                        <span className="text-foreground">{created.date}</span>{" "}
                        {created.time}
                      </span>
                      <span title={updated.full}>
                        Updated{" "}
                        <span className="text-foreground">{updated.date}</span>{" "}
                        {updated.time}
                      </span>
                    </div>

                    <div className="mt-3 [&>div]:grid [&>div]:max-w-none [&>div]:grid-cols-2 [&>div]:gap-x-5 [&>div]:space-y-0">
                      <ProjectResourceCell
                        live={live}
                        stats={projectStats[project.id]}
                      />
                    </div>

                    <div className="mt-3">
                      <ProjectQuickActions
                        project={project}
                        onRefresh={fetchProjects}
                        compact
                        className="w-full"
                      />
                    </div>
                  </article>
                );
              })}
            </div>

            {/* Desktop: table */}
            <div className="hidden overflow-x-auto md:block">
              <table className="w-full min-w-[880px] text-left text-sm">
                <thead>
                  <tr className="h-9 border-b border-border text-xs text-muted-foreground">
                    <th className="px-4 font-medium">Project</th>
                    <th className="px-4 font-medium">Status</th>
                    <th className="px-4 font-medium">Created</th>
                    <th className="px-4 font-medium">Updated</th>
                    <th className="px-4 font-medium">Resources</th>
                    <th className="px-4 text-right font-medium">
                      <span className="sr-only">Actions</span>
                    </th>
                  </tr>
                </thead>
                <tbody className="divide-y divide-border">
                  {pageItems.map((project) => {
                    const created = formatDateTime(project.created_at);
                    const updated = formatDateTime(project.updated_at);
                    return (
                      <tr
                        key={project.id}
                        role="link"
                        tabIndex={0}
                        aria-label={`Open ${project.name}`}
                        onClick={() => navigate(`/projects/${project.id}`)}
                        onKeyDown={(e) =>
                          openProjectFromKeyboard(e, () =>
                            navigate(`/projects/${project.id}`),
                          )
                        }
                        className="h-14 cursor-pointer transition-colors duration-150 hover:bg-secondary/50 focus-visible:bg-secondary/50 focus-visible:-outline-offset-2"
                      >
                        <td className="max-w-80 px-4 py-2.5">
                          <p className="truncate text-sm font-medium leading-5">
                            {project.name}
                          </p>
                          <Manifest parts={projectManifest(project)} />
                        </td>
                        <td className="px-4 py-2.5">
                          <StatusBadge status={project.status} />
                        </td>
                        <td
                          className="whitespace-nowrap px-4 py-2.5 leading-5"
                          title={created.full}
                        >
                          <p className="text-[13px] text-foreground">{created.date}</p>
                          <p className="text-xs text-muted-foreground">{created.time}</p>
                        </td>
                        <td
                          className="whitespace-nowrap px-4 py-2.5 leading-5"
                          title={updated.full}
                        >
                          <p className="text-[13px] text-foreground">{updated.date}</p>
                          <p className="text-xs text-muted-foreground">{updated.time}</p>
                        </td>
                        <td className="px-4 py-2.5">
                          <ProjectResourceCell
                            live={
                              project.status === "running" ||
                              project.status === "degraded"
                            }
                            stats={projectStats[project.id]}
                          />
                        </td>
                        <td className="px-4 py-2.5">
                          <ProjectQuickActions
                            project={project}
                            onRefresh={fetchProjects}
                            compact
                          />
                        </td>
                      </tr>
                    );
                  })}
                </tbody>
              </table>
            </div>
          </>
        )}

        {!loading && projects.length > 0 && (
          <div className="flex flex-wrap items-center gap-x-6 gap-y-1 border-t border-border px-4 py-2">
            <p className="text-[13px] text-muted-foreground">
              <span className="font-mono text-foreground tabular-nums">
                {rangeLabel}
              </span>{" "}
              of{" "}
              <span className="font-mono text-foreground tabular-nums">
                {filtered.length}
              </span>{" "}
              projects
            </p>

            {resources && (
              <div
                className="order-last flex w-full items-center gap-4 pb-1.5 sm:order-0 sm:ml-auto sm:w-auto sm:pb-0"
                title="Host resource usage"
              >
                <span className="text-xs text-subtle">Host</span>
                <ResourceMeter
                  label="CPU"
                  percent={resources.cpuPercent}
                  display={`${Math.round(resources.cpuPercent)}%`}
                  className="w-32"
                />
                <ResourceMeter
                  label="RAM"
                  percent={resources.memoryPercent}
                  display={`${Math.round(resources.memoryPercent)}%`}
                  className="w-32"
                />
              </div>
            )}

            <div className={cn("ml-auto flex items-center gap-1", resources && "sm:ml-0")}>
                <Button
                  type="button"
                  variant="ghost"
                  size="icon"
                  disabled={safePage <= 0}
                  onClick={() => setPage(Math.max(0, safePage - 1))}
                  aria-label="Previous page"
                >
                  <ChevronLeft />
                </Button>
                <span className="min-w-12 text-center font-mono text-xs text-muted-foreground tabular-nums">
                  {safePage + 1} / {pageCount}
                </span>
                <Button
                  type="button"
                  variant="ghost"
                  size="icon"
                  disabled={safePage >= pageCount - 1}
                  onClick={() => setPage(Math.min(pageCount - 1, safePage + 1))}
                  aria-label="Next page"
                >
                  <ChevronRight />
                </Button>
            </div>
          </div>
        )}
      </div>
    </>
  );
}

/** Mono facts under a resource name, joined with " · ". */
function Manifest({ parts }: { parts: string[] }) {
  if (parts.length === 0) return null;
  return (
    <p className="mt-0.5 truncate font-mono text-xs leading-4 text-muted-foreground">
      {parts.map((part, i) => (
        <span key={`${i}-${part}`}>
          {i > 0 && <span className="text-subtle"> · </span>}
          {part}
        </span>
      ))}
    </p>
  );
}

function projectManifest(project: Project): string[] {
  const parts: string[] = [];
  if (project.source_type === "github") {
    if (project.github_branch) parts.push(project.github_branch);
  } else {
    parts.push(project.source_type);
  }
  const domains = (project.domain || "")
    .split(",")
    .map((d) => d.trim())
    .filter(Boolean);
  if (domains.length > 0) {
    parts.push(
      domains.length > 1 ? `${domains[0]} +${domains.length - 1}` : domains[0],
    );
  } else if (project.port) {
    parts.push(`:${project.port}`);
  }
  if (project.build_type && project.build_type !== "auto") {
    parts.push(project.build_type);
  }
  return parts;
}
