// Full-screen IDE-style Monaco editor for project source files.

import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import Editor, { type OnMount } from "@monaco-editor/react";
import { Button } from "./ui/button";
import { API_URL, cn } from "@/lib/utils";
import { authFetch } from "@/lib/auth";
import { useTheme } from "@/lib/theme";
import { toast } from "sonner";
import {
  Save,
  Loader2,
  X,
  Circle,
  WrapText,
  Map as MapIcon,
} from "lucide-react";

interface FileEditorProps {
  projectId: string;
  filename: string;
  content: string;
  onClose: () => void;
  onSave: () => void;
}

function languageFromPath(name: string): string {
  const base = name.split("/").pop() || name;
  if (base === "Dockerfile" || base.startsWith("Dockerfile.")) return "dockerfile";
  const ext = base.includes(".") ? base.split(".").pop()?.toLowerCase() : "";
  switch (ext) {
    case "js":
    case "jsx":
    case "mjs":
    case "cjs":
      return "javascript";
    case "ts":
    case "tsx":
      return "typescript";
    case "py":
      return "python";
    case "json":
      return "json";
    case "html":
    case "htm":
      return "html";
    case "css":
      return "css";
    case "scss":
      return "scss";
    case "md":
    case "mdx":
      return "markdown";
    case "yml":
    case "yaml":
      return "yaml";
    case "sh":
    case "bash":
      return "shell";
    case "toml":
      return "ini";
    case "go":
      return "go";
    case "rs":
      return "rust";
    case "sql":
      return "sql";
    case "env":
      return "ini";
    default:
      return "plaintext";
  }
}

