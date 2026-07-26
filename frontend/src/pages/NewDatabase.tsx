import { useEffect, useMemo, useRef, useState } from "react";
import { useNavigate } from "react-router-dom";
import {
  ArrowLeft,
  Check,
  ChevronUp,
  Database,
  Loader2,
  Server,
  Sparkles,
  Tag,
} from "lucide-react";
import { PageHeader } from "@/components/shell/PageHeader";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Card } from "@/components/ui/card";
import { API_URL, cn } from "@/lib/utils";
import { authFetch } from "@/lib/auth";
import { toast } from "sonner";

interface EngineVersion {
  tag: string;
  label: string;
  recommended?: boolean;
}

interface Engine {
  id: string;
  label: string;
  description: string;
  image: string;
  imageRepo?: string;
  versions?: EngineVersion[];
  port: number;
  defaultEnvKey: string;
}

function recommendedTag(engine: Engine | undefined): string {
  if (!engine?.versions?.length) {
    const fromImage = engine?.image?.includes(":")
      ? engine.image.split(":").slice(1).join(":")
      : "";
    return fromImage || "";
  }
  return (
    engine.versions.find((v) => v.recommended)?.tag || engine.versions[0].tag
  );
}

export default function NewDatabasePage() {
  const navigate = useNavigate();
  const [engines, setEngines] = useState<Engine[]>([]);
  const [engineId, setEngineId] = useState("postgres");
  const [versionTag, setVersionTag] = useState("");
  const [versionOpen, setVersionOpen] = useState(false);
  const [name, setName] = useState("");
  const [loadingEngines, setLoadingEngines] = useState(true);
  const [creating, setCreating] = useState(false);
  const versionPickerRef = useRef<HTMLDivElement>(null);

  useEffect(() => {
    if (!versionOpen) return;
    const onDoc = (e: MouseEvent) => {
      if (
        versionPickerRef.current &&
        !versionPickerRef.current.contains(e.target as Node)
      ) {
        setVersionOpen(false);
      }
    };
    const onKey = (e: KeyboardEvent) => {
      if (e.key === "Escape") setVersionOpen(false);
    };
    document.addEventListener("mousedown", onDoc);
    document.addEventListener("keydown", onKey);
    return () => {
      document.removeEventListener("mousedown", onDoc);
      document.removeEventListener("keydown", onKey);
    };
  }, [versionOpen]);

  useEffect(() => {
    (async () => {
      try {
        const res = await authFetch(`${API_URL}/api/databases/engines`);
        if (!res.ok) throw new Error("Failed to load engines");
        const data = await res.json();
        const list: Engine[] = Array.isArray(data) ? data : [];
        setEngines(list);
        if (list[0]?.id) {
          setEngineId(list[0].id);
          setVersionTag(recommendedTag(list[0]));
        }
      } catch (err) {
        toast.error(err instanceof Error ? err.message : "Failed to load engines");
      } finally {
        setLoadingEngines(false);
      }
    })();
  }, []);

  const selected = useMemo(
    () => engines.find((e) => e.id === engineId),
    [engines, engineId],
  );

  const versions = selected?.versions?.length
    ? selected.versions
    : selected
      ? [
          {
            tag: recommendedTag(selected),
            label: recommendedTag(selected),
            recommended: true,
          },
        ]
      : [];

  const selectedVersion =
    versions.find((v) => v.tag === versionTag) || versions[0];

  const selectEngine = (id: string) => {
    setEngineId(id);
    const eng = engines.find((e) => e.id === id);
    setVersionTag(recommendedTag(eng));
    setVersionOpen(false);
  };

  const create = async () => {
    const trimmed = name.trim();
    if (!trimmed) {
      toast.error("Name is required");
      return;
    }
    if (!versionTag) {
      toast.error("Select a version");
      return;
    }
    setCreating(true);
    try {
      const res = await authFetch(`${API_URL}/api/databases`, {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({
          name: trimmed,
          engine: engineId,
          version: versionTag,
        }),
      });
      const data = await res.json().catch(() => ({}));
      if (!res.ok) throw new Error(data.error || "Create failed");

      toast.success("Database created — deploying now", {
        description: `${data.engine?.label || "Database"} ${data.version || versionTag}. Watch pull/logs on Deployments. Connection URL is on Overview after it’s running.`,
        duration: 7000,
      });

      // Start deploy in background; detail page polls deployment logs (pull is async).
      authFetch(`${API_URL}/api/deployments/${data.id}/deploy`, {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ trigger: "create" }),
      }).catch(() => {
        toast.error("Deploy didn’t start", {
          description: "Open the database and use Redeploy to try again.",
          duration: 8000,
        });
      });

      navigate(`/projects/${data.id}?tab=deployments`);
    } catch (err) {
      toast.error(err instanceof Error ? err.message : "Create failed", {
        duration: 7000,
      });
    } finally {
      setCreating(false);
    }
  };

  const fullImage =
    selected?.imageRepo && versionTag
      ? `${selected.imageRepo}:${versionTag}`
      : selected?.image || "";

  return (
    <>
      <button
        type="button"
        onClick={() => navigate("/databases")}
        className="mb-6 inline-flex items-center gap-2 text-sm text-muted-foreground transition-colors hover:text-foreground"
      >
        <ArrowLeft className="h-4 w-4" />
        Back to Databases
      </button>

      <PageHeader
        eyebrow="Deploy"
        title="New Database"
        description="Pick an engine and a Docker Hub version. Tags refresh from the official image catalog automatically."
        icon={Database}
      />

      <div className="w-full space-y-6 pb-28 sm:pb-24">
        <Card className="space-y-5 rounded-2xl border-border/60 p-5 sm:p-6">
          <div className="space-y-2">
            <label className="text-sm font-medium" htmlFor="db-name">
              Name
            </label>
            <Input
              id="db-name"
              value={name}
              onChange={(e) => setName(e.target.value)}
              placeholder="my-postgres"
              maxLength={120}
              className="h-11 bg-secondary/30"
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
              <div className="grid grid-cols-1 gap-2 sm:grid-cols-2 lg:grid-cols-3">
                {engines.map((engine) => (
                  <button
                    key={engine.id}
                    type="button"
                    onClick={() => selectEngine(engine.id)}
                    className={cn(
                      "rounded-2xl border p-4 text-left transition-colors",
                      engineId === engine.id
                        ? "border-brand/40 bg-brand/10"
                        : "border-border/60 bg-card/40 hover:bg-secondary/40",
                    )}
                  >
                    <div className="flex items-center gap-2 text-sm font-semibold">
                      <Server className="h-4 w-4 text-muted-foreground" />
                      {engine.label}
                    </div>
                    <p className="mt-1.5 text-xs leading-relaxed text-muted-foreground">
                      {engine.description}
                    </p>
                    <p className="mt-2 font-mono text-[10px] text-muted-foreground/80">
                      {engine.imageRepo || engine.image} · :{engine.port} ·{" "}
                      {engine.defaultEnvKey}
                    </p>
                  </button>
                ))}
              </div>
            )}
          </div>

          <div className="space-y-2">
            <label className="text-sm font-medium" htmlFor="db-version">
              Version
            </label>

            {versions.length > 0 && (
              <div
                ref={versionPickerRef}
                className="relative w-full max-w-sm sm:max-w-xs"
              >
                <button
                  id="db-version"
                  type="button"
                  aria-haspopup="listbox"
                  aria-expanded={versionOpen}
                  onClick={() => setVersionOpen((o) => !o)}
                  className={cn(
                    "flex h-10 w-full items-center gap-2 rounded-xl border border-border/60 bg-secondary/30 px-3 text-left text-sm transition-colors",
                    "hover:border-brand/40",
                    versionOpen && "border-brand/40 ring-2 ring-brand/15",
                  )}
                >
                  <Tag
                    className={cn(
                      "h-3.5 w-3.5 shrink-0",
                      versionOpen ? "text-brand" : "text-muted-foreground",
                    )}
                  />
                  <span className="min-w-0 flex-1 truncate">
                    <span className="font-medium">
                      {selectedVersion?.label || "Select version"}
                    </span>
                    {selectedVersion?.recommended && (
                      <span className="ml-1.5 text-[10px] font-semibold uppercase tracking-wider text-brand">
                        Rec
                      </span>
                    )}
                  </span>
                  {fullImage && (
                    <span className="hidden max-w-[40%] truncate font-mono text-[10px] text-muted-foreground sm:inline">
                      {fullImage}
                    </span>
                  )}
                  <ChevronUp
                    className={cn(
                      "h-4 w-4 shrink-0 text-muted-foreground transition-transform",
                      !versionOpen && "rotate-180",
                      versionOpen && "text-brand",
                    )}
                  />
                </button>

                {versionOpen && (
                  <div
                    role="listbox"
                    className="absolute bottom-full left-0 right-0 z-50 mb-1.5 overflow-hidden rounded-xl border border-border/60 bg-popover shadow-xl shadow-black/15"
                  >
                    <div className="max-h-[min(11rem,32vh)] overflow-y-auto overscroll-contain p-1">
                      {versions.map((v) => {
                        const active = v.tag === versionTag;
                        return (
                          <button
                            key={v.tag}
                            type="button"
                            role="option"
                            aria-selected={active}
                            onClick={() => {
                              setVersionTag(v.tag);
                              setVersionOpen(false);
                            }}
                            className={cn(
                              "flex w-full items-center gap-2 rounded-lg px-2.5 py-2 text-left text-sm transition-colors",
                              active
                                ? "bg-brand/10 font-medium text-brand"
                                : "text-foreground hover:bg-secondary/60",
                            )}
                          >
                            <span className="min-w-0 flex-1 truncate">
                              {v.label}
                              <span className="ml-1.5 font-mono text-[10px] font-normal text-muted-foreground">
                                :{v.tag}
                              </span>
                            </span>
                            {v.recommended && (
                              <span className="shrink-0 text-[9px] font-semibold uppercase tracking-wider text-brand/80">
                                Rec
                              </span>
                            )}
                            {active && (
                              <Check className="h-3.5 w-3.5 shrink-0" />
                            )}
                          </button>
                        );
                      })}
                    </div>
                  </div>
                )}
              </div>
            )}
            <p className="text-[11px] leading-relaxed text-muted-foreground">
              Versions load from Docker Hub (majors + Alpine where available).
              {fullImage ? (
                <span className="mt-0.5 block font-mono text-[10px] sm:hidden">
                  {fullImage}
                </span>
              ) : null}
            </p>
          </div>

          <div className="rounded-xl border border-border/60 bg-secondary/20 px-4 py-3 text-xs leading-relaxed text-muted-foreground">
            Host ports stay off. After deploy, link this database to a project or a
            specific service — Docklift joins networks and injects the connection URL.
          </div>
        </Card>

        <div className="pointer-events-none fixed inset-x-0 bottom-0 z-20 lg:left-[var(--shell-rail,17rem)]">
          <div className="pointer-events-auto border-t border-border/60 bg-background/90 px-3 py-3 backdrop-blur-xl sm:px-4 md:px-8">
            <div className="mx-auto flex w-full max-w-7xl flex-col gap-3 sm:flex-row sm:items-center sm:justify-between sm:gap-4">
              <div className="min-w-0">
                <p className="truncate text-sm font-semibold tracking-tight">
                  {name.trim() || "Untitled database"}
                </p>
                <div className="mt-1.5 flex flex-wrap gap-1.5">
                  {selected && (
                    <span className="inline-flex items-center rounded-lg border border-brand/25 bg-brand/10 px-2 py-0.5 text-[10px] font-medium text-brand">
                      {selected.label}
                    </span>
                  )}
                  {versionTag && (
                    <span className="inline-flex items-center rounded-lg border border-border/60 bg-secondary/40 px-2 py-0.5 font-mono text-[10px] font-medium text-muted-foreground">
                      {versionTag}
                    </span>
                  )}
                </div>
              </div>
              <Button
                type="button"
                onClick={create}
                disabled={
                  creating || loadingEngines || !name.trim() || !versionTag
                }
                className="h-11 w-full shrink-0 bg-brand px-6 font-semibold text-brand-foreground shadow-lg shadow-brand/20 hover:brightness-110 sm:w-auto sm:min-w-[11rem]"
              >
                {creating ? (
                  <Loader2 className="mr-2 h-4 w-4 animate-spin" />
                ) : (
                  <Sparkles className="mr-2 h-4 w-4" />
                )}
                Create & deploy
              </Button>
            </div>
          </div>
        </div>
      </div>
    </>
  );
}
