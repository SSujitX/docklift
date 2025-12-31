import { Globe } from "lucide-react";

export const CustomDomains = () => (
  <section id="domains" className="scroll-mt-20 mb-12 text-left">
    <h2 className="flex items-center gap-2 text-2xl font-bold mb-4">
      <Globe className="h-6 w-6 text-cyan-500" />
      Custom Domains
    </h2>
    <p className="text-muted-foreground mb-4">
      Connect custom domains to your Docklift panel and deployed services.
    </p>

    <div className="bg-secondary/50 rounded-xl p-6 mb-6">
      <h4 className="font-semibold mb-3">Docklift Panel Domain</h4>
      <p className="text-sm text-muted-foreground mb-4">
        Access Docklift itself via custom domain instead of IP:port.
      </p>
      <ol className="list-decimal list-inside space-y-2 text-muted-foreground">
        <li>Go to <strong>Settings → Domain</strong></li>
        <li>Add your domain (e.g., <code className="bg-primary/10 px-1.5 py-0.5 rounded text-primary">panel.example.com</code>)</li>
        <li>Port defaults to <strong>8080</strong> (Docklift frontend)</li>
        <li>Configure DNS A record pointing to your server IP</li>
      </ol>
    </div>

    <div className="bg-secondary/50 rounded-xl p-6 mb-6">
      <h4 className="font-semibold mb-3">Service Domains</h4>
      <p className="text-sm text-muted-foreground mb-4">
        Assign custom domains to individual project services.
      </p>
      <ol className="list-decimal list-inside space-y-2 text-muted-foreground">
        <li>Go to <strong>Project → Domains</strong> tab</li>
        <li>Add domain for each service (e.g., <code className="bg-primary/10 px-1.5 py-0.5 rounded text-primary">api.example.com</code>)</li>
        <li>Nginx automatically routes traffic to the service</li>
      </ol>
    </div>
    
    <div className="bg-secondary/50 rounded-xl p-6 mb-6">
      <h4 className="font-semibold mb-3">DNS Configuration</h4>
      <div className="overflow-x-auto">
        <table className="w-full text-sm">
          <thead>
            <tr className="border-b border-border">
              <th className="text-left py-2 font-semibold">Type</th>
              <th className="text-left py-2 font-semibold">Name</th>
              <th className="text-left py-2 font-semibold">Value</th>
              <th className="text-left py-2 font-semibold">Usage</th>
            </tr>
          </thead>
          <tbody className="text-muted-foreground">
            <tr className="border-b border-border/50">
              <td className="py-2 font-mono text-cyan-500">A</td>
              <td className="py-2 font-mono">@</td>
              <td className="py-2">YOUR_SERVER_IP</td>
              <td className="py-2">Root domain (example.com)</td>
            </tr>
            <tr className="border-b border-border/50">
              <td className="py-2 font-mono text-cyan-500">A</td>
              <td className="py-2 font-mono">panel</td>
              <td className="py-2">YOUR_SERVER_IP</td>
              <td className="py-2">Docklift panel</td>
            </tr>
            <tr className="border-b border-border/50">
              <td className="py-2 font-mono text-cyan-500">A</td>
              <td className="py-2 font-mono">api</td>
              <td className="py-2">YOUR_SERVER_IP</td>
              <td className="py-2">API service</td>
            </tr>
            <tr>
              <td className="py-2 font-mono text-cyan-500">A</td>
              <td className="py-2 font-mono">*</td>
              <td className="py-2">YOUR_SERVER_IP</td>
              <td className="py-2">Wildcard (all subdomains)</td>
            </tr>
          </tbody>
        </table>
      </div>
    </div>

    <div className="bg-gradient-to-r from-cyan-500/10 to-blue-500/10 rounded-xl p-4 border border-cyan-500/20">
      <p className="text-sm">
        <span className="font-semibold text-cyan-500">Cloudflare:</span>{" "}
        Enable proxy (orange cloud) for SSL. Set SSL mode to <strong>Flexible</strong>.
      </p>
    </div>
  </section>
);
