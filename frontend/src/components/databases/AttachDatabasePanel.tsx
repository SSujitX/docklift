import { useCallback, useEffect, useState } from "react";
import { Database, Link2, Loader2, Unlink } from "lucide-react";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { API_URL } from "@/lib/utils";
import { authFetch } from "@/lib/auth";
import { toast } from "sonner";

interface AppLink {
  id: string;
  service_name: string;
  env_key: string;
  database_project: {
    id: string;
    name: string;
    status: string | null;
    db_engine: string | null;
  };
}

interface DbOption {
  id: string;
  name: string;
  db_engine: string | null;
  status: string | null;
}

export function AttachDatabasePanel({
  appProjectId,
  services,
}: {
  appProjectId: string;
  services: { id: string; name: string }[];
}) {
  const [links, setLinks] = useState<AppLink[]>([]);
  const [dbs, setDbs] = useState<DbOption[]>([]);
  const [loading, setLoading] = useState(true);
  const [dbId, setDbId] = useState("");
  const [serviceName, setServiceName] = useState("");
  const [overwrite, setOverwrite] = useState(false);
  const [busy, setBusy] = useState(false);

  const load = useCallback(async () => {
    try {
      const [lRes, dRes] = await Promise.all([
        authFetch(`${API_URL}/api/databases/links/by-app/${appProjectId}`),
        authFetch(`${API_URL}/api/databases`),
      ]);
      if (lRes.ok) setLinks(await lRes.json());
      if (dRes.ok) {
        const list = await dRes.json();
        setDbs(
          (Array.isArray(list) ? list : []).filter(
            (d: DbOption) => d.status === "running" || d.status === "degraded",
          ),
        );
      }
    } catch {
      toast.error("Failed to load database links");
    } finally {
      setLoading(false);
    }
  }, [appProjectId]);

  useEffect(() => {
    load();
  }, [load]);

  const attach = async () => {
    if (!dbId) {
      toast.error("Select a database");
      return;
    }
    setBusy(true);
    try {
      const res = await authFetch(`${API_URL}/api/databases/${dbId}/links`, {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({
          app_project_id: appProjectId,
          service_name: serviceName || "",
          overwrite,
        }),
      });
      const data = await res.json().catch(() => ({}));
      if (!res.ok) throw new Error(data.error || "Attach failed");
      toast.success(data.note || "Attached — redeploy to apply env");
      setDbId("");
      setServiceName("");
      setOverwrite(false);
      await load();
    } catch (err) {
      toast.error(err instanceof Error ? err.message : "Attach failed");
    } finally {
      setBusy(false);
    }
  };

  const unlink = async (databaseId: string, linkId: string) => {
    setBusy(true);
    try {
      const res = await authFetch(
        `${API_URL}/api/databases/${databaseId}/links/${linkId}`,
        { method: "DELETE" },
      );
      const data = await res.json().catch(() => ({}));
      if (!res.ok) throw new Error(data.error || "Unlink failed");
      toast.success(data.note || "Unlinked");
      await load();
    } catch (err) {
      toast.error(err instanceof Error ? err.message : "Unlink failed");
    } finally {
      setBusy(false);
    }
  };

  return (
    <Card className="border-border shadow-none">
      <CardHeader className="pb-3">
        <CardTitle className="flex items-center gap-2 text-base">
          <Database className="h-4 w-4 text-muted-foreground" />
          Attach database
        </CardTitle>
      </CardHeader>
      <CardContent className="space-y-4">
        {loading ? (
          <div className="flex items-center gap-2 text-sm text-muted-foreground">
            <Loader2 className="h-4 w-4 animate-spin" />
            Loading…
          </div>
        ) : (
          <>
            <p className="text-xs leading-relaxed text-muted-foreground">
              Link a managed database over the private Docker network and inject its
              connection URL into this project (or one service). Redeploy afterward.
            </p>
            <div className="flex flex-col gap-2 sm:flex-row sm:items-end">
              <label className="flex-1 space-y-1 text-xs">
                <span className="text-muted-foreground">Database</span>
                <select
                  value={dbId}
                  onChange={(e) => setDbId(e.target.value)}
                  className="flex h-9 w-full rounded-lg border border-border bg-background px-3 text-sm"
                >
                  <option value="">Select database…</option>
                  {dbs.map((d) => (
                    <option key={d.id} value={d.id}>
                      {d.name}
                      {d.db_engine ? ` (${d.db_engine})` : ""}
                    </option>
                  ))}
                </select>
              </label>
              <label className="flex-1 space-y-1 text-xs">
                <span className="text-muted-foreground">Service scope</span>
                <select
                  value={serviceName}
                  onChange={(e) => setServiceName(e.target.value)}
                  className="flex h-9 w-full rounded-lg border border-border bg-background px-3 text-sm"
                >
                  <option value="">All services (shared)</option>
                  {services.map((s) => (
                    <option key={s.id} value={s.name}>
                      {s.name}
                    </option>
                  ))}
                </select>
              </label>
              <Button
                type="button"
                onClick={attach}
                disabled={busy || !dbId || dbs.length === 0}
                className="h-9 bg-brand text-brand-foreground"
              >
                {busy ? (
                  <Loader2 className="h-4 w-4 animate-spin" />
                ) : (
                  <Link2 className="h-4 w-4" />
                )}
                Attach
              </Button>
            </div>
            <label className="flex items-center gap-2 text-xs text-muted-foreground">
              <input
                type="checkbox"
                checked={overwrite}
                onChange={(e) => setOverwrite(e.target.checked)}
                className="rounded border-border"
              />
              Overwrite existing env key if present (e.g. manual DATABASE_URL)
            </label>
            {dbs.length === 0 && (
              <p className="text-sm text-muted-foreground">
                No running managed databases. Create and deploy one under Databases.
              </p>
            )}
            {links.length > 0 && (
              <ul className="space-y-2">
                {links.map((l) => (
                  <li
                    key={l.id}
                    className="flex items-center justify-between gap-2 rounded-lg border border-border/60 px-3 py-2 text-sm"
                  >
                    <div className="min-w-0">
                      <div className="truncate font-medium">
                        {l.database_project.name}
                      </div>
                      <div className="text-[11px] text-muted-foreground">
                        {l.database_project.db_engine || "db"} ·{" "}
                        {l.service_name ? `Service ${l.service_name}` : "All services"} ·{" "}
                        <span className="font-mono">{l.env_key}</span>
                      </div>
                    </div>
                    <Button
                      type="button"
                      variant="ghost"
                      size="sm"
                      disabled={busy}
                      onClick={() => unlink(l.database_project.id, l.id)}
                      className="text-muted-foreground hover:text-destructive"
                    >
                      <Unlink className="h-3.5 w-3.5" />
                      Unlink
                    </Button>
                  </li>
                ))}
              </ul>
            )}
          </>
        )}
      </CardContent>
    </Card>
  );
}
