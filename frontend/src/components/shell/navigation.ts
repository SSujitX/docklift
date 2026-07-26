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
  /**
   * Path prefixes that keep this rail item selected (section pages).
   * Default: the item `href` (and nested paths under it).
   * Use for sections whose list URL differs from nested URLs (e.g. Projects
   * list is `/` but create/detail are `/projects/*`).
   */
  section?: string[];
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
        // List is `/`; create + detail live under `/projects/*`.
        section: ["/", "/projects"],
      },
      {
        label: "Databases",
        href: "/databases",
        icon: Database,
        description: "Managed data services",
        // List + `/databases/new` (and future `/databases/:id`).
        section: ["/databases"],
      },
      {
        label: "Ports",
        href: "/ports",
        icon: Anchor,
        description: "Allocated and free host ports",
        section: ["/ports"],
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
        section: ["/system"],
      },
      {
        label: "Logs",
        href: "/logs",
        icon: ScrollText,
        description: "Live output from every service",
        section: ["/logs"],
      },
      {
        label: "Terminal",
        href: "/terminal",
        icon: SquareTerminal,
        description: "Interactive shell on the server",
        section: ["/terminal"],
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
        section: ["/settings"],
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

/** True when `pathname` is exactly `prefix` or nested under it. */
function pathInSection(pathname: string, prefix: string): boolean {
  if (prefix === "/") return pathname === "/";
  return pathname === prefix || pathname.startsWith(`${prefix}/`);
}

/**
 * Highlight a rail item for its whole section (list + nested create/detail),
 * not only the exact `href`.
 */
export function isNavActive(pathname: string, item: NavItem): boolean {
  const prefixes = item.section?.length ? item.section : [item.href];
  return prefixes.some((prefix) => pathInSection(pathname, prefix));
}

/** The nav entry that owns the current URL, including nested pages. */
export function activeNavItem(pathname: string): NavItem | undefined {
  return navItems.find((item) => isNavActive(pathname, item));
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
