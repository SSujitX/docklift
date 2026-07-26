// Compact account menu for the top bar: avatar trigger → email (copyable) + profile + sign out.
// Theme lives on the TopBar toggle, so appearance is not duplicated here.

import { useEffect, useRef, useState } from "react";
import { Link } from "react-router-dom";
import { Check, Copy, LogOut, UserCircle } from "lucide-react";
import { toast } from "sonner";
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
  const [copied, setCopied] = useState(false);
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

  useEffect(() => {
    if (!open) setCopied(false);
  }, [open]);

  if (!user) return null;

  const label = displayName(user.name, user.email);
  const email = user.email?.trim() || "";

  const copyEmail = async () => {
    if (!email) return;
    try {
      await navigator.clipboard.writeText(email);
      setCopied(true);
      toast.success("Email copied");
      window.setTimeout(() => setCopied(false), 1600);
    } catch {
      toast.error("Could not copy email");
    }
  };

  return (
    <div ref={containerRef} className="relative">
      <button
        type="button"
        onClick={() => setOpen((value) => !value)}
        aria-expanded={open}
        aria-haspopup="menu"
        aria-label={`Account menu for ${label}`}
        title={email || label}
        className={cn(
          "flex h-9 w-9 items-center justify-center rounded-full transition-colors",
          open ? "bg-secondary/70" : "hover:bg-secondary/50",
        )}
      >
        <span className="relative flex h-7 w-7 shrink-0 items-center justify-center rounded-full bg-gradient-to-br from-brand to-blue-600 text-[11px] font-bold text-white">
          {initials(user.name, user.email)}
          <span className="absolute -bottom-px -right-px h-2 w-2 rounded-full border-2 border-background bg-emerald-500" />
        </span>
      </button>

      {open && (
        <div
          role="menu"
          className="absolute right-0 top-full z-50 mt-2 w-[min(17rem,calc(100vw-1.5rem))] overflow-hidden rounded-2xl border border-border/60 bg-background shadow-xl shadow-black/10 animate-in fade-in slide-in-from-top-2 duration-150"
        >
          <div className="border-b border-border/60 px-3 py-3">
            <div className="flex items-center gap-3">
              <span className="flex h-9 w-9 shrink-0 items-center justify-center rounded-full bg-gradient-to-br from-brand to-blue-600 text-xs font-bold text-white">
                {initials(user.name, user.email)}
              </span>
              <div className="min-w-0 flex-1">
                <p className="truncate text-sm font-semibold">{label}</p>
                {email && label.toLowerCase() !== email.toLowerCase() && (
                  <p className="text-[11px] text-muted-foreground">Signed in</p>
                )}
              </div>
            </div>
            {email && (
              <button
                type="button"
                role="menuitem"
                onClick={() => void copyEmail()}
                title="Copy email"
                className="mt-2.5 flex w-full items-center gap-2 rounded-xl border border-border/60 bg-secondary/30 px-2.5 py-2 text-left transition-colors hover:border-brand/30 hover:bg-brand/5"
              >
                <span className="min-w-0 flex-1 truncate font-mono text-xs text-foreground/90">
                  {email}
                </span>
                {copied ? (
                  <Check className="h-3.5 w-3.5 shrink-0 text-emerald-500" />
                ) : (
                  <Copy className="h-3.5 w-3.5 shrink-0 text-muted-foreground" />
                )}
              </button>
            )}
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
