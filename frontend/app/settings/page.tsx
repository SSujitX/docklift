"use client";

import { useState, useEffect, Suspense } from "react";
import { useRouter, useSearchParams } from "next/navigation";
import { Header } from "@/components/Header";
import { Footer } from "@/components/Footer";
import { Card } from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import { Button } from "@/components/ui/button";
import { Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle, DialogTrigger } from "@/components/ui/dialog";
import { Server, Network, Container, Info, Loader2, Check, X, Sparkles, Globe, Plus, Trash2, ExternalLink } from "lucide-react";
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

interface DomainConfig {
  domain: string;
  port: number;
}

function SettingsContent() {
  const [githubStatus, setGithubStatus] = useState<GitHubStatus | null>(null);
  const [loading, setLoading] = useState(true);
  const [disconnecting, setDisconnecting] = useState(false);
  const [showGitHubConnect, setShowGitHubConnect] = useState(false);
  const [activeTab, setActiveTab] = useState('github');
  
  // Domain State
  const [domains, setDomains] = useState<DomainConfig[]>([]);
  const [loadingDomains, setLoadingDomains] = useState(false);
  const [showAddDomain, setShowAddDomain] = useState(false);
  const [newDomain, setNewDomain] = useState({ domain: '', port: '' });
  const [addingDomain, setAddingDomain] = useState(false);

  const searchParams = useSearchParams();

  useEffect(() => {
    fetchGitHubStatus();
    
    if (searchParams.get("github") === "connected") {
      toast.success("GitHub account connected successfully!");
    }

    if (activeTab === 'domain') {
      fetchDomains();
    }
  }, [searchParams, activeTab]);

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

  const fetchDomains = async () => {
    setLoadingDomains(true);
    try {
      const res = await fetch(`${API_URL}/api/domains`);
      if (res.ok) {
        const data = await res.json();
        setDomains(data);
      }
    } catch (error) {
      toast.error("Failed to load domains");
    } finally {
      setLoadingDomains(false);
    }
  };

  const handleAddDomain = async () => {
    if (!newDomain.domain || !newDomain.port) {
      toast.error("Please fill in all fields");
      return;
    }

    setAddingDomain(true);
    try {
      const res = await fetch(`${API_URL}/api/domains`, {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({
          domain: newDomain.domain,
          port: parseInt(newDomain.port)
        })
      });

      const data = await res.json();
      
      if (!res.ok) throw new Error(data.error || "Failed to add domain");
      
      toast.success(`Domain ${newDomain.domain} added successfully`);
      setShowAddDomain(false);
      setNewDomain({ domain: '', port: '' });
      fetchDomains();
    } catch (error: any) {
      toast.error(error.message);
    } finally {
      setAddingDomain(false);
    }
  };

  const handleDeleteDomain = async (domain: string) => {
    if (!confirm(`Are you sure you want to remove ${domain}?`)) return;

    try {
      const res = await fetch(`${API_URL}/api/domains/${domain}`, {
        method: "DELETE"
      });
      
      if (!res.ok) throw new Error("Failed to delete domain");
      
      toast.success("Domain removed");
      fetchDomains();
    } catch (error: any) {
      toast.error(error.message);
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

            {/* Domain Tab */}
            {activeTab === 'domain' && (
              <div className="space-y-6 animate-in fade-in slide-in-from-bottom-4 duration-500">
                <Card className="p-6 border-indigo-500/20">
                  <div className="flex items-center justify-between mb-6">
                    <div className="flex items-center gap-3">
                      <div className="p-3 rounded-xl bg-indigo-500/10">
                        <Globe className="h-6 w-6 text-indigo-500" />
                      </div>
                      <div>
                        <h2 className="text-xl font-semibold">Server Domain</h2>
                        <p className="text-sm text-muted-foreground">Access your Docklift panel via custom domain instead of IP</p>
                      </div>
                    </div>
                    
                    <Dialog open={showAddDomain} onOpenChange={setShowAddDomain}>
                      <DialogTrigger asChild>
                        <Button className="bg-indigo-600 hover:bg-indigo-700">
                          <Plus className="h-4 w-4 mr-2" />
                          Add Domain
                        </Button>
                      </DialogTrigger>
                      <DialogContent>
                        <DialogHeader>
                          <DialogTitle>Add Server Domain</DialogTitle>
                          <DialogDescription>
                            Access Docklift using a custom domain instead of IP address. Configure your DNS first!
                          </DialogDescription>
                        </DialogHeader>
                        
                        {/* DNS Instructions */}
                        <div className="p-3 rounded-lg bg-amber-500/10 border border-amber-500/20 text-sm space-y-2">
                          <p className="font-medium text-amber-600 dark:text-amber-400">📝 DNS Setup Required</p>
                          <p className="text-muted-foreground">Add an <strong>A Record</strong> in your domain's DNS settings:</p>
                          <div className="font-mono text-xs bg-secondary/50 p-2 rounded">
                            <div className="grid grid-cols-3 gap-2">
                              <span className="text-muted-foreground">Type</span>
                              <span className="text-muted-foreground">Host</span>
                              <span className="text-muted-foreground">Value</span>
                            </div>
                            <div className="grid grid-cols-3 gap-2 font-bold">
                              <span>A</span>
                              <span>@ or subdomain</span>
                              <span>Your Server IP</span>
                            </div>
                          </div>
                          <p className="text-xs text-muted-foreground">Use <code className="bg-secondary px-1 rounded">@</code> for root domain (example.com) or a subdomain like <code className="bg-secondary px-1 rounded">panel</code> for panel.example.com</p>
                        </div>
                        <div className="space-y-4 py-4">
                          <div className="space-y-2">
                            <label className="text-sm font-medium">Domain Name</label>
                            <Input 
                              placeholder="panel.yourdomain.com" 
                              value={newDomain.domain}
                              onChange={(e) => setNewDomain({ ...newDomain, domain: e.target.value })}
                            />
                          </div>
                          <div className="space-y-2">
                            <label className="text-sm font-medium">Docklift Port</label>
                            <Input 
                              type="number" 
                              placeholder="8080" 
                              value={newDomain.port}
                              onChange={(e) => setNewDomain({ ...newDomain, port: e.target.value })}
                            />
                            <p className="text-xs text-muted-foreground">Docklift runs on port 8080 by default. Change only if you modified it.</p>
                          </div>
                        </div>
                        <DialogFooter>
                          <Button variant="outline" onClick={() => setShowAddDomain(false)}>Cancel</Button>
                          <Button onClick={handleAddDomain} disabled={addingDomain} className="bg-indigo-600 hover:bg-indigo-700">
                            {addingDomain && <Loader2 className="mr-2 h-4 w-4 animate-spin" />}
                            Add Mapping
                          </Button>
                        </DialogFooter>
                      </DialogContent>
                    </Dialog>
                  </div>

                  {loadingDomains ? (
                    <div className="flex justify-center p-8">
                      <Loader2 className="h-8 w-8 animate-spin text-indigo-500" />
                    </div>
                  ) : domains.length > 0 ? (
                    <div className="rounded-xl border border-border/50 overflow-hidden">
                      <table className="w-full text-left text-sm">
                        <thead className="bg-secondary/50 font-medium text-muted-foreground">
                          <tr>
                            <th className="px-4 py-3">Domain</th>
                            <th className="px-4 py-3">Target Port</th>
                            <th className="px-4 py-3 text-right">Actions</th>
                          </tr>
                        </thead>
                        <tbody className="divide-y divide-border/50">
                          {domains.map((d) => (
                            <tr key={d.domain} className="hover:bg-secondary/20">
                              <td className="px-4 py-3 font-medium flex items-center gap-2">
                                <a href={`http://${d.domain}`} target="_blank" rel="noreferrer" className="flex items-center gap-2 hover:text-indigo-500 hover:underline">
                                  {d.domain}
                                  <ExternalLink className="h-3 w-3 opacity-50" />
                                </a>
                              </td>
                              <td className="px-4 py-3 font-mono text-muted-foreground">{d.port}</td>
                              <td className="px-4 py-3 text-right">
                                <Button 
                                  variant="ghost" 
                                  size="sm" 
                                  className="text-red-500 hover:text-red-600 hover:bg-red-500/10"
                                  onClick={() => handleDeleteDomain(d.domain)}
                                >
                                  <Trash2 className="h-4 w-4" />
                                </Button>
                              </td>
                            </tr>
                          ))}
                        </tbody>
                      </table>
                    </div>
                  ) : (
                    /* Empty State */
                    <div className="rounded-xl border border-border/50 overflow-hidden">
                      <div className="p-8 text-center bg-secondary/20">
                        <div className="inline-flex items-center justify-center p-4 rounded-full bg-secondary/50 mb-4">
                          <Globe className="h-8 w-8 text-muted-foreground/50" />
                        </div>
                        <h3 className="font-medium text-lg mb-1">No Server Domain Configured</h3>
                        <p className="text-muted-foreground text-sm max-w-sm mx-auto mb-4">
                          Access your Docklift panel using a custom domain like <strong>panel.yourdomain.com</strong> instead of IP:Port.
                        </p>
                      </div>
                    </div>
                  )}
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
