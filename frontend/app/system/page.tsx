"use client";

import { Header } from "@/components/Header";
import { Footer } from "@/components/Footer";
import { SystemOverview } from "@/components/SystemOverview";

export default function SystemPage() {
  return (
    <div className="min-h-screen flex flex-col">
      <Header />

      <main className="flex-1 container max-w-7xl mx-auto px-4 md:px-6 py-8">
        <SystemOverview />
      </main>

      <Footer />
    </div>
  );
}
