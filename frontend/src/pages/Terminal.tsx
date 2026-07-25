// Terminal page - web-based shell and control center

import { Suspense } from "react";
import { Loader2, SquareTerminal } from "lucide-react";
import { PageHeader } from "@/components/shell/PageHeader";
import { TerminalView } from "@/components/TerminalView";

function TerminalContent() {
  return (
    <>
      <PageHeader
        eyebrow="Operate"
        title="Terminal"
        description="Root shell on this host, plus quiet controls for packages, upgrades, and restarts."
        icon={SquareTerminal}
      />
      <TerminalView />
    </>
  );
}

export default function TerminalPage() {
  return (
    <Suspense fallback={
      <div className="flex min-h-[60vh] items-center justify-center">
        <Loader2 className="h-8 w-8 animate-spin text-brand" />
      </div>
    }>
      <TerminalContent />
    </Suspense>
  );
}
