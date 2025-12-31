import { Github } from "lucide-react";

export const GithubIntegration = () => (
  <section id="github" className="scroll-mt-20 mb-12 text-left">
    <h2 className="flex items-center gap-2 text-2xl font-bold mb-4">
      <Github className="h-6 w-6 text-cyan-500" />
      GitHub Integration
    </h2>
    <p className="text-muted-foreground mb-4">
      Connect your GitHub account to deploy directly from public and private repositories.
    </p>

    <div className="bg-secondary/50 rounded-xl p-6 mb-4">
      <h4 className="font-semibold mb-3 text-emerald-500">🔗 How to Connect (Install)</h4>
      <ol className="list-decimal list-inside space-y-2 text-muted-foreground">
        <li>Go to <strong>Settings → GitHub</strong></li>
        <li>Click <strong>Connect GitHub</strong></li>
        <li>Enter an app name (e.g., <code className="bg-primary/10 px-1.5 py-0.5 rounded text-primary">my-docklift</code>)</li>
        <li>Click <strong>Create GitHub App</strong></li>
        <li>You&apos;ll be redirected to GitHub to create the app</li>
        <li>Select which repositories to grant access</li>
        <li>Click <strong>Install</strong> on GitHub</li>
        <li>Docklift auto-detects the installation ✅</li>
      </ol>
    </div>

    <div className="bg-secondary/50 rounded-xl p-6 mb-4">
      <h4 className="font-semibold mb-3 text-red-500">🗑️ How to Disconnect (Uninstall)</h4>
      <ol className="list-decimal list-inside space-y-2 text-muted-foreground">
        <li>Go to <strong>Settings → GitHub</strong></li>
        <li>Click <strong>Connect GitHub</strong> (opens dialog)</li>
        <li>Click <strong>Disconnect GitHub</strong> button</li>
      </ol>
      <div className="mt-4 p-3 bg-amber-500/10 rounded-lg border border-amber-500/20">
        <p className="text-sm text-amber-600 dark:text-amber-400">
          <strong>Note:</strong> To fully remove, also uninstall the app from GitHub:<br/>
          GitHub → Settings → Applications → Installed GitHub Apps → Configure → Uninstall
        </p>
      </div>
    </div>

    <div className="grid gap-4 md:grid-cols-2">
      <div className="bg-secondary/50 rounded-xl p-4">
        <h4 className="font-semibold text-cyan-500">Features</h4>
        <ul className="list-disc list-inside text-sm text-muted-foreground mt-2 space-y-1">
          <li>List all accessible repositories</li>
          <li>Select branch to deploy</li>
          <li>Auto-pull on redeploy</li>
          <li>Private repository support</li>
          <li>No manual token creation needed</li>
        </ul>
      </div>
      <div className="bg-secondary/50 rounded-xl p-4">
        <h4 className="font-semibold text-violet-500">API Endpoints</h4>
        <ul className="list-disc list-inside text-sm text-muted-foreground mt-2 space-y-1">
          <li><code>GET /api/github/app-status</code></li>
          <li><code>POST /api/github/manifest</code></li>
          <li><code>POST /api/github/check-installation</code></li>
          <li><code>GET /api/github/repos</code></li>
          <li><code>GET /api/github/repos/:owner/:repo/branches</code></li>
        </ul>
      </div>
    </div>
  </section>
);
