import { useEffect, useState } from "react";
import { Link, useNavigate } from "react-router-dom";
import {
  ArrowLeft,
  Database,
  Loader2,
  Server,
} from "lucide-react";
import { PageHeader } from "@/components/shell/PageHeader";
import { Button } from "@/components/ui/button";
import { API_URL, cn } from "@/lib/utils";
import { authFetch } from "@/lib/auth";
import { toast } from "sonner";

interface Engine {
  id: string;
  label: string;
  description: string;
  image: string;
  port: number;
  defaultEnvKey: string;
}

export default function NewDatabasePage() {
  const navigate = useNavigate();
  const [engines, setEngines] = useState<Engine[]>([]);
  const [engineId, setEngineId] = useState("postgres");
  const [name, setName] = useState("");
  const [loadingEngines, setLoadingEngines] = useState(true);
  const [creating, setCreating] = useState(false);

  useEffect(() => {
    (async () => {
      try {
        const res = await authFetch(`${API_URL}/api/databases/engines`);
        if (!res.ok) throw new Error("Failed to load engines");
        const data = await res.json();
        setEngines(Array.isArray(data) ? data : []);
        if (Array.isArray(data) && data[0]?.id) setEngineId(data[0].id);
      } catch (err) {
        toast.error(err instanceof Error ? err.message : "Failed to load engines");
      } finally {
        setLoadingEngines(false);
      }
    })();
  }, []);

  const create = async () => {
    const trimmed = name.trim();
    if (!trimmed) {
      toast.error("Name is required");
      return;
    }
    setCreating(true);
    try {
      const res = await authFetch(`${API_URL}/api/databases`, {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ name: trimmed, engine: engineId }),
      });
      const data = await res.json().catch(() => ({}));
      if (!res.ok) throw new Error(data.error || "Create failed");

      toast.success(`${data.engine?.label || "Database"} created — starting deploy`);
      toast.message("Copy the connection URL from the Connection panel (password is stored on the project)");

      // Kick deploy (fire-and-forget stream); detail page will show status/logs
      authFetch(`${API_URL}/api/deployments/${data.id}/deploy`, {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ trigger: "create" }),
      }).catch(() => {
        /* detail page can retry */
      });

      navigate(`/projects/${data.id}?tab=overview`);
    } catch (err) {
      toast.error(err instanceof Error ? err.message : "Create failed");
    } finally {
      setCreating(false);
    }
  };

  return (
    <>
      <Link
        to="/databases"
        className="mb-6 inline-flex items-center gap-2 text-muted-foreground transition-colors hover:text-foreground"
      >
        <ArrowLeft className="h-4 w-4" />
        Back to Databases
      </Link>

      <PageHeader
        eyebrow="Deploy"
        title="New Database"
        description="One-click managed database from an official image. Private by default — link to apps instead of exposing IP:port."
        icon={Database}
      />

      <div className="mx-auto max-w-2xl space-y-6">
        <div className="space-y-2">
          <label className="text-sm font-medium">Name</label>
          <input
            value={name}
            onChange={(e) => setName(e.target.value)}
            placeholder="my-postgres"
            maxLength={120}
            className="flex h-11 w-full rounded-xl border border-border bg-background px-4 text-sm outline-none focus:ring-2 focus:ring-ring"
          />
        </div>

        <div className="space-y-2">
          <label className="text-sm font-medium">Engine</label>
          {loadingEngines ? (
            <div className="flex items-center gap-2 text-sm text-muted-foreground">
              <Loader2 className="h-4 w-4 animate-spin" />
              Loading engines…
            </div>
          ) : (
            <div className="grid gap-3 sm:grid-cols-2">
              {engines.map((engine) => (
                <button
                  key={engine.id}
                  type="button"
                  onClick={() => setEngineId(engine.id)}
                  className={cn(
                    "rounded-2xl border p-4 text-left transition-colors",
                    engineId === engine.id
                      ? "border-brand/40 bg-brand/10"
                      : "border-border bg-card/40 hover:bg-secondary/40",
                  )}
                >
                  <div className="flex items-center gap-2 font-semibold">
                    <Server className="h-4 w-4 text-muted-foreground" />
                    {engine.label}
                  </div>
                  <p className="mt-1.5 text-xs leading-relaxed text-muted-foreground">
                    {engine.description}
                  </p>
                  <p className="mt-2 font-mono text-[10px] text-muted-foreground/80">
                    {engine.image} · :{engine.port} · {engine.defaultEnvKey}
                  </p>
                </button>
              ))}
            </div>
          )}
        </div>

        <div className="rounded-xl border border-border/60 bg-secondary/20 px-4 py-3 text-xs leading-relaxed text-muted-foreground">
          Host ports stay off. After deploy, link this database to a project or a
          specific service — DockLift joins networks and injects the connection URL.
        </div>

        <Button
          type="button"
          onClick={create}
          disabled={creating || loadingEngines || !name.trim()}
          className="h-11 w-full bg-brand font-semibold text-brand-foreground sm:w-auto sm:px-8"
        >
          {creating ? (
            <Loader2 className="h-4 w-4 animate-spin" />
          ) : (
            <Database className="h-4 w-4" />
          )}
          Create & deploy
        </Button>
      </div>
    </>
  );
}
