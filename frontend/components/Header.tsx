// Header component - navigation bar with logo, nav links, theme toggle, GitHub stars
"use client";

import { useState, useEffect } from "react";
import { Container, Settings, Moon, Sun, Monitor, BookOpen, Anchor, LayoutGrid, ChevronDown, Gauge, Menu, X, LogOut, Activity } from "lucide-react";
import { GithubIcon } from "./icons/GithubIcon";
import Link from "next/link";
import { usePathname, useRouter } from "next/navigation";
import { useTheme } from "@/lib/theme";
import { Button } from "./ui/button";
import { cn } from "@/lib/utils";
import { useAuth } from "./AuthProvider";

// Global cache for star count to prevent flickering during theme changes or rerenders
let cachedStars: number | null = null;

// Format star count: 2000 → 2k, 2669 → 2.6k, 12500 → 12.5k
function formatStars(count: number | null): string {
  if (count === null) return "...";
  if (count < 1000) return count.toString();
  const k = count / 1000;
  return k % 1 === 0 ? `${k}k` : `${k.toFixed(1)}k`;
}

export function Header() {
  const { theme, setTheme, resolvedTheme } = useTheme();
  const { isAuthenticated, logout } = useAuth();
  const isDark = resolvedTheme === "dark";
  const [stars, setStars] = useState<number | null>(cachedStars);
  const [mobileMenuOpen, setMobileMenuOpen] = useState(false);
  const pathname = usePathname();

  useEffect(() => {
    // Only fetch if not already cached to prevent unnecessary requests and flickering
    if (cachedStars === null) {
      fetch("https://api.github.com/repos/SSujitX/docklift")
        .then(res => res.json())
        .then(data => {
          setStars(data.stargazers_count);
          cachedStars = data.stargazers_count;
        })
        .catch(() => setStars(null));
    }
  }, []);

  // Close mobile menu on route change
  useEffect(() => {
    setMobileMenuOpen(false);
  }, [pathname]);

  const toggleTheme = () => {
    setTheme(theme === "dark" ? "light" : "dark");
  };

  const navItems = [
    { name: "Projects", href: "/", icon: LayoutGrid },
    { name: "Ports", href: "/ports", icon: Anchor },
    { name: "System", href: "/system", icon: Gauge },
    { name: "Terminal", href: "/terminal", icon: Monitor },
    { name: "Logs", href: "/logs", icon: Activity },
  ];

  return (
    <>
      <header className="sticky top-0 z-50 w-full border-b border-border/40 bg-background/60 backdrop-blur-2xl">
        <div className="container flex h-14 md:h-16 items-center justify-between px-3 md:px-6 max-w-7xl mx-auto">
          {/* Left: Menu + Logo */}
          <div className="flex items-center gap-2 md:gap-4 lg:gap-8">
            {/* Mobile Menu Button */}
            <Button
              variant="ghost"
              size="icon"
              className="lg:hidden h-10 w-10 rounded-xl bg-secondary/50 border border-border/40"
              onClick={() => setMobileMenuOpen(!mobileMenuOpen)}
            >
              {mobileMenuOpen ? <X className="h-5 w-5" /> : <Menu className="h-5 w-5" />}
            </Button>

            <Link href="/" className="flex items-center gap-2 group">
              <div className="relative">
                <div className="absolute inset-0 bg-gradient-to-br from-cyan-500 to-blue-600 rounded-xl blur-lg opacity-40 group-hover:opacity-70 transition-all duration-500 group-hover:scale-110" />
                <div className="relative bg-gradient-to-br from-cyan-500 to-blue-600 p-2 md:p-2.5 rounded-xl shadow-lg shadow-cyan-500/20 group-hover:rotate-6 transition-transform duration-300">
                  <Container className="h-4 w-4 md:h-5 md:w-5 text-white" strokeWidth={2.5} />
                </div>
              </div>
              <div className="flex flex-col">
                <span className="text-lg md:text-xl font-black tracking-tight bg-clip-text text-transparent bg-gradient-to-r from-foreground to-foreground/70">Docklift</span>
                <span className="text-[8px] md:text-[9px] font-bold text-cyan-500 tracking-[0.2em] uppercase -mt-0.5 ml-0.5 hidden md:block">Automated</span>
              </div>
            </Link>

            {/* Desktop Nav */}
            <nav className="hidden lg:flex items-center gap-2">
              {navItems.map((item) => {
                const isActive = pathname === item.href || (item.href !== "/" && pathname.startsWith(item.href));
                return (
                  <Link key={item.name} href={item.href}>
                    <div className={cn(
                      "relative px-4 py-2 rounded-xl text-sm font-bold transition-all duration-300 flex items-center gap-2 group",
                      isActive 
                        ? "text-foreground bg-secondary/80 shadow-inner" 
                        : "text-muted-foreground hover:text-foreground hover:bg-secondary/40"
                    )}>
                      <item.icon className={cn(
                        "h-4 w-4 transition-transform group-hover:scale-110",
                        isActive ? "text-cyan-500" : "text-muted-foreground/60 group-hover:text-cyan-500/80"
                      )} />
                      {item.name}
                      {isActive && (
                        <div className="absolute -bottom-[17px] left-1/2 -translate-x-1/2 w-1/2 h-1 bg-cyan-500 rounded-t-full shadow-[0_-2px_10px_rgba(6,182,212,0.5)]" />
                      )}
                    </div>
                  </Link>
                );
              })}
            </nav>
          </div>
          
          {/* Right: Actions */}
          <div className="flex items-center gap-2 md:gap-3">
            {/* GitHub Star Button - Desktop only */}
            <a
              href="https://github.com/SSujitX/docklift"
              target="_blank"
              rel="noopener noreferrer"
              className="hidden md:block"
            >
              <div className="flex items-center group cursor-pointer h-9 active:scale-95 transition-transform duration-200">
                <div className={cn(
                  "flex items-center gap-0 rounded-full overflow-hidden transition-all",
                  isDark
                    ? "bg-black border border-zinc-700 shadow-lg shadow-black/50 hover:border-zinc-600"
                    : "bg-white border border-zinc-200 shadow-sm hover:shadow-md hover:border-zinc-300"
                )}>
                  <div className={cn(
                    "flex items-center justify-center h-9 w-9 border-r",
                    isDark ? "bg-zinc-900 border-zinc-700" : "bg-zinc-100 border-zinc-200"
                  )}>
                    <GithubIcon className={cn("h-4 w-4 group-hover:scale-110 transition-transform", isDark ? "text-white" : "text-zinc-900")} />
                  </div>
                  <div className="flex items-center gap-1.5 px-3 h-full">
                    <span className={cn("text-[10px] font-bold uppercase tracking-wider transition-colors", isDark ? "text-zinc-400" : "text-zinc-500")}>Star</span>
                    <div className={cn("flex items-center gap-1 rounded-full px-2 py-0.5", isDark ? "bg-zinc-800" : "bg-zinc-100")}>
                      <span className={cn("text-xs", isDark ? "text-yellow-400" : "text-yellow-500")}>&#9733;</span>
                      <span className={cn("text-xs font-bold", isDark ? "text-zinc-100" : "text-zinc-900")}>
                        {formatStars(stars)}
                      </span>
                    </div>
                  </div>
                </div>
              </div>
            </a>

            {/* Desktop Action Bar */}
            <div className="hidden md:flex items-center bg-secondary/50 dark:bg-zinc-900 border border-border/40 dark:border-white/10 p-1 rounded-full shadow-sm">
              <Link href="/docs">
                <Button variant="ghost" size="icon" className="text-muted-foreground hover:text-foreground hover:bg-background/60 dark:hover:bg-white/5 h-9 w-9 rounded-full transition-all active:scale-95" title="Documentation">
                  <BookOpen className="h-[18px] w-[18px]" />
                </Button>
              </Link>

              <div className="w-px h-4 bg-border/40 dark:bg-white/10 mx-0.5" />

              <Link href="/settings">
                <Button variant="ghost" size="icon" className="text-muted-foreground hover:text-foreground hover:bg-background/60 dark:hover:bg-white/5 h-9 w-9 rounded-full transition-all active:scale-95" title="Settings">
                  <Settings className="h-[18px] w-[18px]" />
                </Button>
              </Link>
              
              <div className="w-px h-4 bg-border/40 dark:bg-white/10 mx-0.5" />
              
              <Button 
                variant="ghost" 
                size="icon" 
                onClick={toggleTheme}
                className="text-muted-foreground hover:text-foreground hover:bg-background/60 dark:hover:bg-white/5 h-9 w-9 rounded-full transition-all active:scale-95"
                title={theme === "dark" ? "Switch to Light Mode" : "Switch to Dark Mode"}
              >
                {theme === "dark" ? (
                  <Moon className="h-[18px] w-[18px] text-cyan-400 transition-transform duration-500 hover:-rotate-12" />
                ) : (
                  <Sun className="h-[18px] w-[18px] text-amber-500 transition-transform duration-500 hover:rotate-45" />
                )}
              </Button>

              {isAuthenticated && (
                <>
                  <div className="w-px h-4 bg-border/40 dark:bg-white/10 mx-0.5" />
                  <div className="pl-0.5 pr-0.5">
                    <ProfileMenu />
                  </div>
                </>
              )}
            </div>

            {/* Mobile: Theme + Profile only */}
            <div className="flex md:hidden items-center gap-1.5">
              <Button 
                variant="ghost" 
                size="icon" 
                onClick={toggleTheme}
                className="h-10 w-10 rounded-xl bg-secondary/50 border border-border/40"
              >
                {theme === "dark" ? (
                  <Moon className="h-5 w-5 text-cyan-400" />
                ) : (
                  <Sun className="h-5 w-5 text-amber-500" />
                )}
              </Button>

              {isAuthenticated && <ProfileMenu />}
            </div>
          </div>
        </div>
      </header>

      {/* Mobile Navigation Overlay - Compact */}
      {mobileMenuOpen && (
        <div className="lg:hidden fixed inset-0 z-40 bg-background/98 backdrop-blur-2xl animate-in fade-in duration-200">
          <div className="flex flex-col h-full pt-16 pb-4 px-4">
            {/* Version Badge - Compact */}
            <div className="flex items-center gap-2 mb-4 px-1">
              <span className="text-[10px] font-bold text-muted-foreground uppercase tracking-wider">Version</span>
              <span className="text-[10px] font-bold text-cyan-500 bg-cyan-500/10 px-1.5 py-0.5 rounded border border-cyan-500/20">v{process.env.NEXT_PUBLIC_APP_VERSION || '1.3.3'}</span>
            </div>

            {/* Main Navigation - Compact */}
            <nav className="flex flex-col gap-1">
              {navItems.map((item) => {
                const isActive = pathname === item.href || (item.href !== "/" && pathname.startsWith(item.href));
                return (
                  <Link key={item.name} href={item.href} onClick={() => setMobileMenuOpen(false)}>
                    <div className={cn(
                      "flex items-center gap-3 px-3 py-2.5 rounded-xl text-base font-bold transition-all duration-200 active:scale-[0.98]",
                      isActive 
                        ? "bg-gradient-to-r from-cyan-500/15 to-blue-500/10 text-cyan-500 border border-cyan-500/20" 
                        : "text-foreground hover:bg-secondary/50"
                    )}>
                      <div className={cn(
                        "flex items-center justify-center h-9 w-9 rounded-lg",
                        isActive ? "bg-cyan-500/20" : "bg-secondary/50"
                      )}>
                        <item.icon className={cn("h-4 w-4", isActive ? "text-cyan-500" : "text-muted-foreground")} strokeWidth={isActive ? 2.5 : 2} />
                      </div>
                      {item.name}
                    </div>
                  </Link>
                );
              })}

              {/* Divider */}
              <div className="h-px bg-border/50 my-2" />

              {/* Quick Links - Compact */}
              <Link href="/docs" onClick={() => setMobileMenuOpen(false)}>
                <div className="flex items-center gap-3 px-3 py-2.5 rounded-xl text-base font-bold text-foreground hover:bg-secondary/50 transition-all active:scale-[0.98]">
                  <div className="flex items-center justify-center h-9 w-9 rounded-lg bg-secondary/50">
                    <BookOpen className="h-4 w-4 text-muted-foreground" />
                  </div>
                  Documentation
                </div>
              </Link>

              <Link href="/settings" onClick={() => setMobileMenuOpen(false)}>
                <div className="flex items-center gap-3 px-3 py-2.5 rounded-xl text-base font-bold text-foreground hover:bg-secondary/50 transition-all active:scale-[0.98]">
                  <div className="flex items-center justify-center h-9 w-9 rounded-lg bg-secondary/50">
                    <Settings className="h-4 w-4 text-muted-foreground" />
                  </div>
                  Settings
                </div>
              </Link>

              {/* GitHub Link - Compact */}
              <a href="https://github.com/SSujitX/docklift" target="_blank" rel="noopener noreferrer" onClick={() => setMobileMenuOpen(false)}>
                <div className="flex items-center gap-3 px-3 py-2.5 rounded-xl text-base font-bold text-foreground hover:bg-secondary/50 transition-all active:scale-[0.98]">
                  <div className="flex items-center justify-center h-9 w-9 rounded-lg bg-black">
                    <GithubIcon className="h-4 w-4 text-white" />
                  </div>
                  <span className="flex-1">GitHub</span>
                  <span className="text-xs font-bold text-muted-foreground bg-secondary/50 px-2 py-0.5 rounded">{formatStars(stars)} ⭐</span>
                </div>
              </a>
            </nav>
            
            {/* Profile Section at Bottom - Compact */}
            <div className="mt-auto pt-4 border-t border-border/50">
              <ProfileMenu isMobile />
            </div>
          </div>
        </div>
      )}
    </>
  );
}

