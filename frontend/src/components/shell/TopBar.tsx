// Context strip above page content: breadcrumbs, search, GitHub star, theme,
// and account menu on the right.

import { Fragment, useEffect, useState } from "react";
import { Link, useLocation } from "react-router-dom";
import { ChevronRight, Container, Menu, Monitor, Moon, Search, Star, Sun } from "lucide-react";
import { GithubIcon } from "@/components/icons/GithubIcon";
import { useTheme } from "@/lib/theme";
import { cn } from "@/lib/utils";
import { AccountMenu } from "./AccountMenu";
import {
  formatStars,
  getCachedStars,
  loadStars,
} from "./SidebarStatus";
import { activeNavItem, breadcrumbsFor } from "./navigation";
import { useShell } from "./ShellContext";

export function TopBar() {
  const { pathname } = useLocation();
  const { setMobileOpen, setPaletteOpen, breadcrumbLeaf } = useShell();
  const { theme, resolvedTheme, setTheme } = useTheme();
  const [stars, setStars] = useState<number | null>(getCachedStars);
  const [modKey, setModKey] = useState("⌘");
  const crumbs = breadcrumbsFor(pathname, breadcrumbLeaf ?? undefined);
  const current = activeNavItem(pathname);
  const mobileTitle =
    breadcrumbLeaf || crumbs[crumbs.length - 1]?.label || current?.label || "Docklift";

  useEffect(() => {
    void loadStars().then(setStars);
  }, []);

  useEffect(() => {
    const isApple = /Mac|iPhone|iPad|iPod/i.test(navigator.platform);
    setModKey(isApple ? "⌘" : "Ctrl+");
  }, []);

  const cycleTheme = () => {
    const order = ["light", "dark", "system"] as const;
    const index = order.indexOf(theme);
    setTheme(order[(index + 1) % order.length]);
  };

  return (
    <header className="sticky top-0 z-30 flex h-14 items-center gap-2 border-b border-border/60 bg-background/80 px-3 backdrop-blur-xl sm:gap-3 sm:px-4 md:px-6">
      <button
        type="button"
        onClick={() => setMobileOpen(true)}
        aria-label="Open navigation"
        className="flex h-9 w-9 shrink-0 items-center justify-center rounded-xl border border-border/60 text-muted-foreground transition-colors hover:text-foreground lg:hidden"
      >
        <Menu className="h-4.5 w-4.5" />
      </button>

      <Link
        to="/"
        className="flex min-w-0 items-center gap-2 lg:hidden"
        aria-label="Docklift home"
      >
        <span className="flex h-7 w-7 shrink-0 items-center justify-center rounded-lg bg-gradient-to-br from-brand to-blue-600">
          <Container className="h-4 w-4 text-white" strokeWidth={2.5} />
        </span>
        <span className="truncate text-sm font-semibold tracking-tight">
          {mobileTitle}
        </span>
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

      <div className="ml-auto flex shrink-0 items-center gap-1.5 sm:gap-2">
        <button
          type="button"
          onClick={() => setPaletteOpen(true)}
          aria-label="Open command palette"
          className="flex h-9 w-9 items-center justify-center rounded-xl border border-border/60 bg-secondary/30 text-muted-foreground transition-colors hover:bg-secondary/60 hover:text-foreground sm:w-auto sm:min-w-[14rem] sm:gap-2 sm:px-3 md:min-w-[16rem] lg:min-w-[18rem]"
        >
          <Search className="h-4 w-4 shrink-0" />
          <span className="hidden flex-1 text-left text-xs font-medium sm:inline">
            Search…
          </span>
          <kbd className="hidden rounded border border-border/60 bg-background/80 px-1.5 py-0.5 text-[10px] font-semibold md:inline">
            {modKey}K
          </kbd>
        </button>

        <a
          href="https://github.com/SSujitX/docklift"
          target="_blank"
          rel="noopener noreferrer"
          title={`Star on GitHub — ${formatStars(stars)}`}
          className="hidden h-9 items-center gap-1.5 rounded-xl border border-border/60 px-2.5 text-muted-foreground transition-colors hover:text-foreground md:flex"
        >
          <GithubIcon className="h-4 w-4" />
          <span className="flex items-center gap-1 text-xs font-semibold tabular-nums">
            <Star className="h-3 w-3 fill-amber-400 text-amber-400" />
            {formatStars(stars)}
          </span>
        </a>

        <button
          type="button"
          onClick={cycleTheme}
          aria-label={`Theme: ${theme}. Click to cycle light, dark, system.`}
          title={
            theme === "system"
              ? `Theme: Auto (${resolvedTheme})`
              : `Theme: ${theme}`
          }
          className="flex h-9 w-9 items-center justify-center rounded-xl border border-border/60 text-muted-foreground transition-colors hover:text-foreground"
        >
          {theme === "system" ? (
            <Monitor className="h-4 w-4" />
          ) : resolvedTheme === "dark" ? (
            <Sun className="h-4 w-4" />
          ) : (
            <Moon className="h-4 w-4" />
          )}
        </button>

        <AccountMenu />
      </div>
    </header>
  );
}
