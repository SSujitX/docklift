"use client";

import { usePathname, useRouter } from "next/navigation";
import { Header } from "@/components/Header";
import { Footer } from "@/components/Footer";
import { cn } from "@/lib/utils";
import { 
  Info, Rocket, RefreshCw, Globe, Cpu, Terminal, Key, Database, Activity, HardDrive, 
  Network, Server, FileCode, Container, Trash2, Power, FolderTree, Plug, User, 
  ShieldCheck, Github, Shield, Wrench, Download, Code
} from "lucide-react";
import Link from "next/link";

const sections = [
  { id: "introduction", title: "Introduction", icon: Info, path: "/docs/introduction" },
  { id: "installation", title: "Installation", icon: Download, path: "/docs/installation" },
  { id: "github", title: "GitHub Integration", icon: Github, path: "/docs/github" },
  { id: "autodeploy", title: "Auto-Deploy", icon: RefreshCw, path: "/docs/autodeploy" },
  { id: "deployment", title: "Deployment", icon: Container, path: "/docs/deployment" },
  { id: "dockerfile", title: "Dockerfile", icon: FileCode, path: "/docs/dockerfile" },
  { id: "domains", title: "Custom Domains", icon: Globe, path: "/docs/domains" },
  { id: "environment", title: "Environment Variables", icon: Key, path: "/docs/environment" },
  { id: "system", title: "System Overview", icon: Cpu, path: "/docs/system" },
  { id: "terminal", title: "Web Terminal", icon: Terminal, path: "/docs/terminal" },
  { id: "api", title: "API Reference", icon: Code, path: "/docs/api" },
  { id: "files", title: "File Management", icon: FolderTree, path: "/docs/files" },
  { id: "ports", title: "Port Management", icon: Plug, path: "/docs/ports" },
  { id: "profile", title: "Profile Management", icon: User, path: "/docs/profile" },
  { id: "reset-password", title: "Reset Password", icon: ShieldCheck, path: "/docs/reset-password" },
  { id: "commands", title: "Useful Commands", icon: Wrench, path: "/docs/commands" },
  { id: "troubleshooting", title: "Troubleshooting", icon: Shield, path: "/docs/troubleshooting" },
];

export default function DocsLayout({ children }: { children: React.ReactNode }) {
  const pathname = usePathname();

  return (
    <div className="min-h-screen bg-background font-sans selection:bg-cyan-500/30 selection:text-cyan-200">
      <Header />
      
      <div className="max-w-7xl mx-auto px-4 sm:px-6 lg:px-8 py-10">
        <div className="flex flex-col lg:flex-row gap-12">
          {/* Sidebar Navigation */}
          <aside className="lg:w-64 lg:shrink-0">
            <div className="sticky top-24 space-y-1">
              <h4 className="text-xs font-black text-muted-foreground/60 uppercase tracking-[0.2em] mb-4 ml-3">
                Documentation
              </h4>
              <nav className="space-y-1">
                {sections.map((section) => {
                  const isActive = pathname === section.path || (pathname === "/docs" && section.id === "introduction");
                  return (
                    <Link
                      key={section.id}
                      href={section.path}
                      className={cn(
                        "w-full flex items-center gap-3 px-3 py-2.5 text-sm font-bold rounded-xl transition-all duration-200 group text-left",
                        isActive
                          ? "bg-cyan-500/10 text-cyan-500 shadow-[0_0_20px_rgba(6,182,212,0.1)]"
                          : "text-muted-foreground hover:bg-secondary/80 hover:text-foreground"
                      )}
                    >
                      <section.icon className={cn(
                        "h-4 w-4 transition-transform group-hover:scale-110",
                        isActive ? "text-cyan-500" : "text-muted-foreground/50"
                      )} />
                      {section.title}
                    </Link>
                  );
                })}
              </nav>
            </div>
          </aside>

          {/* Main Content */}
          <main className="flex-1 max-w-4xl">
            <div className="pb-20">
              {children}
            </div>
          </main>
        </div>
      </div>

      <Footer />
    </div>
  );
}
