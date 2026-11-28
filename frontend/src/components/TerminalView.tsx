// TerminalView component - interactive xterm.js shell with system controls

import { useState, useEffect, useRef, useCallback, type CSSProperties } from "react";
import { useSearchParams } from "react-router-dom";
import { ArrowDown, Maximize2, Minimize2 } from "lucide-react";
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
/** Keep shell WS deferred across StrictMode remount + query strip for sidebar upgrade. */
let pendingShellDeferForUpgrade = false;

const PASSWORD_CANCEL = Symbol("terminal_password_cancel");

/**
 * The terminal is a fixed dark surface in both app themes. These are the only
 * literal colours in this file: the surface and a restrained 16-colour ANSI set.
 */
const TERMINAL_SURFACE = "#0f1215";
const TERMINAL_FONT = '"IBM Plex Mono", ui-monospace, SFMono-Regular, Menlo, monospace';
const XTERM_THEME = {
  background: TERMINAL_SURFACE,
  foreground: "#e6eaee",
  cursor: "#39c1d0", // dark-theme brand teal, hsl(186 62% 52%)
  cursorAccent: TERMINAL_SURFACE,
  selectionBackground: "#e6eaee2e",
  selectionForeground: "#ffffff",
  black: "#2a3036",
  red: "#e0726a",
  green: "#5db48a",
  yellow: "#d6a75a",
  blue: "#7aa3d6",
  magenta: "#ad98cf",
  cyan: "#5ab3bf",
  white: "#c5ccd3",
  brightBlack: "#6b757f",
  brightRed: "#ec9089",
  brightGreen: "#83c9a5",
  brightYellow: "#e5c07f",
  brightBlue: "#9dbbe3",
  brightMagenta: "#c4b4e0",
  brightCyan: "#82c8d1",
  brightWhite: "#f2f5f7",
};

// Same palette in light and dark: the surface does not follow the app theme.
function xtermTheme(_mode: "dark" | "light") {
  return XTERM_THEME;
}

const HOST_GROUP =
  "inline-flex shrink-0 divide-x divide-input overflow-hidden rounded-md border border-input bg-card";
const HOST_ITEM =
  "h-[30px] rounded-none px-3 text-[13px] focus-visible:outline-offset-[-2px] pointer-coarse:h-[38px]";

/**
 * Fixed dark surfaces sit inside light pages too. The `dark` class re-declares the
 * dark token triples on the element; Tailwind colour utilities read --color-*,
 * which were resolved at :root, so re-derive the ones used here from the scoped triples.
 */
