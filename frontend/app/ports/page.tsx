"use client";

import { useEffect, useState } from "react";
import { Header } from "@/components/Header";
import { Card } from "@/components/ui/card";
import { Port } from "@/lib/types";
import { API_URL } from "@/lib/utils";
import { cn } from "@/lib/utils";
import { Lock, Unlock, Network } from "lucide-react";

export default function PortsPage() {
  const [ports, setPorts] = useState<Port[]>([]);
  const [loading, setLoading] = useState(true);

  useEffect(() => {
    fetch(`${API_URL}/api/ports`)
      .then((res) => res.json())
      .then((data) => {
        setPorts(data);
        setLoading(false);
      })
      .catch(() => setLoading(false));
  }, []);

  const usedPorts = ports.filter((p) => p.is_locked);
  const freePorts = ports.filter((p) => !p.is_locked);

  return (
    <div className="min-h-screen">
      <Header />

      <main className="container max-w-7xl mx-auto px-4 md:px-6 py-8">
        <div className="mb-8">
          <h1 className="text-3xl font-bold tracking-tight">Port Management</h1>
          <p className="text-muted-foreground mt-1">
            <span className="text-amber-500">{usedPorts.length} allocated</span> · <span className="text-emerald-500">{freePorts.length} available</span>
          </p>
        </div>

        <div className="grid grid-cols-1 lg:grid-cols-2 gap-6">
          <Card className="p-6">
            <div className="flex items-center gap-3 mb-5">
              <div className="p-2.5 rounded-xl bg-amber-500/10">
                <Lock className="h-5 w-5 text-amber-500" />
              </div>
              <div>
                <h2 className="font-semibold">Used Ports</h2>
                <p className="text-sm text-muted-foreground">{usedPorts.length} active allocations</p>
              </div>
            </div>
            
            {loading ? (
              <div className="grid grid-cols-5 sm:grid-cols-8 gap-2">
                {[1, 2, 3, 4].map((i) => (
                  <div key={i} className="h-10 bg-secondary rounded-lg shimmer" />
                ))}
              </div>
            ) : usedPorts.length === 0 ? (
              <div className="text-center py-8 text-muted-foreground text-sm">No ports in use</div>
            ) : (
              <div className="grid grid-cols-1 sm:grid-cols-2 gap-3">
                {usedPorts.map((port) => (
                  <div
                    key={port.port}
                    className="group relative px-4 py-3 rounded-xl bg-amber-500/10 border border-amber-500/20 flex flex-col hover:bg-amber-500/20 transition-all duration-300 shadow-sm"
                  >
                    <div className="flex items-center justify-between gap-2">
                      <span className="font-mono text-base font-bold text-amber-600 dark:text-amber-400">:{port.port}</span>
                      <div className="h-1.5 w-1.5 rounded-full bg-amber-500 animate-pulse" />
                    </div>
                    <p className="text-xs font-semibold truncate mt-1 text-foreground/80">
                      {port.project?.name || "Unknown Project"}
                    </p>
                    <p className="text-[10px] text-muted-foreground font-mono mt-0.5 opacity-60">
                      {port.project_id?.split('-')[0] || "No ID"}
                    </p>
                  </div>
                ))}
              </div>
            )}
          </Card>

          <Card className="p-6">
            <div className="flex items-center gap-3 mb-5">
              <div className="p-2.5 rounded-xl bg-emerald-500/10">
                <Unlock className="h-5 w-5 text-emerald-500" />
              </div>
              <div>
                <h2 className="font-semibold">Available Ports</h2>
                <p className="text-sm text-muted-foreground">{freePorts.length} free for deployment</p>
              </div>
            </div>
            
            {loading ? (
              <div className="grid grid-cols-5 sm:grid-cols-8 gap-2">
                {[1, 2, 3, 4, 5, 6, 7, 8].map((i) => (
                  <div key={i} className="h-10 bg-secondary rounded-lg shimmer" />
                ))}
              </div>
            ) : (
              <div className="grid grid-cols-5 sm:grid-cols-8 gap-2 max-h-[400px] overflow-auto pr-2 custom-scrollbar">
                {freePorts.slice(0, 60).map((port) => (
                  <div
                    key={port.port}
                    className="px-2 py-2.5 rounded-lg bg-emerald-500/5 hover:bg-emerald-500/15 border border-emerald-500/10 hover:border-emerald-500/30 text-center transition-all duration-300"
                  >
                    <span className="font-mono text-sm text-emerald-600 dark:text-emerald-400">{port.port}</span>
                  </div>
                ))}
                {freePorts.length > 60 && (
                  <div className="col-span-full text-center text-[10px] font-bold text-muted-foreground py-3 bg-secondary/30 rounded-lg border border-dashed border-border mt-2">
                    +{freePorts.length - 60} MORE PORTS AVAILABLE
                  </div>
                )}
              </div>
            )}
          </Card>
        </div>

        <Card className="mt-6 p-6 border-border/40 bg-secondary/10">
          <div className="flex items-center gap-3 mb-4">
            <div className="p-2.5 rounded-xl bg-cyan-500/10">
              <Network className="h-5 w-5 text-cyan-500" />
            </div>
            <h2 className="font-semibold">Network Infrastructure</h2>
          </div>
          <div className="flex flex-wrap items-center gap-x-8 gap-y-4 text-sm">
            <div className="flex flex-col gap-1">
              <span className="text-[10px] font-bold text-muted-foreground uppercase tracking-widest">Port Range</span>
              <code className="font-mono bg-secondary/80 px-3 py-1 rounded-lg border border-border/50 text-xs">3001 - 3100</code>
            </div>
            <div className="flex flex-col gap-1">
              <span className="text-[10px] font-bold text-muted-foreground uppercase tracking-widest">Global Capacity</span>
              <span className="font-bold text-foreground">{ports.length} <span className="text-muted-foreground font-normal ml-1">Total Nodes</span></span>
            </div>
            <div className="flex flex-col gap-1">
              <span className="text-[10px] font-bold text-muted-foreground uppercase tracking-widest">Virtual Network</span>
              <code className="font-mono bg-secondary/80 px-3 py-1 rounded-lg border border-border/50 text-xs">docklift_network</code>
            </div>
          </div>
        </Card>
      </main>
    </div>
  );
}
