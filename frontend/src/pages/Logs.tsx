// Logs page — one-viewport layout; log stream fills remaining height.

import { useState } from "react";
import { SystemLogsPanel } from "@/components/SystemLogsPanel";
import {
  ScrollText,
  Server,
  Globe,
  Shield,
  Network,
  LockKeyhole,
} from "lucide-react";
import { cn } from "@/lib/utils";

// One entry per container in docker-compose.yml — `container` must match the name
// the backend maps in GET /api/system/logs/:service.
const SERVICES = [
  {
    id: "backend",
    label: "Backend",
    short: "API",
    icon: Server,
    description: "API & business logic",
    container: "docklift-backend",
  },
  {
    id: "frontend",
    label: "Frontend",
    short: "UI",
    icon: Globe,
    description: "Vite SPA dashboard",
    container: "docklift-frontend",
  },
  {
    id: "proxy",
    label: "Public Proxy",
    short: "Proxy",
    icon: Shield,
    description: "Domains + HTTPS :80/:443",
    container: "docklift-nginx-proxy",
  },
  {
    id: "nginx",
    label: "Dashboard Gateway",
    short: "Gate",
    icon: Network,
    description: "Panel on :8080",
    container: "docklift-nginx",
  },
  {
    id: "certbot",
    label: "Certbot",
    short: "TLS",
    icon: LockKeyhole,
    description: "Cert renewals (12h)",
    container: "docklift-certbot",
  },
] as const;

export default function LogsPage() {
  const [activeService, setActiveService] = useState<string>("backend");
  const active =
    SERVICES.find((service) => service.id === activeService) ?? SERVICES[0];

  return (
    <div
      className={cn(
        "flex flex-col",
        "h-[calc(100dvh-var(--shell-topbar)-2.5rem)]",
        "sm:h-[calc(100dvh-var(--shell-topbar)-4rem)]",
      )}
    >
      {/* Compact header */}
      <div className="mb-2 flex shrink-0 items-center gap-2.5 sm:mb-3 sm:gap-3">
        <span className="flex h-9 w-9 shrink-0 items-center justify-center rounded-xl border border-brand/20 bg-brand/10 sm:h-10 sm:w-10 sm:rounded-2xl">
          <ScrollText className="h-4 w-4 text-brand sm:h-[1.125rem] sm:w-[1.125rem]" />
        </span>
        <div className="min-w-0">
          <h1 className="truncate text-lg font-bold tracking-tight sm:text-xl">
            System Logs
          </h1>
          <p className="truncate text-[11px] text-muted-foreground sm:text-xs">
            Live streams from Docklift control-plane containers
          </p>
        </div>
      </div>

      {/* Service picker — horizontal scroll on small screens */}
      <div
        className="-mx-3 mb-2 flex shrink-0 gap-1.5 overflow-x-auto px-3 pb-0.5 [-ms-overflow-style:none] [scrollbar-width:none] sm:mx-0 sm:mb-3 sm:flex-wrap sm:gap-2 sm:overflow-visible sm:px-0 [&::-webkit-scrollbar]:hidden"
        role="tablist"
        aria-label="Docklift services"
      >
        {SERVICES.map(({ id, label, short, icon: Icon, description }) => {
          const selected = activeService === id;
          return (
            <button
              key={id}
              type="button"
              role="tab"
              aria-selected={selected}
              onClick={() => setActiveService(id)}
              className={cn(
                "group flex shrink-0 items-center gap-2 rounded-xl border px-2.5 py-1.5 text-left text-sm font-medium transition-colors sm:gap-2.5 sm:px-3 sm:py-2",
                selected
                  ? "border-brand/30 bg-brand/10 text-brand"
                  : "border-border/60 bg-card/50 text-muted-foreground hover:border-border hover:bg-card hover:text-foreground",
              )}
            >
              <Icon
                className={cn(
                  "h-3.5 w-3.5 shrink-0 sm:h-4 sm:w-4",
                  selected
                    ? "text-brand"
                    : "text-muted-foreground/60 group-hover:text-foreground",
                )}
              />
              <span className="sm:hidden text-xs font-semibold">{short}</span>
              <span className="hidden min-w-0 sm:block">
                <span className="block text-sm font-semibold leading-tight">
                  {label}
                </span>
                <span
                  className={cn(
                    "block text-[10px] leading-tight",
                    selected ? "text-brand/70" : "text-muted-foreground/60",
                  )}
                >
                  {description}
                </span>
              </span>
            </button>
          );
        })}
      </div>

      {/* Log viewer fills the rest of the viewport */}
      <div className="min-h-0 flex-1 overflow-hidden rounded-2xl border border-border/60">
        <SystemLogsPanel
          key={active.id}
          service={active.id}
          label={active.label}
          container={active.container}
          isActive
          height="h-full"
          className="h-full"
        />
      </div>
    </div>
  );
}
