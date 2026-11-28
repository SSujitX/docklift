// Cache keys for each page's primary GET data, and prefetch-on-intent for the nav that opens them.
// Pages and prefetch must use the same URL strings, so both read them from here.

import { apiCache } from "./auth";
import { API_URL } from "./utils";

export const API = {
  projects: `${API_URL}/api/projects`,
  systemQuick: `${API_URL}/api/system/quick`,
  systemStats: `${API_URL}/api/system/stats`,
  systemIP: `${API_URL}/api/system/ip`,
  ports: `${API_URL}/api/ports`,
  tokens: `${API_URL}/api/tokens`,
  openapi: `${API_URL}/api/openapi.json`,
  githubStatus: `${API_URL}/api/github/status`,
  githubInstallations: `${API_URL}/api/github/installations`,
  domains: `${API_URL}/api/domains`,
  sslEmail: `${API_URL}/api/domains/ssl/email`,
  backups: `${API_URL}/api/backup`,
  backupUploads: `${API_URL}/api/backup/uploads`,
} as const;

/** Project detail: the requests its first poll makes (history page 0). */
export const projectAPI = {
  project: (id: string) => `${API_URL}/api/projects/${id}`,
  files: (id: string) => `${API_URL}/api/files/${id}`,
  services: (id: string) => `${API_URL}/api/deployments/${id}/services`,
  autoDeploy: (id: string) => `${API_URL}/api/projects/${id}/auto-deploy`,
  history: (id: string, limit: number, offset: number) =>
    `${API_URL}/api/deployments/${id}?limit=${limit}&offset=${offset}&meta=1`,
  latestSuccess: (id: string) =>
    `${API_URL}/api/deployments/${id}?limit=1&offset=0&status=success`,
};

const SETTINGS_TAB_DATA: Record<string, string[]> = {
  github: [API.githubStatus],
  server: [API.systemIP],
  domain: [API.domains, API.sslEmail, API.systemIP],
  backup: [API.backups],
  restore: [API.backupUploads],
};

/** Primary GET data for an in-app href (`/`, `/ports`, `/settings?tab=domain`, …). */
export function dataForHref(href: string): string[] {
  const [path, query = ""] = href.split("?");
  switch (path) {
    case "/":
      return [API.projects, API.systemQuick];
    case "/databases":
      return [API.projects];
    case "/ports":
      return [API.ports];
    case "/system":
      return [API.systemStats];
    case "/developer":
      return [API.tokens, API.openapi];
    case "/settings": {
      const tab = new URLSearchParams(query).get("tab") || "profile";
      return SETTINGS_TAB_DATA[tab] ?? [];
    }
    default:
      return [];
  }
}

const INTENT_DELAY_MS = 80;
let intentTimer: ReturnType<typeof setTimeout> | undefined;

/**
 * Hover / keyboard-focus handlers that warm the cache for `href` after a short pause. Cheap by
 * design: one pending timer app-wide, nothing for links without known data (external, streams),
 * and `apiCache.prefetch` skips data that is fresh or already loading.
 */
export function prefetchIntent(href: string) {
  const urls = dataForHref(href);
  if (urls.length === 0) return {};
  const start = () => {
    clearTimeout(intentTimer);
    intentTimer = setTimeout(() => {
      for (const url of urls) apiCache.prefetch(url);
    }, INTENT_DELAY_MS);
  };
  const cancel = () => clearTimeout(intentTimer);
  return {
    onPointerEnter: start,
    onFocus: start,
    onPointerLeave: cancel,
    onBlur: cancel,
  };
}
