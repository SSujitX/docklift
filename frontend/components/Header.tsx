// Header component - navigation bar with logo, nav links, theme toggle, GitHub stars
"use client";

import { useState, useEffect } from "react";
import { Container, Settings, Moon, Sun, Monitor, BookOpen, Anchor, LayoutGrid, ChevronDown, Gauge, Menu, X, LogOut } from "lucide-react";
import { GithubIcon } from "./icons/GithubIcon";
import Link from "next/link";
import { usePathname } from "next/navigation";
import { useTheme } from "@/lib/theme";
import { Button } from "./ui/button";
import { cn } from "@/lib/utils";
import { useAuth } from "./AuthProvider";

// Global cache for star count to prevent flickering during theme changes or rerenders
let cachedStars: number | null = null;

export function Header() {
  const { theme, setTheme } = useTheme();
  const { isAuthenticated, logout } = useAuth();
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

  const cycleTheme = () => {
    if (theme === "light") setTheme("dark");
    else if (theme === "dark") setTheme("system");
    else setTheme("light");
  };

  const navItems = [
    { name: "Projects", href: "/", icon: LayoutGrid },
    { name: "Ports", href: "/ports", icon: Anchor },
    { name: "System", href: "/system", icon: Gauge },
    { name: "Terminal", href: "/terminal", icon: Monitor },
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
                <div className="flex items-center gap-2.5 bg-secondary/40 hover:bg-secondary/60 border border-border/40 rounded-full pl-1.5 pr-4 h-full transition-all group-hover:border-cyan-500/30 group-hover:shadow-[0_0_15px_rgba(6,182,212,0.1)]">
                  <div className="flex items-center justify-center h-6 w-6 rounded-full bg-black shadow-sm ring-1 ring-white/10 group-hover:scale-110 transition-transform">
                    <GithubIcon className="h-3.5 w-3.5 text-white" />
                  </div>
                  <div className="flex items-center gap-2">
                    <span className="text-xs font-black tracking-tight text-muted-foreground group-hover:text-foreground transition-colors">Stars</span>
                    <span className="text-sm font-black text-foreground">
                      {stars !== null ? stars : "..."}
                    </span>
                  </div>
                </div>
              </div>
            </a>

            {/* Desktop Action Bar */}
            <div className="hidden md:flex items-center bg-secondary/30 p-1.5 rounded-2xl border border-border/40 backdrop-blur-md shadow-sm">
              <Link href="/docs">
                <Button variant="ghost" size="icon" className="text-muted-foreground hover:text-foreground hover:bg-background/80 h-9 w-9 rounded-xl transition-all active:scale-90" title="Documentation">
                  <BookOpen className="h-[18px] w-[18px]" />
                </Button>
              </Link>

              <div className="w-px h-5 bg-border/50 mx-1" />

              <Link href="/settings">
                <Button variant="ghost" size="icon" className="text-muted-foreground hover:text-foreground hover:bg-background/80 h-9 w-9 rounded-xl transition-all active:scale-90" title="Settings">
                  <Settings className="h-[18px] w-[18px]" />
                </Button>
              </Link>
              
              <div className="w-px h-5 bg-border/50 mx-1" />
              
              <Button 
                variant="ghost" 
                size="icon" 
                onClick={cycleTheme}
                className="text-muted-foreground hover:text-foreground hover:bg-background/80 h-9 w-9 rounded-xl transition-all active:scale-90"
              >
                {theme === "light" && <Sun className="h-[18px] w-[18px] text-amber-500 transition-transform duration-500 hover:rotate-45" />}
                {theme === "dark" && <Moon className="h-[18px] w-[18px] text-cyan-400 transition-transform duration-500 hover:-rotate-12" />}
                {theme === "system" && <Monitor className="h-[18px] w-[18px] transition-transform duration-300" />}
              </Button>

              {isAuthenticated && (
                <>
                  <div className="w-px h-5 bg-border/50 mx-1" />
                  <ProfileMenu />
                </>
              )}
            </div>

            {/* Mobile: Theme + Profile only */}
            <div className="flex md:hidden items-center gap-1.5">
              <Button 
                variant="ghost" 
                size="icon" 
                onClick={cycleTheme}
                className="h-10 w-10 rounded-xl bg-secondary/50 border border-border/40"
              >
                {theme === "light" && <Sun className="h-5 w-5 text-amber-500" />}
                {theme === "dark" && <Moon className="h-5 w-5 text-cyan-400" />}
                {theme === "system" && <Monitor className="h-5 w-5" />}
              </Button>

              {isAuthenticated && <ProfileMenu />}
            </div>
          </div>
        </div>
      </header>

      {/* Mobile Navigation Overlay - Redesigned */}
      {mobileMenuOpen && (
        <div className="lg:hidden fixed inset-0 z-40 bg-background/98 backdrop-blur-2xl animate-in fade-in duration-200">
          <div className="flex flex-col h-full pt-20 pb-8 px-5">
            {/* Version Badge */}
            <div className="flex items-center gap-2 mb-6 px-2">
              <span className="text-xs font-bold text-muted-foreground">VERSION</span>
              <span className="text-xs font-bold text-cyan-500 bg-cyan-500/10 px-2 py-1 rounded-lg border border-cyan-500/20">v{process.env.NEXT_PUBLIC_APP_VERSION || '1.3.3'}</span>
            </div>

            {/* Main Navigation */}
            <nav className="flex flex-col gap-2">
              {navItems.map((item) => {
                const isActive = pathname === item.href || (item.href !== "/" && pathname.startsWith(item.href));
                return (
                  <Link key={item.name} href={item.href} onClick={() => setMobileMenuOpen(false)}>
                    <div className={cn(
                      "flex items-center gap-4 px-5 py-4 rounded-2xl text-lg font-black transition-all duration-200 active:scale-[0.98]",
                      isActive 
                        ? "bg-gradient-to-r from-cyan-500/15 to-blue-500/10 text-cyan-500 border border-cyan-500/20" 
                        : "text-foreground hover:bg-secondary/50"
                    )}>
                      <div className={cn(
                        "flex items-center justify-center h-11 w-11 rounded-xl",
                        isActive ? "bg-cyan-500/20" : "bg-secondary/50"
                      )}>
                        <item.icon className={cn("h-5 w-5", isActive ? "text-cyan-500" : "text-muted-foreground")} strokeWidth={isActive ? 2.5 : 2} />
                      </div>
                      {item.name}
                    </div>
                  </Link>
                );
              })}

              {/* Divider */}
              <div className="h-px bg-border/50 my-4" />

              {/* Quick Links */}
              <Link href="/docs" onClick={() => setMobileMenuOpen(false)}>
                <div className="flex items-center gap-4 px-5 py-4 rounded-2xl text-lg font-black text-foreground hover:bg-secondary/50 transition-all active:scale-[0.98]">
                  <div className="flex items-center justify-center h-11 w-11 rounded-xl bg-secondary/50">
                    <BookOpen className="h-5 w-5 text-muted-foreground" />
                  </div>
                  Documentation
                </div>
              </Link>

              <Link href="/settings" onClick={() => setMobileMenuOpen(false)}>
                <div className="flex items-center gap-4 px-5 py-4 rounded-2xl text-lg font-black text-foreground hover:bg-secondary/50 transition-all active:scale-[0.98]">
                  <div className="flex items-center justify-center h-11 w-11 rounded-xl bg-secondary/50">
                    <Settings className="h-5 w-5 text-muted-foreground" />
                  </div>
                  Settings
                </div>
              </Link>

              {/* GitHub Link */}
              <a href="https://github.com/SSujitX/docklift" target="_blank" rel="noopener noreferrer" onClick={() => setMobileMenuOpen(false)}>
                <div className="flex items-center gap-4 px-5 py-4 rounded-2xl text-lg font-black text-foreground hover:bg-secondary/50 transition-all active:scale-[0.98]">
                  <div className="flex items-center justify-center h-11 w-11 rounded-xl bg-black">
                    <GithubIcon className="h-5 w-5 text-white" />
                  </div>
                  <span className="flex-1">GitHub</span>
                  <span className="text-sm font-bold text-muted-foreground bg-secondary/50 px-2.5 py-1 rounded-lg">{stars || "..."} ⭐</span>
                </div>
              </a>
            </nav>
            
            {/* Profile Section at Bottom */}
            <div className="mt-auto pt-6 border-t border-border/50">
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
  const [isOpen, setIsOpen] = useState(false);

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
      <div className="space-y-6">
        <div className="flex items-center gap-5 px-6 py-3">
          <div className="h-20 w-20 rounded-full bg-gradient-to-br from-cyan-500 via-cyan-400 to-blue-600 flex items-center justify-center text-white font-black text-3xl shadow-[0_12px_40px_rgb(6,182,212,0.4)] border-4 border-background ring-2 ring-cyan-500/20">
            {user?.name?.charAt(0).toUpperCase() || "U"}
          </div>
          <div className="flex flex-col gap-0.5">
            <span className="text-2xl font-black text-foreground tracking-tight leading-none mb-1">{user?.name}</span>
            <span className="text-sm font-bold text-muted-foreground uppercase tracking-widest opacity-60">{user?.email}</span>
          </div>
        </div>
        <div className="grid grid-cols-2 gap-4 px-2">
          <Link href="/settings?tab=profile" className="flex-1">
            <Button variant="secondary" className="w-full justify-center gap-3 rounded-[1.5rem] h-14 text-base font-black border border-border/50 shadow-sm active:scale-95 transition-all">
              <Settings className="h-5 w-5 opacity-70" />
              Settings
            </Button>
          </Link>
          <Button 
            variant="ghost" 
            onClick={logout}
            className="flex-1 justify-center gap-3 rounded-[1.5rem] h-14 text-base font-black text-red-500 hover:bg-red-500/10 active:scale-95 transition-all"
          >
            <LogOut className="h-5 w-5" />
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
          "h-10 px-2 rounded-xl transition-all active:scale-95 flex items-center gap-3 group",
          isOpen ? "bg-secondary/80 text-foreground ring-1 ring-border/50 shadow-inner" : "text-muted-foreground hover:text-foreground hover:bg-background/80"
        )}
      >
        <div className="h-8 w-8 rounded-full bg-gradient-to-br from-cyan-500 via-cyan-400 to-blue-600 flex items-center justify-center text-white text-base font-black shadow-[0_4px_12px_rgba(6,182,212,0.3)] group-hover:shadow-[0_4px_20px_rgba(6,182,212,0.5)] transition-all group-hover:scale-105 border-2 border-white/20 shrink-0">
          {user?.name?.charAt(0).toUpperCase() || "U"}
        </div>
        <span className="hidden lg:inline text-[15px] font-black tracking-tight leading-none pt-0.5">{user?.name?.split(' ')[0]}</span>
        <ChevronDown className={cn("h-4 w-4 transition-transform duration-500 opacity-60 group-hover:opacity-100", isOpen ? "rotate-180" : "")} />
      </Button>

      {isOpen && (
        <div className="absolute right-0 top-full mt-3 w-64 rounded-[2rem] bg-card/80 backdrop-blur-2xl border border-border/50 shadow-[0_20px_50px_rgba(0,0,0,0.3)] p-3 animate-in fade-in zoom-in-95 duration-300 z-[100] origin-top-right">
          <div className="px-4 py-4 mb-2 bg-secondary/30 rounded-[1.5rem] border border-border/20">
            <p className="text-base font-black text-foreground tracking-tight leading-none mb-1.5">{user?.name}</p>
            <p className="text-[10px] font-black text-muted-foreground/60 truncate uppercase tracking-[0.15em]">{user?.email}</p>
          </div>
          
          <div className="space-y-1">
            <Link href="/settings?tab=profile" onClick={() => setIsOpen(false)}>
              <div className="flex items-center gap-3.5 px-3 py-3 rounded-2xl text-sm font-black text-muted-foreground/80 hover:text-foreground hover:bg-secondary/80 transition-all cursor-pointer group">
                <div className="flex items-center justify-center h-10 w-10 rounded-xl bg-secondary/50 group-hover:bg-cyan-500/10 group-hover:text-cyan-500 transition-all group-hover:scale-110 group-hover:rotate-6 border border-border/20 group-hover:border-cyan-500/30">
                  <Settings className="h-5 w-5" />
                </div>
                <span>Profile Settings</span>
              </div>
            </Link>
            
            <div 
              onClick={() => { logout(); setIsOpen(false); }}
              className="flex items-center gap-3.5 px-3 py-3 rounded-2xl text-sm font-black text-red-500/80 hover:text-red-500 hover:bg-red-500/10 transition-all cursor-pointer group"
            >
              <div className="flex items-center justify-center h-10 w-10 rounded-xl bg-red-500/5 group-hover:bg-red-500/10 transition-all group-hover:scale-110 group-hover:-rotate-6 border border-red-500/10 group-hover:border-red-500/20">
                <LogOut className="h-5 w-5" />
              </div>
              <span>Sign Out</span>
            </div>
          </div>
        </div>
      )}
    </div>
  );
}
