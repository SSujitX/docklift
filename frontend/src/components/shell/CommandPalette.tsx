// Ctrl/Cmd+K launcher: jump to any page, open a project, or run a shell action.

import { useEffect, useMemo, useRef, useState } from "react";
import { useNavigate } from "react-router-dom";
import {
  Container,
  Database,
  LogOut,
  Moon,
  Plus,
  Search,
  Sun,
} from "lucide-react";
import { useAuth } from "@/components/AuthProvider";
import { apiCache } from "@/lib/auth";
import { API } from "@/lib/prefetch";
import { useFocusTrap } from "@/lib/focusTrap";
import { useTheme } from "@/lib/theme";
import { cn } from "@/lib/utils";
import type { Project } from "@/lib/types";
import { SETTINGS_SECTIONS, settingsHref } from "@/lib/settingsNav";
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
    const cached = apiCache.peek(API.projects);
    if (Array.isArray(cached)) setProjects(cached as Project[]);
    apiCache
      .fetch(API.projects)
      .then((res) => (res.ok ? res.json() : []))
      .then((data) => setProjects(Array.isArray(data) ? data : []))
      .catch(() => {});
  }, [paletteOpen]);

  const commands = useMemo<Command[]>(() => {
    const close = (action: () => void) => () => {
      setPaletteOpen(false);
      action();
    };

    const pages: Command[] = [
      ...navItems.map((item) => ({
        id: `nav:${item.href}`,
        label: item.label,
        hint: item.description,
        group: "Go to",
        icon: item.icon,
        run: close(() => {
          if (item.external) {
            window.open(item.href, "_blank", "noopener,noreferrer");
            return;
          }
          navigate(item.href);
        }),
      })),
      ...SETTINGS_SECTIONS.map((section) => ({
        id: `settings:${section.id}`,
        label: section.label,
        hint: section.description,
        group: "Settings",
        icon: section.icon,
        run: close(() => navigate(settingsHref(section.id))),
      })),
    ];

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
    <div className="fixed inset-0 z-100 flex items-start justify-center p-4 pt-[12vh]">
      <button
        type="button"
        aria-label="Close command palette"
        onClick={() => setPaletteOpen(false)}
        className="absolute inset-0 cursor-default bg-black/50 animate-in fade-in-0 duration-150"
      />

      <div
        ref={dialogRef}
        role="dialog"
        aria-modal="true"
        aria-label="Command palette"
        onKeyDown={onKeyDown}
        className="relative w-full max-w-xl overflow-hidden rounded-lg border border-border bg-popover text-popover-foreground shadow-popover animate-in fade-in-0 slide-in-from-top-1 duration-150"
      >
        <div className="flex items-center gap-2.5 border-b border-border px-3">
          <Search className="size-4 shrink-0 text-subtle" />
          <input
            ref={inputRef}
            value={query}
            onChange={(event) => setQuery(event.target.value)}
            placeholder="Search pages, projects and actions…"
            className="h-12 min-w-0 flex-1 bg-transparent text-sm text-foreground outline-hidden placeholder:text-subtle"
          />
          <kbd className="rounded-sm border border-border px-1 font-mono text-[11px] leading-4 text-subtle">
            Esc
          </kbd>
        </div>

        <div ref={listRef} className="shell-scroll max-h-[52vh] overflow-y-auto p-1.5">
          {results.length === 0 ? (
            <p className="px-3 py-8 text-center text-[13px] text-muted-foreground">
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
                    <p className="px-2 pb-1 pt-2.5 text-xs leading-4 text-subtle">
                      {command.group}
                    </p>
                  )}
                  <button
                    type="button"
                    data-selected={isSelected}
                    onMouseEnter={() => setSelected(index)}
                    onClick={command.run}
                    className={cn(
                      "flex h-9 w-full items-center gap-2.5 rounded-md px-2 text-left transition-colors duration-100 pointer-coarse:h-11",
                      "focus-visible:outline-2 focus-visible:-outline-offset-2 focus-visible:outline-ring",
                      isSelected && "bg-secondary",
                    )}
                  >
                    <command.icon
                      className={cn(
                        "size-4 shrink-0",
                        isSelected ? "text-foreground" : "text-subtle",
                      )}
                    />
                    <span className="min-w-0 shrink truncate text-[13.5px] font-medium text-foreground">
                      {command.label}
                    </span>
                    <span className="ml-auto min-w-0 max-w-[55%] truncate text-xs text-muted-foreground">
                      {command.hint}
                    </span>
                    <kbd
                      aria-hidden="true"
                      className={cn(
                        "w-3 shrink-0 text-center font-mono text-xs text-subtle",
                        !isSelected && "invisible",
                      )}
                    >
                      ↵
                    </kbd>
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
