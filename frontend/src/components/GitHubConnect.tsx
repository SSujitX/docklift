// GitHubConnect component - GitHub App creation, installation, and connection flow

import { useState, useRef, useEffect } from "react";
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
import { Loader2, ExternalLink } from "lucide-react";
import { GithubIcon } from "./icons/GithubIcon";
import { getAuthHeaders, startGithubInstallAndNavigate } from "@/lib/auth";

const API_URL = import.meta.env.VITE_API_URL || "";

interface GitHubAppStatus {
  configured: boolean;
  installed: boolean;
  appId: string | null;
  appName: string | null;
  appSlug: string | null;
  installationId: string | null;
  username: string | null;
  avatarUrl: string | null;
  installUrl: string | null;
}

interface GitHubConnectProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  onConnected?: () => void;
}

export function GitHubConnect({ open, onOpenChange, onConnected }: GitHubConnectProps) {
  const [appName, setAppName] = useState("");
  const [loading, setLoading] = useState(false);
  const [status, setStatus] = useState<GitHubAppStatus | null>(null);
  const formRef = useRef<HTMLFormElement>(null);

  // Check current GitHub App status
  useEffect(() => {
    if (open) {
      fetchStatus();
    }
  }, [open]);

  const fetchStatus = async () => {
    try {
      const res = await fetch(`${API_URL}/api/github/app-status`, { headers: getAuthHeaders() });
      if (res.ok) {
        const data = await res.json();
        setStatus(data);
      }
    } catch (error) {
      console.error("Failed to fetch GitHub status:", error);
    }
  };

  const handleCreateApp = async () => {
    if (!appName.trim()) return;

    setLoading(true);
    try {
      const res = await fetch(`${API_URL}/api/github/manifest`, {
        method: "POST",
        credentials: "same-origin",
        headers: { "Content-Type": "application/json", ...getAuthHeaders() },
        body: JSON.stringify({ 
          appName: appName.trim(),
          returnUrl: window.location.href
        }),
      });

      if (!res.ok) {
        throw new Error("Failed to generate manifest");
      }

      const data = await res.json();
      
      // Create and submit form to GitHub
      const form = document.createElement("form");
      form.method = "POST";
      form.action = data.action;
      form.target = "_self";

      const input = document.createElement("input");
      input.type = "hidden";
      input.name = "manifest";
      input.value = data.manifest;

      form.appendChild(input);
      document.body.appendChild(form);
      form.submit();
    } catch (error) {
      console.error("Failed to create GitHub App:", error);
      setLoading(false);
    }
  };

  const handleInstall = async () => {
    setLoading(true);
    setCheckMessage(null);
    try {
      await startGithubInstallAndNavigate(window.location.href);
      setIsPolling(true);
    } catch (error) {
      console.error("Failed to start GitHub install:", error);
      setCheckMessage(error instanceof Error ? error.message : "Failed to start GitHub install");
    } finally {
      setLoading(false);
    }
  };

  const handleAddAccount = async () => {
    setLoading(true);
    try {
      await startGithubInstallAndNavigate(window.location.href);
    } catch (error) {
      console.error("Failed to start GitHub install:", error);
    } finally {
      setLoading(false);
    }
  };

  const handleDisconnect = async () => {
    setLoading(true);
    setIsPolling(false);
    try {
      const res = await fetch(`${API_URL}/api/github/disconnect`, {
        method: "POST",
        headers: getAuthHeaders(),
      });
      if (!res.ok) {
        const data = await res.json().catch(() => ({}));
        throw new Error((data as { error?: string }).error || "Disconnect failed");
      }
      setStatus(null);
      setAppName("");
    } catch (error) {
      console.error("Failed to disconnect:", error);
    } finally {
      setLoading(false);
    }
  };

  const [checkMessage, setCheckMessage] = useState<string | null>(null);
  const [isPolling, setIsPolling] = useState(false);
  const isLocalhost = typeof window !== 'undefined' && window.location.hostname === 'localhost';

  // Auto-poll for installation status when polling is enabled
  useEffect(() => {
    if (!isPolling || !status?.configured) return;

    const pollInterval = setInterval(async () => {
      try {
        const res = await fetch(`${API_URL}/api/github/check-installation`, { method: "POST", headers: getAuthHeaders() });
        const data = await res.json();
        if (data.found) {
          setIsPolling(false);
          await fetchStatus();
          onConnected?.();
        }
      } catch {
        // Ignore errors during polling
      }
    }, 3000); // Poll every 3 seconds

    return () => clearInterval(pollInterval);
  }, [isPolling, status?.configured, onConnected]);

  const handleCheckInstallation = async () => {
    setLoading(true);
    setCheckMessage(null);
    try {
      const res = await fetch(`${API_URL}/api/github/check-installation`, { method: "POST", headers: getAuthHeaders() });
      const data = await res.json();
      if (data.found) {
        setIsPolling(false);
        // Refresh status
        await fetchStatus();
        onConnected?.();
      } else {
        setCheckMessage(data.message || "No installation found. Please install the app on GitHub first.");
      }
    } catch (error) {
      console.error("Failed to check installation:", error);
      setCheckMessage("Failed to check installation. Please try again.");
    } finally {
      setLoading(false);
    }
  };

  const handleStopPolling = () => {
    setIsPolling(false);
  };

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent>
        <DialogHeader>
          <DialogTitle>
            {status?.configured ? "GitHub App connected" : "Connect GitHub App"}
          </DialogTitle>
          <DialogDescription>
            {status?.configured
              ? "Your GitHub App is configured. You can now deploy private repositories."
              : "Create a GitHub App to deploy public and private repositories."}
          </DialogDescription>
        </DialogHeader>

        {/* Already configured and installed */}
        {status?.configured && status?.installed && (
          <div className="space-y-4">
            <dl className="divide-y divide-border border-y border-border text-[13px]">
              <div className="flex items-center justify-between gap-4 py-2.5">
                <dt className="text-muted-foreground">Status</dt>
                <dd className="inline-flex items-center gap-1.5 font-medium">
                  <span aria-hidden="true" className="size-2 rounded-full bg-ok" />
                  Connected
                </dd>
              </div>
              <div className="flex items-center justify-between gap-4 py-2.5">
                <dt className="text-muted-foreground">App</dt>
                <dd className="min-w-0 truncate font-mono text-xs">{status.appName}</dd>
              </div>
              <div className="flex items-center justify-between gap-4 py-2.5">
                <dt className="text-muted-foreground">Account</dt>
                <dd className="min-w-0 truncate font-mono text-xs">@{status.username}</dd>
              </div>
            </dl>

            <div className="rounded-md border border-info/40 bg-secondary/60 px-3 py-2.5 text-[13px] leading-5">
              <p className="font-medium">Multi-account support</p>
              <p className="mt-0.5 text-muted-foreground">
                You can install this app on multiple accounts (personal and organizations). All
                repositories will appear together in one list.
              </p>
            </div>

            <DialogFooter>
              <Button
                variant="outline"
                className="text-danger sm:mr-auto"
                onClick={handleDisconnect}
                disabled={loading}
              >
                {loading ? <Loader2 className="animate-spin" /> : null}
                Disconnect GitHub
              </Button>
              <Button variant="outline" onClick={handleAddAccount} disabled={loading}>
                <ExternalLink className="text-subtle" />
                Add another account
              </Button>
            </DialogFooter>
          </div>
        )}

        {/* Configured but not installed */}
        {status?.configured && !status?.installed && (
          <div className="space-y-4">
            {/* Show polling state */}
            {isPolling ? (
              <>
                <div className="flex items-start gap-2.5 border-y border-border py-3">
                  <Loader2 className="mt-0.5 shrink-0 animate-spin text-subtle" />
                  <div className="min-w-0 text-[13px] leading-5">
                    <p className="font-medium">Waiting for installation…</p>
                    <p className="text-muted-foreground">
                      Complete the installation on GitHub, then come back here
                    </p>
                  </div>
                </div>

                <div className="text-[13px] leading-5">
                  <p className="font-medium">On GitHub:</p>
                  <ol className="mt-1 list-decimal space-y-0.5 pl-5 text-muted-foreground marker:font-mono marker:text-xs">
                    <li>Select which repositories to grant access</li>
                    <li>Click &quot;Install&quot; or &quot;Save&quot;</li>
                    <li>Return here — we&apos;ll detect it automatically</li>
                  </ol>
                </div>

                <DialogFooter>
                  <Button variant="outline" onClick={handleStopPolling}>
                    Cancel
                  </Button>
                </DialogFooter>
              </>
            ) : (
              <>
                <dl className="divide-y divide-border border-y border-border text-[13px]">
                  <div className="flex items-center justify-between gap-4 py-2.5">
                    <dt className="text-muted-foreground">Status</dt>
                    <dd className="inline-flex items-center gap-1.5 font-medium">
                      <span aria-hidden="true" className="size-2 rounded-full bg-warn" />
                      App created — needs installation
                    </dd>
                  </div>
                  <div className="flex items-center justify-between gap-4 py-2.5">
                    <dt className="text-muted-foreground">App</dt>
                    <dd className="min-w-0 truncate font-mono text-xs">{status.appName}</dd>
                  </div>
                </dl>

                {checkMessage && (
                  <div className="rounded-md border border-danger/40 bg-secondary/60 px-3 py-2.5 text-[13px] leading-5">
                    {checkMessage}
                  </div>
                )}

                <DialogFooter>
                  <Button
                    variant="ghost"
                    className="text-muted-foreground sm:mr-auto"
                    onClick={handleDisconnect}
                    disabled={loading}
                  >
                    Start over
                  </Button>
                  <Button variant="outline" onClick={handleCheckInstallation} disabled={loading}>
                    {loading ? <Loader2 className="animate-spin" /> : null}
                    I already installed
                  </Button>
                  <Button onClick={handleInstall}>
                    <ExternalLink />
                    Install GitHub App
                  </Button>
                </DialogFooter>
              </>
            )}
          </div>
        )}

        {/* Not configured - show creation form */}
        {!status?.configured && (
          <div className="space-y-4">
            <div className="space-y-1.5">
              <label htmlFor="appName" className="text-[13px] font-medium">
                GitHub App name
              </label>
              <Input
                id="appName"
                placeholder="my-docklift"
                value={appName}
                onChange={(e) => setAppName(e.target.value)}
                disabled={loading}
              />
              <p className="text-xs text-muted-foreground">
                This will create:{" "}
                <code className="font-mono text-foreground">docklift-{appName || "my-app"}</code>
              </p>
            </div>

            <div className="text-[13px] leading-5">
              <p className="font-medium">What happens next</p>
              <ol className="mt-1 list-decimal space-y-0.5 pl-5 text-muted-foreground marker:font-mono marker:text-xs">
                <li>You&apos;ll be redirected to GitHub</li>
                <li>Create and install the app on your repos</li>
                <li>
                  {typeof window !== 'undefined' && window.location.hostname === 'localhost'
                    ? <span className="text-warn">Return here manually after installation</span>
                    : "You'll be redirected back automatically"
                  }
                </li>
              </ol>
            </div>

            <DialogFooter>
              <Button onClick={handleCreateApp} disabled={loading || !appName.trim()}>
                {loading ? <Loader2 className="animate-spin" /> : <GithubIcon className="size-4" />}
                Create GitHub App
              </Button>
            </DialogFooter>
          </div>
        )}
      </DialogContent>
    </Dialog>
  );
}
