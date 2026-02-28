// TerminalView component - interactive xterm.js shell with system controls
"use client";

import { useSearchParams } from "next/navigation";
import { useState, useEffect, useRef, useCallback } from "react";
import { 
  Terminal as TerminalIcon, 
  RefreshCw, 
  Power, 
  AlertTriangle,
  CheckCircle2,
  XCircle,
  Trash2,
  Maximize2,
  Minimize2,
} from "lucide-react";
import { Card } from "@/components/ui/card";
import { Button } from "@/components/ui/button";
import { 
  Dialog, 
  DialogContent, 
  DialogHeader, 
  DialogTitle, 
  DialogDescription, 
  DialogFooter 
} from "@/components/ui/dialog";
import { Input } from "@/components/ui/input";
import { API_URL } from "@/lib/utils";
import { getAuthHeaders } from "@/lib/auth";
import { toast } from "sonner";
import { cn } from "@/lib/utils";

export function TerminalView() {
  const [showRebootDialog, setShowRebootDialog] = useState(false);
  const [showResetDialog, setShowResetDialog] = useState(false);
  const [showPurgeDialog, setShowPurgeDialog] = useState(false);
  const [isProcessing, setIsProcessing] = useState(false);
  const [showPasswordDialog, setShowPasswordDialog] = useState(false);
  const [passwordInput, setPasswordInput] = useState("");
  const [passwordError, setPasswordError] = useState("");
  const [connected, setConnected] = useState(false);
  const [connecting, setConnecting] = useState(false);
  const [isFullscreen, setIsFullscreen] = useState(false);
  
  const terminalRef = useRef<HTMLDivElement>(null);
  const xtermRef = useRef<any>(null);
  const wsRef = useRef<WebSocket | null>(null);
  const fitAddonRef = useRef<any>(null);
  const passwordInputRef = useRef<HTMLInputElement>(null);
  const passwordResolveRef = useRef<((pwd: string) => void) | null>(null);
  const searchParams = useSearchParams();

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
        letterSpacing: 0.5,
        allowTransparency: true,
        theme: {
          background: "#0c0c0c",
          foreground: "#d4d4d4",
          cursor: "#22d3ee",
          cursorAccent: "#0c0c0c",
          selectionBackground: "#22d3ee33",
          selectionForeground: "#ffffff",
          black: "#1e1e1e",
          red: "#f87171",
          green: "#4ade80",
          yellow: "#facc15",
          blue: "#60a5fa",
          magenta: "#c084fc",
          cyan: "#22d3ee",
          white: "#d4d4d4",
          brightBlack: "#525252",
          brightRed: "#fca5a5",
          brightGreen: "#86efac",
          brightYellow: "#fde047",
          brightBlue: "#93c5fd",
          brightMagenta: "#d8b4fe",
          brightCyan: "#67e8f9",
          brightWhite: "#ffffff",
        },
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

  const promptPassword = useCallback((): Promise<string> => {
    return new Promise((resolve) => {
      passwordResolveRef.current = resolve;
      setPasswordError("");
      setPasswordInput("");
      setShowPasswordDialog(true);
    });
  }, []);

  const handlePasswordSubmit = useCallback(() => {
    if (!passwordInput.trim()) return;
    const pwd = passwordInput;
    setPasswordInput("");
    setShowPasswordDialog(false);
    if (passwordResolveRef.current) {
      passwordResolveRef.current(pwd);
      passwordResolveRef.current = null;
    }
  }, [passwordInput]);

  const connectWebSocket = useCallback(async (term: any, fitAddon: any) => {
    setConnecting(true);

    // Get JWT token directly from localStorage
    const token = typeof window !== "undefined" ? localStorage.getItem("docklift_token") : null;

    if (!token) {
      term.writeln("  \x1b[1;31m✗ Not authenticated. Please log in first.\x1b[0m");
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
            // Prompt for password
            const password = await promptPassword();
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
            // Re-prompt
            const password = await promptPassword();
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
  }, [promptPassword]);

  // System action handler (same as before)
  const handleSystemAction = async (action: 'reboot' | 'reset' | 'purge' | 'update-system' | 'upgrade') => {
    setIsProcessing(true);
    setShowRebootDialog(false);
    setShowResetDialog(false);
    setShowPurgeDialog(false);
    
    try {
      const res = await fetch(`${API_URL}/api/system/${action}`, {
        method: "POST",
        headers: getAuthHeaders(),
      });
      const data = await res.json();
      
      if (!res.ok) throw new Error(data.error || "Action failed");
      toast.success(data.message || `${action} successful`);
    } catch (err: any) {
      toast.error(err.message || `Failed to ${action} server`);
    } finally {
      setIsProcessing(false);
    }
  };

  return (
    <div className="flex flex-col gap-5 animate-in fade-in slide-in-from-bottom-4 duration-700">
      {/* Control Center - Action Cards */}
      <div className="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-4 gap-3">
        {/* System Update Card */}
        <Card className="p-3 bg-background border-border shadow-sm hover:shadow-md transition-shadow duration-300 rounded-xl group border-l-4 border-l-cyan-500">
          <div className="flex items-center justify-between gap-3">
            <div className="flex items-center gap-3">
              <div className="p-2 rounded-lg bg-cyan-500/10">
                <RefreshCw className="h-4 w-4 text-cyan-500" />
              </div>
              <div className="flex flex-col">
                <h3 className="font-bold text-sm">Update System</h3>
                <p className="text-[10px] text-muted-foreground leading-tight">apt update & upgrade</p>
              </div>
            </div>
            <Button 
              variant="secondary"
              size="sm"
              onClick={() => handleSystemAction('update-system')}
              disabled={isProcessing}
              className="font-bold h-8 px-3 rounded-lg shadow-sm bg-cyan-500/10 text-cyan-500 hover:bg-cyan-500/20 border border-cyan-500/20"
            >
              Update
            </Button>
          </div>
        </Card>

        {/* Docklift Upgrade Card */}
        <Card className="p-3 bg-background border-border shadow-sm hover:shadow-md transition-shadow duration-300 rounded-xl group border-l-4 border-l-emerald-500">
          <div className="flex items-center justify-between gap-3">
            <div className="flex items-center gap-3">
              <div className="p-2 rounded-lg bg-emerald-500/10">
                <TerminalIcon className="h-4 w-4 text-emerald-500" />
              </div>
              <div className="flex flex-col">
                <h3 className="font-bold text-sm">Upgrade Docklift</h3>
                <p className="text-[10px] text-muted-foreground leading-tight">Latest version</p>
              </div>
            </div>
            <Button 
              variant="secondary"
              size="sm"
              onClick={() => handleSystemAction('upgrade')}
              disabled={isProcessing}
              className="font-bold h-8 px-3 rounded-lg shadow-sm bg-emerald-500/10 text-emerald-500 hover:bg-emerald-500/20 border border-emerald-500/20"
            >
              Upgrade
            </Button>
          </div>
        </Card>

        {/* Reboot Card */}
        <Card className="p-3 bg-background border-border shadow-sm hover:shadow-md transition-shadow duration-300 rounded-xl group border-l-4 border-l-rose-500">
          <div className="flex items-center justify-between gap-3">
            <div className="flex items-center gap-3">
              <div className="p-2 rounded-lg bg-rose-500/10">
                <Power className="h-4 w-4 text-rose-500" />
              </div>
              <div className="flex flex-col">
                <h3 className="font-bold text-sm">Reboot Server</h3>
                <p className="text-[10px] text-muted-foreground leading-tight">Full system restart</p>
              </div>
            </div>
            <Button 
              variant="destructive"
              size="sm"
              onClick={() => setShowRebootDialog(true)}
              className="font-bold h-8 px-3 rounded-lg shadow-sm"
            >
              Reboot
            </Button>
          </div>
        </Card>

        {/* Reset Card */}
        <Card className="p-3 bg-background border-border shadow-sm hover:shadow-md transition-shadow duration-300 rounded-xl group border-l-4 border-l-amber-500">
          <div className="flex items-center justify-between gap-3">
            <div className="flex items-center gap-3">
              <div className="p-2 rounded-lg bg-amber-500/10">
                <RefreshCw className="h-4 w-4 text-amber-500" />
              </div>
              <div className="flex flex-col">
                <h3 className="font-bold text-sm">Reset Services</h3>
                <p className="text-[10px] text-muted-foreground leading-tight">Docker restart</p>
              </div>
            </div>
            <Button 
              variant="outline"
              size="sm"
              onClick={() => setShowResetDialog(true)}
              className="font-bold h-8 px-3 rounded-lg shadow-sm border-amber-500/30 text-amber-500 hover:bg-amber-500/10"
            >
              Reset
            </Button>
          </div>
        </Card>
      </div>

      {/* Interactive Terminal */}
      <Card 
        className={cn(
          "flex flex-col bg-[#0c0c0c] border border-[#2a2a2a] overflow-hidden shadow-[0_20px_50px_rgba(0,0,0,0.5)] rounded-2xl relative transition-all duration-300",
          isFullscreen ? "fixed inset-0 z-50 rounded-none border-0 h-screen w-screen m-0" : ""
        )}
      >
        {/* Subtle glow */}
        <div className="absolute inset-0 bg-[radial-gradient(circle_at_50%_0%,rgba(34,211,238,0.03),transparent)] pointer-events-none" />
        
        {/* Terminal Header */}
        <div className="px-3 sm:px-4 py-2.5 border-b border-[#2a2a2a] flex items-center justify-between bg-[#111]/90 backdrop-blur-md z-10">
          <div className="flex items-center gap-3 sm:gap-5">
            <div className="flex gap-1.5 sm:gap-2">
              <div className="w-3 h-3 rounded-full bg-[#ff5f56] shadow-[0_0_8px_rgba(255,95,86,0.25)]" />
              <div className="w-3 h-3 rounded-full bg-[#ffbd2e] shadow-[0_0_8px_rgba(255,189,46,0.25)]" />
              <div className="w-3 h-3 rounded-full bg-[#27c93f] shadow-[0_0_8px_rgba(39,201,63,0.25)]" />
            </div>
            <div className="hidden sm:flex items-center gap-2 px-3 py-1 bg-[#1a1a1a] rounded-lg border border-[#252525]">
              <TerminalIcon className="h-3.5 w-3.5 text-cyan-400" />
              <span className="text-[11px] font-bold text-[#888] tracking-[0.15em] uppercase">bash — root@docklift</span>
            </div>
          </div>
          <div className="flex items-center gap-2">
            <div className={cn(
              "flex items-center gap-1.5 px-2.5 py-1 rounded-full border text-[10px] font-bold tracking-wide transition-all",
              connected 
                ? "text-emerald-400 bg-emerald-400/10 border-emerald-400/20 shadow-[0_0_10px_rgba(52,211,153,0.08)]"
                : connecting
                  ? "text-amber-400 bg-amber-400/10 border-amber-400/20"
                  : "text-zinc-500 bg-zinc-500/10 border-zinc-500/20"
            )}>
              <div className={cn(
                "w-1.5 h-1.5 rounded-full",
                connected ? "bg-emerald-400 animate-pulse" : connecting ? "bg-amber-400 animate-pulse" : "bg-zinc-500"
              )} />
              <span className="hidden sm:inline">{connected ? "CONNECTED" : connecting ? "CONNECTING" : "OFFLINE"}</span>
            </div>

            <Button
              variant="ghost"
              size="sm"
              onClick={() => {
                const newState = !isFullscreen;
                setIsFullscreen(newState);
                // Give layout time to adjust before fitting
                setTimeout(() => {
                  if (fitAddonRef.current) {
                    try { fitAddonRef.current.fit(); } catch {}
                    // Notify backend of resize
                    if (wsRef.current?.readyState === WebSocket.OPEN && xtermRef.current) {
                      try {
                        wsRef.current.send(JSON.stringify({ 
                          type: "resize", 
                          cols: xtermRef.current.cols, 
                          rows: xtermRef.current.rows 
                        }));
                      } catch {}
                    }
                  }
                }, 100);
              }}
              className="h-6 w-6 p-0 text-muted-foreground hover:text-foreground hover:bg-white/10 rounded-md ml-1"
              title={isFullscreen ? "Exit Full Screen" : "Full Screen"}
            >
              {isFullscreen ? <Minimize2 className="h-3.5 w-3.5" /> : <Maximize2 className="h-3.5 w-3.5" />}
            </Button>
          </div>
        </div>

        <div 
          ref={terminalRef}
          className={cn(
            "flex-1 p-1 z-10",
            !isFullscreen && "min-h-[280px] sm:min-h-[400px] md:min-h-[500px] lg:min-h-[600px]"
          )}
          style={{ background: "#0c0c0c" }}
        />
      </Card>

      {/* Reboot Confirmation Dialog */}
      <Dialog open={showRebootDialog} onOpenChange={setShowRebootDialog}>
        <DialogContent className="sm:max-w-md bg-background border-border shadow-2xl rounded-3xl">
          <DialogHeader className="space-y-4">
            <div className="mx-auto w-16 h-16 rounded-2xl bg-rose-500/10 flex items-center justify-center">
              <Power className="h-8 w-8 text-rose-500" />
            </div>
            <div className="text-center space-y-2">
              <DialogTitle className="text-2xl font-bold tracking-tight">Full System Reboot</DialogTitle>
              <DialogDescription className="text-muted-foreground text-sm leading-relaxed max-w-[280px] mx-auto">
                Safety check: you are about to restart the physical server machine.
              </DialogDescription>
            </div>
          </DialogHeader>
          
          <div className="space-y-3 my-4">
            <div className="p-4 rounded-2xl bg-secondary/30 border border-border flex items-center gap-4">
              <AlertTriangle className="h-5 w-5 text-rose-500 shrink-0" />
              <p className="text-sm font-semibold">Every project will go offline (2m)</p>
            </div>
            <div className="p-4 rounded-2xl bg-secondary/30 border border-border flex items-center gap-4">
              <XCircle className="h-5 w-5 text-rose-500 shrink-0" />
              <p className="text-sm font-semibold">Ongoing deployments will be lost</p>
            </div>
          </div>

          <DialogFooter className="flex flex-col sm:flex-row gap-3 pt-2">
            <Button
              variant="ghost"
              onClick={() => setShowRebootDialog(false)}
              className="flex-1 font-bold text-base h-12 rounded-2xl"
            >
              Cancel
            </Button>
            <Button
              onClick={() => handleSystemAction('reboot')}
              className="flex-1 rounded-2xl bg-gradient-to-r from-violet-500 to-purple-600 hover:from-violet-600 hover:to-purple-700 text-white font-bold text-base h-12 shadow-xl shadow-violet-500/20"
            >
              Restart Server
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>

      {/* Reset Confirmation Dialog */}
      <Dialog open={showResetDialog} onOpenChange={setShowResetDialog}>
        <DialogContent className="sm:max-w-md bg-background border-border shadow-2xl rounded-3xl">
          <DialogHeader className="space-y-4">
            <div className="mx-auto w-16 h-16 rounded-2xl bg-amber-500/10 flex items-center justify-center">
              <RefreshCw className="h-8 w-8 text-amber-500" />
            </div>
            <div className="text-center space-y-2">
              <DialogTitle className="text-2xl font-bold tracking-tight">Reset Stack Services</DialogTitle>
              <DialogDescription className="text-muted-foreground text-sm leading-relaxed max-w-[280px] mx-auto">
                Refreshes core Docklift components and background workers.
              </DialogDescription>
            </div>
          </DialogHeader>
          
          <div className="space-y-3 my-4">
            <div className="p-4 rounded-2xl bg-secondary/30 border border-border flex items-center gap-4">
              <CheckCircle2 className="h-5 w-5 text-amber-500 shrink-0" />
              <p className="text-sm font-semibold">Fixes service communication errors</p>
            </div>
            <div className="p-4 rounded-2xl bg-secondary/30 border border-border flex items-center gap-4">
              <CheckCircle2 className="h-5 w-5 text-amber-500 shrink-0" />
              <p className="text-sm font-semibold">Flushes background worker queues</p>
            </div>
          </div>

          <DialogFooter className="flex flex-col sm:flex-row gap-3 pt-2">
            <Button
              variant="ghost"
              onClick={() => setShowResetDialog(false)}
              className="flex-1 font-bold text-base h-12 rounded-2xl"
            >
              Wait, Cancel
            </Button>
            <Button
              variant="warning"
              onClick={() => handleSystemAction('reset')}
              className="flex-1 rounded-2xl bg-gradient-to-r from-violet-500 to-purple-600 hover:from-violet-600 hover:to-purple-700 text-white font-bold text-base h-12 shadow-xl shadow-violet-500/20 border-0"
            >
              Reset Services
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>

      {/* Purge Confirmation Dialog */}
      <Dialog open={showPurgeDialog} onOpenChange={setShowPurgeDialog}>
        <DialogContent className="sm:max-w-md bg-background border-border shadow-2xl">
          <DialogHeader className="space-y-3">
            <div className="mx-auto w-14 h-14 rounded-2xl bg-cyan-500/10 flex items-center justify-center border border-cyan-500/10">
              <Trash2 className="h-6 w-6 text-cyan-500" />
            </div>
            <div className="text-center space-y-1">
              <DialogTitle className="text-xl font-bold tracking-tight">Purge Resources</DialogTitle>
              <DialogDescription className="text-muted-foreground text-sm">
                Complete cleanup: Docker + HOST system. Safe operations only.
              </DialogDescription>
            </div>
          </DialogHeader>
          
          <div className="space-y-2 my-2">
            <div className="p-3 rounded-xl bg-secondary/50 border border-border flex items-center gap-3">
              <CheckCircle2 className="h-4 w-4 text-cyan-500" />
              <p className="text-xs font-bold">Docker cleanup + Restart user containers</p>
            </div>
            <div className="p-3 rounded-xl bg-secondary/50 border border-border flex items-center gap-3">
              <CheckCircle2 className="h-4 w-4 text-cyan-500" />
              <p className="text-xs font-bold">HOST cleanup (cache, logs, apt, temp)</p>
            </div>
            <div className="p-3 rounded-xl bg-secondary/50 border border-border flex items-center gap-3">
              <CheckCircle2 className="h-4 w-4 text-cyan-500" />
              <p className="text-xs font-bold">Clear swap if safe (30%+ RAM free)</p>
            </div>
          </div>

          <DialogFooter className="flex flex-col sm:flex-row gap-2 mt-2">
            <Button
              variant="ghost"
              onClick={() => setShowPurgeDialog(false)}
              className="flex-1 rounded-xl font-bold h-10"
            >
              Cancel
            </Button>
            <Button
              variant="success"
              onClick={() => handleSystemAction('purge')}
              className="flex-1 rounded-xl font-bold h-10 shadow-lg shadow-emerald-500/20"
            >
              Purge All
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>

      {/* Terminal Password Verification Dialog */}
      <Dialog open={showPasswordDialog} onOpenChange={(open) => {
        if (!open) {
          setShowPasswordDialog(false);
          setPasswordInput("");
          setPasswordError("");
        }
      }}>
        <DialogContent className="sm:max-w-md bg-background border-border shadow-2xl rounded-3xl">
          <DialogHeader className="space-y-4">
            <div className="mx-auto w-16 h-16 rounded-2xl bg-cyan-500/10 flex items-center justify-center border border-cyan-500/20">
              <TerminalIcon className="h-8 w-8 text-cyan-500" />
            </div>
            <div className="text-center space-y-2">
              <DialogTitle className="text-2xl font-bold tracking-tight">Terminal Access</DialogTitle>
              <DialogDescription className="text-muted-foreground text-sm leading-relaxed max-w-[300px] mx-auto">
                Verify your password to open an interactive terminal session.
              </DialogDescription>
            </div>
          </DialogHeader>

          <form onSubmit={(e) => { e.preventDefault(); handlePasswordSubmit(); }} className="space-y-4 my-2">
            <div className="space-y-2">
              <Input
                ref={passwordInputRef}
                type="password"
                placeholder="Enter your account password"
                value={passwordInput}
                onChange={(e) => setPasswordInput(e.target.value)}
                className="h-12 rounded-xl bg-secondary/30 border-border text-center font-mono tracking-widest text-lg"
                autoFocus
              />
              {passwordError && (
                <p className="text-rose-400 text-xs font-semibold text-center">{passwordError}</p>
              )}
            </div>

            <div className="p-3 rounded-xl bg-amber-500/5 border border-amber-500/20 flex items-start gap-3">
              <AlertTriangle className="h-4 w-4 text-amber-500 shrink-0 mt-0.5" />
              <p className="text-[11px] text-amber-500/80 font-medium leading-relaxed">
                This opens a full interactive shell session. Your password is only used for verification and is never stored.
              </p>
            </div>

            <DialogFooter className="flex flex-col sm:flex-row gap-3 pt-1">
              <Button
                type="button"
                variant="ghost"
                onClick={() => { setShowPasswordDialog(false); setPasswordInput(""); setPasswordError(""); }}
                className="flex-1 font-bold text-base h-12 rounded-2xl"
              >
                Cancel
              </Button>
              <Button
                type="submit"
                disabled={!passwordInput.trim()}
                className="flex-1 rounded-2xl bg-gradient-to-r from-cyan-500 to-blue-600 hover:from-cyan-600 hover:to-blue-700 text-white font-bold text-base h-12 shadow-xl shadow-cyan-500/20"
              >
                Open Terminal
              </Button>
            </DialogFooter>
          </form>
        </DialogContent>
      </Dialog>
    </div>
  );
}
