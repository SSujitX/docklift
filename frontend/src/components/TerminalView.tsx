// TerminalView component - interactive xterm.js shell with system controls

import { useState, useEffect, useRef, useCallback } from "react";
import { useSearchParams } from "react-router-dom";
import { Maximize2, Minimize2 } from "lucide-react";
import { Button } from "@/components/ui/button";
import {
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
  DialogDescription,
  DialogFooter,
} from "@/components/ui/dialog";
import { Input } from "@/components/ui/input";
import { API_URL, cn } from "@/lib/utils";
import { authFetch } from "@/lib/auth";
import { toast } from "sonner";
import { useTheme } from "@/lib/theme";
import {
  fetchVersionInfo,
  getCachedVersion,
  type VersionInfo,
} from "@/components/shell/SidebarStatus";

type WaitKind = "upgrade" | "update-system";

type WaitState = {
  kind: WaitKind;
  current?: string;
  latest?: string;
  simulated?: boolean;
};

/** Survives React StrictMode remounts when URL query is cleared before dialog state commits. */
let pendingUpgradeConfirm = false;

const PASSWORD_CANCEL = Symbol("terminal_password_cancel");

function xtermTheme(mode: "dark" | "light") {
  if (mode === "light") {
    return {
      background: "#fafafa",
      foreground: "#171717",
      cursor: "#171717",
      cursorAccent: "#fafafa",
      selectionBackground: "#17171722",
      selectionForeground: "#171717",
      black: "#171717",
      red: "#b91c1c",
      green: "#15803d",
      yellow: "#a16207",
      blue: "#1d4ed8",
      magenta: "#6b21a8",
      cyan: "#0e7490",
      white: "#525252",
      brightBlack: "#737373",
      brightRed: "#dc2626",
      brightGreen: "#16a34a",
      brightYellow: "#ca8a04",
      brightBlue: "#2563eb",
      brightMagenta: "#7c3aed",
      brightCyan: "#0891b2",
      brightWhite: "#0a0a0a",
    };
  }
  return {
    background: "#0a0a0a",
    foreground: "#e5e5e5",
    cursor: "#e5e5e5",
    cursorAccent: "#0a0a0a",
    selectionBackground: "#e5e5e533",
    selectionForeground: "#fafafa",
    black: "#0a0a0a",
    red: "#f87171",
    green: "#4ade80",
    yellow: "#facc15",
    blue: "#93c5fd",
    magenta: "#d8b4fe",
    cyan: "#67e8f9",
    white: "#e5e5e5",
    brightBlack: "#737373",
    brightRed: "#fca5a5",
    brightGreen: "#86efac",
    brightYellow: "#fde047",
    brightBlue: "#bfdbfe",
    brightMagenta: "#e9d5ff",
    brightCyan: "#a5f3fc",
    brightWhite: "#fafafa",
  };
}

function formatVersion(v?: string) {
  if (!v) return "—";
  return v.startsWith("v") ? v : `v${v}`;
}

