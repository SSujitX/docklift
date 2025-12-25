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
            <span className="text-amber-500">{usedPorts.length} used</span> · <span className="text-emerald-500">{freePorts.length} available</span>
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
                <p className="text-sm text-muted-foreground">{usedPorts.length} allocated</p>
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
              <div className="grid grid-cols-5 sm:grid-cols-8 gap-2">
                {usedPorts.map((port) => (
                  <div
                    key={port.port}
                    className="group relative px-2 py-2 rounded-lg bg-amber-500/10 border border-amber-500/20 text-center hover:bg-amber-500/20 transition-colors cursor-default"
                    title={port.project_id || ""}
                  >
                    <span className="font-mono text-sm text-amber-600 dark:text-amber-400">{port.port}</span>
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
                <p className="text-sm text-muted-foreground">{freePorts.length} free</p>
              </div>
            </div>
            
            {loading ? (
              <div className="grid grid-cols-5 sm:grid-cols-8 gap-2">
                {[1, 2, 3, 4, 5, 6, 7, 8].map((i) => (
                  <div key={i} className="h-10 bg-secondary rounded-lg shimmer" />
                ))}
              </div>
            ) : (
              <div className="grid grid-cols-5 sm:grid-cols-8 gap-2 max-h-[300px] overflow-auto">
                {freePorts.slice(0, 40).map((port) => (
                  <div
                    key={port.port}
                    className="px-2 py-2 rounded-lg bg-emerald-500/10 border border-emerald-500/20 text-center"
                  >
                    <span className="font-mono text-sm text-emerald-600 dark:text-emerald-400">{port.port}</span>
                  </div>
                ))}
                {freePorts.length > 40 && (
                  <div className="col-span-full text-center text-sm text-muted-foreground py-2">
                    +{freePorts.length - 40} more available
                  </div>
                )}
              </div>
            )}
          </Card>
        </div>

        <Card className="mt-6 p-6">
          <div className="flex items-center gap-3 mb-4">
            <div className="p-2.5 rounded-xl bg-primary/10">
              <Network className="h-5 w-5 text-primary" />
            </div>
            <h2 className="font-semibold">Configuration</h2>
          </div>
          <div className="flex flex-wrap items-center gap-6 text-sm">
            <div className="flex items-center gap-2">
              <span className="text-muted-foreground">Range:</span>
              <code className="font-mono bg-secondary px-3 py-1 rounded-lg">3001 - 3100</code>
            </div>
            <div className="flex items-center gap-2">
              <span className="text-muted-foreground">Total:</span>
              <span className="font-semibold">{ports.length} ports</span>
            </div>
            <div className="flex items-center gap-2">
              <span className="text-muted-foreground">Network:</span>
              <code className="font-mono bg-secondary px-3 py-1 rounded-lg">hostify_network</code>
            </div>
          </div>
        </Card>
      </main>
    </div>
  );
}
