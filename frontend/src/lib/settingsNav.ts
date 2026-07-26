// Settings sections shared by the shell tree, Settings page tabs, and command palette.

import type { ComponentType } from "react";
import {
  Archive,
  Container,
  Globe,
  Network,
  RotateCcw,
  Server,
  User,
} from "lucide-react";
import { GithubIcon } from "@/components/icons/GithubIcon";

export type SettingsIcon = ComponentType<{
  className?: string;
  strokeWidth?: number;
}>;

export interface SettingsSection {
  id: string;
  label: string;
  description: string;
  icon: SettingsIcon;
}

export const SETTINGS_SECTIONS: SettingsSection[] = [
  {
    id: "profile",
    label: "Profile",
    description: "Account identity and password",
    icon: User,
  },
  {
    id: "github",
    label: "GitHub",
    description: "Deploy from repositories",
    icon: GithubIcon,
  },
  {
    id: "server",
    label: "Server",
    description: "Host identity and IP",
    icon: Server,
  },
  {
    id: "port",
    label: "Port",
    description: "Host port pool defaults",
    icon: Network,
  },
  {
    id: "docker",
    label: "Docker",
    description: "Engine and runtime notes",
    icon: Container,
  },
  {
    id: "domain",
    label: "Domain",
    description: "Panel hostname and HTTPS",
    icon: Globe,
  },
  {
    id: "backup",
    label: "Backup",
    description: "Create and download snapshots",
    icon: Archive,
  },
  {
    id: "restore",
    label: "Restore",
    description: "Upload and restore snapshots",
    icon: RotateCcw,
  },
];

export const SETTINGS_TAB_IDS = SETTINGS_SECTIONS.map((s) => s.id);

export function settingsHref(tabId: string): string {
  return `/settings?tab=${tabId}`;
}

export function settingsTabFromSearch(search: string): string {
  const tab = new URLSearchParams(search).get("tab");
  if (tab && SETTINGS_TAB_IDS.includes(tab)) return tab;
  return "profile";
}

export function settingsSectionLabel(tabId: string): string {
  return SETTINGS_SECTIONS.find((s) => s.id === tabId)?.label || "Settings";
}