export function TerminalView({ className }: { className?: string }) {
  const { resolvedTheme } = useTheme();
  const [searchParams, setSearchParams] = useSearchParams();
  const [showRebootDialog, setShowRebootDialog] = useState(false);
  const [showResetDialog, setShowResetDialog] = useState(false);
  const [showPurgeDialog, setShowPurgeDialog] = useState(false);
  const [showUpgradeConfirm, setShowUpgradeConfirm] = useState(false);
  const [showUpdateConfirm, setShowUpdateConfirm] = useState(false);
  const [waitState, setWaitState] = useState<WaitState | null>(null);
  const [versionInfo, setVersionInfo] = useState<VersionInfo | null>(
    () => getCachedVersion(),
  );
  const [versionLoading, setVersionLoading] = useState(false);
  const [refreshInSec, setRefreshInSec] = useState(90);
  const [isProcessing, setIsProcessing] = useState(false);
  const [showPasswordDialog, setShowPasswordDialog] = useState(false);
  const [passwordInput, setPasswordInput] = useState("");
  const [passwordError, setPasswordError] = useState("");
  const [passwordPrompt, setPasswordPrompt] = useState({
    title: "Confirm with password",
    description: "Enter your account password to continue.",
    submitLabel: "Confirm",
  });
  const [connected, setConnected] = useState(false);
  const [connecting, setConnecting] = useState(false);
  const [isFullscreen, setIsFullscreen] = useState(false);
  
  const terminalRef = useRef<HTMLDivElement>(null);
  const xtermRef = useRef<any>(null);
  const wsRef = useRef<WebSocket | null>(null);
  const fitAddonRef = useRef<any>(null);
  const passwordInputRef = useRef<HTMLInputElement>(null);
  const themeRef = useRef(resolvedTheme);
  themeRef.current = resolvedTheme;
  const passwordResolveRef = useRef<
    ((value: string | typeof PASSWORD_CANCEL) => void) | null
  >(null);

  // Cleanup on unmount
  useEffect(() => {
    return () => {
      if (wsRef.current) {
        wsRef.current.close();
        wsRef.current = null;
      }
      if (xtermRef.current) {
        xtermRef.current.dispose();
        xtermRef.current = null;
      }
    };
  }, []);

  // Initialize xterm.js terminal and connect on mount
  const contextMenuHandlerRef = useRef<((e: Event) => void) | null>(null);
  useEffect(() => {
    if (!terminalRef.current) return;

    let disposed = false;

    async function initTerminal() {
      // Dynamic imports for xterm (only works in browser)
      const { Terminal } = await import("@xterm/xterm");
      const { FitAddon } = await import("@xterm/addon-fit");
      const { WebLinksAddon } = await import("@xterm/addon-web-links");
      const { ClipboardAddon } = await import("@xterm/addon-clipboard");

      // xterm CSS is imported globally via globals.css or layout

      if (disposed || !terminalRef.current) return;

      const fitAddon = new FitAddon();
      fitAddonRef.current = fitAddon;

      const term = new Terminal({
        cursorBlink: true,
        cursorStyle: "block",
        fontSize: 14,
        fontFamily: "'JetBrains Mono', 'Fira Code', 'Cascadia Code', 'Menlo', 'Monaco', 'Consolas', monospace",
        fontWeight: "400",
        fontWeightBold: "600",
        lineHeight: 1.35,
        letterSpacing: 0,
        allowTransparency: false,
        theme: xtermTheme(themeRef.current),
        scrollback: 5000,
        convertEol: true,
        allowProposedApi: true,
      });

      term.loadAddon(fitAddon);
      term.loadAddon(new WebLinksAddon());
      term.loadAddon(new ClipboardAddon());

      // Handle Ctrl+C (Copy if selection, else Interrupt) and Ctrl+V (Paste)
      term.attachCustomKeyEventHandler((arg) => {
        if (arg.ctrlKey && arg.code === "KeyC" && arg.type === "keydown") {
          const selection = term.getSelection();
          if (selection) {
            navigator.clipboard.writeText(selection);
            return false;
          }
        }
        if (arg.ctrlKey && arg.code === "KeyV" && arg.type === "keydown") {
          navigator.clipboard.readText().then((text) => {
            term.paste(text);
          });
          return false;
        }
        return true;
      });

      // Handle right click to paste (store in ref for cleanup)
      const contextMenuHandler = (e: Event) => {
        e.preventDefault();
        navigator.clipboard.readText().then((text) => {
          term.paste(text);
        });
      };
      contextMenuHandlerRef.current = contextMenuHandler;
      terminalRef.current!.addEventListener('contextmenu', contextMenuHandler);

      term.open(terminalRef.current!);
      xtermRef.current = term;

      // Fit to container
      requestAnimationFrame(() => {
        try { fitAddon.fit(); } catch {}
      });

      // Connect to server
      term.writeln("\x1b[90mConnecting to server...\x1b[0m");
      term.writeln("");

      // Now connect WebSocket
      connectWebSocket(term, fitAddon);
    }

    initTerminal();

    return () => {
      disposed = true;
      // Cleanup contextmenu listener to prevent memory leak
      if (terminalRef.current && contextMenuHandlerRef.current) {
        terminalRef.current.removeEventListener('contextmenu', contextMenuHandlerRef.current);
      }
    };
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

  // Keep xterm colors in sync with app light/dark
  useEffect(() => {
    if (!xtermRef.current) return;
    xtermRef.current.options.theme = xtermTheme(resolvedTheme);
  }, [resolvedTheme]);

  // Handle window resize
  useEffect(() => {
    const handleResize = () => {
      if (fitAddonRef.current && xtermRef.current) {
        try {
          fitAddonRef.current.fit();
          // Send resize to backend
          if (wsRef.current?.readyState === WebSocket.OPEN) {
            const { cols, rows } = xtermRef.current;
            wsRef.current.send(JSON.stringify({ type: "resize", cols, rows }));
          }
        } catch {}
      }
    };

    window.addEventListener("resize", handleResize);

    // Also observe the terminal container for size changes
    const observer = new ResizeObserver(handleResize);
    if (terminalRef.current) observer.observe(terminalRef.current);

    return () => {
      window.removeEventListener("resize", handleResize);
      observer.disconnect();
    };
  }, []);

  const cancelPasswordPrompt = useCallback(() => {
    setShowPasswordDialog(false);
    setPasswordInput("");
    setPasswordError("");
    if (!passwordResolveRef.current) return;
    passwordResolveRef.current(PASSWORD_CANCEL);
    passwordResolveRef.current = null;
    setConnecting(false);
    if (wsRef.current) {
      wsRef.current.close();
      wsRef.current = null;
    }
  }, []);

  const promptPassword = useCallback(
    (opts?: {
      title?: string;
      description?: string;
      submitLabel?: string;
    }): Promise<string | typeof PASSWORD_CANCEL> => {
      return new Promise((resolve) => {
        passwordResolveRef.current = resolve;
        setPasswordPrompt({
          title: opts?.title || "Confirm with password",
          description:
            opts?.description ||
            "Enter your account password to continue.",
          submitLabel: opts?.submitLabel || "Confirm",
        });
        setPasswordError("");
        setPasswordInput("");
        setShowPasswordDialog(true);
      });
    },
    [],
  );

  const handlePasswordSubmit = useCallback(() => {
    if (!passwordInput.trim()) return;
    const pwd = passwordInput;
    if (passwordResolveRef.current) {
      passwordResolveRef.current(pwd);
      passwordResolveRef.current = null;
    }
    setPasswordInput("");
    setShowPasswordDialog(false);
  }, [passwordInput]);

  const connectWebSocket = useCallback(async (term: any, fitAddon: any) => {
    setConnecting(true);

    // Session JWT stays in Authorization — only a short-lived purpose=terminal token goes in the WS URL
    const sessionToken = typeof window !== "undefined" ? localStorage.getItem("docklift_token") : null;

    if (!sessionToken) {
      term.writeln("  \x1b[1;31m✗ Not authenticated. Please log in first.\x1b[0m");
      setConnecting(false);
      return;
    }

    let token: string;
    try {
      const tokenRes = await authFetch(`${API_URL}/api/auth/terminal-token`, {
        method: "POST",
      });
      if (!tokenRes.ok) {
        throw new Error("Failed to issue terminal token");
      }
      const tokenData = await tokenRes.json();
      token = tokenData.token;
      if (!token) throw new Error("Empty terminal token");
    } catch (err: any) {
      term.writeln(`  \x1b[1;31m✗ ${err?.message || "Terminal auth failed"}\x1b[0m`);
      setConnecting(false);
      return;
    }

    // Build WebSocket URL — use same origin (through Nginx) or API_URL if set
    const wsProtocol = window.location.protocol === "https:" ? "wss:" : "ws:";
    const apiHost = API_URL
      ? API_URL.replace(/^https?:\/\//, "").replace(/\/$/, "")
      : window.location.host;
    const wsUrl = `${wsProtocol}//${apiHost}/ws/terminal?token=${encodeURIComponent(token)}`;

    try {
      const ws = new WebSocket(wsUrl);
      wsRef.current = ws;

      ws.onopen = () => {
        term.writeln("  \x1b[1;32m✓ Connected to server\x1b[0m");
        term.writeln("  \x1b[90mAuthenticating...\x1b[0m");
      };

      ws.onmessage = async (event) => {
        try {
          const msg = JSON.parse(event.data);

          if (msg.type === "auth_required") {
            const password = await promptPassword({
              title: "Confirm terminal access",
              description:
                "Enter your account password to open a root shell on this host.",
              submitLabel: "Open shell",
            });
            if (password === PASSWORD_CANCEL) {
              setConnecting(false);
              ws.close();
              return;
            }
            ws.send(JSON.stringify({
              type: "auth", 
              password,
              cols: term.cols,
              rows: term.rows,
            }));
          }

          if (msg.type === "auth_success") {
            setConnected(true);
            setConnecting(false);
            term.writeln("  \x1b[1;32m✓ Authenticated — terminal ready\x1b[0m");
            term.writeln("");

            // Fit again after auth
            requestAnimationFrame(() => {
              try { 
                fitAddon.fit();
                ws.send(JSON.stringify({ type: "resize", cols: term.cols, rows: term.rows }));
              } catch {}
            });

            // Wire up terminal input → WebSocket
            term.onData((data: string) => {
              if (ws.readyState === WebSocket.OPEN) {
                ws.send(JSON.stringify({ type: "input", data }));
              }
            });
          }

          if (msg.type === "auth_error") {
            term.writeln(`  \x1b[1;31m✗ ${msg.message || "Authentication failed"}\x1b[0m`);
            setConnecting(false);
            const password = await promptPassword({
              title: "Confirm terminal access",
              description:
                "Password rejected. Enter your account password to open a root shell.",
              submitLabel: "Open shell",
            });
            if (password === PASSWORD_CANCEL) {
              setConnecting(false);
              ws.close();
              return;
            }
            ws.send(JSON.stringify({
              type: "auth", 
              password,
              cols: term.cols,
              rows: term.rows,
            }));
          }

          if (msg.type === "output") {
            term.write(msg.data);
          }

          if (msg.type === "exit") {
            term.writeln("");
            term.writeln("  \x1b[1;33m⚠ Shell session ended\x1b[0m");
            setConnected(false);
          }
        } catch {}
      };

      ws.onclose = () => {
        setConnected(false);
        setConnecting(false);
        term.writeln("");
        term.writeln("  \x1b[1;33m⚠ Disconnected from server\x1b[0m");
        term.writeln("  \x1b[90mPress any key to reconnect...\x1b[0m");

        // Reconnect on keypress
        const disposable = term.onData(() => {
          disposable.dispose();
          term.writeln("");
          term.writeln("  \x1b[90mReconnecting...\x1b[0m");
          connectWebSocket(term, fitAddon);
        });
      };

      ws.onerror = () => {
        setConnecting(false);
        // onclose will handle the error message
      };

    } catch (err: any) {
      term.writeln(`  \x1b[1;31m✗ Connection failed: ${err.message}\x1b[0m`);
      setConnecting(false);
    }
  }, [promptPassword, cancelPasswordPrompt]);

  const clearTerminalQuery = useCallback(() => {
    if (!searchParams.has("confirm") && !searchParams.has("action")) return;
    const next = new URLSearchParams(searchParams);
    next.delete("confirm");
    next.delete("action");
    setSearchParams(next, { replace: true });
  }, [searchParams, setSearchParams]);

  const openUpgradeConfirm = useCallback(async () => {
    setShowUpgradeConfirm(true);
    setVersionLoading(true);
    try {
      const info = await fetchVersionInfo(true);
      if (info) setVersionInfo(info);
    } finally {
      setVersionLoading(false);
    }
  }, []);

  const dismissUpgradeConfirm = useCallback(() => {
    pendingUpgradeConfirm = false;
    setShowUpgradeConfirm(false);
  }, []);

  // Deep links from sidebar: ?confirm=upgrade | legacy ?action=upgrade → confirm (not fake wait)
  useEffect(() => {
    const confirm = searchParams.get("confirm");
    const action = searchParams.get("action");
    if (
      confirm === "upgrade" ||
      action === "upgrade" ||
      action === "upgrade_simulated"
    ) {
      pendingUpgradeConfirm = true;
      clearTerminalQuery();
    }
    // Module flag survives StrictMode remount after the query is stripped.
    // Consume on microtask so the remount effect still sees the flag.
    if (pendingUpgradeConfirm) {
      void openUpgradeConfirm();
      queueMicrotask(() => {
        pendingUpgradeConfirm = false;
      });
    }
  }, [searchParams, clearTerminalQuery, openUpgradeConfirm]);

  useEffect(() => {
    if (!waitState) return;
    setRefreshInSec(waitState.kind === "upgrade" ? 90 : 120);
    const id = window.setInterval(() => {
      setRefreshInSec((s) => (s > 0 ? s - 1 : 0));
    }, 1000);
    return () => window.clearInterval(id);
  }, [waitState]);

  const handleSystemAction = async (
    action: "reboot" | "reset" | "purge" | "update-system" | "upgrade",
  ) => {
    setShowRebootDialog(false);
    setShowResetDialog(false);
    setShowPurgeDialog(false);
    setShowUpgradeConfirm(false);
    setShowUpdateConfirm(false);
    if (action === "upgrade") pendingUpgradeConfirm = false;

    const labels: Record<typeof action, { title: string; description: string; submit: string }> = {
      upgrade: {
        title: "Confirm Docklift upgrade",
        description:
          "Enter your account password. Cancel stops the upgrade — JWT alone is not enough.",
        submit: "Start upgrade",
      },
      "update-system": {
        title: "Confirm package update",
        description:
          "Enter your account password to run apt update/upgrade on this host.",
        submit: "Start update",
      },
      reboot: {
        title: "Confirm server reboot",
        description: "Enter your account password to reboot this host.",
        submit: "Reboot",
      },
      reset: {
        title: "Confirm stack reset",
        description:
          "Enter your account password to restart Docklift core containers.",
        submit: "Reset stack",
      },
      purge: {
        title: "Confirm image purge",
        description:
          "Enter your account password to remove dangling Docker images.",
        submit: "Purge images",
      },
    };

    const password = await promptPassword({
      title: labels[action].title,
      description: labels[action].description,
      submitLabel: labels[action].submit,
    });
    if (password === PASSWORD_CANCEL) {
      toast.message("Cancelled — password required");
      return;
    }

    setIsProcessing(true);
    try {
      const res = await authFetch(`${API_URL}/api/system/${action}`, {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ password }),
      });
      const data = await res.json().catch(() => ({}));

      if (!res.ok) {
        if (data?.requirePassword) {
          toast.error(data.error || "Password required");
        }
        throw new Error(data.error || "Action failed");
      }

      if (action === "upgrade" || action === "update-system") {
        const simulated = Boolean(data.message?.includes("Simulated"));
        setWaitState({
          kind: action,
          current: versionInfo?.current ?? getCachedVersion()?.current,
          latest: versionInfo?.latest ?? getCachedVersion()?.latest,
          simulated,
        });
        return;
      }

      toast.success(data.message || `${action} successful`);
    } catch (err: any) {
      toast.error(err.message || `Failed to ${action} server`);
    } finally {
      setIsProcessing(false);
    }
  };

  const fitAfterLayout = () => {
    setTimeout(() => {
      if (fitAddonRef.current) {
        try {
          fitAddonRef.current.fit();
        } catch {}
        if (wsRef.current?.readyState === WebSocket.OPEN && xtermRef.current) {
          try {
            wsRef.current.send(
              JSON.stringify({
                type: "resize",
                cols: xtermRef.current.cols,
                rows: xtermRef.current.rows,
              }),
            );
          } catch {}
        }
      }
    }, 100);
  };

  const termBg = resolvedTheme === "light" ? "#fafafa" : "#0a0a0a";
  const statusLabel = connected ? "live" : connecting ? "connecting" : "offline";
  const hostLocked =
    !!waitState && !waitState.simulated && waitState.kind === "upgrade";
  const hostBusy = isProcessing || !!waitState;
  const upgradeTargetLabel =
    versionLoading && !versionInfo?.latest
      ? "…"
      : versionInfo?.latest
        ? formatVersion(versionInfo.latest)
        : versionLoading
          ? "…"
          : "unknown";

  return (
    <div className={cn("flex min-h-0 flex-col gap-2", className)}>
      {/* Host controls — compact strip; scroll horizontally on small screens */}
      <div className="flex shrink-0 items-center gap-2 border-b border-border/60 pb-2">
        <p className="hidden shrink-0 text-[10px] font-semibold uppercase tracking-[0.14em] text-muted-foreground sm:block">
          Host
        </p>
        <div className="-mx-1 flex min-w-0 flex-1 gap-1 overflow-x-auto px-1 [-ms-overflow-style:none] [scrollbar-width:none] [&::-webkit-scrollbar]:hidden">
          <Button
            variant="ghost"
            size="sm"
            disabled={hostBusy}
            onClick={() => setShowUpdateConfirm(true)}
            className="h-8 shrink-0 px-2.5 text-xs font-medium text-muted-foreground hover:text-foreground"
          >
            Update packages
          </Button>
          <Button
            variant="ghost"
            size="sm"
            disabled={hostBusy}
            onClick={() => void openUpgradeConfirm()}
            className="h-8 shrink-0 px-2.5 text-xs font-medium text-muted-foreground hover:text-foreground"
          >
            Upgrade Docklift
          </Button>
          <Button
            variant="ghost"
            size="sm"
            disabled={isProcessing || hostLocked}
            onClick={() => setShowPurgeDialog(true)}
            className="h-8 shrink-0 px-2.5 text-xs font-medium text-muted-foreground hover:text-foreground"
          >
            Purge images
          </Button>
          <span
            aria-hidden
            className="mx-0.5 hidden h-4 w-px shrink-0 bg-border sm:inline-block"
          />
          <Button
            variant="ghost"
            size="sm"
            disabled={isProcessing || hostLocked}
            onClick={() => setShowResetDialog(true)}
            className="h-8 shrink-0 px-2.5 text-xs font-medium text-muted-foreground hover:text-foreground"
          >
            Reset stack
          </Button>
          <Button
            variant="ghost"
            size="sm"
            disabled={isProcessing || hostLocked}
            onClick={() => setShowRebootDialog(true)}
            className="h-8 shrink-0 px-2.5 text-xs font-medium text-destructive hover:bg-destructive/10 hover:text-destructive"
          >
            Reboot
          </Button>
        </div>
      </div>

      {/* Shell frame — fills remaining viewport; xterm FitAddon follows ResizeObserver */}
      <div
        className={cn(
          "flex min-h-0 flex-1 flex-col overflow-hidden rounded-2xl border border-border/60 bg-background",
          isFullscreen &&
            "fixed inset-0 z-50 m-0 h-screen w-screen rounded-none border-0",
        )}
      >
        <div className="flex shrink-0 items-center justify-between gap-3 border-b border-border/60 px-3 py-1.5 sm:py-2">
          <div className="flex min-w-0 items-baseline gap-2">
            <span className="truncate font-mono text-xs text-foreground">
              root@docklift
            </span>
            <span className="text-muted-foreground/50">·</span>
            <span
              className={cn(
                "shrink-0 text-[11px] tabular-nums",
                connected
                  ? "text-foreground"
                  : connecting
                    ? "text-muted-foreground"
                    : "text-muted-foreground/70",
              )}
            >
              {statusLabel}
            </span>
          </div>
          <Button
            variant="ghost"
            size="icon"
            onClick={() => {
              setIsFullscreen((v) => !v);
              fitAfterLayout();
            }}
            className="h-7 w-7 shrink-0 text-muted-foreground hover:text-foreground"
            title={isFullscreen ? "Exit full screen" : "Full screen"}
          >
            {isFullscreen ? (
              <Minimize2 className="h-3.5 w-3.5" />
            ) : (
              <Maximize2 className="h-3.5 w-3.5" />
            )}
          </Button>
        </div>

        <div
          ref={terminalRef}
          className="min-h-0 flex-1 p-2"
          style={{ background: termBg }}
        />
      </div>

      <Dialog
        open={showUpgradeConfirm}
        onOpenChange={(open) => {
          if (!open) dismissUpgradeConfirm();
          else setShowUpgradeConfirm(true);
        }}
      >
        <DialogContent className="sm:max-w-md gap-5">
          <DialogHeader className="space-y-2 pr-6">
            <DialogTitle>Upgrade Docklift?</DialogTitle>
            <DialogDescription>
              The panel will go offline while the stack rebuilds. Keep this tab
              open — when it returns, refresh.
            </DialogDescription>
          </DialogHeader>

          <div className="rounded-lg border border-border bg-muted/40 px-3 py-3 font-mono text-xs">
            <div className="flex items-center justify-between gap-3">
              <span className="text-muted-foreground">Now</span>
              <span className="tabular-nums text-foreground">
                {versionLoading && !versionInfo
                  ? "…"
                  : formatVersion(versionInfo?.current)}
              </span>
            </div>
            <div className="mt-2 flex items-center justify-between gap-3 border-t border-border/70 pt-2">
              <span className="text-muted-foreground">Installing</span>
              <span className="tabular-nums font-medium text-foreground">
                {upgradeTargetLabel}
              </span>
            </div>
          </div>

          <p className="text-sm text-muted-foreground">
            {upgradeTargetLabel === "unknown"
              ? "Could not resolve the target version from GitHub. You can still start — upgrade.sh will pull the latest release."
              : "Usually ready in 1–2 minutes. Deployed apps keep running; only the Docklift panel restarts."}
          </p>

          <DialogFooter className="gap-2 sm:gap-0">
            <Button
              variant="outline"
              onClick={dismissUpgradeConfirm}
              disabled={isProcessing}
            >
              Cancel
            </Button>
            <Button
              disabled={isProcessing || versionLoading}
              onClick={() => handleSystemAction("upgrade")}
            >
              {isProcessing
                ? "Starting…"
                : versionLoading
                  ? "Checking version…"
                  : "Start upgrade"}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>

      <Dialog open={showUpdateConfirm} onOpenChange={setShowUpdateConfirm}>
        <DialogContent className="sm:max-w-md gap-5">
          <DialogHeader className="space-y-2 pr-6">
            <DialogTitle>Update host packages?</DialogTitle>
            <DialogDescription>
              Runs <span className="font-mono text-foreground">apt update</span>{" "}
              and upgrade on the server. This can take several minutes.
            </DialogDescription>
          </DialogHeader>
          <p className="text-sm text-muted-foreground">
            The panel usually stays up. If this page stops responding, wait 1–2
            minutes, then refresh.
          </p>
          <DialogFooter className="gap-2 sm:gap-0">
            <Button
              variant="outline"
              onClick={() => setShowUpdateConfirm(false)}
              disabled={isProcessing}
            >
              Cancel
            </Button>
            <Button
              disabled={isProcessing}
              onClick={() => handleSystemAction("update-system")}
            >
              {isProcessing ? "Starting…" : "Start update"}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>

      <Dialog
        open={!!waitState}
        onOpenChange={(open) => {
          if (!open) setWaitState(null);
        }}
      >
        <DialogContent
          className="sm:max-w-md gap-5"
          showCloseButton={false}
          onPointerDownOutside={(e) => e.preventDefault()}
          onEscapeKeyDown={(e) => e.preventDefault()}
        >
          <DialogHeader className="space-y-2">
            <DialogTitle>
              {waitState?.kind === "upgrade"
                ? waitState.simulated
                  ? "Upgrade simulated"
                  : "Upgrade in progress"
                : waitState?.simulated
                  ? "Package update simulated"
                  : "Package update in progress"}
            </DialogTitle>
            <DialogDescription>
              {waitState?.kind === "upgrade"
                ? waitState.simulated
                  ? "Dev mode did not restart the stack. Nothing is offline."
                  : "This UI will go offline while Docklift restarts. That is expected."
                : waitState?.simulated
                  ? "Dev mode did not change host packages."
                  : "Host packages are updating in the background. This page may briefly disconnect."}
            </DialogDescription>
          </DialogHeader>

          {waitState?.kind === "upgrade" && !waitState.simulated && (
            <div className="rounded-lg border border-border bg-muted/40 px-3 py-3 font-mono text-xs">
              <div className="flex items-center justify-between gap-3">
                <span className="text-muted-foreground">Target</span>
                <span className="tabular-nums font-medium text-foreground">
                  {formatVersion(waitState.latest)}
                </span>
              </div>
              {waitState.current && (
                <div className="mt-2 flex items-center justify-between gap-3 border-t border-border/70 pt-2">
                  <span className="text-muted-foreground">From</span>
                  <span className="tabular-nums text-foreground">
                    {formatVersion(waitState.current)}
                  </span>
                </div>
              )}
            </div>
          )}

          {!waitState?.simulated && (
            <p className="text-sm text-muted-foreground">
              {refreshInSec > 0 ? (
                <>
                  Try refreshing in about{" "}
                  <span className="font-medium tabular-nums text-foreground">
                    {Math.floor(refreshInSec / 60)}:
                    {String(refreshInSec % 60).padStart(2, "0")}
                  </span>
                  {waitState?.kind === "upgrade"
                    ? ". The new Docklift version is available once the panel comes back."
                    : ". The panel should respond again once package updates finish."}
                </>
              ) : (
                <>You can refresh now — if the page still fails, wait another minute.</>
              )}
            </p>
          )}

          <DialogFooter className="gap-2 sm:gap-0">
            {(waitState?.simulated || refreshInSec === 0) && (
              <Button variant="outline" onClick={() => setWaitState(null)}>
                Dismiss
              </Button>
            )}
            <Button onClick={() => window.location.reload()}>
              Refresh page
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>

      <Dialog open={showRebootDialog} onOpenChange={setShowRebootDialog}>
        <DialogContent className="sm:max-w-md">
          <DialogHeader>
            <DialogTitle>Reboot this server?</DialogTitle>
            <DialogDescription>
              Every project goes offline for a few minutes. In-flight
              deployments stop. The machine restarts completely.
            </DialogDescription>
          </DialogHeader>
          <DialogFooter className="gap-2 sm:gap-0">
            <Button variant="outline" onClick={() => setShowRebootDialog(false)}>
              Cancel
            </Button>
            <Button
              variant="destructive"
              disabled={isProcessing}
              onClick={() => handleSystemAction("reboot")}
            >
              Reboot
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>

      <Dialog open={showResetDialog} onOpenChange={setShowResetDialog}>
        <DialogContent className="sm:max-w-md">
          <DialogHeader>
            <DialogTitle>Reset Docklift services?</DialogTitle>
            <DialogDescription>
              Restarts core Docklift containers and workers. Running app
              projects stay up unless they depend on the panel briefly.
            </DialogDescription>
          </DialogHeader>
          <DialogFooter className="gap-2 sm:gap-0">
            <Button variant="outline" onClick={() => setShowResetDialog(false)}>
              Cancel
            </Button>
            <Button
              disabled={isProcessing}
              onClick={() => handleSystemAction("reset")}
            >
              Reset stack
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>

      <Dialog open={showPurgeDialog} onOpenChange={setShowPurgeDialog}>
        <DialogContent className="sm:max-w-md">
          <DialogHeader>
            <DialogTitle>Purge dangling images?</DialogTitle>
            <DialogDescription>
              Removes untagged Docker images only. No host prune, journal wipe,
              or restart of other workloads.
            </DialogDescription>
          </DialogHeader>
          <DialogFooter className="gap-2 sm:gap-0">
            <Button variant="outline" onClick={() => setShowPurgeDialog(false)}>
              Cancel
            </Button>
            <Button
              disabled={isProcessing}
              onClick={() => handleSystemAction("purge")}
            >
              Purge images
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>

      <Dialog
        open={showPasswordDialog}
        onOpenChange={(open) => {
          if (!open) cancelPasswordPrompt();
        }}
      >
        <DialogContent className="sm:max-w-md">
          <DialogHeader>
            <DialogTitle>{passwordPrompt.title}</DialogTitle>
            <DialogDescription>{passwordPrompt.description}</DialogDescription>
          </DialogHeader>
          <form
            onSubmit={(e) => {
              e.preventDefault();
              handlePasswordSubmit();
            }}
            className="space-y-4"
          >
            <div className="space-y-2">
              <Input
                ref={passwordInputRef}
                type="password"
                placeholder="Account password"
                value={passwordInput}
                onChange={(e) => setPasswordInput(e.target.value)}
                className="h-10 font-mono"
                autoComplete="current-password"
                autoFocus
              />
              {passwordError && (
                <p className="text-sm text-destructive">{passwordError}</p>
              )}
            </div>
            <DialogFooter className="gap-2 sm:gap-0">
              <Button
                type="button"
                variant="outline"
                onClick={cancelPasswordPrompt}
              >
                Cancel
              </Button>
              <Button type="submit" disabled={!passwordInput.trim()}>
                {passwordPrompt.submitLabel}
              </Button>
            </DialogFooter>
          </form>
        </DialogContent>
      </Dialog>
    </div>
  );
}
