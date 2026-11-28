// Compact account menu for the top bar: avatar trigger → email (copyable) + profile + sign out.
// Theme lives on the TopBar toggle, so appearance is not duplicated here.

import { useEffect, useRef, useState } from "react";
import { Link } from "react-router-dom";
import { Check, Copy, LogOut, UserCircle } from "lucide-react";
import { toast } from "sonner";
import { useAuth } from "@/components/AuthProvider";
import { cn } from "@/lib/utils";

/** Menu row: 32px (40px on touch). */
const MENU_ITEM =
  "flex h-8 w-full items-center gap-2.5 rounded-md px-2 text-left text-[13px] font-medium text-foreground transition-colors duration-150 pointer-coarse:h-10 focus-visible:outline-2 focus-visible:-outline-offset-2 focus-visible:outline-ring";

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
          "flex size-8 shrink-0 items-center justify-center rounded-full transition-colors duration-150 pointer-coarse:size-10",
          "focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-ring",
          open ? "bg-secondary" : "hover:bg-secondary",
        )}
      >
        <span className="flex size-[26px] shrink-0 items-center justify-center rounded-full bg-brand text-[11.5px] font-semibold text-brand-foreground ring-2 ring-brand/25">
          {initials(user.name, user.email)}
        </span>
      </button>

      {open && (
        <div
          role="menu"
          className="absolute right-0 top-full z-50 mt-1.5 w-[min(16rem,calc(100vw-1.5rem))] overflow-hidden rounded-lg border border-border bg-popover text-popover-foreground shadow-popover animate-in fade-in-0 slide-in-from-top-1 duration-150"
        >
          <div className="border-b border-border p-2">
            <div className="px-2 pb-1.5 pt-1">
              <p className="truncate text-[13.5px] font-medium leading-5">{label}</p>
              {email && label.toLowerCase() !== email.toLowerCase() && (
                <p className="text-xs text-muted-foreground">Signed in</p>
              )}
            </div>
            {email && (
              <button
                type="button"
                role="menuitem"
                onClick={() => void copyEmail()}
                title="Copy email"
                className={cn(MENU_ITEM, "bg-secondary/60 hover:bg-secondary")}
              >
                <span className="min-w-0 flex-1 truncate font-mono text-xs text-foreground">
                  {email}
                </span>
                {copied ? (
                  <Check className="size-3.5 shrink-0 text-ok" />
                ) : (
                  <Copy className="size-3.5 shrink-0 text-subtle" />
                )}
              </button>
            )}
          </div>

          <div className="p-1">
            <Link
              to="/settings?tab=profile"
              role="menuitem"
              onClick={() => setOpen(false)}
              className={cn(MENU_ITEM, "hover:bg-secondary")}
            >
              <UserCircle className="size-4 text-subtle" />
              Profile settings
            </Link>
            <button
              type="button"
              role="menuitem"
              onClick={() => {
                setOpen(false);
                logout();
              }}
              className={cn(MENU_ITEM, "hover:bg-secondary")}
            >
              <LogOut className="size-4 text-subtle" />
              Sign out
            </button>
          </div>
        </div>
      )}
    </div>
  );
}
