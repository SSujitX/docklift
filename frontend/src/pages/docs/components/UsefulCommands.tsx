import { Wrench } from "lucide-react";
import { CommandBlock } from "./DocsShared";

export const UsefulCommands = () => (
  <section id="commands" className="scroll-mt-20 mb-12 text-left">
    <h2 className="flex items-center gap-2 text-2xl font-bold mb-4">
      <Wrench className="h-6 w-6 text-cyan-500" />
      Useful Commands
    </h2>
    <p className="text-muted-foreground mb-6">
      Debugging and maintenance commands. Full guide:{" "}
      <code className="text-xs bg-secondary px-1.5 py-0.5 rounded">COMMANDS.md</code> in the repo.
      Click any command to copy.
    </p>

    <div className="space-y-8">
      <div className="bg-secondary/50 rounded-xl p-6">
        <h4 className="font-semibold mb-4 text-cyan-500 flex items-center gap-2">
          Start / rebuild
        </h4>
        <div className="space-y-4">
          {[
            { cmd: "docker compose up -d --build", desc: "Build & start all services" },
            { cmd: "docker compose up -d --build frontend", desc: "Rebuild frontend only" },
            { cmd: "docker compose up -d --build backend", desc: "Rebuild backend only" },
            { cmd: "docker compose ps", desc: "Service status" },
          ].map((item, i) => (
            <CommandBlock key={i} label={item.desc} command={item.cmd} color="cyan" />
          ))}
        </div>
      </div>

      <div className="bg-secondary/50 rounded-xl p-6">
        <h4 className="font-semibold mb-4 text-cyan-500 flex items-center gap-2">
          Platform logs
        </h4>
        <div className="space-y-4">
          {[
            { cmd: "docker logs docklift-backend --tail 100 -f", desc: "Backend API logs" },
            { cmd: "docker logs docklift-frontend --tail 100 -f", desc: "Frontend (Vite SPA) logs" },
            { cmd: "docker logs docklift-nginx --tail 100 -f", desc: "Dashboard gateway (:8080)" },
            { cmd: "docker logs docklift-nginx-proxy --tail 100 -f", desc: "App domains proxy (:80)" },
            { cmd: "docker logs docklift-certbot --tail 100 -f", desc: "Certificate renewals" },
            { cmd: "docker compose logs -f --tail 100", desc: "All Compose services live" },
          ].map((item, i) => (
            <CommandBlock key={i} label={item.desc} command={item.cmd} color="cyan" />
          ))}
        </div>
      </div>

      <div className="bg-secondary/50 rounded-xl p-6">
        <h4 className="font-semibold mb-4 text-violet-500 flex items-center gap-2">
          Nginx
        </h4>
        <div className="space-y-4">
          {[
            { cmd: "docker exec docklift-nginx nginx -t", desc: "Test gateway config" },
            { cmd: "docker exec docklift-nginx nginx -s reload", desc: "Reload gateway" },
            { cmd: "docker exec docklift-nginx-proxy nginx -t", desc: "Test domain proxy config" },
            { cmd: "docker exec docklift-nginx-proxy nginx -s reload", desc: "Reload domain proxy" },
          ].map((item, i) => (
            <CommandBlock key={i} label={item.desc} command={item.cmd} color="violet" />
          ))}
        </div>
      </div>

      <div className="bg-secondary/50 rounded-xl p-6">
        <h4 className="font-semibold mb-4 text-violet-500 flex items-center gap-2">
          Project apps
        </h4>
        <div className="space-y-4">
          {[
            { cmd: "docker ps --filter name=dl_ --filter name=docklift", desc: "List Docklift + app containers" },
            { cmd: "docker logs dl_<slug>_<id>_<service> --tail 200 -f", desc: "App container logs" },
          ].map((item, i) => (
            <CommandBlock key={i} label={item.desc} command={item.cmd} color="violet" />
          ))}
        </div>
      </div>

      <div className="bg-secondary/50 rounded-xl p-6">
        <h4 className="font-semibold mb-4 text-amber-500 flex items-center gap-2">
          Password & version
        </h4>
        <div className="space-y-4">
          {[
            { cmd: "docker exec -it docklift-backend node dist/scripts/reset-password.js", desc: "Reset admin password (prints new one)" },
            { cmd: 'grep \'"version"\' backend/package.json', desc: "Show current Docklift version" },
            { cmd: "curl -s http://SERVER_IP:8080/api/health", desc: "Health check via dashboard gateway" },
          ].map((item, i) => (
            <CommandBlock key={i} label={item.desc} command={item.cmd} color="amber" />
          ))}
        </div>
        <p className="text-xs text-muted-foreground mt-4">
          Releases: push conventional commits, then GitHub Actions → <strong>Release & Test</strong>.
          Do not use <code className="bg-background px-1 rounded">npm version</code> by hand.
        </p>
      </div>

      <div className="bg-secondary/50 rounded-xl p-6">
        <h4 className="font-semibold mb-4 text-emerald-500 flex items-center gap-2">
          Network & ports
        </h4>
        <div className="space-y-4">
          {[
            { cmd: "docker network inspect docklift_network", desc: "Control-plane network" },
            { cmd: "docker network ls --filter label=com.docklift.managed=true", desc: "Project networks (dl-net-*)" },
            { cmd: "sudo ss -tulpn | grep -E ':(80|443|8080|5500)'", desc: "Check host listeners" },
            { cmd: "for port in {5500..5600}; do sudo fuser -k ${port}/tcp 2>/dev/null; done", desc: "Free app port pool (5500–5600)" },
          ].map((item, i) => (
            <CommandBlock key={i} label={item.desc} command={item.cmd} color="emerald" />
          ))}
        </div>
      </div>

      <div className="bg-secondary/50 rounded-xl p-6">
        <h4 className="font-semibold mb-4 text-red-500 flex items-center gap-2">
          Install / upgrade / uninstall
        </h4>
        <div className="space-y-4">
          {[
            { cmd: "curl -fsSL https://raw.githubusercontent.com/SSujitX/docklift/master/install.sh | sudo bash", desc: "Install latest release" },
            { cmd: "curl -fsSL https://raw.githubusercontent.com/SSujitX/docklift/master/install.sh | sudo bash -s -- v=2.0.2", desc: "Install pinned release (v=)" },
            { cmd: "curl -fsSL https://raw.githubusercontent.com/SSujitX/docklift/master/upgrade.sh | sudo bash", desc: "Safe upgrade (keeps data)" },
            { cmd: 'curl -fsSL "https://raw.githubusercontent.com/SSujitX/docklift/master/uninstall.sh" | sudo bash -s -- -y', desc: "Full uninstall (destructive)" },
          ].map((item, i) => (
            <CommandBlock key={i} label={item.desc} command={item.cmd} color="red" />
          ))}
        </div>
      </div>

      <div className="bg-secondary/50 rounded-xl p-6">
        <h4 className="font-semibold mb-4 text-amber-500 flex items-center gap-2">
          Local development
        </h4>
        <div className="space-y-4">
          {[
            { cmd: "cd backend && bun run dev", desc: "Backend on :8000" },
            { cmd: "cd frontend && bun run dev", desc: "Vite frontend on :3600" },
            { cmd: "cd backend && bun run db:studio", desc: "Prisma Studio" },
            { cmd: "cd backend && bun run reset-password", desc: "Reset password (local)" },
            { cmd: "cd frontend && bun run build", desc: "Production frontend build" },
          ].map((item, i) => (
            <CommandBlock key={i} label={item.desc} command={item.cmd} color="amber" />
          ))}
        </div>
      </div>
    </div>
  </section>
);
