// Documentation page - comprehensive guide to Docklift features and API reference
"use client";

import { useState, useEffect } from "react";
import { Header } from "@/components/Header";
import { Footer } from "@/components/Footer";
import { 
  Container, Rocket, FileCode, Settings2, Terminal, FolderTree, 
  History, Plug, Globe, Github, Server, Key, Database, Activity, Cpu, 
  HardDrive, Network, Shield, RefreshCw, Trash2, Power, Code, Download, Info, Wrench, Copy, Check
} from "lucide-react";
import { Button } from "@/components/ui/button";
import { copyToClipboard, cn } from "@/lib/utils";

const sections = [
  { id: "introduction", title: "Introduction", icon: Info },
  { id: "installation", title: "Installation", icon: Download },
  { id: "github", title: "GitHub Integration", icon: Github },
  { id: "autodeploy", title: "Auto-Deploy", icon: RefreshCw },
  { id: "deployment", title: "Deployment", icon: Container },
  { id: "dockerfile", title: "Dockerfile", icon: FileCode },
  { id: "domains", title: "Custom Domains", icon: Globe },
  { id: "environment", title: "Environment Variables", icon: Key },
  { id: "system", title: "System Overview", icon: Cpu },
  { id: "terminal", title: "Web Terminal", icon: Terminal },
  { id: "api", title: "API Reference", icon: Code },
  { id: "files", title: "File Management", icon: FolderTree },
  { id: "ports", title: "Port Management", icon: Plug },
  { id: "commands", title: "Useful Commands", icon: Wrench },
  { id: "troubleshooting", title: "Troubleshooting", icon: Shield },
];

const CopyButton = ({ text, className }: { text: string, className?: string }) => {
  const [copied, setCopied] = useState(false);
  const handleCopy = () => {
    copyToClipboard(text);
    setCopied(true);
    setTimeout(() => setCopied(false), 2000);
  };
  return (
    <button
      onClick={handleCopy}
      className={cn(
        "flex items-center gap-2 px-3 py-1.5 rounded-lg bg-white/5 hover:bg-white/10 text-zinc-400 hover:text-white transition-all border border-white/5 shrink-0 active:scale-95",
        className
      )}
    >
      {copied ? <Check className="h-3.5 w-3.5 text-emerald-500" /> : <Copy className="h-3.5 w-3.5" />}
      <span className="text-[10px] font-bold uppercase tracking-tight hidden sm:inline">
        {copied ? "Copied" : "Copy"}
      </span>
    </button>
  );
};

