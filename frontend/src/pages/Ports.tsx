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
import { PageHeader } from "@/components/shell/PageHeader";
import { Button } from "@/components/ui/button";
import { Port } from "@/lib/types";
import { cn } from "@/lib/utils";
import { apiCache } from "@/lib/auth";
import { API } from "@/lib/prefetch";
import { ChevronLeft, ChevronRight, RefreshCw } from "lucide-react";

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

/** New shape: { ports, private_running } — still accept a bare array. */
function parsePorts(data: any): { ports: Port[]; privateRunning: PrivateRunning[] } {
  if (Array.isArray(data)) return { ports: data, privateRunning: [] };
  return {
    ports: Array.isArray(data?.ports) ? data.ports : [],
    privateRunning: Array.isArray(data?.private_running) ? data.private_running : [],
  };
}

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
  // Last known data draws at once on a revisit; the mount fetch below refreshes it.
  const [cached] = useState(() => {
    const data = apiCache.peek(API.ports);
    return data === undefined ? null : parsePorts(data);
  });
  const [ports, setPorts] = useState<Port[]>(cached?.ports ?? []);
  const [privateRunning, setPrivateRunning] = useState<PrivateRunning[]>(
    cached?.privateRunning ?? [],
  );
  const [loading, setLoading] = useState(cached === null);
  const [error, setError] = useState<string | null>(null);
  const [filter, setFilter] = useState<PortFilter>("all");
  const [page, setPage] = useState(0);
  const [pageSize, setPageSize] = useState(12);

  const fetchPorts = useCallback(async () => {
    try {
      const res = await apiCache.fetch(API.ports);
      if (!res.ok) {
        const data = await res.json().catch(() => ({}));
        throw new Error(
          (data as { error?: string }).error ||
            `Failed to load ports (${res.status})`,
        );
      }
      const parsed = parsePorts(await res.json());
      setPorts(parsed.ports);
      setPrivateRunning(parsed.privateRunning);
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
      <PageHeader
        title="Ports"
        description={`Opt-in host port pool ${poolRange}. Private apps and databases don't appear here.`}
        className="mb-4 shrink-0"
        actions={
          <Button
            variant="outline"
            size="icon"
            onClick={() => {
              setLoading(true);
              void fetchPorts();
            }}
            title="Refresh ports"
          >
            <RefreshCw className="text-subtle" />
          </Button>
        }
      />

      {/* Running without a host port — explains an allocated count of 0 */}
      {!loading && privateRunning.length > 0 && (
        <div className="mb-4 shrink-0 rounded-md border border-info/40 bg-secondary/60 px-3 py-2.5 text-[13px] leading-5">
          <p className="font-medium text-foreground">
            {privateRunning.length} running without a host port
          </p>
          <p className="text-muted-foreground">
            Host ports are opt-in. Databases stay private (link over Docker DNS).
            Apps need{" "}
            <span className="text-foreground">Build → Publish host ports</span> +
            redeploy, or a domain.
          </p>
          <div className="mt-1.5 flex flex-wrap gap-x-4 gap-y-1">
            {privateRunning.map((p) => (
              <button
                key={p.id}
                type="button"
                title={p.reason}
                onClick={() => navigate(`/projects/${p.id}`)}
                className="inline-flex max-w-full items-center gap-1.5 rounded-sm text-[13px] text-foreground underline-offset-4 transition-colors duration-150 hover:underline pointer-coarse:min-h-10"
              >
                <span
                  aria-hidden="true"
                  className={cn(
                    "size-1.5 shrink-0 rounded-full",
                    p.status === "running"
                      ? "bg-ok"
                      : p.status === "building" || p.status === "pending"
                        ? "animate-pulse bg-warn motion-reduce:animate-none"
                        : "bg-warn",
                  )}
                />
                <span className="truncate">{p.name}</span>
                <span className="shrink-0 font-mono text-xs text-subtle">
                  {p.project_type === "database" ? "db" : "app"}
                </span>
              </button>
            ))}
          </div>
        </div>
      )}

      {/* Filters + pool meta — one row */}
      <div className="hairline-b mb-4 flex shrink-0 items-center gap-4">
        <div
          className="flex min-w-0 flex-1 gap-5 overflow-x-auto scrollbar-none"
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
                  "inline-flex h-10 shrink-0 items-center gap-1.5 border-b-2 text-[13.5px] transition-colors duration-150 focus-visible:-outline-offset-2",
                  active
                    ? "border-brand font-medium text-foreground"
                    : "border-transparent text-muted-foreground hover:text-foreground",
                )}
              >
                {tab.label}
                <span className="font-mono text-[13px] tabular-nums text-subtle">
                  {tab.count}
                </span>
              </button>
            );
          })}
        </div>
        <span className="hidden shrink-0 font-mono text-xs text-subtle tabular-nums sm:inline">
          {ports.length} · docklift_network
        </span>
      </div>

      {error && (
        <div className="mb-4 shrink-0 rounded-md border border-danger/40 bg-secondary/60 px-3 py-2.5 text-[13px] text-foreground">
          {error}
        </div>
      )}

      {/* Fill remaining height — scroll only here */}
      <div className="flex min-h-0 flex-1 flex-col overflow-hidden rounded-lg border border-border bg-card">
        {loading ? (
          <div className="divide-y divide-border overflow-hidden" aria-hidden="true">
            {[1, 2, 3, 4, 5, 6].map((i) => (
              <div key={i} className="flex h-11 items-center px-4">
                <div className="h-2.5 w-16 rounded-sm bg-secondary" />
              </div>
            ))}
          </div>
        ) : ports.length === 0 ? (
          <div className="flex flex-1 flex-col items-center justify-center px-4 py-14 text-center">
            <p className="text-[15px] font-semibold">No ports in the pool</p>
            <p className="mx-auto mt-1 max-w-sm text-[13.5px] text-muted-foreground">
              Filled from the configured host port range at startup.
            </p>
          </div>
        ) : filtered.length === 0 ? (
          <div className="flex flex-1 flex-col items-center justify-center px-4 py-14 text-center">
            <p className="text-[13.5px] text-muted-foreground">
              No ports match this filter.
            </p>
            <Button
              type="button"
              variant="link"
              onClick={() => setFilter("all")}
              className="mt-1 h-auto px-0 text-[13.5px]"
            >
              Show all ports
            </Button>
          </div>
        ) : (
          <>
            <div
              ref={listRef}
              className="shell-scroll min-h-0 flex-1 overflow-hidden overscroll-contain"
            >
              {/* Mobile */}
              <div className="divide-y divide-border md:hidden">
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
                        "flex h-13 items-center gap-3 px-4",
                        clickable &&
                          "cursor-pointer transition-colors duration-150 hover:bg-secondary/50 focus-visible:bg-secondary/50 focus-visible:-outline-offset-2",
                      )}
                    >
                      <span
                        className={cn(
                          "w-14 shrink-0 font-mono text-sm tabular-nums",
                          allocated
                            ? "font-medium text-foreground"
                            : "text-muted-foreground",
                        )}
                      >
                        :{port.port}
                      </span>
                      <div className="min-w-0 flex-1">
                        {allocated ? (
                          <>
                            <p className="truncate text-sm font-medium leading-5">
                              {port.project?.name || "Unknown project"}
                            </p>
                            <p className="truncate font-mono text-xs leading-4 text-muted-foreground">
                              in use
                              {port.project?.status && (
                                <>
                                  <span className="text-subtle"> · </span>
                                  {port.project.status}
                                </>
                              )}
                            </p>
                          </>
                        ) : (
                          <p className="text-[13px] text-subtle">
                            Free for assignment
                          </p>
                        )}
                      </div>
                      {clickable && (
                        <ChevronRight className="size-4 shrink-0 text-subtle" />
                      )}
                    </article>
                  );
                })}
              </div>

              {/* Desktop */}
              <table className="hidden w-full text-left text-sm md:table">
                <thead className="sticky top-0 z-10 bg-card">
                  <tr className="h-9 border-b border-border text-xs text-muted-foreground">
                    <th className="w-32 px-4 font-medium">Port</th>
                    <th className="w-36 px-4 font-medium">Status</th>
                    <th className="px-4 font-medium">Project</th>
                    <th className="px-4 text-right font-medium">
                      <span className="sr-only">Open</span>
                    </th>
                  </tr>
                </thead>
                <tbody className="divide-y divide-border">
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
                          "group h-11",
                          clickable &&
                            "cursor-pointer transition-colors duration-150 hover:bg-secondary/50 focus-visible:bg-secondary/50 focus-visible:-outline-offset-2",
                        )}
                      >
                        <td className="px-4">
                          <span
                            className={cn(
                              "font-mono text-sm tabular-nums",
                              allocated
                                ? "font-medium text-foreground"
                                : "text-muted-foreground",
                            )}
                          >
                            :{port.port}
                          </span>
                        </td>
                        <td className="px-4">
                          {allocated ? (
                            <span className="inline-flex items-center gap-2 text-[13px] font-medium text-foreground">
                              <span
                                aria-hidden="true"
                                className="size-2 rounded-full bg-foreground/70"
                              />
                              Allocated
                            </span>
                          ) : (
                            <span className="inline-flex items-center gap-2 text-[13px] text-subtle">
                              <span
                                aria-hidden="true"
                                className="size-2 rounded-full border border-subtle/60"
                              />
                              Available
                            </span>
                          )}
                        </td>
                        <td className="px-4">
                          {allocated ? (
                            <div className="min-w-0">
                              <p className="truncate text-sm font-medium leading-5">
                                {port.project?.name || "Unknown project"}
                              </p>
                              <p className="truncate font-mono text-xs leading-4 text-muted-foreground">
                                {port.project_id?.split("-")[0] || "—"}
                                {port.project?.status && (
                                  <>
                                    <span className="text-subtle"> · </span>
                                    {port.project.status}
                                  </>
                                )}
                              </p>
                            </div>
                          ) : (
                            <span className="text-subtle">—</span>
                          )}
                        </td>
                        <td className="px-4 text-right">
                          {clickable ? (
                            <span className="inline-flex items-center gap-1 text-[13px] text-muted-foreground transition-colors duration-150 group-hover:text-foreground group-focus-visible:text-foreground">
                              Open
                              <ChevronRight className="size-4 text-subtle" />
                            </span>
                          ) : null}
                        </td>
                      </tr>
                    );
                  })}
                </tbody>
              </table>
            </div>

            <div className="flex shrink-0 items-center justify-between gap-2 border-t border-border px-4 py-2">
              <p className="text-[13px] text-muted-foreground">
                <span className="font-mono text-foreground tabular-nums">
                  {filtered.length === 0
                    ? "0"
                    : `${safePage * pageSize + 1}–${Math.min(
                        (safePage + 1) * pageSize,
                        filtered.length,
                      )}`}
                </span>{" "}
                of{" "}
                <span className="font-mono text-foreground tabular-nums">
                  {filtered.length}
                </span>
              </p>
              <div className="flex items-center gap-1">
                <Button
                  type="button"
                  variant="ghost"
                  size="sm"
                  disabled={safePage <= 0}
                  onClick={() => setPage((p) => Math.max(0, p - 1))}
                  className="gap-1 px-2"
                >
                  <ChevronLeft />
                  <span className="hidden sm:inline">Prev</span>
                </Button>
                <span className="min-w-12 text-center font-mono text-xs text-muted-foreground tabular-nums">
                  {safePage + 1} / {pageCount}
                </span>
                <Button
                  type="button"
                  variant="ghost"
                  size="sm"
                  disabled={safePage >= pageCount - 1}
                  onClick={() =>
                    setPage((p) => Math.min(pageCount - 1, p + 1))
                  }
                  className="gap-1 px-2"
                >
                  <span className="hidden sm:inline">Next</span>
                  <ChevronRight />
                </Button>
              </div>
            </div>
          </>
        )}
      </div>
    </div>
  );
}
