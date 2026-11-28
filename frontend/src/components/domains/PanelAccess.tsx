// "Only allow access through the domain" (Settings → Domain): the switch
// under a panel domain, the one-click suggestion shown when the page is
// viewed through an HTTPS panel domain, and the step-up password dialog that
// turns the restriction on.
import { useState } from "react";
import { Loader2, Lock, ShieldCheck, Terminal, X } from "lucide-react";
import { toast } from "sonner";
import { Button } from "@/components/ui/button";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { Input } from "@/components/ui/input";
import { authFetch } from "@/lib/auth";
import {
  dashboardIPURL,
  PANEL_ACCESS_RECOVERY_COMMAND,
  panelAccessSwitchState,
  type PanelAccess,
} from "@/lib/panelAccess";
import { API_URL, cn } from "@/lib/utils";

async function putAccess(domain: string, body: { domainOnly: boolean; password?: string }) {
  const res = await authFetch(`${API_URL}/api/domains/${encodeURIComponent(domain)}/access`, {
    method: "PUT",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(body),
  });
  const data = await res.json().catch(() => ({}));
  if (!res.ok) throw new Error(data.error || "Failed to change dashboard access");
}

/**
 * The enable dialog and the disable call, shared by every switch and the
 * suggestion of one card. onChanged refreshes the domain list.
 */
export function usePanelAccess(serverIP: string, onChanged: () => unknown) {
  const [target, setTarget] = useState<string | null>(null);
  const [password, setPassword] = useState("");
  const [busy, setBusy] = useState(false);

  const close = () => {
    setTarget(null);
    setPassword("");
  };

  const enable = async () => {
    if (!target || !password.trim()) return;
    setBusy(true);
    try {
      await putAccess(target, { domainOnly: true, password });
      toast.success(`Dashboard now only answers at https://${target}`, {
        description: `${dashboardIPURL(serverIP)} no longer works.`,
      });
      close();
    } catch (err: any) {
      toast.error(err?.message || "Failed to change dashboard access");
    } finally {
      setBusy(false);
      // Also after a failure: the switch must show what the server has.
      await Promise.resolve(onChanged());
    }
  };

  const disable = async (domain: string) => {
    setBusy(true);
    try {
      await putAccess(domain, { domainOnly: false });
      toast.success(`Access by ${dashboardIPURL(serverIP)} is allowed again`);
    } catch (err: any) {
      toast.error(err?.message || "Failed to change dashboard access");
    } finally {
      setBusy(false);
      await Promise.resolve(onChanged());
    }
  };

  const dialog = (
    <Dialog open={target !== null} onOpenChange={(open) => !open && !busy && close()}>
      <DialogContent className="bg-background border-border shadow-2xl sm:max-w-md">
        <DialogHeader className="space-y-3">
          <div className="mx-auto mb-2 flex h-12 w-12 items-center justify-center rounded-2xl bg-brand/10">
            <ShieldCheck className="h-6 w-6 text-brand" />
          </div>
          <DialogTitle className="text-center text-xl font-bold tracking-tight">
            Only allow access through the domain
          </DialogTitle>
          <DialogDescription className="text-center text-sm text-muted-foreground">
            <span className="font-mono text-foreground break-all">{dashboardIPURL(serverIP)}</span> stops
            working at once: browsers are sent to{" "}
            <span className="font-mono text-foreground break-all">https://{target}</span>, API and terminal
            requests on that port are refused.
          </DialogDescription>
        </DialogHeader>

        <div className="my-2 min-w-0 space-y-2 rounded-xl border border-border bg-secondary/30 p-3">
          <p className="flex items-center gap-2 text-xs font-semibold">
            <Terminal className="h-3.5 w-3.5 shrink-0 text-muted-foreground" />
            If the domain ever stops working, run on the server:
          </p>
          <code className="block rounded bg-background px-2 py-1.5 text-[11px] break-all select-all">
            {PANEL_ACCESS_RECOVERY_COMMAND}
          </code>
        </div>

        <form
          onSubmit={(e) => {
            e.preventDefault();
            void enable();
          }}
        >
          <div className="mb-2 space-y-2">
            <label htmlFor="panel-access-password" className="text-xs font-semibold text-muted-foreground">
              Confirm with account password
            </label>
            <Input
              id="panel-access-password"
              type="password"
              value={password}
              onChange={(e) => setPassword(e.target.value)}
              placeholder="Your DockLift password"
              autoComplete="current-password"
              className="rounded-xl"
            />
          </div>
          <DialogFooter className="flex flex-col gap-2 pt-2 sm:flex-row">
            <Button type="button" variant="ghost" onClick={close} disabled={busy} className="flex-1 rounded-xl font-bold">
              Cancel
            </Button>
            <Button type="submit" disabled={busy || !password.trim()} className="flex-1 rounded-xl font-bold">
              {busy && <Loader2 className="mr-2 h-4 w-4 animate-spin" />}
              Turn on
            </Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  );

  return { busy, requestEnable: setTarget, disable, dialog };
}

