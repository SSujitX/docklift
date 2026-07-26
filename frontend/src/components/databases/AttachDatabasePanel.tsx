import { useCallback, useEffect, useRef, useState } from "react";
import {
  Check,
  ChevronDown,
  Database,
  Link2,
  Loader2,
  Unlink,
} from "lucide-react";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { API_URL, cn } from "@/lib/utils";
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

function RoundedSelect({
  value,
  onChange,
  disabled,
  placeholder,
  options,
  label,
}: {
  value: string;
  onChange: (value: string) => void;
  disabled?: boolean;
  placeholder: string;
  options: { value: string; label: string }[];
  label: string;
}) {
  const [open, setOpen] = useState(false);
  const ref = useRef<HTMLDivElement>(null);
  const selected = options.find((o) => o.value === value);

  useEffect(() => {
    const onDoc = (e: MouseEvent) => {
      if (ref.current && !ref.current.contains(e.target as Node)) setOpen(false);
    };
    document.addEventListener("mousedown", onDoc);
    return () => document.removeEventListener("mousedown", onDoc);
  }, []);

  return (
    <label className="flex min-w-0 flex-1 flex-col gap-1.5 text-xs">
      <span className="font-medium text-muted-foreground">{label}</span>
      <div className="relative" ref={ref}>
        <button
          type="button"
          disabled={disabled}
          onClick={() => setOpen((v) => !v)}
          className={cn(
            "flex h-11 w-full items-center gap-2 rounded-xl border border-border/60 bg-background px-3 text-left text-sm transition-colors",
            "hover:border-brand/40 disabled:cursor-not-allowed disabled:opacity-50",
            open && "border-brand/40 ring-2 ring-brand/15",
          )}
        >
          <span
            className={cn(
              "min-w-0 flex-1 truncate",
              selected ? "font-medium text-foreground" : "text-muted-foreground",
            )}
          >
            {selected?.label || placeholder}
          </span>
          <ChevronDown
            className={cn(
              "h-4 w-4 shrink-0 text-muted-foreground transition-transform",
              open && "rotate-180 text-brand",
            )}
          />
        </button>
        {open && (
          <div className="absolute left-0 right-0 z-50 mt-1.5 overflow-hidden rounded-xl border border-border/60 bg-popover shadow-xl shadow-black/10">
            <div className="max-h-[min(14rem,40vh)] overflow-y-auto p-1">
              {options.length === 0 ? (
                <p className="px-3 py-6 text-center text-sm text-muted-foreground">
                  No options
                </p>
              ) : (
                options.map((opt) => {
                  const active = opt.value === value;
                  return (
                    <button
                      key={opt.value || "__empty"}
                      type="button"
                      onClick={() => {
                        onChange(opt.value);
                        setOpen(false);
                      }}
                      className={cn(
                        "flex w-full items-center gap-2 rounded-lg px-3 py-2.5 text-left text-sm transition-colors",
                        active
                          ? "bg-brand/10 font-medium text-brand"
                          : "text-foreground hover:bg-secondary/60",
                      )}
                    >
                      <span className="min-w-0 flex-1 truncate">{opt.label}</span>
                      {active && <Check className="h-3.5 w-3.5 shrink-0" />}
                    </button>
                  );
                })
              )}
            </div>
          </div>
        )}
      </div>
    </label>
  );
}

