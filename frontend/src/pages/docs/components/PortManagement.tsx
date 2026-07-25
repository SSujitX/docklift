import { Plug } from "lucide-react";

export const PortManagement = () => (
  <section id="ports" className="scroll-mt-20 mb-12 text-left">
    <h2 className="flex items-center gap-2 text-2xl font-bold mb-4">
      <Plug className="h-6 w-6 text-cyan-500" />
      Port Management
    </h2>
    <p className="text-muted-foreground mb-4">
      Host ports are <strong>opt-in</strong>. By default an app can be <strong>Running</strong> but not
      public — Project → Overview shows <strong>Not public yet</strong> until you add a domain/subdomain
      or enable Publish host ports and redeploy. Preferred path: domain on nginx-proxy
      (project network → <code className="bg-primary/10 px-1.5 py-0.5 rounded text-primary">container_name:internal_port</code>).
    </p>

    <div className="bg-secondary/50 rounded-xl p-6 mb-4">
      <h4 className="font-semibold mb-3">Publish host ports</h4>
      <p className="text-sm text-muted-foreground mb-3">
        In <strong>Project → Build Settings</strong>, enable <strong>Publish host ports</strong> and redeploy
        if you need a raw <code className="bg-primary/10 px-1.5 py-0.5 rounded text-primary">SERVER_IP:port</code> URL.
        Ports are taken from the pool below.
      </p>
      <div className="overflow-x-auto">
        <table className="w-full text-sm">
          <thead>
            <tr className="border-b border-border">
              <th className="text-left py-2 font-semibold">Setting</th>
              <th className="text-left py-2 font-semibold">Default</th>
              <th className="text-left py-2 font-semibold">Description</th>
            </tr>
          </thead>
          <tbody className="text-muted-foreground">
            <tr className="border-b border-border/50">
              <td className="py-2 font-mono text-xs">PORT_RANGE_START</td>
              <td className="py-2">5500</td>
              <td className="py-2">First host port when publish is enabled</td>
            </tr>
            <tr>
              <td className="py-2 font-mono text-xs">PORT_RANGE_END</td>
              <td className="py-2">5600</td>
              <td className="py-2">Last host port in the pool</td>
            </tr>
          </tbody>
        </table>
      </div>
    </div>

    <div className="bg-gradient-to-r from-cyan-500/10 to-blue-500/10 rounded-xl p-4 border border-cyan-500/20">
      <p className="text-sm">
        <span className="font-semibold text-cyan-500">View Allocations:</span>{" "}
        Go to <code className="bg-primary/10 px-1.5 py-0.5 rounded text-primary">/ports</code> to see host ports
        currently reserved for projects that opted in.
      </p>
    </div>
  </section>
);
