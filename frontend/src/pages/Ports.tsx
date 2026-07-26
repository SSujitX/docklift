// Ports page — one-viewport layout; list scrolls inside, page does not.

import {
  useCallback,
  useEffect,
  useMemo,
  useRef,
  useState,
  type KeyboardEvent,
} from "react";
import { useNavigate } from "react-router-dom";
import { Button } from "@/components/ui/button";
import { Port } from "@/lib/types";
import { API_URL, cn } from "@/lib/utils";
import { authFetch } from "@/lib/auth";
import {
  Anchor,
  ChevronLeft,
  ChevronRight,
  ExternalLink,
  Lock,
  Network,
  RefreshCw,
  Unlock,
} from "lucide-react";

type PortFilter = "all" | "allocated" | "available";

interface PrivateRunning {
  id: string;
  name: string;
  status: string;
  project_type?: string;
  db_engine?: string | null;
  publish_host_port?: boolean;
  reason: string;
}

/** Desktop table row height (px) — keep in sync with py-2 + content */
const DESKTOP_ROW_PX = 44;
const DESKTOP_HEAD_PX = 36;
/** Mobile list row height */
const MOBILE_ROW_PX = 52;

function openFromKeyboard(event: KeyboardEvent, open: () => void) {
  if (event.target !== event.currentTarget) return;
  if (event.key === "Enter" || event.key === " ") {
    event.preventDefault();
    open();
  }
}

