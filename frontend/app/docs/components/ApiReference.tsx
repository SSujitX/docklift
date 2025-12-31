import { Code } from "lucide-react";

export const ApiReference = () => (
  <section id="api" className="scroll-mt-20 mb-12 text-left">
    <h2 className="flex items-center gap-2 text-2xl font-bold mb-4">
      <Code className="h-6 w-6 text-cyan-500" />
      API Reference
    </h2>
    <p className="text-muted-foreground mb-4">
      Backend API runs on port <code className="bg-primary/10 px-1.5 py-0.5 rounded text-primary">4000</code> by default.
    </p>

    <div className="space-y-6">
      <div className="bg-secondary/50 rounded-xl p-6">
        <h4 className="font-semibold mb-3 text-cyan-500">Projects API</h4>
        <div className="overflow-x-auto">
          <table className="w-full text-sm">
            <thead>
              <tr className="border-b border-border">
                <th className="text-left py-2 font-semibold w-20">Method</th>
                <th className="text-left py-2 font-semibold">Endpoint</th>
                <th className="text-left py-2 font-semibold">Description</th>
              </tr>
            </thead>
            <tbody className="text-muted-foreground font-mono text-xs">
              <tr className="border-b border-border/50">
                <td className="py-2 text-emerald-500">GET</td>
                <td className="py-2">/api/projects</td>
                <td className="py-2 font-sans">List all projects</td>
              </tr>
              <tr className="border-b border-border/50">
                <td className="py-2 text-emerald-500">GET</td>
                <td className="py-2">/api/projects/:id</td>
                <td className="py-2 font-sans">Get project details</td>
              </tr>
              <tr className="border-b border-border/50">
                <td className="py-2 text-blue-500">POST</td>
                <td className="py-2">/api/projects</td>
                <td className="py-2 font-sans">Create project</td>
              </tr>
              <tr className="border-b border-border/50">
                <td className="py-2 text-amber-500">PATCH</td>
                <td className="py-2">/api/projects/:id</td>
                <td className="py-2 font-sans">Update project</td>
              </tr>
              <tr>
                <td className="py-2 text-red-500">DELETE</td>
                <td className="py-2">/api/projects/:id</td>
                <td className="py-2 font-sans">Delete project</td>
              </tr>
            </tbody>
          </table>
        </div>
      </div>

      <div className="bg-secondary/50 rounded-xl p-6">
        <h4 className="font-semibold mb-3 text-violet-500">Deployments API</h4>
        <div className="overflow-x-auto">
          <table className="w-full text-sm">
            <thead>
              <tr className="border-b border-border">
                <th className="text-left py-2 font-semibold w-20">Method</th>
                <th className="text-left py-2 font-semibold">Endpoint</th>
                <th className="text-left py-2 font-semibold">Description</th>
              </tr>
            </thead>
            <tbody className="text-muted-foreground font-mono text-xs">
              <tr className="border-b border-border/50">
                <td className="py-2 text-blue-500">POST</td>
                <td className="py-2">/api/deployments/:projectId/deploy</td>
                <td className="py-2 font-sans">Deploy project (streams logs)</td>
              </tr>
              <tr className="border-b border-border/50">
                <td className="py-2 text-blue-500">POST</td>
                <td className="py-2">/api/deployments/:projectId/stop</td>
                <td className="py-2 font-sans">Stop containers</td>
              </tr>
              <tr className="border-b border-border/50">
                <td className="py-2 text-blue-500">POST</td>
                <td className="py-2">/api/deployments/:projectId/restart</td>
                <td className="py-2 font-sans">Restart containers</td>
              </tr>
              <tr>
                <td className="py-2 text-blue-500">POST</td>
                <td className="py-2">/api/deployments/:projectId/redeploy</td>
                <td className="py-2 font-sans">Rebuild and redeploy</td>
              </tr>
            </tbody>
          </table>
        </div>
      </div>

      <div className="bg-secondary/50 rounded-xl p-6">
        <h4 className="font-semibold mb-3 text-emerald-500">System API</h4>
        <div className="overflow-x-auto">
          <table className="w-full text-sm">
            <thead>
              <tr className="border-b border-border">
                <th className="text-left py-2 font-semibold w-20">Method</th>
                <th className="text-left py-2 font-semibold">Endpoint</th>
                <th className="text-left py-2 font-semibold">Description</th>
              </tr>
            </thead>
            <tbody className="text-muted-foreground font-mono text-xs">
              <tr className="border-b border-border/50">
                <td className="py-2 text-emerald-500">GET</td>
                <td className="py-2">/api/system/stats</td>
                <td className="py-2 font-sans">Full system stats</td>
              </tr>
              <tr className="border-b border-border/50">
                <td className="py-2 text-emerald-500">GET</td>
                <td className="py-2">/api/system/ip</td>
                <td className="py-2 font-sans">Server public IP</td>
              </tr>
              <tr className="border-b border-border/50">
                <td className="py-2 text-blue-500">POST</td>
                <td className="py-2">/api/system/purge</td>
                <td className="py-2 font-sans">Clean Docker caches</td>
              </tr>
              <tr className="border-b border-border/50">
                <td className="py-2 text-blue-500">POST</td>
                <td className="py-2">/api/system/reboot</td>
                <td className="py-2 font-sans">Reboot server</td>
              </tr>
              <tr>
                <td className="py-2 text-blue-500">POST</td>
                <td className="py-2">/api/system/execute</td>
                <td className="py-2 font-sans">Execute shell command</td>
              </tr>
            </tbody>
          </table>
        </div>
      </div>
    </div>
  </section>
);
