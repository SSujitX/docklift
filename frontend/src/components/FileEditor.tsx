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
      className="fixed inset-0 z-100 flex flex-col bg-background"
      role="dialog"
      aria-modal="true"
      aria-label={`Edit ${basename}`}
    >
      {/* Title bar */}
      <header className="flex shrink-0 flex-col gap-2 border-b border-border bg-background px-3 py-2 sm:flex-row sm:items-center sm:justify-between sm:gap-4 sm:px-4">
        <div className="flex min-w-0 items-center gap-2">
          <Button
            type="button"
            variant="ghost"
            size="icon"
            onClick={handleClose}
            className="shrink-0 text-subtle hover:text-foreground"
            title="Close (Esc)"
          >
            <X />
          </Button>
          <div className="min-w-0 flex-1">
            <div className="flex min-w-0 items-center gap-2">
              {isDirty && (
                <Circle className="size-2 shrink-0 fill-warn text-warn" />
              )}
              <h2 className="truncate font-mono text-sm font-medium">
                {basename}
              </h2>
              <span className="hidden shrink-0 rounded-sm border border-border px-1.5 text-xs leading-5 text-muted-foreground sm:inline">
                {language}
              </span>
              {isDirty && (
                <span className="shrink-0 text-xs text-warn">
                  Unsaved
                </span>
              )}
            </div>
            <p className="truncate font-mono text-xs text-muted-foreground">
              {pathParts.length > 1 ? pathParts.join(" / ") : filename}
            </p>
          </div>
        </div>

        <div className="flex items-center gap-2">
          <Button
            type="button"
            variant="outline"
            size="sm"
            onClick={() => setWordWrap((w) => (w === "on" ? "off" : "on"))}
            className={cn(
              wordWrap === "on" ? "bg-secondary text-foreground" : "text-muted-foreground",
            )}
            title="Toggle word wrap"
          >
            <WrapText className="size-3.5" />
            <span className="hidden sm:inline">Wrap</span>
          </Button>
          {!isNarrow && (
            <Button
              type="button"
              variant="outline"
              size="sm"
              onClick={() => setMinimap((m) => !m)}
              className={cn(
                minimap ? "bg-secondary text-foreground" : "text-muted-foreground",
              )}
              title="Toggle minimap"
            >
              <MapIcon className="size-3.5" />
              <span className="hidden sm:inline">Map</span>
            </Button>
          )}
          <Button
            type="button"
            onClick={() => void handleSave()}
            disabled={saving || !isDirty}
            className="flex-1 sm:flex-none"
          >
            {saving ? (
              <Loader2 className="animate-spin" />
            ) : (
              <Save />
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
            <div className="flex h-full items-center justify-center gap-2 text-[13px] text-muted-foreground">
              <Loader2 className="size-4 animate-spin" />
              Loading editor…
            </div>
          }
          options={{
            fontSize: isNarrow ? 13 : 14,
            fontFamily:
              '"IBM Plex Mono", ui-monospace, SFMono-Regular, Menlo, Monaco, Consolas, "Liberation Mono", "Courier New", monospace',
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
      <footer className="flex shrink-0 items-center justify-between gap-3 border-t border-border bg-background px-3 py-1.5 text-xs text-muted-foreground sm:px-4">
        <div className="flex min-w-0 items-center gap-2 sm:gap-3">
          <span className="font-mono">{language}</span>
          <span className="text-subtle">·</span>
          <span className="font-mono">UTF-8</span>
          <span className="hidden text-subtle sm:inline">·</span>
          <span className="hidden truncate sm:inline">
            {isDirty ? "Modified" : "Saved"}
          </span>
        </div>
        <div className="shrink-0 font-mono">
          <span className="hidden sm:inline">Ctrl/⌘S save · Esc close</span>
          <span className="sm:hidden">⌘S · Esc</span>
        </div>
      </footer>
    </div>
  );
}
