// "Only allow access through the domain" (Settings → Domain): the switch
// under a panel domain, the one-click suggestion shown when the page is
// viewed through an HTTPS panel domain, and the step-up password dialog that
// turns the restriction on.
import { useState } from "react";
import { Loader2, X } from "lucide-react";
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
      <DialogContent>
        <DialogHeader>
          <DialogTitle>Only allow access through the domain</DialogTitle>
          <DialogDescription>
            <span className="font-mono text-foreground break-all">{dashboardIPURL(serverIP)}</span> stops
            working at once: browsers are sent to{" "}
            <span className="font-mono text-foreground break-all">https://{target}</span>, API and terminal
            requests on that port are refused.
          </DialogDescription>
        </DialogHeader>

        <div className="min-w-0 space-y-1.5 rounded-md bg-secondary/60 px-3 py-2.5">
          <p className="text-[13px] font-medium">If the domain ever stops working, run on the server:</p>
          <code className="block font-mono text-xs break-all text-foreground select-all">
            {PANEL_ACCESS_RECOVERY_COMMAND}
          </code>
        </div>

        <form
          className="space-y-4"
          onSubmit={(e) => {
            e.preventDefault();
            void enable();
          }}
        >
          <div className="space-y-1.5">
            <label htmlFor="panel-access-password" className="text-[13px] font-medium">
              Confirm with account password
            </label>
            <Input
              id="panel-access-password"
              type="password"
              value={password}
              onChange={(e) => setPassword(e.target.value)}
              placeholder="Your DockLift password"
              autoComplete="current-password"
            />
          </div>
          <DialogFooter>
            <Button type="button" variant="outline" onClick={close} disabled={busy}>
              Cancel
            </Button>
            <Button type="submit" disabled={busy || !password.trim()}>
              {busy && <Loader2 className="animate-spin" />}
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
    <div className="flex items-start justify-between gap-4 rounded-md bg-secondary/60 px-3 py-2.5">
      <div className="min-w-0 space-y-1">
        <label htmlFor={id} className="block text-[13px] font-medium leading-5">
          Only allow access through the domain
        </label>
        <p className="text-xs leading-[1.125rem] text-muted-foreground wrap-break-word">
          <span className="font-mono">{dashboardIPURL(serverIP)}</span> will stop working; use{" "}
          <span className="font-mono">https://{domain ?? "<domain>"}</span>. Recovery:{" "}
          <code className="font-mono text-foreground">{PANEL_ACCESS_RECOVERY_COMMAND}</code>
        </p>
        {state.hint && <p className="text-xs leading-[1.125rem] text-warn wrap-break-word">{state.hint}</p>}
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
          "relative mt-0.5 inline-flex h-5 w-9 shrink-0 items-center rounded-full border transition-colors duration-150 disabled:cursor-not-allowed disabled:opacity-50 pointer-coarse:after:absolute pointer-coarse:after:-inset-2.5",
          state.checked ? "border-brand bg-brand" : "border-input bg-card",
        )}
      >
        <span
          className={cn(
            "inline-block size-3.5 rounded-full transition-[transform,background-color] duration-150",
            state.checked ? "translate-x-[1.0625rem] bg-brand-foreground" : "translate-x-[0.1875rem] bg-subtle",
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
    <div className="flex flex-col gap-3 rounded-md border border-info/40 bg-secondary/60 px-3 py-2.5 sm:flex-row sm:items-center">
      <div className="min-w-0 flex-1 space-y-0.5">
        <p className="text-[13px] font-medium leading-5">
          You are using <span className="font-mono break-all">https://{domain}</span>. Close the unencrypted address?
        </p>
        <p className="text-xs leading-[1.125rem] text-muted-foreground wrap-break-word">
          Turn on &ldquo;Only allow access through the domain&rdquo; so{" "}
          <span className="font-mono">{dashboardIPURL(serverIP)}</span> stops working.
        </p>
      </div>
      <div className="flex shrink-0 items-center gap-1">
        <Button type="button" variant="outline" size="sm" disabled={busy} onClick={() => onEnable(domain)}>
          Turn on
        </Button>
        <Button
          type="button"
          variant="ghost"
          size="icon"
          className="text-subtle"
          onClick={() => onDismiss(domain)}
          aria-label="Dismiss suggestion"
        >
          <X />
        </Button>
      </div>
    </div>
  );
}
