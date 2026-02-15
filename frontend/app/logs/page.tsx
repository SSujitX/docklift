// Logs page - system container logs (backend, frontend, proxy, etc.)
"use client";

import { useState } from "react";
import { Header } from "@/components/Header";
import { Footer } from "@/components/Footer";
import { SystemLogsPanel } from "@/components/SystemLogsPanel";
import { ScrollText, Server, Globe, Database } from "lucide-react";
import { cn } from "@/lib/utils";

const SERVICES = [
  { id: "backend", label: "Backend", icon: Server },
  { id: "frontend", label: "Frontend", icon: Globe },
  { id: "proxy", label: "Nginx Proxy", icon: Globe },
  { id: "database", label: "Database", icon: Database },
] as const;

export default function LogsPage() {
  const [activeService, setActiveService] = useState<string>("backend");

  return (
    <div className="min-h-screen flex flex-col">
      <Header />

      <main className="flex-1 container max-w-7xl mx-auto px-4 md:px-6 py-8">
        <div className="mb-6">
          <h1 className="text-2xl font-bold flex items-center gap-2">
            <ScrollText className="h-7 w-7 text-primary" />
            System Logs
          </h1>
          <p className="text-muted-foreground mt-1 text-sm">
            Real-time logs for Docklift services. Select a service below.
          </p>
        </div>

        {/* Service tabs */}
        <div className="flex flex-wrap gap-2 mb-6 border-b border-border/40 pb-4">
          {SERVICES.map(({ id, label, icon: Icon }) => (
            <button
              key={id}
              type="button"
              onClick={() => setActiveService(id)}
              className={cn(
                "flex items-center gap-2 px-4 py-2 rounded-xl text-sm font-medium transition-colors",
                activeService === id
                  ? "bg-primary text-primary-foreground"
                  : "bg-secondary/50 text-muted-foreground hover:bg-secondary hover:text-foreground",
              )}
            >
              <Icon className="h-4 w-4" />
              {label}
            </button>
          ))}
        </div>

        <SystemLogsPanel service={activeService} isActive={true} />
      </main>

      <Footer />
    </div>
  );
}
