// Release state for the rail footer: upgrade prompt only.
// GitHub stars live in the TopBar; version cache is still shared with Terminal.

import { useEffect, useState } from "react";
import { useNavigate } from "react-router-dom";
import { ArrowUp } from "lucide-react";
import { getAuthHeaders } from "@/lib/auth";
import { cn } from "@/lib/utils";

export interface VersionInfo {
  current: string;
  latest: string;
  updateAvailable: boolean;
  githubOk?: boolean;
  checkedAt?: string;
}

export function getCachedVersion(): VersionInfo | null {
  return cachedVersion;
}

/** Soft-cached version check shared with Terminal upgrade dialogs. */
export function fetchVersionInfo(forceRefresh = false): Promise<VersionInfo | null> {
  if (forceRefresh) {
    lastVersionFetchAt = 0;
    cachedVersion = null;
  }
  return loadVersion(forceRefresh);
}

// Soft cache shared by desktop + mobile rails. One poller only — both Sidebars
// mount in AppShell, so per-instance intervals would double GitHub traffic.
let cachedStars: number | null = null;
let cachedVersion: VersionInfo | null = null;
let versionRequest: Promise<VersionInfo | null> | null = null;
let starsRequest: Promise<number | null> | null = null;
let lastVersionFetchAt = 0;
/** Bumped on each network fetch so stale responses never overwrite newer ones. */
let versionFetchGen = 0;
let pollerSubscribers = 0;
let pollerIntervalId: number | null = null;
let focusBound = false;

const CLIENT_REFRESH_WHEN_CURRENT_MS = 2 * 60 * 1000;
const CLIENT_REFRESH_WHEN_UPDATE_MS = 15 * 60 * 1000;
const FOCUS_DEBOUNCE_MS = 60 * 1000;

type VersionListener = (data: VersionInfo | null) => void;
const versionListeners = new Set<VersionListener>();

function notifyVersionListeners(data: VersionInfo | null) {
  for (const listener of versionListeners) {
    listener(data);
  }
}

function loadVersion(forceRefresh = false): Promise<VersionInfo | null> {
  const now = Date.now();
  const ttl = cachedVersion?.updateAvailable
    ? CLIENT_REFRESH_WHEN_UPDATE_MS
    : cachedVersion?.githubOk === false
      ? 30 * 1000
      : CLIENT_REFRESH_WHEN_CURRENT_MS;

  if (
    !forceRefresh &&
    cachedVersion &&
    lastVersionFetchAt > 0 &&
    now - lastVersionFetchAt < ttl
  ) {
    return Promise.resolve(cachedVersion);
  }

  // Coalesce routine polls; force refresh always starts a fresh check
  if (versionRequest && !forceRefresh) {
    return versionRequest;
  }

  // Routine polls skip ?refresh=1 — server TTL is enough; force only for upgrade UI
  const url = forceRefresh
    ? "/api/system/version?refresh=1"
    : "/api/system/version";
  const gen = ++versionFetchGen;
  const req = fetch(url, { headers: getAuthHeaders() })
    .then((res) => (res.ok ? res.json() : null))
    .then((data: VersionInfo | null) => {
      // Superseded by a newer fetch (e.g. force refresh during a poll)
      if (gen !== versionFetchGen) {
        return cachedVersion;
      }
      if (data) {
        cachedVersion = data;
        lastVersionFetchAt = Date.now();
        notifyVersionListeners(data);
      }
      return data;
    })
    .catch(() => null)
    .finally(() => {
      if (versionRequest === req) versionRequest = null;
    });
  versionRequest = req;

  return req;
}

function ensureVersionPoller() {
  if (pollerIntervalId === null) {
    pollerIntervalId = window.setInterval(() => {
      if (cachedVersion?.updateAvailable) return;
      void loadVersion();
    }, CLIENT_REFRESH_WHEN_CURRENT_MS);
  }
  if (!focusBound) {
    focusBound = true;
    window.addEventListener("focus", () => {
      if (cachedVersion?.updateAvailable) return;
      if (Date.now() - lastVersionFetchAt < FOCUS_DEBOUNCE_MS) return;
      void loadVersion();
    });
  }
}

