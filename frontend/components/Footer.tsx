"use client";

import { Container, Github, Heart } from "lucide-react";
import Link from "next/link";

export function Footer() {
  return (
    <footer className="border-t border-border/40 bg-background/50 mt-auto">
      <div className="container max-w-7xl mx-auto px-4 md:px-6 py-8">
        <div className="flex flex-col md:flex-row items-center justify-between gap-4">
          <div className="flex items-center gap-2">
            <div className="bg-gradient-to-br from-cyan-500 to-blue-600 p-1.5 rounded-md">
              <Container className="h-3.5 w-3.5 text-white" strokeWidth={2.5} />
            </div>
            <span className="text-sm font-semibold">Docklift</span>
            <span className="text-muted-foreground text-sm">• Self-hosted Docker deployments</span>
          </div>

          <nav className="flex items-center gap-6 text-sm">
            <Link href="/docs" className="text-muted-foreground hover:text-foreground transition-colors">
              Documentation
            </Link>
            <a 
              href="https://github.com/yourusername/docklift" 
              target="_blank" 
              rel="noopener noreferrer"
              className="text-muted-foreground hover:text-foreground transition-colors flex items-center gap-1.5"
            >
              <Github className="h-4 w-4" />
              GitHub
            </a>
          </nav>
        </div>

        <div className="mt-6 pt-6 border-t border-border/40 flex flex-col md:flex-row items-center justify-between gap-2 text-sm text-muted-foreground">
          <p>© {new Date().getFullYear()} Docklift. Open source under MIT license.</p>
          <p className="flex items-center gap-1">
            Made with <Heart className="h-3.5 w-3.5 text-red-500 fill-red-500" /> for developers
          </p>
        </div>
      </div>
    </footer>
  );
}
