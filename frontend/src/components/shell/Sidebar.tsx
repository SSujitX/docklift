// The left rail: brand, primary action, grouped navigation, release status and
// the account block. Shared by the desktop rail and the mobile drawer.

import { Link, useLocation } from "react-router-dom";
import {
  Container,
  PanelLeftClose,
  PanelLeftOpen,
  Plus,
  Search,
  X,
} from "lucide-react";
import { cn } from "@/lib/utils";
import { isNavActive, navGroups } from "./navigation";
import { SidebarStatus } from "./SidebarStatus";
import { SidebarUser } from "./SidebarUser";
import { useShell } from "./ShellContext";

export function Sidebar({ variant }: { variant: "desktop" | "mobile" }) {
  const { pathname } = useLocation();
  const { collapsed, toggleCollapsed, setMobileOpen, setPaletteOpen } = useShell();
  const isMobile = variant === "mobile";
  // The drawer is always full width; only the desktop rail can collapse.
  const isCollapsed = !isMobile && collapsed;

  return (
    <div className="shell-rail-surface flex h-full flex-col border-r border-sidebar-border text-sidebar-foreground">
      <div
        className={cn(
          "flex h-14 shrink-0 items-center border-b border-sidebar-border",
          isCollapsed ? "justify-center px-2" : "gap-2 px-3",
        )}
      >
        <Link
          to="/"
          className="group flex min-w-0 flex-1 items-center gap-2.5"
          onClick={() => isMobile && setMobileOpen(false)}
        >
          <span className="relative flex h-9 w-9 shrink-0 items-center justify-center rounded-xl bg-gradient-to-br from-brand to-blue-600 shadow-lg shadow-brand/20 transition-transform duration-300 group-hover:scale-105">
            <Container className="h-[18px] w-[18px] text-white" strokeWidth={2.5} />
          </span>
          {!isCollapsed && (
            <span className="min-w-0">
              <span className="block truncate text-[15px] font-bold leading-tight tracking-tight">
                Docklift
              </span>
              <span className="block text-[9px] font-semibold uppercase tracking-[0.2em] text-brand">
                Deploy Deck
              </span>
            </span>
          )}
        </Link>

        {isMobile ? (
          <button
            type="button"
            onClick={() => setMobileOpen(false)}
            aria-label="Close navigation"
            className="flex h-9 w-9 items-center justify-center rounded-xl text-sidebar-muted transition-colors hover:bg-sidebar-accent hover:text-sidebar-foreground"
          >
            <X className="h-4.5 w-4.5" />
          </button>
        ) : (
          !isCollapsed && (
            <button
              type="button"
              onClick={toggleCollapsed}
              title="Collapse sidebar (Ctrl+B)"
              aria-label="Collapse sidebar"
              className="flex h-8 w-8 items-center justify-center rounded-lg text-sidebar-muted transition-colors hover:bg-sidebar-accent hover:text-sidebar-foreground"
            >
              <PanelLeftClose className="h-4 w-4" />
            </button>
          )
        )}
      </div>

      <div className={cn("shrink-0 space-y-2 pt-3", isCollapsed ? "px-2" : "px-3")}>
        {isCollapsed && (
          <button
            type="button"
            onClick={toggleCollapsed}
            title="Expand sidebar (Ctrl+B)"
            aria-label="Expand sidebar"
            className="flex h-9 w-full items-center justify-center rounded-xl text-sidebar-muted transition-colors hover:bg-sidebar-accent hover:text-sidebar-foreground"
          >
            <PanelLeftOpen className="h-4 w-4" />
          </button>
        )}

        <Link
          to="/projects/new"
          onClick={() => isMobile && setMobileOpen(false)}
          title={isCollapsed ? "New project" : undefined}
          className={cn(
            "flex h-10 items-center justify-center gap-2 rounded-xl bg-brand text-sm font-semibold text-brand-foreground",
            "shadow-lg shadow-brand/20 transition-all hover:brightness-110 active:scale-[0.98]",
          )}
        >
          <Plus className="h-4 w-4" strokeWidth={2.5} />
          {!isCollapsed && "New project"}
        </Link>

        <button
          type="button"
          onClick={() => {
            setMobileOpen(false);
            setPaletteOpen(true);
          }}
          title={isCollapsed ? "Search (Ctrl+K)" : undefined}
          className={cn(
            "flex h-9 w-full items-center rounded-xl border border-sidebar-border bg-sidebar-accent/40 text-sidebar-muted",
            "transition-colors hover:border-sidebar-muted/40 hover:text-sidebar-foreground",
            isCollapsed ? "justify-center" : "gap-2 px-2.5",
          )}
        >
          <Search className="h-4 w-4 shrink-0" />
          {!isCollapsed && (
            <>
              <span className="flex-1 text-left text-xs font-medium">Search…</span>
              <kbd className="rounded border border-sidebar-border bg-sidebar px-1.5 py-0.5 text-[10px] font-semibold">
                ⌘K
              </kbd>
            </>
          )}
        </button>
      </div>

      <nav
        className={cn(
          "shell-scroll mt-4 flex-1 space-y-5 overflow-y-auto pb-4",
          isCollapsed ? "px-2" : "px-3",
        )}
      >
        {navGroups.map((group) => (
          <div key={group.label} className="space-y-1">
            {isCollapsed ? (
              <div className="mx-auto mb-2 h-px w-6 bg-sidebar-border" />
            ) : (
              <p className="px-2.5 pb-1 text-[10px] font-semibold uppercase tracking-[0.18em] text-sidebar-muted/70">
                {group.label}
              </p>
            )}

            {group.items.map((item) => {
              const active = isNavActive(pathname, item);
              return (
                <Link
                  key={item.href}
                  to={item.href}
                  onClick={() => isMobile && setMobileOpen(false)}
                  title={isCollapsed ? item.label : undefined}
                  aria-current={active ? "page" : undefined}
                  className={cn(
                    "group relative flex items-center rounded-xl text-sm font-medium transition-colors",
                    isCollapsed ? "h-10 justify-center" : "gap-2.5 px-2.5 py-2",
                    active
                      ? "bg-brand/12 text-sidebar-foreground"
                      : "text-sidebar-muted hover:bg-sidebar-accent/70 hover:text-sidebar-foreground",
                  )}
                >
                  {active && (
                    <span className="absolute left-0 top-1/2 h-5 w-[3px] -translate-y-1/2 rounded-r-full bg-brand shadow-[0_0_12px_hsl(var(--brand)/0.7)]" />
                  )}
                  <item.icon
                    className={cn(
                      "h-4.5 w-4.5 shrink-0 transition-colors",
                      active ? "text-brand" : "text-sidebar-muted group-hover:text-sidebar-foreground",
                    )}
                  />
                  {!isCollapsed && <span className="truncate">{item.label}</span>}
                </Link>
              );
            })}
          </div>
        ))}
      </nav>

      <div
        className={cn(
          "shrink-0 space-y-3 border-t border-sidebar-border py-3",
          isCollapsed ? "px-2" : "px-3",
        )}
      >
        <SidebarStatus collapsed={isCollapsed} />
        <SidebarUser collapsed={isCollapsed} />
      </div>
    </div>
  );
}
