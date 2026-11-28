import { useCallback, useEffect, useRef, useState } from "react";
import {
  Check,
  ChevronDown,
  Link2,
  Loader2,
  Unlink,
} from "lucide-react";
import { Button } from "@/components/ui/button";
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from "@/components/ui/card";
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
    <label className="flex min-w-0 flex-1 flex-col gap-1.5">
      <span className="text-[13px] font-medium text-foreground">{label}</span>
      <div className="relative" ref={ref}>
        <button
          type="button"
          disabled={disabled}
          onClick={() => setOpen((v) => !v)}
          className={cn(
            "flex h-9 w-full items-center gap-2 rounded-md border bg-card px-3 text-left text-sm transition-[border-color,box-shadow] duration-150 pointer-coarse:h-10",
            "disabled:cursor-not-allowed disabled:opacity-50",
            open
              ? "border-brand ring-3 ring-brand/20"
              : "border-input hover:border-subtle",
          )}
        >
          <span
            className={cn(
              "min-w-0 flex-1 truncate",
              selected ? "text-foreground" : "text-subtle",
            )}
          >
            {selected?.label || placeholder}
          </span>
          <ChevronDown
            className={cn(
              "size-4 shrink-0 text-subtle transition-transform duration-150",
              open && "rotate-180",
            )}
          />
        </button>
        {open && (
          <div className="absolute left-0 right-0 z-50 mt-1 overflow-hidden rounded-lg border border-border bg-popover shadow-popover">
            <div className="max-h-[min(14rem,40vh)] overflow-y-auto p-1">
              {options.length === 0 ? (
                <p className="px-3 py-6 text-center text-[13px] text-muted-foreground">
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
                        "flex h-8 w-full items-center gap-2 rounded-[4px] px-2 text-left text-sm transition-colors duration-150 pointer-coarse:h-10",
                        active
                          ? "bg-secondary text-foreground"
                          : "text-foreground hover:bg-secondary/60",
                      )}
                    >
                      <span className="min-w-0 flex-1 truncate">{opt.label}</span>
                      {active && <Check className="size-4 shrink-0 text-brand" />}
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
    <Card>
      <CardHeader>
        <CardTitle className="flex flex-wrap items-baseline gap-2">
          Attach database
          {scoped && lockedServiceName && (
            <span className="font-mono text-[13px] font-normal text-muted-foreground">
              {lockedServiceName}
            </span>
          )}
        </CardTitle>
        {!loading && (
          <CardDescription>
            {scoped
              ? `Link a managed database and inject its connection URL into ${lockedServiceName}. Redeploy afterward.`
              : "Link a managed database over the private Docker network and inject its connection URL into all services or one service. Redeploy afterward."}
          </CardDescription>
        )}
      </CardHeader>
      {loading ? (
        <CardContent className="flex items-center gap-2 text-[13px] text-muted-foreground">
          <Loader2 className="size-4 animate-spin" />
          Loading…
        </CardContent>
      ) : (
        <>
          <CardContent className="space-y-3">
            {dbs.length === 0 && (
              <p className="rounded-md border border-info/40 bg-secondary/60 px-3 py-2.5 text-[13px] leading-5 text-muted-foreground">
                No running managed databases. Create and deploy one under Databases.
              </p>
            )}

            <div
              className={cn(
                "grid gap-3",
                scoped ? "max-w-md grid-cols-1" : "grid-cols-1 sm:grid-cols-2",
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

            <div className="flex flex-col gap-3 sm:flex-row sm:items-center sm:justify-between">
              <label className="flex items-start gap-2 text-[13px] leading-5 text-muted-foreground">
                <input
                  type="checkbox"
                  checked={overwrite}
                  onChange={(e) => setOverwrite(e.target.checked)}
                  className="mt-0.5 size-4 shrink-0 rounded-sm border-input accent-brand"
                />
                <span>
                  Overwrite existing env key if present (e.g. manual{" "}
                  <span className="font-mono text-xs">DATABASE_URL</span>)
                </span>
              </label>

              <Button
                type="button"
                variant="outline"
                onClick={attach}
                disabled={busy || !dbId || dbs.length === 0}
                className="w-full shrink-0 sm:w-auto"
              >
                {busy ? <Loader2 className="animate-spin" /> : <Link2 />}
                Attach
                {scoped && lockedServiceName ? ` to ${lockedServiceName}` : ""}
              </Button>
            </div>
          </CardContent>

          {visibleLinks.length > 0 && (
            <ul className="divide-y divide-border border-t border-border">
              {visibleLinks.map((l) => (
                <li
                  key={l.id}
                  className="flex items-center justify-between gap-3 px-4 py-2.5"
                >
                  <div className="min-w-0">
                    <div className="truncate text-sm font-medium">
                      {l.database_project.name}
                    </div>
                    <div className="truncate font-mono text-xs text-muted-foreground">
                      {l.database_project.db_engine || "db"}
                      <span className="text-subtle"> · </span>
                      {l.service_name ? `service ${l.service_name}` : "all services"}
                      <span className="text-subtle"> · </span>
                      {l.env_key}
                    </div>
                  </div>
                  <Button
                    type="button"
                    variant="ghost"
                    size="sm"
                    disabled={busy}
                    onClick={() => unlink(l.database_project.id, l.id)}
                    className="shrink-0 text-muted-foreground hover:text-danger"
                  >
                    <Unlink />
                    Unlink
                  </Button>
                </li>
              ))}
            </ul>
          )}
        </>
      )}
    </Card>
  );
}
