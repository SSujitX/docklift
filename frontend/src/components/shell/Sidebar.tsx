// The left rail: brand, primary action, grouped navigation (with Settings tree),
// and version/upgrade footer. Shared by the desktop rail and the mobile drawer.

import { useEffect, useState } from "react";
import { Link, useLocation } from "react-router-dom";
import {
  ChevronDown,
  Container,
  PanelLeftClose,
  PanelLeftOpen,
  Plus,
  X,
} from "lucide-react";
import { cn } from "@/lib/utils";
import { settingsTabFromSearch } from "@/lib/settingsNav";
import { isNavActive, navGroups, type NavItem } from "./navigation";
import { SidebarStatus } from "./SidebarStatus";
import { useShell } from "./ShellContext";

function SettingsTree({
  item,
  isCollapsed,
  onNavigate,
}: {
  item: NavItem;
  isCollapsed: boolean;
  onNavigate: () => void;
}) {
  const { pathname, search } = useLocation();
  const onSettings = pathname.startsWith("/settings");
  const [peekOpen, setPeekOpen] = useState(false);
  const [userCollapsed, setUserCollapsed] = useState(false);
  const activeTab = settingsTabFromSearch(search);

  useEffect(() => {
    if (!onSettings) {
      setPeekOpen(false);
      setUserCollapsed(false);
    }
  }, [onSettings]);

  // Collapsed by default; auto-expand on Settings routes (chevron can collapse).
  const treeOpen =
    !isCollapsed && (onSettings ? !userCollapsed : peekOpen);
  const parentActive = onSettings;

  if (isCollapsed) {
    return (
      <Link
        to="/settings?tab=profile"
        onClick={onNavigate}
        title="Settings"
        aria-current={parentActive ? "page" : undefined}
        className={cn(
          "group relative flex h-10 items-center justify-center rounded-xl text-sm font-medium transition-colors",
          parentActive
            ? "bg-brand/12 text-sidebar-foreground"
            : "text-sidebar-muted hover:bg-sidebar-accent/70 hover:text-sidebar-foreground",
        )}
      >
        {parentActive && (
          <span className="absolute left-0 top-1/2 h-5 w-[3px] -translate-y-1/2 rounded-r-full bg-brand shadow-[0_0_12px_hsl(var(--brand)/0.7)]" />
        )}
        <item.icon
          className={cn(
            "h-4.5 w-4.5 shrink-0",
            parentActive ? "text-brand" : "text-sidebar-muted group-hover:text-sidebar-foreground",
          )}
        />
      </Link>
    );
  }

  return (
    <div className="space-y-0.5">
      <div
        className={cn(
          "group relative flex items-center rounded-xl text-sm font-medium transition-colors",
          parentActive
            ? "bg-brand/12 text-sidebar-foreground"
            : "text-sidebar-muted hover:bg-sidebar-accent/70 hover:text-sidebar-foreground",
        )}
      >
        {parentActive && (
          <span className="absolute left-0 top-1/2 h-5 w-[3px] -translate-y-1/2 rounded-r-full bg-brand shadow-[0_0_12px_hsl(var(--brand)/0.7)]" />
        )}
        <Link
          to="/settings?tab=profile"
          onClick={onNavigate}
          className="flex min-w-0 flex-1 items-center gap-2.5 px-2.5 py-2"
        >
          <item.icon
            className={cn(
              "h-4.5 w-4.5 shrink-0",
              parentActive ? "text-brand" : "text-sidebar-muted group-hover:text-sidebar-foreground",
            )}
          />
          <span className="truncate">{item.label}</span>
        </Link>
        <button
          type="button"
          onClick={() => {
            if (onSettings) setUserCollapsed((value) => !value);
            else setPeekOpen((open) => !open);
          }}
          aria-expanded={treeOpen}
          aria-label={treeOpen ? "Collapse settings" : "Expand settings"}
          className="mr-1.5 flex h-7 w-7 shrink-0 items-center justify-center rounded-lg text-sidebar-muted transition-colors hover:bg-sidebar-accent hover:text-sidebar-foreground"
        >
          <ChevronDown
            className={cn(
              "h-4 w-4 transition-transform duration-200",
              treeOpen && "rotate-180",
            )}
          />
        </button>
      </div>

      {treeOpen && item.children && (
        <div className="ml-3 space-y-0.5 border-l border-sidebar-border pl-2">
          {item.children.map((child) => {
            const tab = new URLSearchParams(child.href.split("?")[1] || "").get(
              "tab",
            );
            const childActive = onSettings && activeTab === tab;
            return (
              <Link
                key={child.href}
                to={child.href}
                onClick={onNavigate}
                aria-current={childActive ? "page" : undefined}
                className={cn(
                  "flex items-center gap-2 rounded-lg px-2.5 py-1.5 text-[13px] font-medium transition-colors",
                  childActive
                    ? "bg-brand/10 text-brand"
                    : "text-sidebar-muted hover:bg-sidebar-accent/70 hover:text-sidebar-foreground",
                )}
              >
                <child.icon className="h-3.5 w-3.5 shrink-0" />
                <span className="truncate">{child.label}</span>
              </Link>
            );
          })}
        </div>
      )}
    </div>
  );
}

