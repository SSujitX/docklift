// EnvVarsManager component - CRUD for project environment variables (build args and runtime)

import { useState, useEffect, useCallback, useRef } from "react";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Card } from "@/components/ui/card";
import { EnvVariable } from "@/lib/types";
import { API_URL } from "@/lib/utils";
import { authFetch } from "@/lib/auth";
import { Plus, Trash2, Eye, EyeOff, Key, Loader2, RotateCw, Shield, FlaskConical, Globe, AlertCircle, Info } from "lucide-react";
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
      <div className="flex items-center justify-center py-8 text-muted-foreground">
        <Loader2 className="h-5 w-5 animate-spin mr-2" />
        Loading...
      </div>
    );
  }

  return (
    <div className="space-y-6">
      {multiService && (
        <div className="rounded-lg border border-border bg-secondary/30 px-3 py-2.5 text-sm">
          <span className="font-medium text-foreground">{scopeLabel}</span>
          <p className="mt-0.5 text-xs text-muted-foreground">
            {serviceName
              ? "Only injected into this service’s build and runtime. Same key in Shared is overridden here."
              : "Injected into every service. Use a service-scoped key when values must differ."}
          </p>
        </div>
      )}

      {/* New Professional Input Section */}
      <Card className="overflow-hidden border-border p-0 shadow-none">
        <div className="flex items-center justify-between border-b border-border bg-secondary/30 px-5 py-3">
          <div className="flex items-center gap-2">
            <Plus className="h-4 w-4 text-muted-foreground" />
            <span className="text-xs font-medium text-muted-foreground">
              {showBulkImport ? "Bulk import" : "Add variable"}
              {multiService ? ` · ${scopeLabel}` : ""}
            </span>
          </div>
          <div className="flex items-center gap-3">
            <Button
              variant="ghost"
              size="sm"
              onClick={() => setShowBulkImport(!showBulkImport)}
              className="h-7 px-3 text-xs font-medium"
            >
              {showBulkImport ? "Single Mode" : "Bulk Import"}
            </Button>
            <div className="text-[10px] font-bold text-muted-foreground/60 flex items-center gap-1">
              <Shield className="h-3 w-3" /> SECURE STORAGE
            </div>
          </div>
        </div>
        
        <div className="p-6 space-y-6">
          {showBulkImport ? (
            <div className="space-y-4">
              <div className="space-y-2">
                <label className="text-[10px] font-bold text-muted-foreground uppercase tracking-wider ml-1">
                  Paste KEY=VALUE pairs (one per line)
                </label>
                <textarea
                  placeholder={`ADMIN_USERNAME=admin
DATABASE_URL=postgresql://user:pass@host:5432/db
SESSION_SECRET=your-secret-here
# Lines starting with # are ignored`}
                  value={bulkContent}
                  onChange={(e) => setBulkContent(e.target.value)}
                  className="w-full h-40 p-4 font-mono text-sm bg-background/50 border border-border/60 rounded-xl resize-none focus:outline-none focus:border-cyan-500/50"
                />
              </div>
              
              {/* Build/Runtime Toggles for Bulk Import */}
              <div className="flex items-center gap-6 p-3 rounded-xl bg-secondary/30 border border-border/40">
                <div className="flex items-center gap-2 group cursor-pointer" onClick={() => setIsBuildArg(!isBuildArg)}>
                  <div className={cn(
                    "h-4 w-4 rounded border transition-colors flex items-center justify-center",
                    isBuildArg ? "bg-orange-500 border-orange-500" : "bg-transparent border-muted-foreground/40"
                  )}>
                    {isBuildArg && <Check className="h-3 w-3 text-white" strokeWidth={4} />}
                  </div>
                  <div className="flex flex-col">
                    <span className="text-xs font-semibold">Build Argument</span>
                    <span className="text-[10px] text-muted-foreground">Used during Docker build</span>
                  </div>
                </div>

                <div className="flex items-center gap-2 group cursor-pointer" onClick={() => setIsRuntime(!isRuntime)}>
                  <div className={cn(
                    "h-4 w-4 rounded border transition-colors flex items-center justify-center",
                    isRuntime ? "bg-orange-500 border-orange-500" : "bg-transparent border-muted-foreground/40"
                  )}>
                    {isRuntime && <Check className="h-3 w-3 text-white" strokeWidth={4} />}
                  </div>
                  <div className="flex flex-col">
                    <span className="text-xs font-semibold">Runtime Variable</span>
                    <span className="text-[10px] text-muted-foreground">Available to your app</span>
                  </div>
                </div>
              </div>

              <div className="flex items-center justify-between">
                <p className="text-xs text-muted-foreground">
                  {bulkContent.split('\n').filter(l => l.trim() && !l.trim().startsWith('#') && l.includes('=')).length} variables detected
                </p>
                <Button 
                  onClick={handleBulkImport} 
                  disabled={bulkAdding} 
                  className="px-8 h-11 bg-cyan-600 hover:bg-cyan-500 text-white rounded-xl shadow-lg shadow-cyan-600/20 font-bold transition-all"
                >
                  {bulkAdding ? <Loader2 className="h-4 w-4 animate-spin mr-2" /> : <Plus className="h-4 w-4 mr-2" />}
                  Import All
                </Button>
              </div>
            </div>
          ) : (
          <>
          <div className="grid grid-cols-1 md:grid-cols-2 gap-6">
            <div className="space-y-2">
              <label className="text-[10px] font-bold text-muted-foreground uppercase tracking-wider ml-1">Key Name</label>
              <Input
                placeholder="e.g. DATABASE_URL"
                value={newKey}
                onChange={(e) => setNewKey(e.target.value.toUpperCase().replace(/\s+/g, '_'))}
                className="font-mono h-11 border-border/60 focus:border-cyan-500/50 bg-background/50"
              />
            </div>
            <div className="space-y-2">
              <label className="text-[10px] font-bold text-muted-foreground uppercase tracking-wider ml-1">Secret Value</label>
              <Input
                placeholder="••••••••••••"
                value={newValue}
                onChange={(e) => setNewValue(e.target.value)}
                className="h-11 border-border/60 focus:border-cyan-500/50 bg-background/50"
                type="password"
              />
            </div>
          </div>

          <div className="flex flex-col sm:flex-row items-start sm:items-center justify-between gap-4 p-4 rounded-xl bg-secondary/30 border border-border/40">
            <div className="flex items-center gap-6">
              <div className="flex items-center gap-2 group cursor-pointer" onClick={() => setIsBuildArg(!isBuildArg)}>
                <div className={cn(
                  "h-4 w-4 rounded border transition-colors flex items-center justify-center",
                  isBuildArg ? "bg-cyan-500 border-cyan-500" : "bg-transparent border-muted-foreground/40"
                )}>
                  {isBuildArg && <Check className="h-3 w-3 text-white" strokeWidth={4} />}
                </div>
                <div className="flex flex-col">
                  <span className="text-xs font-semibold">Build Argument</span>
                  <span className="text-[10px] text-muted-foreground">Used during Docker build</span>
                </div>
              </div>

              <div className="flex items-center gap-2 group cursor-pointer" onClick={() => setIsRuntime(!isRuntime)}>
                <div className={cn(
                  "h-4 w-4 rounded border transition-colors flex items-center justify-center",
                  isRuntime ? "bg-blue-500 border-blue-500" : "bg-transparent border-muted-foreground/40"
                )}>
                  {isRuntime && <Check className="h-3 w-3 text-white" strokeWidth={4} />}
                </div>
                <div className="flex flex-col">
                  <span className="text-xs font-semibold">Runtime Variable</span>
                  <span className="text-[10px] text-muted-foreground">Available to your app</span>
                </div>
              </div>

              {isBuildArg && (
                <div className="flex items-center gap-2 group cursor-pointer" onClick={() => setIsSecret(!isSecret)}>
                  <div className={cn(
                    "h-4 w-4 rounded border transition-colors flex items-center justify-center",
                    isSecret ? "bg-rose-500 border-rose-500" : "bg-transparent border-muted-foreground/40"
                  )}>
                    {isSecret && <Check className="h-3 w-3 text-white" strokeWidth={4} />}
                  </div>
                  <div className="flex flex-col">
                    <span className="text-xs font-semibold">BuildKit secret</span>
                    <span className="text-[10px] text-muted-foreground">Not --build-arg; needs secret mount in Dockerfile</span>
                  </div>
                </div>
              )}
            </div>

            <Button 
              onClick={handleAdd} 
              disabled={adding} 
              className="w-full sm:w-auto px-8 h-11 bg-cyan-600 hover:bg-cyan-500 text-white rounded-xl shadow-lg shadow-cyan-600/20 font-bold transition-all"
            >
              {adding ? <Loader2 className="h-4 w-4 animate-spin mr-2" /> : <Plus className="h-4 w-4 mr-2" />}
              Save Variable
            </Button>
          </div>
          </>
          )}
        </div>
      </Card>

      <div className="bg-amber-500/5 border border-amber-500/10 p-4 rounded-xl flex items-start gap-3">
        <AlertCircle className="h-5 w-5 text-amber-500 shrink-0 mt-0.5" />
        <div className="text-xs text-amber-600/80 leading-relaxed font-medium">
          <span className="font-bold text-amber-600">Redeploy Required:</span> Any changes here will not take effect on currently running containers. You must trigger a fresh deploy from the main project page to inject updated variables.
        </div>
      </div>

      <div className="bg-blue-500/5 border border-blue-500/10 p-4 rounded-xl flex items-start gap-3">
        <Info className="h-5 w-5 text-blue-500 shrink-0 mt-0.5" />
        <div className="text-xs text-blue-600/80 leading-relaxed font-medium">
          <span className="font-bold text-blue-600">Docker Build & Runtime:</span> If you are using these variables during build (e.g. Prisma), ensure you add <code className="bg-blue-500/10 px-1 rounded border border-blue-500/20">ARG VARIABLE_NAME</code> in your Dockerfile. For runtime access, consider adding <code className="bg-blue-500/10 px-1 rounded border border-blue-500/20">ENV VARIABLE_NAME=$VARIABLE_NAME</code> if your framework requires it.
        </div>
      </div>

      {/* Modern List Header */}
      <div className="flex items-center justify-between px-1">
        <h4 className="text-sm font-bold text-muted-foreground/80 flex items-center gap-2 uppercase tracking-widest">
          Active Environment ({envVars.length})
        </h4>
        <div className="flex items-center gap-4 text-[10px] font-bold text-muted-foreground/40 uppercase tracking-widest">
          <span className="flex items-center gap-1"><FlaskConical className="h-3 w-3" /> Build</span>
          <span className="flex items-center gap-1"><Globe className="h-3 w-3" /> Run</span>
        </div>
      </div>

      {/* List of variables */}
      {envVars.length === 0 ? (
        <Card className="p-20 text-center border-dashed border-border/60 bg-transparent flex flex-col items-center gap-4">
          <div className="h-16 w-16 rounded-full bg-secondary/30 flex items-center justify-center">
            <Key className="h-8 w-8 text-muted-foreground/20" />
          </div>
          <div className="space-y-1">
            <p className="font-bold text-muted-foreground/80">Ambient variables empty</p>
            <p className="text-xs text-muted-foreground max-w-[200px]">Define your application secrets in the form above.</p>
          </div>
        </Card>
      ) : (
        <Card className="divide-y divide-border/30 border-border/40 overflow-hidden shadow-xl shadow-black/5 bg-background/40 backdrop-blur-xl">
          {envVars.map((env) => (
            <div key={env.id} className="group flex flex-col sm:flex-row sm:items-center justify-between p-5 gap-4 hover:bg-secondary/20 transition-all">
              <div className="flex items-start gap-4 flex-1 min-w-0">
                <div className="h-10 w-10 rounded-xl bg-secondary/80 flex items-center justify-center shrink-0 group-hover:bg-background transition-colors">
                  <Key className="h-5 w-5 text-muted-foreground" />
                </div>
                <div className="flex flex-col gap-1.5 flex-1 min-w-0">
                  <div className="flex items-center gap-2">
                    <span className="font-mono text-sm font-bold text-foreground truncate">{env.key}</span>
                  </div>
                  <div className="flex items-center gap-3">
                    <p className="font-mono text-[11px] text-muted-foreground bg-secondary/50 px-2 py-1 rounded truncate flex-1 min-w-[120px]">
                      {visibleValues.has(env.id) ? env.value : maskValue(env.value)}
                    </p>
                    <button
                      onClick={() => toggleVisibility(env.id)}
                      className="text-muted-foreground hover:text-cyan-500 transition-colors p-1"
                    >
                      {visibleValues.has(env.id) ? (
                        <EyeOff className="h-3.5 w-3.5" />
                      ) : (
                        <Eye className="h-3.5 w-3.5" />
                      )}
                    </button>
                  </div>
                </div>
              </div>

              <div className="flex items-center justify-between sm:justify-end gap-6 sm:pl-4 border-t sm:border-t-0 pt-4 sm:pt-0 border-border/20">
                <div className="flex items-center gap-3">
                  <div className={cn(
                    "h-6 px-2.5 rounded-full flex items-center gap-1.5 text-[9px] font-bold uppercase tracking-widest transition-all",
                    env.is_build_arg 
                      ? "bg-orange-100 text-orange-600 border border-orange-300" 
                      : "bg-gray-100 text-gray-400 border border-gray-200"
                  )}>
                    <FlaskConical className="h-3 w-3" />
                    Bld
                  </div>
                  <div className={cn(
                    "h-6 px-2.5 rounded-full flex items-center gap-1.5 text-[9px] font-bold uppercase tracking-widest transition-all",
                    env.is_runtime 
                      ? "bg-orange-100 text-orange-600 border border-orange-300" 
                      : "bg-gray-100 text-gray-400 border border-gray-200"
                  )}>
                    <Globe className="h-3 w-3" />
                    Run
                  </div>
                </div>

                <div className="h-8 w-px bg-border/40 hidden sm:block" />

                <Button
                  variant="ghost"
                  size="sm"
                  onClick={() => handleDelete(env.id, env.key)}
                  className="h-9 w-9 p-0 rounded-xl text-destructive/40 hover:text-white hover:bg-destructive transition-all"
                >
                  <Trash2 className="h-4 w-4" />
                </Button>
              </div>
            </div>
          ))}
        </Card>
      )}
    </div>
  );
}
