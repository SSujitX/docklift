// Project vs service scope rail for multi-Dockerfile projects.
// Single-service projects omit this — tabs stay flat.

import { useCallback, useEffect, useRef, useState } from "react";
import { cn } from "@/lib/utils";
import type { Service } from "@/lib/types";
import { StatusBadge } from "@/components/StatusBadge";
import { ChevronLeft, ChevronRight, Layers } from "lucide-react";

export type ProjectWorkspace = "project" | "service";

const GUTTER_PX = 40;

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
    // Content width can grow without the scroller box resizing
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
      left: dir * Math.max(200, el.clientWidth * 0.6),
      behavior: "smooth",
    });
  };

  if (services.length <= 1) return null;

  const chipBase =
    "inline-flex shrink-0 items-center gap-2 rounded-xl border px-3 py-2.5 text-left transition-colors focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring";

  return (
    <div className={cn("flex flex-col gap-3", className)}>
      <div className="min-w-0">
        <p className="text-[11px] font-medium uppercase tracking-[0.12em] text-muted-foreground">
          Workspace
        </p>
        <p className="mt-0.5 text-xs text-muted-foreground">
          All services covers deploy, build, and shared env. Open one service for
          that app’s env, domains, storage, and runtime logs.
        </p>
      </div>

      <div className="relative">
        <button
          type="button"
          aria-label="Scroll workspace left"
          disabled={!canScrollLeft}
          tabIndex={canScrollLeft ? 0 : -1}
          onClick={() => scrollByAmount(-1)}
          className={cn(
            "absolute left-0 top-1/2 z-10 flex h-8 w-8 -translate-y-1/2 items-center justify-center rounded-full border border-border bg-background/95 text-foreground shadow-md backdrop-blur-sm transition-opacity hover:bg-secondary disabled:pointer-events-none",
            canScrollLeft ? "opacity-100" : "opacity-0",
          )}
        >
          <ChevronLeft className="h-4 w-4" />
        </button>
        <button
          type="button"
          aria-label="Scroll workspace right"
          disabled={!canScrollRight}
          tabIndex={canScrollRight ? 0 : -1}
          onClick={() => scrollByAmount(1)}
          className={cn(
            "absolute right-0 top-1/2 z-10 flex h-8 w-8 -translate-y-1/2 items-center justify-center rounded-full border border-border bg-background/95 text-foreground shadow-md backdrop-blur-sm transition-opacity hover:bg-secondary disabled:pointer-events-none",
            canScrollRight ? "opacity-100" : "opacity-0",
          )}
        >
          <ChevronRight className="h-4 w-4" />
        </button>

        <div
          ref={scrollerRef}
          role="group"
          aria-label="All services or one service workspace"
          className={cn(
            "flex max-w-full gap-2 overflow-x-auto scroll-smooth py-1",
            "custom-scrollbar [scrollbar-width:thin]",
            hasOverflow ? "px-10" : "px-0.5",
          )}
        >
          <button
            type="button"
            data-active={workspace === "project" ? "true" : "false"}
            aria-pressed={workspace === "project"}
            onClick={onSelectProject}
            className={cn(
              chipBase,
              workspace === "project"
                ? "border-brand/50 bg-brand/10 text-foreground shadow-sm ring-2 ring-brand/25"
                : "border-border/60 bg-secondary/40 text-muted-foreground hover:border-border hover:bg-secondary hover:text-foreground",
            )}
          >
            <Layers className="h-3.5 w-3.5 shrink-0" />
            <span className="text-xs font-semibold">All services</span>
            <span
              className={cn(
                "rounded-md px-1.5 py-0.5 text-[10px] font-medium tabular-nums",
                workspace === "project"
                  ? "bg-brand/15 text-foreground"
                  : "bg-background/80 text-muted-foreground",
              )}
            >
              {services.length}
            </span>
          </button>

          {services.map((svc) => {
            const active =
              workspace === "service" && svc.id === selectedId;
            return (
              <button
                key={svc.id}
                type="button"
                data-active={active ? "true" : "false"}
                aria-pressed={active}
                onClick={() => onSelectService(svc.id)}
                className={cn(
                  chipBase,
                  "min-w-[10.5rem] max-w-[16rem]",
                  active
                    ? "border-brand/50 bg-brand/10 text-foreground shadow-sm ring-2 ring-brand/25"
                    : "border-border/60 bg-secondary/40 text-muted-foreground hover:border-border hover:bg-secondary hover:text-foreground",
                )}
              >
                <span className="min-w-0 flex-1 truncate text-xs font-semibold">
                  {svc.name}
                </span>
                <StatusBadge status={svc.status || "pending"} size="sm" />
              </button>
            );
          })}
        </div>
      </div>
    </div>
  );
}
