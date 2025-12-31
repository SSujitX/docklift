import { Shield } from "lucide-react";

export const Troubleshooting = () => (
  <section id="troubleshooting" className="scroll-mt-20 mb-12 text-left">
    <h2 className="flex items-center gap-2 text-2xl font-bold mb-4">
      <Shield className="h-6 w-6 text-cyan-500" />
      Troubleshooting
    </h2>
    <p className="text-muted-foreground mb-4">
      Common issues and solutions.
    </p>

    <div className="space-y-4">
      <div className="bg-secondary/50 rounded-xl p-6">
        <h4 className="font-semibold mb-2 text-red-500">Port Already in Use</h4>
        <ol className="list-decimal list-inside text-sm text-muted-foreground space-y-1">
          <li>Check <code className="bg-primary/10 px-1.5 py-0.5 rounded text-primary">/ports</code> for allocations</li>
          <li>Stop conflicting container</li>
          <li>Or redeploy to get a new port</li>
        </ol>
      </div>

      <div className="bg-secondary/50 rounded-xl p-6">
        <h4 className="font-semibold mb-2 text-red-500">Domain Not Working</h4>
        <ol className="list-decimal list-inside text-sm text-muted-foreground space-y-1">
          <li>Verify DNS: <code className="bg-primary/10 px-1.5 py-0.5 rounded text-primary">nslookup yourdomain.com</code></li>
          <li>Check Nginx config in <code className="bg-primary/10 px-1.5 py-0.5 rounded text-primary">/etc/docklift/nginx-conf/</code></li>
          <li>Reload Nginx: <code className="bg-primary/10 px-1.5 py-0.5 rounded text-primary">docker exec docklift-nginx-proxy nginx -s reload</code></li>
          <li>If using Cloudflare, ensure proxy is enabled and SSL mode is Flexible</li>
        </ol>
      </div>

      <div className="bg-secondary/50 rounded-xl p-6">
        <h4 className="font-semibold mb-2 text-red-500">Build Failures</h4>
        <ol className="list-decimal list-inside text-sm text-muted-foreground space-y-1">
          <li>Check deployment logs in Project → Deployments</li>
          <li>Verify Dockerfile syntax</li>
          <li>Ensure EXPOSE directive is present</li>
          <li>Check environment variables are set correctly</li>
        </ol>
      </div>

      <div className="bg-secondary/50 rounded-xl p-6">
        <h4 className="font-semibold mb-2 text-red-500">Container Keeps Stopping</h4>
        <ol className="list-decimal list-inside text-sm text-muted-foreground space-y-1">
          <li>Check container logs: <code className="bg-primary/10 px-1.5 py-0.5 rounded text-primary">docker logs container_name</code></li>
          <li>Ensure app doesn't exit immediately</li>
          <li>Verify port binding matches EXPOSE</li>
        </ol>
      </div>
    </div>
  </section>
);
