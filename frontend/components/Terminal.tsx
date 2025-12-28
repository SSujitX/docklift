// Terminal component - build log display with syntax highlighting and copy feature
"use client";

import { useEffect, useRef, useState } from "react";
import { cn } from "@/lib/utils";
import { Copy, Check, Terminal as TerminalIcon, Sparkles, Hash, Layers } from "lucide-react";
import { Button } from "./ui/button";

interface TerminalProps {
  logs: string;
  isBuilding?: boolean;
  className?: string;
}

export function Terminal({ logs, isBuilding, className }: TerminalProps) {
  const containerRef = useRef<HTMLDivElement>(null);
  const [autoScroll, setAutoScroll] = useState(true);
  const [copied, setCopied] = useState(false);

  useEffect(() => {
    if (autoScroll && containerRef.current) {
      containerRef.current.scrollTop = containerRef.current.scrollHeight;
    }
  }, [logs, autoScroll]);

  const handleScroll = () => {
    if (containerRef.current) {
      const { scrollTop, scrollHeight, clientHeight } = containerRef.current;
      setAutoScroll(scrollHeight - scrollTop - clientHeight < 50);
    }
  };

  const copyToClipboard = () => {
    navigator.clipboard.writeText(logs);
    setCopied(true);
    setTimeout(() => setCopied(false), 2000);
  };

  const formatLogs = (text: string) => {
    return text.split("\n").map((line, i) => {
      let lineClass = "text-zinc-400";
      let icon = null;
      
      // Success indicators
      if (line.includes("[✓]") || line.includes("✅") || line.toLowerCase().includes("success") || line.includes("DONE") || line.includes("Built")) {
        lineClass = "text-emerald-400 font-medium";
        if (line.includes("DONE") || line.includes("Built") || line.includes("✅")) icon = <Sparkles className="h-3 w-3 inline mr-2 opacity-50" />;
      } 
      // Error indicators
      else if (line.includes("[✗]") || line.includes("❌") || line.toLowerCase().includes("error") || line.toLowerCase().includes("failed")) {
        lineClass = "text-red-400 font-medium";
      }
      // Warning indicators  
      else if (line.toLowerCase().includes("warning") || line.includes("[!]")) {
        lineClass = "text-amber-400";
      }
      // Build steps (#1, #2, etc)
      else if (/^#\d+/.test(line.trim())) {
        lineClass = "text-cyan-400";
        icon = <Hash className="h-3 w-3 inline mr-1.5 opacity-40" />;
      }
      // Docker layer steps ([1/10], [2/10], etc)
      else if (/\[\s*\d+\/\d+\s*\]/.test(line)) {
        lineClass = "text-violet-400 font-medium";
        icon = <Layers className="h-3 w-3 inline mr-1.5 opacity-40" />;
      }
      // Info/build commands
      else if (line.includes("[+]") || line.includes("Building")) {
        lineClass = "text-blue-400";
      }
      // Prompt style
      else if (line.trim().startsWith("$")) {
        lineClass = "text-violet-400 font-semibold";
      }

      return (
        <div key={i} className={cn("leading-relaxed py-0.5 group/line flex items-start", lineClass)}>
          <span className="shrink-0 w-6 opacity-20 group-hover/line:opacity-50 transition-opacity text-[10px] select-none pt-0.5">
            {i + 1}
          </span>
          <span className="whitespace-pre-wrap flex-1">
            {icon}{line || "\u00A0"}
          </span>
        </div>
      );
    });
  };

  return (
    <div id="terminal-wrapper" className={cn("flex flex-col bg-[#09090b] rounded-2xl border border-white/[0.08] dark:border-white/[0.05] overflow-hidden shadow-2xl", className)}>
      {/* Terminal Header */}
      <div className="flex items-center justify-between px-4 py-3 bg-zinc-900/50 border-b border-white/[0.05] backdrop-blur-md">
        <div className="flex items-center gap-3">
          <div className="flex gap-1.5 px-0.5">
            <div className="w-2.5 h-2.5 rounded-full bg-red-500/80" />
            <div className="w-2.5 h-2.5 rounded-full bg-amber-500/80" />
            <div className="w-2.5 h-2.5 rounded-full bg-emerald-500/80" />
          </div>
          <div className="w-px h-3 bg-white/10 mx-1" />
          <div className="flex items-center gap-2 text-[11px] font-bold text-zinc-500 tracking-widest uppercase">
            <TerminalIcon className="h-3 w-3" />
            Output Terminal
          </div>
          {isBuilding && (
            <div className="flex items-center gap-1.5 px-2 py-0.5 bg-amber-500/10 rounded-full border border-amber-500/20">
              <span className="w-1.5 h-1.5 rounded-full bg-amber-500 animate-pulse" />
              <span className="text-[9px] font-bold text-amber-500 uppercase tracking-tight">Streaming</span>
            </div>
          )}
        </div>

        <Button 
          variant="ghost" 
          size="sm" 
          onClick={copyToClipboard}
          className="h-8 px-3 gap-2 text-zinc-500 hover:text-white hover:bg-white/5 transition-all active:scale-95"
        >
          {copied ? (
            <Check className="h-3.5 w-3.5 text-emerald-500" />
          ) : (
            <Copy className="h-3.5 w-3.5" />
          )}
          <span className="text-[10px] font-bold uppercase tracking-wider">{copied ? "Copied" : "Copy"}</span>
        </Button>
      </div>

      {/* Terminal Body */}
      <div
        ref={containerRef}
        onScroll={handleScroll}
        className="flex-1 p-4 overflow-auto scrollbar-terminal font-mono text-[13px] relative selection:bg-white/20"
      >
        <div className="absolute inset-0 bg-[radial-gradient(circle_at_50%_0%,rgba(120,119,198,0.05),transparent)] pointer-events-none" />
        
        {logs ? (
          <div className="relative z-10">
            {formatLogs(logs)}
            {isBuilding && (
              <div className="flex items-center gap-2 mt-4 text-emerald-500 animate-pulse font-bold">
                <span className="text-emerald-500/50">$</span>
                <span className="w-2 h-4 bg-emerald-500/80" />
              </div>
            )}
          </div>
        ) : isBuilding ? (
          <div className="text-amber-500/80 flex items-center gap-3 italic animate-pulse py-4 font-bold">
            <div className="h-2 w-2 rounded-full bg-amber-500" />
            Warming up build environment...
          </div>
        ) : (
          <div className="text-zinc-600 flex flex-col items-center justify-center h-full gap-4 text-center py-20 grayscale opacity-40">
            <TerminalIcon className="h-12 w-12" strokeWidth={1} />
            <div className="space-y-1">
              <p className="font-bold tracking-tight uppercase">Terminal Ready</p>
              <p className="text-xs">Waiting for deployment instructions</p>
            </div>
            <div className="mt-4 flex flex-wrap gap-2 justify-center opacity-50">
              {["npm run build", "docker compose up", "yarn install"].map((cmd) => (
                <div key={cmd} className="px-3 py-1 bg-white/5 rounded-lg border border-white/10 font-mono text-[10px]">{cmd}</div>
              ))}
            </div>
          </div>
        )}
      </div>

      <style jsx>{`
        .scrollbar-terminal::-webkit-scrollbar {
          width: 5px;
          height: 5px;
        }
        .scrollbar-terminal::-webkit-scrollbar-track {
          background: transparent;
        }
        .scrollbar-terminal::-webkit-scrollbar-thumb {
          background: rgba(255, 255, 255, 0.05);
          border-radius: 10px;
        }
        .scrollbar-terminal::-webkit-scrollbar-thumb:hover {
          background: rgba(255, 255, 255, 0.1);
        }
      `}</style>
    </div>
  );
}
