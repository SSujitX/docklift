// Footer component - displays copyright, GitHub link, and credits
"use client";

import { Container, Heart } from "lucide-react";
import { GithubIcon } from "./icons/GithubIcon";

export function Footer() {
  return (
    <footer className="border-t border-border/40 bg-background/50 mt-auto">
      <div className="container max-w-7xl mx-auto px-4 md:px-6 py-6">
        <div className="flex flex-col md:flex-row items-center justify-between gap-4 text-sm text-muted-foreground">
          <div className="flex items-center gap-2">
            <p>© {new Date().getFullYear()} Docklift. Open source under MIT license.</p>
            <span className="text-[10px] font-bold text-cyan-500 bg-cyan-500/10 px-1.5 py-0.5 rounded border border-cyan-500/20">v{process.env.NEXT_PUBLIC_APP_VERSION || '1.3.3'}</span>
          </div>
          <div className="flex items-center gap-4">
            <a 
              href="https://github.com/SSujitX/docklift" 
              target="_blank" 
              rel="noopener noreferrer"
              className="hover:text-foreground transition-colors flex items-center gap-1.5"
            >
              <GithubIcon className="h-4 w-4" />
              GitHub
            </a>
            <span className="flex items-center gap-1">
              Made with <Heart className="h-3.5 w-3.5 text-red-500 fill-red-500" /> for developers
            </span>
          </div>
        </div>
      </div>
    </footer>
  );
}