export function Sidebar({
  variant,
  expandedOnHover = false,
}: {
  variant: "desktop" | "mobile";
  expandedOnHover?: boolean;
}) {
  const { pathname } = useLocation();
  const { collapsed, toggleCollapsed, setMobileOpen } = useShell();
  const isMobile = variant === "mobile";
  // The drawer is always full width; only the desktop rail can collapse.
  const isCollapsed = !isMobile && collapsed && !expandedOnHover;

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
          className={cn(
            "group flex min-w-0 items-center gap-2.5",
            isCollapsed ? "flex-none justify-center" : "flex-1",
          )}
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
              title={
                expandedOnHover
                  ? "Keep sidebar open (Ctrl+B)"
                  : "Collapse sidebar (Ctrl+B)"
              }
              aria-label={expandedOnHover ? "Keep sidebar open" : "Collapse sidebar"}
              className="flex h-8 w-8 items-center justify-center rounded-lg text-sidebar-muted transition-colors hover:bg-sidebar-accent hover:text-sidebar-foreground"
            >
              {expandedOnHover ? (
                <PanelLeftOpen className="h-4 w-4" />
              ) : (
                <PanelLeftClose className="h-4 w-4" />
              )}
            </button>
          )
        )}
      </div>

      <div className={cn("shrink-0 pt-3", isCollapsed ? "px-2" : "px-3")}>
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
              if (item.children?.length) {
                return (
                  <SettingsTree
                    key={item.href}
                    item={item}
                    isCollapsed={isCollapsed}
                    onNavigate={() => isMobile && setMobileOpen(false)}
                  />
                );
              }

              const active = !item.external && isNavActive(pathname, item);
              const className = cn(
                "group relative flex items-center rounded-xl text-sm font-medium transition-colors",
                isCollapsed ? "h-10 justify-center" : "gap-2.5 px-2.5 py-2",
                active
                  ? "bg-brand/12 text-sidebar-foreground"
                  : "text-sidebar-muted hover:bg-sidebar-accent/70 hover:text-sidebar-foreground",
              );
              const icon = (
                <item.icon
                  className={cn(
                    "h-4.5 w-4.5 shrink-0 transition-colors",
                    active ? "text-brand" : "text-sidebar-muted group-hover:text-sidebar-foreground",
                  )}
                />
              );

              if (item.external) {
                return (
                  <a
                    key={item.href}
                    href={item.href}
                    target="_blank"
                    rel="noopener noreferrer"
                    onClick={() => isMobile && setMobileOpen(false)}
                    title={isCollapsed ? item.label : undefined}
                    className={className}
                  >
                    {icon}
                    {!isCollapsed && <span className="truncate">{item.label}</span>}
                  </a>
                );
              }

              return (
                <Link
                  key={item.href}
                  to={item.href}
                  onClick={() => isMobile && setMobileOpen(false)}
                  title={isCollapsed ? item.label : undefined}
                  aria-current={active ? "page" : undefined}
                  className={className}
                >
                  {active && (
                    <span className="absolute left-0 top-1/2 h-5 w-[3px] -translate-y-1/2 rounded-r-full bg-brand shadow-[0_0_12px_hsl(var(--brand)/0.7)]" />
                  )}
                  {icon}
                  {!isCollapsed && <span className="truncate">{item.label}</span>}
                </Link>
              );
            })}
          </div>
        ))}
      </nav>

      <div
        className={cn(
          "shrink-0 border-t border-sidebar-border py-3",
          isCollapsed ? "px-2" : "px-3",
        )}
      >
        <SidebarStatus collapsed={isCollapsed} />
      </div>
    </div>
  );
}
