// New project page - wizard for creating projects via GitHub, public repo, or file upload

import { useState, useEffect, useRef, Suspense } from "react";
import { useNavigate, useSearchParams } from "react-router-dom";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Textarea } from "@/components/ui/textarea";
import { Card } from "@/components/ui/card";
import { BranchSelector, type GitRefKind } from "@/components/BranchSelector";
import { GithubIcon } from "@/components/icons/GithubIcon";
import { PageHeader } from "@/components/shell/PageHeader";
import {
  Upload,
  FolderUp,
  Loader2,
  Check,
  Lock,
  Search,
  Globe,
  ArrowLeft,
  ArrowRight,
  Plus as PlusIcon,
  Trash2,
  X,
  Eye,
  EyeOff,
} from "lucide-react";
import { API_URL, cn } from "@/lib/utils";
import { authFetch, startGithubInstallAndNavigate } from "@/lib/auth";
import { toast } from "sonner";
import {
  Dialog,
  DialogContent,
  DialogFooter,
  DialogHeader,
  DialogTitle,
  DialogTrigger,
} from "@/components/ui/dialog";
import { GitHubConnect } from "@/components/GitHubConnect";

interface GitHubRepo {
  id: number;
  name: string;
  full_name: string;
  owner?: string;
  private: boolean;
  clone_url: string;
  html_url: string;
  description: string | null;
  default_branch: string;
  updated_at: string;
}

interface GitHubStatus {
  connected: boolean;
  username?: string;
  avatar_url?: string;
}

interface GitHubInstallation {
  id: number;
  login: string;
  avatar_url: string;
  type: "User" | "Organization" | string;
}

