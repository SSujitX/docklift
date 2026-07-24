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
        <li><strong>Interactive Bash Session</strong>: Real-time PTY with tab completion and history</li>
        <li><strong>Root Access</strong>: Direct control over the host via Docker privileged mode</li>
        <li><strong>Full Screen Mode</strong>: Maximize terminal for better visibility</li>
        <li><strong>Clipboard Support</strong>: Select to copy, <kbd className="px-1 py-0.5 rounded bg-muted font-mono text-xs">Ctrl+C</kbd> to copy/interrupt, <kbd className="px-1 py-0.5 rounded bg-muted font-mono text-xs">Ctrl+V</kbd> to paste</li>
        <li><strong>Power Tools</strong>: `htop`, `docker`, `git`, and `nano` pre-installed</li>
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