export default function PortsPage() {
  const navigate = useNavigate();
  const listRef = useRef<HTMLDivElement>(null);
  const [ports, setPorts] = useState<Port[]>([]);
  const [privateRunning, setPrivateRunning] = useState<PrivateRunning[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [filter, setFilter] = useState<PortFilter>("all");
  const [page, setPage] = useState(0);
  const [pageSize, setPageSize] = useState(12);

  const fetchPorts = useCallback(async () => {
    try {
      const res = await authFetch(`${API_URL}/api/ports`);
      if (!res.ok) {
        const data = await res.json().catch(() => ({}));
        throw new Error(
          (data as { error?: string }).error ||
            `Failed to load ports (${res.status})`,
        );
      }
      const data = await res.json();
      // New shape: { ports, private_running } — still accept a bare array
      if (Array.isArray(data)) {
        setPorts(data);
        setPrivateRunning([]);
      } else {
        setPorts(Array.isArray(data?.ports) ? data.ports : []);
        setPrivateRunning(
          Array.isArray(data?.private_running) ? data.private_running : [],
        );
      }
      setError(null);
    } catch (err) {
      setError(err instanceof Error ? err.message : "Failed to load ports");
    } finally {
      setLoading(false);
    }
  }, []);

  useEffect(() => {
    void fetchPorts();
  }, [fetchPorts]);

  const usedPorts = useMemo(
    () => ports.filter((p) => p.is_locked),
    [ports],
  );
  const freePorts = useMemo(
    () => ports.filter((p) => !p.is_locked),
    [ports],
  );

  const poolRange = useMemo(() => {
    if (ports.length === 0) return "—";
    const nums = ports.map((p) => p.port);
    return `${Math.min(...nums)}–${Math.max(...nums)}`;
  }, [ports]);

  const filtered = useMemo(() => {
    if (filter === "allocated") return usedPorts;
    if (filter === "available") return freePorts;
    return [...usedPorts, ...freePorts];
  }, [filter, usedPorts, freePorts]);

  // Fit as many rows as the box height allows — no empty gap under a fixed page size
  useEffect(() => {
    const el = listRef.current;
    if (!el) return;

    const measure = () => {
      const h = el.clientHeight;
      const desktop = window.matchMedia("(min-width: 768px)").matches;
      const usable = desktop ? Math.max(0, h - DESKTOP_HEAD_PX) : h;
      const row = desktop ? DESKTOP_ROW_PX : MOBILE_ROW_PX;
      setPageSize(Math.max(4, Math.floor(usable / row)));
    };

    measure();
    const ro = new ResizeObserver(measure);
    ro.observe(el);
    window.addEventListener("resize", measure);
    return () => {
      ro.disconnect();
      window.removeEventListener("resize", measure);
    };
  }, [loading, ports.length, filter]);

  const pageCount = Math.max(1, Math.ceil(filtered.length / pageSize));
  const safePage = Math.min(page, pageCount - 1);
  const pageItems = filtered.slice(
    safePage * pageSize,
    safePage * pageSize + pageSize,
  );

  useEffect(() => {
    setPage(0);
  }, [filter, pageSize]);

  useEffect(() => {
    if (page > pageCount - 1) setPage(Math.max(0, pageCount - 1));
  }, [page, pageCount]);

  const filterTabs: {
    id: PortFilter;
    label: string;
    count: number;
    tone: "brand" | "warning" | "success";
  }[] = [
    { id: "all", label: "All", count: ports.length, tone: "brand" },
    {
      id: "allocated",
      label: "Allocated",
      count: usedPorts.length,
      tone: "warning",
    },
    {
      id: "available",
      label: "Available",
      count: freePorts.length,
      tone: "success",
    },
  ];

  const openPort = (port: Port) => {
    if (port.project_id) navigate(`/projects/${port.project_id}`);
  };

  return (
    <div
      className={cn(
        "flex flex-col",
        // Fill viewport under top bar + main padding — page itself does not scroll
        "h-[calc(100dvh-var(--shell-topbar)-2.5rem)]",
        "sm:h-[calc(100dvh-var(--shell-topbar)-4rem)]",
      )}
    >
      {/* Compact title row */}
      <div className="mb-2 flex shrink-0 items-center justify-between gap-3 sm:mb-3">
        <div className="flex min-w-0 items-center gap-2.5 sm:gap-3">
          <span className="flex h-9 w-9 shrink-0 items-center justify-center rounded-xl border border-brand/20 bg-brand/10 sm:h-10 sm:w-10 sm:rounded-2xl">
            <Anchor className="h-4 w-4 text-brand sm:h-[1.125rem] sm:w-[1.125rem]" />
          </span>
          <div className="min-w-0">
            <h1 className="truncate text-lg font-bold tracking-tight sm:text-xl">
              Ports
            </h1>
            <p className="truncate text-[11px] text-muted-foreground sm:text-xs">
              Opt-in host pool · {poolRange} · private apps/DBs won&apos;t appear
              here
            </p>
          </div>
        </div>
        <Button
          variant="outline"
          size="icon"
          onClick={() => {
            setLoading(true);
            void fetchPorts();
          }}
          title="Refresh ports"
          className="h-9 w-9 shrink-0 border-border/60 bg-background hover:bg-secondary/80 sm:h-10 sm:w-10"
        >
          <RefreshCw className="h-4 w-4 text-muted-foreground" />
        </Button>
      </div>

      {/* Running without a host port — explains ALLOCATED 0 */}
      {!loading && privateRunning.length > 0 && (
        <div className="mb-2 shrink-0 rounded-xl border border-border/60 bg-secondary/25 px-3 py-2 sm:mb-3 sm:px-3.5 sm:py-2.5">
          <p className="text-[11px] font-semibold text-foreground sm:text-xs">
            {privateRunning.length} running without a host port
          </p>
          <p className="mt-0.5 text-[10px] leading-relaxed text-muted-foreground sm:text-[11px]">
            Host ports are opt-in. Databases stay private (link over Docker DNS).
            Apps need <span className="font-medium text-foreground/80">Build → Publish host ports</span>{" "}
            + redeploy, or a domain.
          </p>
          <div className="mt-2 flex flex-wrap gap-1.5">
            {privateRunning.map((p) => (
              <button
                key={p.id}
                type="button"
                title={p.reason}
                onClick={() => navigate(`/projects/${p.id}`)}
                className="inline-flex max-w-full items-center gap-1.5 rounded-lg border border-border/60 bg-background px-2 py-1 text-[11px] font-medium transition-colors hover:border-brand/40 hover:bg-brand/5"
              >
                <span
                  className={cn(
                    "h-1.5 w-1.5 shrink-0 rounded-full",
                    p.status === "running"
                      ? "bg-emerald-500"
                      : p.status === "building" || p.status === "pending"
                        ? "animate-pulse bg-amber-500"
                        : "bg-orange-500",
                  )}
                />
                <span className="truncate">{p.name}</span>
                <span className="shrink-0 text-[9px] uppercase tracking-wider text-muted-foreground">
                  {p.project_type === "database" ? "DB" : "App"}
                </span>
              </button>
            ))}
          </div>
        </div>
      )}

      {/* Filters + pool meta — one row */}
      <div className="mb-2 flex shrink-0 flex-wrap items-center gap-2 sm:mb-3">
        <div
          className="flex min-w-0 flex-1 gap-1.5 overflow-x-auto [-ms-overflow-style:none] [scrollbar-width:none] [&::-webkit-scrollbar]:hidden"
          role="group"
          aria-label="Filter ports"
        >
          {filterTabs.map((tab) => {
            const active = filter === tab.id;
            return (
              <button
                key={tab.id}
                type="button"
                aria-pressed={active}
                onClick={() => setFilter(tab.id)}
                className={cn(
                  "inline-flex shrink-0 items-center gap-1.5 rounded-lg border px-2.5 py-1 text-xs font-medium transition-colors sm:rounded-xl sm:px-3 sm:py-1.5 sm:text-sm",
                  active
                    ? tab.tone === "success"
                      ? "border-emerald-500/30 bg-emerald-500/10 text-emerald-700 dark:text-emerald-300"
                      : tab.tone === "warning"
                        ? "border-amber-500/30 bg-amber-500/10 text-amber-700 dark:text-amber-300"
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
        <span className="hidden items-center gap-1.5 rounded-lg border border-border/60 bg-secondary/30 px-2 py-1 font-mono text-[10px] text-muted-foreground tabular-nums sm:inline-flex">
          <Network className="h-3 w-3" />
          {ports.length} · docklift_network
        </span>
      </div>

      {error && (
        <div className="mb-2 shrink-0 rounded-xl border border-red-500/20 bg-red-500/10 px-3 py-2 text-xs text-red-600 dark:text-red-400 sm:text-sm">
          {error}
        </div>
      )}

      {/* Fill remaining height — scroll only here */}
      <div className="flex min-h-0 flex-1 flex-col overflow-hidden rounded-2xl border border-border/60 bg-card">
        {loading ? (
          <div className="space-y-0 overflow-hidden">
            {[1, 2, 3, 4, 5, 6].map((i) => (
              <div
                key={i}
                className="h-11 animate-pulse border-b border-border/40 bg-secondary/20 last:border-b-0"
              />
            ))}
          </div>
        ) : ports.length === 0 ? (
          <div className="flex flex-1 flex-col items-center justify-center px-4 py-10 text-center">
            <Anchor className="mb-3 h-8 w-8 text-muted-foreground/40" />
            <p className="text-sm font-semibold">No ports in the pool</p>
            <p className="mt-1 max-w-xs text-xs text-muted-foreground">
              Filled from the configured host port range at startup.
            </p>
          </div>
        ) : filtered.length === 0 ? (
          <div className="flex flex-1 flex-col items-center justify-center px-4 py-10 text-center">
            <p className="text-sm text-muted-foreground">
              No ports match this filter.
            </p>
            <button
              type="button"
              onClick={() => setFilter("all")}
              className="mt-2 text-sm font-semibold text-brand hover:underline"
            >
              Show all ports
            </button>
          </div>
        ) : (
          <>
            <div
              ref={listRef}
              className="shell-scroll min-h-0 flex-1 overflow-hidden overscroll-contain"
            >
              {/* Mobile */}
              <div className="divide-y divide-border/40 md:hidden">
                {pageItems.map((port) => {
                  const allocated = port.is_locked;
                  const clickable = Boolean(port.project_id);
                  return (
                    <article
                      key={port.port}
                      role={clickable ? "link" : undefined}
                      tabIndex={clickable ? 0 : undefined}
                      aria-label={
                        clickable
                          ? `Open project for port ${port.port}`
                          : `Port ${port.port}`
                      }
                      onClick={() => clickable && openPort(port)}
                      onKeyDown={(e) =>
                        clickable && openFromKeyboard(e, () => openPort(port))
                      }
                      className={cn(
                        "flex items-center gap-3 px-3 py-2.5",
                        clickable &&
                          "cursor-pointer hover:bg-secondary/30 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-inset focus-visible:ring-brand/40",
                      )}
                    >
                      <span
                        className={cn(
                          "flex h-8 w-8 shrink-0 items-center justify-center rounded-lg border",
                          allocated
                            ? "border-amber-500/25 bg-amber-500/10 text-amber-600 dark:text-amber-400"
                            : "border-emerald-500/25 bg-emerald-500/10 text-emerald-600 dark:text-emerald-400",
                        )}
                      >
                        {allocated ? (
                          <Lock className="h-3.5 w-3.5" />
                        ) : (
                          <Unlock className="h-3.5 w-3.5" />
                        )}
                      </span>
                      <div className="min-w-0 flex-1">
                        <p className="font-mono text-sm font-semibold tabular-nums">
                          :{port.port}
                          <span
                            className={cn(
                              "ml-2 text-[10px] font-semibold uppercase tracking-wider",
                              allocated
                                ? "text-amber-700 dark:text-amber-300"
                                : "text-emerald-700 dark:text-emerald-300",
                            )}
                          >
                            {allocated ? "In use" : "Free"}
                          </span>
                        </p>
                        <p className="truncate text-[11px] text-muted-foreground">
                          {allocated
                            ? port.project?.name || "Unknown project"
                            : "Free for assignment"}
                        </p>
                      </div>
                      {clickable && (
                        <ExternalLink className="h-3.5 w-3.5 shrink-0 text-brand" />
                      )}
                    </article>
                  );
                })}
              </div>

              {/* Desktop */}
              <table className="hidden w-full text-left text-sm md:table">
                <thead className="sticky top-0 z-10 bg-secondary/95 backdrop-blur-sm">
                  <tr className="border-b border-border/60 text-[10px] font-semibold uppercase tracking-[0.14em] text-muted-foreground">
                    <th className="px-4 py-2.5 font-semibold">Port</th>
                    <th className="px-4 py-2.5 font-semibold">Status</th>
                    <th className="px-4 py-2.5 font-semibold">Project</th>
                    <th className="px-4 py-2.5 text-right font-semibold"> </th>
                  </tr>
                </thead>
                <tbody>
                  {pageItems.map((port) => {
                    const allocated = port.is_locked;
                    const clickable = Boolean(port.project_id);
                    return (
                      <tr
                        key={port.port}
                        role={clickable ? "link" : undefined}
                        tabIndex={clickable ? 0 : undefined}
                        aria-label={
                          clickable
                            ? `Open project for port ${port.port}`
                            : undefined
                        }
                        onClick={() => clickable && openPort(port)}
                        onKeyDown={(e) =>
                          clickable &&
                          openFromKeyboard(e, () => openPort(port))
                        }
                        className={cn(
                          "border-b border-border/40 last:border-b-0",
                          clickable &&
                            "cursor-pointer hover:bg-secondary/40 focus-visible:bg-secondary/40 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-inset focus-visible:ring-brand/40",
                        )}
                      >
                        <td className="px-4 py-2">
                          <div className="flex items-center gap-2">
                            <span
                              className={cn(
                                "flex h-7 w-7 shrink-0 items-center justify-center rounded-md border",
                                allocated
                                  ? "border-amber-500/25 bg-amber-500/10 text-amber-600 dark:text-amber-400"
                                  : "border-emerald-500/25 bg-emerald-500/10 text-emerald-600 dark:text-emerald-400",
                              )}
                            >
                              {allocated ? (
                                <Lock className="h-3 w-3" />
                              ) : (
                                <Unlock className="h-3 w-3" />
                              )}
                            </span>
                            <span className="font-mono text-sm font-semibold tabular-nums">
                              :{port.port}
                            </span>
                          </div>
                        </td>
                        <td className="px-4 py-2">
                          <span
                            className={cn(
                              "inline-flex rounded-md px-1.5 py-0.5 text-[10px] font-semibold uppercase tracking-wider",
                              allocated
                                ? "bg-amber-500/10 text-amber-700 dark:text-amber-300"
                                : "bg-emerald-500/10 text-emerald-700 dark:text-emerald-300",
                            )}
                          >
                            {allocated ? "Allocated" : "Available"}
                          </span>
                        </td>
                        <td className="px-4 py-2">
                          {allocated ? (
                            <div className="min-w-0">
                              <p className="truncate text-sm font-medium tracking-tight">
                                {port.project?.name || "Unknown project"}
                              </p>
                              <p className="font-mono text-[10px] text-muted-foreground">
                                {port.project_id?.split("-")[0] || "—"}
                              </p>
                            </div>
                          ) : (
                            <span className="text-muted-foreground">—</span>
                          )}
                        </td>
                        <td className="px-4 py-2 text-right">
                          {clickable ? (
                            <span className="inline-flex items-center gap-1 text-xs font-medium text-brand">
                              Open
                              <ExternalLink className="h-3 w-3" />
                            </span>
                          ) : null}
                        </td>
                      </tr>
                    );
                  })}
                </tbody>
              </table>
            </div>

            <div className="flex shrink-0 items-center justify-between gap-2 border-t border-border/60 px-3 py-2 sm:px-4">
              <p className="text-[11px] text-muted-foreground tabular-nums">
                {filtered.length === 0
                  ? "0"
                  : `${safePage * pageSize + 1}–${Math.min(
                      (safePage + 1) * pageSize,
                      filtered.length,
                    )}`}{" "}
                of {filtered.length}
              </p>
              <div className="flex items-center gap-1.5">
                <Button
                  type="button"
                  variant="outline"
                  size="sm"
                  disabled={safePage <= 0}
                  onClick={() => setPage((p) => Math.max(0, p - 1))}
                  className="h-8 gap-1 border-border/60 px-2"
                >
                  <ChevronLeft className="h-3.5 w-3.5" />
                  <span className="hidden sm:inline">Prev</span>
                </Button>
                <span className="min-w-[3.5rem] text-center text-[11px] tabular-nums text-muted-foreground">
                  {safePage + 1}/{pageCount}
                </span>
                <Button
                  type="button"
                  variant="outline"
                  size="sm"
                  disabled={safePage >= pageCount - 1}
                  onClick={() =>
                    setPage((p) => Math.min(pageCount - 1, p + 1))
                  }
                  className="h-8 gap-1 border-border/60 px-2"
                >
                  <span className="hidden sm:inline">Next</span>
                  <ChevronRight className="h-3.5 w-3.5" />
                </Button>
              </div>
            </div>
          </>
        )}
      </div>
    </div>
  );
}
