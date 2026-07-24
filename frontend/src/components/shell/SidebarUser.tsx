// Account block pinned to the bottom of the rail: identity, theme and sign out.

import { useEffect, useRef, useState } from "react";
import { Link } from "react-router-dom";
import { ChevronsUpDown, LogOut, Monitor, Moon, Sun, UserCircle } from "lucide-react";
import { useAuth } from "@/components/AuthProvider";
import { useTheme } from "@/lib/theme";
import { cn } from "@/lib/utils";

const themeOptions = [
  { value: "light", label: "Light", icon: Sun },
  { value: "dark", label: "Dark", icon: Moon },
  { value: "system", label: "Auto", icon: Monitor },
] as const;

function initials(name?: string, email?: string): string {
  const source = name?.trim() || email?.trim() || "";
  if (!source) return "U";
  const parts = source.split(/[\s@._-]+/).filter(Boolean);
  const letters = parts.slice(0, 2).map((part) => part.charAt(0));
  return (letters.join("") || source.charAt(0)).toUpperCase();
}

export function SidebarUser({ collapsed }: { collapsed: boolean }) {
  const { user, logout } = useAuth();
  const { theme, setTheme } = useTheme();
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

  // Collapsing the rail would leave the popover floating with no anchor.
  useEffect(() => {
    setOpen(false);
  }, [collapsed]);

  if (!user) return null;

  return (
    <div ref={containerRef} className="relative">
      {open && (
        <div
          className={cn(
            "absolute bottom-full z-50 mb-2 w-64 overflow-hidden rounded-2xl border border-sidebar-border",
            "bg-sidebar shadow-2xl shadow-black/30 animate-in fade-in slide-in-from-bottom-2 duration-150",
            collapsed ? "left-0" : "left-0 right-0 w-auto",
          )}
        >
          <div className="border-b border-sidebar-border px-3 py-3">
            <p className="truncate text-sm font-semibold text-sidebar-foreground">
              {user.name}
            </p>
            <p className="truncate text-xs text-sidebar-muted">{user.email}</p>
          </div>

          <div className="p-1.5">
            <Link
              to="/settings?tab=profile"
              onClick={() => setOpen(false)}
              className="flex items-center gap-2.5 rounded-xl px-2.5 py-2 text-sm font-medium text-sidebar-foreground/85 transition-colors hover:bg-sidebar-accent hover:text-sidebar-foreground"
            >
              <UserCircle className="h-4 w-4" />
              Profile settings
            </Link>
          </div>

          <div className="border-t border-sidebar-border p-1.5">
            <p className="px-2.5 pb-1.5 pt-1 text-[10px] font-semibold uppercase tracking-[0.18em] text-sidebar-muted">
              Appearance
            </p>
            <div className="flex gap-1 rounded-xl bg-sidebar-accent/60 p-1">
              {themeOptions.map((option) => (
                <button
                  key={option.value}
                  type="button"
                  onClick={() => setTheme(option.value)}
                  className={cn(
                    "flex flex-1 items-center justify-center gap-1.5 rounded-lg py-1.5 text-[11px] font-semibold transition-colors",
                    theme === option.value
                      ? "bg-sidebar text-brand shadow-sm"
                      : "text-sidebar-muted hover:text-sidebar-foreground",
                  )}
                >
                  <option.icon className="h-3.5 w-3.5" />
                  {option.label}
                </button>
              ))}
            </div>
          </div>

          <div className="border-t border-sidebar-border p-1.5">
            <button
              type="button"
              onClick={() => {
                setOpen(false);
                logout();
              }}
              className="flex w-full items-center gap-2.5 rounded-xl px-2.5 py-2 text-sm font-medium text-red-400 transition-colors hover:bg-red-500/10"
            >
              <LogOut className="h-4 w-4" />
              Sign out
            </button>
          </div>
        </div>
      )}

      <button
        type="button"
        onClick={() => setOpen((value) => !value)}
        title={collapsed ? `${user.name} — ${user.email}` : undefined}
        className={cn(
          "group flex w-full items-center rounded-2xl border transition-colors",
          open
            ? "border-sidebar-border bg-sidebar-accent"
            : "border-transparent hover:border-sidebar-border hover:bg-sidebar-accent/70",
          collapsed ? "justify-center p-1.5" : "gap-2.5 p-2",
        )}
      >
        <span className="relative flex h-9 w-9 shrink-0 items-center justify-center rounded-xl bg-gradient-to-br from-brand to-blue-600 text-xs font-bold text-white">
          {initials(user.name, user.email)}
          <span className="absolute -bottom-0.5 -right-0.5 h-2.5 w-2.5 rounded-full border-2 border-sidebar bg-emerald-500" />
        </span>

        {!collapsed && (
          <>
            <span className="min-w-0 flex-1 text-left">
              <span className="block truncate text-sm font-semibold text-sidebar-foreground">
                {user.name}
              </span>
              <span className="block truncate text-[11px] text-sidebar-muted">
                {user.email}
              </span>
            </span>
            <ChevronsUpDown className="h-4 w-4 shrink-0 text-sidebar-muted transition-colors group-hover:text-sidebar-foreground" />
          </>
        )}
      </button>

      {!collapsed && (
        <p className="mt-1.5 px-2.5 text-[10px] font-medium uppercase tracking-[0.16em] text-sidebar-muted/70">
          {user.role} account
        </p>
      )}
    </div>
  );
}
