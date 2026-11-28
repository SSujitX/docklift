// Terminal page — one-viewport layout; xterm fills remaining height.

import { Suspense } from "react";
import { Loader2 } from "lucide-react";
import { TerminalView } from "@/components/TerminalView";
import { PageHeader } from "@/components/shell/PageHeader";

// main has py-6 (3rem total) under the 48px top bar
const FILL_VIEWPORT = "h-[calc(100dvh-var(--shell-topbar)-3rem)] min-h-[26rem]";

function TerminalContent() {
  return (
    <div className={`flex flex-col ${FILL_VIEWPORT}`}>
      <PageHeader
        title="Terminal"
        description="Root shell on this host. Host actions ask for your account password."
        className="mb-4 shrink-0"
      />

      <TerminalView className="min-h-0 flex-1" />
    </div>
  );
}

export default function TerminalPage() {
  return (
    <Suspense
      fallback={
        <div className={`flex items-center justify-center ${FILL_VIEWPORT}`}>
          <Loader2 className="size-5 animate-spin text-subtle" />
        </div>
      }
    >
      <TerminalContent />
    </Suspense>
  );
}
