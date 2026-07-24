// Authenticated application shell: fixed left rail, mobile drawer, thin context
// bar and the routed page. The rail is fixed rather than a flex column so the
// document keeps its normal scroll behaviour (sticky page elements, xterm sizing).

import { useState, useRef, type CSSProperties } from "react";
import { Outlet } from "react-router-dom";
import { CommandPalette } from "@/components/shell/CommandPalette";
import { Sidebar } from "@/components/shell/Sidebar";
import { ShellProvider, useShell } from "@/components/shell/ShellContext";
import { TopBar } from "@/components/shell/TopBar";
import { cn } from "@/lib/utils";
import { useFocusTrap } from "@/lib/focusTrap";

function ShellFrame() {
  const { collapsed, mobileOpen, setMobileOpen } = useShell();
  const [desktopRailHovered, setDesktopRailHovered] = useState(false);
  const previewExpanded = collapsed && desktopRailHovered;
  const mobileDrawerRef = useRef<HTMLElement>(null);

  useFocusTrap(mobileOpen, mobileDrawerRef);

  return (
    <div
      className="min-h-screen bg-background"
      style={
        collapsed && !previewExpanded
          ? ({ "--shell-rail": "var(--shell-rail-collapsed)" } as CSSProperties)
          : undefined
      }
    >
      <aside
        className="shell-rail fixed inset-y-0 left-0 z-40 hidden transition-[width] duration-200 ease-out lg:block"
        onMouseEnter={() => collapsed && setDesktopRailHovered(true)}
        onMouseLeave={() => setDesktopRailHovered(false)}
      >
        <Sidebar variant="desktop" expandedOnHover={previewExpanded} />
      </aside>

      <div
        className={cn(
          "fixed inset-0 z-50 bg-background/70 backdrop-blur-sm transition-opacity duration-200 lg:hidden",
          mobileOpen ? "opacity-100" : "pointer-events-none opacity-0",
        )}
        onClick={() => setMobileOpen(false)}
        aria-hidden={!mobileOpen}
      />
      <aside
        ref={mobileDrawerRef}
        className={cn(
          "fixed inset-y-0 left-0 z-50 w-[17rem] max-w-[85vw] transition-transform duration-200 ease-out lg:hidden",
          // `invisible` keeps the closed drawer out of the tab order.
          mobileOpen ? "visible translate-x-0" : "invisible -translate-x-full",
        )}
        aria-hidden={!mobileOpen}
      >
        <Sidebar variant="mobile" />
      </aside>

      <div className="shell-inset transition-[padding] duration-200 ease-out">
        <TopBar />
        <main className="mx-auto w-full max-w-7xl px-4 py-8 md:px-8">
          <Outlet />
        </main>
      </div>

      <CommandPalette />
    </div>
  );
}

export function AppShell() {
  return (
    <ShellProvider>
      <ShellFrame />
    </ShellProvider>
  );
}
