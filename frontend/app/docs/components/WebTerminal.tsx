import { Terminal } from "lucide-react";
import { TerminalWindow } from "./DocsShared";

export const WebTerminal = () => (
  <section id="terminal" className="scroll-mt-20 mb-12 text-left">
    <h2 className="flex items-center gap-2 text-2xl font-bold mb-4">
      <Terminal className="h-6 w-6 text-cyan-500" />
      Web Terminal
    </h2>
    <p className="text-muted-foreground mb-4">
      Full SSH-like terminal in your browser at <code className="bg-primary/10 px-1.5 py-0.5 rounded text-primary">/terminal</code>.
    </p>
    
    <div className="bg-secondary/50 rounded-xl p-6 mb-4">
      <h4 className="font-semibold mb-3">Features</h4>
      <ul className="list-disc list-inside text-muted-foreground space-y-1">
        <li>Full xterm.js terminal emulator</li>
        <li>Execute any shell command</li>
        <li>Quick command shortcuts (docker ps, docker stats, etc.)</li>
        <li>Command history</li>
        <li>Color-coded output</li>
      </ul>
    </div>

    <TerminalWindow 
      title="Common Docker Commands"
      items={[
        { comment: "List running containers", cmd: "docker ps" },
        { comment: "View container logs", cmd: "docker logs <container_id>" },
        { comment: "Live resource usage", cmd: "docker stats" },
        { comment: "System process monitor", cmd: "htop" }
      ]}
    />
  </section>
);
