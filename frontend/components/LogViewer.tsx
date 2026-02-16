"use client";

import { useEffect, useState, useRef, useMemo, useCallback } from "react";
import {
  Pause,
  ChevronDown,
  Trash2,
  Download,
  ScrollText,
  Loader2,
  Search,
  X,
  Hash,
} from "lucide-react";
import { Button } from "@/components/ui/button";
import { cn } from "@/lib/utils";
import { toast } from "sonner";

// ──────────────────────────────────────────────
// ANSI Color Parser
// ──────────────────────────────────────────────
const ANSI_COLORS: Record<number, string> = {
  30: "#6b7280", 31: "#ef4444", 32: "#22c55e", 33: "#eab308",
  34: "#3b82f6", 35: "#a855f7", 36: "#06b6d4", 37: "#d1d5db",
  90: "#9ca3af", 91: "#f87171", 92: "#4ade80", 93: "#facc15",
  94: "#60a5fa", 95: "#c084fc", 96: "#22d3ee", 97: "#f3f4f6",
};

function cleanLogLine(line: string): string {
  // Strip Docker timestamp prefix (e.g., 2026-02-15T16:41:43.550268685Z)
  let cleaned = line.replace(/^\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}\.\d+Z\s?/, "");
  return cleaned.trimEnd();
}

