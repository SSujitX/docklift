// GitHubConnect component - GitHub App creation, installation, and connection flow
"use client";

import { useState, useRef, useEffect } from "react";
import { Button } from "@/components/ui/button";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { Input } from "@/components/ui/input";
import { Loader2, CheckCircle, ExternalLink } from "lucide-react";
import { GithubIcon } from "./icons/GithubIcon";
import { getAuthHeaders } from "@/lib/auth";

const API_URL = process.env.NEXT_PUBLIC_API_URL || "";

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

  const handleInstall = () => {
    if (status?.installUrl) {
      // Start polling for installation
      setIsPolling(true);
      setCheckMessage(null);
      // Open GitHub in new tab so we can poll
      window.open(status.installUrl, '_blank');
    }
  };

  const handleDisconnect = async () => {
    setLoading(true);
    setIsPolling(false);
    try {
      await fetch(`${API_URL}/api/github/disconnect`, { method: "POST", headers: getAuthHeaders() });
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
      <DialogContent className="sm:max-w-md">
        <DialogHeader>
          <DialogTitle className="flex items-center gap-2">
            <GithubIcon className="h-5 w-5" />
            {status?.configured ? "GitHub App Connected" : "Connect GitHub App"}
          </DialogTitle>
          <DialogDescription>
            {status?.configured 
              ? "Your GitHub App is configured. You can now deploy private repositories."
              : "Create a GitHub App to deploy public and private repositories."}
          </DialogDescription>
        </DialogHeader>

        <div className="space-y-4 py-4">
          {/* Already configured and installed */}
          {status?.configured && status?.installed && (
            <div className="space-y-4">
              <div className="flex items-center gap-3 p-4 bg-green-500/10 border border-green-500/20 rounded-lg">
                <CheckCircle className="h-8 w-8 text-green-500" />
                <div>
                  <p className="font-medium text-green-400">Connected</p>
                  <p className="text-sm text-muted-foreground">
                    {status.appName} • @{status.username}
                  </p>
                </div>
              </div>

              {/* Multi-account info */}
              <div className="p-3 rounded-lg bg-cyan-500/10 border border-cyan-500/20 text-xs space-y-1">
                <p className="text-cyan-400 font-medium">💡 Multi-Account Support</p>
                <p className="text-gray-600 dark:text-gray-300">
                  You can install this app on multiple accounts (Personal + Organizations). 
                  All repositories will appear together in one list.
                </p>
              </div>

              <Button
                className="w-full bg-gradient-to-r from-cyan-600 to-blue-600 hover:from-cyan-700 hover:to-blue-700"
                onClick={() => status.installUrl && window.open(status.installUrl, '_blank')}
                disabled={!status.installUrl}
              >
                <ExternalLink className="h-4 w-4 mr-2" />
                Add Another Account
              </Button>
              
              <Button
                variant="outline"
                className="w-full text-destructive hover:bg-destructive/10"
                onClick={handleDisconnect}
                disabled={loading}
              >
                {loading ? <Loader2 className="h-4 w-4 animate-spin mr-2" /> : null}
                Disconnect GitHub
              </Button>
            </div>
          )}

          {/* Configured but not installed */}
          {status?.configured && !status?.installed && (
            <div className="space-y-4">
              {/* Show polling state */}
              {isPolling ? (
                <>
                  <div className="flex items-center gap-3 p-4 bg-cyan-500/10 border border-cyan-500/20 rounded-lg">
                    <Loader2 className="h-8 w-8 text-cyan-500 animate-spin" />
                    <div>
                      <p className="font-medium text-cyan-600 dark:text-cyan-400">Waiting for installation...</p>
                      <p className="text-sm text-muted-foreground">
                        Complete the installation on GitHub, then come back here
                      </p>
                    </div>
                  </div>

                  <div className="p-3 rounded-lg bg-blue-500/10 border border-blue-500/20 text-xs space-y-1">
                    <p className="text-blue-400 font-medium">📝 On GitHub:</p>
                    <ol className="text-gray-600 dark:text-gray-300 list-decimal list-inside space-y-0.5">
                      <li>Select which repositories to grant access</li>
                      <li>Click &quot;Install&quot; or &quot;Save&quot;</li>
                      <li>Return here - we&apos;ll detect it automatically!</li>
                    </ol>
                  </div>

                  <Button
                    variant="outline"
                    className="w-full"
                    onClick={handleStopPolling}
                  >
                    Cancel
                  </Button>
                </>
              ) : (
                <>
                  <div className="flex items-center gap-3 p-4 bg-amber-100 dark:bg-amber-950/50 border border-amber-300 dark:border-amber-800 rounded-lg">
                    <GithubIcon className="h-8 w-8 text-amber-700 dark:text-amber-600" />
                    <div>
                      <p className="font-medium text-amber-800 dark:text-amber-500">App Created</p>
                      <p className="text-sm text-amber-700/70 dark:text-amber-400/70">
                        {status.appName} - needs installation
                      </p>
                    </div>
                  </div>
                  
                  <Button
                    className="w-full bg-gradient-to-r from-gray-800 to-gray-900 hover:from-gray-700 hover:to-gray-800"
                    onClick={handleInstall}
                  >
                    <ExternalLink className="h-4 w-4 mr-2" />
                    Install GitHub App
                  </Button>
                  
                  <Button
                    variant="secondary"
                    className="w-full"
                    onClick={handleCheckInstallation}
                    disabled={loading}
                  >
                    {loading ? <Loader2 className="h-4 w-4 animate-spin mr-2" /> : <CheckCircle className="h-4 w-4 mr-2" />}
                    I Already Installed
                  </Button>

                  {checkMessage && (
                    <div className="p-3 rounded-lg bg-rose-500/10 border border-rose-500/20 text-sm text-rose-600 dark:text-rose-400">
                      {checkMessage}
                    </div>
                  )}

                  <Button
                    variant="outline"
                    className="w-full"
                    onClick={handleDisconnect}
                    disabled={loading}
                  >
                    Start Over
                  </Button>
                </>
              )}
            </div>
          )}

          {/* Not configured - show creation form */}
          {!status?.configured && (
            <div className="space-y-4">
              <div className="space-y-2">
                <label htmlFor="appName" className="text-sm font-medium">GitHub App Name</label>
                <Input
                  id="appName"
                  placeholder="my-docklift"
                  value={appName}
                  onChange={(e) => setAppName(e.target.value)}
                  disabled={loading}
                />
                <p className="text-xs text-muted-foreground">
                  This will create: <code className="bg-muted px-1 rounded">docklift-{appName || "my-app"}</code>
                </p>
              </div>

              <Button
                className="w-full bg-gradient-to-r from-gray-800 to-gray-900 hover:from-gray-700 hover:to-gray-800"
                onClick={handleCreateApp}
                disabled={loading || !appName.trim()}
              >
                {loading ? (
                  <Loader2 className="h-4 w-4 animate-spin mr-2" />
                ) : (
                  <GithubIcon className="h-4 w-4 mr-2" />
                )}
                Create GitHub App
              </Button>

              <div className="p-3 rounded-lg bg-blue-500/10 border border-blue-500/20 text-xs space-y-1">
                <p className="text-blue-400 font-medium">What happens next:</p>
                <ol className="text-gray-700 dark:text-gray-300 list-decimal list-inside space-y-0.5">
                  <li>You&apos;ll be redirected to GitHub</li>
                  <li>Create and install the app on your repos</li>
                  <li>
                    {typeof window !== 'undefined' && window.location.hostname === 'localhost' 
                      ? <span className="text-amber-600 dark:text-yellow-400">Return here manually after installation</span>
                      : "You'll be redirected back automatically"
                    }
                  </li>
                </ol>
              </div>
            </div>
          )}
        </div>
      </DialogContent>
    </Dialog>
  );
}
