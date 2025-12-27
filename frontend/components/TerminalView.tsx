"use client";

import { useState, useEffect, useRef } from "react";
import { 
  Terminal as TerminalIcon, 
  RefreshCw, 
  Trash2, 
  Power, 
  Play, 
  ChevronRight, 
  History,
  Info,
  AlertTriangle,
  CheckCircle2,
  XCircle,
  Clock,
  Search,
  Settings as SettingsIcon,
  Maximize2
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
import { API_URL } from "@/lib/utils";
import { toast } from "sonner";
import { cn } from "@/lib/utils";

interface CommandLog {
  id: string;
  command: string;
  output: string;
  error?: string;
  timestamp: Date;
}

export function TerminalView() {
  const [command, setCommand] = useState("");
  const [logs, setLogs] = useState<CommandLog[]>([]);
  const [executing, setExecuting] = useState(false);
  const [showRebootDialog, setShowRebootDialog] = useState(false);
  const [showResetDialog, setShowResetDialog] = useState(false);
  const [showPurgeDialog, setShowPurgeDialog] = useState(false);
  const [isProcessing, setIsProcessing] = useState(false);
  
  const scrollRef = useRef<HTMLDivElement>(null);
  const inputRef = useRef<HTMLInputElement>(null);

  useEffect(() => {
    if (scrollRef.current) {
      scrollRef.current.scrollTop = scrollRef.current.scrollHeight;
    }
  }, [logs]);

  // Focus input on mount and after execution
  useEffect(() => {
    if (!executing && inputRef.current) {
      inputRef.current.focus();
    }
  }, [executing]);

  const handleExecute = async (e?: React.FormEvent) => {
    if (e) e.preventDefault();
    if (!command.trim() || executing) return;

    const currentCommand = command.trim();
    setCommand("");
    setExecuting(true);

    try {
      const res = await fetch(`${API_URL}/api/system/execute`, {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ command: currentCommand }),
      });
      
      const data = await res.json();
      
      const newLog: CommandLog = {
        id: Math.random().toString(36).substr(2, 9),
        command: currentCommand,
        output: data.output || "",
        error: data.error || (res.ok ? undefined : "Unknown error"),
        timestamp: new Date(),
      };
      
      setLogs(prev => [...prev.slice(-49), newLog]); // Keep last 50
    } catch (err: any) {
      toast.error("Execution failed: " + err.message);
    } finally {
      setExecuting(false);
    }
  };

  const handleSystemAction = async (action: 'reboot' | 'reset' | 'purge') => {
    setIsProcessing(true);
    setShowRebootDialog(false);
    setShowResetDialog(false);
    setShowPurgeDialog(false);
    
    try {
      const res = await fetch(`${API_URL}/api/system/${action}`, {
        method: "POST",
      });
      const data = await res.json();
      
      if (!res.ok) throw new Error(data.error || "Action failed");
      
      toast.success(data.message || `${action} successful`);
      
      const newLog: CommandLog = {
        id: Math.random().toString(36).substr(2, 9),
        command: `system:${action}`,
        output: data.message || `System ${action} executed.`,
        timestamp: new Date(),
      };
      setLogs(prev => [...prev.slice(-49), newLog]);
      
    } catch (err: any) {
      toast.error(err.message || `Failed to ${action} server`);
    } finally {
      setIsProcessing(false);
    }
  };

  return (
    <div className="flex flex-col gap-5 animate-in fade-in slide-in-from-bottom-4 duration-700">
      {/* Control Center - Ultra Compact */}
      <div className="grid grid-cols-1 md:grid-cols-2 gap-3">
        {/* Reboot Card */}
        <Card className="p-3 bg-background border-border shadow-sm hover:shadow-md transition-shadow duration-300 rounded-xl group border-l-4 border-l-rose-500">
          <div className="flex items-center justify-between gap-4">
            <div className="flex items-center gap-3">
              <div className="p-2 rounded-lg bg-rose-500/10">
                <Power className="h-4 w-4 text-rose-500" />
              </div>
              <div className="flex flex-col">
                <h3 className="font-bold text-sm">Reboot</h3>
                <p className="text-[10px] text-muted-foreground leading-tight">Full system restart</p>
              </div>
            </div>
            <Button 
              variant="destructive"
              size="sm"
              onClick={() => setShowRebootDialog(true)}
              className="font-bold h-8 px-4 rounded-lg shadow-sm"
            >
              Restart
            </Button>
          </div>
        </Card>

        {/* Reset Card */}
        <Card className="p-3 bg-background border-border shadow-sm hover:shadow-md transition-shadow duration-300 rounded-xl group border-l-4 border-l-amber-500">
          <div className="flex items-center justify-between gap-4">
            <div className="flex items-center gap-3">
              <div className="p-2 rounded-lg bg-amber-500/10">
                <RefreshCw className="h-4 w-4 text-amber-500" />
              </div>
              <div className="flex flex-col">
                <h3 className="font-bold text-sm">Reset</h3>
                <p className="text-[10px] text-muted-foreground leading-tight">Service refresh</p>
              </div>
            </div>
            <Button 
              variant="warning"
              size="sm"
              onClick={() => setShowResetDialog(true)}
              className="font-bold h-8 px-4 rounded-lg shadow-sm"
            >
              Reset
            </Button>
          </div>
        </Card>
      </div>

      {/* Terminal View */}
      <Card className="flex-1 min-h-[600px] flex flex-col bg-[#0d0d0d] border-[#1a1a1a] overflow-hidden shadow-[0_20px_50px_rgba(0,0,0,0.5)] rounded-3xl relative">
        {/* Glow effect */}
        <div className="absolute inset-0 bg-[radial-gradient(circle_at_50%_0%,rgba(56,189,248,0.05),transparent)] pointer-events-none" />
        
        {/* Mac-style Terminal Header */}
        <div className="p-4 border-b border-[#1a1a1a] flex items-center justify-between bg-[#141414]/90 backdrop-blur-md z-10">
          <div className="flex items-center gap-5">
            <div className="flex gap-2">
              <div className="w-3.5 h-3.5 rounded-full bg-[#ff5f56] shadow-[0_0_10px_rgba(255,95,86,0.3)]" />
              <div className="w-3.5 h-3.5 rounded-full bg-[#ffbd2e] shadow-[0_0_10px_rgba(255,189,46,0.3)]" />
              <div className="w-3.5 h-3.5 rounded-full bg-[#27c93f] shadow-[0_0_10px_rgba(39,201,63,0.3)]" />
            </div>
            <div className="flex items-center gap-2.5 px-4 py-1.5 bg-[#1a1a1a] rounded-lg border border-[#2a2a2a] shadow-inner">
              <TerminalIcon className="h-4 w-4 text-cyan-400" />
              <span className="text-[11px] font-bold text-[#aaa] tracking-[0.2em] uppercase">docklift@root:~</span>
            </div>
          </div>
          <div className="flex items-center gap-3">
             <div className="flex items-center gap-2.5 text-[10px] font-bold text-emerald-400 bg-emerald-400/10 px-3 py-1 rounded-full border border-emerald-400/20 shadow-[0_0_15px_rgba(52,211,153,0.1)]">
                <div className="w-2 h-2 rounded-full bg-emerald-400 animate-pulse" />
                SYSTEM ACTIVE
             </div>
             <Button 
              variant="ghost" 
              size="sm" 
              className="h-8 px-3 text-[10px] font-bold uppercase tracking-widest text-[#666] hover:text-rose-400 hover:bg-[#1a1a1a] rounded-lg border border-transparent hover:border-rose-400/20 transition-all"
              onClick={() => setLogs([])}
            >
              Clear
            </Button>
          </div>
        </div>

        {/* Output Scroll Area */}
        <div 
          ref={scrollRef}
          className="flex-1 overflow-y-auto p-6 font-mono text-[14px] space-y-4 scrollbar-thin scrollbar-thumb-[#252525] scroll-smooth z-10"
        >
          {logs.length === 0 ? (
            <div className="flex flex-col items-center justify-center h-full text-[#333] gap-4">
              <div className="p-4 rounded-full bg-[#111] border border-[#1a1a1a] shadow-inner">
                <History className="h-10 w-10 opacity-30 animate-pulse" />
              </div>
              <p className="font-bold uppercase tracking-[0.3em] text-[11px] opacity-40">System Console Ready</p>
            </div>
          ) : (
            logs.map(log => (
              <div key={log.id} className="space-y-2 group animate-in fade-in slide-in-from-left-2 duration-300">
                <div className="flex items-center gap-3 text-[#777]">
                  <span className="text-emerald-500 font-black text-base">➜</span>
                  <span className="text-cyan-400 font-bold tracking-tight">~</span>
                  <span className="font-bold text-[#eee] text-[15px]">{log.command}</span>
                  <span className="text-[10px] opacity-0 group-hover:opacity-100 transition-opacity ml-auto text-[#444] font-mono">
                    {log.timestamp.toLocaleTimeString()}
                  </span>
                </div>
                {log.output && (
                  <pre className="pl-8 text-[#ccc] whitespace-pre-wrap leading-relaxed select-text font-medium bg-[#111]/30 p-3 rounded-xl border border-[#1a1a1a]/50">
                    {log.output}
                  </pre>
                )}
                {log.error && (
                  <pre className="pl-8 text-rose-400 whitespace-pre-wrap leading-relaxed italic bg-rose-400/5 py-3 px-4 rounded-xl border-l-4 border-rose-500/50 font-semibold text-[13px]">
                    {log.error}
                  </pre>
                )}
              </div>
            ))
          )}
        </div>

        {/* Input Area */}
        <div className="p-5 bg-[#0a0a0a] border-t border-[#1a1a1a] z-10">
          <div className="relative group">
            <div className="absolute -inset-1 bg-gradient-to-r from-cyan-500/20 to-violet-500/20 rounded-2xl blur opacity-0 group-focus-within:opacity-100 transition duration-500" />
            <form onSubmit={handleExecute} className="relative flex items-center gap-4 bg-[#111] rounded-2xl px-5 py-3 border border-[#222] focus-within:border-cyan-500/40 transition-all shadow-2xl">
              <span className="text-cyan-500 font-black text-xl select-none group-focus-within:scale-110 transition-transform duration-300">λ</span>
              <input 
                ref={inputRef}
                type="text" 
                value={command}
                onChange={(e) => setCommand(e.target.value)}
                placeholder="Execute system command..."
                className="flex-1 bg-transparent border-none outline-none text-[#eee] font-mono text-[15px] placeholder:text-[#333] tracking-tight"
                autoComplete="off"
                disabled={executing}
                autoFocus
              />
              <button 
                type="submit" 
                disabled={!command.trim() || executing}
                className="flex items-center justify-center h-9 w-9 rounded-xl bg-[#1a1a1a] hover:bg-[#252525] text-[#555] hover:text-cyan-400 border border-[#2a2a2a] transition-all hover:scale-110 active:scale-95 shadow-lg shadow-black/50"
              >
                {executing ? (
                  <RefreshCw className="h-4 w-4 animate-spin" />
                ) : (
                  <Maximize2 className="h-4 w-4" />
                )}
              </button>
            </form>
          </div>
        </div>
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
                Deep clean of resource caches.
              </DialogDescription>
            </div>
          </DialogHeader>
          
          <div className="space-y-2 my-2">
            <div className="p-3 rounded-xl bg-secondary/50 border border-border flex items-center gap-3">
              <CheckCircle2 className="h-4 w-4 text-cyan-500" />
              <p className="text-xs font-bold">Prune Artifacts</p>
            </div>
            <div className="p-3 rounded-xl bg-secondary/50 border border-border flex items-center gap-3">
              <CheckCircle2 className="h-4 w-4 text-cyan-500" />
              <p className="text-xs font-bold">Clear Image Cache</p>
            </div>
            <div className="p-3 rounded-xl bg-secondary/50 border border-border flex items-center gap-3">
              <CheckCircle2 className="h-4 w-4 text-cyan-500" />
              <p className="text-xs font-bold">Drop Memory Pages</p>
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
    </div>
  );
}
