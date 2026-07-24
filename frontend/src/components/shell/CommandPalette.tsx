// Ctrl/Cmd+K launcher: jump to any page, open a project, or run a shell action.

import { useEffect, useMemo, useRef, useState } from "react";
import { useNavigate } from "react-router-dom";
import {
  ArrowRight,
  Container,
  Database,
  LogOut,
  Moon,
  Plus,
  Search,
  Sun,
} from "lucide-react";
import { useAuth } from "@/components/AuthProvider";
import { authFetch } from "@/lib/auth";
import { useFocusTrap } from "@/lib/focusTrap";
import { useTheme } from "@/lib/theme";
import { API_URL, cn } from "@/lib/utils";
import type { Project } from "@/lib/types";
import { navItems, type IconComponent } from "./navigation";
import { useShell } from "./ShellContext";

interface Command {
  id: string;
  label: string;
  hint: string;
  group: string;
  icon: IconComponent;
  run: () => void;
}

export function CommandPalette() {
  const { paletteOpen, setPaletteOpen } = useShell();
  const [query, setQuery] = useState("");
  const [selected, setSelected] = useState(0);
  const [projects, setProjects] = useState<Project[]>([]);
  const inputRef = useRef<HTMLInputElement>(null);
  const listRef = useRef<HTMLDivElement>(null);
  const dialogRef = useRef<HTMLDivElement>(null);
  const navigate = useNavigate();
  const { logout } = useAuth();
  const { resolvedTheme, setTheme } = useTheme();

  useFocusTrap(paletteOpen, dialogRef);

  useEffect(() => {
    if (!paletteOpen) {
      setQuery("");
      setSelected(0);
      return;
    }
    inputRef.current?.focus();
    authFetch(`${API_URL}/api/projects`)
      .then((res) => (res.ok ? res.json() : []))
      .then((data) => setProjects(Array.isArray(data) ? data : []))
      .catch(() => {});
  }, [paletteOpen]);

  const commands = useMemo<Command[]>(() => {
    const close = (action: () => void) => () => {
      setPaletteOpen(false);
      action();
    };

    const pages: Command[] = navItems.map((item) => ({
      id: `nav:${item.href}`,
      label: item.label,
      hint: item.description,
      group: "Go to",
      icon: item.icon,
      run: close(() => navigate(item.href)),
    }));

    const projectCommands: Command[] = projects.map((project) => ({
      id: `project:${project.id}`,
      label: project.name,
      hint: `${project.project_type === "database" ? "Database" : "Application"} · ${project.status}`,
      group: "Projects",
      icon: project.project_type === "database" ? Database : Container,
      run: close(() => navigate(`/projects/${project.id}`)),
    }));

    const actions: Command[] = [
      {
        id: "action:new-project",
        label: "New project",
        hint: "Deploy from GitHub or an upload",
        group: "Actions",
        icon: Plus,
        run: close(() => navigate("/projects/new")),
      },
      {
        id: "action:theme",
        label: resolvedTheme === "dark" ? "Switch to light theme" : "Switch to dark theme",
        hint: "Appearance",
        group: "Actions",
        icon: resolvedTheme === "dark" ? Sun : Moon,
        run: close(() => setTheme(resolvedTheme === "dark" ? "light" : "dark")),
      },
      {
        id: "action:sign-out",
        label: "Sign out",
        hint: "End this session",
        group: "Actions",
        icon: LogOut,
        run: close(logout),
      },
    ];

    return [...pages, ...projectCommands, ...actions];
  }, [projects, navigate, logout, resolvedTheme, setTheme, setPaletteOpen]);

  const results = useMemo(() => {
    const needle = query.trim().toLowerCase();
    if (!needle) return commands;
    return commands.filter((command) =>
      `${command.label} ${command.hint} ${command.group}`.toLowerCase().includes(needle),
    );
  }, [commands, query]);

  useEffect(() => {
    setSelected(0);
  }, [query]);

  useEffect(() => {
    listRef.current
      ?.querySelector('[data-selected="true"]')
      ?.scrollIntoView({ block: "nearest" });
  }, [selected, results.length]);

  if (!paletteOpen) return null;

  const onKeyDown = (event: React.KeyboardEvent) => {
    if (event.key === "ArrowDown") {
      event.preventDefault();
      setSelected((index) => (results.length ? (index + 1) % results.length : 0));
    } else if (event.key === "ArrowUp") {
      event.preventDefault();
      setSelected((index) =>
        results.length ? (index - 1 + results.length) % results.length : 0,
      );
    } else if (event.key === "Enter") {
      event.preventDefault();
      results[selected]?.run();
    } else if (event.key === "Escape") {
      event.preventDefault();
      setPaletteOpen(false);
    }
  };

  let lastGroup = "";

  return (
    <div className="fixed inset-0 z-[100] flex items-start justify-center p-4 pt-[12vh]">
      <button
        type="button"
        aria-label="Close command palette"
        onClick={() => setPaletteOpen(false)}
        className="absolute inset-0 cursor-default bg-background/70 backdrop-blur-sm animate-in fade-in duration-150"
      />

      <div
        ref={dialogRef}
        role="dialog"
        aria-modal="true"
        aria-label="Command palette"
        onKeyDown={onKeyDown}
        className="relative w-full max-w-xl overflow-hidden rounded-2xl border border-border bg-card shadow-2xl shadow-black/30 animate-in fade-in zoom-in-95 duration-150"
      >
        <div className="flex items-center gap-3 border-b border-border px-4">
          <Search className="h-4 w-4 shrink-0 text-muted-foreground" />
          <input
            ref={inputRef}
            value={query}
            onChange={(event) => setQuery(event.target.value)}
            placeholder="Search pages, projects and actions…"
            className="h-14 flex-1 bg-transparent text-sm outline-none placeholder:text-muted-foreground"
          />
          <kbd className="rounded border border-border bg-secondary/60 px-1.5 py-0.5 text-[10px] font-semibold text-muted-foreground">
            Esc
          </kbd>
        </div>

        <div ref={listRef} className="shell-scroll max-h-[52vh] overflow-y-auto p-2">
          {results.length === 0 ? (
            <p className="px-3 py-8 text-center text-sm text-muted-foreground">
              Nothing matches “{query}”.
            </p>
          ) : (
            results.map((command, index) => {
              const showGroup = command.group !== lastGroup;
              lastGroup = command.group;
              const isSelected = index === selected;
              return (
                <div key={command.id}>
                  {showGroup && (
                    <p className="px-3 pb-1 pt-3 text-[10px] font-semibold uppercase tracking-[0.18em] text-muted-foreground/70">
                      {command.group}
                    </p>
                  )}
                  <button
                    type="button"
                    data-selected={isSelected}
                    onMouseEnter={() => setSelected(index)}
                    onClick={command.run}
                    className={cn(
                      "flex w-full items-center gap-3 rounded-xl px-3 py-2.5 text-left transition-colors",
                      isSelected ? "bg-brand/12" : "hover:bg-secondary/60",
                    )}
                  >
                    <command.icon
                      className={cn(
                        "h-4 w-4 shrink-0",
                        isSelected ? "text-brand" : "text-muted-foreground",
                      )}
                    />
                    <span className="min-w-0 flex-1">
                      <span className="block truncate text-sm font-medium">
                        {command.label}
                      </span>
                      <span className="block truncate text-xs text-muted-foreground">
                        {command.hint}
                      </span>
                    </span>
                    {isSelected && (
                      <ArrowRight className="h-3.5 w-3.5 shrink-0 text-brand" />
                    )}
                  </button>
                </div>
              );
            })
          )}
        </div>
      </div>
    </div>
  );
}
