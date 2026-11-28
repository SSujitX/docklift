// Databases page — same list pattern as Projects.

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
import { Plus, RefreshCw, ChevronLeft, ChevronRight } from "lucide-react";
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
        title="Databases"
        description="Managed Postgres, MySQL, Redis and more. Private by default, linked into apps over the Docker network."
        actions={
          <>
            <Button
              variant="outline"
              size="icon"
              onClick={() => fetchProjects()}
              title="Refresh databases"
            >
              <RefreshCw className="text-subtle" />
            </Button>
            <Button
              onClick={() => navigate("/databases/new")}
              className="flex-1 sm:flex-none"
            >
              <Plus />
              New database
            </Button>
          </>
        }
      />

      <div
        className="hairline-b mb-4 flex gap-5 overflow-x-auto scrollbar-none"
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

      {error && (
        <div className="mb-4 rounded-md border border-danger/40 bg-secondary/60 px-3 py-2.5 text-[13px] text-foreground">
          {error}
        </div>
      )}

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
            <h2 className="text-[15px] font-semibold">No databases yet</h2>
            <p className="mx-auto mt-1 max-w-sm text-[13.5px] leading-5 text-muted-foreground">
              Official images with a volume and generated credentials. Link one
              into a project or service; no public IP:port required.
            </p>
            <Button
              variant="outline"
              onClick={() => navigate("/databases/new")}
              className="mt-5"
            >
              <Plus />
              Create database
            </Button>
          </div>
        ) : filtered.length === 0 ? (
          <div className="px-4 py-14 text-center">
            <p className="text-[13.5px] text-muted-foreground">
              No databases match this filter.
            </p>
            <Button
              type="button"
              variant="link"
              onClick={() => setFilter("all")}
              className="mt-1 h-auto px-0 text-[13.5px]"
            >
              Show all databases
            </Button>
          </div>
        ) : (
          <>
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
                      openFromKeyboard(e, () =>
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
                        <Manifest parts={databaseManifest(project)} />
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

            <div className="hidden overflow-x-auto md:block">
              <table className="w-full min-w-[880px] text-left text-sm">
                <thead>
                  <tr className="h-9 border-b border-border text-xs text-muted-foreground">
                    <th className="px-4 font-medium">Database</th>
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
                          openFromKeyboard(e, () =>
                            navigate(`/projects/${project.id}`),
                          )
                        }
                        className="h-14 cursor-pointer transition-colors duration-150 hover:bg-secondary/50 focus-visible:bg-secondary/50 focus-visible:-outline-offset-2"
                      >
                        <td className="max-w-80 px-4 py-2.5">
                          <p className="truncate text-sm font-medium leading-5">
                            {project.name}
                          </p>
                          <Manifest parts={databaseManifest(project)} />
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
                        <td
                          className="px-4 py-2.5 text-right"
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
          </>
        )}

        {!loading && projects.length > 0 && (
          <div className="flex flex-wrap items-center justify-between gap-x-6 gap-y-2 border-t border-border px-4 py-2">
            <p className="text-[13px] text-muted-foreground">
              <span className="font-mono text-foreground tabular-nums">
                {rangeLabel}
              </span>{" "}
              of{" "}
              <span className="font-mono text-foreground tabular-nums">
                {filtered.length}
              </span>{" "}
              databases
            </p>

            <div className="flex items-center gap-1">
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

function databaseManifest(project: Project): string[] {
  const parts: string[] = [engineLabel(project.db_engine)];
  if (project.internal_port) parts.push(`:${project.internal_port}`);
  if (project.container_name) parts.push(project.container_name);
  return parts;
}