function NewProjectContent() {
  const navigate = useNavigate();
  const [searchParams] = useSearchParams();
  const [step, setStep] = useState(1);
  const [loading, setLoading] = useState(false);

  // Form State
  const [sourceType, setSourceType] = useState<"upload" | "github" | "public">(
    searchParams.get("github") === "connected" ? "github" : "public"
  );
  const [projectType, setProjectType] = useState<"app" | "database">("app");
  const [name, setName] = useState("");
  const [githubUrl, setGithubUrl] = useState("");
  const [githubBranch, setGithubBranch] = useState("");
  const [files, setFiles] = useState<FileList | null>(null);
  const [dragActive, setDragActive] = useState(false);
  const [buildType, setBuildType] = useState<"auto" | "dockerfile" | "railpack">("auto");
  const [baseDirectory, setBaseDirectory] = useState(".");
  const [dockerfilePath, setDockerfilePath] = useState("Dockerfile");
  const [internalPort, setInternalPort] = useState(3000);

  // Environment Variables State
  const [envVars, setEnvVars] = useState<{key: string, value: string, is_build_arg: boolean, is_runtime: boolean}[]>([]);
  const [newEnvKey, setNewEnvKey] = useState("");
  const [newEnvValue, setNewEnvValue] = useState("");
  const [newEnvIsBuild, setNewEnvIsBuild] = useState(false);
  const [newEnvIsRuntime, setNewEnvIsRuntime] = useState(true);

  const [showAddEnv, setShowAddEnv] = useState(false);
  const [showBulkEnv, setShowBulkEnv] = useState(false);
  const [bulkEnvContent, setBulkEnvContent] = useState("");
  const [bulkIsBuild, setBulkIsBuild] = useState(true);
  const [bulkIsRuntime, setBulkIsRuntime] = useState(true);
  const [revealedEnvs, setRevealedEnvs] = useState<number[]>([]);

  // GitHub state
  const [githubStatus, setGithubStatus] = useState<GitHubStatus | null>(null);
  const [githubLoading, setGithubLoading] = useState(false);
  const [githubInstallations, setGithubInstallations] = useState<GitHubInstallation[]>([]);
  /** null = all connected accounts; otherwise filter by login */
  const [accountFilter, setAccountFilter] = useState<string | null>(null);
  const [repos, setRepos] = useState<GitHubRepo[]>([]);
  const [reposLoading, setReposLoading] = useState(false);
  const [searchQuery, setSearchQuery] = useState("");
  const [selectedRepo, setSelectedRepo] = useState<GitHubRepo | null>(null);
  const [showGitHubConnect, setShowGitHubConnect] = useState(false);
  const [branches, setBranches] = useState<string[]>([]);
  const [tags, setTags] = useState<string[]>([]);
  const [branchesLoading, setBranchesLoading] = useState(false);
  const [tagsLoading, setTagsLoading] = useState(false);
  const [refKind, setRefKind] = useState<GitRefKind>("branch");
  const refKindRef = useRef<GitRefKind>("branch");
  const [repoAccessError, setRepoAccessError] = useState<string | null>(null);
  /** Sticky warning when /repos is partial or single-install fallback */
  const [reposWarning, setReposWarning] = useState<string | null>(null);

  useEffect(() => {
    refKindRef.current = refKind;
  }, [refKind]);

  useEffect(() => {
    fetchGitHubStatus();
  }, []);

  // Debounced branch + tag fetch for public repo
  useEffect(() => {
    if (sourceType === "public" && githubUrl && isValidGithubUrl(githubUrl)) {
      setBranches([]);
      setTags([]);
      setGithubBranch("");
      setRefKind("branch");
      setRepoAccessError(null);
      const timer = setTimeout(() => {
        const match = githubUrl.match(/github\.com\/([^\/]+\/[^\/]+)/);
        if (match) {
          const repoPath = match[1].replace(/\.git$/, "").replace(/\/$/, "");
          fetchRefs(repoPath, "public");
        }
      }, 600);
      return () => clearTimeout(timer);
    } else if (sourceType === "public") {
      setBranches([]);
      setTags([]);
      setRepoAccessError(null);
    }
  }, [githubUrl, sourceType]);

  const pickDefaultBranch = (list: string[], current: string) => {
    if (list.includes(current)) return current;
    if (list.includes("main")) return "main";
    if (list.includes("master")) return "master";
    return list[0] || "";
  };

  const handleRefKindChange = (next: GitRefKind) => {
    // No tags → stay on Branch (ignore Tag mode)
    if (next === "tag" && tags.length === 0) {
      if (!tagsLoading) {
        toast.message("This repository has no tags — using a branch instead");
      }
      return;
    }
    setRefKind(next);
    if (next === "branch") {
      setGithubBranch((prev) => pickDefaultBranch(branches, prev));
    } else {
      // No selection → latest tag (API returns newest-first)
      setGithubBranch((prev) => (tags.includes(prev) ? prev : tags[0]));
    }
  };

  const fetchRefs = async (repoIdentifier: string, type: "public" | "private") => {
    setBranchesLoading(true);
    setTagsLoading(true);
    setRepoAccessError(null);
    try {
      const [branchesRes, tagsRes] = await Promise.all([
        authFetch(`${API_URL}/api/github/branches?repo=${encodeURIComponent(repoIdentifier)}&type=${type}`),
        authFetch(`${API_URL}/api/github/tags?repo=${encodeURIComponent(repoIdentifier)}&type=${type}`),
      ]);

      if (!branchesRes.ok) {
        const errorData = await branchesRes.json().catch(() => ({}));
        if (branchesRes.status === 404) {
          if (type === "public") {
            setRepoAccessError("Repository not found or is private. For private repos, use the 'Private Repository' tab.");
          }
          throw new Error("Not found");
        }
        if (branchesRes.status === 403) {
          if (type === "public") {
            setRepoAccessError("Cannot access this repository. It may be private - please use the 'Private Repository' tab.");
          }
          throw new Error("Forbidden");
        }
        throw new Error(errorData.error || "Failed");
      }

      const branchList: string[] = await branchesRes.json();
      setBranches(Array.isArray(branchList) ? branchList : []);
      setRepoAccessError(null);

      let tagList: string[] = [];
      if (tagsRes.ok) {
        const raw = await tagsRes.json();
        tagList = Array.isArray(raw) ? raw : [];
      } else if (type === "private") {
        toast.warning("Could not load tags for this repository");
      }
      setTags(tagList);

      // Tag mode with no tags → fall back to Branch
      if (refKindRef.current === "tag" && tagList.length === 0) {
        setRefKind("branch");
        refKindRef.current = "branch";
        setGithubBranch((prev) => pickDefaultBranch(branchList, prev));
      } else {
        setGithubBranch((prev) => {
          if (refKindRef.current === "tag") {
            // Empty/invalid selection → latest tag
            return tagList.includes(prev) ? prev : tagList[0];
          }
          return pickDefaultBranch(branchList, prev);
        });
      }
    } catch {
      setBranches([]);
      setTags([]);
      if (type === "private") toast.error("Failed to fetch branches");
    } finally {
      setBranchesLoading(false);
      setTagsLoading(false);
    }
  };

  const fetchInstallations = async () => {
    try {
      const res = await authFetch(`${API_URL}/api/github/installations`);
      const data = await res.json();
      if (!res.ok) {
        setGithubInstallations([]);
        setAccountFilter(null);
        return;
      }
      const list: GitHubInstallation[] = data.installations || [];
      setGithubInstallations(list);
      setAccountFilter((prev) =>
        prev && !list.some((i) => i.login === prev) ? null : prev,
      );
    } catch {
      setGithubInstallations([]);
      setAccountFilter(null);
    }
  };

  const fetchGitHubStatus = async () => {
    setGithubLoading(true);
    try {
      const res = await authFetch(`${API_URL}/api/github/status`);
      const data = await res.json();
      setGithubStatus(data);
      if (data.connected) {
        await Promise.all([fetchInstallations(), fetchRepos()]);
      } else {
        setGithubInstallations([]);
        setRepos([]);
      }
    } catch {
      setGithubStatus({ connected: false });
      setGithubInstallations([]);
    } finally {
      setGithubLoading(false);
    }
  };

  const fetchRepos = async () => {
    setReposLoading(true);
    try {
      // Always load the union of every installation — account chips filter client-side
      const res = await authFetch(`${API_URL}/api/github/repos`);
      const data = await res.json();
      if (!res.ok) {
        setRepos([]);
        setReposWarning(null);
        toast.error(data?.error || "Failed to fetch repositories");
        return;
      }
      // Prefer envelope { repositories, failedInstallations }; accept legacy bare array
      const list: GitHubRepo[] = Array.isArray(data)
        ? data
        : Array.isArray(data?.repositories)
          ? data.repositories
          : [];
      setRepos(list);

      const failed: { login?: string }[] = Array.isArray(data?.failedInstallations)
        ? data.failedInstallations
        : [];
      if (data?.fallbackSingle) {
        setReposWarning(
          "Showing only the saved GitHub install — could not list all accounts. Retry or reconnect in Settings.",
        );
      } else if (failed.length > 0) {
        const names = failed.map((f) => `@${f.login || "?"}`).join(", ");
        setReposWarning(`Could not load repos for: ${names}`);
        toast.warning(`Could not load repos for: ${names}`);
      } else {
        setReposWarning(null);
      }
    } catch {
      setRepos([]);
      setReposWarning(null);
      toast.error("Failed to fetch repositories");
    } finally {
      setReposLoading(false);
    }
  };

  const handleAddGithubAccount = async () => {
    try {
      await startGithubInstallAndNavigate(window.location.href, { sameTab: true });
    } catch (error) {
      toast.error(error instanceof Error ? error.message : "Failed to start GitHub install");
    }
  };

  const handleConnectGitHub = async () => {
    try {
      await startGithubInstallAndNavigate(window.location.href, { sameTab: true });
    } catch (error) {
      toast.error(error instanceof Error ? error.message : "Failed to start GitHub install");
    }
  };

  const handleSelectRepo = (repo: GitHubRepo) => {
    setSelectedRepo(repo);
    setName(repo.name);
    setGithubUrl(repo.clone_url);
    setGithubBranch(repo.default_branch);
    setRefKind("branch");
    fetchRefs(repo.full_name, "private");
    setStep(2);
  };

  const repoOwner = (repo: GitHubRepo) =>
    (repo.owner || repo.full_name.split("/")[0] || "").toLowerCase();

  const filteredRepos = repos.filter((repo) => {
    if (accountFilter && repoOwner(repo) !== accountFilter.toLowerCase()) {
      return false;
    }
    const q = searchQuery.toLowerCase().trim();
    if (!q) return true;
    return (
      repo.name.toLowerCase().includes(q) ||
      repo.full_name.toLowerCase().includes(q) ||
      repoOwner(repo).includes(q)
    );
  });

  const handleSubmit = async () => {
    if (!name) {
      toast.error("Project name is required");
      return;
    }
    if (!baseDirectory.trim()) {
      toast.error("Base directory is required");
      return;
    }
    if (buildType === "dockerfile" && !dockerfilePath.trim()) {
      toast.error("Dockerfile path is required");
      return;
    }
    if (!Number.isInteger(internalPort) || internalPort < 1 || internalPort > 65535) {
      toast.error("Internal port must be between 1 and 65535");
      return;
    }

    setLoading(true);
    try {
      const formData = new FormData();
      formData.append("name", name);
      formData.append("source_type", sourceType === "public" ? "github" : sourceType);
      formData.append("project_type", projectType);
      formData.append("build_type", buildType);
      formData.append("base_directory", baseDirectory.trim());
      formData.append(
        "dockerfile_path",
        buildType === "dockerfile" ? dockerfilePath.trim() : "",
      );
      formData.append("internal_port", String(internalPort));

      if (sourceType === "github" || sourceType === "public") {
        formData.append("github_url", githubUrl);
        formData.append("github_branch", githubBranch);
      } else if (files) {
        Array.from(files).forEach((file) => formData.append("files", file));
      }

      const createRes = await authFetch(`${API_URL}/api/projects`, {
        method: "POST",
        body: formData,
      });

      if (!createRes.ok) {
        const errBody = await createRes.json().catch(() => ({} as { detail?: string; error?: string }));
        throw new Error(errBody.detail || errBody.error || "Failed to create project");
      }

      const newProject = await createRes.json();

      // 2. Inject ENVs
      for (const env of envVars) {
        await authFetch(`${API_URL}/api/projects/${newProject.id}/env`, {
          method: "POST",
          headers: { "Content-Type": "application/json" },
          body: JSON.stringify(env),
        });
      }

      toast.success("Project created successfully");
      navigate(`/projects/${newProject.id}`);
    } catch (error: any) {
      const errorMessage = error.response?.data?.detail || error.message || "Failed to create project";
      toast.error(errorMessage);
    } finally {
      setLoading(false);
    }
  };

  const handleDrag = (e: React.DragEvent) => {
    e.preventDefault();
    e.stopPropagation();
    setDragActive(e.type === "dragenter" || e.type === "dragover");
  };

  const handleDrop = (e: React.DragEvent) => {
    e.preventDefault();
    e.stopPropagation();
    setDragActive(false);
    if (e.dataTransfer.files?.length) {
      setFiles(e.dataTransfer.files);
      setStep(2);
    }
  };

  const handlePublicSubmit = () => {
    if (!githubUrl) return toast.error("Please enter a repository URL");

    // Strict GitHub URL validation
    const githubPattern = /^https:\/\/github\.com\/[a-zA-Z0-9_.-]+\/[a-zA-Z0-9_.-]+(\.git)?\/?$/;
    if (!githubPattern.test(githubUrl.trim())) {
      return toast.error("Please enter a valid public GitHub repository URL (e.g., https://github.com/username/repo)");
    }

    if (!githubBranch) {
      return toast.error(
        refKind === "tag"
          ? "Select a tag to deploy"
          : "Select a branch to deploy",
      );
    }

    if (!name) setName(githubUrl.split("/").pop()?.replace(".git", "") || "my-app");
    setStep(2);
  };

  // Check if URL is a valid GitHub format
  const isValidGithubUrl = (url: string) => {
    if (!url) return null; // neutral
    const pattern = /^https:\/\/github\.com\/[a-zA-Z0-9_.-]+\/[a-zA-Z0-9_.-]+(\.git)?\/?$/;
    return pattern.test(url.trim());
  };

  const stepDescription =
    step === 1
      ? "Pick a public repository, a private GitHub App repository, or upload source."
      : "Name the project, choose the builder, and set environment variables.";

  const goBack = () => (step > 1 ? setStep(step - 1) : navigate("/"));

  const manifest = [
    "Application",
    sourceType === "github" ? "private" : sourceType,
    buildType,
    githubBranch ? `${refKind === "tag" ? "tag" : "branch"}:${githubBranch}` : null,
    envVars.length > 0 ? `${envVars.length} env` : null,
  ].filter(Boolean);

  return (
    <>
      <PageHeader
        title="New project"
        description={stepDescription}
        meta={
          <div className="flex items-center gap-3" aria-label={`Step ${step} of 2`}>
            <span className="flex gap-1" aria-hidden="true">
              <span className="h-0.5 w-6 rounded-full bg-brand" />
              <span className={cn("h-0.5 w-6 rounded-full", step >= 2 ? "bg-brand" : "bg-border")} />
            </span>
            <span className="text-[13px] text-muted-foreground">
              Step {step} of 2 · {step === 1 ? "Source" : "Configure"}
            </span>
          </div>
        }
        actions={
          step === 1 ? (
            <Button type="button" variant="ghost" size="sm" className="-ml-2.5 text-muted-foreground sm:ml-0" onClick={goBack}>
              <ArrowLeft />
              Back to projects
            </Button>
          ) : undefined
        }
      />

      <div className="w-full space-y-5">
          {/* STEP 1: Source Selection — same content width as Projects list */}
          {step === 1 && (
            <div className="w-full space-y-5">
              <div
                className="grid w-full grid-cols-3 gap-2"
                role="tablist"
                aria-label="Project source"
              >
                {(
                  [
                    { id: "public" as const, label: "Public repository", short: "Public", hint: "Any public GitHub URL" },
                    { id: "github" as const, label: "Private repository", short: "Private", hint: "Through the GitHub App" },
                    { id: "upload" as const, label: "Direct upload", short: "Upload", hint: "A ZIP file or folder" },
                  ] as const
                ).map((tab) => {
                  const active = sourceType === tab.id;
                  return (
                    <button
                      key={tab.id}
                      type="button"
                      role="tab"
                      aria-selected={active}
                      onClick={() => setSourceType(tab.id)}
                      className={cn(
                        "min-h-11 w-full rounded-lg border bg-card px-3 py-2.5 text-left transition-[border-color,box-shadow,color] duration-150",
                        active
                          ? "border-brand text-foreground ring-1 ring-brand"
                          : "border-input text-muted-foreground hover:border-subtle hover:text-foreground",
                      )}
                    >
                      <span className="hidden text-sm font-medium sm:block">{tab.label}</span>
                      <span className="block text-sm font-medium sm:hidden">{tab.short}</span>
                      <span className="mt-0.5 hidden text-xs text-muted-foreground sm:block">{tab.hint}</span>
                    </button>
                  );
                })}
              </div>

              {sourceType === "public" && (
                <Panel title="Public repository" description="Deploy a public GitHub repository by its URL.">
                  <PanelBody>
                    <div className="space-y-5">
                      <Field label="Repository URL">
                        <Input
                          placeholder="https://github.com/username/repo"
                          value={githubUrl}
                          onChange={(e) => setGithubUrl(e.target.value)}
                          className={cn(
                            "font-mono",
                            githubUrl && isValidGithubUrl(githubUrl) === false && "border-danger focus:border-danger focus:ring-danger/20",
                          )}
                        />
                        {githubUrl && isValidGithubUrl(githubUrl) === false && (
                          <span className="flex items-center gap-1 text-xs text-danger">
                            <X className="size-3.5" /> Invalid URL format
                          </span>
                        )}
                        {githubUrl && isValidGithubUrl(githubUrl) === true && (
                          <span className="flex items-center gap-1 text-xs text-ok">
                            <Check className="size-3.5" /> Valid URL
                          </span>
                        )}
                      </Field>
                      <div className="space-y-1.5">
                        <p className="text-[13px] font-medium leading-5">Deploy from</p>
                        <BranchSelector
                          branches={branches}
                          tags={tags}
                          value={githubBranch}
                          onChange={setGithubBranch}
                          kind={refKind}
                          onKindChange={handleRefKindChange}
                          loading={branchesLoading}
                          tagsLoading={tagsLoading}
                          disabled={!githubUrl || isValidGithubUrl(githubUrl) !== true}
                        />
                        {repoAccessError && (
                          <div className="rounded-md border border-warn/40 bg-secondary/60 px-3 py-2.5 text-[13px] leading-5">
                            {repoAccessError}
                          </div>
                        )}
                      </div>
                    </div>
                  </PanelBody>
                  <PanelFooter>
                    <Button
                      onClick={handlePublicSubmit}
                      disabled={
                        !githubUrl ||
                        isValidGithubUrl(githubUrl) !== true ||
                        !githubBranch ||
                        branchesLoading ||
                        (refKind === "tag" && tagsLoading)
                      }
                    >
                      Continue
                      <ArrowRight />
                    </Button>
                  </PanelFooter>
                </Panel>
              )}

              {sourceType === "github" && (
                <>
                  {!githubStatus?.connected ? (
                    <Card className="px-4 py-14 text-center">
                      <h3 className="text-[15px] font-semibold">Connect GitHub</h3>
                      <p className="mx-auto mt-1 max-w-sm text-[13.5px] text-muted-foreground">
                        Install the Docklift GitHub App to deploy private repositories.
                      </p>
                      <Button className="mt-5" onClick={() => setShowGitHubConnect(true)}>
                        <GithubIcon className="size-4" />
                        Connect GitHub
                      </Button>
                      <GitHubConnect
                        open={showGitHubConnect}
                        onOpenChange={setShowGitHubConnect}
                        onConnected={() => {
                          fetchGitHubStatus();
                          setSourceType("github");
                        }}
                      />
                    </Card>
                  ) : (
                    <Card className="overflow-hidden">
                      <div className="flex flex-col gap-3 border-b border-border px-4 py-3 sm:flex-row sm:items-center sm:justify-between sm:px-5">
                        <div className="min-w-0">
                          <h3 className="text-[15px] font-semibold leading-5">Private repository</h3>
                          <p className="mt-0.5 flex min-w-0 items-center gap-1.5 text-[13px] text-muted-foreground">
                            <span aria-hidden="true" className="size-2 shrink-0 rounded-full bg-ok" />
                            <span className="truncate">
                              {githubInstallations.length > 0
                                ? `${githubInstallations.length} GitHub account${githubInstallations.length === 1 ? "" : "s"} connected`
                                : `@${githubStatus.username || "connected"}`}
                            </span>
                          </p>
                        </div>
                        <div className="flex w-full items-center gap-2 sm:w-auto">
                          <div className="relative min-w-0 flex-1 sm:w-60 sm:flex-none">
                            <Search className="pointer-events-none absolute left-2.5 top-1/2 size-4 -translate-y-1/2 text-subtle" />
                            <Input
                              placeholder="Search all repos"
                              value={searchQuery}
                              onChange={(e) => setSearchQuery(e.target.value)}
                              className="pl-8"
                            />
                          </div>
                          <Button
                            type="button"
                            variant="outline"
                            size="lg"
                            className="shrink-0"
                            title="Add another personal account or organization"
                            onClick={handleAddGithubAccount}
                          >
                            <PlusIcon className="text-subtle" />
                            <span className="max-sm:sr-only">Add account</span>
                          </Button>
                        </div>
                      </div>
                      {githubInstallations.length > 0 && (
                        <div className="hairline-b flex gap-5 overflow-x-auto px-4 scrollbar-none sm:px-5">
                          <button
                            type="button"
                            onClick={() => setAccountFilter(null)}
                            className={cn(
                              "h-10 shrink-0 border-b-2 text-[13.5px] transition-colors duration-150",
                              accountFilter === null
                                ? "border-brand text-foreground"
                                : "border-transparent text-muted-foreground hover:text-foreground",
                            )}
                          >
                            All
                          </button>
                          {githubInstallations.map((inst) => (
                            <button
                              key={inst.id}
                              type="button"
                              onClick={() => setAccountFilter(inst.login)}
                              className={cn(
                                "inline-flex h-10 shrink-0 items-center gap-1.5 border-b-2 text-[13.5px] transition-colors duration-150",
                                accountFilter === inst.login
                                  ? "border-brand text-foreground"
                                  : "border-transparent text-muted-foreground hover:text-foreground",
                              )}
                              title={
                                inst.type === "Organization"
                                  ? "Organization"
                                  : "Personal"
                              }
                            >
                              {inst.avatar_url ? (
                                <img
                                  src={inst.avatar_url}
                                  alt=""
                                  className="size-4 rounded-sm"
                                />
                              ) : null}
                              <span className="font-mono text-[13px]">@{inst.login}</span>
                            </button>
                          ))}
                        </div>
                      )}
                      {reposWarning && (
                        <div className="border-b border-border px-4 py-2.5 sm:px-5">
                          <p className="rounded-md border border-warn/40 bg-secondary/60 px-3 py-2 text-[13px] leading-5">
                            {reposWarning}
                          </p>
                        </div>
                      )}
                      <div className="max-h-96 divide-y divide-border overflow-y-auto">
                        {reposLoading || githubLoading ? (
                          <div className="flex items-center justify-center gap-2 py-14 text-[13px] text-muted-foreground">
                            <Loader2 className="size-4 animate-spin" />
                            Loading repositories…
                          </div>
                        ) : filteredRepos.length === 0 ? (
                          <div className="px-4 py-14 text-center">
                            <p className="text-[15px] font-semibold">No repositories found</p>
                            <p className="mx-auto mt-1 max-w-sm text-[13.5px] text-muted-foreground">
                              {accountFilter
                                ? `Nothing matched under @${accountFilter}. Try All, or grant the GitHub App access to more repos.`
                                : "Install the app on your personal account and orgs (Add account), and grant repo access in GitHub."}
                            </p>
                          </div>
                        ) : filteredRepos.map(repo => (
                          <div
                            key={repo.id}
                            onClick={() => handleSelectRepo(repo)}
                            className="group flex cursor-pointer items-center justify-between gap-3 px-4 py-2.5 transition-colors duration-150 hover:bg-secondary/50 sm:px-5"
                          >
                            <div className="min-w-0">
                              <h4 className="flex min-w-0 items-center gap-1.5 text-sm font-medium">
                                <span className="truncate">{repo.full_name}</span>
                                {repo.private ? (
                                  <Lock className="size-3.5 shrink-0 text-subtle" aria-hidden="true" />
                                ) : (
                                  <Globe className="size-3.5 shrink-0 text-subtle" aria-hidden="true" />
                                )}
                              </h4>
                              <p className="mt-0.5 truncate text-[13px] text-muted-foreground">
                                <span className="font-mono text-xs">
                                  {repo.private ? "private" : "public"} · {repo.default_branch}
                                </span>
                                <span className="text-subtle"> · </span>
                                {repo.description || "No description"}
                              </p>
                            </div>
                            <ArrowRight className="size-4 shrink-0 text-subtle opacity-0 transition-opacity duration-150 group-hover:opacity-100" />
                          </div>
                        ))}
                      </div>
                    </Card>
                  )}
                </>
              )}

              {sourceType === "upload" && (
                <Card
                  onDragEnter={handleDrag}
                  onDragLeave={handleDrag}
                  onDragOver={handleDrag}
                  onDrop={handleDrop}
                  className="cursor-pointer"
                  onClick={() => document.getElementById("file-upload-input")?.click()}
                >
                  <input id="file-upload-input" type="file" multiple className="hidden" onChange={(e) => { setFiles(e.target.files); setStep(2); }} />
                  <div className="border-b border-border px-4 py-3 sm:px-5">
                    <h3 className="text-[15px] font-semibold leading-5">Upload project</h3>
                    <p className="mt-0.5 text-[13px] leading-5 text-muted-foreground">
                      Deploy source files from your computer.
                    </p>
                  </div>
                  <div className="p-4 sm:p-5">
                    <div
                      className={cn(
                        "flex flex-col items-center rounded-md border border-dashed px-4 py-12 text-center transition-colors duration-150",
                        dragActive || files ? "border-brand bg-brand/5" : "border-input hover:border-subtle",
                      )}
                    >
                      <FolderUp className={cn("size-5", dragActive ? "text-brand" : "text-subtle")} />
                      <p className="mt-3 text-sm font-medium">Drop a ZIP file or click to browse</p>
                      <p className="mt-0.5 text-[13px] text-muted-foreground">
                        The project continues to configuration once files are selected.
                      </p>
                      {files && (
                        <p className="mt-3 inline-flex items-center gap-1.5 text-[13px] font-medium">
                          <Check className="size-4 text-ok" /> {files.length} file(s) selected
                        </p>
                      )}
                    </div>
                  </div>
                </Card>
              )}
            </div>
          )}
          {step === 2 && (
            <div className="w-full space-y-5">
                <Panel title="Project" description="Name and source of the new project.">
                  <PanelBody>
                    <div className="space-y-5">
                      <Field label="Project name">
                        <Input
                          placeholder="my-app"
                          value={name}
                          onChange={(e) => setName(e.target.value)}
                        />
                      </Field>

                      {(sourceType === "github" || sourceType === "public") && githubUrl && (
                        <div className="space-y-1.5">
                          <div className="flex items-end justify-between gap-3">
                            <div className="min-w-0">
                              <p className="text-[13px] font-medium leading-5">Source</p>
                              <p className="flex min-w-0 items-center gap-1.5 text-muted-foreground">
                                <GithubIcon className="size-3.5 shrink-0 text-subtle" />
                                <span className="truncate font-mono text-[13px] text-foreground">
                                  {selectedRepo?.full_name ||
                                    githubUrl
                                      .replace(/^https:\/\/github\.com\//, "")
                                      .replace(/\.git$/, "")}
                                </span>
                              </p>
                            </div>
                            <Button
                              variant="link"
                              size="sm"
                              onClick={() => setStep(1)}
                              className="shrink-0 px-0"
                            >
                              Change
                            </Button>
                          </div>
                          <p className="pt-2 text-xs text-muted-foreground">
                            Deploy from {refKind === "tag" ? "tag" : "branch"}
                          </p>
                          <BranchSelector
                            branches={branches}
                            tags={tags}
                            value={githubBranch}
                            onChange={setGithubBranch}
                            kind={refKind}
                            onKindChange={handleRefKindChange}
                            loading={branchesLoading}
                            tagsLoading={tagsLoading}
                          />
                        </div>
                      )}
                    </div>
                  </PanelBody>
                </Panel>

                <Panel title="Build" description="Choose how Docklift turns your source into a container.">
                  <PanelBody>
                    <div className="space-y-5">
                      <div className="grid grid-cols-1 gap-2 sm:grid-cols-3">
                        {([
                          ["auto", "Auto", "Detect the best builder"],
                          ["dockerfile", "Dockerfile", "Use your Dockerfile"],
                          ["railpack", "Railpack", "Build from app manifests"],
                        ] as const).map(([value, label, description]) => (
                          <button
                            key={value}
                            type="button"
                            onClick={() => setBuildType(value)}
                            className={cn(
                              "rounded-lg border bg-card p-3 text-left transition-[border-color,box-shadow] duration-150",
                              buildType === value
                                ? "border-brand ring-1 ring-brand"
                                : "border-input hover:border-subtle",
                            )}
                          >
                            <span className={cn("block text-sm font-medium", buildType === value ? "text-foreground" : "text-foreground/85")}>
                              {label}
                            </span>
                            <span className="mt-0.5 block text-xs text-muted-foreground">
                              {description}
                            </span>
                          </button>
                        ))}
                      </div>

                      <div className="grid grid-cols-1 gap-4 sm:grid-cols-2">
                        <Field label="Base directory" help={<>Relative to the project root, for example <code className="font-mono">apps/web</code>.</>}>
                          <Input
                            value={baseDirectory}
                            onChange={(e) => setBaseDirectory(e.target.value)}
                            placeholder="."
                            className="font-mono"
                          />
                        </Field>
                        <Field label="Internal port" help="The port your application listens on inside its container.">
                          <Input
                            type="number"
                            min={1}
                            max={65535}
                            value={internalPort}
                            onChange={(e) => setInternalPort(Number(e.target.value))}
                            className="font-mono tabular-nums"
                          />
                        </Field>
                      </div>

                      {buildType === "dockerfile" && (
                        <Field label="Dockerfile path" help="Path relative to the base directory.">
                          <Input
                            value={dockerfilePath}
                            onChange={(e) => setDockerfilePath(e.target.value)}
                            placeholder="Dockerfile"
                            className="font-mono"
                          />
                        </Field>
                      )}
                    </div>
                  </PanelBody>
                </Panel>

                <Card>
                  <div className="flex flex-col gap-3 border-b border-border px-4 py-3 sm:flex-row sm:items-center sm:justify-between sm:px-5">
                    <div className="min-w-0">
                      <h3 className="flex items-baseline gap-2 text-[15px] font-semibold leading-5">
                        Environment variables
                        {envVars.length > 0 && (
                          <span className="font-mono text-[13px] font-normal text-subtle tabular-nums">{envVars.length}</span>
                        )}
                      </h3>
                      <p className="mt-0.5 text-[13px] leading-5 text-muted-foreground">
                        Saved to the project after it is created.
                      </p>
                    </div>

                    <div className="flex items-center gap-2">
                    <Dialog open={showAddEnv} onOpenChange={setShowAddEnv}>
                      <DialogTrigger asChild>
                        <Button variant="outline" size="sm" className="max-sm:flex-1">
                          <PlusIcon className="text-subtle" /> Add
                        </Button>
                      </DialogTrigger>
                      <DialogContent>
                        <DialogHeader>
                          <DialogTitle>Add environment variable</DialogTitle>
                        </DialogHeader>
                        <div className="space-y-4">
                          <Field label="Key">
                            <Input
                              placeholder="e.g. API_KEY"
                              value={newEnvKey}
                              onChange={(e) => setNewEnvKey(e.target.value.toUpperCase().replace(/\s+/g, '_'))}
                              className="font-mono"
                            />
                          </Field>
                          <Field label="Value">
                            <Input
                              placeholder="Value"
                              type="text"
                              value={newEnvValue}
                              onChange={(e) => setNewEnvValue(e.target.value)}
                              className="font-mono"
                            />
                          </Field>

                          <div className="space-y-1">
                            <p className="text-[13px] font-medium leading-5">Inject in</p>
                            <div className="flex flex-col gap-1 sm:flex-row sm:gap-6">
                              <button
                                type="button"
                                onClick={() => setNewEnvIsBuild(!newEnvIsBuild)}
                                className="flex items-start gap-2.5 rounded-md py-1.5 text-left"
                              >
                                <CheckMark on={newEnvIsBuild} />
                                <span>
                                  <span className="block text-[13px] font-medium leading-5">Build</span>
                                  <span className="block text-xs text-muted-foreground">Next.js/Prisma build-args</span>
                                </span>
                              </button>
                              <button
                                type="button"
                                onClick={() => setNewEnvIsRuntime(!newEnvIsRuntime)}
                                className="flex items-start gap-2.5 rounded-md py-1.5 text-left"
                              >
                                <CheckMark on={newEnvIsRuntime} />
                                <span>
                                  <span className="block text-[13px] font-medium leading-5">Runtime</span>
                                  <span className="block text-xs text-muted-foreground">Available after deploy</span>
                                </span>
                              </button>
                            </div>
                          </div>

                          {!newEnvIsBuild && !newEnvIsRuntime && (
                            <p className="rounded-md border border-warn/40 bg-secondary/60 px-3 py-2.5 text-[13px] leading-5">
                              You must select at least one injection scope for this variable to be active.
                            </p>
                          )}
                        </div>
                        <DialogFooter>
                          <Button variant="outline" onClick={() => setShowAddEnv(false)}>Cancel</Button>
                          <Button
                            disabled={!newEnvKey || !newEnvValue || (!newEnvIsBuild && !newEnvIsRuntime)}
                            onClick={() => {
                              if (!newEnvKey || !newEnvValue) return;
                              let value = newEnvValue.trim();
                              if (value.length >= 2) {
                                const first = value.charAt(0);
                                const last = value.charAt(value.length - 1);
                                if ((first === '"' && last === '"') || (first === "'" && last === "'")) {
                                  value = value.substring(1, value.length - 1);
                                }
                              }
                              setEnvVars([...envVars, {key: newEnvKey, value, is_build_arg: newEnvIsBuild, is_runtime: newEnvIsRuntime}]);
                              setNewEnvKey("");
                              setNewEnvValue("");
                              setShowAddEnv(false);
                            }}
                          >
                            Add variable
                          </Button>
                        </DialogFooter>
                      </DialogContent>
                    </Dialog>

                    <Dialog open={showBulkEnv} onOpenChange={setShowBulkEnv}>
                      <DialogTrigger asChild>
                        <Button variant="outline" size="sm" className="max-sm:flex-1">
                          <Upload className="text-subtle" /> Bulk import
                        </Button>
                      </DialogTrigger>
                      <DialogContent className="sm:max-w-lg">
                        <DialogHeader>
                          <DialogTitle>Bulk import environment variables</DialogTitle>
                        </DialogHeader>
                        <div className="space-y-4">
                          <label className="block space-y-1.5">
                            <span className="block text-[13px] font-medium leading-5">
                              Paste KEY=VALUE pairs (one per line)
                            </span>
                            <Textarea
                              placeholder={`ADMIN_USERNAME=admin
DATABASE_URL=postgresql://user:pass@host:5432/db
SESSION_SECRET=your-secret-here
# Lines starting with # are ignored`}
                              value={bulkEnvContent}
                              onChange={(e) => setBulkEnvContent(e.target.value)}
                              className="h-40 resize-none font-mono text-[13px]"
                            />
                          </label>

                          <div className="flex flex-col gap-1 sm:flex-row sm:gap-6">
                            <div className="flex cursor-pointer items-start gap-2.5 py-1.5" onClick={() => setBulkIsBuild(!bulkIsBuild)}>
                              <CheckMark on={bulkIsBuild} />
                              <div className="flex flex-col">
                                <span className="text-[13px] font-medium leading-5">Build argument</span>
                                <span className="text-xs text-muted-foreground">Used during Docker build</span>
                              </div>
                            </div>

                            <div className="flex cursor-pointer items-start gap-2.5 py-1.5" onClick={() => setBulkIsRuntime(!bulkIsRuntime)}>
                              <CheckMark on={bulkIsRuntime} />
                              <div className="flex flex-col">
                                <span className="text-[13px] font-medium leading-5">Runtime variable</span>
                                <span className="text-xs text-muted-foreground">Available to your app</span>
                              </div>
                            </div>
                          </div>

                          <p className="font-mono text-xs text-muted-foreground tabular-nums">
                            {bulkEnvContent.split('\n').filter(l => l.trim() && !l.trim().startsWith('#') && l.includes('=')).length} variables detected
                          </p>
                        </div>
                        <DialogFooter>
                          <Button variant="outline" onClick={() => setShowBulkEnv(false)}>Cancel</Button>
                          <Button
                            disabled={!bulkEnvContent.trim()}
                            onClick={() => {
                              const lines = bulkEnvContent.split('\n').filter(l => l.trim() && !l.trim().startsWith('#'));
                              const newVars: typeof envVars = [];
                              for (const line of lines) {
                                const eqIndex = line.indexOf('=');
                                if (eqIndex === -1) continue;
                                const key = line.substring(0, eqIndex).trim();
                                let value = line.substring(eqIndex + 1).trim();
                                if (value.length >= 2) {
                                  const first = value.charAt(0);
                                  const last = value.charAt(value.length - 1);
                                  if ((first === '"' && last === '"') || (first === "'" && last === "'")) {
                                    value = value.substring(1, value.length - 1);
                                  }
                                }
                                if (key) {
                                  newVars.push({ key, value, is_build_arg: bulkIsBuild, is_runtime: bulkIsRuntime });
                                }
                              }
                              if (newVars.length > 0) {
                                setEnvVars([...envVars, ...newVars]);
                                toast.success(`Added ${newVars.length} environment variable(s)`);
                              }
                              setBulkEnvContent("");
                              setShowBulkEnv(false);
                            }}
                          >
                            Import all
                          </Button>
                        </DialogFooter>
                      </DialogContent>
                    </Dialog>
                    </div>
                  </div>

                  {envVars.length === 0 ? (
                    <div className="px-4 py-12 text-center">
                      <p className="text-[15px] font-semibold">No environment variables</p>
                      <p className="mx-auto mt-1 max-w-sm text-[13.5px] text-muted-foreground">
                        None added yet. Add them one by one or paste a .env file.
                      </p>
                    </div>
                  ) : (
                    <ul className="divide-y divide-border">
                      {envVars.map((env, i) => (
                        <li key={i} className="group flex items-center justify-between gap-3 px-4 py-2.5 transition-colors duration-150 hover:bg-secondary/50 sm:px-5">
                          <div className="min-w-0">
                            <div className="flex min-w-0 flex-wrap items-center gap-x-2 gap-y-1">
                              <span className="truncate font-mono text-[13px] font-medium">{env.key}</span>
                              <ScopeTag on={env.is_build_arg}>build</ScopeTag>
                              <ScopeTag on={env.is_runtime}>runtime</ScopeTag>
                            </div>
                            <span className="mt-0.5 block truncate font-mono text-xs text-muted-foreground">
                              {revealedEnvs.includes(i) ? env.value : "••••••••••••"}
                            </span>
                          </div>
                          <div className="flex shrink-0 items-center gap-0.5 sm:opacity-60 sm:transition-opacity sm:duration-150 sm:group-hover:opacity-100 sm:group-focus-within:opacity-100">
                            <Button
                              type="button"
                              variant="ghost"
                              size="icon"
                              className="text-subtle"
                              onClick={() => {
                                if (revealedEnvs.includes(i)) {
                                  setRevealedEnvs(revealedEnvs.filter(idx => idx !== i));
                                } else {
                                  setRevealedEnvs([...revealedEnvs, i]);
                                }
                              }}
                            >
                              {revealedEnvs.includes(i) ? <EyeOff /> : <Eye />}
                              <span className="sr-only">{revealedEnvs.includes(i) ? "Hide value" : "Show value"}</span>
                            </Button>
                            <Button
                              type="button"
                              variant="ghost"
                              size="icon"
                              className="text-subtle hover:text-danger"
                              onClick={() => setEnvVars(envVars.filter((_, idx) => idx !== i))}
                            >
                              <Trash2 />
                              <span className="sr-only">Remove {env.key}</span>
                            </Button>
                          </div>
                        </li>
                      ))}
                    </ul>
                  )}
                </Card>

              {/* Summary + create */}
              <Card className="flex flex-col gap-3 px-4 py-3 sm:flex-row sm:items-center sm:justify-between sm:px-5">
                <div className="min-w-0">
                  <p className="truncate text-sm font-medium">
                    {name || "Untitled project"}
                  </p>
                  <p className="mt-0.5 truncate font-mono text-xs text-muted-foreground">
                    {manifest.join(" · ")}
                  </p>
                </div>
                <div className="flex flex-col-reverse gap-2 sm:flex-row sm:items-center">
                  <Button type="button" variant="ghost" className="text-muted-foreground" onClick={goBack}>
                    <ArrowLeft />
                    Back
                  </Button>
                  <Button
                    onClick={handleSubmit}
                    disabled={loading || !name.trim()}
                    size="lg"
                  >
                    {loading && <Loader2 className="animate-spin" />}
                    Create project
                  </Button>
                </div>
              </Card>
            </div>
          )}
      </div>
    </>
  );
}

export default function NewProjectPage() {
  return (
    <Suspense fallback={
      <div className="flex min-h-[60vh] items-center justify-center">
        <Loader2 className="size-5 animate-spin text-subtle" />
      </div>
    }>
      <NewProjectContent />
    </Suspense>
  );
}

/* ---------- presentational helpers (layout only, no state) ---------- */

function Panel({
  title,
  description,
  children,
}: {
  title: string;
  description?: string;
  children: React.ReactNode;
}) {
  return (
    <Card>
      <div className="border-b border-border px-4 py-3 sm:px-5">
        <h3 className="text-[15px] font-semibold leading-5">{title}</h3>
        {description && (
          <p className="mt-0.5 text-[13px] leading-5 text-muted-foreground">{description}</p>
        )}
      </div>
      {children}
    </Card>
  );
}

function PanelBody({ children }: { children: React.ReactNode }) {
  return (
    <div className="p-4 sm:p-5">
      <div className="max-w-xl">{children}</div>
    </div>
  );
}

function PanelFooter({ children }: { children: React.ReactNode }) {
  return (
    <div className="flex flex-col-reverse gap-2 border-t border-border px-4 py-3 sm:flex-row sm:items-center sm:justify-end sm:px-5">
      {children}
    </div>
  );
}

function Field({
  label,
  help,
  children,
}: {
  label: string;
  help?: React.ReactNode;
  children: React.ReactNode;
}) {
  return (
    <label className="block space-y-1.5">
      <span className="block text-[13px] font-medium leading-5">{label}</span>
      {children}
      {help && <span className="block text-xs leading-[1.125rem] text-muted-foreground">{help}</span>}
    </label>
  );
}

function CheckMark({ on }: { on: boolean }) {
  return (
    <span
      aria-hidden="true"
      className={cn(
        "mt-0.5 flex size-4 shrink-0 items-center justify-center rounded-sm border transition-colors duration-150",
        on ? "border-brand bg-brand text-brand-foreground" : "border-input bg-card",
      )}
    >
      {on && <Check className="size-3 stroke-[2.5]" />}
    </span>
  );
}

function ScopeTag({ on, children }: { on: boolean; children: React.ReactNode }) {
  return (
    <span
      className={cn(
        "rounded-sm border px-1.5 py-px font-mono text-xs leading-4",
        on ? "border-border text-muted-foreground" : "border-transparent text-subtle line-through",
      )}
    >
      {children}
    </span>
  );
}
