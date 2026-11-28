// EnvVarsManager component - CRUD for project environment variables (build args and runtime)

import { useState, useEffect, useCallback, useRef } from "react";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { EnvVariable } from "@/lib/types";
import { API_URL } from "@/lib/utils";
import { authFetch } from "@/lib/auth";
import { Trash2, Eye, EyeOff, Loader2, Shield } from "lucide-react";
import { toast } from "sonner";
import { cn } from "@/lib/utils";
import { Check } from "lucide-react";

interface EnvVarsManagerProps {
  projectId: string;
  /** Empty string = shared across all services. Service name = scoped to that app. */
  serviceName?: string;
  /** When true, show shared vs service framing copy. */
  multiService?: boolean;
}

export function EnvVarsManager({
  projectId,
  serviceName = "",
  multiService = false,
}: EnvVarsManagerProps) {
  const [envVars, setEnvVars] = useState<EnvVariable[]>([]);
  const [loading, setLoading] = useState(true);
  const [adding, setAdding] = useState(false);
  const [newKey, setNewKey] = useState("");
  const [newValue, setNewValue] = useState("");
  const [isBuildArg, setIsBuildArg] = useState(true);
  const [isRuntime, setIsRuntime] = useState(true);
  const [isSecret, setIsSecret] = useState(false);
  const [visibleValues, setVisibleValues] = useState<Set<string>>(new Set());
  const [showBulkImport, setShowBulkImport] = useState(false);
  const [bulkContent, setBulkContent] = useState("");
  const [bulkAdding, setBulkAdding] = useState(false);

  const scopeLabel = serviceName
    ? `Service · ${serviceName}`
    : multiService
      ? "Shared · all services"
      : "Project";

  const scopeRef = useRef(serviceName);
  const projectIdRef = useRef(projectId);
  const envFetchGen = useRef(0);
  scopeRef.current = serviceName;
  projectIdRef.current = projectId;

  const fetchEnvVars = useCallback(async (forScope?: string) => {
    const scope = forScope ?? scopeRef.current;
    const pid = projectIdRef.current;
    const gen = ++envFetchGen.current;
    setLoading(true);
    setEnvVars([]);
    try {
      const q = new URLSearchParams({ service: scope });
      const res = await authFetch(
        `${API_URL}/api/projects/${pid}/env?${q.toString()}`,
      );
      if (gen !== envFetchGen.current) return;
      if (scope !== scopeRef.current || pid !== projectIdRef.current) return;
      if (!res.ok) {
        toast.error("Failed to load environment variables");
        return;
      }
      const data = await res.json();
      if (gen !== envFetchGen.current) return;
      if (scope !== scopeRef.current || pid !== projectIdRef.current) return;
      setEnvVars(data);
    } catch (error) {
      if (gen !== envFetchGen.current) return;
      console.error(error);
      toast.error("Failed to load environment variables");
    } finally {
      if (gen === envFetchGen.current) setLoading(false);
    }
  }, []);

  useEffect(() => {
    fetchEnvVars(serviceName);
    return () => {
      envFetchGen.current += 1;
    };
  }, [projectId, serviceName, fetchEnvVars]);

  const handleAdd = async () => {
    if (!newKey.trim() || !newValue.trim()) {
      toast.error("Key and value are required");
      return;
    }

    const scopeAtStart = serviceName;
    setAdding(true);
    try {
      const res = await authFetch(`${API_URL}/api/projects/${projectId}/env`, {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({
          key: newKey.trim(),
          value: newValue.trim(),
          is_build_arg: isBuildArg,
          is_runtime: isRuntime,
          is_secret: isSecret,
          service_name: scopeAtStart,
        }),
      });

      if (res.ok) {
        setNewKey("");
        setNewValue("");
        setIsSecret(false);
        if (scopeRef.current === scopeAtStart) {
          await fetchEnvVars(scopeAtStart);
        }
        toast.success("Environment variable added");
      } else {
        const err = await res.json().catch(() => ({}));
        toast.error((err as { error?: string }).error || "Failed to add variable");
      }
    } catch (error) {
      toast.error("Failed to add variable");
    } finally {
      setAdding(false);
    }
  };

  const handleBulkImport = async () => {
    if (!bulkContent.trim()) {
      toast.error("Paste your environment variables");
      return;
    }

    const scopeAtStart = serviceName;
    setBulkAdding(true);
    try {
      const res = await authFetch(`${API_URL}/api/projects/${projectId}/env/bulk`, {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({
          content: bulkContent,
          is_build_arg: isBuildArg,
          is_runtime: isRuntime,
          service_name: scopeAtStart,
        }),
      });

      if (res.ok) {
        const data = await res.json();
        setBulkContent("");
        setShowBulkImport(false);
        if (scopeRef.current === scopeAtStart) {
          await fetchEnvVars(scopeAtStart);
        }
        toast.success(data.message);
      } else {
        const err = await res.json();
        toast.error(err.error || "Failed to import");
      }
    } catch (error) {
      toast.error("Failed to import variables");
    } finally {
      setBulkAdding(false);
    }
  };

  const handleDelete = async (envId: string, key: string) => {
    const scopeAtStart = serviceName;
    try {
      const res = await authFetch(`${API_URL}/api/projects/${projectId}/env/${envId}`, {
        method: "DELETE",
      });
      if (res.ok) {
        if (scopeRef.current === scopeAtStart) {
          await fetchEnvVars(scopeAtStart);
        }
        toast.success(`Removed ${key}`);
      }
    } catch (error) {
      toast.error("Failed to delete");
    }
  };

  const toggleVisibility = (id: string) => {
    const newVisible = new Set(visibleValues);
    if (newVisible.has(id)) {
      newVisible.delete(id);
    } else {
      newVisible.add(id);
    }
    setVisibleValues(newVisible);
  };

  const maskValue = (value: string) => "•".repeat(Math.min(value.length, 20));

  if (loading) {
    return (
      <div className="flex items-center justify-center gap-2 rounded-lg border border-border bg-card py-10 text-[13px] text-muted-foreground">
        <Loader2 className="size-4 animate-spin" />
        Loading…
      </div>
    );
  }

  return (
    <div className="space-y-5">
      {multiService && (
        <div className="rounded-md border border-info/40 bg-secondary/60 px-3 py-2.5 text-[13px]">
          <span className="font-medium text-foreground">{scopeLabel}</span>
          <p className="mt-0.5 text-muted-foreground">
            {serviceName
              ? "Only injected into this service’s build and runtime. Same key in Shared is overridden here."
              : "Injected into every service. Use a service-scoped key when values must differ."}
          </p>
        </div>
      )}

      {/* Variables */}
      <section className="rounded-lg border border-border bg-card">
        <div className="flex items-center justify-between gap-3 border-b border-border px-4 py-3">
          <h3 className="text-[15px] font-semibold leading-5">
            Variables{" "}
            <span className="font-mono text-[13px] font-normal text-subtle">
              {envVars.length}
            </span>
          </h3>
          <span className="hidden text-xs text-muted-foreground sm:inline">
            Values stay masked until revealed
          </span>
        </div>

        {envVars.length === 0 ? (
          <div className="px-4 py-14 text-center">
            <p className="text-[15px] font-semibold">No variables yet</p>
            <p className="mx-auto mt-1 max-w-sm text-[13.5px] text-muted-foreground">
              Add a key and value below. They are injected on the next deploy.
            </p>
          </div>
        ) : (
          <>
            <div className="hidden h-9 grid-cols-[minmax(0,1fr)_minmax(0,1.3fr)_9rem_2rem] items-center gap-4 border-b border-border px-4 text-xs font-medium text-muted-foreground md:grid">
              <span>Key</span>
              <span>Value</span>
              <span>Used in</span>
              <span />
            </div>
            <ul className="divide-y divide-border">
              {envVars.map((env) => (
                <li
                  key={env.id}
                  className="group grid min-h-11 grid-cols-[minmax(0,1fr)_auto] items-center gap-x-4 gap-y-0.5 px-4 py-2 transition-colors duration-150 hover:bg-secondary/50 md:grid-cols-[minmax(0,1fr)_minmax(0,1.3fr)_9rem_2rem] md:py-1.5"
                >
                  <span className="min-w-0 truncate font-mono text-[13px] font-medium text-foreground">
                    {env.key}
                  </span>

                  <div className="col-start-1 row-start-2 flex min-w-0 items-center gap-1 md:col-start-auto md:row-start-auto">
                    <span className="min-w-0 truncate font-mono text-[13px] text-muted-foreground">
                      {visibleValues.has(env.id) ? env.value : maskValue(env.value)}
                    </span>
                    <button
                      onClick={() => toggleVisibility(env.id)}
                      className="inline-flex size-7 shrink-0 items-center justify-center rounded-md text-subtle transition-colors duration-150 hover:bg-secondary hover:text-foreground pointer-coarse:size-9"
                    >
                      {visibleValues.has(env.id) ? (
                        <EyeOff className="size-3.5" />
                      ) : (
                        <Eye className="size-3.5" />
                      )}
                    </button>
                  </div>

                  <div className="col-start-2 row-start-2 flex flex-wrap items-center justify-end gap-1.5 md:col-start-auto md:row-start-auto md:justify-start">
                    {env.is_build_arg && <Tag>Build</Tag>}
                    {env.is_runtime && <Tag>Runtime</Tag>}
                    {env.is_secret && <Tag>Secret</Tag>}
                    {!env.is_build_arg && !env.is_runtime && (
                      <span className="text-xs text-subtle">—</span>
                    )}
                  </div>

                  <Button
                    variant="ghost"
                    size="icon"
                    onClick={() => handleDelete(env.id, env.key)}
                    className="col-start-2 row-start-1 justify-self-end text-subtle opacity-60 hover:text-danger group-hover:opacity-100 focus-visible:opacity-100 md:col-start-auto md:row-start-auto"
                  >
                    <Trash2 />
                  </Button>
                </li>
              ))}
            </ul>
          </>
        )}
      </section>

      {/* Add / bulk import */}
      <section className="rounded-lg border border-border bg-card">
        <div className="flex items-center justify-between gap-3 border-b border-border px-4 py-3">
          <div className="min-w-0">
            <h3 className="text-[15px] font-semibold leading-5">
              {showBulkImport ? "Bulk import" : "Add variable"}
            </h3>
            <p className="mt-0.5 flex items-center gap-1.5 text-[13px] text-muted-foreground">
              <Shield className="size-3.5 shrink-0 text-subtle" />
              <span className="truncate">
                Secure storage{multiService ? ` · ${scopeLabel}` : ""}
              </span>
            </p>
          </div>
          <Button
            variant="outline"
            size="sm"
            onClick={() => setShowBulkImport(!showBulkImport)}
            className="shrink-0"
          >
            {showBulkImport ? "Single variable" : "Bulk import"}
          </Button>
        </div>

        {showBulkImport ? (
          <>
            <div className="space-y-4 p-4 sm:p-5">
              <div className="space-y-1.5">
                <label className="text-[13px] font-medium">
                  Paste <span className="font-mono">KEY=VALUE</span> pairs, one per line
                </label>
                <textarea
                  placeholder={`ADMIN_USERNAME=admin
DATABASE_URL=postgresql://user:pass@host:5432/db
SESSION_SECRET=your-secret-here
# Lines starting with # are ignored`}
                  value={bulkContent}
                  onChange={(e) => setBulkContent(e.target.value)}
                  className="h-40 w-full resize-none rounded-md border border-input bg-card px-3 py-2 font-mono text-[13px] transition-colors placeholder:text-subtle focus-visible:border-brand focus-visible:outline-hidden focus-visible:ring-3 focus-visible:ring-brand/20"
                />
              </div>

              <div className="flex flex-wrap gap-x-6 gap-y-3">
                <Option
                  checked={isBuildArg}
                  onToggle={() => setIsBuildArg(!isBuildArg)}
                  label="Build argument"
                  hint="Used during Docker build"
                />
                <Option
                  checked={isRuntime}
                  onToggle={() => setIsRuntime(!isRuntime)}
                  label="Runtime variable"
                  hint="Available to your app"
                />
              </div>
            </div>

            <div className="flex items-center justify-between gap-3 border-t border-border px-4 py-3">
              <p className="text-xs text-muted-foreground">
                <span className="font-mono">
                  {bulkContent.split('\n').filter(l => l.trim() && !l.trim().startsWith('#') && l.includes('=')).length}
                </span>{" "}
                variables detected
              </p>
              <Button onClick={handleBulkImport} disabled={bulkAdding}>
                {bulkAdding && <Loader2 className="animate-spin" />}
                Import all
              </Button>
            </div>
          </>
        ) : (
          <>
            <div className="space-y-4 p-4 sm:p-5">
              <div className="grid grid-cols-1 gap-4 md:grid-cols-2">
                <div className="space-y-1.5">
                  <label className="text-[13px] font-medium">Key</label>
                  <Input
                    placeholder="DATABASE_URL"
                    value={newKey}
                    onChange={(e) => setNewKey(e.target.value.toUpperCase().replace(/\s+/g, '_'))}
                    className="font-mono"
                  />
                </div>
                <div className="space-y-1.5">
                  <label className="text-[13px] font-medium">Value</label>
                  <Input
                    placeholder="••••••••••••"
                    value={newValue}
                    onChange={(e) => setNewValue(e.target.value)}
                    className="font-mono"
                    type="password"
                  />
                </div>
              </div>

              <div className="flex flex-wrap gap-x-6 gap-y-3">
                <Option
                  checked={isBuildArg}
                  onToggle={() => setIsBuildArg(!isBuildArg)}
                  label="Build argument"
                  hint="Used during Docker build"
                />
                <Option
                  checked={isRuntime}
                  onToggle={() => setIsRuntime(!isRuntime)}
                  label="Runtime variable"
                  hint="Available to your app"
                />
                {isBuildArg && (
                  <Option
                    checked={isSecret}
                    onToggle={() => setIsSecret(!isSecret)}
                    label="BuildKit secret"
                    hint="Not --build-arg; needs a secret mount in the Dockerfile"
                  />
                )}
              </div>
            </div>

            <div className="flex justify-end border-t border-border px-4 py-3">
              <Button onClick={handleAdd} disabled={adding} className="w-full sm:w-auto">
                {adding && <Loader2 className="animate-spin" />}
                Save variable
              </Button>
            </div>
          </>
        )}
      </section>

      <div className="rounded-md border border-warn/40 bg-secondary/60 px-3 py-2.5 text-[13px] leading-5 text-muted-foreground">
        <span className="font-medium text-foreground">Redeploy required.</span>{" "}
        Changes here do not take effect on running containers. Trigger a fresh deploy from
        the project page to inject updated variables.
      </div>

      <div className="rounded-md border border-info/40 bg-secondary/60 px-3 py-2.5 text-[13px] leading-5 text-muted-foreground">
        <span className="font-medium text-foreground">Docker build and runtime.</span>{" "}
        If you use these variables during build (e.g. Prisma), add{" "}
        <code className="font-mono text-foreground">ARG VARIABLE_NAME</code> to your Dockerfile.
        For runtime access, consider adding{" "}
        <code className="font-mono text-foreground">ENV VARIABLE_NAME=$VARIABLE_NAME</code> if
        your framework requires it.
      </div>
    </div>
  );
}

function Tag({ children }: { children: React.ReactNode }) {
  return (
    <span className="rounded-sm border border-border px-1.5 text-xs leading-5 text-muted-foreground">
      {children}
    </span>
  );
}

/** Checkbox-style toggle row (click target is the whole row, as before). */
function Option({
  checked,
  onToggle,
  label,
  hint,
}: {
  checked: boolean;
  onToggle: () => void;
  label: string;
  hint: string;
}) {
  return (
    <div className="flex cursor-pointer items-start gap-2" onClick={onToggle}>
      <div
        className={cn(
          "mt-0.5 flex size-4 shrink-0 items-center justify-center rounded-sm border transition-colors duration-150",
          checked ? "border-brand bg-brand" : "border-input bg-card",
        )}
      >
        {checked && <Check className="size-3 stroke-[3] text-brand-foreground" />}
      </div>
      <div className="flex flex-col">
        <span className="text-[13px] font-medium leading-5">{label}</span>
        <span className="text-xs text-muted-foreground">{hint}</span>
      </div>
    </div>
  );
}
