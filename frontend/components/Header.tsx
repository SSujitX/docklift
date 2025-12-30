// Header component - navigation bar with logo, nav links, theme toggle, GitHub stars
"use client";

import { useState, useEffect } from "react";
import { Container, Settings, Moon, Sun, Monitor, BookOpen, Anchor, LayoutGrid, ChevronDown, Gauge, Menu, X } from "lucide-react";
import { GithubIcon } from "./icons/GithubIcon";
import Link from "next/link";
import { usePathname } from "next/navigation";
import { useTheme } from "@/lib/theme";
import { Button } from "./ui/button";
import { cn } from "@/lib/utils";

export function Header() {
  const { theme, setTheme } = useTheme();
  const [stars, setStars] = useState<number | null>(null);
  const [mobileMenuOpen, setMobileMenuOpen] = useState(false);
  const pathname = usePathname();

  useEffect(() => {
    fetch("https://api.github.com/repos/SSujitX/docklift")
      .then(res => res.json())
      .then(data => setStars(data.stargazers_count))
      .catch(() => setStars(null));
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
      <header className="sticky top-0 z-50 w-full border-b border-white/10 dark:border-white/5 bg-background/60 backdrop-blur-2xl transition-all duration-300">
        <div className="container flex h-16 items-center justify-between px-4 md:px-6 max-w-7xl mx-auto">
          <div className="flex items-center gap-4 lg:gap-8">
            {/* Mobile Menu Button */}
            <Button
              variant="ghost"
              size="icon"
              className="lg:hidden h-9 w-9 rounded-lg"
              onClick={() => setMobileMenuOpen(!mobileMenuOpen)}
            >
              {mobileMenuOpen ? <X className="h-5 w-5" /> : <Menu className="h-5 w-5" />}
            </Button>

            <Link href="/" className="flex items-center gap-2 sm:gap-3 group px-1">
              <div className="relative">
                <div className="absolute inset-0 bg-gradient-to-br from-cyan-500 to-blue-600 rounded-xl blur-lg opacity-40 group-hover:opacity-70 transition-all duration-500 group-hover:scale-110" />
                <div className="relative bg-gradient-to-br from-cyan-500 to-blue-600 p-2 sm:p-2.5 rounded-xl shadow-lg shadow-cyan-500/20 group-hover:rotate-6 transition-transform duration-300">
                  <Container className="h-4 w-4 sm:h-5 sm:w-5 text-white" strokeWidth={2.5} />
                </div>
              </div>
              <div className="flex flex-col">
                <span className="text-lg sm:text-xl font-black tracking-tight bg-clip-text text-transparent bg-gradient-to-r from-foreground to-foreground/70">Docklift</span>
                <span className="text-[8px] sm:text-[9px] font-bold text-cyan-500 tracking-[0.2em] uppercase -mt-1 ml-0.5 hidden sm:block">Automated</span>
              </div>
            </Link>

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
          
          <nav className="flex items-center gap-1 sm:gap-2">
            {/* GitHub Star Button - Hidden on mobile */}
            <a 
              href="https://github.com/SSujitX/docklift" 
              target="_blank" 
              rel="noopener noreferrer"
              className="mr-1 sm:mr-2 hidden sm:block"
            >
              <div className="flex items-center gap-2 bg-secondary/50 hover:bg-secondary border border-border/40 rounded-xl px-3 sm:px-4 py-1.5 transition-all duration-300 group cursor-pointer hover:border-cyan-500/30">
                <GithubIcon className="h-4 w-4 text-muted-foreground group-hover:text-foreground transition-colors" />
                <div className="flex flex-col items-start leading-none gap-0.5">
                  <span className="text-[9px] font-bold text-muted-foreground uppercase tracking-widest group-hover:text-cyan-500 transition-colors">Star on</span>
                  <span className="text-[11px] font-black">GitHub</span>
                </div>
                {stars !== null && (
                  <div className="ml-2 flex items-center gap-1 bg-background/50 px-1.5 py-0.5 rounded-lg border border-border/20 text-[10px] font-bold">
                    <span className="text-amber-500">★</span>
                    <span>{stars}</span>
                  </div>
                )}
              </div>
            </a>

            <div className="flex items-center bg-secondary/30 p-1 rounded-xl border border-border/40">
              {/* GitHub Star - Mobile only compact version */}
              <a 
                href="https://github.com/SSujitX/docklift" 
                target="_blank" 
                rel="noopener noreferrer"
                className="sm:hidden"
              >
                <div className="flex items-center gap-1 text-muted-foreground hover:text-foreground hover:bg-background/80 h-8 px-2 rounded-lg transition-all active:scale-90">
                  {stars !== null && (
                    <span className="text-[10px] font-bold text-amber-500">★{stars}</span>
                  )}
                  <GithubIcon className="h-4 w-4" />
                </div>
              </a>

              <div className="w-px h-4 bg-border/50 mx-0.5 sm:hidden" />

              <Link href="/docs">
                <Button variant="ghost" size="icon" className="text-muted-foreground hover:text-foreground hover:bg-background/80 h-8 w-8 sm:h-9 sm:w-9 rounded-lg transition-all active:scale-90" title="Documentation">
                  <BookOpen className="h-4 w-4 sm:h-[18px] sm:w-[18px]" />
                </Button>
              </Link>

              <div className="w-px h-4 bg-border/50 mx-0.5 sm:mx-1" />

              <Link href="/settings">
                <Button variant="ghost" size="icon" className="text-muted-foreground hover:text-foreground hover:bg-background/80 h-8 w-8 sm:h-9 sm:w-9 rounded-lg transition-all active:scale-90" title="Settings">
                  <Settings className="h-4 w-4 sm:h-[18px] sm:w-[18px]" />
                </Button>
              </Link>
              
              <div className="w-px h-4 bg-border/50 mx-0.5 sm:mx-1" />
              
              <Button 
                variant="ghost" 
                size="icon" 
                onClick={cycleTheme}
                className="text-muted-foreground hover:text-foreground hover:bg-background/80 h-8 w-8 sm:h-9 sm:w-9 rounded-lg transition-all active:scale-90"
              >
                {theme === "light" && <Sun className="h-4 w-4 sm:h-[18px] sm:w-[18px] text-amber-500" />}
                {theme === "dark" && <Moon className="h-4 w-4 sm:h-[18px] sm:w-[18px] text-cyan-400" />}
                {theme === "system" && <Monitor className="h-4 w-4 sm:h-[18px] sm:w-[18px]" />}
              </Button>
            </div>
          </nav>
        </div>
      </header>

      {/* Mobile Navigation Overlay */}
      {mobileMenuOpen && (
        <div className="lg:hidden fixed inset-0 z-40 bg-background/95 backdrop-blur-xl">
          <div className="flex flex-col h-full pt-20 px-6">
            <nav className="flex flex-col gap-2">
              {navItems.map((item) => {
                const isActive = pathname === item.href || (item.href !== "/" && pathname.startsWith(item.href));
                return (
                  <Link key={item.name} href={item.href} onClick={() => setMobileMenuOpen(false)}>
                    <div className={cn(
                      "flex items-center gap-4 px-4 py-4 rounded-2xl text-lg font-bold transition-all duration-200",
                      isActive 
                        ? "bg-cyan-500/10 text-cyan-500 border border-cyan-500/20" 
                        : "text-foreground hover:bg-secondary/50"
                    )}>
                      <item.icon className={cn("h-6 w-6", isActive ? "text-cyan-500" : "text-muted-foreground")} />
                      {item.name}
                    </div>
                  </Link>
                );
              })}
            </nav>


          </div>
        </div>
      )}
    </>
  );
}