export function FileEditor({
  projectId,
  filename,
  content,
  onClose,
  onSave,
}: FileEditorProps) {
  const { resolvedTheme } = useTheme();
  const [value, setValue] = useState(content);
  const [saving, setSaving] = useState(false);
  const [wordWrap, setWordWrap] = useState<"on" | "off">("on");
  const [minimap, setMinimap] = useState(true);
  const [isNarrow, setIsNarrow] = useState(false);

  const language = useMemo(() => languageFromPath(filename), [filename]);
  const basename = filename.split("/").pop() || filename;
  const pathParts = filename.split("/").filter(Boolean);

  useEffect(() => {
    const mq = window.matchMedia("(max-width: 767px)");
    const apply = () => {
      setIsNarrow(mq.matches);
      if (mq.matches) setMinimap(false);
    };
    apply();
    mq.addEventListener("change", apply);
    return () => mq.removeEventListener("change", apply);
  }, []);

  const [baseline, setBaseline] = useState(content);
  const isDirty = value !== baseline;

  const handleSave = useCallback(async () => {
    if (saving) return;
    setSaving(true);
    try {
      const res = await authFetch(
        `${API_URL}/api/files/${projectId}/content?path=${encodeURIComponent(filename)}`,
        {
          method: "PUT",
          headers: { "Content-Type": "application/json" },
          body: JSON.stringify({ filename, content: value }),
        },
      );
      if (!res.ok) {
        const data = await res.json().catch(() => ({}));
        throw new Error(data.error || "Save failed");
      }
      setBaseline(value);
      toast.success(`Saved ${basename}`);
      onSave();
    } catch (error) {
      toast.error(error instanceof Error ? error.message : "Save failed");
    } finally {
      setSaving(false);
    }
  }, [saving, projectId, filename, value, basename, onSave]);

  const handleSaveRef = useRef(handleSave);
  handleSaveRef.current = handleSave;
  const dirtyRef = useRef(isDirty);
  dirtyRef.current = isDirty;

  useEffect(() => {
    const onKey = (e: KeyboardEvent) => {
      if ((e.metaKey || e.ctrlKey) && e.key.toLowerCase() === "s") {
        e.preventDefault();
        void handleSaveRef.current();
      }
      if (e.key === "Escape") {
        e.preventDefault();
        if (dirtyRef.current && !window.confirm("Discard unsaved changes?")) return;
        onClose();
      }
    };
    window.addEventListener("keydown", onKey);
    return () => window.removeEventListener("keydown", onKey);
  }, [onClose]);

  // Lock body scroll while editor is open
  useEffect(() => {
    const prev = document.body.style.overflow;
    document.body.style.overflow = "hidden";
    return () => {
      document.body.style.overflow = prev;
    };
  }, []);

  const handleClose = () => {
    if (isDirty && !window.confirm("Discard unsaved changes?")) return;
    onClose();
  };

  const onMount: OnMount = (editor, monaco) => {
    editor.focus();
    editor.addCommand(monaco.KeyMod.CtrlCmd | monaco.KeyCode.KeyS, () => {
      void handleSaveRef.current();
    });
  };

  const monacoTheme = resolvedTheme === "light" ? "vs" : "vs-dark";

  return (
    <div
      className="fixed inset-0 z-[100] flex flex-col bg-background"
      role="dialog"
      aria-modal="true"
      aria-label={`Edit ${basename}`}
    >
      {/* IDE title bar */}
      <header className="flex shrink-0 flex-col gap-2 border-b border-border/60 bg-secondary/30 px-3 py-2.5 sm:flex-row sm:items-center sm:justify-between sm:gap-4 sm:px-4">
        <div className="flex min-w-0 items-center gap-3">
          <Button
            type="button"
            variant="ghost"
            size="icon"
            onClick={handleClose}
            className="h-9 w-9 shrink-0 rounded-xl"
            title="Close (Esc)"
          >
            <X className="h-4 w-4" />
          </Button>
          <div className="min-w-0 flex-1">
            <div className="flex min-w-0 items-center gap-2">
              {isDirty && (
                <Circle className="h-2.5 w-2.5 shrink-0 fill-brand text-brand" />
              )}
              <h2 className="truncate text-sm font-semibold tracking-tight">
                {basename}
              </h2>
              <span className="hidden shrink-0 rounded-md border border-border/60 bg-background/60 px-1.5 py-0.5 text-[10px] font-semibold uppercase tracking-wider text-muted-foreground sm:inline">
                {language}
              </span>
              {isDirty && (
                <span className="shrink-0 text-[10px] font-medium text-brand">
                  Unsaved
                </span>
              )}
            </div>
            <p className="mt-0.5 truncate font-mono text-[10px] text-muted-foreground">
              {pathParts.length > 1 ? pathParts.join(" / ") : filename}
            </p>
          </div>
        </div>

        <div className="flex items-center gap-1.5 sm:gap-2">
          <Button
            type="button"
            variant="outline"
            size="sm"
            onClick={() => setWordWrap((w) => (w === "on" ? "off" : "on"))}
            className={cn(
              "h-9 rounded-xl border-border/60 px-2.5",
              wordWrap === "on" && "border-brand/30 bg-brand/10 text-brand",
            )}
            title="Toggle word wrap"
          >
            <WrapText className="h-4 w-4" />
            <span className="hidden sm:inline">Wrap</span>
          </Button>
          {!isNarrow && (
            <Button
              type="button"
              variant="outline"
              size="sm"
              onClick={() => setMinimap((m) => !m)}
              className={cn(
                "h-9 rounded-xl border-border/60 px-2.5",
                minimap && "border-brand/30 bg-brand/10 text-brand",
              )}
              title="Toggle minimap"
            >
              <MapIcon className="h-4 w-4" />
              <span className="hidden sm:inline">Map</span>
            </Button>
          )}
          <Button
            type="button"
            onClick={() => void handleSave()}
            disabled={saving || !isDirty}
            className="h-9 flex-1 rounded-xl bg-brand px-4 font-semibold text-brand-foreground shadow-lg shadow-brand/15 hover:brightness-110 disabled:opacity-50 sm:flex-none"
          >
            {saving ? (
              <Loader2 className="h-4 w-4 animate-spin" />
            ) : (
              <Save className="h-4 w-4" />
            )}
            Save
          </Button>
        </div>
      </header>

      {/* Editor canvas — full remaining viewport */}
      <div className="min-h-0 flex-1">
        <Editor
          height="100%"
          language={language}
          theme={monacoTheme}
          value={value}
          onChange={(val) => setValue(val ?? "")}
          onMount={onMount}
          loading={
            <div className="flex h-full flex-col items-center justify-center gap-3 text-muted-foreground">
              <Loader2 className="h-7 w-7 animate-spin text-brand" />
              <span className="text-sm">Loading editor…</span>
            </div>
          }
          options={{
            fontSize: isNarrow ? 13 : 14,
            fontFamily:
              'ui-monospace, SFMono-Regular, Menlo, Monaco, Consolas, "Liberation Mono", "Courier New", monospace',
            fontLigatures: true,
            minimap: {
              enabled: minimap && !isNarrow,
              scale: 1,
              showSlider: "mouseover",
            },
            scrollBeyondLastLine: false,
            padding: { top: 12, bottom: 12 },
            smoothScrolling: true,
            cursorBlinking: "smooth",
            cursorSmoothCaretAnimation: "on",
            formatOnPaste: true,
            formatOnType: true,
            automaticLayout: true,
            tabSize: 2,
            wordWrap,
            lineNumbers: "on",
            lineNumbersMinChars: 3,
            renderLineHighlight: "all",
            bracketPairColorization: { enabled: true },
            guides: { indentation: true, bracketPairs: true },
            stickyScroll: { enabled: true },
            find: { addExtraSpaceOnTop: false },
            mouseWheelZoom: true,
            accessibilitySupport: "auto",
          }}
        />
      </div>

      {/* Status bar */}
      <footer className="flex shrink-0 items-center justify-between gap-3 border-t border-border/60 bg-secondary/40 px-3 py-1.5 text-[11px] text-muted-foreground sm:px-4">
        <div className="flex min-w-0 items-center gap-2 sm:gap-3">
          <span className="uppercase tracking-wide">{language}</span>
          <span className="text-border">·</span>
          <span>UTF-8</span>
          <span className="hidden text-border sm:inline">·</span>
          <span className="hidden truncate sm:inline">
            {isDirty ? "Modified" : "Saved"}
          </span>
        </div>
        <div className="shrink-0 tabular-nums">
          <span className="hidden sm:inline">Ctrl/⌘S save · Esc close</span>
          <span className="sm:hidden">⌘S · Esc</span>
        </div>
      </footer>
    </div>
  );
}
