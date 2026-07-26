// New project page - wizard for creating projects via GitHub, public repo, or file upload

import { useState, useEffect, useRef, Suspense } from "react";
import { useNavigate, useSearchParams } from "react-router-dom";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Card } from "@/components/ui/card";
import { BranchSelector, type GitRefKind } from "@/components/BranchSelector";
import { GithubIcon } from "@/components/icons/GithubIcon";
import { PageHeader } from "@/components/shell/PageHeader";
import {
  Upload,
  FolderUp,
  Loader2,
  Sparkles,
  Check,
  Lock,
  Search,
  Globe,
  ArrowLeft,
  ArrowRight,
  Shield,
  FlaskConical,
  Info,
  Plus as PlusIcon,
  Trash2,
  X,
  Eye,
  EyeOff,
  Box,
  Container,
} from "lucide-react";
import { API_URL, cn } from "@/lib/utils";
import { authFetch, startGithubInstallAndNavigate } from "@/lib/auth";
import { toast } from "sonner";
import {
  Dialog,
  DialogContent,
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

  return (
    <>
      <button
        type="button"
        onClick={() => (step > 1 ? setStep(step - 1) : navigate("/"))}
        className="mb-6 inline-flex items-center gap-2 text-sm text-muted-foreground transition-colors hover:text-foreground"
      >
        <ArrowLeft className="h-4 w-4" />
        {step === 1 ? "Back to Projects" : "Back"}
      </button>

      <PageHeader
        eyebrow="Deploy"
        title="New Project"
        description={stepDescription}
        icon={Container}
        actions={
          <div
            className="inline-flex items-center gap-2 rounded-xl border border-border/60 bg-secondary/40 px-3 py-1.5"
            aria-label={`Step ${step} of 2`}
          >
            <span
              className={cn(
                "flex h-7 w-7 items-center justify-center rounded-lg text-xs font-semibold",
                step >= 1
                  ? "bg-brand text-brand-foreground"
                  : "bg-muted text-muted-foreground",
              )}
            >
              1
            </span>
            <span
              className={cn(
                "h-0.5 w-6 rounded-full",
                step >= 2 ? "bg-brand" : "bg-border",
              )}
            />
            <span
              className={cn(
                "flex h-7 w-7 items-center justify-center rounded-lg text-xs font-semibold",
                step >= 2
                  ? "bg-brand text-brand-foreground"
                  : "bg-muted text-muted-foreground",
              )}
            >
              2
            </span>
          </div>
        }
      />

      <div className="w-full space-y-6">
          {/* STEP 1: Source Selection — same content width as Projects list */}
          {step === 1 && (
            <div className="w-full space-y-5 sm:space-y-6">
              <div
                className="grid w-full grid-cols-3 gap-1.5 sm:gap-2"
                role="tablist"
                aria-label="Project source"
              >
                {(
                  [
                    { id: "public" as const, label: "Public Repository", short: "Public", icon: Globe },
                    { id: "github" as const, label: "Private Repository", short: "Private", icon: Lock },
                    { id: "upload" as const, label: "Direct upload", short: "Upload", icon: Upload },
                  ] as const
                ).map((tab) => {
                  const active = sourceType === tab.id;
                  const Icon = tab.icon;
                  return (
                    <button
                      key={tab.id}
                      type="button"
                      role="tab"
                      aria-selected={active}
                      onClick={() => setSourceType(tab.id)}
                      className={cn(
                        "inline-flex min-h-11 w-full items-center justify-center gap-1.5 rounded-xl border px-1.5 py-2.5 text-center text-xs font-medium transition-colors sm:min-h-12 sm:gap-2 sm:px-3 sm:text-sm",
                        active
                          ? "border-brand/30 bg-brand/10 text-brand"
                          : "border-border/60 bg-secondary/40 text-muted-foreground hover:text-foreground",
                      )}
                    >
                      <Icon className="h-3.5 w-3.5 shrink-0 sm:h-4 sm:w-4" />
                      <span className="hidden truncate sm:inline">{tab.label}</span>
                      <span className="truncate sm:hidden">{tab.short}</span>
                    </button>
                  );
                })}
              </div>

              {sourceType === "public" && (
                <Card className="rounded-2xl border-border/60 p-5 sm:p-6">
                  <div className="space-y-5">
                    <div className="space-y-2">
                      <label className="text-sm font-medium">Repository URL</label>
                      <Input 
                        placeholder="https://github.com/username/repo" 
                        value={githubUrl}
                        onChange={(e) => setGithubUrl(e.target.value)}
                        className={cn(
                          "h-11 bg-secondary/30",
                          githubUrl && isValidGithubUrl(githubUrl) === false && "border-red-500 focus-visible:ring-red-500",
                          githubUrl && isValidGithubUrl(githubUrl) === true && "border-emerald-500 focus-visible:ring-emerald-500"
                        )}
                      />
                      {githubUrl && isValidGithubUrl(githubUrl) === false && (
                        <p className="flex items-center gap-1 text-xs text-red-500">
                          <X className="h-3 w-3" /> Invalid URL format
                        </p>
                      )}
                      {githubUrl && isValidGithubUrl(githubUrl) === true && (
                        <p className="flex items-center gap-1 text-xs text-emerald-600 dark:text-emerald-400">
                          <Check className="h-3 w-3" /> Valid URL
                        </p>
                      )}
                    </div>
                    <div className="space-y-2">
                      <label className="text-sm font-medium">Deploy from</label>
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
                        <div className="flex items-start gap-2 rounded-xl border border-amber-500/20 bg-amber-500/10 p-3">
                          <Lock className="mt-0.5 h-4 w-4 shrink-0 text-amber-500" />
                          <span className="text-xs text-amber-600 dark:text-amber-400">{repoAccessError}</span>
                        </div>
                      )}
                    </div>
                    <Button 
                      onClick={handlePublicSubmit} 
                      disabled={
                        !githubUrl ||
                        isValidGithubUrl(githubUrl) !== true ||
                        !githubBranch ||
                        branchesLoading ||
                        (refKind === "tag" && tagsLoading)
                      }
                      className="h-11 w-full bg-brand font-semibold text-brand-foreground shadow-lg shadow-brand/20 hover:brightness-110"
                    >
                      Continue <ArrowRight className="ml-2 h-4 w-4" />
                    </Button>
                  </div>
                </Card>
              )}

              {sourceType === "github" && (
                <div className="space-y-4">
                  {!githubStatus?.connected ? (
                    <Card className="flex flex-col items-center rounded-2xl border-2 border-dashed border-border/60 p-8 text-center sm:p-10">
                      <div className="mb-4 flex h-14 w-14 items-center justify-center rounded-2xl border border-brand/20 bg-brand/10">
                        <GithubIcon className="h-7 w-7 text-brand" />
                      </div>
                      <h3 className="mb-2 text-lg font-semibold tracking-tight">Connect GitHub</h3>
                      <p className="mb-6 max-w-xs text-sm text-muted-foreground">
                        Install the Docklift GitHub App to deploy private repositories.
                      </p>
                      <Button
                        onClick={() => setShowGitHubConnect(true)}
                        className="h-10 bg-brand px-6 font-semibold text-brand-foreground shadow-lg shadow-brand/20 hover:brightness-110"
                      >
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
                    <Card className="overflow-hidden rounded-2xl border-border/60">
                      <div className="space-y-3 border-b border-border/60 bg-secondary/30 px-4 py-3">
                        <div className="flex flex-col justify-between gap-3 sm:flex-row sm:items-center">
                          <div className="flex min-w-0 items-center gap-2">
                            <div className="h-2 w-2 shrink-0 rounded-full bg-emerald-500" />
                            <span className="truncate text-xs font-medium text-muted-foreground">
                              {githubInstallations.length > 0
                                ? `${githubInstallations.length} GitHub account${githubInstallations.length === 1 ? "" : "s"} connected`
                                : `@${githubStatus.username || "connected"}`}
                            </span>
                          </div>
                          <div className="flex w-full items-center gap-2 sm:w-auto">
                            <div className="relative flex-1 sm:w-56">
                              <Search className="absolute left-3 top-1/2 h-3.5 w-3.5 -translate-y-1/2 text-muted-foreground" />
                              <Input
                                placeholder="Search all repos..."
                                value={searchQuery}
                                onChange={(e) => setSearchQuery(e.target.value)}
                                className="h-9 border-border/40 bg-background/50 pl-9 text-sm"
                              />
                            </div>
                            <Button
                              type="button"
                              variant="outline"
                              size="sm"
                              className="h-9 shrink-0 rounded-xl border-border/60"
                              title="Add another personal account or organization"
                              onClick={handleAddGithubAccount}
                            >
                              <PlusIcon className="h-3.5 w-3.5 sm:mr-1" />
                              <span className="hidden sm:inline">Add</span>
                            </Button>
                          </div>
                        </div>
                        {githubInstallations.length > 0 && (
                          <div className="flex flex-wrap gap-1.5">
                            <button
                              type="button"
                              onClick={() => setAccountFilter(null)}
                              className={cn(
                                "h-7 rounded-lg border px-2.5 text-[11px] font-semibold transition-colors",
                                accountFilter === null
                                  ? "border-brand/30 bg-brand/10 text-brand"
                                  : "border-border/40 bg-background/50 text-muted-foreground hover:text-foreground",
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
                                  "inline-flex h-7 max-w-full items-center gap-1.5 rounded-lg border px-2.5 text-[11px] font-semibold transition-colors",
                                  accountFilter === inst.login
                                    ? "border-brand/30 bg-brand/10 text-brand"
                                    : "border-border/40 bg-background/50 text-muted-foreground hover:text-foreground",
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
                                    className="h-4 w-4 rounded"
                                  />
                                ) : null}
                                <span className="truncate">@{inst.login}</span>
                              </button>
                            ))}
                          </div>
                        )}
                        {reposWarning && (
                          <p className="text-[11px] leading-snug text-amber-600 dark:text-amber-400">
                            {reposWarning}
                          </p>
                        )}
                      </div>
                      <div className="max-h-80 divide-y divide-border/40 overflow-y-auto">
                        {reposLoading || githubLoading ? (
                          <div className="flex flex-col items-center gap-3 p-12 text-muted-foreground">
                            <Loader2 className="h-6 w-6 animate-spin" />
                            <span className="text-sm">Loading repos...</span>
                          </div>
                        ) : filteredRepos.length === 0 ? (
                          <div className="space-y-2 p-12 text-center text-sm text-muted-foreground">
                            <p>No repositories found</p>
                            <p className="text-xs">
                              {accountFilter
                                ? `Nothing matched under @${accountFilter}. Try All, or grant the GitHub App access to more repos.`
                                : "Install the app on your personal account and orgs (Add), and grant repo access in GitHub."}
                            </p>
                          </div>
                        ) : filteredRepos.map(repo => (
                          <div 
                            key={repo.id}
                            onClick={() => handleSelectRepo(repo)}
                            className="group flex cursor-pointer items-center justify-between p-4 transition-colors hover:bg-secondary/40"
                          >
                            <div className="flex min-w-0 items-center gap-3">
                              <div className="flex h-9 w-9 shrink-0 items-center justify-center rounded-xl border border-border/60 bg-secondary/40">
                                {repo.private ? <Lock className="h-4 w-4 text-muted-foreground" /> : <Globe className="h-4 w-4 text-muted-foreground" />}
                              </div>
                              <div className="min-w-0">
                                <h4 className="truncate text-sm font-medium">{repo.full_name}</h4>
                                <p className="truncate text-xs text-muted-foreground">{repo.description || "No description"}</p>
                              </div>
                            </div>
                            <ArrowRight className="ml-2 h-4 w-4 shrink-0 text-muted-foreground opacity-0 transition-opacity group-hover:opacity-100" />
                          </div>
                        ))}
                      </div>
                    </Card>
                  )}
                </div>
              )}

              {sourceType === "upload" && (
                <Card 
                  onDragEnter={handleDrag}
                  onDragLeave={handleDrag}
                  onDragOver={handleDrag}
                  onDrop={handleDrop}
                  className={cn(
                    "cursor-pointer rounded-2xl border-2 border-dashed p-10 text-center transition-colors sm:p-12",
                    dragActive ? "border-brand/50 bg-brand/5" : "border-border/60 hover:border-brand/40 hover:bg-secondary/20",
                    files && "border-brand/40 bg-brand/5"
                  )}
                  onClick={() => document.getElementById("file-upload-input")?.click()}
                >
                  <input id="file-upload-input" type="file" multiple className="hidden" onChange={(e) => { setFiles(e.target.files); setStep(2); }} />
                  <div className="flex flex-col items-center">
                    <div className="mb-4 flex h-14 w-14 items-center justify-center rounded-2xl border border-brand/20 bg-brand/10">
                      <FolderUp className="h-7 w-7 text-brand" />
                    </div>
                    <h3 className="mb-2 text-lg font-semibold tracking-tight">Upload Project</h3>
                    <p className="max-w-sm text-sm text-muted-foreground">
                      Drop a ZIP file or click to browse
                    </p>
                    {files && (
                      <div className="mt-4 inline-flex items-center gap-2 rounded-xl border border-brand/30 bg-brand/10 px-4 py-1.5 text-sm font-medium text-brand">
                        <Check className="h-4 w-4" /> {files.length} file(s) selected
                      </div>
                    )}
                  </div>
                </Card>
              )}
            </div>
          )}
          {step === 2 && (
            <div className="w-full space-y-6 pb-28 sm:pb-24">
                <Card className="space-y-4 rounded-2xl border-border/60 p-5">
                  <h3 className="flex items-center gap-2 text-base font-semibold tracking-tight">
                    <Sparkles className="h-4 w-4 text-brand" />
                    Configuration
                  </h3>
                  
                  <div className="space-y-2">
                    <label className="text-sm font-medium">Project Name</label>
                    <Input 
                      placeholder="my-app" 
                      value={name}
                      onChange={(e) => setName(e.target.value)}
                      className="h-11 bg-secondary/30" 
                    />
                  </div>

                  {(sourceType === "github" || sourceType === "public") && githubUrl && (
                    <div className="space-y-3 rounded-xl border border-border/60 bg-secondary/30 p-4">
                      <div className="flex flex-col justify-between gap-3 sm:flex-row sm:items-start">
                        <div className="flex min-w-0 items-center gap-3">
                          <GithubIcon className="h-5 w-5 shrink-0" />
                          <div className="min-w-0">
                            <p className="truncate text-sm font-medium">
                              {selectedRepo?.full_name ||
                                githubUrl
                                  .replace(/^https:\/\/github\.com\//, "")
                                  .replace(/\.git$/, "")}
                            </p>
                            <p className="mt-0.5 text-xs text-muted-foreground">
                              Deploy from {refKind === "tag" ? "tag" : "branch"}
                            </p>
                          </div>
                        </div>
                        <Button 
                          variant="ghost" 
                          size="sm" 
                          onClick={() => setStep(1)} 
                          className="shrink-0 text-xs font-medium text-brand hover:text-brand"
                        >
                          Change
                        </Button>
                      </div>
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
                </Card>

                <Card className="space-y-5 rounded-2xl border-border/60 p-5">
                  <div>
                    <h3 className="flex items-center gap-2 text-base font-semibold tracking-tight">
                      <Box className="h-4 w-4 text-muted-foreground" />
                      Build
                    </h3>
                    <p className="mt-1 text-xs text-muted-foreground">
                      Choose how Docklift turns your source into a container.
                    </p>
                  </div>

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
                          "rounded-2xl border p-3 text-left transition-colors",
                          buildType === value
                            ? "border-brand/40 bg-brand/10"
                            : "border-border/60 bg-card/40 hover:bg-secondary/40",
                        )}
                      >
                        <span className="text-sm font-semibold">{label}</span>
                        <span className="mt-1 block text-[10px] leading-relaxed text-muted-foreground">
                          {description}
                        </span>
                      </button>
                    ))}
                  </div>

                  <div className="grid grid-cols-1 sm:grid-cols-2 gap-4">
                    <div className="space-y-2">
                      <label className="text-sm font-medium">Base Directory</label>
                      <Input
                        value={baseDirectory}
                        onChange={(e) => setBaseDirectory(e.target.value)}
                        placeholder="."
                        className="h-10 bg-secondary/30 font-mono"
                      />
                      <p className="text-[10px] text-muted-foreground">
                        Relative to the project root, for example <code>apps/web</code>.
                      </p>
                    </div>
                    <div className="space-y-2">
                      <label className="text-sm font-medium">Internal Port</label>
                      <Input
                        type="number"
                        min={1}
                        max={65535}
                        value={internalPort}
                        onChange={(e) => setInternalPort(Number(e.target.value))}
                        className="h-10 bg-secondary/30 font-mono"
                      />
                      <p className="text-[10px] text-muted-foreground">
                        The port your application listens on inside its container.
                      </p>
                    </div>
                  </div>

                  {buildType === "dockerfile" && (
                    <div className="space-y-2">
                      <label className="text-sm font-medium">Dockerfile Path</label>
                      <Input
                        value={dockerfilePath}
                        onChange={(e) => setDockerfilePath(e.target.value)}
                        placeholder="Dockerfile"
                        className="h-10 bg-secondary/30 font-mono"
                      />
                      <p className="text-[10px] text-muted-foreground">
                        Path relative to the base directory.
                      </p>
                    </div>
                  )}
                </Card>

                <Card className="space-y-4 rounded-2xl border-border/60 p-5">
                  <div className="flex flex-col justify-between gap-3 sm:flex-row sm:items-center">
                    <h3 className="flex items-center gap-2 text-base font-semibold tracking-tight">
                      <Lock className="h-4 w-4 text-muted-foreground" />
                      Environment Variables
                    </h3>

                    <div className="flex flex-wrap items-center gap-2">
                    <Dialog open={showAddEnv} onOpenChange={setShowAddEnv}>
                      <DialogTrigger asChild>
                        <Button variant="outline" size="sm" className="h-9 rounded-xl border-border/60 text-xs">
                          <PlusIcon className="mr-1.5 h-3.5 w-3.5" /> Add
                        </Button>
                      </DialogTrigger>
                      <DialogContent className="sm:max-w-md">
                        <DialogHeader>
                          <DialogTitle>Add Environment Variable</DialogTitle>
                        </DialogHeader>
                        <div className="space-y-6 pt-4">
                          <div className="grid grid-cols-1 gap-4">
                            <div className="space-y-1.5">
                              <label className="ml-1 text-[10px] font-semibold uppercase tracking-wider text-muted-foreground">Key</label>
                              <Input 
                                placeholder="e.g. API_KEY" 
                                value={newEnvKey}
                                onChange={(e) => setNewEnvKey(e.target.value.toUpperCase().replace(/\s+/g, '_'))}
                                className="h-11 border-border/40 bg-secondary/20 font-mono"
                              />
                            </div>
                            <div className="space-y-1.5">
                              <label className="ml-1 text-[10px] font-semibold uppercase tracking-wider text-muted-foreground">Value</label>
                              <Input 
                                placeholder="Value..." 
                                type="text"
                                value={newEnvValue}
                                onChange={(e) => setNewEnvValue(e.target.value)}
                                className="h-11 border-border/40 bg-secondary/20 font-mono"
                              />
                            </div>
                          </div>
                          
                          <div className="flex flex-wrap items-center gap-4">
                            <div className="flex flex-col gap-1.5">
                              <button 
                                type="button"
                                onClick={() => setNewEnvIsBuild(!newEnvIsBuild)}
                                className={cn(
                                  "inline-flex w-fit items-center gap-2 rounded-xl border px-3 py-1.5 text-[10px] font-semibold uppercase tracking-wider transition-colors",
                                  newEnvIsBuild ? "border-brand/30 bg-brand/10 text-brand" : "border-border/40 text-muted-foreground hover:border-border"
                                )}
                              >
                                <FlaskConical className="h-3 w-3" /> Inject in Build
                              </button>
                              <span className="px-1 text-[9px] font-medium text-muted-foreground">Next.js/Prisma build-args</span>
                            </div>

                            <div className="flex flex-col gap-1.5">
                              <button 
                                type="button"
                                onClick={() => setNewEnvIsRuntime(!newEnvIsRuntime)}
                                className={cn(
                                  "inline-flex w-fit items-center gap-2 rounded-xl border px-3 py-1.5 text-[10px] font-semibold uppercase tracking-wider transition-colors",
                                  newEnvIsRuntime ? "border-brand/30 bg-brand/10 text-brand" : "border-border/40 text-muted-foreground hover:border-border"
                                )}
                              >
                                <Globe className="h-3 w-3" /> Inject in Runtime
                              </button>
                              <span className="px-1 text-[9px] font-medium text-muted-foreground">Available after deploy</span>
                            </div>
                          </div>

                          {!newEnvIsBuild && !newEnvIsRuntime && (
                            <div className="flex gap-2 rounded-xl border border-amber-500/20 bg-amber-500/5 p-3">
                              <Info className="h-4 w-4 shrink-0 text-amber-500" />
                              <p className="text-[10px] font-medium text-amber-700 dark:text-amber-400">You must select at least one injection scope for this variable to be active.</p>
                            </div>
                          )}

                          <div className="flex justify-end gap-3 pt-2">
                            <Button variant="ghost" onClick={() => setShowAddEnv(false)} className="rounded-xl">Cancel</Button>
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
                              className="h-11 rounded-xl bg-brand px-8 font-semibold text-brand-foreground shadow-lg shadow-brand/20 hover:brightness-110 disabled:opacity-50"
                            >
                              Add Variable
                            </Button>
                          </div>
                        </div>
                      </DialogContent>
                    </Dialog>

                    <Dialog open={showBulkEnv} onOpenChange={setShowBulkEnv}>
                      <DialogTrigger asChild>
                        <Button variant="outline" size="sm" className="h-9 w-full rounded-xl border-border/60 px-4 text-xs sm:w-auto">
                          <Upload className="mr-2 h-4 w-4" /> Bulk Import
                        </Button>
                      </DialogTrigger>
                      <DialogContent className="sm:max-w-lg">
                        <DialogHeader>
                          <DialogTitle>Bulk Import Environment Variables</DialogTitle>
                        </DialogHeader>
                        <div className="space-y-4 pt-4">
                          <div className="space-y-2">
                            <label className="ml-1 text-[10px] font-semibold uppercase tracking-wider text-muted-foreground">
                              Paste KEY=VALUE pairs (one per line)
                            </label>
                            <textarea
                              placeholder={`ADMIN_USERNAME=admin
DATABASE_URL=postgresql://user:pass@host:5432/db
SESSION_SECRET=your-secret-here
# Lines starting with # are ignored`}
                              value={bulkEnvContent}
                              onChange={(e) => setBulkEnvContent(e.target.value)}
                              className="h-40 w-full resize-none rounded-xl border border-border/40 bg-secondary/20 p-4 font-mono text-sm outline-none focus:border-brand/50 focus:ring-2 focus:ring-brand/20"
                            />
                          </div>
                          
                          <div className="flex flex-col gap-3 rounded-xl border border-border/40 bg-secondary/30 p-3 sm:flex-row sm:items-center sm:gap-6">
                            <div className="group flex cursor-pointer items-center gap-2" onClick={() => setBulkIsBuild(!bulkIsBuild)}>
                              <div className={cn(
                                "flex h-4 w-4 items-center justify-center rounded border transition-colors",
                                bulkIsBuild ? "border-brand bg-brand" : "border-muted-foreground/40 bg-transparent"
                              )}>
                                {bulkIsBuild && <Check className="h-3 w-3 text-brand-foreground" strokeWidth={4} />}
                              </div>
                              <div className="flex flex-col">
                                <span className="text-xs font-semibold">Build Argument</span>
                                <span className="text-[10px] text-muted-foreground">Used during Docker build</span>
                              </div>
                            </div>

                            <div className="group flex cursor-pointer items-center gap-2" onClick={() => setBulkIsRuntime(!bulkIsRuntime)}>
                              <div className={cn(
                                "flex h-4 w-4 items-center justify-center rounded border transition-colors",
                                bulkIsRuntime ? "border-brand bg-brand" : "border-muted-foreground/40 bg-transparent"
                              )}>
                                {bulkIsRuntime && <Check className="h-3 w-3 text-brand-foreground" strokeWidth={4} />}
                              </div>
                              <div className="flex flex-col">
                                <span className="text-xs font-semibold">Runtime Variable</span>
                                <span className="text-[10px] text-muted-foreground">Available to your app</span>
                              </div>
                            </div>
                          </div>

                          <p className="text-xs text-muted-foreground">
                            {bulkEnvContent.split('\n').filter(l => l.trim() && !l.trim().startsWith('#') && l.includes('=')).length} variables detected
                          </p>
                          <div className="flex justify-end gap-3 pt-2">
                            <Button variant="ghost" onClick={() => setShowBulkEnv(false)} className="rounded-xl">Cancel</Button>
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
                              className="h-11 rounded-xl bg-brand px-8 font-semibold text-brand-foreground shadow-lg shadow-brand/20 hover:brightness-110 disabled:opacity-50"
                            >
                              Import All
                            </Button>
                          </div>
                        </div>
                      </DialogContent>
                    </Dialog>
                    </div>
                  </div>

                  <div className="space-y-4">
                    {envVars.length === 0 ? (
                      <div className="rounded-2xl border border-dashed border-border/60 bg-secondary/10 px-4 py-12 text-center">
                         <Shield className="mx-auto mb-3 h-8 w-8 text-muted-foreground/30" />
                         <p className="text-xs font-medium text-muted-foreground">No custom environment variables added yet.</p>
                      </div>
                    ) : (
                      <div className="grid grid-cols-1 gap-2">
                        {envVars.map((env, i) => (
                          <div key={i} className="group flex items-center justify-between rounded-xl border border-border/60 bg-secondary/30 p-3.5 transition-colors hover:border-brand/30">
                            <div className="flex min-w-0 items-center gap-3 sm:gap-4">
                              <div className="flex h-8 w-8 shrink-0 items-center justify-center rounded-lg border border-border/60 bg-background/50">
                                <Lock className="h-3.5 w-3.5 text-muted-foreground" />
                              </div>
                              <div className="flex min-w-0 flex-col">
                                <div className="flex flex-wrap items-center gap-2">
                                  <span className="truncate font-mono text-sm font-semibold tracking-tight">{env.key}</span>
                                  <div className="flex gap-1.5">
                                    <div className={cn(
                                      "flex h-5 items-center gap-1 rounded-md border px-1.5 text-[8px] font-semibold uppercase tracking-wider",
                                      env.is_build_arg 
                                        ? "border-brand/30 bg-brand/10 text-brand" 
                                        : "border-border/60 bg-secondary/40 text-muted-foreground"
                                    )}>
                                      <FlaskConical className="h-2.5 w-2.5" />
                                      Bld
                                    </div>
                                    <div className={cn(
                                      "flex h-5 items-center gap-1 rounded-md border px-1.5 text-[8px] font-semibold uppercase tracking-wider",
                                      env.is_runtime 
                                        ? "border-brand/30 bg-brand/10 text-brand" 
                                        : "border-border/60 bg-secondary/40 text-muted-foreground"
                                    )}>
                                      <Globe className="h-2.5 w-2.5" />
                                      Run
                                    </div>
                                  </div>
                                </div>
                                <span className="mt-0.5 truncate font-mono text-xs text-muted-foreground opacity-80">
                                  {revealedEnvs.includes(i) ? env.value : "••••••••••••"}
                                </span>
                              </div>
                            </div>
                            <div className="flex items-center gap-1 opacity-100 sm:opacity-0 sm:group-hover:opacity-100">
                              <button 
                                type="button"
                                onClick={() => {
                                  if (revealedEnvs.includes(i)) {
                                    setRevealedEnvs(revealedEnvs.filter(idx => idx !== i));
                                  } else {
                                    setRevealedEnvs([...revealedEnvs, i]);
                                  }
                                }}
                                className="rounded-lg p-2 text-muted-foreground transition-colors hover:bg-secondary"
                              >
                                {revealedEnvs.includes(i) ? <EyeOff className="h-4 w-4" /> : <Eye className="h-4 w-4" />}
                              </button>
                              <button 
                                type="button"
                                onClick={() => setEnvVars(envVars.filter((_, idx) => idx !== i))}
                                className="rounded-lg p-2 text-muted-foreground transition-colors hover:bg-destructive/5 hover:text-destructive"
                              >
                                <Trash2 className="h-4 w-4" />
                              </button>
                            </div>
                          </div>
                        ))}
                      </div>
                    )}
                  </div>
                </Card>

              {/* Sticky one-page CTA — same width as form, no empty sidebar */}
              <div className="pointer-events-none fixed inset-x-0 bottom-0 z-20 lg:left-[var(--shell-rail,17rem)]">
                <div className="pointer-events-auto border-t border-border/60 bg-background/90 px-3 py-3 backdrop-blur-xl sm:px-4 md:px-8">
                  <div className="mx-auto flex w-full max-w-7xl flex-col gap-3 sm:flex-row sm:items-center sm:justify-between sm:gap-4">
                    <div className="min-w-0">
                      <p className="truncate text-sm font-semibold tracking-tight">
                        {name || "Untitled project"}
                      </p>
                      <div className="mt-1.5 flex flex-wrap gap-1.5">
                        <span className="inline-flex items-center rounded-lg border border-border/60 bg-secondary/40 px-2 py-0.5 text-[10px] font-medium text-muted-foreground">
                          Application
                        </span>
                        <span className="inline-flex items-center rounded-lg border border-border/60 bg-secondary/40 px-2 py-0.5 text-[10px] font-medium capitalize text-muted-foreground">
                          {sourceType === "github" ? "Private" : sourceType}
                        </span>
                        <span className="inline-flex items-center rounded-lg border border-border/60 bg-secondary/40 px-2 py-0.5 text-[10px] font-medium capitalize text-muted-foreground">
                          {buildType}
                        </span>
                        {githubBranch && (
                          <span className="inline-flex max-w-[12rem] items-center truncate rounded-lg border border-brand/25 bg-brand/10 px-2 py-0.5 text-[10px] font-medium text-brand">
                            {refKind === "tag" ? "tag" : "branch"}:{githubBranch}
                          </span>
                        )}
                        {envVars.length > 0 && (
                          <span className="inline-flex items-center rounded-lg border border-border/60 bg-secondary/40 px-2 py-0.5 text-[10px] font-medium text-muted-foreground">
                            {envVars.length} env
                          </span>
                        )}
                      </div>
                    </div>
                    <Button
                      onClick={handleSubmit}
                      disabled={loading || !name.trim()}
                      className="h-11 w-full shrink-0 bg-brand px-6 font-semibold text-brand-foreground shadow-lg shadow-brand/20 hover:brightness-110 sm:w-auto sm:min-w-[11rem]"
                    >
                      {loading ? (
                        <Loader2 className="mr-2 h-4 w-4 animate-spin" />
                      ) : (
                        <Sparkles className="mr-2 h-4 w-4" />
                      )}
                      Create Project
                    </Button>
                  </div>
                </div>
              </div>
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
        <Loader2 className="h-8 w-8 animate-spin text-brand" />
      </div>
    }>
      <NewProjectContent />
    </Suspense>
  );
}
