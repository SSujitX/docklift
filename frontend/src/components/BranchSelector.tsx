// Git ref picker: Branch or Tag (newest-first), mobile-friendly searchable dropdown.

import { useState, useRef, useEffect } from "react";
import { Check, ChevronDown, GitBranch, Loader2, Search, Tag, X } from "lucide-react";
import { cn } from "@/lib/utils";

export type GitRefKind = "branch" | "tag";

interface BranchSelectorProps {
  branches: string[];
  tags?: string[];
  value: string;
  onChange: (value: string) => void;
  kind?: GitRefKind;
  onKindChange?: (kind: GitRefKind) => void;
  loading?: boolean;
  tagsLoading?: boolean;
  disabled?: boolean;
  className?: string;
  /** Compact trigger (e.g. step-2 summary row). */
  compact?: boolean;
  /** Hide the Branch | Tag switcher (branches only). */
  hideKindSwitch?: boolean;
}

export function BranchSelector({
  branches,
  tags = [],
  value,
  onChange,
  kind = "branch",
  onKindChange,
  loading = false,
  tagsLoading = false,
  disabled = false,
  className,
  compact = false,
  hideKindSwitch = false,
}: BranchSelectorProps) {
  const [isOpen, setIsOpen] = useState(false);
  const [search, setSearch] = useState("");
  const containerRef = useRef<HTMLDivElement>(null);
  const inputRef = useRef<HTMLInputElement>(null);

  const items = kind === "tag" ? tags : branches;
  const listLoading = kind === "tag" ? tagsLoading : loading;

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

  useEffect(() => {
    if (isOpen && inputRef.current) {
      inputRef.current.focus();
    }
  }, [isOpen]);

  const filtered = items.filter((item) =>
    item.toLowerCase().includes(search.toLowerCase()),
  );

  const placeholder =
    kind === "tag"
      ? listLoading
        ? "Loading tags…"
        : "Select tag"
      : listLoading
        ? "Loading branches…"
        : "Select branch";

  const switchKind = (next: GitRefKind) => {
    if (next === kind || !onKindChange) return;
    onKindChange(next);
    setSearch("");
    setIsOpen(false);
  };

  return (
    <div className={cn("w-full space-y-2", className)} ref={containerRef}>
      {!hideKindSwitch && onKindChange && (
        <div
          className="grid grid-cols-2 gap-1.5 rounded-xl border border-border/60 bg-secondary/30 p-1"
          role="tablist"
          aria-label="Deploy from"
        >
          {(
            [
              { id: "branch" as const, label: "Branch", icon: GitBranch },
              { id: "tag" as const, label: "Tag", icon: Tag },
            ] as const
          ).map((tab) => {
            const active = kind === tab.id;
            const Icon = tab.icon;
            const tagUnavailable =
              tab.id === "tag" && !tagsLoading && tags.length === 0;
            const tabDisabled = disabled || tagUnavailable;
            return (
              <button
                key={tab.id}
                type="button"
                role="tab"
                aria-selected={active}
                aria-disabled={tabDisabled}
                title={
                  tagUnavailable
                    ? "No tags in this repository"
                    : undefined
                }
                disabled={tabDisabled}
                onClick={() => switchKind(tab.id)}
                className={cn(
                  "inline-flex min-h-9 items-center justify-center gap-1.5 rounded-lg px-2 py-2 text-xs font-semibold transition-colors sm:text-sm",
                  active
                    ? "bg-brand/10 text-brand shadow-sm"
                    : "text-muted-foreground hover:text-foreground",
                  tabDisabled && "cursor-not-allowed opacity-40 hover:text-muted-foreground",
                )}
              >
                <Icon className="h-3.5 w-3.5 shrink-0" />
                {tab.label}
                <span className="tabular-nums text-[10px] opacity-70 sm:text-xs">
                  {tab.id === "branch" ? branches.length : tags.length}
                </span>
              </button>
            );
          })}
        </div>
      )}

      <div className="relative">
        <button
          type="button"
          onClick={() => !disabled && setIsOpen(!isOpen)}
          disabled={disabled}
          className={cn(
            "flex w-full items-center gap-2 rounded-xl border border-border/60 bg-secondary/40 px-3 text-sm transition-colors",
            compact ? "h-9" : "h-11",
            "hover:border-brand/40",
            disabled && "cursor-not-allowed opacity-50",
            isOpen && "border-brand/40 ring-2 ring-brand/15",
          )}
        >
          {kind === "tag" ? (
            <Tag
              className={cn(
                "h-4 w-4 shrink-0",
                isOpen ? "text-brand" : "text-muted-foreground",
              )}
            />
          ) : (
            <GitBranch
              className={cn(
                "h-4 w-4 shrink-0",
                isOpen ? "text-brand" : "text-muted-foreground",
              )}
            />
          )}

          <span
            className={cn(
              "min-w-0 flex-1 truncate text-left",
              value ? "font-medium text-foreground" : "text-muted-foreground",
            )}
          >
            {listLoading && !value ? placeholder : value || placeholder}
          </span>

          {listLoading ? (
            <Loader2 className="h-4 w-4 shrink-0 animate-spin text-muted-foreground" />
          ) : (
            <ChevronDown
              className={cn(
                "h-4 w-4 shrink-0 text-muted-foreground transition-transform",
                isOpen && "rotate-180 text-brand",
              )}
            />
          )}
        </button>

        {isOpen && (
          <div className="absolute left-0 right-0 z-[100] mt-1.5 overflow-hidden rounded-xl border border-border/60 bg-popover shadow-xl shadow-black/10">
            <div className="border-b border-border/50 p-2">
              <div className="relative">
                <Search className="absolute left-2.5 top-1/2 h-3.5 w-3.5 -translate-y-1/2 text-muted-foreground" />
                <input
                  ref={inputRef}
                  value={search}
                  onChange={(e) => setSearch(e.target.value)}
                  placeholder={kind === "tag" ? "Filter tags…" : "Filter branches…"}
                  className="h-9 w-full rounded-lg border-0 bg-secondary/50 pl-8 pr-8 text-sm outline-none placeholder:text-muted-foreground/60 focus:ring-2 focus:ring-brand/20"
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

            <div className="max-h-[min(16rem,45vh)] overflow-y-auto overflow-x-hidden overscroll-contain">
              {listLoading ? (
                <div className="flex items-center justify-center gap-2 py-8 text-sm text-muted-foreground">
                  <Loader2 className="h-4 w-4 animate-spin" />
                  Loading…
                </div>
              ) : filtered.length === 0 ? (
                <div className="px-3 py-8 text-center text-sm text-muted-foreground">
                  {kind === "tag"
                    ? search
                      ? "No tags match"
                      : "No tags in this repository"
                    : search
                      ? "No branches match"
                      : "No branches found"}
                </div>
              ) : (
                filtered.map((item, index) => {
                  const isSelected = value === item;
                  return (
                    <button
                      key={`${kind}-${item}`}
                      type="button"
                      onClick={() => {
                        onChange(item);
                        setIsOpen(false);
                        setSearch("");
                      }}
                      className={cn(
                        "flex w-full items-center gap-2 px-3 py-2.5 text-left text-sm transition-colors",
                        isSelected
                          ? "bg-brand/10 font-medium text-brand"
                          : "text-foreground hover:bg-secondary/50",
                      )}
                    >
                      {kind === "tag" ? (
                        <Tag className="h-3.5 w-3.5 shrink-0 text-muted-foreground" />
                      ) : (
                        <GitBranch className="h-3.5 w-3.5 shrink-0 text-muted-foreground" />
                      )}
                      <span className="min-w-0 flex-1 truncate break-all">{item}</span>
                      {kind === "tag" && index === 0 && !search && (
                        <span className="shrink-0 rounded-md border border-brand/25 bg-brand/10 px-1.5 py-0.5 text-[9px] font-semibold uppercase tracking-wider text-brand">
                          Latest
                        </span>
                      )}
                      {isSelected && <Check className="h-3.5 w-3.5 shrink-0" />}
                    </button>
                  );
                })
              )}
            </div>
          </div>
        )}
      </div>

      {kind === "tag" && (
        <p className="text-[11px] leading-relaxed text-muted-foreground">
          Pinned to this tag. Push auto-deploy will not move it until you change the ref.
        </p>
      )}
    </div>
  );
}
