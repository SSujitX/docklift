"use client";

import { useState, useEffect } from "react";
import { Header } from "@/components/Header";
import { Footer } from "@/components/Footer";
import { Card } from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import { Button } from "@/components/ui/button";
import { Server, Network, Container, Info, Loader2, Check, X, Sparkles } from "lucide-react";
import { GithubIcon } from "@/components/icons/GithubIcon";
import { toast } from "sonner";
import { GitHubConnect } from "@/components/GitHubConnect";

const API_URL = process.env.NEXT_PUBLIC_API_URL || "http://localhost:8000";

interface GitHubStatus {
  connected: boolean;
  username?: string;
  avatar_url?: string;
  name?: string;
  error?: string;
}

export default function SettingsPage() {
  const [githubStatus, setGithubStatus] = useState<GitHubStatus | null>(null);
  const [loading, setLoading] = useState(true);
  const [disconnecting, setDisconnecting] = useState(false);
  const [showGitHubConnect, setShowGitHubConnect] = useState(false);

  useEffect(() => {
    fetchGitHubStatus();
  }, []);

  const fetchGitHubStatus = async () => {
    try {
      const controller = new AbortController();
      const timeoutId = setTimeout(() => controller.abort(), 5000); // 5s timeout
      
      const res = await fetch(`${API_URL}/api/github/status`, {
        signal: controller.signal
      });
      clearTimeout(timeoutId);
      const data = await res.json();
      setGithubStatus(data);
    } catch {
      setGithubStatus({ connected: false });
    } finally {
      setLoading(false);
    }
  };

  const handleConnectGitHub = () => {
    setShowGitHubConnect(true);
  };

  const handleDisconnectGitHub = async () => {
    setDisconnecting(true);
    try {
      await fetch(`${API_URL}/api/github/disconnect`, { method: "POST" });
      setGithubStatus({ connected: false });
      toast.success("GitHub disconnected");
    } catch {
      toast.error("Failed to disconnect");
    } finally {
      setDisconnecting(false);
    }
  };

  return (
    <div className="min-h-screen flex flex-col">
      <Header />

      <main className="flex-1 container max-w-3xl mx-auto px-4 md:px-6 py-8">
        <div className="mb-8">
          <h1 className="text-3xl font-bold tracking-tight">Settings</h1>
          <p className="text-muted-foreground mt-1">Server configuration and integrations</p>
        </div>

        <div className="space-y-6">
          {/* GitHub Integration */}
          {/* GitHub Integration */}
          <div className="relative group">
            <div className="absolute inset-0 bg-gradient-to-r from-cyan-500/20 to-blue-500/20 rounded-3xl blur-xl opacity-75" />
            <Card className="relative p-6 border-cyan-500/20 bg-card/50 backdrop-blur-sm">
              <div className="flex flex-col sm:flex-row items-center gap-6">
                <div className="relative shrink-0">
                  <div className="absolute inset-0 bg-cyan-500/20 blur-xl rounded-full" />
                  <div className="relative p-4 rounded-2xl bg-gradient-to-br from-zinc-900 to-black border border-zinc-800 shadow-xl">
                    <GithubIcon className="h-8 w-8 text-white" />
                  </div>
                </div>
                
                <div className="flex-1 text-center sm:text-left space-y-1">
                  <h2 className="text-xl font-bold">GitHub Integration</h2>
                  <p className="text-muted-foreground">
                    Connect to automatically deploy public and private repositories.
                  </p>
                </div>

                <div className="shrink-0 pt-4 sm:pt-0">
                  {loading ? (
                    <div className="flex items-center gap-2 text-muted-foreground px-4 py-2 bg-secondary/50 rounded-lg">
                      <Loader2 className="h-4 w-4 animate-spin" />
                      <span>Checking...</span>
                    </div>
                  ) : githubStatus?.connected ? (
                    <div className="flex flex-col items-end gap-3">
                      <div className="flex items-center gap-3 bg-green-500/10 border border-green-500/20 px-4 py-2 rounded-lg">
                        {githubStatus.avatar_url && (
                          <img 
                            src={githubStatus.avatar_url} 
                            alt={githubStatus.username} 
                            className="h-6 w-6 rounded-full"
                          />
                        )}
                        <div className="flex items-center gap-2">
                          <Check className="h-4 w-4 text-green-500" />
                          <span className="font-medium text-green-500">Connected</span>
                          <span className="text-muted-foreground text-sm">@{githubStatus.username}</span>
                        </div>
                      </div>
                      <Button 
                        variant="ghost" 
                        size="sm"
                        onClick={handleDisconnectGitHub}
                        disabled={disconnecting}
                        className="text-muted-foreground hover:text-destructive"
                      >
                        {disconnecting ? (
                          <Loader2 className="h-3 w-3 animate-spin mr-2" />
                        ) : (
                          <X className="h-3 w-3 mr-2" />
                        )}
                        Disconnect
                      </Button>
                    </div>
                  ) : (
                    <Button 
                      onClick={handleConnectGitHub}
                      size="lg"
                      className="gap-2 bg-gradient-to-r from-cyan-500 to-blue-600 hover:from-cyan-600 hover:to-blue-700 text-white shadow-lg shadow-cyan-500/25"
                    >
                      <Sparkles className="h-4 w-4" />
                      Connect GitHub
                    </Button>
                  )}
                  <GitHubConnect 
                    open={showGitHubConnect} 
                    onOpenChange={setShowGitHubConnect}
                    onConnected={fetchGitHubStatus}
                  />
                </div>
              </div>
            </Card>
          </div>

          {/* Server Settings */}
          <Card className="p-6">
            <div className="flex items-center gap-3 mb-5">
              <div className="p-2.5 rounded-xl bg-primary/10">
                <Server className="h-5 w-5 text-primary" />
              </div>
              <div>
                <h2 className="font-semibold">Server</h2>
                <p className="text-sm text-muted-foreground">API and deployment settings</p>
              </div>
            </div>
            <div className="grid gap-4">
              <div className="space-y-2">
                <label className="text-sm font-medium">API URL</label>
                <Input value={API_URL} disabled className="bg-secondary/50" />
              </div>
              <div className="space-y-2">
                <label className="text-sm font-medium">Deployments Path</label>
                <Input value="/deployments" disabled className="bg-secondary/50" />
              </div>
            </div>
          </Card>

          {/* Port Configuration */}
          <Card className="p-6">
            <div className="flex items-center gap-3 mb-5">
              <div className="p-2.5 rounded-xl bg-emerald-500/10">
                <Network className="h-5 w-5 text-emerald-500" />
              </div>
              <div>
                <h2 className="font-semibold">Port Configuration</h2>
                <p className="text-sm text-muted-foreground">Allocatable port range</p>
              </div>
            </div>
            <div className="grid grid-cols-2 gap-4">
              <div className="space-y-2">
                <label className="text-sm font-medium">Start Port</label>
                <Input type="number" value="3001" disabled className="bg-secondary/50" />
              </div>
              <div className="space-y-2">
                <label className="text-sm font-medium">End Port</label>
                <Input type="number" value="3100" disabled className="bg-secondary/50" />
              </div>
            </div>
          </Card>

          {/* Docker Settings */}
          <Card className="p-6">
            <div className="flex items-center gap-3 mb-5">
              <div className="p-2.5 rounded-xl bg-blue-500/10">
                <Container className="h-5 w-5 text-blue-500" />
              </div>
              <div>
                <h2 className="font-semibold">Docker</h2>
                <p className="text-sm text-muted-foreground">Container network settings</p>
              </div>
            </div>
            <div className="space-y-2">
              <label className="text-sm font-medium">Network Name</label>
              <Input value="docklift_network" disabled className="bg-secondary/50" />
            </div>
          </Card>

          {/* About */}
          <Card className="p-6 bg-gradient-to-r from-cyan-500/10 to-blue-500/10 border-primary/20">
            <div className="flex items-center gap-3">
              <div className="p-2.5 rounded-xl bg-primary/10">
                <Info className="h-5 w-5 text-primary" />
              </div>
              <div>
                <h2 className="font-semibold">About Docklift</h2>
                <p className="text-sm text-muted-foreground">Version 1.0.0 · Self-hosted Docker deployment platform</p>
              </div>
            </div>
          </Card>
        </div>
      </main>

      <Footer />
    </div>
  );
}
