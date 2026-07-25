import { useLocation, Link, Outlet } from "react-router-dom";
import { cn } from "@/lib/utils";
import {
  Info,
  RefreshCw,
  Globe,
  Cpu,
  Terminal,
  Key,
  FileCode,
  Container,
  FolderTree,
  Plug,
  User,
  ShieldCheck,
  Github,
  Shield,
  Wrench,
  Download,
  Code,
  Archive,
  Database,
} from "lucide-react";

const sections = [
  { id: "introduction", title: "Introduction", icon: Info, path: "/docs/introduction" },
  { id: "installation", title: "Installation", icon: Download, path: "/docs/installation" },
  { id: "github", title: "GitHub Integration", icon: Github, path: "/docs/github" },
  { id: "autodeploy", title: "Auto-Deploy", icon: RefreshCw, path: "/docs/autodeploy" },
  { id: "deployment", title: "Deployment", icon: Container, path: "/docs/deployment" },
  { id: "dockerfile", title: "Dockerfile", icon: FileCode, path: "/docs/dockerfile" },
  { id: "domains", title: "Custom Domains", icon: Globe, path: "/docs/domains" },
  { id: "environment", title: "Environment Variables", icon: Key, path: "/docs/environment" },
  { id: "databases", title: "Managed Databases", icon: Database, path: "/docs/databases" },
  { id: "system", title: "System Overview", icon: Cpu, path: "/docs/system" },
  { id: "terminal", title: "Web Terminal", icon: Terminal, path: "/docs/terminal" },
  { id: "api", title: "API Reference", icon: Code, path: "/docs/api" },
  { id: "files", title: "File Management", icon: FolderTree, path: "/docs/files" },
  { id: "ports", title: "Port Management", icon: Plug, path: "/docs/ports" },
  { id: "profile", title: "Profile Management", icon: User, path: "/docs/profile" },
  { id: "backup", title: "Backup & Restore", icon: Archive, path: "/docs/backup" },
  { id: "reset-password", title: "Reset Password", icon: ShieldCheck, path: "/docs/reset-password" },
  { id: "commands", title: "Useful Commands", icon: Wrench, path: "/docs/commands" },
  { id: "troubleshooting", title: "Troubleshooting", icon: Shield, path: "/docs/troubleshooting" },
];

export default function DocsLayout() {
  const { pathname } = useLocation();

  return (
    <div className="flex flex-col gap-10 lg:flex-row lg:gap-12">
      {/* Secondary navigation — the primary rail stays on the far left */}
      <aside className="lg:w-60 lg:shrink-0">
        <div className="shell-scroll sticky top-20 space-y-1 lg:max-h-[calc(100vh-6rem)] lg:overflow-y-auto lg:pr-2">
          <h4 className="mb-3 ml-3 text-[10px] font-semibold uppercase tracking-[0.2em] text-muted-foreground/70">
            Documentation
          </h4>
          <nav className="flex gap-1 overflow-x-auto pb-2 lg:flex-col lg:overflow-visible lg:pb-0">
            {sections.map((section) => {
              const isActive =
                pathname === section.path ||
                (pathname === "/docs" && section.id === "introduction");
              return (
                <Link
                  key={section.id}
                  to={section.path}
                  className={cn(
                    "group relative flex shrink-0 items-center gap-2.5 rounded-xl px-3 py-2 text-sm font-medium transition-colors",
                    isActive
                      ? "bg-brand/10 text-brand"
                      : "text-muted-foreground hover:bg-secondary/70 hover:text-foreground",
                  )}
                >
                  {isActive && (
                    <span className="absolute left-0 top-1/2 hidden h-5 w-[3px] -translate-y-1/2 rounded-r-full bg-brand lg:block" />
                  )}
                  <section.icon
                    className={cn(
                      "h-4 w-4 shrink-0",
                      isActive ? "text-brand" : "text-muted-foreground/50",
                    )}
                  />
                  <span className="whitespace-nowrap">{section.title}</span>
                </Link>
              );
            })}
          </nav>
        </div>
      </aside>

      <main className="min-w-0 flex-1 pb-10">
        <Outlet />
      </main>
    </div>
  );
}
