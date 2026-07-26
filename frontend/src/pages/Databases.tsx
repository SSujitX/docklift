// Databases page — same shell width / responsive patterns as Projects.

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
  aggregateProjectStats,
  type ProjectResourceStats,
} from "@/components/project/ProjectResourceCell";
import { StatusBadge } from "@/components/StatusBadge";
import { Button } from "@/components/ui/button";
import { Project } from "@/lib/types";
import { API_URL, cn } from "@/lib/utils";
import { authFetch } from "@/lib/auth";
import {
  Plus,
  RefreshCw,
  Database,
  Sparkles,
  ChevronLeft,
  ChevronRight,
  Server,
} from "lucide-react";
import { useNavigate } from "react-router-dom";

type StatusFilter = "all" | "running" | "stopped" | "building" | "error";

const PAGE_SIZE = 8;

const ENGINE_LABELS: Record<string, string> = {
  postgres: "PostgreSQL",
  mysql: "MySQL",
  mariadb: "MariaDB",
  redis: "Redis",
  mongodb: "MongoDB",
};

function engineLabel(engine?: string | null) {
  if (!engine) return "Database";
  return ENGINE_LABELS[engine] || engine;
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

function openFromKeyboard(event: KeyboardEvent, open: () => void) {
  if (event.target !== event.currentTarget) return;
  if (event.key === "Enter" || event.key === " ") {
    event.preventDefault();
    open();
  }
}

export default function DatabasesPage() {
  const [projects, setProjects] = useState<Project[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [filter, setFilter] = useState<StatusFilter>("all");
  const [page, setPage] = useState(0);
  const [projectStats, setProjectStats] = useState<
    Record<string, ProjectResourceStats | null | undefined>
  >({});
  const pageItemsRef = useRef<Project[]>([]);
  const statsInFlightRef = useRef(false);
  const navigate = useNavigate();

  const fetchProjects = useCallback(async () => {
    try {
      const res = await authFetch(`${API_URL}/api/projects`);
      if (!res.ok) {
        const data = await res.json().catch(() => ({}));
        throw new Error(
          (data as { error?: string }).error ||
            `Failed to load databases (${res.status})`,
        );
      }
      const data = await res.json();
      const databaseProjects = Array.isArray(data)
        ? data.filter((p: Project) => p.project_type === "database")
        : [];
      setProjects(databaseProjects);
      setError(null);
    } catch (err) {
      console.error("Failed to fetch database services:", err);
      setError(err instanceof Error ? err.message : "Failed to load databases");
    } finally {
      setLoading(false);
    }
  }, []);

  useEffect(() => {
    fetchProjects();
    const interval = setInterval(fetchProjects, 10000);
    return () => clearInterval(interval);
  }, [fetchProjects]);

  const filtered = useMemo(
    () => projects.filter((p) => matchesFilter(p, filter)),
    [projects, filter],
  );

  useEffect(() => {
    setPage(0);
  }, [filter]);

  const pageCount = Math.max(1, Math.ceil(filtered.length / PAGE_SIZE));
  const safePage = Math.min(page, pageCount - 1);
  const pageItems = filtered.slice(
    safePage * PAGE_SIZE,
    (safePage + 1) * PAGE_SIZE,
  );
  pageItemsRef.current = pageItems;

  const pageLiveKey = pageItems
    .filter((p) => p.status === "running" || p.status === "degraded")
    .map((p) => p.id)
    .join(",");

  useEffect(() => {
    let cancelled = false;

    const loadStats = async () => {
      if (statsInFlightRef.current) return;
      const live = pageItemsRef.current.filter(
        (p) => p.status === "running" || p.status === "degraded",
      );
      if (live.length === 0) {
        setProjectStats({});
        return;
      }
      statsInFlightRef.current = true;
      try {
        const entries = await Promise.all(
          live.map(async (p) => {
            try {
              const res = await authFetch(
                `${API_URL}/api/deployments/${p.id}/stats`,
              );
              if (!res.ok) return [p.id, null] as const;
              const data = await res.json();
              return [p.id, aggregateProjectStats(data)] as const;
            } catch {
              return [p.id, null] as const;
            }
          }),
        );
        if (cancelled) return;
        const next: Record<string, ProjectResourceStats | null | undefined> = {};
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

  const runningCount = projects.filter(
    (p) => p.status === "running" || p.status === "degraded",
  ).length;
  const stoppedCount = projects.filter(
    (p) => p.status === "stopped" || p.status === "pending",
  ).length;
  const buildingCount = projects.filter((p) => p.status === "building").length;
  const errorCount = projects.filter((p) => p.status === "error").length;

  const statusTabs: Array<{
    id: StatusFilter;
    label: string;
    count: number;
    tone?: "success" | "warning" | "danger";
  }> = [
    { id: "all", label: "Total", count: projects.length },
    { id: "running", label: "Running", count: runningCount, tone: "success" },
    { id: "stopped", label: "Stopped", count: stoppedCount },
    { id: "building", label: "Building", count: buildingCount, tone: "warning" },
    { id: "error", label: "Errored", count: errorCount, tone: "danger" },
  ];

  return (
    <>
      <PageHeader
        title="Databases"
        description="Managed Postgres, MySQL, Redis, and more — private by default, linkable into any app."
        icon={Database}
        actions={
          <>
            <Button
              variant="outline"
              size="icon"
              onClick={() => fetchProjects()}
              title="Refresh databases"
              className="h-10 w-10 border-border/60 bg-background hover:bg-secondary/80"
            >
              <RefreshCw className="h-4 w-4 text-muted-foreground" />
            </Button>
            <Button
              onClick={() => navigate("/databases/new")}
              className="h-10 flex-1 bg-brand px-4 font-semibold text-brand-foreground shadow-lg shadow-brand/20 hover:brightness-110 sm:flex-none sm:px-5"
            >
              <Plus className="h-4 w-4" />
              <span className="sm:hidden">New</span>
              <span className="hidden sm:inline">New Database</span>
            </Button>
          </>
        }
      />

      <div
        className="-mx-3 mb-4 flex gap-2 overflow-x-auto px-3 pb-1 [-ms-overflow-style:none] [scrollbar-width:none] sm:mx-0 sm:flex-wrap sm:overflow-visible sm:px-0 sm:pb-0 [&::-webkit-scrollbar]:hidden"
        role="group"
        aria-label="Filter databases by status"
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
                "inline-flex shrink-0 items-center gap-2 rounded-xl border px-3 py-1.5 text-sm font-medium transition-colors",
                active
                  ? tab.tone === "success"
                    ? "border-emerald-500/30 bg-emerald-500/10 text-emerald-700 dark:text-emerald-300"
                    : tab.tone === "warning"
                      ? "border-amber-500/30 bg-amber-500/10 text-amber-700 dark:text-amber-300"
                      : tab.tone === "danger"
                        ? "border-red-500/30 bg-red-500/10 text-red-700 dark:text-red-300"
                        : "border-brand/30 bg-brand/10 text-brand"
                  : "border-border/60 bg-secondary/40 text-muted-foreground hover:text-foreground",
              )}
            >
              <span className="text-[10px] font-semibold uppercase tracking-wider opacity-70">
                {tab.label}
              </span>
              <span className="font-bold tabular-nums">{tab.count}</span>
            </button>
          );
        })}
      </div>

      {error && (
        <div className="mb-4 rounded-xl border border-red-500/20 bg-red-500/10 px-4 py-3 text-sm text-red-600 dark:text-red-400">
          {error}
        </div>
      )}

      {loading ? (
        <div className="overflow-hidden rounded-2xl border border-border/60">
          {[1, 2, 3, 4].map((i) => (
            <div
              key={i}
              className="h-16 animate-pulse border-b border-border/40 bg-secondary/20 last:border-b-0"
            />
          ))}
        </div>
      ) : projects.length === 0 ? (
        <div className="flex flex-col items-center justify-center rounded-3xl border border-dashed border-border/60 px-4 py-20 text-center">
          <div className="mb-6 flex h-20 w-20 items-center justify-center rounded-[28px] border border-brand/20 bg-brand/10">
            <Database className="h-9 w-9 text-brand" />
          </div>
          <h2 className="mb-3 text-2xl font-bold">No databases yet</h2>
          <p className="mb-8 max-w-sm leading-relaxed text-muted-foreground">
            One-click official images with volumes and generated credentials. Link
            into a project or service — no public IP:port required.
          </p>
          <Button
            onClick={() => navigate("/databases/new")}
            size="lg"
            className="h-12 bg-brand px-8 font-semibold text-brand-foreground shadow-xl shadow-brand/20 hover:brightness-110"
          >
            <Sparkles className="h-4 w-4" />
            Launch Database
          </Button>
        </div>
      ) : filtered.length === 0 ? (
        <div className="rounded-2xl border border-dashed border-border/60 px-4 py-16 text-center">
          <p className="text-sm text-muted-foreground">
            No databases match this filter.
          </p>
          <button
            type="button"
            onClick={() => setFilter("all")}
            className="mt-3 text-sm font-semibold text-brand hover:underline"
          >
            Show all databases
          </button>
        </div>
      ) : (
        <>
          <div className="space-y-3 md:hidden">
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
                    openFromKeyboard(e, () =>
                      navigate(`/projects/${project.id}`),
                    )
                  }
                  className="rounded-2xl border border-border/60 bg-card p-4 shadow-sm transition-colors hover:bg-secondary/30 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-brand/40"
                >
                  <div className="flex items-start justify-between gap-3">
                    <div className="flex min-w-0 items-start gap-3">
                      <span
                        className={cn(
                          "mt-1.5 h-2 w-2 shrink-0 rounded-full",
                          project.status === "running"
                            ? "bg-emerald-500"
                            : project.status === "building"
                              ? "animate-pulse bg-amber-500"
                              : project.status === "error"
                                ? "bg-red-500"
                                : project.status === "degraded"
                                  ? "bg-orange-500"
                                  : "bg-zinc-400",
                        )}
                      />
                      <div className="min-w-0">
                        <p className="truncate text-base font-semibold tracking-tight">
                          {project.name}
                        </p>
                        <div className="mt-0.5 flex items-center gap-1 text-xs text-muted-foreground">
                          <Server className="h-3 w-3 shrink-0" />
                          <span className="truncate">
                            {engineLabel(project.db_engine)}
                          </span>
                        </div>
                      </div>
                    </div>
                    <StatusBadge status={project.status} size="sm" />
                  </div>

                  <div className="mt-3 grid grid-cols-2 gap-3 text-xs">
                    <div title={created.full}>
                      <p className="text-[10px] font-semibold uppercase tracking-wider text-muted-foreground">
                        Created
                      </p>
                      <p className="mt-0.5 tabular-nums text-foreground">
                        {created.date}
                      </p>
                      <p className="tabular-nums text-muted-foreground">
                        {created.time}
                      </p>
                    </div>
                    <div title={updated.full}>
                      <p className="text-[10px] font-semibold uppercase tracking-wider text-muted-foreground">
                        Updated
                      </p>
                      <p className="mt-0.5 tabular-nums text-foreground">
                        {updated.date}
                      </p>
                      <p className="tabular-nums text-muted-foreground">
                        {updated.time}
                      </p>
                    </div>
                  </div>

                  <div className="mt-3 border-t border-border/40 pt-3">
                    <ProjectResourceCell
                      live={live}
                      stats={projectStats[project.id]}
                    />
                  </div>

                  <div className="mt-3 border-t border-border/40 pt-3">
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

          <div className="hidden overflow-hidden rounded-2xl border border-border/60 bg-card md:block">
            <div className="overflow-x-auto">
              <table className="w-full min-w-[860px] text-left text-sm">
                <thead>
                  <tr className="border-b border-border/60 bg-secondary/30 text-[10px] font-semibold uppercase tracking-[0.14em] text-muted-foreground">
                    <th className="px-4 py-3 font-semibold">Database</th>
                    <th className="px-4 py-3 font-semibold">Status</th>
                    <th className="px-4 py-3 font-semibold">Created</th>
                    <th className="px-4 py-3 font-semibold">Updated</th>
                    <th className="px-4 py-3 font-semibold">Resources</th>
                    <th className="px-4 py-3 text-right font-semibold">Actions</th>
                  </tr>
                </thead>
                <tbody>
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
                          openFromKeyboard(e, () =>
                            navigate(`/projects/${project.id}`),
                          )
                        }
                        className="cursor-pointer border-b border-border/40 transition-colors last:border-b-0 hover:bg-secondary/40 focus-visible:bg-secondary/40 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-brand/40"
                      >
                        <td className="px-4 py-3.5">
                          <div className="flex min-w-0 items-center gap-3">
                            <span
                              className={cn(
                                "h-2 w-2 shrink-0 rounded-full",
                                project.status === "running"
                                  ? "bg-emerald-500 shadow-sm shadow-emerald-500/50"
                                  : project.status === "building"
                                    ? "animate-pulse bg-amber-500"
                                    : project.status === "error"
                                      ? "bg-red-500"
                                      : project.status === "degraded"
                                        ? "bg-orange-500"
                                        : "bg-zinc-400",
                              )}
                            />
                            <div className="min-w-0">
                              <p className="truncate font-semibold tracking-tight">
                                {project.name}
                              </p>
                              <div className="mt-0.5 flex items-center gap-1 text-xs text-muted-foreground">
                                <Server className="h-3 w-3 shrink-0" />
                                <span className="truncate">
                                  {engineLabel(project.db_engine)}
                                </span>
                              </div>
                            </div>
                          </div>
                        </td>
                        <td className="px-4 py-3.5">
                          <StatusBadge status={project.status} size="sm" />
                        </td>
                        <td
                          className="px-4 py-3.5 tabular-nums"
                          title={created.full}
                        >
                          <p className="text-sm text-foreground">{created.date}</p>
                          <p className="text-xs text-muted-foreground">
                            {created.time}
                          </p>
                        </td>
                        <td
                          className="px-4 py-3.5 tabular-nums"
                          title={updated.full}
                        >
                          <p className="text-sm text-foreground">{updated.date}</p>
                          <p className="text-xs text-muted-foreground">
                            {updated.time}
                          </p>
                        </td>
                        <td className="px-4 py-3.5">
                          <ProjectResourceCell
                            live={
                              project.status === "running" ||
                              project.status === "degraded"
                            }
                            stats={projectStats[project.id]}
                          />
                        </td>
                        <td
                          className="px-4 py-3.5 text-right"
                          onClick={(e) => e.stopPropagation()}
                        >
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
          </div>
        </>
      )}

      {!loading && projects.length > 0 && (
        <div className="mt-4 flex flex-col gap-3 sm:flex-row sm:items-center sm:justify-between">
          <p className="text-xs leading-relaxed text-muted-foreground">
            Showing{" "}
            <span className="font-semibold text-foreground">
              {filtered.length === 0
                ? 0
                : `${safePage * PAGE_SIZE + 1}–${Math.min(
                    (safePage + 1) * PAGE_SIZE,
                    filtered.length,
                  )}`}
            </span>{" "}
            of{" "}
            <span className="font-semibold text-foreground">
              {filtered.length}
            </span>{" "}
            databases
          </p>

          <div className="flex items-center justify-between gap-1.5 sm:justify-end">
            <Button
              type="button"
              variant="outline"
              size="icon"
              className="h-9 w-9 border-border/60 sm:h-8 sm:w-8"
              disabled={safePage <= 0}
              onClick={() => setPage(Math.max(0, safePage - 1))}
              aria-label="Previous page"
            >
              <ChevronLeft className="h-4 w-4" />
            </Button>
            <span className="min-w-[4.5rem] text-center text-xs font-medium tabular-nums text-muted-foreground">
              {safePage + 1} / {pageCount}
            </span>
            <Button
              type="button"
              variant="outline"
              size="icon"
              className="h-9 w-9 border-border/60 sm:h-8 sm:w-8"
              disabled={safePage >= pageCount - 1}
              onClick={() => setPage(Math.min(pageCount - 1, safePage + 1))}
              aria-label="Next page"
            >
              <ChevronRight className="h-4 w-4" />
            </Button>
          </div>
        </div>
      )}
    </>
  );
}
