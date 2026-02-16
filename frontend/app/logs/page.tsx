// Logs page - system container logs (backend, frontend, proxy, nginx)
"use client";

import { useState } from "react";
import { Header } from "@/components/Header";
import { Footer } from "@/components/Footer";
import { SystemLogsPanel } from "@/components/SystemLogsPanel";
import { ScrollText, Server, Globe, Shield, Network } from "lucide-react";
import { cn } from "@/lib/utils";

const SERVICES = [
  { id: "backend", label: "Backend", icon: Server, description: "API & business logic" },
  { id: "frontend", label: "Frontend", icon: Globe, description: "Next.js dashboard" },
  { id: "proxy", label: "Nginx Proxy", icon: Shield, description: "Reverse proxy & domains" },
  { id: "nginx", label: "Nginx", icon: Network, description: "Static gateway" },
] as const;

export default function LogsPage() {
  const [activeService, setActiveService] = useState<string>("backend");

  return (
    <div className="min-h-screen flex flex-col">
      <Header />

      <main className="flex-1 container max-w-7xl mx-auto px-4 md:px-6 py-8">
        <div className="mb-8">
          <h1 className="text-3xl font-bold flex items-center gap-3">
            <div className="p-2 rounded-xl bg-primary/10 border border-primary/20">
              <ScrollText className="h-6 w-6 text-primary" />
            </div>
            System Logs
          </h1>
          <p className="text-muted-foreground mt-2 text-sm max-w-lg">
            Real-time log streaming for all Docklift services. Select a service to view its live output.
          </p>
        </div>

        {/* Service tabs */}
        <div className="flex flex-wrap gap-2 mb-6">
          {SERVICES.map(({ id, label, icon: Icon, description }) => (
            <button
              key={id}
              type="button"
              onClick={() => setActiveService(id)}
              className={cn(
                "group flex items-center gap-2.5 px-4 py-2.5 rounded-xl text-sm font-medium transition-all duration-200 border",
                activeService === id
                  ? "bg-primary/10 text-primary border-primary/30 shadow-sm shadow-primary/10"
                  : "bg-card/50 text-muted-foreground border-border/50 hover:bg-card hover:text-foreground hover:border-border",
              )}
            >
              <Icon className={cn(
                "h-4 w-4 transition-colors",
                activeService === id ? "text-primary" : "text-muted-foreground/60 group-hover:text-foreground"
              )} />
              <div className="text-left">
                <div className="font-semibold">{label}</div>
                <div className={cn(
                  "text-[10px] leading-tight hidden sm:block",
                  activeService === id ? "text-primary/70" : "text-muted-foreground/50"
                )}>{description}</div>
              </div>
            </button>
          ))}
        </div>

        <SystemLogsPanel key={activeService} service={activeService} isActive={true} />
      </main>

      <Footer />
    </div>
  );
}
