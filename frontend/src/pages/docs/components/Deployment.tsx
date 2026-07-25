import { Container, FolderTree } from "lucide-react";
import { StaticCodeBlock } from "./DocsShared";

export const Deployment = () => (
  <section id="deployment" className="scroll-mt-20 mb-12 text-left">
    <h2 className="flex items-center gap-2 text-2xl font-bold mb-4">
      <Container className="h-6 w-6 text-cyan-500" />
      Deployment
    </h2>
    <p className="text-muted-foreground mb-4">
      Docklift prefers your Dockerfile and automatically falls back to Railpack when one is not present.
    </p>
    
    <div className="grid gap-4 md:grid-cols-2 lg:grid-cols-4 mb-6">
      <div className="bg-secondary/50 rounded-xl p-4">
        <h4 className="font-semibold text-cyan-500">Deploy</h4>
        <p className="text-sm text-muted-foreground">Detect, build, and start a new image</p>
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
        <p className="text-sm text-muted-foreground">Recreate from the last successfully built image</p>
      </div>
    </div>

    <div className="bg-secondary/50 rounded-xl p-6 mb-4">
      <h4 className="font-semibold mb-3">Deployment Process</h4>
      <ol className="list-decimal list-inside space-y-2 text-muted-foreground">
        <li>Pull latest code from GitHub (if applicable)</li>
        <li>Use a repository Dockerfile, or let Railpack detect the framework</li>
        <li>Build a tagged image (BuildKit secrets for marked build vars)</li>
        <li>Write runtime compose under deployments/.docklift/&lt;id&gt;/ on a per-project network</li>
        <li>Start containers (host ports only if Publish host ports is enabled)</li>
        <li>Attach nginx-proxy to the project network; update domain vhosts</li>
        <li>Stream logs to the browser in real time</li>
      </ol>
      <p className="text-xs text-muted-foreground mt-3">
        Cancel anytime tears containers down for a fresh start. Past success/failed history is not rewritten.
        Partial fleets show status <strong>degraded</strong>.
      </p>
    </div>

    <div className="bg-secondary/50 rounded-xl p-6">
      <h4 className="font-semibold mb-3">Multi-Service Projects</h4>
      <p className="text-sm text-muted-foreground mb-4">
        Dockerfile projects can contain multiple services. Railpack projects currently resolve to one application service.
        Prefer custom domains; host ports are optional.
      </p>
      <StaticCodeBlock 
        title="Project Structure"
        icon={FolderTree}
        color="blue"
        code={`my-project/
├── api/
│   └── Dockerfile      # → domain or opt-in host port
├── frontend/
│   └── Dockerfile
└── worker/
    └── Dockerfile`}
      />
    </div>
  </section>
);
