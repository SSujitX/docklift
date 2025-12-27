"use client";

import { useState, useEffect, Suspense } from "react";
import { useRouter, useSearchParams } from "next/navigation";
import { Header } from "@/components/Header";
import { Footer } from "@/components/Footer";
import { Card } from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import { Button } from "@/components/ui/button";
import { Server, Network, Container, Info, Loader2, Check, X, Sparkles, Globe, Plus } from "lucide-react";
import { GithubIcon } from "@/components/icons/GithubIcon";
import { toast } from "sonner";
import { API_URL } from "@/lib/utils";
import { GitHubConnect } from "@/components/GitHubConnect";

interface GitHubStatus {
  connected: boolean;
  username?: string;
  avatar_url?: string;
  name?: string;
  error?: string;
}

function SettingsContent() {
  const [githubStatus, setGithubStatus] = useState<GitHubStatus | null>(null);
  const [loading, setLoading] = useState(true);
  const [disconnecting, setDisconnecting] = useState(false);
  const [showGitHubConnect, setShowGitHubConnect] = useState(false);
  const [activeTab, setActiveTab] = useState('github');
  const searchParams = useSearchParams();

  useEffect(() => {
    fetchGitHubStatus();
    
    if (searchParams.get("github") === "connected") {
      toast.success("GitHub account connected successfully!");
    }
  }, [searchParams]);

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
    <div className="min-h-screen flex flex-col bg-background">
      <Header />

      <main className="flex-1 container max-w-6xl mx-auto px-4 md:px-6 py-8">
        <div className="mb-8">
          <h1 className="text-3xl font-bold tracking-tight">Settings</h1>
          <p className="text-muted-foreground mt-1">Manage your server, integrations, and domains</p>
        </div>

        <div className="flex flex-col md:flex-row gap-8">
          {/* Sidebar Navigation */}
          <aside className="w-full md:w-64 shrink-0 space-y-2">
            <nav className="flex md:flex-col overflow-x-auto md:overflow-visible gap-2 p-1">
              {[
                { id: 'github', label: 'GitHub', icon: GithubIcon },
                { id: 'server', label: 'Server', icon: Server },
                { id: 'port', label: 'Port', icon: Network },
                { id: 'docker', label: 'Docker', icon: Container },
                { id: 'domain', label: 'Domains', icon: Globe },
              ].map((item) => {
                const Icon = item.icon;
                const isActive = activeTab === item.id;
                return (
                  <button
                    key={item.id}
                    onClick={() => setActiveTab(item.id)}
                    className={`flex items-center gap-3 px-4 py-3 rounded-xl text-sm font-medium transition-all whitespace-nowrap
                      ${isActive 
                        ? "bg-primary text-primary-foreground shadow-lg shadow-primary/20 scale-[1.02]" 
                        : "hover:bg-muted text-muted-foreground hover:text-foreground"
                      }`}
                  >
                    <Icon className={`h-4 w-4 ${isActive ? "text-primary-foreground" : ""}`} />
                    {item.label}
                  </button>
                );
              })}
            </nav>
          </aside>

          {/* Main Content Area */}
          <div className="flex-1 min-w-0">
            {/* GitHub Tab */}
            {activeTab === 'github' && (
              <div className="space-y-6 animate-in fade-in slide-in-from-bottom-4 duration-500">
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
                          <div className="flex flex-col items-center sm:items-end gap-3 w-full sm:w-auto">
                            <div className="flex items-center gap-4 bg-green-500/10 border border-green-500/20 px-5 py-3 rounded-2xl w-full sm:w-auto">
                              {githubStatus.avatar_url ? (
                                <img 
                                  src={githubStatus.avatar_url} 
                                  alt={githubStatus.username} 
                                  className="h-10 w-10 rounded-xl border border-green-500/20"
                                />
                              ) : (
                                <div className="h-10 w-10 rounded-xl bg-green-500/20 flex items-center justify-center font-bold text-green-500">
                                  {githubStatus.username?.charAt(0).toUpperCase()}
                                </div>
                              )}
                              <div className="flex flex-col">
                                <div className="flex items-center gap-2">
                                  <span className="font-bold text-green-500">Connected</span>
                                  <Check className="h-4 w-4 text-green-500" />
                                </div>
                                <span className="text-sm text-muted-foreground font-medium truncate max-w-[150px]">@{githubStatus.username}</span>
                              </div>
                            </div>
                            
                            <Button 
                              variant="ghost" 
                              size="sm"
                              onClick={handleDisconnectGitHub}
                              disabled={disconnecting}
                              className="h-8 text-xs text-muted-foreground hover:text-destructive hover:bg-destructive/10 rounded-lg group/btn"
                            >
                              {disconnecting ? (
                                <Loader2 className="h-3.5 w-3.5 animate-spin mr-2" />
                              ) : (
                                <X className="h-3.5 w-3.5 mr-2 group-hover/btn:rotate-90 transition-transform" />
                              )}
                              Disconnect GitHub Account
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
              </div>
            )}

            {/* Server Tab */}
            {activeTab === 'server' && (
              <div className="space-y-6 animate-in fade-in slide-in-from-bottom-4 duration-500">
                <Card className="p-6">
                  <div className="flex items-center gap-3 mb-6">
                    <div className="p-3 rounded-xl bg-primary/10">
                      <Server className="h-6 w-6 text-primary" />
                    </div>
                    <div>
                      <h2 className="text-xl font-semibold">Server Configuration</h2>
                      <p className="text-sm text-muted-foreground">Core system and API settings</p>
                    </div>
                  </div>
                  <div className="grid gap-6">
                    <div className="space-y-2">
                      <label className="text-sm font-medium">API Endpoint</label>
                      <div className="flex gap-2">
                        <Input 
                          value={API_URL || (typeof window !== 'undefined' ? window.location.origin : 'Detecting...')} 
                          disabled 
                          className="bg-secondary/50 font-mono" 
                        />
                      </div>
                      <p className="text-xs text-muted-foreground">The base URL for backend communication.</p>
                    </div>
                    <div className="space-y-2">
                      <label className="text-sm font-medium">Deployments Directory</label>
                      <Input value="/deployments" disabled className="bg-secondary/50 font-mono" />
                      <p className="text-xs text-muted-foreground">Absolute path where project files are stored on the host.</p>
                    </div>
                  </div>
                </Card>
              </div>
            )}

            {/* Port Tab */}
            {activeTab === 'port' && (
              <div className="space-y-6 animate-in fade-in slide-in-from-bottom-4 duration-500">
                <Card className="p-6">
                  <div className="flex items-center gap-3 mb-6">
                    <div className="p-3 rounded-xl bg-emerald-500/10">
                      <Network className="h-6 w-6 text-emerald-500" />
                    </div>
                    <div>
                      <h2 className="text-xl font-semibold">Port Allocation</h2>
                      <p className="text-sm text-muted-foreground">Manage the range of ports available for deployments</p>
                    </div>
                  </div>
                  <div className="grid grid-cols-1 md:grid-cols-2 gap-6">
                    <div className="space-y-2">
                      <label className="text-sm font-medium">Start Port</label>
                      <Input type="number" value="3001" disabled className="bg-secondary/50 font-mono" />
                    </div>
                    <div className="space-y-2">
                      <label className="text-sm font-medium">End Port</label>
                      <Input type="number" value="3100" disabled className="bg-secondary/50 font-mono" />
                    </div>
                  </div>
                  <div className="mt-4 p-4 bg-secondary/30 rounded-lg text-sm text-muted-foreground">
                    <p>Docklift automatically assigns the next available port from this pool when creating new deployments.</p>
                  </div>
                </Card>
              </div>
            )}

            {/* Docker Tab */}
            {activeTab === 'docker' && (
              <div className="space-y-6 animate-in fade-in slide-in-from-bottom-4 duration-500">
                <Card className="p-6">
                  <div className="flex items-center gap-3 mb-6">
                    <div className="p-3 rounded-xl bg-blue-500/10">
                      <Container className="h-6 w-6 text-blue-500" />
                    </div>
                    <div>
                      <h2 className="text-xl font-semibold">Docker Network</h2>
                      <p className="text-sm text-muted-foreground">Container orchestration settings</p>
                    </div>
                  </div>
                  <div className="space-y-4">
                    <div className="space-y-2">
                      <label className="text-sm font-medium">Network Bridge</label>
                      <Input value="docklift_network" disabled className="bg-secondary/50 font-mono" />
                      <p className="text-xs text-muted-foreground">
                        All application containers are attached to this bridge network to allow internal communication.
                      </p>
                    </div>
                  </div>
                </Card>
              </div>
            )}

            {/* Domain Tab (New) */}
            {activeTab === 'domain' && (
              <div className="space-y-6 animate-in fade-in slide-in-from-bottom-4 duration-500">
                <Card className="p-6 border-indigo-500/20">
                  <div className="flex items-center justify-between mb-6">
                    <div className="flex items-center gap-3">
                      <div className="p-3 rounded-xl bg-indigo-500/10">
                        <Globe className="h-6 w-6 text-indigo-500" />
                      </div>
                      <div>
                        <h2 className="text-xl font-semibold">Domain Management</h2>
                        <p className="text-sm text-muted-foreground">Map custom domains to your services</p>
                      </div>
                    </div>
                    <Button className="bg-indigo-600 hover:bg-indigo-700">
                      <Plus className="h-4 w-4 mr-2" />
                      Add Domain
                    </Button>
                  </div>

                  {/* Domain List Placeholder */}
                  <div className="rounded-xl border border-border/50 overflow-hidden">
                    <div className="p-8 text-center bg-secondary/20">
                      <div className="inline-flex items-center justify-center p-4 rounded-full bg-secondary/50 mb-4">
                        <Globe className="h-8 w-8 text-muted-foreground/50" />
                      </div>
                      <h3 className="font-medium text-lg mb-1">No Custom Domains</h3>
                      <p className="text-muted-foreground text-sm max-w-sm mx-auto mb-4">
                        You can map custom domains (e.g., app.example.com) to your specific container ports.
                      </p>
                      <Button variant="outline" className="gap-2">
                        Learn how to configure DNS
                      </Button>
                    </div>
                  </div>
                </Card>
              </div>
            )}
            
          </div>
        </div>
      </main>

      <Footer />
    </div>
  );
}

export default function SettingsPage() {
  return (
    <Suspense fallback={
      <div className="min-h-screen flex items-center justify-center bg-black">
        <Loader2 className="h-8 w-8 animate-spin text-cyan-500" />
      </div>
    }>
      <SettingsContent />
    </Suspense>
  );
}
