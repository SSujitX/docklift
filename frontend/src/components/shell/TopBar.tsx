// Context strip above page content: breadcrumbs, search, GitHub star, theme,
// and account menu on the right.

import { Fragment, useEffect, useState } from "react";
import { Link, useLocation } from "react-router-dom";
import { ChevronRight, Container, Menu, Monitor, Moon, Search, Sun } from "lucide-react";
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

const FOCUS_RING =
  "focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-ring";

/** Ghost icon button: 32px, 40px on touch. */
const ICON_BUTTON = cn(
  "flex size-8 shrink-0 items-center justify-center rounded-md text-muted-foreground transition-colors duration-150 hover:bg-secondary hover:text-foreground pointer-coarse:size-10",
  FOCUS_RING,
);

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
    setModKey(isApple ? "⌘" : "Ctrl ");
  }, []);

  const cycleTheme = () => {
    const order = ["light", "dark", "system"] as const;
    const index = order.indexOf(theme);
    setTheme(order[(index + 1) % order.length]);
  };

  return (
    <header className="sticky top-0 z-30 flex h-12 items-center gap-2 border-b border-border bg-background px-4 sm:px-6 xl:px-8">
      <button
        type="button"
        onClick={() => setMobileOpen(true)}
        aria-label="Open navigation"
        className={cn(ICON_BUTTON, "-ml-2 lg:hidden")}
      >
        <Menu className="size-4" />
      </button>

      <Link
        to="/"
        className={cn("flex min-w-0 items-center gap-2 rounded-md lg:hidden", FOCUS_RING)}
        aria-label="Docklift home"
      >
        <span className="flex size-6 shrink-0 items-center justify-center rounded-md bg-brand">
          <Container className="size-3.5 stroke-2 text-brand-foreground" />
        </span>
        <span className="truncate text-[13.5px] font-medium text-foreground">
          {mobileTitle}
        </span>
      </Link>

      <nav
        aria-label="Breadcrumb"
        className="hidden min-w-0 flex-1 items-center gap-1.5 text-[13.5px] lg:flex"
      >
        {crumbs.map((crumb, index) => {
          const isLast = index === crumbs.length - 1;
          return (
            <Fragment key={`${crumb.label}-${index}`}>
              {index > 0 && (
                <ChevronRight className="size-3.5 shrink-0 text-subtle" />
              )}
              {crumb.href && !isLast ? (
                <Link
                  to={crumb.href}
                  className={cn(
                    "truncate rounded-sm text-muted-foreground transition-colors duration-150 hover:text-foreground",
                    FOCUS_RING,
                  )}
                >
                  {crumb.label}
                </Link>
              ) : (
                <span
                  className={cn(
                    "truncate",
                    isLast ? "font-medium text-foreground" : "text-muted-foreground",
                  )}
                >
                  {crumb.label}
                </span>
              )}
            </Fragment>
          );
        })}
      </nav>

      <div className="ml-auto flex shrink-0 items-center gap-1 sm:gap-1.5">
        <button
          type="button"
          onClick={() => setPaletteOpen(true)}
          aria-label="Open command palette"
          className={cn(
            "flex h-8 w-8 shrink-0 items-center justify-center rounded-md text-muted-foreground transition-colors duration-150 hover:bg-secondary hover:text-foreground pointer-coarse:h-10 pointer-coarse:w-10",
            "sm:w-56 sm:justify-start sm:gap-2 sm:border sm:border-input sm:bg-card sm:px-2.5 sm:text-subtle sm:hover:bg-card sm:hover:border-subtle sm:hover:text-muted-foreground sm:pointer-coarse:w-56",
            FOCUS_RING,
          )}
        >
          <Search className="size-4 shrink-0" />
          <span className="hidden flex-1 text-left text-[13px] sm:inline">Search…</span>
          <kbd className="hidden rounded-sm border border-border px-1 font-mono text-[11px] leading-4 text-subtle sm:inline">
            {modKey}K
          </kbd>
        </button>

        <a
          href="https://github.com/SSujitX/docklift"
          target="_blank"
          rel="noopener noreferrer"
          title={`Star on GitHub — ${formatStars(stars)}`}
          className={cn(
            "hidden h-8 items-center gap-1.5 rounded-md px-2 text-muted-foreground transition-colors duration-150 hover:bg-secondary hover:text-foreground md:flex",
            FOCUS_RING,
          )}
        >
          <GithubIcon className="size-4" />
          <span className="font-mono text-xs tabular-nums">{formatStars(stars)}</span>
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
          className={ICON_BUTTON}
        >
          {theme === "system" ? (
            <Monitor className="size-4" />
          ) : resolvedTheme === "dark" ? (
            <Sun className="size-4" />
          ) : (
            <Moon className="size-4" />
          )}
        </button>

        <AccountMenu />
      </div>
    </header>
  );
}
