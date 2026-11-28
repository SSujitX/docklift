import { useEffect, useMemo, useRef, useState } from "react";
import { useNavigate } from "react-router-dom";
import {
  ArrowLeft,
  Check,
  ChevronUp,
  Loader2,
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
        className="mb-4 inline-flex items-center gap-1.5 rounded-sm text-[13px] text-muted-foreground transition-colors duration-150 hover:text-foreground"
      >
        <ArrowLeft className="size-4 text-subtle" />
        Back to databases
      </button>

      <PageHeader
        title="New database"
        description="Pick an engine and a Docker Hub version. Tags refresh from the official image catalog."
      />

      <Card className="w-full">
        <div className="divide-y divide-border">
          <section className="p-4 sm:p-5">
            <div className="max-w-xl space-y-1.5">
              <label className="block text-[13px] font-medium" htmlFor="db-name">
                Name
              </label>
              <Input
                id="db-name"
                value={name}
                onChange={(e) => setName(e.target.value)}
                placeholder="my-postgres"
                maxLength={120}
              />
            </div>
          </section>

          <section className="space-y-2 p-4 sm:p-5">
            <label className="block text-[13px] font-medium">Engine</label>
            {loadingEngines ? (
              <div className="flex items-center gap-2 text-[13px] text-muted-foreground">
                <Loader2 className="size-4 animate-spin" />
                Loading engines…
              </div>
            ) : (
              <div className="grid grid-cols-1 gap-2 sm:grid-cols-2 lg:grid-cols-3">
                {engines.map((engine) => {
                  const active = engineId === engine.id;
                  return (
                    <button
                      key={engine.id}
                      type="button"
                      onClick={() => selectEngine(engine.id)}
                      className={cn(
                        "flex flex-col rounded-lg border bg-card p-3 text-left transition-colors duration-150",
                        active
                          ? "border-brand ring-1 ring-brand"
                          : "border-input hover:bg-secondary/50",
                      )}
                    >
                      <div className="flex items-center justify-between gap-2">
                        <span className="text-sm font-medium text-foreground">
                          {engine.label}
                        </span>
                        {active && <Check className="size-4 text-brand" />}
                      </div>
                      <p className="mt-1 text-[13px] leading-5 text-muted-foreground">
                        {engine.description}
                      </p>
                      <p className="mt-2 truncate font-mono text-xs text-muted-foreground">
                        {engine.imageRepo || engine.image}
                        <span className="text-subtle"> · </span>:{engine.port}
                        <span className="text-subtle"> · </span>
                        {engine.defaultEnvKey}
                      </p>
                    </button>
                  );
                })}
              </div>
            )}
          </section>

          <section className="space-y-1.5 p-4 sm:p-5">
            <label className="block text-[13px] font-medium" htmlFor="db-version">
              Version
            </label>

            {versions.length > 0 && (
              <div ref={versionPickerRef} className="relative w-full max-w-sm">
                <button
                  id="db-version"
                  type="button"
                  aria-haspopup="listbox"
                  aria-expanded={versionOpen}
                  onClick={() => setVersionOpen((o) => !o)}
                  className={cn(
                    "flex h-9 w-full items-center gap-2 rounded-md border bg-card px-3 text-left text-sm transition-[border-color,box-shadow] duration-150 pointer-coarse:h-10",
                    versionOpen
                      ? "border-brand ring-3 ring-brand/20"
                      : "border-input hover:border-subtle",
                  )}
                >
                  <Tag className="size-4 shrink-0 text-subtle" />
                  <span className="min-w-0 flex-1 truncate">
                    <span className="text-foreground">
                      {selectedVersion?.label || "Select version"}
                    </span>
                    {selectedVersion?.recommended && (
                      <span className="ml-2 text-xs text-subtle">
                        recommended
                      </span>
                    )}
                  </span>
                  {fullImage && (
                    <span className="hidden max-w-[40%] truncate font-mono text-xs text-muted-foreground sm:inline">
                      {fullImage}
                    </span>
                  )}
                  <ChevronUp
                    className={cn(
                      "size-4 shrink-0 text-subtle transition-transform duration-150",
                      !versionOpen && "rotate-180",
                    )}
                  />
                </button>

                {versionOpen && (
                  <div
                    role="listbox"
                    className="absolute left-0 right-0 top-full z-50 mt-1 overflow-hidden rounded-lg border border-border bg-popover shadow-popover"
                  >
                    <div className="max-h-[min(15rem,40vh)] overflow-y-auto overscroll-contain p-1">
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
                              "flex h-8 w-full items-center gap-2 rounded-[4px] px-2 text-left text-sm transition-colors duration-150 pointer-coarse:h-10",
                              active
                                ? "bg-secondary text-foreground"
                                : "text-foreground hover:bg-secondary/60",
                            )}
                          >
                            <span className="min-w-0 flex-1 truncate">
                              {v.label}
                              <span className="ml-2 font-mono text-xs text-muted-foreground">
                                :{v.tag}
                              </span>
                            </span>
                            {v.recommended && (
                              <span className="shrink-0 text-xs text-subtle">
                                recommended
                              </span>
                            )}
                            {active && (
                              <Check className="size-4 shrink-0 text-brand" />
                            )}
                          </button>
                        );
                      })}
                    </div>
                  </div>
                )}
              </div>
            )}
            <p className="text-xs leading-5 text-muted-foreground">
              Versions load from Docker Hub (majors + Alpine where available).
              {fullImage ? (
                <span className="block font-mono sm:hidden">{fullImage}</span>
              ) : null}
            </p>

            <div className="mt-4! max-w-xl rounded-md border border-info/40 bg-secondary/60 px-3 py-2.5 text-[13px] leading-5 text-muted-foreground">
              Host ports stay off. After deploy, link this database to a project
              or a specific service. Docklift joins the networks and injects the
              connection URL.
            </div>
          </section>

          <div className="flex flex-col gap-3 px-4 py-3 sm:flex-row sm:items-center sm:justify-between sm:px-5">
            <div className="min-w-0">
              <p className="truncate text-sm font-medium">
                {name.trim() || "Untitled database"}
              </p>
              <p className="mt-0.5 truncate font-mono text-xs text-muted-foreground">
                {selected?.label}
                {selected && versionTag && (
                  <>
                    <span className="text-subtle"> · </span>
                    {versionTag}
                  </>
                )}
              </p>
            </div>
            <Button
              type="button"
              size="lg"
              onClick={create}
              disabled={
                creating || loadingEngines || !name.trim() || !versionTag
              }
              className="w-full shrink-0 sm:w-auto"
            >
              {creating && <Loader2 className="animate-spin" />}
              Create and deploy
            </Button>
          </div>
        </div>
      </Card>
    </>
  );
}
