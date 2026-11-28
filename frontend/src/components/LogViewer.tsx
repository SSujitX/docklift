
import {
  useEffect,
  useLayoutEffect,
  useState,
  useRef,
  useMemo,
  useCallback,
} from "react";
import {
  Search,
  X,
  Download,
  Trash2,
  ArrowDown,
  Loader2,
  Maximize2,
  Minimize2,
  Copy,
  Check,
} from "lucide-react";
import { Button } from "@/components/ui/button";
import { cn, copyToClipboard } from "@/lib/utils";
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
      // Timestamp format: 2026-Feb-05 12:06:06.047
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

// The log viewer is a fixed dark surface in both themes, so its colours cannot
// come from the theme tokens (which are dark-on-light in light mode). This is
// the single place they are defined; levels mirror ok / warn / danger / info.
const LOG = {
  surface: "#0f1215",
  toolbar: "#151a1e",
  border: "#232a30",
  borderStrong: "#2f373e",
  text: "#d3d9de",
  dim: "#8a949c",
  faint: "#5a646c",
  ok: "#5cc596",
  warn: "#e6b35c",
  danger: "#f0857a",
  info: "#82adeb",
  markBg: "rgba(230, 179, 92, 0.28)",
  markText: "#f6e2bd",
} as const;

const TOOL_BTN =
  "size-7 rounded-md text-white/50 hover:bg-white/10 hover:text-white/90 pointer-coarse:size-9";

