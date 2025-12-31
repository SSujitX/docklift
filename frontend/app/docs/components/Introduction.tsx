import { Info, Rocket, RefreshCw, Globe, Cpu, Terminal, Key, Database, Activity, HardDrive, Network, Server, FileCode, Container, Trash2, Power, FolderTree, Plug, User, ShieldCheck, Github, Shield, Wrench, Download } from "lucide-react";

export const Introduction = () => (
  <section id="introduction" className="scroll-mt-20 mb-12">
    <h2 className="flex items-center gap-2 text-2xl font-bold mb-4 text-left">
      <Info className="h-6 w-6 text-cyan-500" />
      Introduction
    </h2>
    <p className="text-muted-foreground mb-4 text-left">
      <strong>Docklift</strong> is a self-hosted Docker deployment platform that makes it easy to deploy, manage, and monitor your containerized applications.
    </p>
    
    <div className="bg-gradient-to-r from-cyan-500/10 to-blue-500/10 rounded-xl p-6 border border-cyan-500/20 mb-6 text-left">
      <h4 className="font-semibold mb-3 text-lg">What is Docklift?</h4>
      <p className="text-muted-foreground mb-4">
        Docklift is like Vercel or Railway, but self-hosted on your own VPS. Deploy apps from GitHub or file uploads with a single click, manage custom domains, monitor system resources, and access your server terminal - all from a beautiful web interface.
      </p>
    </div>

    <div className="grid gap-4 md:grid-cols-2 lg:grid-cols-3 mb-6 text-left">
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

    <div className="bg-secondary/50 rounded-xl p-6 text-left">
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
);
