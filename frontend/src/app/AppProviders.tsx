import { ThemeProvider } from "@/lib/theme";
import { AuthProvider } from "@/components/AuthProvider";
import { Toaster } from "@/components/ui/sonner";
import { ScrollToTop } from "@/components/ScrollToTop";
import { VersionChecker } from "@/components/VersionChecker";
import type { ReactNode } from "react";

export function AppProviders({ children }: { children: ReactNode }) {
  return (
    <ThemeProvider
      attribute="class"
      defaultTheme="system"
      enableSystem
      disableTransitionOnChange
    >
      <AuthProvider>
        <VersionChecker />
        <ScrollToTop />
        {children}
        {/* Top-center: visible above sticky footers on every page */}
        <Toaster />
      </AuthProvider>
    </ThemeProvider>
  );
}
