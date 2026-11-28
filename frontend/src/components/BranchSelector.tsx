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
          className="inline-flex rounded-md border border-input bg-card p-0.5"
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
                  "inline-flex h-7 items-center gap-1.5 rounded-[4px] px-2.5 text-[13px] font-medium transition-colors duration-150 pointer-coarse:h-9",
                  active
                    ? "bg-secondary text-foreground"
                    : "text-muted-foreground hover:text-foreground",
                  tabDisabled && "cursor-not-allowed opacity-50 hover:text-muted-foreground",
                )}
              >
                <Icon className="size-3.5 shrink-0" />
                {tab.label}
                <span className="font-mono text-xs text-subtle">
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
            "flex w-full items-center gap-2 rounded-md border border-input bg-card px-3 text-sm transition-colors duration-150",
            compact ? "h-8 pointer-coarse:h-10" : "h-9 pointer-coarse:h-10",
            "hover:bg-secondary/50",
            disabled && "cursor-not-allowed opacity-50",
            isOpen && "border-brand ring-3 ring-brand/20",
          )}
        >
          {kind === "tag" ? (
            <Tag className="size-4 shrink-0 text-subtle" />
          ) : (
            <GitBranch className="size-4 shrink-0 text-subtle" />
          )}

          <span
            className={cn(
              "min-w-0 flex-1 truncate text-left",
              value ? "font-mono text-foreground" : "text-subtle",
            )}
          >
            {listLoading && !value ? placeholder : value || placeholder}
          </span>

          {listLoading ? (
            <Loader2 className="size-4 shrink-0 animate-spin text-subtle" />
          ) : (
            <ChevronDown
              className={cn(
                "size-4 shrink-0 text-subtle transition-transform duration-150",
                isOpen && "rotate-180",
              )}
            />
          )}
        </button>

        {isOpen && (
          <div className="absolute left-0 right-0 z-100 mt-1 overflow-hidden rounded-lg border border-border bg-popover shadow-popover">
            <div className="border-b border-border p-1.5">
              <div className="relative">
                <Search className="absolute left-2.5 top-1/2 size-3.5 -translate-y-1/2 text-subtle" />
                <input
                  ref={inputRef}
                  value={search}
                  onChange={(e) => setSearch(e.target.value)}
                  placeholder={kind === "tag" ? "Filter tags…" : "Filter branches…"}
                  className="h-8 w-full rounded-md border-0 bg-transparent pl-8 pr-8 text-[13px] outline-hidden placeholder:text-subtle"
                />
                {search && (
                  <button
                    type="button"
                    onClick={() => setSearch("")}
                    className="absolute right-2 top-1/2 -translate-y-1/2 text-subtle hover:text-foreground"
                  >
                    <X className="size-3.5" />
                  </button>
                )}
              </div>
            </div>

            <div className="max-h-[min(16rem,45vh)] overflow-y-auto overflow-x-hidden overscroll-contain p-1">
              {listLoading ? (
                <div className="flex items-center justify-center gap-2 py-8 text-[13px] text-muted-foreground">
                  <Loader2 className="size-4 animate-spin" />
                  Loading…
                </div>
              ) : filtered.length === 0 ? (
                <div className="px-3 py-8 text-center text-[13px] text-muted-foreground">
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
                        "flex h-8 w-full items-center gap-2 rounded-md px-2.5 text-left text-[13px] transition-colors duration-150 pointer-coarse:h-10",
                        isSelected
                          ? "bg-secondary text-foreground"
                          : "text-foreground hover:bg-secondary/60",
                      )}
                    >
                      <span className="min-w-0 flex-1 truncate break-all font-mono">{item}</span>
                      {kind === "tag" && index === 0 && !search && (
                        <span className="shrink-0 rounded-sm border border-border px-1.5 text-xs leading-5 text-muted-foreground">
                          Latest
                        </span>
                      )}
                      {isSelected && <Check className="size-3.5 shrink-0 text-brand" />}
                    </button>
                  );
                })
              )}
            </div>
          </div>
        )}
      </div>

      {kind === "tag" && (
        <p className="text-xs leading-5 text-muted-foreground">
          Pinned to this tag. Push auto-deploy will not move it until you change the ref.
        </p>
      )}
    </div>
  );
}
