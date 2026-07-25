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
        <h4 className="font-semibold mb-2 text-red-500">Can&apos;t open SERVER_IP:55xx</h4>
        <ol className="list-decimal list-inside text-sm text-muted-foreground space-y-1">
          <li>Host ports are off by default — use your custom domain, or enable <strong>Publish host ports</strong> in Build Settings and redeploy</li>
          <li>Check <code className="bg-primary/10 px-1.5 py-0.5 rounded text-primary">/ports</code> for reserved host ports</li>
        </ol>
      </div>

      <div className="bg-secondary/50 rounded-xl p-6">
        <h4 className="font-semibold mb-2 text-red-500">Domain returns 502</h4>
        <ol className="list-decimal list-inside text-sm text-muted-foreground space-y-1">
          <li>Confirm the app container is running and listening on its internal port</li>
          <li>Verify DNS: <code className="bg-primary/10 px-1.5 py-0.5 rounded text-primary">nslookup yourdomain.com</code></li>
          <li>Ensure nginx-proxy is on the project network after deploy</li>
          <li>Reload proxy: <code className="bg-primary/10 px-1.5 py-0.5 rounded text-primary">docker exec docklift-nginx-proxy nginx -s reload</code></li>
          <li>Cloudflare: SSL mode <strong>Full (strict)</strong> once HTTPS is active</li>
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
