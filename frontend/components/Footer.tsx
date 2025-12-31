// Footer component - displays copyright, version check, and social links
"use client";

import { useState, useEffect } from "react";
import { useRouter } from "next/navigation";
import { ArrowUp, Loader2 } from "lucide-react";
import { GithubIcon } from "./icons/GithubIcon";
import { Button } from "./ui/button";
import { getAuthHeaders } from "@/lib/auth";

interface VersionInfo {
  current: string;
  latest: string;
  updateAvailable: boolean;
}

export function Footer() {
  const [versionInfo, setVersionInfo] = useState<VersionInfo | null>(null);
  const [upgrading, setUpgrading] = useState(false);

  const router = useRouter();

  useEffect(() => {
    // Check for updates on mount
    fetch('/api/system/version', { headers: getAuthHeaders() })
      .then(res => res.json())
      .then(data => setVersionInfo(data))
      .catch(() => {});
  }, []);

  const handleUpgrade = async () => {
    setUpgrading(true);
    try {
      await fetch('/api/system/upgrade', { 
        method: 'POST',
        headers: getAuthHeaders()
      });
      // Redirect to terminal to see the upgrade progress
      router.push('/terminal?action=upgrade');
    } catch {
      setUpgrading(false);
    }
  };

  return (
    <footer className="border-t border-border/40 bg-background/50 mt-auto">
      {/* Update Available Banner */}
      {versionInfo?.updateAvailable && (
        <div className="bg-gradient-to-r from-cyan-500/10 to-blue-500/10 border-b border-cyan-500/20">
          <div className="container max-w-7xl mx-auto px-4 py-2 md:py-3">
            <div className="flex flex-col sm:flex-row items-center justify-center gap-2 sm:gap-4">
              <div className="flex items-center gap-2 text-xs sm:text-sm font-bold text-cyan-600 dark:text-cyan-400">
                <ArrowUp className="h-4 w-4 animate-bounce" />
                <span>New version available!</span>
                <span className="text-muted-foreground">v{versionInfo.current} → v{versionInfo.latest}</span>
              </div>
              <Button 
                size="sm" 
                onClick={handleUpgrade}
                disabled={upgrading}
                className="h-7 text-xs px-3 bg-cyan-500 hover:bg-cyan-600 text-white"
              >
                {upgrading ? (
                  <>
                    <Loader2 className="h-3 w-3 mr-1 animate-spin" />
                    Upgrading...
                  </>
                ) : (
                  'Upgrade Now'
                )}
              </Button>
            </div>
          </div>
        </div>
      )}

      {/* Main Footer - Single row on desktop */}
      <div className="container max-w-7xl mx-auto px-4 md:px-6 py-3">
        <div className="flex flex-col sm:flex-row items-center justify-between gap-2 text-xs sm:text-sm text-muted-foreground">
          {/* Left: Copyright + Version */}
          <div className="flex items-center gap-2">
            <span>Copyright © {new Date().getFullYear()} Docklift.</span>
            <span className="font-bold text-cyan-500 bg-cyan-500/10 px-1.5 py-0.5 rounded text-[10px] border border-cyan-500/20">
              v{versionInfo?.current || process.env.NEXT_PUBLIC_APP_VERSION || '1.3.3'}
            </span>
          </div>

          {/* Right: Social Icons */}
          <div className="flex items-center gap-3">
            <a 
              href="https://github.com/SSujitX/docklift" 
              target="_blank" 
              rel="noopener noreferrer"
              className="text-muted-foreground hover:text-foreground transition-colors p-1.5 rounded-lg hover:bg-secondary/50"
              title="GitHub"
            >
              <GithubIcon className="h-4 w-4" />
            </a>
            {/* Add more social icons here later */}
          </div>
        </div>
      </div>
    </footer>
  );
}

