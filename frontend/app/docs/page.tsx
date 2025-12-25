"use client";

import { useState, useEffect } from "react";
import { Header } from "@/components/Header";
import { Footer } from "@/components/Footer";
import { ChevronUp, Container, Rocket, FileCode, Settings2, Terminal, FolderTree, History, Plug, Globe } from "lucide-react";
import { Button } from "@/components/ui/button";

const sections = [
  { id: "getting-started", title: "Getting Started", icon: Rocket },
  { id: "deployment", title: "Deployment", icon: Container },
  { id: "dockerfile", title: "Dockerfile", icon: FileCode },
  { id: "domains", title: "Custom Domains", icon: Globe },
  { id: "configuration", title: "Configuration", icon: Settings2 },
  { id: "terminal", title: "Terminal Logs", icon: Terminal },
  { id: "files", title: "File Management", icon: FolderTree },
  { id: "history", title: "Deployment History", icon: History },
  { id: "ports", title: "Port Management", icon: Plug },
];

export default function DocsPage() {
  const [activeSection, setActiveSection] = useState("getting-started");
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
                On this page
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
                Learn how to deploy and manage your Docker containers with Docklift.
              </p>
            </div>

            <div className="prose prose-neutral dark:prose-invert max-w-none">
              <section id="getting-started" className="scroll-mt-20 mb-12">
                <h2 className="flex items-center gap-2 text-2xl font-bold mb-4">
                  <Rocket className="h-6 w-6 text-cyan-500" />
                  Getting Started
                </h2>
                <p className="text-muted-foreground mb-4">
                  Docklift is a self-hosted Docker deployment platform. Deploy your apps with a single click.
                </p>
                <div className="bg-secondary/50 rounded-xl p-6 mb-4">
                  <h4 className="font-semibold mb-2">Quick Start</h4>
                  <ol className="list-decimal list-inside space-y-2 text-muted-foreground">
                    <li>Click <strong>New Project</strong> on the dashboard</li>
                    <li>Upload your project files (must include a Dockerfile)</li>
                    <li>Click <strong>Deploy</strong></li>
                    <li>Access your app at the assigned port</li>
                  </ol>
                </div>
              </section>

              <section id="deployment" className="scroll-mt-20 mb-12">
                <h2 className="flex items-center gap-2 text-2xl font-bold mb-4">
                  <Container className="h-6 w-6 text-cyan-500" />
                  Deployment
                </h2>
                <p className="text-muted-foreground mb-4">
                  Docklift automatically builds and deploys Docker containers from your project files.
                </p>
                <div className="grid gap-4 md:grid-cols-2">
                  <div className="bg-secondary/50 rounded-xl p-4">
                    <h4 className="font-semibold text-cyan-500">Deploy</h4>
                    <p className="text-sm text-muted-foreground">Build and start container</p>
                  </div>
                  <div className="bg-secondary/50 rounded-xl p-4">
                    <h4 className="font-semibold text-orange-500">Stop</h4>
                    <p className="text-sm text-muted-foreground">Stop running container</p>
                  </div>
                  <div className="bg-secondary/50 rounded-xl p-4">
                    <h4 className="font-semibold text-green-500">Restart</h4>
                    <p className="text-sm text-muted-foreground">Restart without rebuild</p>
                  </div>
                  <div className="bg-secondary/50 rounded-xl p-4">
                    <h4 className="font-semibold text-violet-500">Redeploy</h4>
                    <p className="text-sm text-muted-foreground">Rebuild and recreate</p>
                  </div>
                </div>
              </section>

              <section id="dockerfile" className="scroll-mt-20 mb-12">
                <h2 className="flex items-center gap-2 text-2xl font-bold mb-4">
                  <FileCode className="h-6 w-6 text-cyan-500" />
                  Dockerfile Requirements
                </h2>
                <p className="text-muted-foreground mb-4">
                  Your project must include a Dockerfile. Docklift reads the EXPOSE directive to set up port mapping.
                </p>
                <div className="bg-zinc-900 rounded-xl p-4 font-mono text-sm text-zinc-300 overflow-x-auto">
                  <pre>{`FROM node:20-alpine
WORKDIR /app
COPY package*.json ./
RUN npm install
COPY . .
EXPOSE 3000
CMD ["npm", "start"]`}</pre>
                </div>
              </section>

              <section id="domains" className="scroll-mt-20 mb-12">
                <h2 className="flex items-center gap-2 text-2xl font-bold mb-4">
                  <Globe className="h-6 w-6 text-cyan-500" />
                  Custom Domains
                </h2>
                <p className="text-muted-foreground mb-4">
                  Connect your own domain to your deployed apps. Add a domain when creating a project.
                </p>
                
                <div className="bg-secondary/50 rounded-xl p-6 mb-6">
                  <h4 className="font-semibold mb-3">DNS Configuration</h4>
                  <p className="text-sm text-muted-foreground mb-4">
                    Point your domain to your server's IP address using DNS records:
                  </p>
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
                          <td className="py-2 font-mono">app</td>
                          <td className="py-2">YOUR_SERVER_IP</td>
                          <td className="py-2">Subdomain (app.example.com)</td>
                        </tr>
                        <tr className="border-b border-border/50">
                          <td className="py-2 font-mono text-violet-500">CNAME</td>
                          <td className="py-2 font-mono">www</td>
                          <td className="py-2">example.com</td>
                          <td className="py-2">www subdomain</td>
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

                <div className="bg-zinc-900 rounded-xl p-4 font-mono text-sm text-zinc-300 mb-4">
                  <div className="text-zinc-500 mb-2"># Example Cloudflare/Namecheap DNS Record</div>
                  <pre>{`Type:  A
Name:  @   (or 'app' for subdomain)
Value: 123.45.67.89  (your server IP)
TTL:   Auto`}</pre>
                </div>

                <div className="bg-gradient-to-r from-cyan-500/10 to-blue-500/10 rounded-xl p-4 border border-cyan-500/20">
                  <p className="text-sm">
                    <span className="font-semibold text-cyan-500">Note:</span>{" "}
                    Domain routing requires a reverse proxy (Nginx/Traefik) on your server to route traffic to containers.
                  </p>
                </div>
              </section>

              <section id="configuration" className="scroll-mt-20 mb-12">
                <h2 className="flex items-center gap-2 text-2xl font-bold mb-4">
                  <Settings2 className="h-6 w-6 text-cyan-500" />
                  Configuration
                </h2>
                <p className="text-muted-foreground mb-4">
                  Docklift auto-generates docker-compose.yml with proper networking and port mapping.
                </p>
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
                        <td className="py-2">3001</td>
                        <td className="py-2">Starting port for projects</td>
                      </tr>
                      <tr className="border-b border-border/50">
                        <td className="py-2 font-mono text-xs">PORT_RANGE_END</td>
                        <td className="py-2">3100</td>
                        <td className="py-2">Ending port for projects</td>
                      </tr>
                      <tr>
                        <td className="py-2 font-mono text-xs">DOCKER_NETWORK</td>
                        <td className="py-2">docklift_network</td>
                        <td className="py-2">Docker network name</td>
                      </tr>
                    </tbody>
                  </table>
                </div>
              </section>

              <section id="terminal" className="scroll-mt-20 mb-12">
                <h2 className="flex items-center gap-2 text-2xl font-bold mb-4">
                  <Terminal className="h-6 w-6 text-cyan-500" />
                  Terminal Logs
                </h2>
                <p className="text-muted-foreground mb-4">
                  Real-time build and deployment logs streamed directly to your browser.
                </p>
                <ul className="list-disc list-inside text-muted-foreground space-y-1">
                  <li>Color-coded output for success, errors, and warnings</li>
                  <li>Build step indicators with Docker layer caching status</li>
                  <li>Container lifecycle events (creating, starting, stopping)</li>
                </ul>
              </section>

              <section id="files" className="scroll-mt-20 mb-12">
                <h2 className="flex items-center gap-2 text-2xl font-bold mb-4">
                  <FolderTree className="h-6 w-6 text-cyan-500" />
                  File Management
                </h2>
                <p className="text-muted-foreground mb-4">
                  Browse and edit project files directly in the browser.
                </p>
                <ul className="list-disc list-inside text-muted-foreground space-y-1">
                  <li>Tree view of all project files</li>
                  <li>Inline editor for text files</li>
                  <li>Syntax highlighting for common file types</li>
                </ul>
              </section>

              <section id="history" className="scroll-mt-20 mb-12">
                <h2 className="flex items-center gap-2 text-2xl font-bold mb-4">
                  <History className="h-6 w-6 text-cyan-500" />
                  Deployment History
                </h2>
                <p className="text-muted-foreground mb-4">
                  Track all deployments with timestamps and status. Click any deployment to view its logs.
                </p>
              </section>

              <section id="ports" className="scroll-mt-20 mb-12">
                <h2 className="flex items-center gap-2 text-2xl font-bold mb-4">
                  <Plug className="h-6 w-6 text-cyan-500" />
                  Port Management
                </h2>
                <p className="text-muted-foreground mb-4">
                  Automatic port allocation from configurable range. View all allocated ports at /ports.
                </p>
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
