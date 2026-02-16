"use client";

import { useEffect, useState, useRef, useMemo, useCallback } from "react";
import {
  Search,
  X,
  Download,
  Trash2,
  ArrowDown,
  Loader2,
  ScrollText,
  Maximize2,
  Minimize2,
} from "lucide-react";
import { Button } from "@/components/ui/button";
import { cn } from "@/lib/utils";
import { toast } from "sonner";

// ──────────────────────────────────────────────
// Timestamp & ANSI handling
// ──────────────────────────────────────────────

// Docker timestamps: 2026-02-15T16:41:43.550268685Z
const DOCKER_TS_REGEX = /^(\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}\.\d+Z)\s?/;

function parseTimestamp(line: string): { timestamp: string | null; content: string } {
  const match = line.match(DOCKER_TS_REGEX);
  if (match) {
    const raw = match[1];
    try {
      const d = new Date(raw);
      // Format like Coolify: 2026-Feb-05 12:06:06.047
      const months = ["Jan","Feb","Mar","Apr","May","Jun","Jul","Aug","Sep","Oct","Nov","Dec"];
      const year = d.getFullYear();
      const month = months[d.getMonth()];
      const day = String(d.getDate()).padStart(2, "0");
      const h = String(d.getHours()).padStart(2, "0");
      const m = String(d.getMinutes()).padStart(2, "0");
      const s = String(d.getSeconds()).padStart(2, "0");
      const ms = String(d.getMilliseconds()).padStart(3, "0");
      return {
        timestamp: `${year}-${month}-${day} ${h}:${m}:${s}.${ms}`,
        content: line.slice(match[0].length),
      };
    } catch {
      return { timestamp: null, content: line };
    }
  }
  return { timestamp: null, content: line };
}

// Color mapping for ANSI codes
const ANSI_COLORS: Record<number, string> = {
  30: "#6b7280", 31: "#f87171", 32: "#4ade80", 33: "#fbbf24",
  34: "#60a5fa", 35: "#c084fc", 36: "#22d3ee", 37: "#e5e7eb",
  90: "#9ca3af", 91: "#fca5a5", 92: "#86efac", 93: "#fde68a",
  94: "#93c5fd", 95: "#d8b4fe", 96: "#67e8f9", 97: "#f9fafb",
};

function AnsiLine({ text, highlight }: { text: string; highlight?: string }) {
  if (!text || !text.trim()) return null;

  // eslint-disable-next-line no-control-regex
  const parts = text.split(/(\x1b\[[0-9;]*m)/g);
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
      if (highlight && highlight.length > 0) {
        const regex = new RegExp(`(${highlight.replace(/[.*+?^${}()|[\]\\]/g, "\\$&")})`, "gi");
        const segments = part.split(regex);
        segments.forEach((seg, j) => {
          if (regex.test(seg)) {
            elements.push(
              <mark key={`${i}-${j}`} className="bg-yellow-500/30 text-yellow-200 rounded-[2px] px-0.5">{seg}</mark>
            );
          } else if (seg) {
            elements.push(
              <span key={`${i}-${j}`} style={{ color: currentColor || undefined, fontWeight: isBold ? 700 : undefined }}>{seg}</span>
            );
          }
        });
      } else {
        elements.push(
          <span key={i} style={{ color: currentColor || undefined, fontWeight: isBold ? 700 : undefined }}>{part}</span>
        );
      }
    }
  });

  return <>{elements}</>;
}