// Color mapping for ANSI codes (bright variants share the base level colour)
const ANSI_COLORS: Record<number, string> = {
  30: LOG.faint, 31: LOG.danger, 32: LOG.ok, 33: LOG.warn,
  34: LOG.info, 35: LOG.info, 36: LOG.info, 37: LOG.text,
  90: LOG.dim, 91: LOG.danger, 92: LOG.ok, 93: LOG.warn,
  94: LOG.info, 95: LOG.info, 96: LOG.info, 97: LOG.text,
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
        const needle = highlight.toLowerCase();
        const segments = part.split(regex);
        segments.forEach((seg, j) => {
          if (seg && seg.toLowerCase() === needle) {
            elements.push(
              <mark key={`${i}-${j}`} className="rounded-sm px-0.5" style={{ backgroundColor: LOG.markBg, color: LOG.markText }}>{seg}</mark>
            );
          } else if (seg) {
            elements.push(
              <span key={`${i}-${j}`} style={{ color: currentColor || undefined, fontWeight: isBold ? 600 : undefined }}>{seg}</span>
            );
          }
        });
      } else {
        elements.push(
          <span key={i} style={{ color: currentColor || undefined, fontWeight: isBold ? 600 : undefined }}>{part}</span>
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
  if (lower.includes("error") || lower.includes("fatal") || lower.includes("panic") || lower.includes("fail") || lower.includes("❌")) return LOG.danger;

  // Warnings — amber
  if (lower.includes("warn") || lower.includes("⚠")) return LOG.warn;

  // Success — green
  if (lower.includes("✓") || lower.includes("✅") || lower.includes("success") || lower.includes("ready") || lower.includes("loaded") || lower.includes("running at")) return LOG.ok;

  // Info markers — blue
  if (lower.includes("info") || lower.includes("🚀") || lower.includes("starting")) return LOG.info;

  // Separators / borders — dim
  if (/^[═╔╗╚╝║─┌┐└┘│├┤┬┴┼\-=*]{3,}/.test(text.trim())) return LOG.faint;

  // Docker build steps
  if (/^#\d+\s/.test(text.trim()) || /^step\s+\d+/i.test(text.trim())) return LOG.info;

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
  const [copied, setCopied] = useState(false);
  const scrollRef = useRef<HTMLDivElement>(null);
  const searchInputRef = useRef<HTMLInputElement>(null);
  const programmaticScrollRef = useRef(false);
  const copiedResetRef = useRef<ReturnType<typeof setTimeout> | null>(null);
  const logCount = logs.length;

  useEffect(() => {
    return () => {
      if (copiedResetRef.current) clearTimeout(copiedResetRef.current);
    };
  }, []);

  // Follow the tail on this container only. Flag wraps sync scrollTop writes so
  // rapid SSE cannot leave programmaticScroll stuck true (which blocked pause).
  useLayoutEffect(() => {
    if (!autoScroll) return;
    const el = scrollRef.current;
    if (!el) return;

    const stick = () => {
      programmaticScrollRef.current = true;
      el.scrollTop = el.scrollHeight;
      programmaticScrollRef.current = false;
    };
    stick();
    let inner = 0;
    const outer = requestAnimationFrame(() => {
      stick();
      inner = requestAnimationFrame(stick);
    });
    return () => {
      cancelAnimationFrame(outer);
      cancelAnimationFrame(inner);
      programmaticScrollRef.current = false;
    };
  }, [logCount, logs[logs.length - 1], autoScroll, isFullscreen]);

  // Focus search
  useEffect(() => {
    if (showSearch && searchInputRef.current) {
      searchInputRef.current.focus();
    }
  }, [showSearch]);

  // Detect manual scroll-up / wheel to pause auto-scroll
  useEffect(() => {
    const el = scrollRef.current;
    if (!el) return;
    const pauseIfNotAtBottom = () => {
      if (programmaticScrollRef.current) return;
      const atBottom = el.scrollHeight - el.scrollTop - el.clientHeight < 80;
      setAutoScroll(atBottom);
    };
    const onWheel = () => {
      // User intent — do not wait for scroll event after programmatic stick
      programmaticScrollRef.current = false;
      requestAnimationFrame(pauseIfNotAtBottom);
    };
    el.addEventListener("scroll", pauseIfNotAtBottom, { passive: true });
    el.addEventListener("wheel", onWheel, { passive: true });
    el.addEventListener("touchmove", onWheel, { passive: true });
    return () => {
      el.removeEventListener("scroll", pauseIfNotAtBottom);
      el.removeEventListener("wheel", onWheel);
      el.removeEventListener("touchmove", onWheel);
    };
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
    const q = searchQuery.toLowerCase();
    return logs.filter((l) => l.toLowerCase().includes(q)).length;
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

  const copyAriaLabel =
    logs.length === 0
      ? "No logs to copy"
      : copied
        ? "Copied"
        : "Copy all logs";

  const handleCopy = useCallback(async () => {
    if (logs.length === 0) return;
    // Cheap length gate before allocating a giant joined string.
    if (logs.length > 3000) {
      toast.message("Log buffer is large — use Download for a reliable copy", {
        action: {
          label: "Download",
          onClick: () => handleDownload(),
        },
      });
      return;
    }
    const text = logs.join("\n");
    // Very large buffers often fail or hitch in the clipboard path (esp. HTTP fallback).
    if (text.length > 500_000) {
      toast.message("Log buffer is large — use Download for a reliable copy", {
        action: {
          label: "Download",
          onClick: () => handleDownload(),
        },
      });
      return;
    }
    const ok = await copyToClipboard(text);
    if (!ok) {
      toast.error("Could not copy logs — try Download instead");
      return;
    }
    setCopied(true);
    toast.success("Logs copied");
    if (copiedResetRef.current) clearTimeout(copiedResetRef.current);
    copiedResetRef.current = setTimeout(() => setCopied(false), 2000);
  }, [logs, handleDownload]);

  return (
    <div
      className={cn(
        "relative flex flex-col overflow-hidden rounded-lg border",
        isFullscreen
          ? "fixed inset-0 z-50 h-screen rounded-none border-0"
          : cn(
              // Parent-driven fill (Logs page) must not force a tall min-height
              height.includes("h-full") ? "min-h-0" : "min-h-[280px]",
              height,
            ),
      )}
      style={{ backgroundColor: LOG.surface, borderColor: LOG.border }}
    >
      {/* ─── Toolbar ─── */}
      <div
        className="flex min-h-9 shrink-0 items-center justify-between gap-2 border-b pl-3 pr-1"
        style={{ backgroundColor: LOG.toolbar, borderColor: LOG.border }}
      >
        {/* Left: status + title */}
        <div className="flex min-w-0 items-center gap-2.5">
          <span
            className="size-2 shrink-0 rounded-full"
            style={{ backgroundColor: connected ? LOG.ok : LOG.faint }}
          />
          <span className="truncate text-[13px] font-medium" style={{ color: LOG.text }}>
            {title}
          </span>
          {connected && (
            <span className="text-xs" style={{ color: LOG.ok }}>
              Live
            </span>
          )}
          {subtitle && (
            <span
              className="hidden max-w-[220px] truncate font-mono text-xs sm:inline"
              style={{ color: LOG.dim }}
            >
              {subtitle}
            </span>
          )}
        </div>

        {/* Right: search + actions */}
        <div className="flex shrink-0 items-center gap-0.5">
          {/* Inline search */}
          {showSearch && (
            <div
              className="mr-1 flex h-7 items-center gap-1.5 rounded-md border px-2"
              style={{ borderColor: LOG.border, backgroundColor: LOG.surface }}
            >
              <Search className="size-3.5 shrink-0" style={{ color: LOG.dim }} />
              <input
                ref={searchInputRef}
                type="text"
                placeholder="Find in logs"
                value={searchQuery}
                onChange={(e) => setSearchQuery(e.target.value)}
                className="w-[120px] bg-transparent font-mono text-xs outline-hidden placeholder:text-white/35 sm:w-[160px]"
                style={{ color: LOG.text }}
              />
              {searchQuery && (
                <span className="font-mono text-xs tabular-nums" style={{ color: LOG.dim }}>
                  {matchCount}
                </span>
              )}
              <button
                onClick={() => { setSearchQuery(""); setShowSearch(false); }}
                className="text-white/45 transition-colors hover:text-white/85"
              >
                <X className="size-3.5" />
              </button>
            </div>
          )}

          <Button
            variant="ghost" size="icon"
            className={cn(TOOL_BTN, showSearch && "bg-white/10 text-white/90")}
            onClick={() => { setShowSearch(!showSearch); if (showSearch) setSearchQuery(""); }}
            title="Find in logs"
          >
            <Search className="size-3.5" />
          </Button>
          <Button
            variant="ghost" size="icon"
            className={TOOL_BTN}
            onClick={() => setIsFullscreen(!isFullscreen)}
            title={isFullscreen ? "Exit fullscreen" : "Fullscreen"}
          >
            {isFullscreen ? <Minimize2 className="size-3.5" /> : <Maximize2 className="size-3.5" />}
          </Button>
          <Button
            variant="ghost" size="icon"
            className={TOOL_BTN}
            onClick={handleCopy}
            title={copyAriaLabel}
            aria-label={copyAriaLabel}
            disabled={logs.length === 0}
          >
            {copied ? (
              <Check className="size-3.5" style={{ color: LOG.ok }} />
            ) : (
              <Copy className="size-3.5" />
            )}
          </Button>
          <Button
            variant="ghost" size="icon"
            className={TOOL_BTN}
            onClick={handleDownload}
            title="Download"
          >
            <Download className="size-3.5" />
          </Button>
          <Button
            variant="ghost" size="icon"
            className={TOOL_BTN}
            onClick={onClear}
            title="Clear"
          >
            <Trash2 className="size-3.5" />
          </Button>
        </div>
      </div>

      {/* ─── Log Lines ─── */}
      <div
        ref={scrollRef}
        className="custom-scrollbar flex-1 overflow-y-auto font-mono text-[12.5px] leading-[1.65]"
        tabIndex={0}
      >
        {logs.length === 0 ? (
          <div className="flex h-full items-center justify-center" style={{ color: LOG.dim }}>
            <div className="flex items-center gap-2 text-[13px]">
              {!connected && <Loader2 className="size-3.5 animate-spin" />}
              {connected ? "Waiting for logs…" : "Connecting…"}
            </div>
          </div>
        ) : (
          <div className="py-2">
            {logs.map((rawLine, i) => {
              const { timestamp, content } = parseTimestamp(rawLine);
              const hasAnsi = HAS_ANSI.test(content);
              const smartColor = !hasAnsi ? getLineColor(content) : null;

              return (
                <div
                  key={i}
                  className="group flex items-baseline px-3 py-px hover:bg-white/[0.03] sm:px-4"
                >
                  {/* Timestamp */}
                  {timestamp && (
                    <span
                      className="mr-3 shrink-0 select-none text-xs tabular-nums"
                      style={{ color: LOG.faint }}
                    >
                      {timestamp}
                    </span>
                  )}

                  {/* Content */}
                  <span
                    className="flex-1 whitespace-pre-wrap break-all"
                    style={smartColor && !hasAnsi ? { color: smartColor } : { color: LOG.text }}
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
          </div>
        )}
      </div>

      {/* ─── Resume following ─── */}
      {!autoScroll && logs.length > 0 && (
        <button
          type="button"
          onClick={() => {
            const el = scrollRef.current;
            setAutoScroll(true);
            if (!el) return;
            programmaticScrollRef.current = true;
            el.scrollTop = el.scrollHeight;
            programmaticScrollRef.current = false;
            requestAnimationFrame(() => {
              programmaticScrollRef.current = true;
              el.scrollTop = el.scrollHeight;
              programmaticScrollRef.current = false;
            });
          }}
          className="absolute bottom-3 right-3 flex h-8 items-center gap-1.5 rounded-md border px-2.5 text-xs font-medium text-white/80 shadow-popover transition-colors hover:text-white"
          style={{ backgroundColor: LOG.toolbar, borderColor: LOG.borderStrong }}
          title="Resume following the newest logs"
        >
          <ArrowDown className="size-3.5" />
          Latest
        </button>
      )}
    </div>
  );
}
