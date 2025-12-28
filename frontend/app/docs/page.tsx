// Documentation page - comprehensive guide to Docklift features and API reference
"use client";

import { useState, useEffect } from "react";
import { Header } from "@/components/Header";
import { Footer } from "@/components/Footer";
import { 
  ChevronUp, Container, Rocket, FileCode, Settings2, Terminal, FolderTree, 
  History, Plug, Globe, Github, Server, Key, Database, Activity, Cpu, 
  HardDrive, Network, Shield, RefreshCw, Trash2, Power, Code, Download, Info, Wrench, Copy, Check
} from "lucide-react";
import { Button } from "@/components/ui/button";

const sections = [
  { id: "introduction", title: "Introduction", icon: Info },
  { id: "installation", title: "Installation", icon: Download },
  { id: "github", title: "GitHub Integration", icon: Github },
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

export default function DocsPage() {
  const [activeSection, setActiveSection] = useState("introduction");
  const [showScrollTop, setShowScrollTop] = useState(false);

  useEffect(() => {
    const handleScroll = () => {
      setShowScrollTop(window.scrollY > 300);
      
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

  const scrollToTop = () => {
    window.scrollTo({ top: 0, behavior: "smooth" });
  };

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
        <div className="flex gap-8">
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

          {/* Main Content */}
          <main className="flex-1 min-w-0">
            <div className="mb-8">
              <h1 className="text-4xl font-bold mb-2">Documentation</h1>
              <p className="text-lg text-muted-foreground">
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
                  <h4 className="font-semibold mb-3 text-emerald-500">📥 Install</h4>
                  <div className="bg-zinc-900 rounded-lg p-3 font-mono text-sm text-zinc-300 mb-3">
                    <div className="text-zinc-500 mb-2"># One-liner install (recommended)</div>
                    <pre className="text-cyan-400">curl -fsSL https://raw.githubusercontent.com/SSujitX/docklift/master/install.sh | sudo bash</pre>
                  </div>
                  <div className="bg-zinc-900 rounded-lg p-3 font-mono text-sm text-zinc-300">
                    <div className="text-zinc-500 mb-2"># Manual install</div>
                    <pre>{`git clone https://github.com/SSujitX/docklift.git
cd docklift
docker compose up -d`}</pre>
                  </div>
                  <p className="text-sm text-muted-foreground mt-3">
                    Access Docklift at <code className="bg-primary/10 px-1.5 py-0.5 rounded text-primary">http://YOUR_SERVER_IP:8080</code>
                  </p>
                </div>

                <div className="bg-secondary/50 rounded-xl p-6 mb-4">
                  <h4 className="font-semibold mb-3 text-red-500">🗑️ Uninstall</h4>
                  <div className="bg-zinc-900 rounded-lg p-3 font-mono text-sm text-zinc-300 mb-3">
                    <div className="text-zinc-500 mb-2"># One-liner uninstall</div>
                    <pre className="text-red-400">curl -fsSL "https://raw.githubusercontent.com/SSujitX/docklift/master/uninstall.sh" | sudo bash -s -- -y</pre>
                  </div>
                  <div className="bg-zinc-900 rounded-lg p-3 font-mono text-sm text-zinc-300">
                    <div className="text-zinc-500 mb-2"># Manual uninstall</div>
                    <pre>{`cd docklift
docker compose down -v     # Stop and remove containers
cd ..
rm -rf docklift            # Remove files`}</pre>
                  </div>
                  <div className="mt-3 p-3 bg-amber-500/10 rounded-lg border border-amber-500/20">
                    <p className="text-sm text-amber-600 dark:text-amber-400">
                      <strong>Warning:</strong> This will delete all deployed projects and data!
                    </p>
                  </div>
                </div>

                <div className="bg-gradient-to-r from-cyan-500/10 to-blue-500/10 rounded-xl p-6 border border-cyan-500/20 mb-4">
                  <h4 className="font-semibold mb-3 text-cyan-500">⬆️ Upgrade (Preserves Data)</h4>
                  <div className="bg-zinc-900 rounded-lg p-3 font-mono text-sm text-zinc-300 mb-3">
                    <div className="text-zinc-500 mb-2"># Safe one-liner upgrade</div>
                    <pre className="text-cyan-400">curl -fsSL https://raw.githubusercontent.com/SSujitX/docklift/master/upgrade.sh | sudo bash</pre>
                  </div>
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
                  <h4 className="font-semibold mb-3">🛠️ Development Mode</h4>
                  <p className="text-sm text-muted-foreground mb-3">For contributing or local development:</p>
                  <div className="bg-zinc-900 rounded-lg p-3 font-mono text-sm text-zinc-300">
                    <pre>{`# Backend
cd backend && bun install
bun run dev

# Frontend (new terminal)
cd frontend && npm install
npm run dev`}</pre>
                  </div>
                  <p className="text-sm text-muted-foreground mt-3">
                    Access at <code className="bg-primary/10 px-1.5 py-0.5 rounded text-primary">http://localhost:3000</code>
                  </p>
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
                  <p className="text-sm text-muted-foreground mb-3">
                    Docklift supports projects with multiple Dockerfiles. Each service gets its own port and can have custom domains.
                  </p>
                  <div className="bg-zinc-900 rounded-lg p-3 font-mono text-sm text-zinc-300">
                    <pre>{`my-project/
├── api/
│   └── Dockerfile      # → Port 6001
├── frontend/
│   └── Dockerfile      # → Port 6002
└── worker/
    └── Dockerfile      # → Port 6003`}</pre>
                  </div>
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
                
                <div className="bg-zinc-900 rounded-xl p-4 font-mono text-sm text-zinc-300 overflow-x-auto mb-4">
                  <div className="text-zinc-500 mb-2"># Example: Node.js Application</div>
                  <pre>{`FROM node:20-alpine
WORKDIR /app
COPY package*.json ./
RUN npm install
COPY . .
EXPOSE 3000
CMD ["npm", "start"]`}</pre>
                </div>

                <div className="bg-zinc-900 rounded-xl p-4 font-mono text-sm text-zinc-300 overflow-x-auto mb-4">
                  <div className="text-zinc-500 mb-2"># Example: Python Flask</div>
                  <pre>{`FROM python:3.11-slim
WORKDIR /app
COPY requirements.txt .
RUN pip install -r requirements.txt
COPY . .
EXPOSE 5000
CMD ["python", "app.py"]`}</pre>
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

                <div className="bg-zinc-900 rounded-xl p-4 font-mono text-sm text-zinc-300 overflow-x-auto">
                  <div className="text-zinc-500 mb-2"># Example variables</div>
                  <pre>{`DATABASE_URL=postgresql://user:pass@host:5432/db
API_KEY=sk-123456789
NODE_ENV=production
PORT=3000`}</pre>
                </div>
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

                <div className="bg-zinc-900 rounded-xl p-4 font-mono text-sm text-zinc-300">
                  <div className="text-zinc-500 mb-2"># Example commands</div>
                  <pre>{`docker ps                    # List running containers
docker logs <container>      # View container logs
docker stats                 # Live resource usage
htop                         # System process monitor`}</pre>
                </div>
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
                    <div className="space-y-3">
                      {[
                        { cmd: "docker logs docklift-backend --tail 50 -f", desc: "View Backend logs" },
                        { cmd: "docker logs docklift-frontend --tail 50 -f", desc: "View Frontend logs" },
                        { cmd: "docker compose up -d --build", desc: "Build & run" },
                        { cmd: "docker logs docklift-nginx-proxy --tail 50 -f", desc: "View Nginx Proxy logs" },
                      ].map((item, i) => (
                        <div key={i} className="group">
                          <p className="text-xs text-muted-foreground mb-1"># {item.desc}</p>
                          <button
                            onClick={() => {
                              navigator.clipboard.writeText(item.cmd);
                            }}
                            className="w-full text-left font-mono text-sm bg-zinc-900 hover:bg-zinc-800 text-zinc-300 px-4 py-2.5 rounded-lg flex items-center justify-between group transition-colors"
                          >
                            <code className="text-cyan-400">{item.cmd}</code>
                            <Copy className="h-4 w-4 text-zinc-500 group-hover:text-cyan-400 transition-colors" />
                          </button>
                        </div>
                      ))}
                    </div>
                  </div>

                  {/* Project Debugging */}
                  <div className="bg-secondary/50 rounded-xl p-6">
                    <h4 className="font-semibold mb-4 text-violet-500 flex items-center gap-2">
                      🛰️ Project Debugging
                    </h4>
                    <div className="space-y-3">
                      {[
                        { cmd: "docker ps --filter name=dl_ --filter name=docklift_", desc: "List all Docklift-related containers" },
                        { cmd: "docker logs dl_<container_name> --tail 100 -f", desc: "View logs for a specific project container" },
                      ].map((item, i) => (
                        <div key={i} className="group">
                          <p className="text-xs text-muted-foreground mb-1"># {item.desc}</p>
                          <button
                            onClick={() => {
                              navigator.clipboard.writeText(item.cmd);
                            }}
                            className="w-full text-left font-mono text-sm bg-zinc-900 hover:bg-zinc-800 text-zinc-300 px-4 py-2.5 rounded-lg flex items-center justify-between group transition-colors"
                          >
                            <code className="text-violet-400">{item.cmd}</code>
                            <Copy className="h-4 w-4 text-zinc-500 group-hover:text-violet-400 transition-colors" />
                          </button>
                        </div>
                      ))}
                    </div>
                  </div>

                  {/* Cleaning & Resetting */}
                  <div className="bg-secondary/50 rounded-xl p-6">
                    <h4 className="font-semibold mb-4 text-red-500 flex items-center gap-2">
                      🧹 Cleaning & Resetting
                    </h4>
                    <div className="space-y-3">
                      {[
                        { cmd: 'curl -fsSL "https://raw.githubusercontent.com/SSujitX/docklift/master/uninstall.sh" | sudo bash -s -- -y', desc: "Nuclear Uninstall (Force-kills everything & deletes all data)" },
                        { cmd: "sudo fuser -k 3001/tcp", desc: "Force-kill anything holding port 3001" },
                        { cmd: "for port in {3001..3050}; do sudo fuser -k ${port}/tcp 2>/dev/null; done", desc: "Force-kill ports 3001-3050 range" },
                      ].map((item, i) => (
                        <div key={i} className="group">
                          <p className="text-xs text-muted-foreground mb-1"># {item.desc}</p>
                          <button
                            onClick={() => {
                              navigator.clipboard.writeText(item.cmd);
                            }}
                            className="w-full text-left font-mono text-sm bg-zinc-900 hover:bg-zinc-800 text-zinc-300 px-4 py-2.5 rounded-lg flex items-center justify-between group transition-colors overflow-x-auto"
                          >
                            <code className="text-red-400 whitespace-nowrap">{item.cmd}</code>
                            <Copy className="h-4 w-4 text-zinc-500 group-hover:text-red-400 transition-colors shrink-0 ml-2" />
                          </button>
                        </div>
                      ))}
                    </div>
                  </div>

                  {/* Network & Port Check */}
                  <div className="bg-secondary/50 rounded-xl p-6">
                    <h4 className="font-semibold mb-4 text-emerald-500 flex items-center gap-2">
                      🌐 Network & Port Check
                    </h4>
                    <div className="space-y-3">
                      {[
                        { cmd: "sudo netstat -tulpn | grep 3001", desc: "Check if a port is in use and by what process" },
                        { cmd: "docker network inspect docklift_network", desc: "Inspect the Docklift internal network" },
                      ].map((item, i) => (
                        <div key={i} className="group">
                          <p className="text-xs text-muted-foreground mb-1"># {item.desc}</p>
                          <button
                            onClick={() => {
                              navigator.clipboard.writeText(item.cmd);
                            }}
                            className="w-full text-left font-mono text-sm bg-zinc-900 hover:bg-zinc-800 text-zinc-300 px-4 py-2.5 rounded-lg flex items-center justify-between group transition-colors"
                          >
                            <code className="text-emerald-400">{item.cmd}</code>
                            <Copy className="h-4 w-4 text-zinc-500 group-hover:text-emerald-400 transition-colors" />
                          </button>
                        </div>
                      ))}
                    </div>
                  </div>

                  {/* Development Commands */}
                  <div className="bg-secondary/50 rounded-xl p-6">
                    <h4 className="font-semibold mb-4 text-amber-500 flex items-center gap-2">
                      🚀 Development Commands (Bun)
                    </h4>
                    <div className="space-y-3">
                      {[
                        { cmd: "bunx prisma studio", desc: "Open DB GUI" },
                        { cmd: "bunx prisma db push", desc: "Push schema changes" },
                        { cmd: "bunx prisma generate", desc: "Regenerate Prisma client" },
                        { cmd: "bun run dev", desc: "Start dev server (frontend/backend)" },
                        { cmd: "bunx next dev -p 3001", desc: "Frontend on custom port" },
                        { cmd: "bun run build", desc: "Build for production" },
                        { cmd: "bunx tsc --noEmit", desc: "TypeScript check" },
                      ].map((item, i) => (
                        <div key={i} className="group">
                          <p className="text-xs text-muted-foreground mb-1"># {item.desc}</p>
                          <button
                            onClick={() => {
                              navigator.clipboard.writeText(item.cmd);
                            }}
                            className="w-full text-left font-mono text-sm bg-zinc-900 hover:bg-zinc-800 text-zinc-300 px-4 py-2.5 rounded-lg flex items-center justify-between group transition-colors"
                          >
                            <code className="text-amber-400">{item.cmd}</code>
                            <Copy className="h-4 w-4 text-zinc-500 group-hover:text-amber-400 transition-colors" />
                          </button>
                        </div>
                      ))}
                    </div>
                  </div>

                  {/* Version Management */}
                  <div className="bg-secondary/50 rounded-xl p-6">
                    <h4 className="font-semibold mb-4 text-blue-500 flex items-center gap-2">
                      📦 Update & Version Management
                    </h4>
                    <div className="space-y-3">
                      {[
                        { cmd: "bun outdated", desc: "Check outdated packages" },
                        { cmd: "bun update", desc: "Update packages" },
                        { cmd: "bunx npm-check-updates -u && bun install", desc: "Update all to latest" },
                        { cmd: "bun version patch", desc: "Bump patch version (0.1.5 → 0.1.6)" },
                        { cmd: "bun version minor", desc: "Bump minor version (0.1.5 → 0.2.0)" },
                        { cmd: "bun version major", desc: "Bump major version (0.1.5 → 1.0.0)" },
                      ].map((item, i) => (
                        <div key={i} className="group">
                          <p className="text-xs text-muted-foreground mb-1"># {item.desc}</p>
                          <button
                            onClick={() => {
                              navigator.clipboard.writeText(item.cmd);
                            }}
                            className="w-full text-left font-mono text-sm bg-zinc-900 hover:bg-zinc-800 text-zinc-300 px-4 py-2.5 rounded-lg flex items-center justify-between group transition-colors"
                          >
                            <code className="text-blue-400">{item.cmd}</code>
                            <Copy className="h-4 w-4 text-zinc-500 group-hover:text-blue-400 transition-colors" />
                          </button>
                        </div>
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

      {/* Scroll to top button */}
      {showScrollTop && (
        <Button
          onClick={scrollToTop}
          className="fixed bottom-6 right-6 h-10 w-10 rounded-full shadow-lg bg-primary hover:bg-primary/90"
          size="icon"
        >
          <ChevronUp className="h-5 w-5" />
        </Button>
      )}

      <Footer />
    </div>
  );
}
