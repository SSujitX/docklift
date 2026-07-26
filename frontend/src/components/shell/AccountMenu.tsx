// Compact account menu for the top bar: profile + sign out.
// Theme lives on the TopBar toggle, so appearance is not duplicated here.

import { useEffect, useRef, useState } from "react";
import { Link } from "react-router-dom";
import { LogOut, UserCircle } from "lucide-react";
import { useAuth } from "@/components/AuthProvider";
import { cn } from "@/lib/utils";

function initials(name?: string, email?: string): string {
  const source = name?.trim() || email?.trim() || "";
  if (!source) return "U";
  const parts = source.split(/[\s@._-]+/).filter(Boolean);
  const letters = parts.slice(0, 2).map((part) => part.charAt(0));
  return (letters.join("") || source.charAt(0)).toUpperCase();
}

function displayName(name?: string, email?: string): string {
  const n = name?.trim() || "";
  const e = email?.trim() || "";
  if (n && n.toLowerCase() !== e.toLowerCase()) return n;
  return e || n || "Account";
}

export function AccountMenu() {
  const { user, logout } = useAuth();
  const [open, setOpen] = useState(false);
  const containerRef = useRef<HTMLDivElement>(null);

  useEffect(() => {
    if (!open) return;
    const onPointerDown = (event: MouseEvent) => {
      if (!containerRef.current?.contains(event.target as Node)) setOpen(false);
    };
    const onKeyDown = (event: KeyboardEvent) => {
      if (event.key === "Escape") setOpen(false);
    };
    document.addEventListener("mousedown", onPointerDown);
    document.addEventListener("keydown", onKeyDown);
    return () => {
      document.removeEventListener("mousedown", onPointerDown);
      document.removeEventListener("keydown", onKeyDown);
    };
  }, [open]);

  if (!user) return null;

  const label = displayName(user.name, user.email);
  const showEmailUnder =
    Boolean(user.email) && label.toLowerCase() !== user.email.toLowerCase();

  return (
    <div ref={containerRef} className="relative">
      <button
        type="button"
        onClick={() => setOpen((value) => !value)}
        aria-expanded={open}
        aria-haspopup="menu"
        aria-label={`Account menu for ${label}`}
        title={user.email || label}
        className={cn(
          "flex h-9 items-center gap-2 rounded-full pl-1 pr-1.5 transition-colors sm:pr-2.5",
          open
            ? "bg-secondary/70"
            : "hover:bg-secondary/50",
        )}
      >
        <span className="relative flex h-7 w-7 shrink-0 items-center justify-center rounded-full bg-gradient-to-br from-brand to-blue-600 text-[11px] font-bold text-white">
          {initials(user.name, user.email)}
          <span className="absolute -bottom-px -right-px h-2 w-2 rounded-full border-2 border-background bg-emerald-500" />
        </span>
        <span className="hidden max-w-[9rem] truncate text-left text-sm font-medium leading-none sm:block">
          {label}
        </span>
      </button>

      {open && (
        <div
          role="menu"
          className="absolute right-0 top-full z-50 mt-2 w-[min(16rem,calc(100vw-1.5rem))] overflow-hidden rounded-2xl border border-border/60 bg-background shadow-xl shadow-black/10 animate-in fade-in slide-in-from-top-2 duration-150"
        >
          <div className="flex items-center gap-3 border-b border-border/60 px-3 py-3">
            <span className="flex h-9 w-9 shrink-0 items-center justify-center rounded-full bg-gradient-to-br from-brand to-blue-600 text-xs font-bold text-white">
              {initials(user.name, user.email)}
            </span>
            <div className="min-w-0">
              <p className="truncate text-sm font-semibold">{label}</p>
              {showEmailUnder && (
                <p className="truncate text-xs text-muted-foreground">{user.email}</p>
              )}
            </div>
          </div>

          <div className="p-1.5">
            <Link
              to="/settings?tab=profile"
              role="menuitem"
              onClick={() => setOpen(false)}
              className="flex items-center gap-2.5 rounded-xl px-2.5 py-2 text-sm font-medium text-foreground/85 transition-colors hover:bg-secondary hover:text-foreground"
            >
              <UserCircle className="h-4 w-4" />
              Profile settings
            </Link>
            <button
              type="button"
              role="menuitem"
              onClick={() => {
                setOpen(false);
                logout();
              }}
              className="flex w-full items-center gap-2.5 rounded-xl px-2.5 py-2 text-sm font-medium text-red-500 transition-colors hover:bg-red-500/10"
            >
              <LogOut className="h-4 w-4" />
              Sign out
            </button>
          </div>
        </div>
      )}
    </div>
  );
}
