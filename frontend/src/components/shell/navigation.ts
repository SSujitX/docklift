// Single source of truth for shell navigation — the sidebar, breadcrumbs and
// command palette all read from here so a new page only has to be added once.

import type { ComponentType } from "react";
import {
  Anchor,
  BookOpen,
  Database,
  Gauge,
  LayoutGrid,
  ScrollText,
  Settings,
  SquareTerminal,
} from "lucide-react";
import { SETTINGS_SECTIONS, settingsHref } from "@/lib/settingsNav";

/** lucide-react ships untyped here, so icons are described structurally. */
export type IconComponent = ComponentType<{
  className?: string;
  strokeWidth?: number;
}>;

export interface NavChild {
  label: string;
  href: string;
  icon: IconComponent;
  description?: string;
}

export interface NavItem {
  label: string;
  href: string;
  icon: IconComponent;
  description: string;
  /** Only highlight on an exact path match (used for the "/" root). */
  exact?: boolean;
  /** Opens outside the app shell (e.g. docs site). */
  external?: boolean;
  /** Collapsible tree children (e.g. Settings sections). */
  children?: NavChild[];
}

export interface NavGroup {
  label: string;
  items: NavItem[];
}

export const navGroups: NavGroup[] = [
  {
    label: "Deploy",
    items: [
      {
        label: "Projects",
        href: "/",
        icon: LayoutGrid,
        description: "Every application you deploy",
        exact: true,
      },
      {
        label: "Databases",
        href: "/databases",
        icon: Database,
        description: "Managed data services",
      },
      {
        label: "Ports",
        href: "/ports",
        icon: Anchor,
        description: "Allocated and free host ports",
      },
    ],
  },
  {
    label: "Operate",
    items: [
      {
        label: "System",
        href: "/system",
        icon: Gauge,
        description: "CPU, memory and disk pressure",
      },
      {
        label: "Logs",
        href: "/logs",
        icon: ScrollText,
        description: "Live output from every service",
      },
      {
        label: "Terminal",
        href: "/terminal",
        icon: SquareTerminal,
        description: "Interactive shell on the server",
      },
    ],
  },
  {
    label: "Workspace",
    items: [
      {
        label: "Settings",
        href: "/settings",
        icon: Settings,
        description: "Account, domains and integrations",
        children: SETTINGS_SECTIONS.map((section) => ({
          label: section.label,
          href: settingsHref(section.id),
          icon: section.icon,
          description: section.description,
        })),
      },
      {
        label: "Docs",
        href: "https://docklift.dev",
        icon: BookOpen,
        description: "Guides and commands on docklift.dev",
        external: true,
      },
    ],
  },
];

export const navItems: NavItem[] = navGroups.flatMap((group) => group.items);

export function isNavActive(pathname: string, item: NavItem): boolean {
  if (item.exact) return pathname === item.href;
  return pathname === item.href || pathname.startsWith(`${item.href}/`);
}

/** The nav entry that owns the current URL, including nested pages. */
export function activeNavItem(pathname: string): NavItem | undefined {
  const direct = navItems.find((item) => isNavActive(pathname, item));
  if (direct) return direct;
  // Project pages live under the Projects entry even though their paths differ.
  if (pathname.startsWith("/projects")) {
    return navItems.find((item) => item.href === "/");
  }
  return undefined;
}

const segmentLabels: Record<string, string> = {
  projects: "Projects",
  new: "New Project",
  databases: "Databases",
  ports: "Ports",
  system: "System",
  logs: "Logs",
  terminal: "Terminal",
  settings: "Settings",
};

export interface Crumb {
  label: string;
  href?: string;
}

/** Breadcrumb trail for the top bar. Unknown segments (ids, slugs) are titled. */
export function breadcrumbsFor(pathname: string, leafLabel?: string): Crumb[] {
  const segments = pathname.split("/").filter(Boolean);
  if (segments.length === 0) return [{ label: "Projects" }];

  if (segments[0] === "settings") {
    const crumbs: Crumb[] = [{ label: "Settings", href: "/settings" }];
    if (leafLabel) crumbs.push({ label: leafLabel });
    return crumbs;
  }

  const crumbs: Crumb[] = [];
  segments.forEach((segment, index) => {
    const href = `/${segments.slice(0, index + 1).join("/")}`;
    const isLast = index === segments.length - 1;
    const known = segmentLabels[segment];
    const label = known || (isLast && leafLabel) || titleize(segment);
    crumbs.push({ label, href: isLast ? undefined : href });
  });
  return crumbs;
}

function titleize(segment: string): string {
  const readable = segment.replace(/[-_]/g, " ");
  return readable.charAt(0).toUpperCase() + readable.slice(1);
}
