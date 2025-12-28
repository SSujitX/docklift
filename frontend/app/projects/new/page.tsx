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
  const [domain, setDomain] = useState("");
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

  useEffect(() => {
    fetchGitHubStatus();
    fetchExistingDatabases();
  }, []);

  // Debounced branch fetch for public repo
  useEffect(() => {
    if (sourceType === "public" && githubUrl) {
      setBranches([]); // Clear previous branches
      setGithubBranch(""); // Clear previous selection
      const timer = setTimeout(() => {
        const match = githubUrl.match(/github\.com\/([^\/]+\/[^\/]+)/);
        if (match) {
          fetchBranches(match[1], "public");
        }
      }, 600);
      return () => clearTimeout(timer);
    } else {
        setBranches([]);
    }
  }, [githubUrl, sourceType]);

  const fetchBranches = async (repoIdentifier: string, type: "public" | "private") => {
    setBranchesLoading(true);
    try {
      const res = await fetch(`${API_URL}/api/github/branches?repo=${repoIdentifier}&type=${type}`);
      if (!res.ok) throw new Error("Failed");
      const data = await res.json();
      setBranches(data);
      
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
      const res = await fetch(`${API_URL}/api/github/status`);
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
      const res = await fetch(`${API_URL}/api/github/repos?per_page=50`);
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
      const res = await fetch(`${API_URL}/api/projects`);
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
      formData.append("domain", domain);

      if (sourceType === "github" || sourceType === "public") {
        formData.append("github_url", githubUrl);
        formData.append("github_branch", githubBranch);
      } else if (files) {
        Array.from(files).forEach((file) => formData.append("files", file));
      }

      // 1. Create Project
      const response = await axios.post(`${API_URL}/api/projects`, formData, {
        headers: { "Content-Type": "multipart/form-data" },
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
          headers: { "Content-Type": "application/json" },
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
    if (!name) setName(githubUrl.split("/").pop()?.replace(".git", "") || "my-app");
    setStep(2);
  };

  return (
    <div className="min-h-screen flex flex-col bg-background selection:bg-cyan-500/30">
      <Header />

      <main className="flex-1 container max-w-5xl mx-auto px-4 py-12 md:py-20">
        <div className="mb-12">
          <button 
            onClick={() => step > 1 ? setStep(step - 1) : router.push("/")}
            className="flex items-center gap-2 text-muted-foreground hover:text-foreground transition-colors mb-6 group"
          >
            <ArrowLeft className="h-4 w-4 group-hover:-translate-x-1 transition-transform" />
            {step === 1 ? "Back to Dashboard" : "Previous Step"}
          </button>
          
          <div className="flex flex-col md:flex-row md:items-end justify-between gap-6">
            <div>
              <h1 className="text-4xl font-extrabold tracking-tight bg-gradient-to-r from-foreground to-foreground/70 bg-clip-text text-transparent">
                Create New Project
              </h1>
              <p className="text-muted-foreground text-lg mt-2 font-medium">
                {step === 1 && "Connect your code source"}
                {step === 2 && "Configure your application environment"}
              </p>
            </div>
            
            <div className="flex items-center gap-3">
              {[1, 2].map((s) => (
                <div 
                  key={s} 
                  className={cn(
                    "h-2 rounded-full transition-all duration-500",
                    step === s ? "w-12 bg-cyan-500 shadow-[0_0_10px_rgba(6,182,212,0.3)]" : 
                    step > s ? "w-6 bg-cyan-500/40" : "w-6 bg-secondary"
                  )}
                />
              ))}
            </div>
          </div>
        </div>

        <div className="grid grid-cols-1 gap-8 animate-in fade-in slide-in-from-bottom-4 duration-500">
          {/* STEP 1: Source Selection */}
          {step === 1 && (
            <div className="space-y-8">
              <div className="flex flex-wrap gap-4 p-1.5 bg-secondary/30 rounded-2xl border border-border/40 w-fit mx-auto">
               <button
                  onClick={() => setSourceType("public")}
                  className={cn(
                    "flex items-center gap-2 px-6 py-3 rounded-xl font-bold transition-all",
                    sourceType === "public" ? "bg-background text-foreground shadow-lg" : "text-muted-foreground hover:text-foreground"
                  )}
                >
                  <Globe className="h-5 w-5" />
                  Public Repository
                </button>
                <button
                  onClick={() => setSourceType("github")}
                  className={cn(
                    "flex items-center gap-2 px-6 py-3 rounded-xl font-bold transition-all",
                    sourceType === "github" ? "bg-background text-foreground shadow-lg" : "text-muted-foreground hover:text-foreground"
                  )}
                >
                  <GithubIcon className="h-5 w-5" />
                  Private GitHub Repository
                </button>
                <button
                  onClick={() => setSourceType("upload")}
                  className={cn(
                    "flex items-center gap-2 px-6 py-3 rounded-xl font-bold transition-all",
                    sourceType === "upload" ? "bg-background text-foreground shadow-lg" : "text-muted-foreground hover:text-foreground"
                  )}
                >
                  <Upload className="h-5 w-5" />
                  Direct Upload
                </button>
              </div>

              {sourceType === "public" && (
                <Card className="p-8 border-border/40 overflow-hidden rounded-3xl shadow-xl shadow-black/5 animate-in fade-in slide-in-from-bottom-2">
                  <div className="space-y-6 max-w-xl">
                    <div className="space-y-2">
                      <label className="text-lg font-semibold">Repository URL</label>
                      <Input 
                        placeholder="https://github.com/username/repo" 
                        value={githubUrl}
                        onChange={(e) => setGithubUrl(e.target.value)}
                        className="h-12 text-base bg-secondary/30"
                      />
                    </div>
                     <div className="space-y-2">
                      <label className="text-lg font-semibold">Branch</label>
                      <BranchSelector
                        branches={branches}
                        value={githubBranch}
                        onChange={setGithubBranch}
                        loading={branchesLoading}
                        disabled={!githubUrl}
                      />
                    </div>
                    <Button 
                      onClick={handlePublicSubmit} 
                      size="lg" 
                      className="w-full h-12 text-base gap-2 bg-gradient-to-r from-cyan-500 to-blue-600 hover:from-cyan-600 hover:to-blue-700 text-white shadow-lg shadow-cyan-500/25"
                    >
                      Continue <ArrowRight className="h-5 w-5" />
                    </Button>
                  </div>
                </Card>
              )}

              {sourceType === "github" && (
                <div className="grid grid-cols-1 gap-6 animate-in fade-in slide-in-from-bottom-2">
                  {!githubStatus?.connected ? (
                    <Card className="p-12 text-center border-dashed border-2 rounded-3xl flex flex-col items-center">
                      <div className="h-20 w-20 rounded-3xl bg-zinc-900 flex items-center justify-center mb-6 shadow-2xl">
                        <GithubIcon className="h-10 w-10 text-white" />
                      </div>
                      <h3 className="text-2xl font-bold mb-3">Connect your GitHub account</h3>
                      <p className="text-muted-foreground max-w-sm mb-8 font-medium">
                        Create a GitHub App to access your public and private repositories.
                      </p>
                      <Button onClick={() => setShowGitHubConnect(true)} size="lg" className="h-12 px-8 bg-zinc-900 hover:bg-zinc-800 text-white rounded-xl">
                        Create GitHub App
                      </Button>
                      <GitHubConnect 
                        open={showGitHubConnect} 
                        onOpenChange={setShowGitHubConnect}
                        onConnected={() => {
                          fetchGitHubStatus();
                          setSourceType("github"); // Stay on github tab
                        }}
                      />
                    </Card>
                  ) : (
                    <Card className="p-0 border-border/40 overflow-hidden rounded-3xl shadow-xl shadow-black/5">
                      <div className="bg-secondary/40 px-6 py-4 flex items-center justify-between border-b border-border/50">
                        <div className="flex items-center gap-3">
                          <div className="h-6 w-6 rounded-full bg-green-500/20 flex items-center justify-center">
                            <div className="h-2 w-2 rounded-full bg-green-500 animate-pulse" />
                          </div>
                          <span className="text-sm font-bold text-muted-foreground uppercase tracking-widest">Connected as @{githubStatus.username}</span>
                        </div>
                        <div className="relative w-72">
                          <Search className="absolute left-3 top-1/2 -translate-y-1/2 h-4 w-4 text-muted-foreground" />
                          <Input 
                            placeholder="Search repos..." 
                            value={searchQuery}
                            onChange={(e) => setSearchQuery(e.target.value)}
                            className="pl-9 h-9 text-sm bg-background/50 border-border/40 focus:border-cyan-500/50" 
                          />
                        </div>
                      </div>
                      <div className="max-h-[500px] overflow-y-auto divide-y divide-border/30 no-scrollbar">
                        {reposLoading ? (
                          <div className="p-20 flex flex-col items-center gap-4 text-muted-foreground">
                            <Loader2 className="h-8 w-8 animate-spin" />
                            <span className="font-medium">Fetching repositories...</span>
                          </div>
                        ) : filteredRepos.map(repo => (
                          <div 
                            key={repo.id}
                            onClick={() => handleSelectRepo(repo)}
                            className="group flex items-center justify-between p-5 hover:bg-cyan-500/[0.02] cursor-pointer transition-colors"
                          >
                            <div className="flex items-center gap-4">
                              <div className="h-12 w-12 rounded-xl bg-secondary/50 flex items-center justify-center group-hover:bg-cyan-500/10 transition-colors">
                                {repo.private ? <Lock className="h-5 w-5 text-muted-foreground group-hover:text-cyan-500" /> : <Globe className="h-5 w-5 text-muted-foreground group-hover:text-cyan-500" />}
                              </div>
                              <div>
                                <h4 className="font-bold group-hover:text-cyan-500 transition-colors">{repo.name}</h4>
                                <p className="text-xs text-muted-foreground mt-0.5 truncate max-w-[400px]">{repo.description || "No description provided"}</p>
                              </div>
                            </div>
                            <Button variant="ghost" className="opacity-0 group-hover:opacity-100 transition-opacity gap-2 font-bold text-cyan-500">
                              Import <ArrowRight className="h-4 w-4" />
                            </Button>
                          </div>
                        ))}
                      </div>
                    </Card>
                  )}
                </div>
              )}

              {sourceType === "upload" && (
                <div 
                  onDragEnter={handleDrag}
                  onDragLeave={handleDrag}
                  onDragOver={handleDrag}
                  onDrop={handleDrop}
                  className={cn(
                    "border-4 border-dashed rounded-[40px] p-20 text-center transition-all duration-300 flex flex-col items-center group cursor-pointer animate-in fade-in slide-in-from-bottom-2",
                    dragActive ? "border-cyan-500 bg-cyan-500/[0.05] scale-[1.01]" : "border-border/60 hover:border-cyan-500/40 hover:bg-secondary/20",
                    files && "border-cyan-500/50 bg-cyan-500/[0.02]"
                  )}
                  onClick={() => document.getElementById("file-upload-input")?.click()}
                >
                  <input id="file-upload-input" type="file" multiple className="hidden" onChange={(e) => { setFiles(e.target.files); setStep(2); }} />
                  <div className="h-24 w-24 rounded-full bg-secondary flex items-center justify-center mb-8 group-hover:scale-110 transition-transform shadow-inner">
                    <FolderUp className="h-10 w-10 text-muted-foreground group-hover:text-cyan-500 transition-colors" />
                  </div>
                  <h3 className="text-3xl font-extrabold mb-3">Drop your project</h3>
                  <p className="text-muted-foreground text-lg max-w-sm font-medium leading-relaxed">
                    Drag and drop your project folder or click to browse. <span className="text-cyan-500 font-bold">Please upload a ZIP file containing the root directory</span> of your project.
                  </p>

                  {files && (
                    <div className="mt-8 px-6 py-2 rounded-full bg-cyan-500/10 text-cyan-500 font-bold border border-cyan-500/20 flex items-center gap-2">
                       <Check className="h-4 w-4" strokeWidth={3} /> {files.length} Files Selected
                    </div>
                  )}
                </div>
              )}
            </div>
          )}
          {step === 2 && (
            <div className="grid grid-cols-1 lg:grid-cols-3 gap-8">
              <div className="lg:col-span-2 space-y-8">
                {/* General Config */}
                <Card className="p-8 space-y-6 rounded-3xl shadow-xl shadow-black/5">
                  <div className="flex items-center gap-3 mb-2">
                    <div className="p-2 rounded-lg bg-cyan-500/10 text-cyan-500">
                      <Sparkles className="h-5 w-5" />
                    </div>
                    <h3 className="text-xl font-bold font-display">General Configuration</h3>
                  </div>
                  
                  <div className="grid grid-cols-1 md:grid-cols-2 gap-6">
                    <div className="space-y-2">
                      <label className="text-[10px] font-bold text-muted-foreground uppercase tracking-widest ml-1 text-xs">Project Name</label>
                      <Input 
                        placeholder="my-cool-app" 
                        value={name}
                        onChange={(e) => setName(e.target.value)}
                        className="h-12 bg-secondary/30 border-border/40 focus:border-cyan-500/50 rounded-xl px-4 font-bold" 
                      />
                    </div>
                    <div className="space-y-2">
                      <label className="text-[10px] font-bold text-muted-foreground uppercase tracking-widest ml-1 text-xs">Custom Domain (Optional)</label>
                      <Input 
                        placeholder="app.example.com" 
                        value={domain}
                        onChange={(e) => setDomain(e.target.value)}
                        className="h-12 bg-secondary/30 border-border/40 focus:border-cyan-500/50 rounded-xl px-4 font-bold" 
                      />
                    </div>
                  </div>

                  {sourceType === "github" && (
                    <div className="p-4 rounded-2xl bg-secondary/20 border border-border/40 flex items-center justify-between">
                      <div className="flex items-center gap-3">
                        <GithubIcon className="h-5 w-5 text-muted-foreground" />
                        <div>
                          <p className="text-sm font-bold">{selectedRepo?.full_name}</p>
                           <div className="flex items-center gap-2 mt-1">
                            <span className="text-[10px] text-muted-foreground uppercase tracking-tighter">Branch:</span>
                            <BranchSelector
                                branches={branches}
                                value={githubBranch}
                                onChange={setGithubBranch}
                                loading={branchesLoading}
                                className="h-8 text-xs w-[140px]"
                            />
                           </div>
                        </div>
                      </div>
                      <Button variant="ghost" size="sm" onClick={() => setStep(1)} className="text-xs h-8 text-cyan-500">Change Repo</Button>
                    </div>
                  )}
                </Card>

                {/* ENVIRONMENT VARIABLES MANAGER */}
                <Card className="p-8 space-y-6 rounded-3xl shadow-xl shadow-black/5">
                  <div className="flex items-center justify-between mb-2">
                    <div className="flex items-center gap-3">
                      <div className="p-2 rounded-lg bg-emerald-500/10 text-emerald-500">
                        <Lock className="h-5 w-5" />
                      </div>
                      <h3 className="text-xl font-bold">Environment Variables</h3>
                    </div>

                    <Dialog open={showAddEnv} onOpenChange={setShowAddEnv}>
                      <DialogTrigger asChild>
                        <Button variant="outline" size="sm" className="h-9 px-4 rounded-xl font-bold border-emerald-500/20 text-emerald-600 hover:bg-emerald-500/5 hover:border-emerald-500/40">
                          <PlusIcon className="h-4 w-4 mr-2" /> Add Variable
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
                                  newEnvIsRuntime ? "bg-emerald-500/10 border-emerald-500/30 text-emerald-500" : "bg-transparent border-border/40 text-muted-foreground hover:border-border"
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
                                setEnvVars([...envVars, {key: newEnvKey, value: newEnvValue, is_build_arg: newEnvIsBuild, is_runtime: newEnvIsRuntime}]);
                                setNewEnvKey("");
                                setNewEnvValue("");
                                setShowAddEnv(false);
                              }}
                              className="bg-emerald-500 hover:bg-emerald-600 text-white h-11 px-8 rounded-xl font-bold shadow-lg disabled:opacity-50"
                            >
                              Add Variable
                            </Button>
                          </div>
                        </div>
                      </DialogContent>
                    </Dialog>
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
                                    {env.is_build_arg && <span className="text-[8px] font-bold uppercase tracking-widest bg-cyan-500/10 text-cyan-500 px-1.5 py-0.5 rounded border border-cyan-500/20">Build</span>}
                                    {env.is_runtime && <span className="text-[8px] font-bold uppercase tracking-widest bg-emerald-500/10 text-emerald-500 px-1.5 py-0.5 rounded border border-emerald-500/20">Runtime</span>}
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
                              onChange={(e) => setDbUrl(e.target.value)}
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
                          <p className="text-[10px] leading-relaxed text-blue-700/80 font-medium italic">
                            Prisma and Next.js Static Generation often require the database URL at build time. Selecting "Inject in Build" will pass this as a Docker build-arg.
                          </p>
                        </div>
                      </div>
                    )}
                  </div>
                </Card>
              </div>

              <div className="space-y-8">
                <Card className="p-6 bg-secondary/20 border-border/40 rounded-3xl sticky top-24">
                  <h4 className="text-sm font-bold uppercase tracking-widest text-muted-foreground mb-6">Deployment Summary</h4>
                  <div className="space-y-6">
                    <div className="flex items-center justify-between py-2 border-b border-border/30">
                      <span className="text-xs font-bold text-muted-foreground uppercase">Resource</span>
                      <span className="text-xs font-bold bg-muted px-2 py-0.5 rounded capitalize">Application</span>
                    </div>
                    <div className="flex items-center justify-between py-2 border-b border-border/30">
                      <span className="text-xs font-bold text-muted-foreground uppercase">Source</span>
                      <span className="text-xs font-bold bg-muted px-2 py-0.5 rounded capitalize">{sourceType}</span>
                    </div>
                    <div className="pt-2">
                       <p className="text-[10px] text-muted-foreground font-medium mb-1">PROJECT NAME</p>
                       <p className="font-bold text-base truncate">{name || "Untitled Project"}</p>
                    </div>
                    <div className="space-y-4 pt-4">
                      <Button 
                        onClick={handleSubmit} 
                        disabled={loading}
                        className="w-full h-14 bg-gradient-to-r from-cyan-500 to-blue-600 hover:from-cyan-600 hover:to-blue-700 text-white rounded-2xl shadow-xl shadow-cyan-500/20 font-bold group"
                      >
                        {loading ? <Loader2 className="h-5 w-5 animate-spin mr-2" /> : <Sparkles className="h-5 w-5 mr-2 group-hover:rotate-12 transition-transform" />}
                        Deploy Application
                      </Button>
                      <p className="text-[10px] text-center text-muted-foreground font-bold px-4">
                        By deploying, you agree to automate your workflow 🚀
                      </p>
                    </div>
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
