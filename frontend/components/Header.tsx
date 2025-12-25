"use client";

import { Container, Settings, Moon, Sun, Monitor, BookOpen, Github } from "lucide-react";
import Link from "next/link";
import { useTheme } from "@/lib/theme";
import { Button } from "./ui/button";

export function Header() {
  const { theme, setTheme } = useTheme();

  const cycleTheme = () => {
    if (theme === "light") setTheme("dark");
    else if (theme === "dark") setTheme("system");
    else setTheme("light");
  };

  return (
    <header className="sticky top-0 z-50 w-full border-b border-border/40 bg-background/80 backdrop-blur-xl">
      <div className="container flex h-14 items-center justify-between px-4 md:px-6 max-w-7xl mx-auto">
        <div className="flex items-center gap-6">
          <Link href="/" className="flex items-center gap-2.5 group">
            <div className="relative">
              <div className="absolute inset-0 bg-gradient-to-br from-cyan-500 to-blue-600 rounded-lg blur-md opacity-60 group-hover:opacity-80 transition-opacity" />
              <div className="relative bg-gradient-to-br from-cyan-500 to-blue-600 p-2 rounded-lg">
                <Container className="h-4 w-4 text-white" strokeWidth={2.5} />
              </div>
            </div>
            <span className="text-lg font-bold tracking-tight">Docklift</span>
          </Link>

          <nav className="hidden md:flex items-center gap-1">
            <Link href="/docs">
              <Button variant="ghost" size="sm" className="text-muted-foreground hover:text-foreground text-sm">
                <BookOpen className="h-4 w-4 mr-1.5" />
                Docs
              </Button>
            </Link>
            <Link href="/ports">
              <Button variant="ghost" size="sm" className="text-muted-foreground hover:text-foreground text-sm">
                Ports
              </Button>
            </Link>
          </nav>
        </div>
        
        <nav className="flex items-center gap-1">
          <a href="https://github.com/yourusername/docklift" target="_blank" rel="noopener noreferrer">
            <Button variant="ghost" size="icon" className="text-muted-foreground hover:text-foreground h-8 w-8">
              <Github className="h-4 w-4" />
            </Button>
          </a>
          <Link href="/settings">
            <Button variant="ghost" size="icon" className="text-muted-foreground hover:text-foreground h-8 w-8">
              <Settings className="h-4 w-4" />
            </Button>
          </Link>
          <div className="w-px h-5 bg-border mx-1" />
          <Button 
            variant="ghost" 
            size="icon" 
            onClick={cycleTheme}
            className="text-muted-foreground hover:text-foreground h-8 w-8"
          >
            {theme === "light" && <Sun className="h-4 w-4" />}
            {theme === "dark" && <Moon className="h-4 w-4" />}
            {theme === "system" && <Monitor className="h-4 w-4" />}
          </Button>
        </nav>
      </div>
    </header>
  );
}
