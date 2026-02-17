// Terminal page - web-based shell and control center
"use client";

import { Header } from "@/components/Header";
import { Footer } from "@/components/Footer";
import { TerminalView } from "../../components/TerminalView";
import { Suspense } from "react";
import { Loader2 } from "lucide-react";

function TerminalContent() {
  return (
    <div className="min-h-screen flex flex-col">
      <Header />

      <main className="flex-1 container max-w-7xl mx-auto px-4 md:px-6 py-8 flex flex-col gap-6">
        <div>
          <h1 className="text-3xl font-bold tracking-tight">Terminal</h1>
          <p className="text-muted-foreground mt-1">Interactive shell with full system access</p>
        </div>

        <TerminalView />
      </main>

      <Footer />
    </div>
  );
}

export default function TerminalPage() {
  return (
    <Suspense fallback={
      <div className="min-h-screen flex items-center justify-center bg-black">
        <Loader2 className="h-8 w-8 animate-spin text-cyan-500" />
      </div>
    }>
      <TerminalContent />
    </Suspense>
  );
}
