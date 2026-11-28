import { useCallback, useEffect, useState } from "react";
import { Copy, Check, Eye, EyeOff, Link2, Unlink, Loader2 } from "lucide-react";
import { Button } from "@/components/ui/button";
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from "@/components/ui/card";
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
      <Card>
        <CardContent className="flex items-center gap-2 py-8 text-[13px] text-muted-foreground">
          <Loader2 className="size-4 animate-spin" />
          Loading connection…
        </CardContent>
      </Card>
    );
  }

  const selectClass =
    "flex h-9 w-full rounded-md border border-input bg-card px-3 text-sm text-foreground transition-[border-color,box-shadow] duration-150 focus:border-brand focus:outline-hidden focus:ring-3 focus:ring-brand/20 disabled:opacity-50 pointer-coarse:h-10";

  return (
    <div className="space-y-4">
      <Card>
        <CardHeader className="flex-row flex-wrap items-center justify-between gap-x-4 gap-y-2">
          <CardTitle className="flex items-baseline gap-2">
            Connection
            {engineLabel || conn?.engine.label ? (
              <span className="text-[13px] font-normal text-muted-foreground">
                {engineLabel || conn?.engine.label}
              </span>
            ) : null}
          </CardTitle>
          {conn && (
            <div className="flex flex-wrap gap-2">
              <Button
                type="button"
                variant="outline"
                size="sm"
                onClick={() => setShowSecret((v) => !v)}
              >
                {showSecret ? <EyeOff /> : <Eye />}
                {showSecret ? "Hide secrets" : "Reveal secrets"}
              </Button>
              <Button
                type="button"
                variant="outline"
                size="sm"
                onClick={() => copy("url", conn.connection_url)}
              >
                {copied === "url" ? <Check className="text-ok" /> : <Copy />}
                Copy URL
              </Button>
            </div>
          )}
        </CardHeader>
        <CardContent className="space-y-4">
          {!conn ? (
            <p className="text-[13px] leading-5 text-muted-foreground">
              Deploy this database to generate a connection URL. Prefer linking to apps
              over publishing host ports.
            </p>
          ) : (
            <>
              <p className="text-[13px] leading-5 text-muted-foreground">
                {conn.note ||
                  "Internal Docker DNS only until you link an app. Do not share IP:port publicly."}
              </p>
              {conn.credentials_init_only === true && (
                <p className="rounded-md border border-warn/40 bg-secondary/60 px-3 py-2.5 text-[13px] leading-5 text-foreground">
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
              <dl className="grid gap-x-8 sm:grid-cols-2">
                <Meta label="Host" value={conn.host} />
                <Meta label="Port" value={String(conn.port)} />
                {conn.username ? <Meta label="User" value={conn.username} /> : null}
                <Meta label="Database" value={conn.database} />
              </dl>
            </>
          )}
        </CardContent>
      </Card>

      <Card>
        <CardHeader>
          <CardTitle>Linked apps</CardTitle>
          <CardDescription>
            Injects{" "}
            <span className="font-mono text-xs text-foreground">
              {conn?.engine.defaultEnvKey || "DATABASE_URL"}
            </span>{" "}
            into the app (or one service). Redeploy the app afterward. Database must be
            running.
          </CardDescription>
        </CardHeader>
        <CardContent className="space-y-3">
          <div className="flex flex-col gap-3 sm:flex-row sm:items-end">
            <label className="flex-1 space-y-1.5">
              <span className="block text-[13px] font-medium">Project</span>
              <select
                value={appId}
                onChange={(e) => {
                  setAppId(e.target.value);
                  setServiceName("");
                }}
                className={selectClass}
              >
                <option value="">Select project…</option>
                {apps.map((a) => (
                  <option key={a.id} value={a.id}>
                    {a.name}
                  </option>
                ))}
              </select>
            </label>
            <label className="flex-1 space-y-1.5">
              <span className="block text-[13px] font-medium">Service (optional)</span>
              <select
                value={serviceName}
                onChange={(e) => setServiceName(e.target.value)}
                disabled={!appId}
                className={selectClass}
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
              variant="outline"
              onClick={link}
              disabled={linking || !appId}
              className="h-9 pointer-coarse:h-10"
            >
              {linking ? <Loader2 className="animate-spin" /> : <Link2 />}
              Link
            </Button>
          </div>
          <label className="flex items-center gap-2 text-[13px] text-muted-foreground">
            <input
              type="checkbox"
              checked={overwrite}
              onChange={(e) => setOverwrite(e.target.checked)}
              className="size-4 rounded-sm border-input accent-brand"
            />
            Overwrite existing env key if present
          </label>
        </CardContent>
        {links.length === 0 ? (
          <p className="border-t border-border px-4 py-3 text-[13px] text-muted-foreground">
            No apps linked yet.
          </p>
        ) : (
          <ul className="divide-y divide-border border-t border-border">
            {links.map((l) => (
              <li
                key={l.id}
                className="flex items-center justify-between gap-3 px-4 py-2.5"
              >
                <div className="min-w-0">
                  <div className="truncate text-sm font-medium">{l.app_project.name}</div>
                  <div className="truncate font-mono text-xs text-muted-foreground">
                    {l.service_name ? `service ${l.service_name}` : "all services"}
                    <span className="text-subtle"> · </span>
                    {l.env_key}
                  </div>
                </div>
                <Button
                  type="button"
                  variant="ghost"
                  size="sm"
                  onClick={() => unlink(l.id)}
                  className="shrink-0 text-muted-foreground hover:text-danger"
                >
                  <Unlink />
                  Unlink
                </Button>
              </li>
            ))}
          </ul>
        )}
      </Card>
    </div>
  );
}

function Meta({ label, value }: { label: string; value: string }) {
  return (
    <div className="flex items-baseline justify-between gap-4 border-b border-border py-2">
      <dt className="shrink-0 text-[13px] text-muted-foreground">{label}</dt>
      <dd className="min-w-0 truncate font-mono text-[13px] text-foreground">{value}</dd>
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
    <div>
      <div className="mb-1.5 flex items-center justify-between gap-2">
        <span className="text-[13px] font-medium">{label}</span>
        <button
          type="button"
          onClick={onCopy}
          className={cn(
            "inline-flex items-center gap-1 rounded-sm text-xs text-muted-foreground transition-colors duration-150 hover:text-foreground",
          )}
        >
          {copied ? <Check className="size-3.5 text-ok" /> : <Copy className="size-3.5" />}
          Copy
        </button>
      </div>
      <code className="block break-all rounded-md bg-secondary/60 px-3 py-2 font-mono text-[13px] leading-5 text-foreground">
        {reveal ? value : "••••••••••••••••••••••••"}
      </code>
    </div>
  );
}