function AnsiLine({ text, highlight }: { text: string; highlight?: string }) {
  const cleaned = cleanLogLine(text);
  if (!cleaned) return null;

  // eslint-disable-next-line no-control-regex
  const parts = cleaned.split(/(\x1b\[[0-9;]*m)/g);
  let currentColor: string | null = null;
  let isBold = false;
  const elements: React.ReactNode[] = [];

  parts.forEach((part, i) => {
    // eslint-disable-next-line no-control-regex
    const ansiMatch = part.match(/^\x1b\[([0-9;]*)m$/);
    if (ansiMatch) {
      const codes = ansiMatch[1].split(";").map(Number);
      for (const code of codes) {
        if (code === 0) { currentColor = null; isBold = false; }
        else if (code === 1) isBold = true;
        else if (ANSI_COLORS[code]) currentColor = ANSI_COLORS[code];
      }
    } else if (part) {
      // If we have a search highlight, split by it
      if (highlight && highlight.length > 0) {
        const regex = new RegExp(`(${highlight.replace(/[.*+?^${}()|[\]\\]/g, "\\$&")})`, "gi");
        const segments = part.split(regex);
        segments.forEach((seg, j) => {
          if (regex.test(seg)) {
            elements.push(
              <mark
                key={`${i}-${j}`}
                className="bg-amber-500/30 text-amber-200 rounded-sm px-0.5"
              >
                {seg}
              </mark>
            );
          } else if (seg) {
            elements.push(
              <span
                key={`${i}-${j}`}
                style={{ color: currentColor || undefined, fontWeight: isBold ? 700 : undefined }}
              >
                {seg}
              </span>
            );
          }
        });
      } else {
        elements.push(
          <span
            key={i}
            style={{ color: currentColor || undefined, fontWeight: isBold ? 700 : undefined }}
          >
            {part}
          </span>
        );
      }
    }
  });

  return <>{elements}</>;
}

// ──────────────────────────────────────────────
// Log level detection
// ──────────────────────────────────────────────
type LogLevel = "error" | "warn" | "info" | "debug" | "default";

function detectLogLevel(line: string): LogLevel {
  const lower = line.toLowerCase();
  if (lower.includes("error") || lower.includes("fatal") || lower.includes("panic") || lower.includes("crit")) return "error";
  if (lower.includes("warn")) return "warn";
  if (lower.includes("debug") || lower.includes("trace")) return "debug";
  if (lower.includes("info")) return "info";
  return "default";
}

const LEVEL_COLORS: Record<LogLevel, string> = {
  error: "bg-red-500",
  warn: "bg-amber-500",
  info: "bg-blue-500",
  debug: "bg-zinc-500",
  default: "bg-zinc-700",
};

// ──────────────────────────────────────────────
// LogViewer Component
// ──────────────────────────────────────────────
export interface LogViewerProps {
  /** Array of raw log lines */
  logs: string[];
  /** Whether the SSE connection is alive */
  connected: boolean;
  /** Name shown in the header bar */
  title: string;
  /** Optional subtitle (e.g. container name) */
  subtitle?: string;
  /** Called when the user clicks "Clear" */
  onClear: () => void;
  /** Called when the user clicks "Download" */
  onDownload?: () => void;
  /** CSS height for the log container. Default: "h-[600px]" */
  height?: string;
  /** Show line numbers. Default: true */
  showLineNumbers?: boolean;
  /** Show level indicator dots. Default: true */
  showLevelIndicators?: boolean;
  /** Filename for download. Default: "logs.txt" */
  downloadFilename?: string;
}

export function LogViewer({
  logs,
  connected,
  title,
  subtitle,
  onClear,
  onDownload,
  height = "h-[600px]",
  showLineNumbers = true,
  showLevelIndicators = true,
  downloadFilename = "logs.txt",
}: LogViewerProps) {
  const [autoScroll, setAutoScroll] = useState(true);
  const [searchQuery, setSearchQuery] = useState("");
  const [showSearch, setShowSearch] = useState(false);
  const endRef = useRef<HTMLDivElement>(null);
  const scrollContainerRef = useRef<HTMLDivElement>(null);
  const searchInputRef = useRef<HTMLInputElement>(null);

  // Auto-scroll to bottom
  useEffect(() => {
    if (autoScroll && endRef.current) {
      endRef.current.scrollIntoView({ behavior: "smooth" });
    }
  }, [logs, autoScroll]);

  // Focus search input when shown
  useEffect(() => {
    if (showSearch && searchInputRef.current) {
      searchInputRef.current.focus();
    }
  }, [showSearch]);

  // Keyboard shortcut: Ctrl+F to search
  useEffect(() => {
    const handler = (e: KeyboardEvent) => {
      if ((e.ctrlKey || e.metaKey) && e.key === "f") {
        const activeEl = document.activeElement;
        const container = scrollContainerRef.current;
        // Only intercept if focus is inside our component
        if (container && (container === activeEl || container.contains(activeEl))) {
          e.preventDefault();
          setShowSearch(true);
        }
      }
      if (e.key === "Escape" && showSearch) {
        setShowSearch(false);
        setSearchQuery("");
      }
    };
    document.addEventListener("keydown", handler);
    return () => document.removeEventListener("keydown", handler);
  }, [showSearch]);

  // Filter logs by search if query is active (only match, don't hide — highlight instead)
  const matchCount = useMemo(() => {
    if (!searchQuery) return 0;
    const regex = new RegExp(searchQuery.replace(/[.*+?^${}()|[\]\\]/g, "\\$&"), "gi");
    return logs.filter((l) => regex.test(l)).length;
  }, [logs, searchQuery]);

  const handleDownload = useCallback(() => {
    if (onDownload) {
      onDownload();
      return;
    }
    const blob = new Blob([logs.join("\n")], { type: "text/plain" });
    const url = URL.createObjectURL(blob);
    const a = document.createElement("a");
    a.href = url;
    a.download = downloadFilename;
    document.body.appendChild(a);
    a.click();
    document.body.removeChild(a);
    URL.revokeObjectURL(url);
    toast.success("Logs downloaded");
  }, [logs, onDownload, downloadFilename]);

  // Compute line number width dynamically
  const lineNumWidth = logs.length > 9999 ? "w-14" : logs.length > 999 ? "w-12" : "w-10";

  return (
    <div
      className={cn(
        "flex flex-col bg-[#0a0a0a] border border-[#1a1a1a] shadow-2xl rounded-2xl overflow-hidden",
        height,
      )}
    >
      {/* ─── Header Bar ─── */}
      <div className="flex items-center justify-between px-4 py-2.5 bg-[#111111] border-b border-[#1a1a1a] shrink-0">
        <div className="flex items-center gap-3 min-w-0">
          {/* Traffic light dots */}
          <div className="flex items-center gap-1.5 shrink-0">
            <div className="h-3 w-3 rounded-full bg-[#ff5f57]" />
            <div className="h-3 w-3 rounded-full bg-[#febc2e]" />
            <div className="h-3 w-3 rounded-full bg-[#28c840]" />
          </div>

          <div className="flex items-center gap-2 min-w-0">
            <div
              className={cn(
                "h-2 w-2 rounded-full shrink-0 transition-colors",
                connected
                  ? "bg-emerald-500 shadow-[0_0_8px_rgba(16,185,129,0.6)] animate-pulse"
                  : "bg-red-500/50"
              )}
            />
            <span className="text-sm font-semibold text-zinc-200 truncate">{title}</span>
            {subtitle && (
              <span className="text-[10px] font-mono text-zinc-500 hidden sm:inline px-1.5 py-0.5 bg-[#1a1a1a] rounded border border-[#222] truncate max-w-[200px]">
                {subtitle}
              </span>
            )}
          </div>

          {/* Log count badge */}
          <span className="text-[10px] font-mono text-zinc-600 hidden md:inline">
            {logs.length.toLocaleString()} lines
          </span>
        </div>

        {/* Controls */}
        <div className="flex items-center gap-0.5 shrink-0">
          <Button
            variant="ghost"
            size="icon"
            className={cn(
              "h-7 w-7 text-zinc-400 hover:text-white hover:bg-white/10",
              showSearch && "text-amber-400 bg-amber-500/10"
            )}
            onClick={() => {
              setShowSearch(!showSearch);
              if (showSearch) setSearchQuery("");
            }}
            title="Search (Ctrl+F)"
          >
            <Search className="h-3.5 w-3.5" />
          </Button>
          <Button
            variant="ghost"
            size="icon"
            className={cn(
              "h-7 w-7 text-zinc-400 hover:text-white hover:bg-white/10",
              showLineNumbers && "text-blue-400 bg-blue-500/10"
            )}
            onClick={() => {/* Line numbers toggled by parent via prop */}}
            title="Line numbers"
          >
            <Hash className="h-3.5 w-3.5" />
          </Button>
          <div className="w-px h-4 bg-[#222] mx-1" />
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
            onClick={onClear}
            title="Clear logs"
          >
            <Trash2 className="h-3.5 w-3.5" />
          </Button>
          <Button
            variant="ghost"
            size="icon"
            className="h-7 w-7 text-zinc-400 hover:text-white hover:bg-white/10"
            onClick={handleDownload}
            title="Download logs"
          >
            <Download className="h-3.5 w-3.5" />
          </Button>
        </div>
      </div>

      {/* ─── Search Bar ─── */}
      {showSearch && (
        <div className="flex items-center gap-2 px-4 py-2 bg-[#0e0e0e] border-b border-[#1a1a1a] shrink-0">
          <Search className="h-3.5 w-3.5 text-zinc-500 shrink-0" />
          <input
            ref={searchInputRef}
            type="text"
            placeholder="Search logs..."
            value={searchQuery}
            onChange={(e) => setSearchQuery(e.target.value)}
            className="flex-1 bg-transparent text-sm text-zinc-200 placeholder-zinc-600 outline-none font-mono"
          />
          {searchQuery && (
            <span className="text-[10px] text-zinc-500 font-mono shrink-0">
              {matchCount} match{matchCount !== 1 ? "es" : ""}
            </span>
          )}
          <Button
            variant="ghost"
            size="icon"
            className="h-6 w-6 text-zinc-500 hover:text-white shrink-0"
            onClick={() => {
              setSearchQuery("");
              setShowSearch(false);
            }}
          >
            <X className="h-3 w-3" />
          </Button>
        </div>
      )}

      {/* ─── Log Content ─── */}
      <div
        ref={scrollContainerRef}
        className="flex-1 overflow-y-auto font-mono text-[13px] leading-[1.6] custom-scrollbar"
        tabIndex={0}
      >
        {logs.length === 0 ? (
          <div className="flex items-center justify-center h-full text-zinc-600">
            <div className="text-center">
              <ScrollText className="h-10 w-10 mx-auto mb-3 opacity-20" />
              <p className="text-sm font-medium">
                {connected ? "Waiting for logs..." : "Connecting..."}
              </p>
              {!connected && (
                <Loader2 className="h-4 w-4 animate-spin mx-auto mt-3 opacity-40" />
              )}
            </div>
          </div>
        ) : (
          <div className="p-1">
            {logs.map((line, i) => {
              const level = showLevelIndicators ? detectLogLevel(line) : "default";

              return (
                <div
                  key={i}
                  className={cn(
                    "flex items-start gap-0 hover:bg-white/[0.03] rounded-sm transition-colors group",
                    level === "error" && "bg-red-500/[0.04]",
                    level === "warn" && "bg-amber-500/[0.03]",
                  )}
                >
                  {/* Line number */}
                  {showLineNumbers && (
                    <span
                      className={cn(
                        "shrink-0 text-right pr-3 pl-2 py-px text-zinc-600 select-none text-[11px] border-r border-[#1a1a1a] group-hover:text-zinc-500",
                        lineNumWidth
                      )}
                    >
                      {i + 1}
                    </span>
                  )}

                  {/* Level indicator */}
                  {showLevelIndicators && level !== "default" && (
                    <div className="shrink-0 flex items-center pt-[7px] pl-2">
                      <div className={cn("h-1.5 w-1.5 rounded-full", LEVEL_COLORS[level])} />
                    </div>
                  )}

                  {/* Log line */}
                  <div
                    className={cn(
                      "flex-1 whitespace-pre-wrap break-all px-2 py-px text-zinc-300",
                      !showLevelIndicators || level === "default" ? "pl-3" : "",
                    )}
                  >
                    <AnsiLine text={line} highlight={searchQuery} />
                  </div>
                </div>
              );
            })}
            <div ref={endRef} />
          </div>
        )}
      </div>

      {/* ─── Status Bar ─── */}
      <div className="flex items-center justify-between px-4 py-1.5 bg-[#111111] border-t border-[#1a1a1a] shrink-0">
        <div className="flex items-center gap-2">
          <span className="text-[10px] text-zinc-600 font-mono">
            {connected ? "● LIVE" : "○ DISCONNECTED"}
          </span>
        </div>
        <div className="flex items-center gap-3">
          <span className="text-[10px] text-zinc-600 font-mono">
            {logs.length.toLocaleString()} lines
          </span>
          {autoScroll && (
            <span className="text-[10px] text-emerald-600 font-mono">AUTO-SCROLL</span>
          )}
        </div>
      </div>
    </div>
  );
}
