// BranchSelector component - responsive dropdown for selecting GitHub branches
"use client";

import { useState, useRef, useEffect } from "react";
import { Check, ChevronDown, GitBranch, Loader2, Search, X } from "lucide-react";
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
  const inputRef = useRef<HTMLInputElement>(null);

  // Close when clicking outside
  useEffect(() => {
    const handleClickOutside = (event: MouseEvent) => {
      if (containerRef.current && !containerRef.current.contains(event.target as Node)) {
        setIsOpen(false);
        setSearch("");
      }
    };
    document.addEventListener("mousedown", handleClickOutside);
    return () => document.removeEventListener("mousedown", handleClickOutside);
  }, []);

  // Focus input when opened
  useEffect(() => {
    if (isOpen && inputRef.current) {
      inputRef.current.focus();
    }
  }, [isOpen]);

  const filteredBranches = branches.filter(b => 
    b.toLowerCase().includes(search.toLowerCase())
  );

  return (
    <div className={cn("relative", className)} ref={containerRef}>
      {/* Trigger Button */}
      <button
        type="button"
        onClick={() => !disabled && setIsOpen(!isOpen)}
        disabled={disabled}
        className={cn(
          "flex items-center gap-2 w-full h-11 px-3 rounded-lg",
          "bg-secondary/40 border border-border/60",
          "hover:border-cyan-500/50 transition-all text-sm",
          disabled && "opacity-50 cursor-not-allowed",
          isOpen && "border-cyan-500 ring-1 ring-cyan-500/20"
        )}
      >
        <GitBranch className={cn(
          "h-4 w-4 shrink-0",
          isOpen ? "text-cyan-500" : "text-muted-foreground"
        )} />
        
        <span className={cn(
          "flex-1 text-left truncate min-w-0",
          value ? "font-medium text-foreground" : "text-muted-foreground"
        )}>
          {loading ? "Loading..." : (value || "Select branch")}
        </span>

        {loading ? (
          <Loader2 className="h-4 w-4 animate-spin text-muted-foreground shrink-0" />
        ) : (
          <ChevronDown className={cn(
            "h-4 w-4 text-muted-foreground transition-transform shrink-0",
            isOpen && "rotate-180 text-cyan-500"
          )} />
        )}
      </button>

      {/* Dropdown */}
      {isOpen && (
        <div className="absolute z-[100] left-0 right-0 mt-1 bg-popover border border-border rounded-lg shadow-xl overflow-hidden">
          {/* Search */}
          <div className="p-2 border-b border-border/50">
            <div className="relative">
              <Search className="absolute left-2.5 top-1/2 -translate-y-1/2 h-3.5 w-3.5 text-muted-foreground" />
              <input
                ref={inputRef}
                value={search}
                onChange={(e) => setSearch(e.target.value)}
                placeholder="Filter..."
                className="w-full h-8 pl-8 pr-8 rounded-md bg-secondary/50 border-0 text-sm outline-none focus:ring-1 focus:ring-cyan-500/50 placeholder:text-muted-foreground/60"
              />
              {search && (
                <button
                  type="button"
                  onClick={() => setSearch("")}
                  className="absolute right-2 top-1/2 -translate-y-1/2 text-muted-foreground hover:text-foreground"
                >
                  <X className="h-3.5 w-3.5" />
                </button>
              )}
            </div>
          </div>

          {/* Branch List */}
          <div className="max-h-48 overflow-y-auto overflow-x-hidden">
            {filteredBranches.length === 0 ? (
              <div className="py-6 text-center text-sm text-muted-foreground">
                No branches found
              </div>
            ) : (
              filteredBranches.map((branch) => {
                const isSelected = value === branch;
                return (
                  <button
                    key={branch}
                    type="button"
                    onClick={() => {
                      onChange(branch);
                      setIsOpen(false);
                      setSearch("");
                    }}
                    className={cn(
                      "flex items-center gap-2 w-full px-3 py-2 text-sm text-left transition-colors",
                      isSelected 
                        ? "bg-cyan-500/10 text-cyan-500 font-medium" 
                        : "text-foreground hover:bg-secondary/50"
                    )}
                  >
                    <GitBranch className="h-3.5 w-3.5 shrink-0 text-muted-foreground" />
                    <span className="flex-1 truncate min-w-0 break-all">{branch}</span>
                    {isSelected && <Check className="h-3.5 w-3.5 shrink-0" />}
                  </button>
                );
              })
            )}
          </div>
        </div>
      )}
    </div>
  );
}
