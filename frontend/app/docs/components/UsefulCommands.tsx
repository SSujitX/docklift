import { Wrench } from "lucide-react";
import { CommandBlock } from "./DocsShared";

export const UsefulCommands = () => (
  <section id="commands" className="scroll-mt-20 mb-12 text-left">
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
);
