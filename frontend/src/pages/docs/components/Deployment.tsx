import { Container, FolderTree } from "lucide-react";
import { StaticCodeBlock } from "./DocsShared";

export const Deployment = () => (
  <section id="deployment" className="scroll-mt-20 mb-12 text-left">
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
);
