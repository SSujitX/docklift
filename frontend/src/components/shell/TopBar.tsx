// Thin context strip above page content. Navigation lives in the rail, so this
// only carries location (breadcrumbs) and the mobile drawer trigger.

import { Fragment } from "react";
import { Link, useLocation } from "react-router-dom";
import { ChevronRight, Container, Menu, Search } from "lucide-react";
import { cn } from "@/lib/utils";
import { activeNavItem, breadcrumbsFor } from "./navigation";
import { useShell } from "./ShellContext";

export function TopBar() {
  const { pathname } = useLocation();
  const { setMobileOpen, setPaletteOpen, breadcrumbLeaf } = useShell();
  const crumbs = breadcrumbsFor(pathname, breadcrumbLeaf ?? undefined);
  const current = activeNavItem(pathname);

  return (
    <header className="sticky top-0 z-30 flex h-14 items-center gap-3 border-b border-border/60 bg-background/80 px-4 backdrop-blur-xl md:px-6">
      <button
        type="button"
        onClick={() => setMobileOpen(true)}
        aria-label="Open navigation"
        className="flex h-9 w-9 shrink-0 items-center justify-center rounded-xl border border-border/60 text-muted-foreground transition-colors hover:text-foreground lg:hidden"
      >
        <Menu className="h-4.5 w-4.5" />
      </button>

      <Link to="/" className="flex items-center gap-2 lg:hidden">
        <span className="flex h-7 w-7 items-center justify-center rounded-lg bg-gradient-to-br from-brand to-blue-600">
          <Container className="h-4 w-4 text-white" strokeWidth={2.5} />
        </span>
        <span className="text-sm font-bold tracking-tight">Docklift</span>
      </Link>

      <nav
        aria-label="Breadcrumb"
        className="hidden min-w-0 flex-1 items-center gap-1.5 text-sm lg:flex"
      >
        {crumbs.map((crumb, index) => {
          const isLast = index === crumbs.length - 1;
          return (
            <Fragment key={`${crumb.label}-${index}`}>
              {index > 0 && (
                <ChevronRight className="h-3.5 w-3.5 shrink-0 text-muted-foreground/50" />
              )}
              {crumb.href && !isLast ? (
                <Link
                  to={crumb.href}
                  className="truncate text-muted-foreground transition-colors hover:text-foreground"
                >
                  {crumb.label}
                </Link>
              ) : (
                <span
                  className={cn(
                    "truncate",
                    isLast ? "font-semibold text-foreground" : "text-muted-foreground",
                  )}
                >
                  {crumb.label}
                </span>
              )}
            </Fragment>
          );
        })}
        {current?.description && crumbs.length === 1 && (
          <span className="ml-2 truncate text-xs text-muted-foreground/70">
            · {current.description}
          </span>
        )}
      </nav>

      <button
        type="button"
        onClick={() => setPaletteOpen(true)}
        aria-label="Open command palette"
        className="ml-auto flex h-9 items-center gap-2 rounded-xl border border-border/60 px-2.5 text-muted-foreground transition-colors hover:text-foreground"
      >
        <Search className="h-4 w-4" />
        <kbd className="hidden rounded border border-border/60 bg-secondary/60 px-1.5 py-0.5 text-[10px] font-semibold sm:inline">
          ⌘K
        </kbd>
      </button>
    </header>
  );
}
