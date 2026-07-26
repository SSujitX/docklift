// Terminal page — one-viewport layout; xterm fills remaining height.

import { Suspense } from "react";
import { Loader2, SquareTerminal } from "lucide-react";
import { TerminalView } from "@/components/TerminalView";

function TerminalContent() {
  return (
    <div
      className={
        "flex h-[calc(100dvh-var(--shell-topbar)-2.5rem)] flex-col " +
        "sm:h-[calc(100dvh-var(--shell-topbar)-4rem)]"
      }
    >
      <div className="mb-2 flex shrink-0 items-center gap-2.5 sm:mb-3 sm:gap-3">
        <span className="flex h-9 w-9 shrink-0 items-center justify-center rounded-xl border border-brand/20 bg-brand/10 sm:h-10 sm:w-10 sm:rounded-2xl">
          <SquareTerminal className="h-4 w-4 text-brand sm:h-[1.125rem] sm:w-[1.125rem]" />
        </span>
        <div className="min-w-0">
          <h1 className="truncate text-lg font-bold tracking-tight sm:text-xl">
            Terminal
          </h1>
          <p className="truncate text-[11px] text-muted-foreground sm:text-xs">
            Root shell on this host · packages, upgrade, purge, reboot
          </p>
        </div>
      </div>

      <TerminalView className="min-h-0 flex-1" />
    </div>
  );
}

export default function TerminalPage() {
  return (
    <Suspense
      fallback={
        <div
          className={
            "flex h-[calc(100dvh-var(--shell-topbar)-2.5rem)] items-center justify-center " +
            "sm:h-[calc(100dvh-var(--shell-topbar)-4rem)]"
          }
        >
          <Loader2 className="h-8 w-8 animate-spin text-brand" />
        </div>
      }
    >
      <TerminalContent />
    </Suspense>
  );
}
