"use client";

import { useEffect, useState, useRef } from "react";
import { 
  Pause, 
  ChevronDown, 
  Trash2, 
  Download, 
  ScrollText, 
  Loader2,
  AlertTriangle
} from "lucide-react";
import { Button } from "@/components/ui/button";
import { Card } from "@/components/ui/card";
import { cn, API_URL } from "@/lib/utils";
import { toast } from "sonner";
import { getAuthHeaders } from "@/lib/auth";

// Simple ANSI color parser to ensure logs look good
// This handles basic colors and reset codes
const AnsiLine = ({ text }: { text: string }) => {
  if (!text) return null;

  // Split by ANSI escape codes
  // eslint-disable-next-line
  const parts = text.split(/(\u001b\[\d+(?:;\d+)*m)/g);
  
  const spans = [];
  let currentColor = "text-zinc-300"; // Default color
  let key = 0;

  for (let i = 0; i < parts.length; i++) {
    const part = parts[i];
    if (part.startsWith("\u001b[")) {
      // It's an ANSI code
      // Simple mapping for common colors
      if (part === "\u001b[0m") currentColor = "text-zinc-300";
      else if (part.includes("31m")) currentColor = "text-red-400";
      else if (part.includes("32m")) currentColor = "text-emerald-400";
      else if (part.includes("33m")) currentColor = "text-amber-400";
      else if (part.includes("34m")) currentColor = "text-blue-400";
      else if (part.includes("35m")) currentColor = "text-purple-400";
      else if (part.includes("36m")) currentColor = "text-cyan-400";
      else if (part.includes("90m")) currentColor = "text-zinc-500"; // Gray
      // Add more as needed
    } else if (part) {
      spans.push(
        <span key={key++} className={currentColor}>
          {part}
        </span>
      );
    }
  }

  return <span>{spans}</span>;
};

interface SystemLogsPanelProps {
  service: string; // 'backend' | 'frontend' | 'database' | 'redis' | 'proxy' | 'nginx'
  isActive: boolean;
}

export function SystemLogsPanel({ service, isActive }: SystemLogsPanelProps) {
  const [logs, setLogs] = useState<string[]>([]);
  const [connected, setConnected] = useState(false);
  const [autoScroll, setAutoScroll] = useState(true);
  const [error, setError] = useState<string | null>(null);
  
  const endRef = useRef<HTMLDivElement>(null);
  const eventSourceRef = useRef<EventSource | null>(null);

  // Auto-scroll
  useEffect(() => {
    if (autoScroll && endRef.current) {
      endRef.current.scrollIntoView({ behavior: "smooth" });
    }
  }, [logs, autoScroll, isActive]);

  // Connect to SSE
  useEffect(() => {
    if (!isActive) {
      eventSourceRef.current?.close();
      eventSourceRef.current = null;
      setConnected(false);
      return;
    }

    const connect = () => {
      // Close existing
      eventSourceRef.current?.close();

      const token = typeof window !== "undefined" ? localStorage.getItem("docklift_token") || "" : "";
      
      // SSE URL
      const isDev = process.env.NODE_ENV === "development";
      const sseBase = API_URL || (isDev && typeof window !== "undefined"
        ? `${window.location.protocol}//${window.location.hostname}:8000`
        : "");
      
      const url = `${sseBase}/api/system/logs/${service}?tail=5000&token=${encodeURIComponent(token)}`;

      const es = new EventSource(url);
      eventSourceRef.current = es;

      es.onopen = () => {
        setConnected(true);
        setError(null);
      };

      es.onmessage = (event) => {
        try {
          const data = JSON.parse(event.data);
          if (data.log) {
            setLogs(prev => {
              const newLogs = [...prev, data.log].slice(-5000); // Keep last 5000 lines
              return newLogs;
            });
          }
        } catch (e) {
         // Raw text fallback
         setLogs(prev => [...prev, event.data].slice(-5000));
        }
      };

      es.onerror = (_err) => {
        // Don't log to console to avoid spam
        setConnected(false);
        // Retry logic is handled by browser for SSE, but if connection dies completely:
        es.close();
        if (isActive) {
           setTimeout(connect, 3000); // Retry in 3s
        }
      };
    };

    connect();

    return () => {
      eventSourceRef.current?.close();
    };
  }, [isActive, service]);

  const clearLogs = () => setLogs([]);

  const downloadLogs = () => {
    const blob = new Blob([logs.join("")], { type: "text/plain" });
    const url = URL.createObjectURL(blob);
    const a = document.createElement("a");
    a.href = url;
    a.download = `docklift-${service}-logs.txt`;
    document.body.appendChild(a);
    a.click();
    document.body.removeChild(a);
    URL.revokeObjectURL(url);
    toast.success("Logs downloaded");
  };

  if (!isActive) return null;

  return (
    <Card className="flex flex-col bg-[#0d0d0d] border-[#1a1a1a] shadow-xl rounded-2xl overflow-hidden h-[600px] animate-in fade-in duration-500">
      {/* Header bar */}
      <div className="flex items-center justify-between px-4 py-3 bg-[#141414] border-b border-[#1a1a1a]">
        <div className="flex items-center gap-3">
          <div className="flex items-center gap-2">
            <div
              className={cn(
                "h-2.5 w-2.5 rounded-full",
                connected
                  ? "bg-emerald-500 shadow-[0_0_10px_rgba(16,185,129,0.5)] animate-pulse"
                  : "bg-red-500/60"
              )}
            />
            <span className="text-sm font-bold text-zinc-300 capitalize">
              {service} Logs
            </span>
          </div>
          <span className="text-[10px] font-mono text-zinc-500 hidden sm:inline px-2 py-0.5 bg-[#1a1a1a] rounded border border-[#222]">
            tail=5000
          </span>
          {error && (
             <span className="text-xs text-red-400 flex items-center gap-1">
               <AlertTriangle className="h-3 w-3" /> Connection lost
             </span>
          )}
        </div>
        
        <div className="flex items-center gap-1">
          <Button
            variant="ghost"
            size="icon"
            className="h-7 w-7 text-zinc-400 hover:text-white hover:bg-white/10"
            onClick={() => setAutoScroll(!autoScroll)}
            title={autoScroll ? "Pause auto-scroll" : "Resume auto-scroll"}
          >
            {autoScroll ? (
              <Pause className="h-3.5 w-3.5" />
            ) : (
              <ChevronDown className="h-3.5 w-3.5" />
            )}
          </Button>
          <Button
            variant="ghost"
            size="icon"
            className="h-7 w-7 text-zinc-400 hover:text-white hover:bg-white/10"
            onClick={clearLogs}
            title="Clear logs"
          >
            <Trash2 className="h-3.5 w-3.5" />
          </Button>
          <Button
            variant="ghost"
            size="icon"
            className="h-7 w-7 text-zinc-400 hover:text-white hover:bg-white/10"
            onClick={downloadLogs}
            title="Download logs"
          >
            <Download className="h-3.5 w-3.5" />
          </Button>
        </div>
      </div>

      {/* Log content */}
      <div className="flex-1 bg-[#0a0a0a] p-4 overflow-y-auto font-mono text-xs leading-relaxed custom-scrollbar">
        {logs.length === 0 ? (
          <div className="flex items-center justify-center h-full text-zinc-600">
            <div className="text-center">
              <ScrollText className="h-8 w-8 mx-auto mb-2 opacity-30" />
              <p>
                {connected ? "Waiting for logs..." : "Connecting to system log stream..."}
              </p>
              {!connected && <Loader2 className="h-4 w-4 animate-spin mx-auto mt-2 opacity-50" />}
            </div>
          </div>
        ) : (
          <>
            {logs.map((line, i) => (
              <div
                key={i}
                className="whitespace-pre-wrap break-all hover:bg-white/5 px-1 rounded transition-colors"
              >
                <AnsiLine text={line} />
              </div>
            ))}
            <div ref={endRef} />
          </>
        )}
      </div>
    </Card>
  );
}
