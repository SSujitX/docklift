// Terminal component - build log display with syntax highlighting and copy feature

import { useEffect, useRef, useState, type CSSProperties } from "react";
import { cn, copyToClipboard } from "@/lib/utils";
import { Copy, Check } from "lucide-react";
import { Button } from "./ui/button";

/** Fixed dark log surface in both app themes — the only literal colour in this file. */
const LOG_SURFACE = "#0f1215";

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

  const handleCopy = () => {
    copyToClipboard(logs);
    setCopied(true);
    setTimeout(() => setCopied(false), 2000);
  };

  // Line colour carries state only: ok / danger / warn / info; structure lines
  // (build steps, layer steps, prompts) get ink weight instead of a hue.
  const formatLogs = (text: string) => {
    return text.split("\n").map((line, i) => {
      let lineClass = "text-muted-foreground";

      // Success indicators
      if (line.includes("[✓]") || line.includes("✅") || line.toLowerCase().includes("success") || line.includes("DONE") || line.includes("Built")) {
        lineClass = "text-ok";
      }
      // Error indicators
      else if (line.includes("[✗]") || line.includes("❌") || line.toLowerCase().includes("error") || line.toLowerCase().includes("failed")) {
        lineClass = "text-danger";
      }
      // Warning indicators
      else if (line.toLowerCase().includes("warning") || line.includes("[!]")) {
        lineClass = "text-warn";
      }
      // Build steps (#1, #2, etc)
      else if (/^#\d+/.test(line.trim())) {
        lineClass = "text-foreground/85";
      }
      // Docker layer steps ([1/10], [2/10], etc)
      else if (/\[\s*\d+\/\d+\s*\]/.test(line)) {
        lineClass = "text-foreground font-medium";
      }
      // Info/build commands
      else if (line.includes("[+]") || line.includes("Building")) {
        lineClass = "text-info";
      }
      // Prompt style
      else if (line.trim().startsWith("$")) {
        lineClass = "text-foreground font-medium";
      }

      return (
        <div key={i} className={cn("group/line flex items-start", lineClass)}>
          <span className="w-9 shrink-0 select-none pr-3 text-right text-xs leading-5 text-subtle/60 transition-opacity duration-150 group-hover/line:text-subtle">
            {i + 1}
          </span>
          <span className="flex-1 whitespace-pre-wrap break-words">
            {line || " "}
          </span>
        </div>
      );
    });
  };

  return (
    <div
      id="terminal-wrapper"
      className={cn(
        "dark flex flex-col overflow-hidden rounded-lg border border-border text-foreground",
        className,
      )}
      style={{ ...DARK_SCOPE, background: LOG_SURFACE }}
    >
      {/* Toolbar */}
      <div className="flex h-9 shrink-0 items-center justify-between gap-3 border-b border-border bg-card pl-3 pr-1">
        <div className="flex min-w-0 items-center gap-2 text-[13px]">
          <span className="truncate font-medium text-foreground">Output</span>
          {logs && (
            <span className="shrink-0 font-mono text-xs text-subtle">
              {logs.split("\n").length} lines
            </span>
          )}
          {isBuilding && (
            <span className="flex shrink-0 items-center gap-1.5 text-xs text-muted-foreground">
              <span
                aria-hidden="true"
                className="size-2 rounded-full bg-warn animate-pulse motion-reduce:animate-none"
              />
              Streaming
            </span>
          )}
        </div>

        <Button
          variant="ghost"
          size="sm"
          onClick={handleCopy}
          className="text-muted-foreground"
        >
          {copied ? <Check className="text-ok" /> : <Copy />}
          {copied ? "Copied" : "Copy"}
        </Button>
      </div>

      {/* Body */}
      <div
        ref={containerRef}
        onScroll={handleScroll}
        className="relative flex-1 overflow-auto py-3 pr-4 font-mono text-[12.5px] leading-5 selection:bg-foreground/20"
      >
        {logs ? (
          <div>
            {formatLogs(logs)}
            {isBuilding && (
              <div className="mt-2 flex items-center gap-2 pl-9 text-muted-foreground">
                <span>$</span>
                <span aria-hidden="true" className="h-4 w-2 bg-brand/80" />
              </div>
            )}
          </div>
        ) : isBuilding ? (
          <div className="flex items-center gap-2 px-4 py-2 text-muted-foreground">
            <span
              aria-hidden="true"
              className="size-2 rounded-full bg-warn animate-pulse motion-reduce:animate-none"
            />
            Preparing the build environment…
          </div>
        ) : (
          <div className="flex h-full flex-col items-center justify-center px-6 py-14 text-center font-sans">
            <p className="text-[15px] font-semibold text-foreground">No build output yet</p>
            <p className="mt-1 max-w-sm text-[13px] text-muted-foreground">
              Output streams here when a deployment starts.
            </p>
          </div>
        )}
      </div>
    </div>
  );
}
