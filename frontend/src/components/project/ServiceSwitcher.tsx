// Project vs service scope rail for multi-Dockerfile projects.
// Single-service projects omit this — tabs stay flat.

import { useCallback, useEffect, useRef, useState } from "react";
import { cn } from "@/lib/utils";
import type { Service } from "@/lib/types";
import { ChevronLeft, ChevronRight } from "lucide-react";

export type ProjectWorkspace = "project" | "service";

const GUTTER_PX = 40;

const STATUS_DOT: Record<string, string> = {
  running: "bg-ok",
  building: "bg-warn animate-pulse motion-reduce:animate-none",
  degraded: "bg-warn",
  error: "bg-danger",
  stopped: "bg-subtle",
  pending: "bg-info",
};

interface ServiceSwitcherProps {
  services: Service[];
  workspace: ProjectWorkspace;
  selectedId: string | null;
  onSelectProject: () => void;
  onSelectService: (serviceId: string) => void;
  className?: string;
}

export function ServiceSwitcher({
  services,
  workspace,
  selectedId,
  onSelectProject,
  onSelectService,
  className,
}: ServiceSwitcherProps) {
  const scrollerRef = useRef<HTMLDivElement>(null);
  const [hasOverflow, setHasOverflow] = useState(false);
  const [canScrollLeft, setCanScrollLeft] = useState(false);
  const [canScrollRight, setCanScrollRight] = useState(false);

  const updateScrollAffordance = useCallback(() => {
    const el = scrollerRef.current;
    if (!el) return;
    const max = el.scrollWidth - el.clientWidth;
    const overflow = max > 4;
    setHasOverflow(overflow);
    setCanScrollLeft(overflow && el.scrollLeft > 4);
    setCanScrollRight(overflow && el.scrollLeft < max - 4);
  }, []);

  const scrollActiveIntoView = useCallback(() => {
    const el = scrollerRef.current;
    if (!el) return;
    const active = el.querySelector<HTMLElement>('[data-active="true"]');
    if (!active) return;
    const left = active.offsetLeft - GUTTER_PX;
    const right = active.offsetLeft + active.offsetWidth + GUTTER_PX;
    const viewLeft = el.scrollLeft;
    const viewRight = el.scrollLeft + el.clientWidth;
    let next = el.scrollLeft;
    if (left < viewLeft) next = left;
    else if (right > viewRight) next = right - el.clientWidth;
    const max = Math.max(0, el.scrollWidth - el.clientWidth);
    el.scrollTo({
      left: Math.max(0, Math.min(max, next)),
      behavior: "smooth",
    });
  }, []);

  useEffect(() => {
    const el = scrollerRef.current;
    if (!el) return;
    updateScrollAffordance();
    el.addEventListener("scroll", updateScrollAffordance, { passive: true });
    const ro = new ResizeObserver(updateScrollAffordance);
    ro.observe(el);
    for (const child of Array.from(el.children)) {
      if (child instanceof HTMLElement) ro.observe(child);
    }
    return () => {
      el.removeEventListener("scroll", updateScrollAffordance);
      ro.disconnect();
    };
  }, [services, updateScrollAffordance]);

  useEffect(() => {
    updateScrollAffordance();
    const id = requestAnimationFrame(() => {
      scrollActiveIntoView();
      updateScrollAffordance();
    });
    return () => cancelAnimationFrame(id);
  }, [workspace, selectedId, services, scrollActiveIntoView, updateScrollAffordance]);

  const scrollByAmount = (dir: -1 | 1) => {
    const el = scrollerRef.current;
    if (!el) return;
    el.scrollBy({
      left: dir * Math.max(160, el.clientWidth * 0.55),
      behavior: "smooth",
    });
  };

  if (services.length <= 1) return null;

  const itemBase =
    "inline-flex h-7 shrink-0 items-center gap-2 rounded-[4px] px-2.5 text-[13px] font-medium transition-colors duration-150 focus-visible:outline-2 focus-visible:-outline-offset-2 focus-visible:outline-ring pointer-coarse:h-9";

  return (
    <div className={cn("flex min-w-0 flex-col gap-2", className)}>
      <div className="flex min-w-0 flex-col gap-2 sm:flex-row sm:items-center sm:gap-3">
        <span className="shrink-0 text-[13px] text-muted-foreground">
          Workspace
        </span>

        <div className="relative min-w-0 max-w-full">
          {hasOverflow && (
            <>
              <button
                type="button"
                aria-label="Scroll workspace left"
                disabled={!canScrollLeft}
                tabIndex={canScrollLeft ? 0 : -1}
                onClick={() => scrollByAmount(-1)}
                className={cn(
                  "absolute left-0 top-1/2 z-10 hidden size-7 -translate-y-1/2 items-center justify-center rounded-md border border-input bg-card text-muted-foreground transition-opacity duration-150 hover:bg-secondary hover:text-foreground disabled:pointer-events-none sm:flex",
                  canScrollLeft ? "opacity-100" : "opacity-0",
                )}
              >
                <ChevronLeft className="size-4" />
              </button>
              <button
                type="button"
                aria-label="Scroll workspace right"
                disabled={!canScrollRight}
                tabIndex={canScrollRight ? 0 : -1}
                onClick={() => scrollByAmount(1)}
                className={cn(
                  "absolute right-0 top-1/2 z-10 hidden size-7 -translate-y-1/2 items-center justify-center rounded-md border border-input bg-card text-muted-foreground transition-opacity duration-150 hover:bg-secondary hover:text-foreground disabled:pointer-events-none sm:flex",
                  canScrollRight ? "opacity-100" : "opacity-0",
                )}
              >
                <ChevronRight className="size-4" />
              </button>
            </>
          )}

          <div
            ref={scrollerRef}
            role="group"
            aria-label="All services or one service workspace"
            className={cn(
              "scrollbar-none flex max-w-full gap-0.5 overflow-x-auto scroll-smooth rounded-md border border-input bg-card p-0.5",
              hasOverflow ? "sm:mx-9" : "",
            )}
          >
            <button
              type="button"
              data-active={workspace === "project" ? "true" : "false"}
              aria-pressed={workspace === "project"}
              onClick={onSelectProject}
              className={cn(
                itemBase,
                workspace === "project"
                  ? "bg-secondary text-foreground"
                  : "text-muted-foreground hover:text-foreground",
              )}
            >
              All services
              <span className="font-mono text-xs text-subtle">
                {services.length}
              </span>
            </button>

            {services.map((svc) => {
              const active = workspace === "service" && svc.id === selectedId;
              const status = svc.status || "pending";
              return (
                <button
                  key={svc.id}
                  type="button"
                  data-active={active ? "true" : "false"}
                  aria-pressed={active}
                  onClick={() => onSelectService(svc.id)}
                  className={cn(
                    itemBase,
                    "max-w-48 sm:max-w-64",
                    active
                      ? "bg-secondary text-foreground"
                      : "text-muted-foreground hover:text-foreground",
                  )}
                >
                  <span
                    aria-hidden="true"
                    title={status}
                    className={cn(
                      "size-2 shrink-0 rounded-full",
                      STATUS_DOT[status] || STATUS_DOT.pending,
                    )}
                  />
                  <span className="min-w-0 truncate">{svc.name}</span>
                </button>
              );
            })}
          </div>
        </div>
      </div>
      <p className="text-xs text-muted-foreground">
        <span className="sm:hidden">All services or one app.</span>
        <span className="hidden sm:inline">
          All services covers deploy, build, and shared env. Open one service
          for that app’s env, domains, storage, and runtime logs.
        </span>
      </p>
    </div>
  );
}
