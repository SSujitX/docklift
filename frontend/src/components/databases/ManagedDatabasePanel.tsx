import { useCallback, useEffect, useState } from "react";
import { Copy, Check, Eye, EyeOff, Link2, Unlink, Loader2, Database } from "lucide-react";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { API_URL, copyToClipboard } from "@/lib/utils";
import { authFetch } from "@/lib/auth";
import { toast } from "sonner";
import { cn } from "@/lib/utils";

interface ConnectionInfo {
  engine: { id: string; label: string; image: string; port: number; defaultEnvKey: string };
  host: string;
  port: number;
  username: string | null;
  database: string;
  password: string;
  connection_url: string;
  note?: string;
  credentials_init_only?: boolean;
  internal_only?: boolean;
}

interface DbLink {
  id: string;
  service_name: string;
  env_key: string;
  app_project: { id: string; name: string; status: string | null };
}

interface AppOption {
  id: string;
  name: string;
  services?: { id: string; name: string }[];
}

export function ManagedDatabasePanel({
  databaseId,
  engineLabel,
}: {
  databaseId: string;
  engineLabel?: string;
}) {
  const [conn, setConn] = useState<ConnectionInfo | null>(null);
  const [links, setLinks] = useState<DbLink[]>([]);
  const [apps, setApps] = useState<AppOption[]>([]);
  const [loading, setLoading] = useState(true);
  const [showSecret, setShowSecret] = useState(false);
  const [copied, setCopied] = useState<string | null>(null);
  const [appId, setAppId] = useState("");
  const [serviceName, setServiceName] = useState("");
  const [linking, setLinking] = useState(false);
  const [overwrite, setOverwrite] = useState(false);

  const load = useCallback(async () => {
    try {
      const [cRes, lRes, pRes] = await Promise.all([
        authFetch(`${API_URL}/api/databases/${databaseId}/connection`),
        authFetch(`${API_URL}/api/databases/${databaseId}/links`),
        authFetch(`${API_URL}/api/projects`),
      ]);
      if (cRes.ok) setConn(await cRes.json());
      else setConn(null);
      if (lRes.ok) setLinks(await lRes.json());
      if (pRes.ok) {
        const all = await pRes.json();
        setApps(
          (Array.isArray(all) ? all : []).filter(
            (p: { project_type?: string }) => p.project_type !== "database",
          ),
        );
      }
    } catch {
      toast.error("Failed to load database details");
    } finally {
      setLoading(false);
    }
  }, [databaseId]);

  useEffect(() => {
    load();
  }, [load]);

  const selectedApp = apps.find((a) => a.id === appId);
  const [services, setServices] = useState<{ id: string; name: string }[]>([]);

  useEffect(() => {
    if (!appId) {
      setServices([]);
      return;
    }
    let cancelled = false;
    (async () => {
      try {
        const res = await authFetch(`${API_URL}/api/deployments/${appId}/services`);
        if (!res.ok) return;
        const data = await res.json();
        if (!cancelled) {
          setServices(
            (Array.isArray(data) ? data : []).map(
              (s: { id: string; name: string }) => ({
                id: s.id,
                name: s.name,
              }),
            ),
          );
        }
      } catch {
        if (!cancelled) setServices([]);
      }
    })();
    return () => {
      cancelled = true;
    };
  }, [appId]);

  void selectedApp;

  const copy = async (key: string, value: string) => {
    const ok = await copyToClipboard(value);
    if (!ok) {
      toast.error("Could not copy");
      return;
    }
    setCopied(key);
    toast.success("Copied");
    setTimeout(() => setCopied(null), 2000);
  };

  const link = async () => {
    if (!appId) {
      toast.error("Select a project");
      return;
    }
    setLinking(true);
    try {
      const res = await authFetch(`${API_URL}/api/databases/${databaseId}/links`, {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({
          app_project_id: appId,
          service_name: serviceName || "",
          overwrite,
        }),
      });
      const data = await res.json().catch(() => ({}));
      if (!res.ok) throw new Error(data.error || "Link failed");
      toast.success(data.note || "Linked — redeploy the app to apply env");
      setAppId("");
      setServiceName("");
      setOverwrite(false);
      await load();
    } catch (err) {
      toast.error(err instanceof Error ? err.message : "Link failed");
    } finally {
      setLinking(false);
    }
  };

  const unlink = async (linkId: string) => {
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
    }
  };

  if (loading) {
    return (
      <Card className="border-border shadow-none">
        <CardContent className="flex items-center gap-2 py-8 text-sm text-muted-foreground">
          <Loader2 className="h-4 w-4 animate-spin" />
          Loading connection…
        </CardContent>
      </Card>
    );
  }

  return (
    <div className="space-y-4">
      <Card className="border-border shadow-none">
        <CardHeader className="pb-3">
          <CardTitle className="flex items-center gap-2 text-base">
            <Database className="h-4 w-4 text-muted-foreground" />
            Connection
            {engineLabel || conn?.engine.label ? (
              <span className="text-xs font-normal text-muted-foreground">
                · {engineLabel || conn?.engine.label}
              </span>
            ) : null}
          </CardTitle>
        </CardHeader>
        <CardContent className="space-y-3">
          {!conn ? (
            <p className="text-sm text-muted-foreground">
              Deploy this database to generate a connection URL. Prefer linking to apps
              over publishing host ports.
            </p>
          ) : (
            <>
              <p className="text-xs leading-relaxed text-muted-foreground">
                {conn.note ||
                  "Internal Docker DNS only until you link an app. Do not share IP:port publicly."}
              </p>
              {conn.credentials_init_only === true && (
                <p className="rounded-lg border border-amber-500/20 bg-amber-500/10 px-3 py-2 text-[11px] leading-relaxed text-amber-800 dark:text-amber-200/90">
                  Passwords for this engine are applied only on first volume init.
                  Changing env and redeploying will not rotate the server password —
                  recreate the database to rotate.
                </p>
              )}
              <SecretRow
                label="Connection URL"
                value={conn.connection_url}
                reveal={showSecret}
                copied={copied === "url"}
                onCopy={() => copy("url", conn.connection_url)}
              />
              <div className="grid gap-2 sm:grid-cols-2">
                <Meta label="Host" value={conn.host} />
                <Meta label="Port" value={String(conn.port)} />
                {conn.username ? <Meta label="User" value={conn.username} /> : null}
                <Meta label="Database" value={conn.database} />
              </div>
              <div className="flex flex-wrap gap-2">
                <Button
                  type="button"
                  variant="outline"
                  size="sm"
                  onClick={() => setShowSecret((v) => !v)}
                >
                  {showSecret ? (
                    <EyeOff className="h-3.5 w-3.5" />
                  ) : (
                    <Eye className="h-3.5 w-3.5" />
                  )}
                  {showSecret ? "Hide secrets" : "Reveal secrets"}
                </Button>
                <Button
                  type="button"
                  variant="outline"
                  size="sm"
                  onClick={() => copy("url", conn.connection_url)}
                >
                  {copied === "url" ? (
                    <Check className="h-3.5 w-3.5 text-emerald-500" />
                  ) : (
                    <Copy className="h-3.5 w-3.5" />
                  )}
                  Copy URL
                </Button>
              </div>
            </>
          )}
        </CardContent>
      </Card>

      <Card className="border-border shadow-none">
        <CardHeader className="pb-3">
          <CardTitle className="flex items-center gap-2 text-base">
            <Link2 className="h-4 w-4 text-muted-foreground" />
            Linked apps
          </CardTitle>
        </CardHeader>
        <CardContent className="space-y-4">
          <div className="flex flex-col gap-2 sm:flex-row sm:items-end">
            <label className="flex-1 space-y-1 text-xs">
              <span className="text-muted-foreground">Project</span>
              <select
                value={appId}
                onChange={(e) => {
                  setAppId(e.target.value);
                  setServiceName("");
                }}
                className="flex h-9 w-full rounded-lg border border-border bg-background px-3 text-sm"
              >
                <option value="">Select project…</option>
                {apps.map((a) => (
                  <option key={a.id} value={a.id}>
                    {a.name}
                  </option>
                ))}
              </select>
            </label>
            <label className="flex-1 space-y-1 text-xs">
              <span className="text-muted-foreground">Service (optional)</span>
              <select
                value={serviceName}
                onChange={(e) => setServiceName(e.target.value)}
                disabled={!appId}
                className="flex h-9 w-full rounded-lg border border-border bg-background px-3 text-sm disabled:opacity-50"
              >
                <option value="">All services (shared env)</option>
                {services.map((s) => (
                  <option key={s.id} value={s.name}>
                    {s.name}
                  </option>
                ))}
              </select>
            </label>
            <Button
              type="button"
              onClick={link}
              disabled={linking || !appId}
              className="h-9 bg-brand text-brand-foreground"
            >
              {linking ? <Loader2 className="h-4 w-4 animate-spin" /> : <Link2 className="h-4 w-4" />}
              Link
            </Button>
          </div>
          <label className="flex items-center gap-2 text-[11px] text-muted-foreground">
            <input
              type="checkbox"
              checked={overwrite}
              onChange={(e) => setOverwrite(e.target.checked)}
              className="rounded border-border"
            />
            Overwrite existing env key if present
          </label>
          <p className="text-[11px] text-muted-foreground">
            Injects {conn?.engine.defaultEnvKey || "DATABASE_URL"} into the app (or one
            service). Redeploy the app afterward. Database must be running.
          </p>
          {links.length === 0 ? (
            <p className="text-sm text-muted-foreground">No apps linked yet.</p>
          ) : (
            <ul className="space-y-2">
              {links.map((l) => (
                <li
                  key={l.id}
                  className="flex items-center justify-between gap-2 rounded-lg border border-border/60 px-3 py-2 text-sm"
                >
                  <div className="min-w-0">
                    <div className="truncate font-medium">{l.app_project.name}</div>
                    <div className="text-[11px] text-muted-foreground">
                      {l.service_name ? `Service ${l.service_name}` : "All services"} ·{" "}
                      <span className="font-mono">{l.env_key}</span>
                    </div>
                  </div>
                  <Button
                    type="button"
                    variant="ghost"
                    size="sm"
                    onClick={() => unlink(l.id)}
                    className="text-muted-foreground hover:text-destructive"
                  >
                    <Unlink className="h-3.5 w-3.5" />
                    Unlink
                  </Button>
                </li>
              ))}
            </ul>
          )}
        </CardContent>
      </Card>
    </div>
  );
}

