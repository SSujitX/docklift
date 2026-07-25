import { Download, Shield } from "lucide-react";
import { CommandBlock, TerminalWindow } from "./DocsShared";

export const Installation = () => (
  <section id="installation" className="scroll-mt-20 mb-12 text-left">
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
        label="Install latest release" 
        command="curl -fsSL https://raw.githubusercontent.com/SSujitX/docklift/master/install.sh | sudo bash" 
        color="emerald"
      />
      <CommandBlock
        label="Install a specific release (bash -s -- v=)"
        command="curl -fsSL https://raw.githubusercontent.com/SSujitX/docklift/master/install.sh | sudo bash -s -- v=2.0.2"
        color="emerald"
      />
      <CommandBlock
        label="Install a specific release (env)"
        command="curl -fsSL https://raw.githubusercontent.com/SSujitX/docklift/master/install.sh | sudo DOCKLIFT_VERSION=2.0.2 bash"
        color="emerald"
      />
      <p className="text-xs text-muted-foreground mb-4">
        Tags come from{" "}
        <a
          href="https://github.com/SSujitX/docklift/releases"
          className="text-cyan-500 hover:underline"
          target="_blank"
          rel="noopener noreferrer"
        >
          GitHub Releases
        </a>
        . Pass <code className="bg-primary/10 px-1 rounded">v=2.0.2</code> to bash or{" "}
        <code className="bg-primary/10 px-1 rounded">DOCKLIFT_VERSION=2.0.2</code> (with or without
        a leading <code className="bg-primary/10 px-1 rounded">v</code>). Default is latest when
        omitted. Pinning/downgrading rebuilds that tag and does not run{" "}
        <code className="bg-primary/10 px-1 rounded">upgrade.sh</code> DB snapshot — backup first.
      </p>
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
        The installer prints <code className="bg-primary/10 px-1.5 py-0.5 rounded text-primary">Dashboard: http://SERVER_IP:8080</code> and a one-time
        <strong> Setup code</strong>. Open that URL, paste the code, and create your admin account.
        Raw HTTP is convenient for first setup — not encrypted. Later: Settings → Domain (HTTPS), a firewall,
        or <code className="bg-primary/10 px-1.5 py-0.5 rounded text-primary">DASHBOARD_BIND=127.0.0.1</code> for localhost-only.
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
          <span className="text-emerald-500">✓</span> Image + DB rollback
        </div>
      </div>
      <p className="text-xs text-muted-foreground mt-3">
        Upgrade pins the previous git ref and database backup; on build or health failure it rolls back automatically.
      </p>
    </div>

    <div className="bg-secondary/50 rounded-xl p-6 mb-4">
      <h4 className="font-semibold mb-4 text-amber-500">🧪 Development Build (Latest Master)</h4>
      <p className="text-sm text-muted-foreground mb-4">For testing the latest features before release:</p>
      <CommandBlock
        label="Install from master branch"
        command="curl -fsSL https://raw.githubusercontent.com/SSujitX/docklift/master/install-dev.sh | sudo bash"
        color="amber"
      />
      <div className="mt-3 p-3 bg-amber-500/10 rounded-lg border border-amber-500/20">
        <p className="text-sm text-amber-600 dark:text-amber-400">
          <strong>Warning:</strong> This installs unreleased code from master branch. Use production install for stable deployments.
        </p>
      </div>
    </div>

    <div className="bg-secondary/50 rounded-xl p-6">
      <h4 className="font-semibold mb-4">🛠️ Local Development</h4>
      <p className="text-sm text-muted-foreground mb-4">For contributing or local development:</p>
      <TerminalWindow
        title="Local Development"
        color="amber"
        items={[
          { comment: "Setup Backend", cmd: "cd backend && bun install" },
          { cmd: "bun run dev" },
          { comment: "Setup Frontend (separate terminal)", cmd: "cd frontend && bun install" },
          { cmd: "bun run dev" }
        ]}
      />
      <p className="text-sm text-muted-foreground mt-3">
        Frontend: <code className="bg-primary/10 px-1.5 py-0.5 rounded text-primary">http://localhost:3600</code>
        {" "}· Backend API: <code className="bg-primary/10 px-1.5 py-0.5 rounded text-primary">http://localhost:8000</code>
      </p>
    </div>
  </section>
);