export function AttachDatabasePanel({
  appProjectId,
  services,
  /** When set (service workspace), attach only to this service — no scope picker. */
  lockedServiceName = null,
}: {
  appProjectId: string;
  services: { id: string; name: string }[];
  lockedServiceName?: string | null;
}) {
  const [links, setLinks] = useState<AppLink[]>([]);
  const [dbs, setDbs] = useState<DbOption[]>([]);
  const [loading, setLoading] = useState(true);
  const [dbId, setDbId] = useState("");
  const [serviceName, setServiceName] = useState("");
  const [overwrite, setOverwrite] = useState(false);
  const [busy, setBusy] = useState(false);

  const scoped = Boolean(lockedServiceName);

  useEffect(() => {
    if (lockedServiceName) {
      setServiceName(lockedServiceName);
    } else {
      setServiceName("");
    }
  }, [lockedServiceName]);

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
    const targetService = lockedServiceName || serviceName;
    setBusy(true);
    try {
      const res = await authFetch(`${API_URL}/api/databases/${dbId}/links`, {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({
          app_project_id: appProjectId,
          service_name: targetService || "",
          overwrite,
        }),
      });
      const data = await res.json().catch(() => ({}));
      if (!res.ok) throw new Error(data.error || "Attach failed");
      toast.success(data.note || "Attached — redeploy to apply env");
      setDbId("");
      if (!lockedServiceName) setServiceName("");
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

  const visibleLinks = lockedServiceName
    ? links.filter(
        (l) => !l.service_name || l.service_name === lockedServiceName,
      )
    : links;

  const dbOptions = dbs.map((d) => ({
    value: d.id,
    label: `${d.name}${d.db_engine ? ` (${d.db_engine})` : ""}`,
  }));

  const scopeOptions = [
    { value: "", label: "All services (shared)" },
    ...services.map((s) => ({ value: s.name, label: s.name })),
  ];

  return (
    <Card className="rounded-2xl border-border/60 shadow-none">
      <CardHeader className="pb-3">
        <CardTitle className="flex flex-wrap items-center gap-2 text-base font-semibold tracking-tight">
          <Database className="h-4 w-4 text-muted-foreground" />
          Attach database
          {scoped && lockedServiceName && (
            <span className="rounded-lg border border-brand/25 bg-brand/10 px-2 py-0.5 text-[10px] font-semibold text-brand">
              {lockedServiceName}
            </span>
          )}
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
              {scoped
                ? `Link a managed database and inject its connection URL into ${lockedServiceName}. Redeploy afterward.`
                : "Link a managed database over the private Docker network and inject its connection URL into all services or one service. Redeploy afterward."}
            </p>

            <div className="flex flex-col gap-3">
              <div
                className={cn(
                  "grid gap-3",
                  scoped ? "grid-cols-1" : "grid-cols-1 sm:grid-cols-2",
                )}
              >
                <RoundedSelect
                  label="Database"
                  placeholder="Select database…"
                  value={dbId}
                  onChange={setDbId}
                  disabled={busy || dbs.length === 0}
                  options={dbOptions}
                />
                {!scoped && (
                  <RoundedSelect
                    label="Service scope"
                    placeholder="All services (shared)"
                    value={serviceName}
                    onChange={setServiceName}
                    disabled={busy}
                    options={scopeOptions}
                  />
                )}
              </div>

              <label className="flex items-start gap-2.5 text-xs leading-relaxed text-muted-foreground">
                <input
                  type="checkbox"
                  checked={overwrite}
                  onChange={(e) => setOverwrite(e.target.checked)}
                  className="mt-0.5 h-4 w-4 rounded-md border-border text-brand focus:ring-brand/30"
                />
                Overwrite existing env key if present (e.g. manual DATABASE_URL)
              </label>

              <Button
                type="button"
                onClick={attach}
                disabled={busy || !dbId || dbs.length === 0}
                className="h-11 w-full rounded-xl bg-brand font-semibold text-brand-foreground shadow-lg shadow-brand/15 hover:brightness-110 sm:w-auto sm:self-start sm:px-6"
              >
                {busy ? (
                  <Loader2 className="h-4 w-4 animate-spin" />
                ) : (
                  <Link2 className="h-4 w-4" />
                )}
                Attach
                {scoped && lockedServiceName ? ` to ${lockedServiceName}` : ""}
              </Button>
            </div>

            {dbs.length === 0 && (
              <p className="rounded-xl border border-dashed border-border/60 bg-secondary/20 px-3 py-3 text-sm text-muted-foreground">
                No running managed databases. Create and deploy one under Databases.
              </p>
            )}

            {visibleLinks.length > 0 && (
              <ul className="space-y-2">
                {visibleLinks.map((l) => (
                  <li
                    key={l.id}
                    className="flex items-center justify-between gap-2 rounded-xl border border-border/60 bg-secondary/20 px-3 py-2.5 text-sm"
                  >
                    <div className="min-w-0">
                      <div className="truncate font-medium">
                        {l.database_project.name}
                      </div>
                      <div className="text-[11px] text-muted-foreground">
                        {l.database_project.db_engine || "db"} ·{" "}
                        {l.service_name
                          ? `Service ${l.service_name}`
                          : "All services"}{" "}
                        · <span className="font-mono">{l.env_key}</span>
                      </div>
                    </div>
                    <Button
                      type="button"
                      variant="ghost"
                      size="sm"
                      disabled={busy}
                      onClick={() => unlink(l.database_project.id, l.id)}
                      className="shrink-0 rounded-xl text-muted-foreground hover:text-destructive"
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
