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

  if (collapsed) {
    return (
      <div className="flex flex-col items-center gap-2">
        {updateAvailable && (
          <button
            type="button"
            onClick={handleUpgrade}
            title={`Upgrade to v${version!.latest}`}
            className="flex h-9 w-9 items-center justify-center rounded-xl bg-brand/15 text-brand transition-colors hover:bg-brand/25"
          >
            <ArrowUp className="h-4 w-4" />
          </button>
        )}
        <p className="text-[9px] font-semibold tracking-wide text-sidebar-muted/70">
          v{currentVersion}
        </p>
      </div>
    );
  }

  return (
    <div className="space-y-2">
      {updateAvailable && (
        <div className="rounded-2xl border border-brand/25 bg-brand/10 p-3">
          <div className="flex items-center gap-2 text-xs font-semibold text-brand">
            <ArrowUp className="h-3.5 w-3.5" />
            Update available
          </div>
          <p className="mt-1 text-[11px] text-sidebar-muted">
            v{version!.current} → v{version!.latest}
          </p>
          <button
            type="button"
            onClick={handleUpgrade}
            className={cn(
              "mt-2.5 flex h-8 w-full items-center justify-center gap-1.5 rounded-lg text-xs font-semibold transition-colors",
              "bg-brand text-brand-foreground hover:brightness-110",
            )}
          >
            Upgrade now
          </button>
        </div>
      )}

      <p className="px-2.5 text-[10px] font-medium tracking-wide text-sidebar-muted/70">
        v{currentVersion}
      </p>
      {checkFailed && (
        <p className="px-2.5 text-[10px] text-amber-600/90 dark:text-amber-400/90">
          Update check unavailable
        </p>
      )}
    </div>
  );
}
