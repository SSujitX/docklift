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
import { buttonVariants } from "@/components/ui/button";
import { cn } from "@/lib/utils";
import { settingsTabFromSearch } from "@/lib/settingsNav";
import { prefetchIntent } from "@/lib/prefetch";
import { isNavActive, navGroups, type NavItem } from "./navigation";
import { SidebarStatus } from "./SidebarStatus";
import { useShell } from "./ShellContext";

const FOCUS_RING =
  "focus-visible:outline-2 focus-visible:-outline-offset-2 focus-visible:outline-ring";

/** One nav row: 32px (40px on touch), quiet until hovered or active. */
function itemClass(active: boolean, isCollapsed: boolean) {
  return cn(
    "group flex h-8 items-center rounded-md text-[13.5px] font-medium transition-colors duration-150 pointer-coarse:h-10",
    FOCUS_RING,
    isCollapsed ? "justify-center" : "gap-2.5 px-2",
    active
      ? "bg-sidebar-accent text-sidebar-foreground"
      : "text-sidebar-muted hover:bg-sidebar-accent hover:text-sidebar-foreground",
  );
}

function iconClass(active: boolean) {
  return cn(
    "size-4 shrink-0 transition-colors duration-150",
    active ? "text-sidebar-foreground" : "text-subtle group-hover:text-sidebar-foreground",
  );
}

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
        className={itemClass(parentActive, true)}
      >
        <item.icon className={iconClass(parentActive)} />
      </Link>
    );
  }

  return (
    <div className="space-y-0.5">
      <div
        className={cn(
          "group flex items-center rounded-md text-[13.5px] font-medium transition-colors duration-150",
          parentActive
            ? "bg-sidebar-accent text-sidebar-foreground"
            : "text-sidebar-muted hover:bg-sidebar-accent hover:text-sidebar-foreground",
        )}
      >
        <Link
          to="/settings?tab=profile"
          onClick={onNavigate}
          className={cn(
            "flex h-8 min-w-0 flex-1 items-center gap-2.5 rounded-md px-2 pointer-coarse:h-10",
            FOCUS_RING,
          )}
        >
          <item.icon className={iconClass(parentActive)} />
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
          className={cn(
            "mr-1 flex size-6 shrink-0 items-center justify-center rounded-md text-subtle transition-colors duration-150 hover:text-sidebar-foreground pointer-coarse:size-10 pointer-coarse:mr-0",
            FOCUS_RING,
          )}
        >
          <ChevronDown
            className={cn(
              "size-4 transition-transform duration-150 motion-reduce:transition-none",
              treeOpen && "rotate-180",
            )}
          />
        </button>
      </div>

      {treeOpen && item.children && (
        <div className="ml-6 space-y-0.5">
          {item.children.map((child) => {
            const tab = new URLSearchParams(child.href.split("?")[1] || "").get(
              "tab",
            );
            const childActive = onSettings && activeTab === tab;
            return (
              <Link
                key={child.href}
                to={child.href}
                {...prefetchIntent(child.href)}
                onClick={onNavigate}
                aria-current={childActive ? "page" : undefined}
                className={cn(
                  "group flex h-7 items-center gap-2 rounded-md px-2 text-[13px] font-medium transition-colors duration-150 pointer-coarse:h-10",
                  FOCUS_RING,
                  childActive
                    ? "bg-sidebar-accent text-sidebar-foreground"
                    : "text-sidebar-muted hover:bg-sidebar-accent hover:text-sidebar-foreground",
                )}
              >
                <child.icon
                  className={cn(
                    "size-3.5 shrink-0",
                    childActive ? "text-sidebar-foreground" : "text-subtle group-hover:text-sidebar-foreground",
                  )}
                />
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
    <div className="flex h-full flex-col border-r border-sidebar-border bg-sidebar text-sidebar-foreground">
      <div
        className={cn(
          "flex h-12 shrink-0 items-center",
          isCollapsed ? "justify-center px-2" : "gap-2 pl-4 pr-2",
        )}
      >
        <Link
          to="/"
          {...prefetchIntent("/")}
          className={cn(
            "flex min-w-0 items-center gap-2.5 rounded-md",
            "focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-ring",
            isCollapsed ? "flex-none justify-center" : "flex-1",
          )}
          onClick={() => isMobile && setMobileOpen(false)}
        >
          <span className="flex size-6 shrink-0 items-center justify-center rounded-md bg-brand">
            <Container className="size-3.5 stroke-2 text-brand-foreground" />
          </span>
          {!isCollapsed && (
            <span className="truncate text-sm font-semibold">Docklift</span>
          )}
        </Link>

        {isMobile ? (
          <button
            type="button"
            onClick={() => setMobileOpen(false)}
            aria-label="Close navigation"
            className={cn(
              "flex size-8 items-center justify-center rounded-md text-subtle transition-colors duration-150 hover:bg-sidebar-accent hover:text-sidebar-foreground pointer-coarse:size-10",
              FOCUS_RING,
            )}
          >
            <X className="size-4" />
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
              className={cn(
                "flex size-8 items-center justify-center rounded-md text-subtle transition-colors duration-150 hover:bg-sidebar-accent hover:text-sidebar-foreground pointer-coarse:size-10",
                FOCUS_RING,
              )}
            >
              {expandedOnHover ? (
                <PanelLeftOpen className="size-4" />
              ) : (
                <PanelLeftClose className="size-4" />
              )}
            </button>
          )
        )}
      </div>

      <div className={cn("shrink-0 pt-1", isCollapsed ? "flex justify-center px-2" : "px-2")}>
        <Link
          to="/projects/new"
          onClick={() => isMobile && setMobileOpen(false)}
          title={isCollapsed ? "New project" : undefined}
          aria-label="New project"
          data-slot="button"
          className={cn(
            buttonVariants({ variant: "outline", size: isCollapsed ? "icon" : "default" }),
            !isCollapsed && "w-full",
          )}
        >
          <Plus className="size-4 text-subtle" />
          {!isCollapsed && "New project"}
        </Link>
      </div>

      <nav className="shell-scroll mt-4 flex-1 space-y-4 overflow-y-auto px-2 pb-4">
        {navGroups.map((group) => (
          <div key={group.label} className="space-y-0.5">
            {isCollapsed ? (
              <div className="mx-auto mb-2 h-px w-6 bg-sidebar-border" />
            ) : (
              <p className="mb-1 px-2 text-xs leading-5 text-subtle">
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
              const className = itemClass(active, isCollapsed);
              const icon = <item.icon className={iconClass(active)} />;

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
                  {...prefetchIntent(item.href)}
                  onClick={() => isMobile && setMobileOpen(false)}
                  title={isCollapsed ? item.label : undefined}
                  aria-current={active ? "page" : undefined}
                  className={className}
                >
                  {icon}
                  {!isCollapsed && <span className="truncate">{item.label}</span>}
                </Link>
              );
            })}
          </div>
        ))}
      </nav>

      <div className="shrink-0 border-t border-sidebar-border px-2 py-2">
        <SidebarStatus collapsed={isCollapsed} />
      </div>
    </div>
  );
}
