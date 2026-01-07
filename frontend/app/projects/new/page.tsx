// New project page - wizard for creating projects via GitHub, public repo, or file upload
"use client";

import { useState, useEffect, Suspense } from "react";
import { useRouter, useSearchParams } from "next/navigation";
import { Header } from "@/components/Header";
import { Footer } from "@/components/Footer";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Card } from "@/components/ui/card";
import { BranchSelector } from "@/components/BranchSelector";
import { GithubIcon } from "@/components/icons/GithubIcon";
import {
  Upload,
  FolderUp,
  Loader2,
  Sparkles,
  Check,
  Lock,
  Search,
  ExternalLink,
  Globe,
  Database,
  ArrowLeft,
  ArrowRight,
  Shield,
  FlaskConical,
  Link as LinkIcon,
  Info,
  ChevronRight,
  Plus as PlusIcon,
  Trash2,
  X,
  Eye,
  EyeOff,
} from "lucide-react";
import { API_URL, cn } from "@/lib/utils";
import { getAuthHeaders } from "@/lib/auth";
import { toast } from "sonner";
import axios from "axios";
import { Project } from "@/lib/types";
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

function NewProjectContent() {
  const router = useRouter();
  const searchParams = useSearchParams();
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

  // Environment Variables State
  const [envVars, setEnvVars] = useState<{key: string, value: string, is_build_arg: boolean, is_runtime: boolean}[]>([]);
  const [newEnvKey, setNewEnvKey] = useState("");
  const [newEnvValue, setNewEnvValue] = useState("");
  const [newEnvIsBuild, setNewEnvIsBuild] = useState(false);
  const [newEnvIsRuntime, setNewEnvIsRuntime] = useState(true);

  // Database Connection Helper State
  const [needsDb, setNeedsDb] = useState(false);
  const [dbUrl, setDbUrl] = useState("");
  const [dbInBuild, setDbInBuild] = useState(true);
  const [dbInRuntime, setDbInRuntime] = useState(true);
  const [existingDbs, setExistingDbs] = useState<Project[]>([]);
  const [showDbAssistant, setShowDbAssistant] = useState(false);
  const [showAddEnv, setShowAddEnv] = useState(false);
  const [showBulkEnv, setShowBulkEnv] = useState(false);
  const [bulkEnvContent, setBulkEnvContent] = useState("");
  const [bulkIsBuild, setBulkIsBuild] = useState(true);
  const [bulkIsRuntime, setBulkIsRuntime] = useState(true);
  const [revealedEnvs, setRevealedEnvs] = useState<number[]>([]);

  // GitHub state
  const [githubStatus, setGithubStatus] = useState<GitHubStatus | null>(null);
  const [githubLoading, setGithubLoading] = useState(false);
  const [repos, setRepos] = useState<GitHubRepo[]>([]);
  const [reposLoading, setReposLoading] = useState(false);
  const [searchQuery, setSearchQuery] = useState("");
  const [selectedRepo, setSelectedRepo] = useState<GitHubRepo | null>(null);
  const [showGitHubConnect, setShowGitHubConnect] = useState(false);
  const [branches, setBranches] = useState<string[]>([]);
  const [branchesLoading, setBranchesLoading] = useState(false);
  const [repoAccessError, setRepoAccessError] = useState<string | null>(null);

  useEffect(() => {
    fetchGitHubStatus();
    fetchExistingDatabases();
  }, []);

  // Debounced branch fetch for public repo
  useEffect(() => {
    if (sourceType === "public" && githubUrl && isValidGithubUrl(githubUrl)) {
      setBranches([]); // Clear previous branches
      setGithubBranch(""); // Clear previous selection
      setRepoAccessError(null); // Clear previous error
      const timer = setTimeout(() => {
        const match = githubUrl.match(/github\.com\/([^\/]+\/[^\/]+)/);
        if (match) {
          const repoPath = match[1].replace(/\.git$/, "").replace(/\/$/, "");
          fetchBranches(repoPath, "public");
        }
      }, 600);
      return () => clearTimeout(timer);
    } else {
        setBranches([]);
        setRepoAccessError(null);
    }
  }, [githubUrl, sourceType]);

  const fetchBranches = async (repoIdentifier: string, type: "public" | "private") => {
    setBranchesLoading(true);
    setRepoAccessError(null);
    try {
      const res = await fetch(`${API_URL}/api/github/branches?repo=${repoIdentifier}&type=${type}`, { headers: getAuthHeaders() });
      if (!res.ok) {
        const errorData = await res.json().catch(() => ({}));
        if (res.status === 404) {
          if (type === "public") {
            setRepoAccessError("Repository not found or is private. For private repos, use the 'Private Repository' tab.");
          }
          throw new Error("Not found");
        }
        if (res.status === 403) {
          if (type === "public") {
            setRepoAccessError("Cannot access this repository. It may be private - please use the 'Private Repository' tab.");
          }
          throw new Error("Forbidden");
        }
        throw new Error(errorData.error || "Failed");
      }
      const data = await res.json();
      setBranches(data);
      setRepoAccessError(null);
      
      // Auto-select default branch if current selection is invalid
      if (!data.includes(githubBranch)) {
         if (data.includes("main")) setGithubBranch("main");
         else if (data.includes("master")) setGithubBranch("master");
         else if (data.length > 0) setGithubBranch(data[0]);
      }
    } catch {
       // Silent fail for public typing
       if (type === 'private') toast.error("Failed to fetch branches");
    } finally {
      setBranchesLoading(false);
    }
  };

  const fetchGitHubStatus = async () => {
    setGithubLoading(true);
    try {
      const res = await fetch(`${API_URL}/api/github/status`, { headers: getAuthHeaders() });
      const data = await res.json();
      setGithubStatus(data);
      if (data.connected) {
        fetchRepos();
      }
    } catch {
      setGithubStatus({ connected: false });
    } finally {
      setGithubLoading(false);
    }
  };

  const fetchRepos = async () => {
    setReposLoading(true);
    try {
      const res = await fetch(`${API_URL}/api/github/repos?per_page=50`, { headers: getAuthHeaders() });
      const data = await res.json();
      setRepos(data);
    } catch {
      toast.error("Failed to fetch repositories");
    } finally {
      setReposLoading(false);
    }
  };

  const fetchExistingDatabases = async () => {
    try {
      const res = await fetch(`${API_URL}/api/projects`, { headers: getAuthHeaders() });
      const data: Project[] = await res.json();
      setExistingDbs(data.filter(p => p.project_type === "database"));
    } catch (error) {
      console.error("Failed to fetch databases");
    }
  };

  const handleConnectGitHub = () => {
    const returnUrl = encodeURIComponent(window.location.href);
    window.location.href = `${API_URL}/api/github/install?return_url=${returnUrl}`;
  };

  const handleSelectRepo = (repo: GitHubRepo) => {
    setSelectedRepo(repo);
    setName(repo.name);
    setGithubUrl(repo.clone_url);
    setGithubBranch(repo.default_branch);
    fetchBranches(repo.full_name, "private");
    setStep(2); // Auto-advance to config (previously 3)
  };

  const filteredRepos = repos.filter(
    (repo) =>
      repo.name.toLowerCase().includes(searchQuery.toLowerCase()) ||
      repo.full_name.toLowerCase().includes(searchQuery.toLowerCase())
  );

  const handleSubmit = async () => {
    if (!name) {
      toast.error("Project name is required");
      return;
    }

    setLoading(true);
    try {
      const formData = new FormData();
      formData.append("name", name);
      formData.append("source_type", sourceType === "public" ? "github" : sourceType);
      formData.append("project_type", projectType);

      if (sourceType === "github" || sourceType === "public") {
        formData.append("github_url", githubUrl);
        formData.append("github_branch", githubBranch);
      } else if (files) {
        Array.from(files).forEach((file) => formData.append("files", file));
      }

      // 1. Create Project
      const response = await axios.post(`${API_URL}/api/projects`, formData, {
        headers: { 
          "Content-Type": "multipart/form-data",
          ...getAuthHeaders() 
        } as any,
        timeout: 300000,
      });

      const newProject = response.data;

      // 2. Inject ENVs
      const varsToInject = [...envVars];
      if (needsDb && dbUrl) {
        varsToInject.push({
          key: "DATABASE_URL",
          value: dbUrl,
          is_build_arg: dbInBuild,
          is_runtime: dbInRuntime,
        });
      }

      for (const env of varsToInject) {
        await fetch(`${API_URL}/api/projects/${newProject.id}/env`, {
          method: "POST",
          headers: { "Content-Type": "application/json", ...getAuthHeaders() },
          body: JSON.stringify(env),
        });
      }

      toast.success("Project created successfully");
      router.push(`/projects/${newProject.id}`);
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
    
    if (!name) setName(githubUrl.split("/").pop()?.replace(".git", "") || "my-app");
    setStep(2);
  };

  // Check if URL is a valid GitHub format
  const isValidGithubUrl = (url: string) => {
    if (!url) return null; // neutral
    const pattern = /^https:\/\/github\.com\/[a-zA-Z0-9_.-]+\/[a-zA-Z0-9_.-]+(\.git)?\/?$/;
    return pattern.test(url.trim());
  };

  return (
    <div className="min-h-screen flex flex-col bg-background">
      <Header />

      <main className="flex-1 container max-w-4xl mx-auto px-4 sm:px-6 py-8 sm:py-12">
        {/* Navigation & Header */}
        <div className="mb-8 sm:mb-10">
          <button 
            onClick={() => step > 1 ? setStep(step - 1) : router.push("/")}
            className="inline-flex items-center gap-2 text-sm text-muted-foreground hover:text-foreground transition-colors mb-4 group"
          >
            <ArrowLeft className="h-4 w-4 group-hover:-translate-x-0.5 transition-transform" />
            {step === 1 ? "Dashboard" : "Back"}
          </button>
          
          <div className="flex flex-col sm:flex-row sm:items-center justify-between gap-4">
            <div>
              <h1 className="text-2xl sm:text-3xl font-bold tracking-tight">
                New Project
              </h1>
              <p className="text-muted-foreground text-sm mt-1">
                {step === 1 && "Select your code source"}
                {step === 2 && "Configure environment"}
              </p>
            </div>
            
            {/* Step Indicator */}
            <div className="flex items-center gap-2 bg-secondary/50 rounded-full px-3 py-1.5">
              <div className={cn(
                "flex items-center justify-center h-6 w-6 rounded-full text-xs font-bold transition-all",
                step >= 1 ? "bg-foreground text-background" : "bg-muted text-muted-foreground"
              )}>1</div>
              <div className={cn("w-6 h-0.5 rounded-full", step >= 2 ? "bg-foreground" : "bg-muted")} />
              <div className={cn(
                "flex items-center justify-center h-6 w-6 rounded-full text-xs font-bold transition-all",
                step >= 2 ? "bg-foreground text-background" : "bg-muted text-muted-foreground"
              )}>2</div>
            </div>
          </div>
        </div>

        <div className="space-y-6">
          {/* STEP 1: Source Selection */}
          {step === 1 && (
            <div className="space-y-6">
              {/* Source Type Tabs */}
              <div className="flex justify-center">
                <div className="inline-flex flex-wrap justify-center gap-2">
                  <button
                    onClick={() => setSourceType("public")}
                    className={cn(
                      "flex items-center gap-1.5 px-3 py-2 rounded-lg text-sm font-medium transition-all",
                      sourceType === "public" 
                        ? "bg-foreground text-background shadow-md" 
                        : "bg-secondary/50 text-muted-foreground hover:text-foreground hover:bg-secondary"
                    )}
                  >
                    <Globe className="h-4 w-4" />
                    <span className="hidden sm:inline">Public Repository</span>
                    <span className="sm:hidden">Public</span>
                  </button>
                  <button
                    onClick={() => setSourceType("github")}
                    className={cn(
                      "flex items-center gap-1.5 px-3 py-2 rounded-lg text-sm font-medium transition-all",
                      sourceType === "github" 
                        ? "bg-foreground text-background shadow-md" 
                        : "bg-secondary/50 text-muted-foreground hover:text-foreground hover:bg-secondary"
                    )}
                  >
                    <Lock className="h-4 w-4" />
                    <span className="hidden sm:inline">Private Repository</span>
                    <span className="sm:hidden">Private</span>
                  </button>
                  <button
                    onClick={() => setSourceType("upload")}
                    className={cn(
                      "flex items-center gap-1.5 px-3 py-2 rounded-lg text-sm font-medium transition-all",
                      sourceType === "upload" 
                        ? "bg-foreground text-background shadow-md" 
                        : "bg-secondary/50 text-muted-foreground hover:text-foreground hover:bg-secondary"
                    )}
                  >
                    <Upload className="h-4 w-4" />
                    Direct Upload
                  </button>
                </div>
              </div>

              {sourceType === "public" && (
                <Card className="p-5 sm:p-6 border-border/30 rounded-2xl">
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
                          githubUrl && isValidGithubUrl(githubUrl) === true && "border-green-500 focus-visible:ring-green-500"
                        )}
                      />
                      {githubUrl && isValidGithubUrl(githubUrl) === false && (
                        <p className="text-xs text-red-500 flex items-center gap-1">
                          <X className="h-3 w-3" /> Invalid URL format
                        </p>
                      )}
                      {githubUrl && isValidGithubUrl(githubUrl) === true && (
                        <p className="text-xs text-green-500 flex items-center gap-1">
                          <Check className="h-3 w-3" /> Valid URL
                        </p>
                      )}
                    </div>
                    <div className="space-y-2">
                      <label className="text-sm font-medium">Branch</label>
                      <BranchSelector
                        branches={branches}
                        value={githubBranch}
                        onChange={setGithubBranch}
                        loading={branchesLoading}
                        disabled={!githubUrl || isValidGithubUrl(githubUrl) !== true}
                      />
                      {repoAccessError && (
                        <div className="p-3 rounded-lg bg-amber-500/10 border border-amber-500/20 flex items-start gap-2">
                          <Lock className="h-4 w-4 text-amber-500 shrink-0 mt-0.5" />
                          <span className="text-xs text-amber-600 dark:text-amber-400">{repoAccessError}</span>
                        </div>
                      )}
                    </div>
                    <Button 
                      onClick={handlePublicSubmit} 
                      disabled={!githubUrl || isValidGithubUrl(githubUrl) !== true}
                      className="w-full h-11 bg-zinc-900 hover:bg-zinc-800 text-white font-medium"
                    >
                      Continue <ArrowRight className="h-4 w-4 ml-2" />
                    </Button>
                  </div>
                </Card>
              )}

              {sourceType === "github" && (
                <div className="space-y-4">
                  {!githubStatus?.connected ? (
                    <Card className="p-8 text-center border-dashed border-2 rounded-2xl flex flex-col items-center">
                      <div className="h-14 w-14 rounded-xl bg-zinc-900 flex items-center justify-center mb-4">
                        <GithubIcon className="h-7 w-7 text-white" />
                      </div>
                      <h3 className="text-lg font-bold mb-2">Connect GitHub</h3>
                      <p className="text-sm text-muted-foreground max-w-xs mb-6">
                        Create a GitHub App to access your repositories.
                      </p>
                      <Button onClick={() => setShowGitHubConnect(true)} className="h-10 px-6 bg-zinc-900 hover:bg-zinc-800 text-white">
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
                    <Card className="border-border/30 rounded-2xl overflow-hidden">
                      {/* Header */}
                      <div className="bg-secondary/30 px-4 py-3 flex flex-col sm:flex-row sm:items-center justify-between gap-3 border-b border-border/30">
                        <div className="flex items-center gap-2">
                          <div className="h-2 w-2 rounded-full bg-green-500" />
                          <span className="text-xs font-medium text-muted-foreground">@{githubStatus.username}</span>
                        </div>
                        <div className="relative w-full sm:w-56">
                          <Search className="absolute left-3 top-1/2 -translate-y-1/2 h-3.5 w-3.5 text-muted-foreground" />
                          <Input 
                            placeholder="Search repos..." 
                            value={searchQuery}
                            onChange={(e) => setSearchQuery(e.target.value)}
                            className="pl-9 h-9 text-sm bg-background/50 border-border/40" 
                          />
                        </div>
                      </div>
                      {/* Repo List */}
                      <div className="max-h-80 overflow-y-auto divide-y divide-border/20">
                        {reposLoading ? (
                          <div className="p-12 flex flex-col items-center gap-3 text-muted-foreground">
                            <Loader2 className="h-6 w-6 animate-spin" />
                            <span className="text-sm">Loading repos...</span>
                          </div>
                        ) : filteredRepos.length === 0 ? (
                          <div className="p-12 text-center text-sm text-muted-foreground">
                            No repositories found
                          </div>
                        ) : filteredRepos.map(repo => (
                          <div 
                            key={repo.id}
                            onClick={() => handleSelectRepo(repo)}
                            className="group flex items-center justify-between p-4 hover:bg-secondary/30 cursor-pointer transition-colors"
                          >
                            <div className="flex items-center gap-3 min-w-0">
                              <div className="h-9 w-9 rounded-lg bg-secondary/50 flex items-center justify-center shrink-0">
                                {repo.private ? <Lock className="h-4 w-4 text-muted-foreground" /> : <Globe className="h-4 w-4 text-muted-foreground" />}
                              </div>
                              <div className="min-w-0">
                                <h4 className="font-medium text-sm truncate">{repo.name}</h4>
                                <p className="text-xs text-muted-foreground truncate">{repo.description || "No description"}</p>
                              </div>
                            </div>
                            <ArrowRight className="h-4 w-4 text-muted-foreground opacity-0 group-hover:opacity-100 shrink-0 ml-2" />
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
                    "border-2 border-dashed rounded-2xl p-10 sm:p-12 text-center cursor-pointer transition-all",
                    dragActive ? "border-cyan-500 bg-cyan-500/5" : "border-border/50 hover:border-cyan-500/40 hover:bg-secondary/20",
                    files && "border-cyan-500/50 bg-cyan-500/5"
                  )}
                  onClick={() => document.getElementById("file-upload-input")?.click()}
                >
                  <input id="file-upload-input" type="file" multiple className="hidden" onChange={(e) => { setFiles(e.target.files); setStep(2); }} />
                  <div className="flex flex-col items-center">
                    <div className="h-14 w-14 rounded-xl bg-secondary flex items-center justify-center mb-4">
                      <FolderUp className="h-7 w-7 text-muted-foreground" />
                    </div>
                    <h3 className="text-lg font-bold mb-2">Upload Project</h3>
                    <p className="text-sm text-muted-foreground max-w-sm">
                      Drop a ZIP file or click to browse
                    </p>
                    {files && (
                      <div className="mt-4 px-4 py-1.5 rounded-full bg-cyan-500/10 text-cyan-500 text-sm font-medium flex items-center gap-2">
                        <Check className="h-4 w-4" /> {files.length} file(s) selected
                      </div>
                    )}
                  </div>
                </Card>
              )}
            </div>
          )}
          {step === 2 && (
            <div className="grid grid-cols-1 lg:grid-cols-3 gap-6">
              <div className="lg:col-span-2 space-y-6">
                {/* General Config */}
                <Card className="p-5 space-y-4 rounded-2xl border-border/30">
                  <h3 className="text-base font-semibold flex items-center gap-2">
                    <Sparkles className="h-4 w-4 text-cyan-500" />
                    Configuration
                  </h3>
                  
                  <div className="space-y-2">
                    <label className="text-sm font-medium">Project Name</label>
                    <Input 
                      placeholder="my-app" 
                      value={name}
                      onChange={(e) => setName(e.target.value)}
                      className="h-10 bg-secondary/30" 
                    />
                  </div>

                  {sourceType === "github" && selectedRepo && (
                    <div className="p-4 rounded-xl bg-secondary/30 border border-border/30 flex flex-col sm:flex-row sm:items-center justify-between gap-3">
                      <div className="flex items-center gap-3 min-w-0">
                        <GithubIcon className="h-5 w-5 shrink-0" />
                        <div className="min-w-0">
                          <p className="text-sm font-medium truncate">{selectedRepo?.full_name}</p>
                          <div className="flex items-center gap-2 mt-1">
                            <span className="text-xs text-muted-foreground">Branch:</span>
                            <BranchSelector
                                branches={branches}
                                value={githubBranch}
                                onChange={setGithubBranch}
                                loading={branchesLoading}
                                className="h-8 text-xs w-36"
                            />
                          </div>
                        </div>
                      </div>
                      <Button 
                        variant="ghost" 
                        size="sm" 
                        onClick={() => setStep(1)} 
                        className="text-xs text-cyan-500 hover:text-cyan-600"
                      >
                        Change
                      </Button>
                    </div>
                  )}
                </Card>

                {/* ENVIRONMENT VARIABLES MANAGER */}
                <Card className="p-5 space-y-4 rounded-2xl border-border/30">
                  <div className="flex flex-col sm:flex-row sm:items-center justify-between gap-3">
                    <h3 className="text-base font-semibold flex items-center gap-2">
                      <Lock className="h-4 w-4 text-emerald-500" />
                      Environment Variables
                    </h3>

                    <div className="flex items-center gap-2">
                    <Dialog open={showAddEnv} onOpenChange={setShowAddEnv}>
                      <DialogTrigger asChild>
                        <Button variant="outline" size="sm" className="h-8 text-xs">
                          <PlusIcon className="h-3.5 w-3.5 mr-1.5" /> Add
                        </Button>
                      </DialogTrigger>
                      <DialogContent className="sm:max-w-md">
                        <DialogHeader>
                          <DialogTitle>Add Environment Variable</DialogTitle>
                        </DialogHeader>
                        <div className="space-y-6 pt-4">
                          <div className="grid grid-cols-1 gap-4">
                            <div className="space-y-1.5">
                              <label className="text-[10px] font-bold text-muted-foreground uppercase tracking-widest ml-1">Key</label>
                              <Input 
                                placeholder="e.g. API_KEY" 
                                value={newEnvKey}
                                onChange={(e) => setNewEnvKey(e.target.value.toUpperCase().replace(/\s+/g, '_'))}
                                className="bg-secondary/20 h-11 border-border/40 font-mono"
                              />
                            </div>
                            <div className="space-y-1.5">
                              <label className="text-[10px] font-bold text-muted-foreground uppercase tracking-widest ml-1">Value</label>
                              <Input 
                                placeholder="Value..." 
                                type="text"
                                value={newEnvValue}
                                onChange={(e) => setNewEnvValue(e.target.value)}
                                className="bg-secondary/20 h-11 border-border/40 font-mono"
                              />
                            </div>
                          </div>
                          
                          <div className="flex flex-wrap items-center gap-4">
                            <div className="flex flex-col gap-1.5">
                              <button 
                                type="button"
                                onClick={() => setNewEnvIsBuild(!newEnvIsBuild)}
                                className={cn(
                                  "flex items-center gap-2 px-3 py-1.5 rounded-lg border transition-all text-[10px] font-bold uppercase tracking-wider w-fit",
                                  newEnvIsBuild ? "bg-cyan-500/10 border-cyan-500/30 text-cyan-500" : "bg-transparent border-border/40 text-muted-foreground hover:border-border"
                                )}
                              >
                                <FlaskConical className="h-3 w-3" /> Inject in Build
                              </button>
                              <span className="text-[9px] text-muted-foreground px-1 font-medium">Next.js/Prisma build-args</span>
                            </div>

                            <div className="flex flex-col gap-1.5">
                              <button 
                                type="button"
                                onClick={() => setNewEnvIsRuntime(!newEnvIsRuntime)}
                                className={cn(
                                  "flex items-center gap-2 px-3 py-1.5 rounded-lg border transition-all text-[10px] font-bold uppercase tracking-wider w-fit",
                                  newEnvIsRuntime ? "bg-blue-500/10 border-blue-500/30 text-blue-500" : "bg-transparent border-border/40 text-muted-foreground hover:border-border"
                                )}
                              >
                                <Globe className="h-3 w-3" /> Inject in Runtime
                              </button>
                              <span className="text-[9px] text-muted-foreground px-1 font-medium">Available after deploy</span>
                            </div>
                          </div>

                          {!newEnvIsBuild && !newEnvIsRuntime && (
                            <div className="p-3 rounded-xl bg-amber-500/5 border border-amber-500/20 flex gap-2">
                              <Info className="h-4 w-4 text-amber-500 shrink-0" />
                              <p className="text-[10px] text-amber-700 font-medium">You must select at least one injection scope for this variable to be active.</p>
                            </div>
                          )}

                          <div className="flex justify-end gap-3 pt-2">
                            <Button variant="ghost" onClick={() => setShowAddEnv(false)} className="rounded-xl font-bold">Cancel</Button>
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
                              className="bg-blue-600 hover:bg-blue-500 text-white h-11 px-8 rounded-xl font-bold shadow-lg disabled:opacity-50"
                            >
                              Add Variable
                            </Button>
                          </div>
                        </div>
                      </DialogContent>
                    </Dialog>

                    <Dialog open={showBulkEnv} onOpenChange={setShowBulkEnv}>
                      <DialogTrigger asChild>
                        <Button variant="outline" size="sm" className="h-9 px-4 rounded-xl font-bold border-cyan-500/20 text-cyan-600 hover:bg-cyan-500/5 hover:border-cyan-500/40 w-full sm:w-auto">
                          <Upload className="h-4 w-4 mr-2" /> Bulk Import
                        </Button>
                      </DialogTrigger>
                      <DialogContent className="sm:max-w-lg">
                        <DialogHeader>
                          <DialogTitle>Bulk Import Environment Variables</DialogTitle>
                        </DialogHeader>
                        <div className="space-y-4 pt-4">
                          <div className="space-y-2">
                            <label className="text-[10px] font-bold text-muted-foreground uppercase tracking-widest ml-1">
                              Paste KEY=VALUE pairs (one per line)
                            </label>
                            <textarea
                              placeholder={`ADMIN_USERNAME=admin
DATABASE_URL=postgresql://user:pass@host:5432/db
SESSION_SECRET=your-secret-here
# Lines starting with # are ignored`}
                              value={bulkEnvContent}
                              onChange={(e) => setBulkEnvContent(e.target.value)}
                              className="w-full h-40 p-4 font-mono text-sm bg-secondary/20 border border-border/40 rounded-xl resize-none focus:outline-none focus:border-cyan-500/50"
                            />
                          </div>
                          
                          {/* Build/Runtime Toggles for Bulk Import */}
                          <div className="flex items-center gap-6 p-3 rounded-xl bg-secondary/30 border border-border/40">
                            <div className="flex items-center gap-2 group cursor-pointer" onClick={() => setBulkIsBuild(!bulkIsBuild)}>
                              <div className={cn(
                                "h-4 w-4 rounded border transition-colors flex items-center justify-center",
                                bulkIsBuild ? "bg-orange-500 border-orange-500" : "bg-transparent border-muted-foreground/40"
                              )}>
                                {bulkIsBuild && <Check className="h-3 w-3 text-white" strokeWidth={4} />}
                              </div>
                              <div className="flex flex-col">
                                <span className="text-xs font-semibold">Build Argument</span>
                                <span className="text-[10px] text-muted-foreground">Used during Docker build</span>
                              </div>
                            </div>

                            <div className="flex items-center gap-2 group cursor-pointer" onClick={() => setBulkIsRuntime(!bulkIsRuntime)}>
                              <div className={cn(
                                "h-4 w-4 rounded border transition-colors flex items-center justify-center",
                                bulkIsRuntime ? "bg-orange-500 border-orange-500" : "bg-transparent border-muted-foreground/40"
                              )}>
                                {bulkIsRuntime && <Check className="h-3 w-3 text-white" strokeWidth={4} />}
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
                            <Button variant="ghost" onClick={() => setShowBulkEnv(false)} className="rounded-xl font-bold">Cancel</Button>
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
                              className="bg-cyan-500 hover:bg-cyan-600 text-white h-11 px-8 rounded-xl font-bold shadow-lg disabled:opacity-50"
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
                      <div className="p-12 text-center rounded-2xl bg-secondary/10 border border-dashed border-border/60">
                         <Shield className="h-8 w-8 text-muted-foreground/30 mx-auto mb-3" />
                         <p className="text-xs text-muted-foreground font-medium">No custom environment variables added yet.</p>
                      </div>
                    ) : (
                      <div className="grid grid-cols-1 gap-2">
                        {envVars.map((env, i) => (
                          <div key={i} className="flex items-center justify-between p-3.5 rounded-xl bg-secondary/30 border border-border/40 group hover:border-emerald-500/20 transition-all">
                            <div className="flex items-center gap-4 min-w-0">
                              <div className="h-8 w-8 rounded-lg bg-emerald-500/5 flex items-center justify-center shrink-0">
                                <Lock className="h-3.5 w-3.5 text-emerald-600" />
                              </div>
                              <div className="flex flex-col min-w-0">
                                <div className="flex items-center gap-2">
                                  <span className="text-sm font-mono font-bold tracking-tight truncate">{env.key}</span>
                                  <div className="flex gap-1.5">
                                    <div className={cn(
                                      "h-5 px-1.5 rounded-full flex items-center gap-1 text-[8px] font-bold uppercase tracking-widest transition-all",
                                      env.is_build_arg 
                                        ? "bg-orange-100 text-orange-600 border border-orange-300" 
                                        : "bg-gray-100 text-gray-400 border border-gray-200"
                                    )}>
                                      <FlaskConical className="h-2.5 w-2.5" />
                                      Bld
                                    </div>
                                    <div className={cn(
                                      "h-5 px-1.5 rounded-full flex items-center gap-1 text-[8px] font-bold uppercase tracking-widest transition-all",
                                      env.is_runtime 
                                        ? "bg-orange-100 text-orange-600 border border-orange-300" 
                                        : "bg-gray-100 text-gray-400 border border-gray-200"
                                    )}>
                                      <Globe className="h-2.5 w-2.5" />
                                      Run
                                    </div>
                                  </div>
                                </div>
                                <span className="text-xs font-mono text-muted-foreground truncate opacity-80 mt-0.5">
                                  {revealedEnvs.includes(i) ? env.value : "••••••••••••"}
                                </span>
                              </div>
                            </div>
                            <div className="flex items-center gap-1 opacity-0 group-hover:opacity-100 transition-opacity">
                              <button 
                                onClick={() => {
                                  if (revealedEnvs.includes(i)) {
                                    setRevealedEnvs(revealedEnvs.filter(idx => idx !== i));
                                  } else {
                                    setRevealedEnvs([...revealedEnvs, i]);
                                  }
                                }}
                                className="p-2 text-muted-foreground hover:bg-secondary rounded-lg transition-colors"
                              >
                                {revealedEnvs.includes(i) ? <EyeOff className="h-4 w-4" /> : <Eye className="h-4 w-4" />}
                              </button>
                              <button 
                                onClick={() => setEnvVars(envVars.filter((_, idx) => idx !== i))}
                                className="p-2 text-muted-foreground hover:text-destructive hover:bg-destructive/5 rounded-lg transition-colors"
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

                {/* DATABASE HELPER */}
                <Card className={cn(
                  "p-0 overflow-hidden rounded-3xl transition-all duration-500 border-2",
                  needsDb ? "border-blue-500/40 bg-blue-500/[0.02]" : "border-border/40"
                )}>
                  <div className="p-8 space-y-8">
                     <div className="flex items-center justify-between">
                      <div className="flex items-center gap-3">
                        <div className={cn(
                          "p-2 rounded-lg transition-colors",
                          needsDb ? "bg-blue-500 text-white" : "bg-blue-500/10 text-blue-500"
                        )}>
                          <Database className="h-5 w-5" />
                        </div>
                        <div>
                          <h3 className="text-xl font-bold">Database Assistant</h3>
                          <p className="text-xs text-muted-foreground font-medium mt-0.5">Quickly inject DATABASE_URL for build and runtime.</p>
                        </div>
                      </div>
                      <button 
                        type="button"
                        onClick={() => setNeedsDb(!needsDb)}
                        className={cn(
                          "h-6 w-11 rounded-full relative transition-colors duration-300 focus:outline-none",
                          needsDb ? "bg-blue-500" : "bg-secondary"
                        )}
                      >
                        <div className={cn(
                          "absolute top-1 left-1 bg-white h-4 w-4 rounded-full transition-transform duration-300 shadow-sm",
                          needsDb ? "translate-x-5" : "translate-x-0"
                        )} />
                      </button>
                    </div>

                    {needsDb && (
                      <div className="space-y-8 animate-in zoom-in-95 fade-in duration-300">
                        <div className="space-y-4">
                          <div className="flex items-center justify-between px-1">
                            <label className="text-[10px] font-bold text-muted-foreground uppercase tracking-widest">Connection String (DATABASE_URL)</label>
                          </div>
                          
                          <div className="relative group">
                            <div className="absolute inset-y-0 left-0 pl-4 flex items-center pointer-events-none">
                              <LinkIcon className="h-4 w-4 text-muted-foreground group-focus-within:text-blue-500" />
                            </div>
                            <Input 
                              placeholder="postgresql://user:pass@host:port/dbname" 
                              value={dbUrl}
                              onChange={(e) => {
                                let val = e.target.value;
                                if (val.length >= 2) {
                                  const first = val.charAt(0);
                                  const last = val.charAt(val.length - 1);
                                  if ((first === '"' && last === '"') || (first === "'" && last === "'")) {
                                    val = val.substring(1, val.length - 1);
                                  }
                                }
                                setDbUrl(val);
                              }}
                              className="h-14 pl-11 bg-background border-blue-500/20 focus:border-blue-500 rounded-2xl shadow-inner font-mono text-sm" 
                            />
                          </div>
                        </div>

                        <div className="grid grid-cols-1 sm:grid-cols-2 gap-4">
                          <div 
                            onClick={() => setDbInBuild(!dbInBuild)}
                            className={cn(
                              "p-4 rounded-2xl border transition-all cursor-pointer flex items-center gap-4 group",
                              dbInBuild ? "border-blue-500/40 bg-blue-500/[0.05]" : "border-border/60 hover:bg-secondary/40"
                            )}
                          >
                            <div className={cn(
                              "h-5 w-5 rounded-full border transition-all flex items-center justify-center",
                              dbInBuild ? "bg-blue-500 border-blue-500" : "bg-transparent border-muted-foreground/30"
                            )}>
                              {dbInBuild && <Check className="h-3 w-3 text-white" strokeWidth={4} />}
                            </div>
                            <div className="flex flex-col">
                              <span className="text-sm font-bold flex items-center gap-1.5 text-blue-600">
                                <FlaskConical className="h-3.5 w-3.5" /> Inject in Build
                              </span>
                              <span className="text-[10px] text-muted-foreground font-medium">Use as --build-arg for Next.js/Prisma</span>
                            </div>
                          </div>

                          <div 
                            onClick={() => setDbInRuntime(!dbInRuntime)}
                            className={cn(
                              "p-4 rounded-2xl border transition-all cursor-pointer flex items-center gap-4 group",
                              dbInRuntime ? "border-blue-500/40 bg-blue-500/[0.05]" : "border-border/60 hover:bg-secondary/40"
                            )}
                          >
                            <div className={cn(
                              "h-5 w-5 rounded-full border transition-all flex items-center justify-center",
                              dbInRuntime ? "bg-emerald-500 border-emerald-500" : "bg-transparent border-muted-foreground/30"
                            )}>
                              {dbInRuntime && <Check className="h-3 w-3 text-white" strokeWidth={4} />}
                            </div>
                            <div className="flex flex-col">
                              <span className="text-sm font-bold flex items-center gap-1.5 text-emerald-600">
                                <Globe className="h-3.5 w-3.5" /> Inject in Runtime
                              </span>
                              <span className="text-[10px] text-muted-foreground font-medium">Available to your app after deploy</span>
                            </div>
                          </div>
                        </div>

                        {!dbInBuild && !dbInRuntime && (
                          <div className="p-4 rounded-2xl bg-amber-500/5 border border-amber-500/20 flex gap-3">
                            <Info className="h-5 w-5 text-amber-500 shrink-0" />
                            <div className="space-y-1">
                              <p className="text-sm text-amber-900 font-bold">No scope selected</p>
                              <p className="text-xs text-amber-700 font-medium leading-relaxed">Please select at least one injection scope (Build or Runtime) for the Database URL to be active.</p>
                            </div>
                          </div>
                        )}

                        <div className="flex items-start gap-3 p-4 rounded-2xl bg-blue-500/5 border border-blue-500/10">
                          <Info className="h-4 w-4 text-blue-500 shrink-0 mt-0.5" />
                          <div className="space-y-1">
                            <p className="text-[10px] leading-relaxed text-blue-700/80 font-medium italic">
                              Prisma and Next.js Static Generation often require the database URL at build time. Selecting "Inject in Build" will pass this as a Docker build-arg.
                            </p>
                            <p className="text-[10px] leading-relaxed text-blue-700/80 font-bold">
                              Ensure your Dockerfile includes <code className="bg-blue-500/10 px-1 rounded border border-blue-500/20 not-italic">ARG DATABASE_URL</code> and optionally <code className="bg-blue-500/10 px-1 rounded border border-blue-500/20 not-italic">ENV DATABASE_URL=$DATABASE_URL</code>
                            </p>
                          </div>
                        </div>
                      </div>
                    )}
                  </div>
                </Card>
              </div>

              <div>
                <Card className="p-5 bg-secondary/20 border-border/30 rounded-2xl sticky top-24">
                  <h4 className="text-xs font-semibold text-muted-foreground mb-4">Summary</h4>
                  <div className="space-y-3">
                    <div className="flex items-center justify-between text-sm">
                      <span className="text-muted-foreground">Type</span>
                      <span className="font-medium">Application</span>
                    </div>
                    <div className="flex items-center justify-between text-sm">
                      <span className="text-muted-foreground">Source</span>
                      <span className="font-medium capitalize">{sourceType}</span>
                    </div>
                    <div className="pt-2 border-t border-border/30">
                       <p className="text-xs text-muted-foreground mb-1">Project</p>
                       <p className="font-semibold truncate">{name || "Untitled"}</p>
                    </div>
                    <Button 
                      onClick={handleSubmit} 
                      disabled={loading}
                      className="w-full h-10 mt-2 bg-zinc-900 hover:bg-zinc-800 text-white shadow-lg shadow-zinc-900/10"
                    >
                      {loading ? <Loader2 className="h-4 w-4 animate-spin mr-2" /> : <Sparkles className="h-4 w-4 mr-2" />}
                      Create Project
                    </Button>
                  </div>
                </Card>
              </div>
            </div>
          )}
        </div>
      </main>

      <Footer />
    </div>
  );
}

export default function NewProjectPage() {
  return (
    <Suspense fallback={
      <div className="min-h-screen flex items-center justify-center bg-black">
        <Loader2 className="h-8 w-8 animate-spin text-cyan-500" />
      </div>
    }>
      <NewProjectContent />
    </Suspense>
  );
}
