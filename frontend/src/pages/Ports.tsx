// Ports page - displays used and available ports across the server

import { useEffect, useState } from "react";
import { PageHeader, StatChip } from "@/components/shell/PageHeader";
import { Card } from "@/components/ui/card";
import { Port } from "@/lib/types";
import { API_URL } from "@/lib/utils";
import { Anchor, Lock, Unlock, Network } from "lucide-react";
import { authFetch } from "@/lib/auth";

export default function PortsPage() {
  const [ports, setPorts] = useState<Port[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    authFetch(`${API_URL}/api/ports`)
      .then(async (res) => {
        if (!res.ok) {
          const data = await res.json().catch(() => ({}));
          throw new Error((data as { error?: string }).error || `Failed to load ports (${res.status})`);
        }
        return res.json();
      })
      .then((data) => {
        setPorts(Array.isArray(data) ? data : []);
        setError(null);
        setLoading(false);
      })
      .catch((err: Error) => {
        setError(err.message || "Failed to load ports");
        setLoading(false);
      });
  }, []);

  const usedPorts = ports.filter((p) => p.is_locked);
  const freePorts = ports.filter((p) => !p.is_locked);
  // Derived from the live pool so the range always matches the server config.
  const poolNumbers = ports.map((p) => p.port);
  const poolRange = poolNumbers.length
    ? `${Math.min(...poolNumbers)} - ${Math.max(...poolNumbers)}`
    : "—";

  return (
    <>
      <PageHeader
        eyebrow="Deploy"
        title="Port Management"
        description="Host ports Docklift assigns to deployed services."
        icon={Anchor}
        meta={
          <>
            <StatChip label="Allocated" value={usedPorts.length} tone="warning" />
            <StatChip label="Available" value={freePorts.length} tone="success" />
          </>
        }
      />

      {error && (
        <div className="mb-4 rounded-xl border border-red-500/20 bg-red-500/10 px-4 py-3 text-sm text-red-600 dark:text-red-400">
          {error}
        </div>
      )}

      <div className="grid grid-cols-1 gap-6 lg:grid-cols-2">
        <Card className="p-6">
          <div className="mb-5 flex items-center gap-3">
            <div className="rounded-xl bg-amber-500/10 p-2.5">
              <Lock className="h-5 w-5 text-amber-500" />
            </div>
            <div>
              <h2 className="font-semibold">Used Ports</h2>
              <p className="text-sm text-muted-foreground">{usedPorts.length} active allocations</p>
            </div>
          </div>

          {loading ? (
            <div className="grid grid-cols-5 gap-2 sm:grid-cols-8">
              {[1, 2, 3, 4].map((i) => (
                <div key={i} className="h-10 rounded-lg bg-secondary shimmer" />
              ))}
            </div>
          ) : usedPorts.length === 0 ? (
            <div className="py-8 text-center text-sm text-muted-foreground">No ports in use</div>
          ) : (
            <div className="grid grid-cols-1 gap-3 sm:grid-cols-2">
              {usedPorts.map((port) => (
                <div
                  key={port.port}
                  className="group relative flex flex-col rounded-xl border border-amber-500/20 bg-amber-500/10 px-4 py-3 shadow-sm transition-all duration-300 hover:bg-amber-500/20"
                >
                  <div className="flex items-center justify-between gap-2">
                    <span className="font-mono text-base font-bold text-amber-600 dark:text-amber-400">:{port.port}</span>
                    <div className="h-1.5 w-1.5 animate-pulse rounded-full bg-amber-500" />
                  </div>
                  <p className="mt-1 truncate text-xs font-semibold text-foreground/80">
                    {port.project?.name || "Unknown Project"}
                  </p>
                  <p className="mt-0.5 font-mono text-[10px] text-muted-foreground opacity-60">
                    {port.project_id?.split('-')[0] || "No ID"}
                  </p>
                </div>
              ))}
            </div>
          )}
        </Card>

        <Card className="p-6">
          <div className="mb-5 flex items-center gap-3">
            <div className="rounded-xl bg-emerald-500/10 p-2.5">
              <Unlock className="h-5 w-5 text-emerald-500" />
            </div>
            <div>
              <h2 className="font-semibold">Available Ports</h2>
              <p className="text-sm text-muted-foreground">{freePorts.length} free for deployment</p>
            </div>
          </div>

          {loading ? (
            <div className="grid grid-cols-5 gap-2 sm:grid-cols-8">
              {[1, 2, 3, 4, 5, 6, 7, 8].map((i) => (
                <div key={i} className="h-10 rounded-lg bg-secondary shimmer" />
              ))}
            </div>
          ) : (
            <div className="shell-scroll grid max-h-[400px] grid-cols-5 gap-2 overflow-auto pr-2 sm:grid-cols-8">
              {freePorts.slice(0, 60).map((port) => (
                <div
                  key={port.port}
                  className="rounded-lg border border-emerald-500/10 bg-emerald-500/5 px-2 py-2.5 text-center transition-all duration-300 hover:border-emerald-500/30 hover:bg-emerald-500/15"
                >
                  <span className="font-mono text-sm text-emerald-600 dark:text-emerald-400">{port.port}</span>
                </div>
              ))}
              {freePorts.length > 60 && (
                <div className="col-span-full mt-2 rounded-lg border border-dashed border-border bg-secondary/30 py-3 text-center text-[10px] font-bold text-muted-foreground">
                  +{freePorts.length - 60} MORE PORTS AVAILABLE
                </div>
              )}
            </div>
          )}
        </Card>
      </div>

      <Card className="mt-6 border-border/40 bg-secondary/10 p-6">
        <div className="mb-4 flex items-center gap-3">
          <div className="rounded-xl bg-brand/10 p-2.5">
            <Network className="h-5 w-5 text-brand" />
          </div>
          <h2 className="font-semibold">Network Infrastructure</h2>
        </div>
        <div className="flex flex-wrap items-center gap-x-8 gap-y-4 text-sm">
          <div className="flex flex-col gap-1">
            <span className="text-[10px] font-bold uppercase tracking-widest text-muted-foreground">Port Range</span>
            <code className="rounded-lg border border-border/50 bg-secondary/80 px-3 py-1 font-mono text-xs">{poolRange}</code>
          </div>
          <div className="flex flex-col gap-1">
            <span className="text-[10px] font-bold uppercase tracking-widest text-muted-foreground">Pool Capacity</span>
            <span className="font-bold text-foreground">
              {ports.length} <span className="ml-1 font-normal text-muted-foreground">total ports</span>
            </span>
          </div>
          <div className="flex flex-col gap-1">
            <span className="text-[10px] font-bold uppercase tracking-widest text-muted-foreground">Docker Network</span>
            <code className="rounded-lg border border-border/50 bg-secondary/80 px-3 py-1 font-mono text-xs">docklift_network</code>
          </div>
        </div>
      </Card>
    </>
  );
}