function Meta({ label, value }: { label: string; value: string }) {
  return (
    <div className="rounded-lg border border-border/50 bg-secondary/20 px-3 py-2">
      <div className="text-[10px] uppercase tracking-wide text-muted-foreground">{label}</div>
      <div className="mt-0.5 truncate font-mono text-xs">{value}</div>
    </div>
  );
}

function SecretRow({
  label,
  value,
  reveal,
  copied,
  onCopy,
}: {
  label: string;
  value: string;
  reveal: boolean;
  copied: boolean;
  onCopy: () => void;
}) {
  return (
    <div className="rounded-lg border border-border/50 bg-secondary/20 px-3 py-2">
      <div className="mb-1 flex items-center justify-between gap-2">
        <span className="text-[10px] uppercase tracking-wide text-muted-foreground">
          {label}
        </span>
        <button
          type="button"
          onClick={onCopy}
          className={cn(
            "inline-flex items-center gap-1 text-[11px] text-muted-foreground hover:text-foreground",
          )}
        >
          {copied ? <Check className="h-3 w-3 text-emerald-500" /> : <Copy className="h-3 w-3" />}
          Copy
        </button>
      </div>
      <code className="block break-all font-mono text-xs text-foreground/90">
        {reveal ? value : "••••••••••••••••••••••••"}
      </code>
    </div>
  );
}