/** The switch shown directly under a panel domain (or alone, disabled, when there is none). */
export function PanelAccessSwitch({
  domain,
  access,
  serverIP,
  busy,
  onEnable,
  onDisable,
}: {
  domain: string | null;
  access?: PanelAccess;
  serverIP: string;
  busy: boolean;
  onEnable: (domain: string) => void;
  onDisable: (domain: string) => void;
}) {
  const state = panelAccessSwitchState(domain ? access : undefined, busy);
  const id = `panel-access-${domain ?? "none"}`;
  return (
    <div className="flex items-start justify-between gap-3 rounded-lg border border-border/50 bg-background/60 p-3">
      <div className="min-w-0 space-y-0.5">
        <label htmlFor={id} className="flex items-center gap-1.5 text-xs font-semibold">
          <Lock className="h-3.5 w-3.5 shrink-0 text-muted-foreground" />
          Only allow access through the domain
        </label>
        <p className="text-[11px] text-muted-foreground wrap-break-word">
          <span className="font-mono">{dashboardIPURL(serverIP)}</span> will stop working; use{" "}
          <span className="font-mono">https://{domain ?? "<domain>"}</span>. Recovery:{" "}
          <code className="font-mono">{PANEL_ACCESS_RECOVERY_COMMAND}</code>
        </p>
        {state.hint && <p className="text-[11px] text-amber-600 wrap-break-word">{state.hint}</p>}
      </div>
      <button
        id={id}
        type="button"
        role="switch"
        aria-checked={state.checked}
        disabled={state.disabled}
        onClick={() => {
          if (!domain) return;
          if (state.checked) onDisable(domain);
          else onEnable(domain);
        }}
        className={cn(
          "relative mt-0.5 inline-flex h-6 w-11 shrink-0 items-center rounded-full border transition-colors focus-visible:outline-hidden focus-visible:ring-2 focus-visible:ring-brand focus-visible:ring-offset-2 focus-visible:ring-offset-background disabled:cursor-not-allowed disabled:opacity-50",
          state.checked ? "border-brand bg-brand" : "border-border bg-secondary",
        )}
      >
        <span
          className={cn(
            "inline-block h-4 w-4 rounded-full bg-background shadow-sm transition-transform",
            state.checked ? "translate-x-6" : "translate-x-1",
          )}
        />
      </button>
    </div>
  );
}

/** One-click suggestion to restrict access to the domain the page is viewed through. */
export function PanelAccessSuggestion({
  domain,
  serverIP,
  busy,
  onEnable,
  onDismiss,
}: {
  domain: string;
  serverIP: string;
  busy: boolean;
  onEnable: (domain: string) => void;
  onDismiss: (domain: string) => void;
}) {
  return (
    <div className="flex flex-col gap-3 rounded-xl border border-brand/30 bg-brand/5 p-3 sm:flex-row sm:items-center">
      <ShieldCheck className="hidden h-5 w-5 shrink-0 text-brand sm:block" />
      <div className="min-w-0 flex-1 space-y-0.5">
        <p className="text-xs font-semibold">
          You are using <span className="font-mono break-all">https://{domain}</span>. Close the unencrypted address?
        </p>
        <p className="text-[11px] text-muted-foreground wrap-break-word">
          Turn on &ldquo;Only allow access through the domain&rdquo; so{" "}
          <span className="font-mono">{dashboardIPURL(serverIP)}</span> stops working.
        </p>
      </div>
      <div className="flex shrink-0 items-center gap-1.5">
        <Button type="button" size="sm" className="h-8 text-xs" disabled={busy} onClick={() => onEnable(domain)}>
          Turn on
        </Button>
        <Button
          type="button"
          variant="ghost"
          size="icon"
          className="h-8 w-8 text-muted-foreground"
          onClick={() => onDismiss(domain)}
          aria-label="Dismiss suggestion"
        >
          <X className="h-4 w-4" />
        </Button>
      </div>
    </div>
  );
}
