// Logs page - system container logs (backend, frontend, proxy, nginx)

import { useState } from "react";
import { PageHeader } from "@/components/shell/PageHeader";
import { SystemLogsPanel } from "@/components/SystemLogsPanel";
import { ScrollText, Server, Globe, Shield, Network } from "lucide-react";
import { cn } from "@/lib/utils";

const SERVICES = [
  { id: "backend", label: "Backend", icon: Server, description: "API & business logic" },
  { id: "frontend", label: "Frontend", icon: Globe, description: "Vite SPA dashboard" },
  { id: "proxy", label: "Nginx Proxy", icon: Shield, description: "Reverse proxy & domains" },
  { id: "nginx", label: "Nginx", icon: Network, description: "Static gateway" },
] as const;

export default function LogsPage() {
  const [activeService, setActiveService] = useState<string>("backend");

  return (
    <>
      <PageHeader
        eyebrow="Operate"
        title="System Logs"
        description="Real-time log streaming for every Docklift service. Pick a service to follow its output."
        icon={ScrollText}
      />

      {/* Service tabs */}
      <div className="mb-6 flex flex-wrap gap-2">
        {SERVICES.map(({ id, label, icon: Icon, description }) => (
          <button
            key={id}
            type="button"
            onClick={() => setActiveService(id)}
            className={cn(
              "group flex items-center gap-2.5 rounded-xl border px-4 py-2.5 text-sm font-medium transition-all duration-200",
              activeService === id
                ? "border-brand/30 bg-brand/10 text-brand shadow-sm shadow-brand/10"
                : "border-border/50 bg-card/50 text-muted-foreground hover:border-border hover:bg-card hover:text-foreground",
            )}
          >
            <Icon className={cn(
              "h-4 w-4 transition-colors",
              activeService === id ? "text-brand" : "text-muted-foreground/60 group-hover:text-foreground"
            )} />
            <div className="text-left">
              <div className="font-semibold">{label}</div>
              <div className={cn(
                "hidden text-[10px] leading-tight sm:block",
                activeService === id ? "text-brand/70" : "text-muted-foreground/50"
              )}>{description}</div>
            </div>
          </button>
        ))}
      </div>

      <SystemLogsPanel key={activeService} service={activeService} isActive={true} />
    </>
  );
}
