"use client";

import { useState } from "react";
import {
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
  DialogDescription,
} from "./ui/dialog";
import { Button } from "./ui/button";
import { Input } from "./ui/input";
import {
  Upload,
  Github,
  FolderUp,
  Loader2,
  Sparkles,
  Check,
} from "lucide-react";
import { API_URL } from "@/lib/utils";
import { cn } from "@/lib/utils";
import { toast } from "sonner";
import axios from "axios";

interface CreateProjectModalProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  onSuccess: () => void;
}

type SourceType = "upload" | "github";

export function CreateProjectModal({
  open,
  onOpenChange,
  onSuccess,
}: CreateProjectModalProps) {
  const [sourceType, setSourceType] = useState<SourceType>("upload");
  const [name, setName] = useState("");
  const [githubUrl, setGithubUrl] = useState("");
  const [githubBranch, setGithubBranch] = useState("main");
  const [domain, setDomain] = useState("");
  const [files, setFiles] = useState<FileList | null>(null);
  const [loading, setLoading] = useState(false);
  const [dragActive, setDragActive] = useState(false);

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault();
    setLoading(true);

    try {
      const formData = new FormData();
      formData.append("name", name);
      formData.append("source_type", sourceType);
      formData.append("domain", domain);

      if (sourceType === "github") {
        formData.append("github_url", githubUrl);
        formData.append("github_branch", githubBranch);
      } else if (files) {
        Array.from(files).forEach((file) => formData.append("files", file));
      }

      console.log("Submitting to:", `${API_URL}/api/projects`);
      // Debug: Log form data entries
      for (const pair of formData.entries()) {
        console.log(pair[0], pair[1]);
      }

      await axios.post(`${API_URL}/api/projects`, formData, {
        headers: {
          "Content-Type": "multipart/form-data",
        },
        timeout: 300000, // 5 minutes for large file uploads
        maxBodyLength: Infinity,
        maxContentLength: Infinity,
      });

      toast.success("Project created successfully");
      onSuccess();
      onOpenChange(false);
      resetForm();
    } catch (error: any) {
      console.error(error);
      const errorMessage =
        error.response?.data?.detail ||
        error.message ||
        "Failed to create project";
      toast.error(errorMessage);
    } finally {
      setLoading(false);
    }
  };

  const resetForm = () => {
    setName("");
    setGithubUrl("");
    setGithubBranch("main");
    setDomain("");
    setFiles(null);
    setSourceType("upload");
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
    if (e.dataTransfer.files?.length) setFiles(e.dataTransfer.files);
  };

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="sm:max-w-[500px]">
        <DialogHeader>
          <DialogTitle className="flex items-center gap-2">
            <div className="h-8 w-8 rounded-lg bg-gradient-to-br from-violet-500 to-purple-600 flex items-center justify-center">
              <Sparkles className="h-4 w-4 text-white" />
            </div>
            New Project
          </DialogTitle>
          <DialogDescription>
            Deploy your app in seconds. Just upload your project with a
            Dockerfile.
          </DialogDescription>
        </DialogHeader>

        <form onSubmit={handleSubmit} className="space-y-5 pt-2">
          <div className="space-y-2">
            <label className="text-sm font-medium">Project Name</label>
            <Input
              placeholder="my-awesome-app"
              value={name}
              onChange={(e) => setName(e.target.value)}
              required
            />
          </div>

          <div className="space-y-3">
            <label className="text-sm font-medium">Source</label>
            <div className="grid grid-cols-2 gap-3">
              <button
                type="button"
                onClick={() => setSourceType("upload")}
                className={cn(
                  "flex items-center gap-3 p-4 rounded-xl border-2 transition-all text-left",
                  sourceType === "upload"
                    ? "border-primary bg-primary/5 shadow-lg shadow-primary/10"
                    : "border-border hover:border-primary/50 hover:bg-secondary/50"
                )}
              >
                <div
                  className={cn(
                    "p-2.5 rounded-lg transition-colors",
                    sourceType === "upload" ? "bg-primary/10" : "bg-secondary"
                  )}
                >
                  <Upload
                    className={cn(
                      "h-5 w-5",
                      sourceType === "upload"
                        ? "text-primary"
                        : "text-muted-foreground"
                    )}
                  />
                </div>
                <div>
                  <div className="font-medium text-sm">Upload</div>
                  <div className="text-xs text-muted-foreground">
                    Your project files
                  </div>
                </div>
              </button>

              <button
                type="button"
                onClick={() => setSourceType("github")}
                className={cn(
                  "flex items-center gap-3 p-4 rounded-xl border-2 transition-all text-left",
                  sourceType === "github"
                    ? "border-primary bg-primary/5 shadow-lg shadow-primary/10"
                    : "border-border hover:border-primary/50 hover:bg-secondary/50"
                )}
              >
                <div
                  className={cn(
                    "p-2.5 rounded-lg transition-colors",
                    sourceType === "github" ? "bg-primary/10" : "bg-secondary"
                  )}
                >
                  <Github
                    className={cn(
                      "h-5 w-5",
                      sourceType === "github"
                        ? "text-primary"
                        : "text-muted-foreground"
                    )}
                  />
                </div>
                <div>
                  <div className="font-medium text-sm">GitHub</div>
                  <div className="text-xs text-muted-foreground">
                    Clone repository
                  </div>
                </div>
              </button>
            </div>
          </div>

          {sourceType === "upload" && (
            <div className="space-y-3">
              <div
                className={cn(
                  "relative border-2 border-dashed rounded-xl p-8 text-center transition-all cursor-pointer",
                  dragActive
                    ? "border-primary bg-primary/5 scale-[1.02]"
                    : "border-border hover:border-primary/50 hover:bg-secondary/30",
                  files && "border-primary/50 bg-primary/5"
                )}
                onDragEnter={handleDrag}
                onDragLeave={handleDrag}
                onDragOver={handleDrag}
                onDrop={handleDrop}
                onClick={() => document.getElementById("file-upload")?.click()}
              >
                <input
                  type="file"
                  id="file-upload"
                  className="hidden"
                  multiple
                  onChange={(e) => setFiles(e.target.files)}
                />
                {files ? (
                  <div className="space-y-2">
                    <div className="h-12 w-12 rounded-full bg-primary/10 flex items-center justify-center mx-auto">
                      <Check className="h-6 w-6 text-primary" />
                    </div>
                    <p className="font-medium text-primary">
                      {files.length} file{files.length > 1 ? "s" : ""} selected
                    </p>
                    <p className="text-sm text-muted-foreground">
                      {Array.from(files)
                        .map((f) => f.name)
                        .join(", ")}
                    </p>
                  </div>
                ) : (
                  <div className="space-y-2">
                    <div className="h-12 w-12 rounded-full bg-secondary flex items-center justify-center mx-auto">
                      <FolderUp className="h-6 w-6 text-muted-foreground" />
                    </div>
                    <p className="font-medium">
                      Drop your project ZIP or files here
                    </p>
                    <p className="text-sm text-muted-foreground">
                      or click to browse
                    </p>
                  </div>
                )}
              </div>

              <div className="bg-gradient-to-r from-violet-500/10 to-purple-500/10 rounded-xl p-4 border border-primary/20">
                <p className="text-sm">
                  <span className="font-semibold text-primary">Tip:</span> For
                  folders, please upload a{" "}
                  <code className="bg-primary/10 px-1.5 py-0.5 rounded text-primary font-mono text-xs">
                    .zip
                  </code>{" "}
                  file to preserve structure.
                </p>
                <p className="text-sm text-muted-foreground mt-1">
                  We auto-generate docker-compose.yml and nginx config for you
                  ✨
                </p>
              </div>
            </div>
          )}

          {sourceType === "github" && (
            <div className="space-y-4">
              <div className="space-y-2">
                <label className="text-sm font-medium">Repository URL</label>
                <Input
                  placeholder="https://github.com/username/repo"
                  value={githubUrl}
                  onChange={(e) => setGithubUrl(e.target.value)}
                />
              </div>
              <div className="space-y-2">
                <label className="text-sm font-medium">Branch</label>
                <Input
                  placeholder="main"
                  value={githubBranch}
                  onChange={(e) => setGithubBranch(e.target.value)}
                />
              </div>
              <div className="bg-gradient-to-r from-violet-500/10 to-purple-500/10 rounded-xl p-4 border border-primary/20">
                <p className="text-sm">
                  <span className="font-semibold text-primary">
                    Requirement:
                  </span>{" "}
                  Repository must have a{" "}
                  <code className="bg-primary/10 px-1.5 py-0.5 rounded text-primary font-mono text-xs">
                    Dockerfile
                  </code>{" "}
                  in root
                </p>
                <p className="text-sm text-muted-foreground mt-1">
                  We handle docker-compose.yml and nginx automatically ✨
                </p>
              </div>
            </div>
          )}

          <div className="space-y-2">
            <label className="text-sm font-medium">
              Domain{" "}
              <span className="text-muted-foreground font-normal">
                (optional)
              </span>
            </label>
            <Input
              placeholder="app.yourdomain.com"
              value={domain}
              onChange={(e) => setDomain(e.target.value)}
            />
          </div>

          <div className="flex gap-3 pt-2">
            <Button
              type="button"
              variant="outline"
              className="flex-1"
              onClick={() => onOpenChange(false)}
            >
              Cancel
            </Button>
            <Button
              type="submit"
              className="flex-1"
              disabled={loading || !name}
            >
              {loading && <Loader2 className="h-4 w-4 animate-spin" />}
              Create Project
            </Button>
          </div>
        </form>
      </DialogContent>
    </Dialog>
  );
}
