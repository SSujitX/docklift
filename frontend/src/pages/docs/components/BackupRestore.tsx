import { HardDrive, Download, Upload, Shield, CheckCircle2, AlertTriangle, FolderArchive } from "lucide-react";

export const BackupRestore = () => (
  <section id="backup" className="scroll-mt-20 mb-12 text-left">
    <h2 className="flex items-center gap-2 text-2xl font-bold mb-4">
      <HardDrive className="h-6 w-6 text-cyan-500" />
      Backup & Restore
    </h2>
    <p className="text-muted-foreground mb-6">
      Back up DockLift&apos;s control database, source projects, configurations, certificates, and secrets.
      Docker named-volume contents and external databases require their own database/file backup.
    </p>

    {/* What's Included */}
    <div className="bg-secondary/40 rounded-2xl p-6 border border-border/40 mb-8">
      <h3 className="font-bold mb-4 flex items-center gap-2">
        <FolderArchive className="h-5 w-5 text-cyan-500" />
        What's Included in Backups
      </h3>
      <div className="grid grid-cols-1 md:grid-cols-2 gap-4">
        <div className="flex items-start gap-3">
          <CheckCircle2 className="h-5 w-5 text-emerald-500 mt-0.5 shrink-0" />
          <div>
            <p className="font-semibold text-sm">Database</p>
            <p className="text-xs text-muted-foreground">DockLift projects, users, settings, environment variables, and storage definitions</p>
          </div>
        </div>
        <div className="flex items-start gap-3">
          <CheckCircle2 className="h-5 w-5 text-emerald-500 mt-0.5 shrink-0" />
          <div>
            <p className="font-semibold text-sm">Project Files</p>
            <p className="text-xs text-muted-foreground">All files from /deployments directory</p>
          </div>
        </div>
        <div className="flex items-start gap-3">
          <CheckCircle2 className="h-5 w-5 text-emerald-500 mt-0.5 shrink-0" />
          <div>
            <p className="font-semibold text-sm">Nginx Configs</p>
            <p className="text-xs text-muted-foreground">Custom domain configurations</p>
          </div>
        </div>
        <div className="flex items-start gap-3">
          <CheckCircle2 className="h-5 w-5 text-emerald-500 mt-0.5 shrink-0" />
          <div>
            <p className="font-semibold text-sm">GitHub App Key</p>
            <p className="text-xs text-muted-foreground">github-app.pem (if configured)</p>
          </div>
        </div>
      </div>
    </div>

    {/* Create Backup */}
    <div className="space-y-6 mb-8">
      <div className="bg-secondary/50 rounded-xl p-6">
        <h4 className="font-semibold mb-3 flex items-center gap-2 text-emerald-500">
          <Download className="h-5 w-5" />
          Creating a Backup
        </h4>
        <ol className="list-decimal list-inside text-sm text-muted-foreground space-y-3">
          <li>Go to <span className="text-foreground font-bold">Settings</span> in the sidebar</li>
          <li>Click the <span className="text-foreground font-bold">Backup</span> tab</li>
          <li>Click <span className="text-foreground font-bold">Create Backup</span></li>
          <li>Wait for the backup to complete (progress shown in real-time)</li>
          <li>Download the backup file (.zip) to your local machine</li>
        </ol>
        <div className="mt-4 p-3 bg-cyan-500/5 border border-cyan-500/20 rounded-lg">
          <p className="text-xs text-muted-foreground">
            <span className="font-bold text-cyan-500">Tip:</span> Backups are stored on the server in <code className="bg-background/50 px-1.5 py-0.5 rounded text-[11px]">/data/backups/</code>. Download important backups to keep them safe.
          </p>
        </div>
      </div>
    </div>

    {/* Restore Backup */}
    <div className="space-y-6 mb-8">
      <div className="bg-secondary/50 rounded-xl p-6 border border-amber-500/20">
        <h4 className="font-semibold mb-3 flex items-center gap-2 text-amber-500">
          <Upload className="h-5 w-5" />
          Restoring from Backup
        </h4>

        <div className="bg-amber-500/5 border border-amber-500/20 rounded-xl p-4 mb-4">
          <div className="flex items-start gap-3">
            <AlertTriangle className="h-5 w-5 text-amber-500 shrink-0 mt-0.5" />
            <div>
              <p className="font-bold text-amber-600 dark:text-amber-500 text-sm mb-1">Warning</p>
              <p className="text-xs text-muted-foreground">
                Restoring will replace ALL current data. Running containers will be stopped. Make sure you have a backup of your current state if needed.
              </p>
            </div>
          </div>
        </div>

        <ol className="list-decimal list-inside text-sm text-muted-foreground space-y-3">
          <li>Go to <span className="text-foreground font-bold">Settings → Restore</span></li>
          <li>Upload a backup .zip or pick a previously uploaded file</li>
          <li>Confirm with your <span className="text-foreground font-bold">account password</span> (step-up auth)</li>
          <li>Wait for the restore stream (API is locked while maintenance runs)</li>
          <li>Backend restarts; projects are reconciled automatically when possible</li>
        </ol>
      </div>
    </div>

    {/* Fresh Install Restore */}
    <div className="bg-secondary/40 rounded-2xl p-6 border border-border/40 mb-8">
      <h3 className="font-bold mb-4 flex items-center gap-2">
        <Shield className="h-5 w-5 text-cyan-500" />
        Restoring on a Fresh Installation
      </h3>
      <p className="text-sm text-muted-foreground mb-4">
        You can restore a backup immediately after installing Docklift on a new server, before creating any users:
      </p>
      <ol className="list-decimal list-inside text-sm text-muted-foreground space-y-2">
        <li>Install Docklift on your new server (note the Setup code from the installer)</li>
        <li>Open <code className="bg-primary/10 px-1.5 py-0.5 rounded text-primary">http://SERVER_IP:8080</code></li>
        <li>Enter the Setup code, then choose <span className="text-foreground font-bold">Restore from Backup</span></li>
        <li>Upload your backup file (no admin password yet — setup token authorizes this once)</li>
        <li>Setup code is consumed only after a full successful restore (admin present + reconcile OK)</li>
        <li>If restore fails or is incomplete, the DB rolls back and the setup code stays valid for retry</li>
        <li>If DB rollback itself fails, DockLift seals recovery (`data/.restore-critical`) — do not retry until repaired and cleared</li>
        <li>If DB rollback itself fails, DockLift seals critical mode (<code className="bg-primary/10 px-1 rounded">.restore-critical</code>) until an admin clears it after manual repair</li>
      </ol>
      <div className="mt-4 p-3 bg-emerald-500/5 border border-emerald-500/20 rounded-lg">
        <p className="text-xs text-muted-foreground">
          <span className="font-bold text-emerald-500">Note:</span> This allows migration between servers without losing any data or needing to reconfigure everything.
        </p>
      </div>
    </div>

    {/* Best Practices */}
    <div className="bg-gradient-to-br from-cyan-500/5 to-violet-500/5 rounded-2xl p-6 border border-cyan-500/10">
      <h3 className="font-bold mb-4">Best Practices</h3>
      <ul className="space-y-3 text-sm text-muted-foreground">
        <li className="flex gap-3">
          <CheckCircle2 className="h-4 w-4 text-cyan-500 mt-0.5 shrink-0" />
          <span>Create backups before major changes or updates</span>
        </li>
        <li className="flex gap-3">
          <CheckCircle2 className="h-4 w-4 text-cyan-500 mt-0.5 shrink-0" />
          <span>Download and store backups off-server (local machine, cloud storage)</span>
        </li>
        <li className="flex gap-3">
          <CheckCircle2 className="h-4 w-4 text-cyan-500 mt-0.5 shrink-0" />
          <span>Test restore on a staging server before production migration</span>
        </li>
        <li className="flex gap-3">
          <CheckCircle2 className="h-4 w-4 text-cyan-500 mt-0.5 shrink-0" />
          <span>Delete old backups from the server to save disk space</span>
        </li>
      </ul>
    </div>
  </section>
);