const CommandBlock = ({ command, label, color = "cyan" }: { command: string, label?: string, color?: string }) => {
  return (
    <div className="group mb-4 last:mb-0">
      {label && <p className="text-xs font-bold text-muted-foreground/80 uppercase tracking-wider mb-2 ml-1"># {label}</p>}
      <div className="relative group/btn">
        <div className="flex items-center gap-3 bg-zinc-900 border border-white/5 rounded-xl px-4 py-2.5 group-hover/btn:border-white/10 transition-all shadow-inner">
          <Terminal className="h-4 w-4 text-zinc-600 shrink-0" />
          <div className="overflow-x-auto no-scrollbar flex-1 font-mono text-sm py-0.5">
            <code className={cn(
              "whitespace-nowrap",
              color === "cyan" ? "text-cyan-400" : 
              color === "red" ? "text-red-400" : 
              color === "emerald" ? "text-emerald-400" : 
              color === "amber" ? "text-amber-400" : 
              color === "violet" ? "text-violet-400" : "text-blue-400"
            )}>
              {command}
            </code>
          </div>
          <CopyButton text={command} />
        </div>
      </div>
    </div>
  );
};

const TerminalWindow = ({ title, items, color = "cyan" }: { title: string, items: { cmd?: string, comment?: string }[], color?: string }) => {
  return (
    <div className="bg-zinc-950 rounded-2xl border border-white/5 overflow-hidden mb-8 shadow-2xl">
      <div className="flex items-center justify-between px-5 py-3.5 bg-white/5 border-b border-white/5">
        <div className="flex items-center gap-4">
          <div className="flex gap-2">
            <div className="w-3 h-3 rounded-full bg-red-500/30 border border-red-500/20" />
            <div className="w-3 h-3 rounded-full bg-amber-500/30 border border-amber-500/20" />
            <div className="w-3 h-3 rounded-full bg-emerald-500/30 border border-emerald-500/20" />
          </div>
          <div className="h-4 w-[1px] bg-white/10 mx-1" />
          <span className="text-[11px] font-black text-zinc-500 uppercase tracking-[0.2em]">{title}</span>
        </div>
      </div>
      <div className="p-6 space-y-4">
        {items.map((item, i) => (
          <div key={i} className="group/item">
            {item.comment && (
              <p className="text-xs text-zinc-500 font-medium italic mb-2 ml-1"># {item.comment}</p>
            )}
            {item.cmd && (
              <div className="flex items-center justify-between gap-4 bg-white/5 rounded-xl px-4 py-2.5 border border-white/5 group-hover/item:border-white/10 transition-all">
                <div className="flex items-center gap-3 min-w-0 flex-1">
                  <span className="text-zinc-600 text-xs font-mono select-none">$</span>
                  <div className="overflow-x-auto no-scrollbar font-mono text-sm py-0.5 w-full">
                    <code className={cn(
                      "whitespace-nowrap",
                      color === "cyan" ? "text-cyan-400" : 
                      color === "emerald" ? "text-emerald-400" : 
                      color === "amber" ? "text-amber-400" : "text-zinc-300"
                    )}>
                      {item.cmd}
                    </code>
                  </div>
                </div>
                <CopyButton text={item.cmd} />
              </div>
            )}
          </div>
        ))}
      </div>
    </div>
  );
};

const StaticCodeBlock = ({ code, icon: Icon = FileCode, title, color = "cyan" }: { code: string, icon?: any, title?: string, color?: string }) => {
  return (
    <div className="relative bg-zinc-950 rounded-2xl border border-white/5 overflow-hidden mb-6 group">
      <div className="flex items-center justify-between px-5 py-3.5 bg-white/5 border-b border-white/5">
        <div className="flex items-center gap-3">
          <Icon className={cn("h-4 w-4", color === "cyan" ? "text-cyan-500" : "text-zinc-500")} />
          <span className="text-[11px] font-bold text-zinc-500 uppercase tracking-wider">{title || "Code Example"}</span>
        </div>
        <CopyButton text={code} className="bg-transparent border-transparent hover:bg-white/5" />
      </div>
      <div className="p-6 overflow-x-auto no-scrollbar text-sm text-zinc-300 font-mono leading-relaxed">
        <pre>{code}</pre>
      </div>
    </div>
  );
};


export default function DocsPage() {
  const [activeSection, setActiveSection] = useState("introduction");

  useEffect(() => {
    const handleScroll = () => {
      const offset = 100;
      for (const section of sections) {
        const element = document.getElementById(section.id);
        if (element) {
          const rect = element.getBoundingClientRect();
          if (rect.top <= offset && rect.bottom >= offset) {
            setActiveSection(section.id);
            break;
          }
        }
      }
    };
    
    window.addEventListener("scroll", handleScroll);
    return () => window.removeEventListener("scroll", handleScroll);
  }, []);

  const scrollToSection = (id: string) => {
    const element = document.getElementById(id);
    if (element) {
      element.scrollIntoView({ behavior: "smooth", block: "start" });
    }
  };

  return (
    <div className="min-h-screen flex flex-col">
      <Header />
      
      <div className="flex-1 container max-w-7xl mx-auto px-4 md:px-6 py-8">
        <div className="flex flex-col lg:flex-row gap-8">
          {/* Sidebar TOC */}
          <aside className="hidden lg:block w-64 shrink-0">
            <div className="sticky top-20">
              <h3 className="font-semibold mb-4 text-sm uppercase tracking-wide text-muted-foreground">
                Documentation
              </h3>
              <nav className="space-y-1">
                {sections.map((section) => (
                  <button
                    key={section.id}
                    onClick={() => scrollToSection(section.id)}
                    className={`w-full flex items-center gap-2 px-3 py-2 text-sm rounded-lg transition-colors ${
                      activeSection === section.id
                        ? "bg-primary/10 text-primary font-medium"
                        : "text-muted-foreground hover:text-foreground hover:bg-secondary/50"
                    }`}
                  >
                    <section.icon className="h-4 w-4" />
                    {section.title}
                  </button>
                ))}
              </nav>
            </div>
          </aside>
          {/* Mobile Section Selector */}
          <div className="lg:hidden sticky top-[64px] z-30 bg-background/95 backdrop-blur-xl py-3 -mx-4 px-4 border-b border-border/30 overflow-x-auto no-scrollbar">
            <div className="flex items-center gap-1.5 w-max">
              {sections.map((section) => (
                <button
                  key={section.id}
                  onClick={() => scrollToSection(section.id)}
                  className={cn(
                    "flex items-center gap-2 px-4 py-2 rounded-full text-xs font-bold transition-all whitespace-nowrap",
                    activeSection === section.id
                      ? "bg-cyan-500/10 text-cyan-600 dark:text-cyan-400 border border-cyan-500/20 shadow-sm"
                      : "bg-secondary/40 text-muted-foreground border border-transparent hover:bg-secondary/60"
                  )}
                >
                  <section.icon className="h-3.5 w-3.5" />
                  {section.title}
                </button>
              ))}
            </div>
          </div>

          {/* Main Content */}
          <main className="flex-1 min-w-0">
            <div className="mb-10 sm:mb-12">
              <h1 className="text-3xl sm:text-4xl md:text-5xl font-extrabold tracking-tight bg-gradient-to-r from-foreground to-foreground/70 bg-clip-text text-transparent mb-3 sm:mb-4">
                Documentation
              </h1>
              <p className="text-base sm:text-lg text-muted-foreground max-w-2xl leading-relaxed">
                Complete guide to deploying and managing Docker containers with Docklift.
              </p>
            </div>

            <div className="prose prose-neutral dark:prose-invert max-w-none">
              {/* Introduction */}
              <section id="introduction" className="scroll-mt-20 mb-12">
                <h2 className="flex items-center gap-2 text-2xl font-bold mb-4">
                  <Info className="h-6 w-6 text-cyan-500" />
                  Introduction
                </h2>
                <p className="text-muted-foreground mb-4">
                  <strong>Docklift</strong> is a self-hosted Docker deployment platform that makes it easy to deploy, manage, and monitor your containerized applications.
                </p>
                
                <div className="bg-gradient-to-r from-cyan-500/10 to-blue-500/10 rounded-xl p-6 border border-cyan-500/20 mb-6">
                  <h4 className="font-semibold mb-3 text-lg">What is Docklift?</h4>
                  <p className="text-muted-foreground mb-4">
                    Docklift is like Vercel or Railway, but self-hosted on your own VPS. Deploy apps from GitHub or file uploads with a single click, manage custom domains, monitor system resources, and access your server terminal - all from a beautiful web interface.
                  </p>
                </div>

                <div className="grid gap-4 md:grid-cols-2 lg:grid-cols-3 mb-6">
                  <div className="bg-secondary/50 rounded-xl p-4">
                    <h4 className="font-semibold text-cyan-500 mb-2">🚀 One-Click Deploy</h4>
                    <p className="text-sm text-muted-foreground">Deploy from GitHub or upload files directly</p>
                  </div>
                  <div className="bg-secondary/50 rounded-xl p-4">
                    <h4 className="font-semibold text-emerald-500 mb-2">🌐 Custom Domains</h4>
                    <p className="text-sm text-muted-foreground">Connect your domains with automatic SSL</p>
                  </div>
                  <div className="bg-secondary/50 rounded-xl p-4">
                    <h4 className="font-semibold text-violet-500 mb-2">📊 System Monitoring</h4>
                    <p className="text-sm text-muted-foreground">Real-time CPU, RAM, GPU, disk, and network stats</p>
                  </div>
                  <div className="bg-secondary/50 rounded-xl p-4">
                    <h4 className="font-semibold text-amber-500 mb-2">💻 Web Terminal</h4>
                    <p className="text-sm text-muted-foreground">Full SSH-like terminal in your browser</p>
                  </div>
                  <div className="bg-secondary/50 rounded-xl p-4">
                    <h4 className="font-semibold text-pink-500 mb-2">🔐 Environment Variables</h4>
                    <p className="text-sm text-muted-foreground">Securely manage secrets and config</p>
                  </div>
                  <div className="bg-secondary/50 rounded-xl p-4">
                    <h4 className="font-semibold text-orange-500 mb-2">🔄 Auto-Deploy</h4>
                    <p className="text-sm text-muted-foreground">Automated deployments on every GitHub push</p>
                  </div>
                  <div className="bg-secondary/50 rounded-xl p-4">
                    <h4 className="font-semibold text-blue-500 mb-2">📦 Multi-Service</h4>
                    <p className="text-sm text-muted-foreground">Deploy projects with multiple Dockerfiles</p>
                  </div>
                </div>

                <div className="bg-secondary/50 rounded-xl p-6">
                  <h4 className="font-semibold mb-2">Quick Start</h4>
                  <ol className="list-decimal list-inside space-y-2 text-muted-foreground">
                    <li>Install Docklift on your VPS</li>
                    <li>Click <strong>New Project</strong> on the dashboard</li>
                    <li>Connect GitHub or upload files (must include a Dockerfile)</li>
                    <li>Enable <strong>Auto-Deploy</strong> in settings for automated updates</li>
                    <li>Click <strong>Deploy</strong></li>
                    <li>Access your app at the assigned port or custom domain</li>
                  </ol>
                </div>
              </section>

              {/* Installation */}
              <section id="installation" className="scroll-mt-20 mb-12">
                <h2 className="flex items-center gap-2 text-2xl font-bold mb-4">
                  <Download className="h-6 w-6 text-cyan-500" />
                  Installation
                </h2>
                <p className="text-muted-foreground mb-4">
                  Install Docklift on any Linux VPS with Docker installed.
                </p>
                
                <div className="bg-secondary/50 rounded-xl p-6 mb-4">
                  <h4 className="font-semibold mb-4 text-emerald-500">📥 Install</h4>
                  <CommandBlock 
                    label="One-liner install (recommended)" 
                    command="curl -fsSL https://raw.githubusercontent.com/SSujitX/docklift/master/install.sh | sudo bash" 
                    color="emerald"
                  />
                  <TerminalWindow 
                    title="Manual Installation"
                    color="emerald"
                    items={[
                      { comment: "Clone the repository", cmd: "git clone https://github.com/SSujitX/docklift.git" },
                      { comment: "Enter directory", cmd: "cd docklift" },
                      { comment: "Start with Docker Compose", cmd: "docker compose up -d" }
                    ]}
                  />
                  <p className="text-sm text-muted-foreground mt-3">
                    Access Docklift at <code className="bg-primary/10 px-1.5 py-0.5 rounded text-primary">http://YOUR_SERVER_IP:8080</code>
                  </p>
                </div>

                <div className="bg-secondary/50 rounded-xl p-6 mb-4">
                  <h4 className="font-semibold mb-4 text-red-500">🗑️ Uninstall</h4>
                  <CommandBlock 
                    label="One-liner uninstall" 
                    command='curl -fsSL "https://raw.githubusercontent.com/SSujitX/docklift/master/uninstall.sh" | sudo bash -s -- -y' 
                    color="red"
                  />
                  <TerminalWindow 
                    title="Manual Uninstall"
                    color="red"
                    items={[
                      { comment: "Enter directory", cmd: "cd docklift" },
                      { comment: "Stop and remove containers", cmd: "docker compose down -v" },
                      { comment: "Remove project files", cmd: "cd .. && rm -rf docklift" }
                    ]}
                  />
                  <div className="mt-3 p-3 bg-amber-500/10 rounded-lg border border-amber-500/20">
                    <p className="text-sm text-amber-600 dark:text-amber-400">
                      <strong>Warning:</strong> This will delete all deployed projects and data!
                    </p>
                  </div>
                </div>

                <div className="bg-gradient-to-r from-cyan-500/10 to-blue-500/10 rounded-xl p-6 border border-cyan-500/20 mb-4">
                  <h4 className="font-semibold mb-4 text-cyan-500">⬆️ Upgrade (Preserves Data)</h4>
                  <CommandBlock 
                    label="Safe one-liner upgrade" 
                    command="curl -fsSL https://raw.githubusercontent.com/SSujitX/docklift/master/upgrade.sh | sudo bash" 
                    color="cyan"
                  />
                  <div className="grid grid-cols-2 md:grid-cols-4 gap-3 mt-4">
                    <div className="flex items-center gap-2 text-xs text-muted-foreground">
                      <span className="text-emerald-500">✓</span> Database preserved
                    </div>
                    <div className="flex items-center gap-2 text-xs text-muted-foreground">
                      <span className="text-emerald-500">✓</span> Projects intact
                    </div>
                    <div className="flex items-center gap-2 text-xs text-muted-foreground">
                      <span className="text-emerald-500">✓</span> Auto-backup
                    </div>
                    <div className="flex items-center gap-2 text-xs text-muted-foreground">
                      <span className="text-emerald-500">✓</span> Auto-migrate DB
                    </div>
                  </div>
                </div>

                <div className="bg-secondary/50 rounded-xl p-6">
                  <h4 className="font-semibold mb-4">🛠️ Development Mode</h4>
                  <p className="text-sm text-muted-foreground mb-4">For contributing or local development:</p>
                  <TerminalWindow 
                    title="Local Development"
                    color="amber"
                    items={[
                      { comment: "Setup Backend", cmd: "cd backend && bun install" },
                      { cmd: "bun run dev" },
                      { comment: "Setup Frontend (separate terminal)", cmd: "cd frontend && npm install" },
                      { cmd: "npm run dev" }
                    ]}
                  />
                  <p className="text-sm text-muted-foreground mt-3">
                    Access at <code className="bg-primary/10 px-1.5 py-0.5 rounded text-primary">http://localhost:3000</code>
                  </p>
                </div>
              </section>

              {/* Auto-Deploy */}
              <section id="autodeploy" className="scroll-mt-20 mb-12">
                <h2 className="flex items-center gap-2 text-2xl font-bold mb-4">
                  <RefreshCw className="h-6 w-6 text-orange-500" />
                  Auto-Deploy (Webhooks)
                </h2>
                <p className="text-muted-foreground mb-4">
                  Enable automated deployments so your application rebuilds every time you push code to GitHub.
                </p>

                <div className="bg-secondary/50 rounded-xl p-6 mb-6">
                  <h4 className="font-semibold mb-3">How it Works</h4>
                  <p className="text-sm text-muted-foreground mb-4">
                    Docklift uses GitHub webhooks to listen for <code className="bg-primary/10 px-1.5 py-0.5 rounded text-primary">push</code> events. When you push code to your selected branch, GitHub notifies your Docklift instance, which then triggers a fresh deployment automatically.
                  </p>
                  
                  <h4 className="font-semibold mb-3 text-sm uppercase tracking-wider opacity-70">To Enable:</h4>
                  <ol className="list-decimal list-inside space-y-2 text-muted-foreground">
                    <li>Go to your <strong>Project Dashboard</strong></li>
                    <li>Navigate to the <strong>Source</strong> configuration tab</li>
                    <li>Toggle the <strong>Auto-Deploy</strong> switch to ON</li>
                  </ol>
                </div>

                <div className="bg-amber-500/10 border border-amber-500/20 rounded-xl p-6 mb-6">
                  <h4 className="font-semibold text-amber-600 dark:text-amber-500 mb-2 flex items-center gap-2">
                    <Shield className="h-5 w-5" />
                    Security (Webhook Secret)
                  </h4>
                  <p className="text-sm text-muted-foreground">
                    When you enable Auto-Deploy, Docklift automatically generates a <strong>Webhook Secret</strong>. This secret is used to verify that incoming deployment requests actually come from GitHub, preventing unauthorized build triggers. 
                    <br /><br />
                    If you are using a <strong>Private GitHub App</strong> (recommended), Docklift handles this configuration automatically.
                  </p>
                </div>

                <div className="grid gap-4 md:grid-cols-2">
                  <div className="bg-secondary/50 rounded-xl p-5 border border-border/40">
                    <h4 className="font-semibold mb-2 flex items-center gap-2">
                      <Rocket className="h-4 w-4 text-cyan-500" />
                      Instant Updates
                    </h4>
                    <p className="text-xs text-muted-foreground">No need to log in to the dashboard to deploy. Just <code className="bg-primary/10 px-1 py-0.5 rounded text-primary">git push</code> and watch it go.</p>
                  </div>
                  <div className="bg-secondary/50 rounded-xl p-5 border border-border/40">
                    <h4 className="font-semibold mb-2 flex items-center gap-2">
                      <History className="h-4 w-4 text-emerald-500" />
                      Deployment History
                    </h4>
                    <p className="text-xs text-muted-foreground">Auto-deployments are recorded in your history as <code className="bg-primary/10 px-1 py-0.5 rounded text-primary">webhook</code> triggers for easy auditing.</p>
                  </div>
                </div>
              </section>

              {/* GitHub Integration */}
              <section id="github" className="scroll-mt-20 mb-12">
                <h2 className="flex items-center gap-2 text-2xl font-bold mb-4">
                  <Github className="h-6 w-6 text-cyan-500" />
                  GitHub Integration
                </h2>
                <p className="text-muted-foreground mb-4">
                  Connect your GitHub account to deploy directly from public and private repositories.
                </p>

                <div className="bg-secondary/50 rounded-xl p-6 mb-4">
                  <h4 className="font-semibold mb-3 text-emerald-500">🔗 How to Connect (Install)</h4>
                  <ol className="list-decimal list-inside space-y-2 text-muted-foreground">
                    <li>Go to <strong>Settings → GitHub</strong></li>
                    <li>Click <strong>Connect GitHub</strong></li>
                    <li>Enter an app name (e.g., <code className="bg-primary/10 px-1.5 py-0.5 rounded text-primary">my-docklift</code>)</li>
                    <li>Click <strong>Create GitHub App</strong></li>
                    <li>You&apos;ll be redirected to GitHub to create the app</li>
                    <li>Select which repositories to grant access</li>
                    <li>Click <strong>Install</strong> on GitHub</li>
                    <li>Docklift auto-detects the installation ✅</li>
                  </ol>
                </div>

                <div className="bg-secondary/50 rounded-xl p-6 mb-4">
                  <h4 className="font-semibold mb-3 text-red-500">🗑️ How to Disconnect (Uninstall)</h4>
                  <ol className="list-decimal list-inside space-y-2 text-muted-foreground">
                    <li>Go to <strong>Settings → GitHub</strong></li>
                    <li>Click <strong>Connect GitHub</strong> (opens dialog)</li>
                    <li>Click <strong>Disconnect GitHub</strong> button</li>
                  </ol>
                  <div className="mt-4 p-3 bg-amber-500/10 rounded-lg border border-amber-500/20">
                    <p className="text-sm text-amber-600 dark:text-amber-400">
                      <strong>Note:</strong> To fully remove, also uninstall the app from GitHub:<br/>
                      GitHub → Settings → Applications → Installed GitHub Apps → Configure → Uninstall
                    </p>
                  </div>
                </div>

                <div className="grid gap-4 md:grid-cols-2">
                  <div className="bg-secondary/50 rounded-xl p-4">
                    <h4 className="font-semibold text-cyan-500">Features</h4>
                    <ul className="list-disc list-inside text-sm text-muted-foreground mt-2 space-y-1">
                      <li>List all accessible repositories</li>
                      <li>Select branch to deploy</li>
                      <li>Auto-pull on redeploy</li>
                      <li>Private repository support</li>
                      <li>No manual token creation needed</li>
                    </ul>
                  </div>
                  <div className="bg-secondary/50 rounded-xl p-4">
                    <h4 className="font-semibold text-violet-500">API Endpoints</h4>
                    <ul className="list-disc list-inside text-sm text-muted-foreground mt-2 space-y-1">
                      <li><code>GET /api/github/app-status</code></li>
                      <li><code>POST /api/github/manifest</code></li>
                      <li><code>POST /api/github/check-installation</code></li>
                      <li><code>GET /api/github/repos</code></li>
                      <li><code>GET /api/github/repos/:owner/:repo/branches</code></li>
                    </ul>
                  </div>
                </div>
              </section>

              {/* Deployment */}
              <section id="deployment" className="scroll-mt-20 mb-12">
                <h2 className="flex items-center gap-2 text-2xl font-bold mb-4">
                  <Container className="h-6 w-6 text-cyan-500" />
                  Deployment
                </h2>
                <p className="text-muted-foreground mb-4">
                  Docklift automatically builds Docker containers from your Dockerfiles and manages their lifecycle.
                </p>
                
                <div className="grid gap-4 md:grid-cols-2 lg:grid-cols-4 mb-6">
                  <div className="bg-secondary/50 rounded-xl p-4">
                    <h4 className="font-semibold text-cyan-500">Deploy</h4>
                    <p className="text-sm text-muted-foreground">Build and start container from Dockerfile</p>
                  </div>
                  <div className="bg-secondary/50 rounded-xl p-4">
                    <h4 className="font-semibold text-red-500">Stop</h4>
                    <p className="text-sm text-muted-foreground">Stop running container gracefully</p>
                  </div>
                  <div className="bg-secondary/50 rounded-xl p-4">
                    <h4 className="font-semibold text-amber-500">Restart</h4>
                    <p className="text-sm text-muted-foreground">Restart without rebuilding image</p>
                  </div>
                  <div className="bg-secondary/50 rounded-xl p-4">
                    <h4 className="font-semibold text-emerald-500">Redeploy</h4>
                    <p className="text-sm text-muted-foreground">Pull latest code, rebuild, and restart</p>
                  </div>
                </div>

                <div className="bg-secondary/50 rounded-xl p-6 mb-4">
                  <h4 className="font-semibold mb-3">Deployment Process</h4>
                  <ol className="list-decimal list-inside space-y-2 text-muted-foreground">
                    <li>Pull latest code from GitHub (if applicable)</li>
                    <li>Scan project for Dockerfiles</li>
                    <li>Auto-allocate ports (6000-7999 range)</li>
                    <li>Generate <code className="bg-primary/10 px-1.5 py-0.5 rounded text-primary">docker-compose.yml</code></li>
                    <li>Run <code className="bg-primary/10 px-1.5 py-0.5 rounded text-primary">docker compose up -d --build</code></li>
                    <li>Update Nginx reverse proxy config</li>
                    <li>Stream logs to browser in real-time</li>
                  </ol>
                </div>

                <div className="bg-secondary/50 rounded-xl p-6">
                  <h4 className="font-semibold mb-3">Multi-Service Projects</h4>
                  <p className="text-sm text-muted-foreground mb-4">
                    Docklift supports projects with multiple Dockerfiles. Each service gets its own port and can have custom domains.
                  </p>
                  <StaticCodeBlock 
                    title="Project Structure"
                    icon={FolderTree}
                    color="blue"
                    code={`my-project/
├── api/
│   └── Dockerfile      # → Port 6001
├── frontend/
│   └── Dockerfile      # → Port 6002
└── worker/
    └── Dockerfile      # → Port 6003`} 
                  />
                </div>
              </section>

              {/* Dockerfile */}
              <section id="dockerfile" className="scroll-mt-20 mb-12">
                <h2 className="flex items-center gap-2 text-2xl font-bold mb-4">
                  <FileCode className="h-6 w-6 text-cyan-500" />
                  Dockerfile Requirements
                </h2>
                <p className="text-muted-foreground mb-4">
                  Your project must include at least one Dockerfile. Docklift reads the <code className="bg-primary/10 px-1.5 py-0.5 rounded text-primary">EXPOSE</code> directive to set up port mapping.
                </p>
                
                <div className="space-y-6">
                  <StaticCodeBlock 
                    title="Example: Node.js Application" 
                    code={`FROM node:20-alpine
WORKDIR /app
COPY package*.json ./
RUN npm install
COPY . .
EXPOSE 3000
CMD ["npm", "start"]`} 
                  />
                  <StaticCodeBlock 
                    title="Example: Python Flask" 
                    code={`FROM python:3.11-slim
WORKDIR /app
COPY requirements.txt .
RUN pip install -r requirements.txt
COPY . .
EXPOSE 5000
CMD ["python", "app.py"]`} 
                  />
                </div>

                <div className="bg-gradient-to-r from-cyan-500/10 to-blue-500/10 rounded-xl p-4 border border-cyan-500/20">
                  <p className="text-sm">
                    <span className="font-semibold text-cyan-500">Important:</span>{" "}
                    Always include <code className="bg-primary/10 px-1.5 py-0.5 rounded text-primary">EXPOSE port</code> in your Dockerfile. Docklift uses this to auto-configure port mapping.
                  </p>
                </div>
              </section>

              {/* Custom Domains */}
              <section id="domains" className="scroll-mt-20 mb-12">
                <h2 className="flex items-center gap-2 text-2xl font-bold mb-4">
                  <Globe className="h-6 w-6 text-cyan-500" />
                  Custom Domains
                </h2>
                <p className="text-muted-foreground mb-4">
                  Connect custom domains to your Docklift panel and deployed services.
                </p>

                <div className="bg-secondary/50 rounded-xl p-6 mb-6">
                  <h4 className="font-semibold mb-3">Docklift Panel Domain</h4>
                  <p className="text-sm text-muted-foreground mb-4">
                    Access Docklift itself via custom domain instead of IP:port.
                  </p>
                  <ol className="list-decimal list-inside space-y-2 text-muted-foreground">
                    <li>Go to <strong>Settings → Domain</strong></li>
                    <li>Add your domain (e.g., <code className="bg-primary/10 px-1.5 py-0.5 rounded text-primary">panel.example.com</code>)</li>
                    <li>Port defaults to <strong>8080</strong> (Docklift frontend)</li>
                    <li>Configure DNS A record pointing to your server IP</li>
                  </ol>
                </div>

                <div className="bg-secondary/50 rounded-xl p-6 mb-6">
                  <h4 className="font-semibold mb-3">Service Domains</h4>
                  <p className="text-sm text-muted-foreground mb-4">
                    Assign custom domains to individual project services.
                  </p>
                  <ol className="list-decimal list-inside space-y-2 text-muted-foreground">
                    <li>Go to <strong>Project → Domains</strong> tab</li>
                    <li>Add domain for each service (e.g., <code className="bg-primary/10 px-1.5 py-0.5 rounded text-primary">api.example.com</code>)</li>
                    <li>Nginx automatically routes traffic to the service</li>
                  </ol>
                </div>
                
                <div className="bg-secondary/50 rounded-xl p-6 mb-6">
                  <h4 className="font-semibold mb-3">DNS Configuration</h4>
                  <div className="overflow-x-auto">
                    <table className="w-full text-sm">
                      <thead>
                        <tr className="border-b border-border">
                          <th className="text-left py-2 font-semibold">Type</th>
                          <th className="text-left py-2 font-semibold">Name</th>
                          <th className="text-left py-2 font-semibold">Value</th>
                          <th className="text-left py-2 font-semibold">Usage</th>
                        </tr>
                      </thead>
                      <tbody className="text-muted-foreground">
                        <tr className="border-b border-border/50">
                          <td className="py-2 font-mono text-cyan-500">A</td>
                          <td className="py-2 font-mono">@</td>
                          <td className="py-2">YOUR_SERVER_IP</td>
                          <td className="py-2">Root domain (example.com)</td>
                        </tr>
                        <tr className="border-b border-border/50">
                          <td className="py-2 font-mono text-cyan-500">A</td>
                          <td className="py-2 font-mono">panel</td>
                          <td className="py-2">YOUR_SERVER_IP</td>
                          <td className="py-2">Docklift panel</td>
                        </tr>
                        <tr className="border-b border-border/50">
                          <td className="py-2 font-mono text-cyan-500">A</td>
                          <td className="py-2 font-mono">api</td>
                          <td className="py-2">YOUR_SERVER_IP</td>
                          <td className="py-2">API service</td>
                        </tr>
                        <tr>
                          <td className="py-2 font-mono text-cyan-500">A</td>
                          <td className="py-2 font-mono">*</td>
                          <td className="py-2">YOUR_SERVER_IP</td>
                          <td className="py-2">Wildcard (all subdomains)</td>
                        </tr>
                      </tbody>
                    </table>
                  </div>
                </div>

                <div className="bg-gradient-to-r from-cyan-500/10 to-blue-500/10 rounded-xl p-4 border border-cyan-500/20">
                  <p className="text-sm">
                    <span className="font-semibold text-cyan-500">Cloudflare:</span>{" "}
                    Enable proxy (orange cloud) for SSL. Set SSL mode to <strong>Full (Strict)</strong>.
                  </p>
                </div>
              </section>

              {/* Environment Variables */}
              <section id="environment" className="scroll-mt-20 mb-12">
                <h2 className="flex items-center gap-2 text-2xl font-bold mb-4">
                  <Key className="h-6 w-6 text-cyan-500" />
                  Environment Variables
                </h2>
                <p className="text-muted-foreground mb-4">
                  Manage secrets and configuration for your deployments.
                </p>

                <div className="bg-secondary/50 rounded-xl p-6 mb-4">
                  <h4 className="font-semibold mb-3">How to Set</h4>
                  <ol className="list-decimal list-inside space-y-2 text-muted-foreground">
                    <li>Go to <strong>Project → Environment</strong> tab</li>
                    <li>Click <strong>Add Variable</strong></li>
                    <li>Enter key and value (values are hidden by default)</li>
                    <li>Click <strong>Save</strong></li>
                    <li><strong>Redeploy</strong> to apply changes</li>
                  </ol>
                </div>

                <StaticCodeBlock 
                  title="Example .env file" 
                  icon={Key}
                  code={`DATABASE_URL=postgresql://user:pass@host:5432/db
API_KEY=sk-123456789
NODE_ENV=production
PORT=3000`} 
                />
              </section>

              {/* System Overview */}
              <section id="system" className="scroll-mt-20 mb-12">
                <h2 className="flex items-center gap-2 text-2xl font-bold mb-4">
                  <Cpu className="h-6 w-6 text-cyan-500" />
                  System Overview
                </h2>
                <p className="text-muted-foreground mb-4">
                  Real-time monitoring of your server resources.
                </p>

                <div className="grid gap-4 md:grid-cols-2 lg:grid-cols-3 mb-6">
                  <div className="bg-secondary/50 rounded-xl p-4">
                    <div className="flex items-center gap-2 mb-2">
                      <Cpu className="h-4 w-4 text-cyan-500" />
                      <h4 className="font-semibold">CPU</h4>
                    </div>
                    <p className="text-sm text-muted-foreground">Usage %, cores, model, temperature</p>
                  </div>
                  <div className="bg-secondary/50 rounded-xl p-4">
                    <div className="flex items-center gap-2 mb-2">
                      <Activity className="h-4 w-4 text-emerald-500" />
                      <h4 className="font-semibold">Memory</h4>
                    </div>
                    <p className="text-sm text-muted-foreground">Total, used, free, percentage</p>
                  </div>
                  <div className="bg-secondary/50 rounded-xl p-4">
                    <div className="flex items-center gap-2 mb-2">
                      <HardDrive className="h-4 w-4 text-violet-500" />
                      <h4 className="font-semibold">Disk</h4>
                    </div>
                    <p className="text-sm text-muted-foreground">Mount points, usage per disk</p>
                  </div>
                  <div className="bg-secondary/50 rounded-xl p-4">
                    <div className="flex items-center gap-2 mb-2">
                      <Network className="h-4 w-4 text-amber-500" />
                      <h4 className="font-semibold">Network</h4>
                    </div>
                    <p className="text-sm text-muted-foreground">RX/TX speed, total bytes</p>
                  </div>
                  <div className="bg-secondary/50 rounded-xl p-4">
                    <div className="flex items-center gap-2 mb-2">
                      <Server className="h-4 w-4 text-pink-500" />
                      <h4 className="font-semibold">GPU</h4>
                    </div>
                    <p className="text-sm text-muted-foreground">Model, VRAM, utilization (if available)</p>
                  </div>
                  <div className="bg-secondary/50 rounded-xl p-4">
                    <div className="flex items-center gap-2 mb-2">
                      <Database className="h-4 w-4 text-blue-500" />
                      <h4 className="font-semibold">Processes</h4>
                    </div>
                    <p className="text-sm text-muted-foreground">Top 10 by CPU/memory usage</p>
                  </div>
                </div>

                <div className="bg-secondary/50 rounded-xl p-6">
                  <h4 className="font-semibold mb-3">System Controls</h4>
                  <div className="grid gap-4 md:grid-cols-3">
                    <div className="flex items-center gap-3 p-3 bg-background/50 rounded-lg">
                      <Trash2 className="h-5 w-5 text-amber-500" />
                      <div>
                        <p className="font-medium">Purge</p>
                        <p className="text-xs text-muted-foreground">Clean Docker and system caches</p>
                      </div>
                    </div>
                    <div className="flex items-center gap-3 p-3 bg-background/50 rounded-lg">
                      <Power className="h-5 w-5 text-red-500" />
                      <div>
                        <p className="font-medium">Reboot</p>
                        <p className="text-xs text-muted-foreground">Restart the server</p>
                      </div>
                    </div>
                    <div className="flex items-center gap-3 p-3 bg-background/50 rounded-lg">
                      <RefreshCw className="h-5 w-5 text-cyan-500" />
                      <div>
                        <p className="font-medium">Reset</p>
                        <p className="text-xs text-muted-foreground">Restart Docklift services</p>
                      </div>
                    </div>
                  </div>
                </div>
              </section>

              {/* Web Terminal */}
              <section id="terminal" className="scroll-mt-20 mb-12">
                <h2 className="flex items-center gap-2 text-2xl font-bold mb-4">
                  <Terminal className="h-6 w-6 text-cyan-500" />
                  Web Terminal
                </h2>
                <p className="text-muted-foreground mb-4">
                  Full SSH-like terminal in your browser at <code className="bg-primary/10 px-1.5 py-0.5 rounded text-primary">/terminal</code>.
                </p>
                
                <div className="bg-secondary/50 rounded-xl p-6 mb-4">
                  <h4 className="font-semibold mb-3">Features</h4>
                  <ul className="list-disc list-inside text-muted-foreground space-y-1">
                    <li>Full xterm.js terminal emulator</li>
                    <li>Execute any shell command</li>
                    <li>Quick command shortcuts (docker ps, docker stats, etc.)</li>
                    <li>Command history</li>
                    <li>Color-coded output</li>
                  </ul>
                </div>

                <TerminalWindow 
                  title="Common Docker Commands"
                  items={[
                    { comment: "List running containers", cmd: "docker ps" },
                    { comment: "View container logs", cmd: "docker logs <container_id>" },
                    { comment: "Live resource usage", cmd: "docker stats" },
                    { comment: "System process monitor", cmd: "htop" }
                  ]}
                />
              </section>

              {/* API Reference */}
              <section id="api" className="scroll-mt-20 mb-12">
                <h2 className="flex items-center gap-2 text-2xl font-bold mb-4">
                  <Code className="h-6 w-6 text-cyan-500" />
                  API Reference
                </h2>
                <p className="text-muted-foreground mb-4">
                  Backend API runs on port <code className="bg-primary/10 px-1.5 py-0.5 rounded text-primary">4000</code> by default.
                </p>

                <div className="space-y-6">
                  <div className="bg-secondary/50 rounded-xl p-6">
                    <h4 className="font-semibold mb-3 text-cyan-500">Projects API</h4>
                    <div className="overflow-x-auto">
                      <table className="w-full text-sm">
                        <thead>
                          <tr className="border-b border-border">
                            <th className="text-left py-2 font-semibold w-20">Method</th>
                            <th className="text-left py-2 font-semibold">Endpoint</th>
                            <th className="text-left py-2 font-semibold">Description</th>
                          </tr>
                        </thead>
                        <tbody className="text-muted-foreground font-mono text-xs">
                          <tr className="border-b border-border/50">
                            <td className="py-2 text-emerald-500">GET</td>
                            <td className="py-2">/api/projects</td>
                            <td className="py-2 font-sans">List all projects</td>
                          </tr>
                          <tr className="border-b border-border/50">
                            <td className="py-2 text-emerald-500">GET</td>
                            <td className="py-2">/api/projects/:id</td>
                            <td className="py-2 font-sans">Get project details</td>
                          </tr>
                          <tr className="border-b border-border/50">
                            <td className="py-2 text-blue-500">POST</td>
                            <td className="py-2">/api/projects</td>
                            <td className="py-2 font-sans">Create project</td>
                          </tr>
                          <tr className="border-b border-border/50">
                            <td className="py-2 text-amber-500">PATCH</td>
                            <td className="py-2">/api/projects/:id</td>
                            <td className="py-2 font-sans">Update project</td>
                          </tr>
                          <tr>
                            <td className="py-2 text-red-500">DELETE</td>
                            <td className="py-2">/api/projects/:id</td>
                            <td className="py-2 font-sans">Delete project</td>
                          </tr>
                        </tbody>
                      </table>
                    </div>
                  </div>

                  <div className="bg-secondary/50 rounded-xl p-6">
                    <h4 className="font-semibold mb-3 text-violet-500">Deployments API</h4>
                    <div className="overflow-x-auto">
                      <table className="w-full text-sm">
                        <thead>
                          <tr className="border-b border-border">
                            <th className="text-left py-2 font-semibold w-20">Method</th>
                            <th className="text-left py-2 font-semibold">Endpoint</th>
                            <th className="text-left py-2 font-semibold">Description</th>
                          </tr>
                        </thead>
                        <tbody className="text-muted-foreground font-mono text-xs">
                          <tr className="border-b border-border/50">
                            <td className="py-2 text-blue-500">POST</td>
                            <td className="py-2">/api/deployments/:projectId/deploy</td>
                            <td className="py-2 font-sans">Deploy project (streams logs)</td>
                          </tr>
                          <tr className="border-b border-border/50">
                            <td className="py-2 text-blue-500">POST</td>
                            <td className="py-2">/api/deployments/:projectId/stop</td>
                            <td className="py-2 font-sans">Stop containers</td>
                          </tr>
                          <tr className="border-b border-border/50">
                            <td className="py-2 text-blue-500">POST</td>
                            <td className="py-2">/api/deployments/:projectId/restart</td>
                            <td className="py-2 font-sans">Restart containers</td>
                          </tr>
                          <tr>
                            <td className="py-2 text-blue-500">POST</td>
                            <td className="py-2">/api/deployments/:projectId/redeploy</td>
                            <td className="py-2 font-sans">Rebuild and redeploy</td>
                          </tr>
                        </tbody>
                      </table>
                    </div>
                  </div>

                  <div className="bg-secondary/50 rounded-xl p-6">
                    <h4 className="font-semibold mb-3 text-emerald-500">System API</h4>
                    <div className="overflow-x-auto">
                      <table className="w-full text-sm">
                        <thead>
                          <tr className="border-b border-border">
                            <th className="text-left py-2 font-semibold w-20">Method</th>
                            <th className="text-left py-2 font-semibold">Endpoint</th>
                            <th className="text-left py-2 font-semibold">Description</th>
                          </tr>
                        </thead>
                        <tbody className="text-muted-foreground font-mono text-xs">
                          <tr className="border-b border-border/50">
                            <td className="py-2 text-emerald-500">GET</td>
                            <td className="py-2">/api/system/stats</td>
                            <td className="py-2 font-sans">Full system stats</td>
                          </tr>
                          <tr className="border-b border-border/50">
                            <td className="py-2 text-emerald-500">GET</td>
                            <td className="py-2">/api/system/ip</td>
                            <td className="py-2 font-sans">Server public IP</td>
                          </tr>
                          <tr className="border-b border-border/50">
                            <td className="py-2 text-blue-500">POST</td>
                            <td className="py-2">/api/system/purge</td>
                            <td className="py-2 font-sans">Clean Docker caches</td>
                          </tr>
                          <tr className="border-b border-border/50">
                            <td className="py-2 text-blue-500">POST</td>
                            <td className="py-2">/api/system/reboot</td>
                            <td className="py-2 font-sans">Reboot server</td>
                          </tr>
                          <tr>
                            <td className="py-2 text-blue-500">POST</td>
                            <td className="py-2">/api/system/execute</td>
                            <td className="py-2 font-sans">Execute shell command</td>
                          </tr>
                        </tbody>
                      </table>
                    </div>
                  </div>
                </div>
              </section>

              {/* File Management */}
              <section id="files" className="scroll-mt-20 mb-12">
                <h2 className="flex items-center gap-2 text-2xl font-bold mb-4">
                  <FolderTree className="h-6 w-6 text-cyan-500" />
                  File Management
                </h2>
                <p className="text-muted-foreground mb-4">
                  Browse and edit project files directly in the browser.
                </p>
                <div className="bg-secondary/50 rounded-xl p-6">
                  <h4 className="font-semibold mb-3">Features</h4>
                  <ul className="list-disc list-inside text-muted-foreground space-y-1">
                    <li>Tree view of all project files</li>
                    <li>Monaco-based code editor (VS Code engine)</li>
                    <li>Syntax highlighting for 100+ languages</li>
                    <li>Edit Dockerfile, config files, source code</li>
                    <li>Changes saved instantly to disk</li>
                  </ul>
                </div>
              </section>

              {/* Port Management */}
              <section id="ports" className="scroll-mt-20 mb-12">
                <h2 className="flex items-center gap-2 text-2xl font-bold mb-4">
                  <Plug className="h-6 w-6 text-cyan-500" />
                  Port Management
                </h2>
                <p className="text-muted-foreground mb-4">
                  Automatic port allocation for all services.
                </p>
                
                <div className="bg-secondary/50 rounded-xl p-6 mb-4">
                  <h4 className="font-semibold mb-3">Port Range</h4>
                  <div className="overflow-x-auto">
                    <table className="w-full text-sm">
                      <thead>
                        <tr className="border-b border-border">
                          <th className="text-left py-2 font-semibold">Setting</th>
                          <th className="text-left py-2 font-semibold">Default</th>
                          <th className="text-left py-2 font-semibold">Description</th>
                        </tr>
                      </thead>
                      <tbody className="text-muted-foreground">
                        <tr className="border-b border-border/50">
                          <td className="py-2 font-mono text-xs">PORT_RANGE_START</td>
                          <td className="py-2">6000</td>
                          <td className="py-2">First available port</td>
                        </tr>
                        <tr>
                          <td className="py-2 font-mono text-xs">PORT_RANGE_END</td>
                          <td className="py-2">7999</td>
                          <td className="py-2">Last available port</td>
                        </tr>
                      </tbody>
                    </table>
                  </div>
                </div>

                <div className="bg-gradient-to-r from-cyan-500/10 to-blue-500/10 rounded-xl p-4 border border-cyan-500/20">
                  <p className="text-sm">
                    <span className="font-semibold text-cyan-500">View Allocations:</span>{" "}
                    Go to <code className="bg-primary/10 px-1.5 py-0.5 rounded text-primary">/ports</code> to see all allocated ports across projects.
                  </p>
                </div>
              </section>

              {/* Useful Commands */}
              <section id="commands" className="scroll-mt-20 mb-12">
                <h2 className="flex items-center gap-2 text-2xl font-bold mb-4">
                  <Wrench className="h-6 w-6 text-cyan-500" />
                  Useful Commands
                </h2>
                <p className="text-muted-foreground mb-6">
                  Helpful commands for debugging and maintaining your Docklift instance. Click on any command to copy.
                </p>

                <div className="space-y-8">
                  {/* Infrastructure Logs */}
                  <div className="bg-secondary/50 rounded-xl p-6">
                    <h4 className="font-semibold mb-4 text-cyan-500 flex items-center gap-2">
                      📜 Check Infrastructure Logs
                    </h4>
                    <div className="space-y-4">
                      {[
                        { cmd: "docker logs docklift-backend --tail 50 -f", desc: "View Backend logs" },
                        { cmd: "docker logs docklift-frontend --tail 50 -f", desc: "View Frontend logs" },
                        { cmd: "docker compose up -d --build", desc: "Build & run" },
                        { cmd: "docker logs docklift-nginx-proxy --tail 50 -f", desc: "View Nginx Proxy logs" },
                      ].map((item, i) => (
                        <CommandBlock key={i} label={item.desc} command={item.cmd} color="cyan" />
                      ))}
                    </div>
                  </div>

                  {/* Project Debugging */}
                  <div className="bg-secondary/50 rounded-xl p-6">
                    <h4 className="font-semibold mb-4 text-violet-500 flex items-center gap-2">
                      🛰️ Project Debugging
                    </h4>
                    <div className="space-y-4">
                      {[
                        { cmd: "docker ps --filter name=dl_ --filter name=docklift_", desc: "List all Docklift-related containers" },
                        { cmd: "docker logs dl_<container_name> --tail 100 -f", desc: "View logs for a specific project container" },
                      ].map((item, i) => (
                        <CommandBlock key={i} label={item.desc} command={item.cmd} color="violet" />
                      ))}
                    </div>
                  </div>

                  {/* Cleaning & Resetting */}
                  <div className="bg-secondary/50 rounded-xl p-6">
                    <h4 className="font-semibold mb-4 text-red-500 flex items-center gap-2">
                      🧹 Cleaning & Resetting
                    </h4>
                    <div className="space-y-4">
                      {[
                        { cmd: 'curl -fsSL "https://raw.githubusercontent.com/SSujitX/docklift/master/uninstall.sh" | sudo bash -s -- -y', desc: "Nuclear Uninstall (Force-kills everything & deletes all data)" },
                        { cmd: "sudo fuser -k 3001/tcp", desc: "Force-kill anything holding port 3001" },
                        { cmd: "for port in {3001..3050}; do sudo fuser -k ${port}/tcp 2>/dev/null; done", desc: "Force-kill ports 3001-3050 range" },
                      ].map((item, i) => (
                        <CommandBlock key={i} label={item.desc} command={item.cmd} color="red" />
                      ))}
                    </div>
                  </div>

                  {/* Network & Port Check */}
                  <div className="bg-secondary/50 rounded-xl p-6">
                    <h4 className="font-semibold mb-4 text-emerald-500 flex items-center gap-2">
                      🌐 Network & Port Check
                    </h4>
                    <div className="space-y-4">
                      {[
                        { cmd: "sudo netstat -tulpn | grep 3001", desc: "Check if a port is in use and by what process" },
                        { cmd: "docker network inspect docklift_network", desc: "Inspect the Docklift internal network" },
                      ].map((item, i) => (
                        <CommandBlock key={i} label={item.desc} command={item.cmd} color="emerald" />
                      ))}
                    </div>
                  </div>

                  {/* Development Commands */}
                  <div className="bg-secondary/50 rounded-xl p-6">
                    <h4 className="font-semibold mb-4 text-amber-500 flex items-center gap-2">
                      🚀 Development Commands (Bun)
                    </h4>
                    <div className="space-y-4">
                      {[
                        { cmd: "bunx prisma studio", desc: "Open DB GUI" },
                        { cmd: "bunx prisma db push", desc: "Push schema changes" },
                        { cmd: "bunx prisma generate", desc: "Regenerate Prisma client" },
                        { cmd: "bun run dev", desc: "Start dev server (frontend/backend)" },
                        { cmd: "bunx next dev -p 3001", desc: "Frontend on custom port" },
                        { cmd: "bun run build", desc: "Build for production" },
                        { cmd: "bunx tsc --noEmit", desc: "TypeScript check" },
                      ].map((item, i) => (
                        <CommandBlock key={i} label={item.desc} command={item.cmd} color="amber" />
                      ))}
                    </div>
                  </div>

                  {/* Version Management */}
                  <div className="bg-secondary/50 rounded-xl p-6">
                    <h4 className="font-semibold mb-4 text-blue-500 flex items-center gap-2">
                      📦 Update & Version Management
                    </h4>
                    <div className="space-y-4">
                      {[
                        { cmd: "bun outdated", desc: "Check outdated packages" },
                        { cmd: "bun update", desc: "Update packages" },
                        { cmd: "bunx npm-check-updates -u && bun install", desc: "Update all to latest" },
                        { cmd: "bun version patch", desc: "Bump patch version (0.1.5 → 0.1.6)" },
                        { cmd: "bun version minor", desc: "Bump minor version (0.1.5 → 0.2.0)" },
                        { cmd: "bun version major", desc: "Bump major version (0.1.5 → 1.0.0)" },
                      ].map((item, i) => (
                        <CommandBlock key={i} label={item.desc} command={item.cmd} color="blue" />
                      ))}
                    </div>
                  </div>
                </div>
              </section>

              {/* Troubleshooting */}
              <section id="troubleshooting" className="scroll-mt-20 mb-12">
                <h2 className="flex items-center gap-2 text-2xl font-bold mb-4">
                  <Shield className="h-6 w-6 text-cyan-500" />
                  Troubleshooting
                </h2>
                <p className="text-muted-foreground mb-4">
                  Common issues and solutions.
                </p>

                <div className="space-y-4">
                  <div className="bg-secondary/50 rounded-xl p-6">
                    <h4 className="font-semibold mb-2 text-red-500">Port Already in Use</h4>
                    <ol className="list-decimal list-inside text-sm text-muted-foreground space-y-1">
                      <li>Check <code className="bg-primary/10 px-1.5 py-0.5 rounded text-primary">/ports</code> for allocations</li>
                      <li>Stop conflicting container</li>
                      <li>Or redeploy to get a new port</li>
                    </ol>
                  </div>

                  <div className="bg-secondary/50 rounded-xl p-6">
                    <h4 className="font-semibold mb-2 text-red-500">Domain Not Working</h4>
                    <ol className="list-decimal list-inside text-sm text-muted-foreground space-y-1">
                      <li>Verify DNS: <code className="bg-primary/10 px-1.5 py-0.5 rounded text-primary">nslookup yourdomain.com</code></li>
                      <li>Check Nginx config in <code className="bg-primary/10 px-1.5 py-0.5 rounded text-primary">/etc/docklift/nginx-conf/</code></li>
                      <li>Reload Nginx: <code className="bg-primary/10 px-1.5 py-0.5 rounded text-primary">docker exec docklift-nginx-proxy nginx -s reload</code></li>
                      <li>If using Cloudflare, ensure proxy is enabled and SSL mode is Full (Strict)</li>
                    </ol>
                  </div>

                  <div className="bg-secondary/50 rounded-xl p-6">
                    <h4 className="font-semibold mb-2 text-red-500">Build Failures</h4>
                    <ol className="list-decimal list-inside text-sm text-muted-foreground space-y-1">
                      <li>Check deployment logs in Project → Deployments</li>
                      <li>Verify Dockerfile syntax</li>
                      <li>Ensure EXPOSE directive is present</li>
                      <li>Check environment variables are set correctly</li>
                    </ol>
                  </div>

                  <div className="bg-secondary/50 rounded-xl p-6">
                    <h4 className="font-semibold mb-2 text-red-500">Container Keeps Stopping</h4>
                    <ol className="list-decimal list-inside text-sm text-muted-foreground space-y-1">
                      <li>Check container logs: <code className="bg-primary/10 px-1.5 py-0.5 rounded text-primary">docker logs container_name</code></li>
                      <li>Ensure app doesn't exit immediately</li>
                      <li>Verify port binding matches EXPOSE</li>
                    </ol>
                  </div>
                </div>
              </section>
            </div>
          </main>
        </div>
      </div>

      <Footer />
    </div>
  );
}
