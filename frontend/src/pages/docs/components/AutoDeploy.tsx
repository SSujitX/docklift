import { RefreshCw, Shield, Rocket, History } from "lucide-react";

export const AutoDeploy = () => (
  <section id="autodeploy" className="scroll-mt-20 mb-12 text-left">
    <h2 className="flex items-center gap-2 text-2xl font-bold mb-4">
      <RefreshCw className="h-6 w-6 text-orange-500" />
      Auto-Deploy (Webhooks)
    </h2>
    <p className="text-muted-foreground mb-4">
      Enable automated deployments so your application rebuilds every time you push code to GitHub.
    </p>

    <div className="bg-secondary/50 rounded-xl p-6 mb-6">
      <h4 className="font-semibold mb-3">How it Works</h4>
      <p className="text-sm text-muted-foreground mb-4">
        Docklift uses GitHub webhooks to listen for <code className="bg-primary/10 px-1.5 py-0.5 rounded text-primary">push</code> events. When you push code to your selected branch, GitHub notifies your Docklift instance, which then triggers a fresh deployment automatically.
      </p>
      
      <h4 className="font-semibold mb-3 text-sm uppercase tracking-wider opacity-70">To Enable:</h4>
      <ol className="list-decimal list-inside space-y-2 text-muted-foreground">
        <li>Go to your <strong>Project Dashboard</strong></li>
        <li>Navigate to the <strong>Source</strong> configuration tab</li>
        <li>Toggle the <strong>Auto-Deploy</strong> switch to ON</li>
      </ol>
    </div>

    <div className="bg-amber-500/10 border border-amber-500/20 rounded-xl p-6 mb-6">
      <h4 className="font-semibold text-amber-600 dark:text-amber-500 mb-2 flex items-center gap-2">
        <Shield className="h-5 w-5" />
        Security (Webhook Secret)
      </h4>
      <p className="text-sm text-muted-foreground">
        When you enable Auto-Deploy, Docklift automatically generates a <strong>Webhook Secret</strong>. This secret is used to verify that incoming deployment requests actually come from GitHub, preventing unauthorized build triggers. 
        <br /><br />
        If you are using a <strong>Private GitHub App</strong> (recommended), Docklift handles this configuration automatically.
      </p>
    </div>

    <div className="grid gap-4 md:grid-cols-2">
      <div className="bg-secondary/50 rounded-xl p-5 border border-border/40">
        <h4 className="font-semibold mb-2 flex items-center gap-2">
          <Rocket className="h-4 w-4 text-cyan-500" />
          Instant Updates
        </h4>
        <p className="text-xs text-muted-foreground">No need to log in to the dashboard to deploy. Just <code className="bg-primary/10 px-1 py-0.5 rounded text-primary">git push</code> and watch it go.</p>
      </div>
      <div className="bg-secondary/50 rounded-xl p-5 border border-border/40">
        <h4 className="font-semibold mb-2 flex items-center gap-2">
          <History className="h-4 w-4 text-emerald-500" />
          Deployment History
        </h4>
        <p className="text-xs text-muted-foreground">Auto-deployments are recorded in your history as <code className="bg-primary/10 px-1 py-0.5 rounded text-primary">webhook</code> triggers for easy auditing.</p>
      </div>
    </div>
  </section>
);
