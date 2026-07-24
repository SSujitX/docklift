// Release state for the rail footer: version, upgrade prompt and GitHub stars.
// Replaces what the old page footer and header used to show.

import { useEffect, useState } from "react";
import { useNavigate } from "react-router-dom";
import { ArrowUp, Loader2, Star } from "lucide-react";
import { GithubIcon } from "@/components/icons/GithubIcon";
import { getAuthHeaders } from "@/lib/auth";
import { cn } from "@/lib/utils";

interface VersionInfo {
  current: string;
  latest: string;
  updateAvailable: boolean;
}

// Cached across mounts so collapsing the rail — or the desktop and drawer rails
// mounting together — never repeats the requests.
let cachedStars: number | null = null;
let cachedVersion: VersionInfo | null = null;
let versionRequest: Promise<VersionInfo | null> | null = null;
let starsRequest: Promise<number | null> | null = null;

function loadVersion(): Promise<VersionInfo | null> {
  versionRequest ??= fetch("/api/system/version", { headers: getAuthHeaders() })
    .then((res) => (res.ok ? res.json() : null))
    .then((data: VersionInfo | null) => {
      cachedVersion = data;
      return data;
    })
    .catch(() => null);
  return versionRequest;
}

function loadStars(): Promise<number | null> {
  starsRequest ??= fetch("https://api.github.com/repos/SSujitX/docklift")
    .then((res) => (res.ok ? res.json() : null))
    .then((data) => {
      cachedStars = data?.stargazers_count ?? null;
      return cachedStars;
    })
    .catch(() => null);
  return starsRequest;
}

function formatStars(count: number | null): string {
  if (count === null) return "—";
  if (count < 1000) return String(count);
  const thousands = count / 1000;
  return thousands % 1 === 0 ? `${thousands}k` : `${thousands.toFixed(1)}k`;
}

export function SidebarStatus({ collapsed }: { collapsed: boolean }) {
  const [version, setVersion] = useState<VersionInfo | null>(cachedVersion);
  const [stars, setStars] = useState<number | null>(cachedStars);
  const [upgrading, setUpgrading] = useState(false);
  const navigate = useNavigate();

  useEffect(() => {
    let active = true;
    if (!cachedVersion) {
      loadVersion().then((data) => {
        if (active && data) setVersion(data);
      });
    }
    if (cachedStars === null) {
      loadStars().then((count) => {
        if (active) setStars(count);
      });
    }
    return () => {
      active = false;
    };
  }, []);

  const handleUpgrade = async () => {
    setUpgrading(true);
    try {
      const res = await fetch("/api/system/upgrade", {
        method: "POST",
        headers: getAuthHeaders(),
      });
      const data = await res.json();
      const action = data.message?.includes("Simulated")
        ? "upgrade_simulated"
        : "upgrade";
      navigate(`/terminal?action=${action}`);
    } catch {
      setUpgrading(false);
    }
  };

  const currentVersion = version?.current || __APP_VERSION__;

  if (collapsed) {
    return (
      <div className="flex flex-col items-center gap-2">
        {version?.updateAvailable && (
          <button
            type="button"
            onClick={handleUpgrade}
            disabled={upgrading}
            title={`Upgrade to v${version.latest}`}
            className="flex h-9 w-9 items-center justify-center rounded-xl bg-brand/15 text-brand transition-colors hover:bg-brand/25"
          >
            {upgrading ? (
              <Loader2 className="h-4 w-4 animate-spin" />
            ) : (
              <ArrowUp className="h-4 w-4" />
            )}
          </button>
        )}
        <a
          href="https://github.com/SSujitX/docklift"
          target="_blank"
          rel="noopener noreferrer"
          title={`Star on GitHub — ${formatStars(stars)}`}
          className="flex h-9 w-9 items-center justify-center rounded-xl text-sidebar-muted transition-colors hover:bg-sidebar-accent hover:text-sidebar-foreground"
        >
          <GithubIcon className="h-4 w-4" />
        </a>
      </div>
    );
  }

  return (
    <div className="space-y-2">
      {version?.updateAvailable && (
        <div className="rounded-2xl border border-brand/25 bg-brand/10 p-3">
          <div className="flex items-center gap-2 text-xs font-semibold text-brand">
            <ArrowUp className="h-3.5 w-3.5" />
            Update available
          </div>
          <p className="mt-1 text-[11px] text-sidebar-muted">
            v{version.current} → v{version.latest}
          </p>
          <button
            type="button"
            onClick={handleUpgrade}
            disabled={upgrading}
            className={cn(
              "mt-2.5 flex h-8 w-full items-center justify-center gap-1.5 rounded-lg text-xs font-semibold transition-colors",
              "bg-brand text-brand-foreground hover:brightness-110 disabled:opacity-60",
            )}
          >
            {upgrading ? (
              <>
                <Loader2 className="h-3.5 w-3.5 animate-spin" />
                Upgrading
              </>
            ) : (
              "Upgrade now"
            )}
          </button>
        </div>
      )}

      <a
        href="https://github.com/SSujitX/docklift"
        target="_blank"
        rel="noopener noreferrer"
        className="flex items-center gap-2.5 rounded-xl px-2.5 py-2 text-sidebar-muted transition-colors hover:bg-sidebar-accent hover:text-sidebar-foreground"
      >
        <GithubIcon className="h-4 w-4 shrink-0" />
        <span className="flex-1 truncate text-xs font-medium">Star on GitHub</span>
        <span className="flex items-center gap-1 rounded-md bg-sidebar-accent px-1.5 py-0.5 text-[10px] font-semibold">
          <Star className="h-2.5 w-2.5 fill-amber-400 text-amber-400" />
          {formatStars(stars)}
        </span>
      </a>

      <p className="px-2.5 text-[10px] font-medium tracking-wide text-sidebar-muted/70">
        v{currentVersion} · © {new Date().getFullYear()} Docklift
      </p>
    </div>
  );
}