const DARK_SCOPE = Object.fromEntries(
  ["foreground", "muted-foreground", "subtle", "card", "secondary", "border", "input", "ring", "brand", "ok", "warn", "danger", "info"].map(
    (name) => [`--color-${name}`, `hsl(var(--${name}))`],
  ),
) as CSSProperties;

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
  /** False while the user has scrolled up in the scrollback. */
  const [atBottom, setAtBottom] = useState(true);
  const [upgradePassword, setUpgradePassword] = useState("");
  const [upgradePasswordError, setUpgradePasswordError] = useState("");
  
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
  /** Skip shell WS auth while sidebar upgrade confirm is open (one password only). */
  const shellDeferredRef = useRef(false);
  /** True while the upgrade confirm dialog is open (survives async xterm init). */
  const upgradeConfirmOpenRef = useRef(false);
  /** Bumped to abort in-flight connectWebSocket (token fetch / WS open). */
  const connectGenerationRef = useRef(0);
  const connectWebSocketRef = useRef<
    ((term: any, fitAddon: any) => Promise<void>) | null
  >(null);
  const ensureShellConnectedRef = useRef<() => void>(() => {});

  const isShellDeferred = () =>
    shellDeferredRef.current || pendingShellDeferForUpgrade;

  const deferShellForUpgrade = () => {
    pendingShellDeferForUpgrade = true;
    shellDeferredRef.current = true;
    connectGenerationRef.current += 1;
  };

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
        fontSize: 13,
        fontFamily: TERMINAL_FONT,
        fontWeight: "400",
        fontWeightBold: "500",
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

      // Show the "Latest" button while the viewport is above the newest line.
      const syncAtBottom = () => {
        const buf = term.buffer.active;
        setAtBottom(buf.viewportY >= buf.baseY);
      };
      term.onScroll(syncAtBottom);
      term.onWriteParsed(syncAtBottom);

      // Fit to container
      requestAnimationFrame(() => {
        try { fitAddon.fit(); } catch {}
      });

      // Sidebar "Upgrade now" lands on ?confirm=upgrade — defer shell auth so the
      // upgrade dialog is the only password prompt.
      const params = new URLSearchParams(window.location.search);
      const deferForUpgrade =
        params.get("confirm") === "upgrade" ||
        params.get("action") === "upgrade" ||
        params.get("action") === "upgrade_simulated" ||
        pendingUpgradeConfirm ||
        pendingShellDeferForUpgrade ||
        shellDeferredRef.current ||
        upgradeConfirmOpenRef.current;

      if (deferForUpgrade) {
        shellDeferredRef.current = true;
        term.writeln(
          "\x1b[90mShell paused — confirm the upgrade first (one password).\x1b[0m",
        );
        term.writeln("");
        // Cancel may have run before xterm was ready — resume now if dialog is gone.
        if (!upgradeConfirmOpenRef.current) {
          ensureShellConnectedRef.current();
        }
      } else {
        term.writeln("\x1b[90mConnecting to server...\x1b[0m");
        term.writeln("");
        void connectWebSocket(term, fitAddon);
      }
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

  const ensureShellConnected = useCallback(() => {
    if (!shellDeferredRef.current && !pendingShellDeferForUpgrade) return;
    // Clear defer even if a live WS already exists (sidebar upgrade while connected).
    if (wsRef.current || connected) {
      pendingShellDeferForUpgrade = false;
      shellDeferredRef.current = false;
      return;
    }
    // xterm still loading — keep defer flags so initTerminal can resume later.
    if (!xtermRef.current || !fitAddonRef.current) return;
    if (!connectWebSocketRef.current) return;
    pendingShellDeferForUpgrade = false;
    shellDeferredRef.current = false;
    xtermRef.current.writeln("\x1b[90mConnecting to server...\x1b[0m");
    xtermRef.current.writeln("");
    void connectWebSocketRef.current(xtermRef.current, fitAddonRef.current);
  }, [connected]);

  useEffect(() => {
    ensureShellConnectedRef.current = ensureShellConnected;
  }, [ensureShellConnected]);

  const connectWebSocket = useCallback(async (term: any, fitAddon: any) => {
    if (isShellDeferred()) {
      setConnecting(false);
      return;
    }

    const gen = connectGenerationRef.current;
    const aborted = () =>
      gen !== connectGenerationRef.current || isShellDeferred();

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
      if (aborted()) {
        setConnecting(false);
        return;
      }
      if (!tokenRes.ok) {
        throw new Error("Failed to issue terminal token");
      }
      const tokenData = await tokenRes.json();
      token = tokenData.token;
      if (!token) throw new Error("Empty terminal token");
    } catch (err: any) {
      if (aborted()) {
        setConnecting(false);
        return;
      }
      term.writeln(`  \x1b[1;31m✗ ${err?.message || "Terminal auth failed"}\x1b[0m`);
      setConnecting(false);
      return;
    }

    if (aborted()) {
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
      if (aborted()) {
        ws.close();
        setConnecting(false);
        return;
      }
      wsRef.current = ws;

      ws.onopen = () => {
        if (aborted()) {
          ws.close();
          return;
        }
        term.writeln("  \x1b[1;32m✓ Connected to server\x1b[0m");
        term.writeln("  \x1b[90mAuthenticating...\x1b[0m");
      };

      ws.onmessage = async (event) => {
        try {
          if (aborted()) {
            ws.close();
            return;
          }
          const msg = JSON.parse(event.data);

          if (msg.type === "auth_required") {
            if (aborted()) {
              ws.close();
              return;
            }
            const password = await promptPassword({
              title: "Confirm terminal access",
              description:
                "Enter your account password to open a root shell on this host.",
              submitLabel: "Open shell",
            });
            if (aborted() || password === PASSWORD_CANCEL) {
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
            if (aborted()) {
              ws.close();
              return;
            }
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
            if (aborted()) {
              ws.close();
              return;
            }
            term.writeln(`  \x1b[1;31m✗ ${msg.message || "Authentication failed"}\x1b[0m`);
            setConnecting(false);
            const password = await promptPassword({
              title: "Confirm terminal access",
              description:
                "Password rejected. Enter your account password to open a root shell.",
              submitLabel: "Open shell",
            });
            if (aborted() || password === PASSWORD_CANCEL) {
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

          if (msg.type === "exit" && msg.reason === "auth_timeout") {
            // The server gave up waiting for the password: close the dialog so a late
            // submit cannot be sent to a closed socket, and say why the session ended.
            cancelPasswordPrompt();
            term.writeln("");
            term.writeln("  \x1b[1;33m⚠ Password prompt timed out — the shell was not opened\x1b[0m");
            toast.error("Terminal sign-in timed out", {
              description: "The password was not entered in time. Reconnect to try again.",
            });
            setConnected(false);
          } else if (msg.type === "exit") {
            term.writeln("");
            term.writeln("  \x1b[1;33m⚠ Shell session ended\x1b[0m");
            setConnected(false);
          }
        } catch {}
      };

      ws.onclose = () => {
        if (wsRef.current === ws) {
          wsRef.current = null;
        }
        setConnected(false);
        setConnecting(false);

        // Upgrade confirm owns the only password prompt — do not arm reconnect.
        if (aborted() || isShellDeferred()) {
          return;
        }

        term.writeln("");
        term.writeln("  \x1b[1;33m⚠ Disconnected from server\x1b[0m");
        term.writeln("  \x1b[90mPress any key to reconnect...\x1b[0m");

        const disposable = term.onData(() => {
          disposable.dispose();
          if (isShellDeferred()) return;
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
      if (aborted()) {
        setConnecting(false);
        return;
      }
      term.writeln(`  \x1b[1;31m✗ Connection failed: ${err.message}\x1b[0m`);
      setConnecting(false);
    }
  }, [promptPassword, cancelPasswordPrompt]);

  useEffect(() => {
    connectWebSocketRef.current = connectWebSocket;
  }, [connectWebSocket]);

  const clearTerminalQuery = useCallback(() => {
    if (!searchParams.has("confirm") && !searchParams.has("action")) return;
    const next = new URLSearchParams(searchParams);
    next.delete("confirm");
    next.delete("action");
    setSearchParams(next, { replace: true });
  }, [searchParams, setSearchParams]);

  const openUpgradeConfirm = useCallback(async () => {
    // Don't race a shell password prompt with the upgrade dialog.
    // Deep-link may have set pendingShellDefer — clear it if the shell is already live.
    upgradeConfirmOpenRef.current = true;
    if (connected) {
      pendingShellDeferForUpgrade = false;
      shellDeferredRef.current = false;
    } else {
      deferShellForUpgrade();
      if (passwordResolveRef.current) {
        cancelPasswordPrompt();
      }
      if (wsRef.current) {
        wsRef.current.close();
        wsRef.current = null;
      }
      setConnecting(false);
    }
    setUpgradePassword("");
    setUpgradePasswordError("");
    setShowUpgradeConfirm(true);
    setVersionLoading(true);
    try {
      const info = await fetchVersionInfo(true);
      if (info) setVersionInfo(info);
    } finally {
      setVersionLoading(false);
    }
  }, [cancelPasswordPrompt, connected]);

  const dismissUpgradeConfirm = useCallback(() => {
    pendingUpgradeConfirm = false;
    upgradeConfirmOpenRef.current = false;
    setShowUpgradeConfirm(false);
    setUpgradePassword("");
    setUpgradePasswordError("");
    // Do not clear defer flags here — ensureShellConnected clears them when it
    // can actually connect; if xterm is not ready yet, flags stay for init resume.
    ensureShellConnected();
  }, [ensureShellConnected]);

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
      pendingShellDeferForUpgrade = true;
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

  const startUpgradeFromConfirm = async () => {
    if (!upgradePassword.trim()) {
      setUpgradePasswordError("Enter your account password");
      return;
    }

    setIsProcessing(true);
    setUpgradePasswordError("");
    try {
      const res = await authFetch(`${API_URL}/api/system/upgrade`, {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ password: upgradePassword }),
      });
      const data = await res.json().catch(() => ({}));

      if (!res.ok) {
        if (data?.requirePassword) {
          setUpgradePasswordError(data.error || "Incorrect password");
          setUpgradePassword("");
          return;
        }
        throw new Error(data.error || "Upgrade failed");
      }

      pendingUpgradeConfirm = false;
      pendingShellDeferForUpgrade = false;
      upgradeConfirmOpenRef.current = false;
      setShowUpgradeConfirm(false);
      setUpgradePassword("");
      const simulated = Boolean(data.message?.includes("Simulated"));
      // Real upgrade: panel restarts. Simulated with no live shell: reconnect on wait dismiss.
      shellDeferredRef.current = Boolean(simulated && !wsRef.current && !connected);
      setWaitState({
        kind: "upgrade",
        current: versionInfo?.current ?? getCachedVersion()?.current,
        latest: versionInfo?.latest ?? getCachedVersion()?.latest,
        simulated,
      });
    } catch (err: any) {
      toast.error(err.message || "Failed to start upgrade");
    } finally {
      setIsProcessing(false);
    }
  };

  const handleSystemAction = async (
    action: "reboot" | "reset" | "purge" | "update-system" | "upgrade",
  ) => {
    // Upgrade password lives on the confirm dialog (single prompt).
    if (action === "upgrade") {
      await startUpgradeFromConfirm();
      return;
    }

    setShowRebootDialog(false);
    setShowResetDialog(false);
    setShowPurgeDialog(false);
    setShowUpdateConfirm(false);

    const labels: Record<
      Exclude<typeof action, "upgrade">,
      { title: string; description: string; submit: string }
    > = {
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
          "Enter your account password to remove unused Docklift images and clear BuildKit cache.",
        submit: "Purge images",
      },
    };

    // Wrong password → re-prompt until success or cancel (JWT alone is never enough).
    for (;;) {
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
            toast.error(data.error || "Incorrect password — try again");
            setIsProcessing(false);
            continue;
          }
          throw new Error(data.error || "Action failed");
        }

        if (action === "update-system") {
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
        return;
      } catch (err: any) {
        toast.error(err.message || `Failed to ${action} server`);
        return;
      } finally {
        setIsProcessing(false);
      }
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

  const termBg = TERMINAL_SURFACE;
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
    <div className={cn("flex min-h-0 flex-col gap-3", className)}>
      {/* Host actions — two quiet button groups; scroll horizontally on small screens */}
      <div className="flex shrink-0 items-center gap-3">
        <span className="hidden shrink-0 text-[13px] text-muted-foreground sm:inline">
          Host actions
        </span>
        <div className="-mx-4 flex min-w-0 flex-1 gap-2 overflow-x-auto px-4 scrollbar-none sm:mx-0 sm:px-0">
          <div className={HOST_GROUP}>
            <Button
              variant="ghost"
              disabled={hostBusy}
              onClick={() => setShowUpdateConfirm(true)}
              className={HOST_ITEM}
            >
              Update packages
            </Button>
            <Button
              variant="ghost"
              disabled={hostBusy}
              onClick={() => void openUpgradeConfirm()}
              className={HOST_ITEM}
            >
              Upgrade Docklift
            </Button>
            <Button
              variant="ghost"
              disabled={isProcessing || hostLocked}
              onClick={() => setShowPurgeDialog(true)}
              className={HOST_ITEM}
            >
              Purge images
            </Button>
          </div>
          <div className={HOST_GROUP}>
            <Button
              variant="ghost"
              disabled={isProcessing || hostLocked}
              onClick={() => setShowResetDialog(true)}
              className={HOST_ITEM}
            >
              Reset stack
            </Button>
            <Button
              variant="ghost"
              disabled={isProcessing || hostLocked}
              onClick={() => setShowRebootDialog(true)}
              className={cn(HOST_ITEM, "text-danger hover:text-danger")}
            >
              Reboot
            </Button>
          </div>
        </div>
      </div>

      {/* Shell frame — fixed dark surface in both themes (the `dark` class scopes the
          dark tokens to it); xterm FitAddon follows ResizeObserver */}
      <div
        className={cn(
          "dark flex min-h-0 flex-1 flex-col overflow-hidden rounded-lg border border-border text-foreground",
          isFullscreen &&
            "fixed inset-0 z-50 m-0 h-screen w-screen rounded-none border-0",
        )}
        style={{ ...DARK_SCOPE, background: termBg }}
      >
        <div className="flex h-9 shrink-0 items-center justify-between gap-3 border-b border-border bg-card pl-3 pr-1">
          <div className="flex min-w-0 items-center gap-2">
            <span
              aria-hidden="true"
              className={cn(
                "size-2 shrink-0 rounded-full",
                connected
                  ? "bg-ok"
                  : connecting
                    ? "animate-pulse bg-warn motion-reduce:animate-none"
                    : "bg-subtle",
              )}
            />
            <span className="truncate font-mono text-xs text-foreground">
              root@docklift
            </span>
            <span className="shrink-0 text-xs text-muted-foreground">
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
            className="size-7 shrink-0 text-muted-foreground pointer-coarse:size-10"
            title={isFullscreen ? "Exit full screen" : "Full screen"}
          >
            {isFullscreen ? <Minimize2 /> : <Maximize2 />}
          </Button>
        </div>

        <div className="relative flex min-h-0 flex-1 flex-col">
          <div
            ref={terminalRef}
            className="min-h-0 flex-1 py-2 pl-3 pr-1"
            style={{ background: termBg }}
          />
          {!atBottom && (
            <button
              type="button"
              onClick={() => {
                xtermRef.current?.scrollToBottom();
                xtermRef.current?.focus();
                setAtBottom(true);
              }}
              title="Scroll to the latest output"
              className="absolute bottom-3 right-5 z-10 flex h-8 items-center gap-1.5 rounded-md border border-border bg-card px-2.5 text-xs font-medium text-muted-foreground shadow-popover transition-colors duration-150 hover:text-foreground focus-visible:outline-hidden focus-visible:ring-3 focus-visible:ring-brand/30"
            >
              <ArrowDown className="size-3.5" />
              Latest
            </button>
          )}
        </div>
      </div>

      <Dialog
        open={showUpgradeConfirm}
        onOpenChange={(open) => {
          if (!open) dismissUpgradeConfirm();
          else setShowUpgradeConfirm(true);
        }}
      >
        <DialogContent>
          <DialogHeader>
            <DialogTitle>Upgrade Docklift?</DialogTitle>
            <DialogDescription>
              The panel will go offline while the stack rebuilds. Keep this tab
              open — when it returns, refresh.
            </DialogDescription>
          </DialogHeader>

          <dl className="divide-y divide-border border-y border-border text-[13px] leading-5">
            <div className="flex items-baseline justify-between gap-3 py-2">
              <dt className="text-muted-foreground">Current version</dt>
              <dd className="font-mono text-foreground">
                {versionLoading && !versionInfo
                  ? "…"
                  : formatVersion(versionInfo?.current)}
              </dd>
            </div>
            <div className="flex items-baseline justify-between gap-3 py-2">
              <dt className="text-muted-foreground">Installing</dt>
              <dd className="font-mono font-medium text-foreground">
                {upgradeTargetLabel}
              </dd>
            </div>
          </dl>

          {upgradeTargetLabel === "unknown" ? (
            <p className="rounded-md border border-warn/40 bg-secondary/60 px-3 py-2.5 text-[13px] leading-5 text-foreground">
              Could not resolve the target version from GitHub. You can still start — upgrade.sh will pull the latest release.
            </p>
          ) : (
            <p className="text-[13px] leading-5 text-muted-foreground">
              Usually ready in 1–2 minutes. Deployed apps keep running; only the Docklift panel restarts.
            </p>
          )}

          <form
            className="space-y-4"
            onSubmit={(e) => {
              e.preventDefault();
              void startUpgradeFromConfirm();
            }}
          >
            <div className="space-y-1.5">
              <label className="block text-[13px] font-medium" htmlFor="upgrade-password">
                Account password
              </label>
              <Input
                id="upgrade-password"
                type="password"
                placeholder="Enter password to start upgrade"
                value={upgradePassword}
                onChange={(e) => {
                  setUpgradePassword(e.target.value);
                  if (upgradePasswordError) setUpgradePasswordError("");
                }}
                className="font-mono"
                autoComplete="current-password"
                autoFocus
                disabled={isProcessing}
              />
              {upgradePasswordError ? (
                <p className="text-[13px] text-danger">{upgradePasswordError}</p>
              ) : (
                <p className="text-xs text-muted-foreground">
                  JWT alone is not enough. Wrong password stays here so you can try again.
                </p>
              )}
            </div>
            <DialogFooter>
              <Button
                type="button"
                variant="outline"
                onClick={dismissUpgradeConfirm}
                disabled={isProcessing}
              >
                Cancel
              </Button>
              <Button
                type="submit"
                disabled={isProcessing || versionLoading || !upgradePassword.trim()}
              >
                {isProcessing
                  ? "Starting…"
                  : versionLoading
                    ? "Checking version…"
                    : "Start upgrade"}
              </Button>
            </DialogFooter>
          </form>
        </DialogContent>
      </Dialog>

      <Dialog open={showUpdateConfirm} onOpenChange={setShowUpdateConfirm}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>Update host packages?</DialogTitle>
            <DialogDescription>
              Runs <span className="font-mono text-foreground">apt update</span>{" "}
              and upgrade on the server. This can take several minutes.
            </DialogDescription>
          </DialogHeader>
          <p className="text-[13px] leading-5 text-muted-foreground">
            The panel usually stays up. If this page stops responding, wait 1–2
            minutes, then refresh.
          </p>
          <DialogFooter>
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
          if (!open) {
            setWaitState(null);
            ensureShellConnected();
          }
        }}
      >
        <DialogContent
          showCloseButton={false}
          onPointerDownOutside={(e) => e.preventDefault()}
          onEscapeKeyDown={(e) => e.preventDefault()}
        >
          <DialogHeader>
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
            <dl className="divide-y divide-border border-y border-border text-[13px] leading-5">
              <div className="flex items-baseline justify-between gap-3 py-2">
                <dt className="text-muted-foreground">Target</dt>
                <dd className="font-mono font-medium text-foreground">
                  {formatVersion(waitState.latest)}
                </dd>
              </div>
              {waitState.current && (
                <div className="flex items-baseline justify-between gap-3 py-2">
                  <dt className="text-muted-foreground">From</dt>
                  <dd className="font-mono text-foreground">
                    {formatVersion(waitState.current)}
                  </dd>
                </div>
              )}
            </dl>
          )}

          {!waitState?.simulated && (
            <p className="text-[13px] leading-5 text-muted-foreground">
              {refreshInSec > 0 ? (
                <>
                  Try refreshing in about{" "}
                  <span className="font-mono text-foreground">
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

          <DialogFooter>
            {(waitState?.simulated || refreshInSec === 0) && (
              <Button
                variant="outline"
                onClick={() => {
                  setWaitState(null);
                  ensureShellConnected();
                }}
              >
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
        <DialogContent>
          <DialogHeader>
            <DialogTitle>Reboot this server?</DialogTitle>
            <DialogDescription>
              Every project goes offline for a few minutes. In-flight
              deployments stop. The machine restarts completely.
            </DialogDescription>
          </DialogHeader>
          <DialogFooter>
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
        <DialogContent>
          <DialogHeader>
            <DialogTitle>Reset Docklift services?</DialogTitle>
            <DialogDescription>
              Restarts core Docklift containers and workers. Running app
              projects stay up unless they depend on the panel briefly.
            </DialogDescription>
          </DialogHeader>
          <DialogFooter>
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
        <DialogContent>
          <DialogHeader>
            <DialogTitle>Purge unused Docklift images?</DialogTitle>
            <DialogDescription>
              Removes unused Docklift images outside keep-2 and clears all
              BuildKit cache. No host prune, journal wipe, or restart of other
              workloads.
            </DialogDescription>
          </DialogHeader>
          <DialogFooter>
            <Button variant="outline" onClick={() => setShowPurgeDialog(false)}>
              Cancel
            </Button>
            <Button
              variant="destructive"
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
        <DialogContent>
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
            <div className="space-y-1.5">
              <Input
                ref={passwordInputRef}
                type="password"
                placeholder="Account password"
                value={passwordInput}
                onChange={(e) => setPasswordInput(e.target.value)}
                className="font-mono"
                autoComplete="current-password"
                autoFocus
              />
              {passwordError && (
                <p className="text-[13px] text-danger">{passwordError}</p>
              )}
            </div>
            <DialogFooter>
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