function releaseVersionPoller() {
  if (pollerSubscribers > 0) return;
  if (pollerIntervalId !== null) {
    window.clearInterval(pollerIntervalId);
    pollerIntervalId = null;
  }
}

export function loadStars(): Promise<number | null> {
  if (cachedStars !== null) return Promise.resolve(cachedStars);
  starsRequest ??= fetch("https://api.github.com/repos/SSujitX/docklift")
    .then((res) => (res.ok ? res.json() : null))
    .then((data) => {
      cachedStars = data?.stargazers_count ?? null;
      return cachedStars;
    })
    .catch(() => null);
  return starsRequest;
}

export function formatStars(count: number | null): string {
  if (count === null) return "—";
  if (count < 1000) return String(count);
  const thousands = count / 1000;
  return thousands % 1 === 0 ? `${thousands}k` : `${thousands.toFixed(1)}k`;
}

export function getCachedStars(): number | null {
  return cachedStars;
}

export function SidebarStatus({ collapsed }: { collapsed: boolean }) {
  const [version, setVersion] = useState<VersionInfo | null>(cachedVersion);
  const navigate = useNavigate();

  useEffect(() => {
    const onVersion: VersionListener = (data) => {
      if (data) setVersion(data);
    };
    versionListeners.add(onVersion);
    pollerSubscribers += 1;
    ensureVersionPoller();
    void loadVersion().then((data) => {
      if (data) setVersion(data);
    });

    return () => {
      versionListeners.delete(onVersion);
      pollerSubscribers = Math.max(0, pollerSubscribers - 1);
      releaseVersionPoller();
    };
  }, []);

  const handleUpgrade = () => {
    // Confirm + offline wait UI lives on Terminal — never fire upgrade from the rail
    navigate("/terminal?confirm=upgrade");
  };

  const currentVersion = version?.current || __APP_VERSION__;
  const updateAvailable = Boolean(version?.updateAvailable);
  const checkFailed = Boolean(version && version.githubOk === false && !updateAvailable);

  const focusRing =
    "focus-visible:outline-2 focus-visible:-outline-offset-2 focus-visible:outline-ring";

  if (collapsed) {
    return (
      <div className="flex flex-col items-center gap-1">
        {updateAvailable && (
          <button
            type="button"
            onClick={handleUpgrade}
            title={`Upgrade to v${version!.latest}`}
            className={cn(
              "relative flex size-8 items-center justify-center rounded-md text-subtle transition-colors duration-150 hover:bg-sidebar-accent hover:text-sidebar-foreground pointer-coarse:size-10",
              focusRing,
            )}
          >
            <ArrowUp className="size-4" />
            <span aria-hidden="true" className="absolute right-1.5 top-1.5 size-1.5 rounded-full bg-info" />
          </button>
        )}
        <p
          className="max-w-full truncate font-mono text-xs text-sidebar-muted"
          title={`v${currentVersion}`}
        >
          v{currentVersion}
        </p>
      </div>
    );
  }

  return (
    <div className="space-y-0.5">
      {updateAvailable && (
        <button
          type="button"
          onClick={handleUpgrade}
          title={`Upgrade from v${version!.current} to v${version!.latest}`}
          className={cn(
            "flex h-8 w-full items-center gap-2 rounded-md px-2 text-left text-[13px] font-medium text-sidebar-foreground transition-colors duration-150 hover:bg-sidebar-accent pointer-coarse:h-10",
            focusRing,
          )}
        >
          <span aria-hidden="true" className="size-2 shrink-0 rounded-full bg-info" />
          <span className="min-w-0 flex-1 truncate">Update available</span>
          <span className="shrink-0 font-mono text-xs text-sidebar-muted">
            v{version!.latest}
          </span>
          <span className="sr-only">Upgrade now</span>
        </button>
      )}

      <p className="flex h-6 items-center gap-2 px-2 font-mono text-xs text-sidebar-muted">
        <span className="truncate">v{currentVersion}</span>
      </p>
      {checkFailed && (
        <p className="flex items-center gap-2 px-2 pb-0.5 text-xs text-sidebar-muted">
          <span aria-hidden="true" className="size-1.5 shrink-0 rounded-full bg-warn" />
          Update check unavailable
        </p>
      )}
    </div>
  );
}
