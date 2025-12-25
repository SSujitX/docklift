"use client";

import { useEffect, useRef, useState } from "react";
import { cn } from "@/lib/utils";

interface TerminalProps {
  logs: string;
  className?: string;
}

export function Terminal({ logs, className }: TerminalProps) {
  const containerRef = useRef<HTMLDivElement>(null);
  const [autoScroll, setAutoScroll] = useState(true);

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

  const formatLogs = (text: string) => {
    return text.split("\n").map((line, i) => {
      let lineClass = "text-zinc-400";
      let prefix = "";
      
      // Success indicators
      if (line.includes("[✓]") || line.toLowerCase().includes("success") || line.includes("DONE") || line.includes("Built")) {
        lineClass = "text-emerald-400 font-medium";
      } 
      // Error indicators
      else if (line.includes("[✗]") || line.toLowerCase().includes("error") || line.toLowerCase().includes("failed")) {
        lineClass = "text-red-400 font-medium";
      }
      // Warning indicators  
      else if (line.toLowerCase().includes("warning") || line.includes("[!]")) {
        lineClass = "text-amber-400";
      }
      // Build steps (#1, #2, etc)
      else if (/^#\d+/.test(line.trim())) {
        lineClass = "text-cyan-400";
      }
      // Docker layer steps ([1/10], [2/10], etc)
      else if (/\[\s*\d+\/\d+\s*\]/.test(line)) {
        lineClass = "text-violet-400 font-medium";
      }
      // CACHED layers
      else if (line.includes("CACHED")) {
        lineClass = "text-zinc-500";
      }
      // Container actions (Creating, Starting, etc)
      else if (line.includes("Container") && (line.includes("Creating") || line.includes("Starting") || line.includes("Started") || line.includes("Created"))) {
        lineClass = "text-blue-400";
      }
      // Container stopping/removing
      else if (line.includes("Container") && (line.includes("Stopping") || line.includes("Stopped") || line.includes("Removing") || line.includes("Removed"))) {
        lineClass = "text-orange-400";
      }
      // Info/build commands
      else if (line.includes("[+]") || line.includes("Building")) {
        lineClass = "text-blue-400";
      }
      // Prompt style
      else if (line.startsWith("$")) {
        lineClass = "text-violet-400 font-semibold";
      }
      // Transferring/loading context
      else if (line.includes("transferring") || line.includes("load")) {
        lineClass = "text-zinc-500 text-xs";
      }

      return (
        <div key={i} className={cn("leading-relaxed", lineClass)}>
          {prefix}{line || "\u00A0"}
        </div>
      );
    });
  };

  return (
    <div
      ref={containerRef}
      onScroll={handleScroll}
      className={cn(
        "bg-zinc-950 dark:bg-zinc-950 rounded-xl border border-border font-mono text-sm p-4 overflow-auto",
        className
      )}
    >
      {logs ? (
        formatLogs(logs)
      ) : (
        <div className="text-zinc-500">
          <span className="text-violet-400">$</span> Waiting for output...
        </div>
      )}
    </div>
  );
}