// ──────────────────────────────────────────────
// Smart color detection for log lines (no ANSI)
// ──────────────────────────────────────────────
function getLineColor(text: string): string | null {
  const lower = text.toLowerCase();

  // Errors — red
  if (lower.includes("error") || lower.includes("fatal") || lower.includes("panic") || lower.includes("fail") || lower.includes("❌")) return "#f87171";

  // Warnings — amber
  if (lower.includes("warn") || lower.includes("⚠")) return "#fbbf24";

  // Success — green
  if (lower.includes("✓") || lower.includes("✅") || lower.includes("success") || lower.includes("ready") || lower.includes("loaded") || lower.includes("running at")) return "#4ade80";

  // Info markers — blue
  if (lower.includes("info") || lower.includes("🚀") || lower.includes("starting")) return "#60a5fa";

  // Separators / borders — dim
  if (/^[═╔╗╚╝║─┌┐└┘│├┤┬┴┼\-=*]{3,}/.test(text.trim())) return "#525252";

  // Docker build steps
  if (/^#\d+\s/.test(text.trim()) || /^step\s+\d+/i.test(text.trim())) return "#93c5fd";

  return null; // no override, use default white
}

// Check if line contains ANSI codes
// eslint-disable-next-line no-control-regex
const HAS_ANSI = /\x1b\[/;

// ──────────────────────────────────────────────
// LogViewer Component
// ──────────────────────────────────────────────
export interface LogViewerProps {
  logs: string[];
  connected: boolean;
  title: string;
  subtitle?: string;
  onClear: () => void;
  onDownload?: () => void;
  height?: string;
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
  downloadFilename = "logs.txt",
}: LogViewerProps) {
  const [autoScroll, setAutoScroll] = useState(true);
  const [searchQuery, setSearchQuery] = useState("");
  const [showSearch, setShowSearch] = useState(false);
  const [isFullscreen, setIsFullscreen] = useState(false);
  const endRef = useRef<HTMLDivElement>(null);
  const scrollRef = useRef<HTMLDivElement>(null);
  const searchInputRef = useRef<HTMLInputElement>(null);

  // Auto-scroll
  useEffect(() => {
    if (autoScroll && endRef.current) {
      endRef.current.scrollIntoView({ behavior: "smooth" });
    }
  }, [logs, autoScroll]);

  // Focus search
  useEffect(() => {
    if (showSearch && searchInputRef.current) {
      searchInputRef.current.focus();
    }
  }, [showSearch]);

  // Detect manual scroll-up to pause auto-scroll
  useEffect(() => {
    const el = scrollRef.current;
    if (!el) return;
    const onScroll = () => {
      const atBottom = el.scrollHeight - el.scrollTop - el.clientHeight < 60;
      setAutoScroll(atBottom);
    };
    el.addEventListener("scroll", onScroll, { passive: true });
    return () => el.removeEventListener("scroll", onScroll);
  }, []);

  // Keyboard shortcuts
  useEffect(() => {
    const handler = (e: KeyboardEvent) => {
      if ((e.ctrlKey || e.metaKey) && e.key === "f") {
        const container = scrollRef.current;
        const activeEl = document.activeElement;
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

  // Search match count
  const matchCount = useMemo(() => {
    if (!searchQuery) return 0;
    const regex = new RegExp(searchQuery.replace(/[.*+?^${}()|[\]\\]/g, "\\$&"), "gi");
    return logs.filter((l) => regex.test(l)).length;
  }, [logs, searchQuery]);

  const handleDownload = useCallback(() => {
    if (onDownload) { onDownload(); return; }
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

  return (
    <div
      className={cn(
        "flex flex-col rounded-xl overflow-hidden border border-[#232323] bg-[#0c0c0c] relative",
        isFullscreen ? "fixed inset-0 z-50 rounded-none h-screen" : height,
      )}
    >
      {/* ─── Header ─── */}
      <div className="flex items-center justify-between px-4 py-2.5 bg-[#141414] border-b border-[#232323] shrink-0">
        {/* Left: status + title */}
        <div className="flex items-center gap-3 min-w-0">
          <div className="flex items-center gap-2">
            <div
              className={cn(
                "h-2 w-2 rounded-full shrink-0",
                connected
                  ? "bg-emerald-400 shadow-[0_0_6px_rgba(52,211,153,0.5)]"
                  : "bg-zinc-600"
              )}
            />
            <span className="text-[13px] font-semibold text-zinc-200">{title}</span>
            {connected && (
              <span className="text-[10px] font-medium text-emerald-400/80 tracking-wide">LIVE</span>
            )}
          </div>
          {subtitle && (
            <span className="text-[11px] font-mono text-zinc-600 hidden sm:inline truncate max-w-[180px]">
              {subtitle}
            </span>
          )}
        </div>

        {/* Right: search + actions */}
        <div className="flex items-center gap-1 shrink-0">
          {/* Inline search */}
          {showSearch && (
            <div className="flex items-center gap-1.5 mr-2 bg-[#1a1a1a] border border-[#2a2a2a] rounded-lg px-2 py-1">
              <Search className="h-3 w-3 text-zinc-500" />
              <input
                ref={searchInputRef}
                type="text"
                placeholder="Find in logs"
                value={searchQuery}
                onChange={(e) => setSearchQuery(e.target.value)}
                className="bg-transparent text-[12px] text-zinc-200 placeholder-zinc-600 outline-none w-[150px] font-mono"
              />
              {searchQuery && (
                <span className="text-[10px] text-zinc-500 font-mono">{matchCount}</span>
              )}
              <button
                onClick={() => { setSearchQuery(""); setShowSearch(false); }}
                className="text-zinc-500 hover:text-zinc-300"
              >
                <X className="h-3 w-3" />
              </button>
            </div>
          )}

          <Button
            variant="ghost" size="icon"
            className={cn("h-7 w-7 text-zinc-500 hover:text-zinc-200 hover:bg-white/5", showSearch && "text-zinc-200 bg-white/5")}
            onClick={() => { setShowSearch(!showSearch); if (showSearch) setSearchQuery(""); }}
            title="Find in logs"
          >
            <Search className="h-3.5 w-3.5" />
          </Button>
          <Button
            variant="ghost" size="icon"
            className="h-7 w-7 text-zinc-500 hover:text-zinc-200 hover:bg-white/5"
            onClick={() => setIsFullscreen(!isFullscreen)}
            title={isFullscreen ? "Exit fullscreen" : "Fullscreen"}
          >
            {isFullscreen ? <Minimize2 className="h-3.5 w-3.5" /> : <Maximize2 className="h-3.5 w-3.5" />}
          </Button>
          <Button
            variant="ghost" size="icon"
            className="h-7 w-7 text-zinc-500 hover:text-zinc-200 hover:bg-white/5"
            onClick={handleDownload}
            title="Download"
          >
            <Download className="h-3.5 w-3.5" />
          </Button>
          <Button
            variant="ghost" size="icon"
            className="h-7 w-7 text-zinc-500 hover:text-zinc-200 hover:bg-white/5"
            onClick={onClear}
            title="Clear"
          >
            <Trash2 className="h-3.5 w-3.5" />
          </Button>
        </div>
      </div>

      {/* ─── Log Lines ─── */}
      <div
        ref={scrollRef}
        className="flex-1 overflow-y-auto font-mono text-[12.5px] leading-[1.7] custom-scrollbar"
        tabIndex={0}
      >
        {logs.length === 0 ? (
          <div className="flex items-center justify-center h-full text-zinc-600">
            <div className="text-center">
              <ScrollText className="h-8 w-8 mx-auto mb-3 opacity-20" />
              <p className="text-[13px]">{connected ? "Waiting for logs..." : "Connecting..."}</p>
              {!connected && <Loader2 className="h-4 w-4 animate-spin mx-auto mt-2 opacity-40" />}
            </div>
          </div>
        ) : (
          <div className="py-1">
            {logs.map((rawLine, i) => {
              const { timestamp, content } = parseTimestamp(rawLine);
              const hasAnsi = HAS_ANSI.test(content);
              const smartColor = !hasAnsi ? getLineColor(content) : null;

              return (
                <div
                  key={i}
                  className="flex items-start hover:bg-white/[0.02] px-4 py-[1px] group transition-colors"
                >
                  {/* Timestamp */}
                  {timestamp && (
                    <span className="shrink-0 text-zinc-600 select-none mr-2 text-[12px] tabular-nums">
                      {timestamp}
                    </span>
                  )}

                  {/* Content */}
                  <span
                    className="flex-1 whitespace-pre-wrap break-all"
                    style={smartColor && !hasAnsi ? { color: smartColor } : { color: "#d4d4d8" }}
                  >
                    {hasAnsi ? (
                      <AnsiLine text={content} highlight={searchQuery} />
                    ) : (
                      searchQuery ? (
                        <AnsiLine text={content} highlight={searchQuery} />
                      ) : (
                        content
                      )
                    )}
                  </span>
                </div>
              );
            })}
            <div ref={endRef} />
          </div>
        )}
      </div>

      {/* ─── Scroll-to-bottom FAB ─── */}
      {!autoScroll && logs.length > 0 && (
        <button
          onClick={() => {
            setAutoScroll(true);
            endRef.current?.scrollIntoView({ behavior: "smooth" });
          }}
          className="absolute bottom-4 right-4 h-8 w-8 rounded-full bg-zinc-800 border border-zinc-700 flex items-center justify-center text-zinc-400 hover:text-white hover:bg-zinc-700 shadow-lg transition-all"
          title="Scroll to bottom"
        >
          <ArrowDown className="h-4 w-4" />
        </button>
      )}
    </div>
  );
}
