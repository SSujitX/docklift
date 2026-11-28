// Logs page — one-viewport layout; log stream fills remaining height.

import { useState } from "react";
import { SystemLogsPanel } from "@/components/SystemLogsPanel";
import { PageHeader } from "@/components/shell/PageHeader";
import { cn } from "@/lib/utils";

// One entry per container in docker-compose.yml — `container` must match the name
// the backend maps in GET /api/system/logs/:service.
const SERVICES = [
  {
    id: "backend",
    label: "Backend",
    short: "API",
    description: "API and business logic",
    container: "docklift-backend",
  },
  {
    id: "proxy",
    label: "Public proxy",
    short: "Proxy",
    description: "Domains and HTTPS on :80 and :443",
    container: "docklift-nginx-proxy",
  },
  {
    id: "certbot",
    label: "Certbot",
    short: "TLS",
    description: "Certificate renewals every 12 hours",
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
        // main has py-6 (3rem total) under the 48px top bar
        "h-[calc(100dvh-var(--shell-topbar)-3rem)] min-h-[28rem]",
      )}
    >
      <PageHeader
        title="Logs"
        description="Live output from the Docklift control-plane containers."
        className="mb-4 shrink-0"
      />

      {/* Toolbar: source tabs + what the selected source is */}
      <div className="mb-3 flex shrink-0 flex-col gap-x-6 gap-y-2 sm:hairline-b sm:flex-row sm:items-center sm:justify-between">
        <div
          className="hairline-b flex gap-5 overflow-x-auto scrollbar-none sm:shadow-none"
          role="tablist"
          aria-label="Docklift services"
        >
          {SERVICES.map(({ id, label }) => {
            const selected = activeService === id;
            return (
              <button
                key={id}
                type="button"
                role="tab"
                aria-selected={selected}
                onClick={() => setActiveService(id)}
                className={cn(
                  "flex h-10 shrink-0 items-center whitespace-nowrap border-b-2 text-[13.5px] font-medium transition-colors duration-150",
                  selected
                    ? "border-brand text-foreground"
                    : "border-transparent text-muted-foreground hover:text-foreground",
                )}
              >
                {label}
              </button>
            );
          })}
        </div>
        <p className="min-w-0 truncate pb-0 text-[13px] leading-5 text-muted-foreground">
          {active.description}
          <span className="text-subtle"> · </span>
          <span className="font-mono text-xs">{active.container}</span>
        </p>
      </div>

      {/* Log viewer fills the rest of the viewport; LogViewer draws its own box */}
      <div className="min-h-0 flex-1">
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
