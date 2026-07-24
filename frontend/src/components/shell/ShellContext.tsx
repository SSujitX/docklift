// Shell state shared by the sidebar, top bar and command palette.

import {
  createContext,
  useCallback,
  useContext,
  useEffect,
  useMemo,
  useState,
  type ReactNode,
} from "react";
import { useLocation } from "react-router-dom";

const COLLAPSE_KEY = "docklift_sidebar_collapsed";

interface ShellContextValue {
  /** Desktop rail is icon-only. */
  collapsed: boolean;
  toggleCollapsed: () => void;
  /** Mobile drawer visibility. */
  mobileOpen: boolean;
  setMobileOpen: (open: boolean) => void;
  paletteOpen: boolean;
  setPaletteOpen: (open: boolean) => void;
  /** Human label for the last breadcrumb, so ids never surface in the UI. */
  breadcrumbLeaf: string | null;
  setBreadcrumbLeaf: (label: string | null) => void;
}

const ShellContext = createContext<ShellContextValue | undefined>(undefined);

export function useShell(): ShellContextValue {
  const context = useContext(ShellContext);
  if (!context) throw new Error("useShell must be used within ShellProvider");
  return context;
}

export function ShellProvider({ children }: { children: ReactNode }) {
  const [collapsed, setCollapsed] = useState(
    () => localStorage.getItem(COLLAPSE_KEY) === "true",
  );
  const [mobileOpen, setMobileOpen] = useState(false);
  const [paletteOpen, setPaletteOpen] = useState(false);
  const [breadcrumbLeaf, setBreadcrumbLeaf] = useState<string | null>(null);
  const { pathname } = useLocation();

  const toggleCollapsed = useCallback(() => {
    setCollapsed((current) => {
      localStorage.setItem(COLLAPSE_KEY, String(!current));
      return !current;
    });
  }, []);

  // Navigating always dismisses the mobile drawer and the palette, and drops the
  // previous page's breadcrumb label.
  useEffect(() => {
    setMobileOpen(false);
    setPaletteOpen(false);
    setBreadcrumbLeaf(null);
  }, [pathname]);

  // The drawer is an overlay, so the page behind it must not scroll.
  useEffect(() => {
    if (!mobileOpen) return;
    const previous = document.body.style.overflow;
    document.body.style.overflow = "hidden";
    return () => {
      document.body.style.overflow = previous;
    };
  }, [mobileOpen]);

  useEffect(() => {
    const onKeyDown = (event: KeyboardEvent) => {
      if (event.key === "Escape") {
        setMobileOpen(false);
        return;
      }
      const meta = event.metaKey || event.ctrlKey;
      if (meta && event.key.toLowerCase() === "k") {
        event.preventDefault();
        setPaletteOpen((open) => !open);
        return;
      }
      if (meta && event.key.toLowerCase() === "b") {
        event.preventDefault();
        toggleCollapsed();
      }
    };
    window.addEventListener("keydown", onKeyDown);
    return () => window.removeEventListener("keydown", onKeyDown);
  }, [toggleCollapsed]);

  const value = useMemo(
    () => ({
      collapsed,
      toggleCollapsed,
      mobileOpen,
      setMobileOpen,
      paletteOpen,
      setPaletteOpen,
      breadcrumbLeaf,
      setBreadcrumbLeaf,
    }),
    [collapsed, toggleCollapsed, mobileOpen, paletteOpen, breadcrumbLeaf],
  );

  return <ShellContext.Provider value={value}>{children}</ShellContext.Provider>;
}

/**
 * Names the current page in the breadcrumb trail. Safe to call with a value that
 * is only known after a fetch — pass undefined until then.
 */
export function useBreadcrumbLeaf(label?: string | null) {
  const { setBreadcrumbLeaf } = useShell();
  useEffect(() => {
    if (!label) return;
    setBreadcrumbLeaf(label);
    return () => setBreadcrumbLeaf(null);
  }, [label, setBreadcrumbLeaf]);
}
