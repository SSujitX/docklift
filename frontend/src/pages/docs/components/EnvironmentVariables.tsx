import { Shield, Key } from "lucide-react";

export const EnvironmentVariables = () => (
  <section id="env" className="scroll-mt-20 mb-12 text-left">
    <h2 className="flex items-center gap-2 text-2xl font-bold mb-4">
      <Shield className="h-6 w-6 text-cyan-500" />
      Environment Variables
    </h2>
    <p className="text-muted-foreground mb-4">
      Securely manage environment variables and secrets for your applications.
    </p>

    <div className="bg-secondary/50 rounded-xl p-6 mb-4 border border-cyan-500/10">
      <h4 className="font-semibold mb-3 flex items-center gap-2 text-cyan-500">
        <Key className="h-4 w-4" />
        Security First
      </h4>
      <p className="text-sm text-muted-foreground mb-4">
        Environment variables are stored in DockLift&apos;s database and injected directly into the container at runtime.
      </p>
      
      <div className="bg-zinc-900 rounded-xl p-4 border border-white/5 font-mono text-xs">
        <div className="flex items-center gap-2 mb-2">
          <span className="text-emerald-500">DATABASE_URL</span>
          <span className="text-zinc-600">=</span>
          <span className="text-cyan-500">postgresql://user:pass@host:5432/db</span>
        </div>
        <div className="flex items-center gap-2">
          <span className="text-emerald-500">API_KEY</span>
          <span className="text-zinc-600">=</span>
          <span className="text-cyan-500">sk_live_51Mzh...</span>
        </div>
      </div>
    </div>

    <div className="bg-secondary/50 rounded-xl p-6 mb-4">
      <h4 className="font-semibold mb-3">Database and File Persistence</h4>
      <ul className="list-disc list-inside text-sm text-muted-foreground space-y-2">
        <li>An external PostgreSQL/MySQL connection in <code className="bg-primary/10 px-1.5 py-0.5 rounded text-primary">DATABASE_URL</code> is not replaced during image builds.</li>
        <li>SQLite databases and uploaded files inside the container need a named mount from the project&apos;s Storage tab.</li>
        <li>Persistent volumes survive deploy, redeploy, stop, restart, and image replacement.</li>
        <li>Application migrations can still alter database contents; persistence is not a substitute for backups.</li>
      </ul>
    </div>

    <div className="bg-secondary/50 rounded-xl p-6">
      <h4 className="font-semibold mb-3">Usage Guidelines</h4>
      <ul className="list-disc list-inside text-sm text-muted-foreground space-y-2">
        <li>Keys must be unique per project; invalid names are rejected</li>
        <li>Mark as <strong>runtime</strong> and/or <strong>build</strong> as needed</li>
        <li>
          <strong>BuildKit secret</strong> (with build): passed as a Docker secret, not{" "}
          <code className="bg-primary/10 px-1.5 py-0.5 rounded text-primary">--build-arg</code>
          — your Dockerfile should use{" "}
          <code className="bg-primary/10 px-1.5 py-0.5 rounded text-primary">RUN --mount=type=secret,id=KEY</code>
        </li>
        <li>Accessible via <code className="bg-primary/10 px-1.5 py-0.5 rounded text-primary">process.env</code> (Node.js) or equivalent at runtime</li>
        <li>Update variables and <strong>Deploy</strong> for changes to take effect</li>
      </ul>
    </div>
  </section>
);