function ProfileMenu({ isMobile = false }: { isMobile?: boolean }) {
  const { user, logout } = useAuth();
  const { resolvedTheme } = useTheme();
  const isDark = resolvedTheme === "dark";
  const [isOpen, setIsOpen] = useState(false);
  const router = useRouter();

  useEffect(() => {
    const handleClickOutside = (event: MouseEvent) => {
      const target = event.target as HTMLElement;
      if (!target.closest('.profile-dropdown')) {
        setIsOpen(false);
      }
    };

    if (isOpen) {
      document.addEventListener('mousedown', handleClickOutside);
    }
    return () => document.removeEventListener('mousedown', handleClickOutside);
  }, [isOpen]);

  if (isMobile) {
    return (
      <div className="space-y-3">
        <div className="flex items-center gap-3 px-3 py-2">
          <div className="h-10 w-10 rounded-full bg-gradient-to-br from-zinc-800 to-zinc-950 flex items-center justify-center text-white font-bold text-sm shadow-md border border-white/10">
            {user?.name?.charAt(0).toUpperCase() || "U"}
          </div>
          <div className="flex flex-col min-w-0">
            <span className="text-sm font-bold text-foreground truncate">{user?.name}</span>
            <span className="text-xs text-muted-foreground truncate">{user?.email}</span>
          </div>
        </div>
        <div className="grid grid-cols-2 gap-2 px-1">
          <Button 
            variant="outline" 
            className="w-full justify-center gap-2 rounded-lg h-9 text-xs font-semibold"
            onClick={() => router.push("/settings?tab=profile")}
          >
            <Settings className="h-3.5 w-3.5" />
            Settings
          </Button>
          <Button 
            variant="outline" 
            onClick={logout}
            className="w-full justify-center gap-2 rounded-lg h-9 text-xs font-semibold text-red-500 hover:text-red-600 hover:bg-red-50 dark:hover:bg-red-950/20 border-red-200 dark:border-red-900/30"
          >
            <LogOut className="h-3.5 w-3.5" />
            Sign Out
          </Button>
        </div>
      </div>
    );
  }

  return (
    <div className="relative profile-dropdown flex items-center">
      <Button 
        variant="ghost" 
        onClick={() => setIsOpen(!isOpen)}
        className={cn(
          "h-9 pl-1 pr-3 rounded-full transition-all active:scale-95 flex items-center gap-2 border border-transparent",
          isOpen ? "bg-secondary text-foreground border-border/50" : "text-muted-foreground hover:text-foreground hover:bg-secondary/50"
        )}
      >
        <div className="h-7 w-7 rounded-full bg-gradient-to-br from-zinc-700 to-zinc-900 flex items-center justify-center text-white text-xs font-bold shadow-sm border border-white/10">
          {user?.name?.charAt(0).toUpperCase() || "U"}
        </div>
        <span className="hidden lg:inline text-sm font-medium">{user?.name?.split(' ')[0]}</span>
        <ChevronDown className={cn("h-3.5 w-3.5 transition-transform duration-300 opacity-50", isOpen ? "rotate-180" : "")} />
      </Button>

      {isOpen && (
        <div className={cn(
          "absolute right-0 top-full mt-2 w-56 rounded-xl p-1.5 animate-in fade-in zoom-in-95 duration-200 z-[100] origin-top-right shadow-xl",
          isDark
            ? "bg-black border border-zinc-700 shadow-black/50"
            : "bg-white border border-zinc-200 shadow-black/10"
        )}>
          <div className={cn("px-3 py-3 flex items-center gap-3 border-b mb-1", isDark ? "border-zinc-700" : "border-zinc-100")}>
            <div className="h-9 w-9 rounded-full bg-gradient-to-br from-zinc-700 to-zinc-900 flex items-center justify-center text-white text-sm font-bold shadow-sm border border-white/10">
              {user?.name?.charAt(0).toUpperCase() || "U"}
            </div>
            <div className="flex flex-col min-w-0">
              <span className={cn("text-sm font-semibold truncate", isDark ? "text-zinc-100" : "text-zinc-900")}>{user?.name}</span>
              <span className={cn("text-xs truncate", isDark ? "text-zinc-400" : "text-zinc-500")}>{user?.email}</span>
            </div>
          </div>

          <div className="space-y-0.5">
            <Link
              href="/settings"
              onClick={() => setIsOpen(false)}
              className={cn(
                "flex items-center gap-2.5 px-3 py-2 rounded-lg text-sm font-medium transition-colors cursor-pointer",
                isDark ? "text-zinc-300 hover:bg-zinc-800" : "text-zinc-700 hover:bg-zinc-100"
              )}
            >
              <Settings className="h-4 w-4" />
              <span>Settings</span>
            </Link>

            <div
              onClick={() => { logout(); setIsOpen(false); }}
              className={cn(
                "flex items-center gap-2.5 px-3 py-2 rounded-lg text-sm font-medium transition-colors cursor-pointer",
                isDark ? "text-red-500 hover:bg-red-900/30" : "text-red-600 hover:bg-red-50"
              )}
            >
              <LogOut className="h-4 w-4" />
              <span>Sign Out</span>
            </div>
          </div>
        </div>
      )}
    </div>
  );
}
