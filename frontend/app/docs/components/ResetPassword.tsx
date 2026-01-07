import { ShieldCheck, Shield } from "lucide-react";
import { TerminalWindow } from "./DocsShared";

export const ResetPassword = () => (
  <section id="reset-password" className="scroll-mt-20 mb-12 text-left">
    <h2 className="flex items-center gap-2 text-2xl font-bold mb-4">
      <ShieldCheck className="h-6 w-6 text-cyan-500" />
      Emergency Reset Password
    </h2>
    <p className="text-muted-foreground mb-4">
      If you lose access to your admin account, you can perform an emergency password reset using the Docklift CLI.
    </p>

    <div className="bg-amber-500/5 border border-amber-500/20 rounded-2xl p-6 mb-8">
      <div className="flex items-start gap-4">
        <div className="bg-amber-500/10 p-2.5 rounded-xl">
          <Shield className="h-5 w-5 text-amber-500" />
        </div>
        <div>
          <h4 className="font-bold text-amber-600 dark:text-amber-500 mb-1">Security Requirement</h4>
          <p className="text-sm text-muted-foreground leading-relaxed">
            For security reasons, this command must be executed directly on the server where Docklift is installed. It cannot be run from the web dashboard.
          </p>
        </div>
      </div>
    </div>

    <div className="space-y-6">
      <div className="bg-secondary/40 rounded-2xl p-6 border border-border/40">
        <h4 className="font-bold mb-4 text-sm uppercase tracking-wider text-muted-foreground/80">Reset Procedure</h4>
        <TerminalWindow
          title="SSH Reset Command"
          color="cyan"
          items={[
            { comment: "1. SSH into your server", cmd: "ssh root@your-server-ip" },
            { comment: "2. Run the reset script via Docker", cmd: "docker exec -it docklift-backend node dist/scripts/reset-password.js" }
          ]}
        />
        
        <div className="mt-6 p-4 bg-background/50 rounded-xl border border-border/20">
          <h5 className="text-sm font-bold mb-2">After Running the Command:</h5>
          <ul className="space-y-3">
            <li className="flex gap-3 text-sm text-muted-foreground">
              <div className="h-5 w-5 rounded-full bg-cyan-500/10 text-cyan-500 flex items-center justify-center shrink-0 font-bold text-[10px]">1</div>
              The CLI will generate a <span className="text-foreground font-bold">New Random Password</span> (16 characters).
            </li>
            <li className="flex gap-3 text-sm text-muted-foreground">
              <div className="h-5 w-5 rounded-full bg-cyan-500/10 text-cyan-500 flex items-center justify-center shrink-0 font-bold text-[10px]">2</div>
              Use this temporary password to log in to the Docklift dashboard.
            </li>
            <li className="flex gap-3 text-sm text-muted-foreground">
              <div className="h-5 w-5 rounded-full bg-cyan-500/10 text-cyan-500 flex items-center justify-center shrink-0 font-bold text-[10px]">3</div>
              Go to <span className="text-foreground font-bold">Profile Settings</span> and update it to a permanent password.
            </li>
          </ul>
        </div>
      </div>
    </div>
  </section>
);
