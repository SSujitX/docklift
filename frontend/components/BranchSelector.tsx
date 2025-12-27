"use client";

import { useState, useRef, useEffect } from "react";
import { Check, ChevronDown, GitBranch, Loader2, Search } from "lucide-react";
import { cn } from "@/lib/utils";

interface BranchSelectorProps {
  branches: string[];
  value: string;
  onChange: (value: string) => void;
  loading?: boolean;
  disabled?: boolean;
  className?: string;
}

export function BranchSelector({ 
  branches, 
  value, 
  onChange, 
  loading = false, 
  disabled = false,
  className 
}: BranchSelectorProps) {
  const [isOpen, setIsOpen] = useState(false);
  const [search, setSearch] = useState("");
  const containerRef = useRef<HTMLDivElement>(null);

  // Close when clicking outside
  useEffect(() => {
    const handleClickOutside = (event: MouseEvent) => {
      if (containerRef.current && !containerRef.current.contains(event.target as Node)) {
        setIsOpen(false);
      }
    };
    document.addEventListener("mousedown", handleClickOutside);
    return () => document.removeEventListener("mousedown", handleClickOutside);
  }, []);

  const filteredBranches = branches.filter(b => 
    b.toLowerCase().includes(search.toLowerCase())
  );

  return (
    <div className={cn("relative", className)} ref={containerRef}>
      <button
        type="button"
        onClick={() => !disabled && setIsOpen(!isOpen)}
        disabled={disabled}
        className={cn(
          "flex items-center justify-between w-full h-12 px-4 rounded-xl bg-secondary/30 border border-border/40 hover:border-cyan-500/50 transition-all font-medium text-sm",
          disabled && "opacity-50 cursor-not-allowed",
          isOpen && "border-cyan-500 ring-2 ring-cyan-500/10"
        )}
      >
        <div className="flex items-center gap-2 truncate">
          <GitBranch className="h-4 w-4 text-muted-foreground" />
          <span className="truncate">
            {value || "Select branch..."}
          </span>
        </div>
        {loading ? (
          <Loader2 className="h-4 w-4 animate-spin text-muted-foreground" />
        ) : (
          <ChevronDown className={cn("h-4 w-4 text-muted-foreground transition-transform", isOpen && "rotate-180")} />
        )}
      </button>

      {isOpen && (
        <div className="absolute z-50 mt-2 w-full bg-card/95 backdrop-blur-xl border border-border/50 rounded-xl shadow-2xl p-2 animate-in fade-in zoom-in-95 duration-200">
          <div className="relative mb-2">
            <Search className="absolute left-3 top-1/2 -translate-y-1/2 h-3.5 w-3.5 text-muted-foreground" />
            <input
              autoFocus
              value={search}
              onChange={(e) => setSearch(e.target.value)}
              placeholder="Filter branches..."
              className="w-full h-9 pl-9 pr-3 rounded-lg bg-secondary/50 border border-transparent focus:border-cyan-500/50 focus:bg-secondary text-sm outline-none transition-all placeholder:text-muted-foreground/70"
            />
          </div>

          <div className="max-h-[240px] overflow-y-auto space-y-1 pr-1 custom-scrollbar">
            {filteredBranches.length === 0 ? (
              <div className="py-8 text-center text-muted-foreground text-xs">
                No branches found
              </div>
            ) : (
              filteredBranches.map((branch) => (
                <button
                  key={branch}
                  type="button"
                  onClick={() => {
                    onChange(branch);
                    setIsOpen(false);
                    setSearch("");
                  }}
                  className={cn(
                    "flex items-center justify-between w-full px-3 py-2 rounded-lg text-sm transition-colors",
                    value === branch 
                      ? "bg-cyan-500/10 text-cyan-500 font-bold" 
                      : "text-muted-foreground hover:text-foreground hover:bg-secondary"
                  )}
                >
                  <span className="truncate">{branch}</span>
                  {value === branch && <Check className="h-3.5 w-3.5" />}
                </button>
              ))
            )}
          </div>
        </div>
      )}
    </div>
  );
}
