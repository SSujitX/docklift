"use client";

import { useState, useCallback } from "react";
import Editor from "@monaco-editor/react";
import { Button } from "./ui/button";
import { API_URL } from "@/lib/utils";
import { Save, Loader2, X, Code2, Sparkles } from "lucide-react";

interface FileEditorProps {
  projectId: string;
  filename: string;
  content: string;
  onClose: () => void;
  onSave: () => void;
}

export function FileEditor({ projectId, filename, content, onClose, onSave }: FileEditorProps) {
  const [value, setValue] = useState(content);
  const [saving, setSaving] = useState(false);

  const handleSave = async () => {
    setSaving(true);
    try {
      await fetch(`${API_URL}/api/files/${projectId}/content?path=${encodeURIComponent(filename)}`, {
        method: "PUT",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ filename, content: value }),
      });
      onSave();
    } catch (error) {
      console.error(error);
    } finally {
      setSaving(false);
    }
  };

  const getLanguage = (name: string) => {
    const ext = name.split(".").pop()?.toLowerCase();
    switch (ext) {
      case "js":
      case "jsx":
        return "javascript";
      case "ts":
      case "tsx":
        return "typescript";
      case "py":
        return "python";
      case "json":
        return "json";
      case "html":
        return "html";
      case "css":
        return "css";
      case "md":
        return "markdown";
      case "yml":
      case "yaml":
        return "yaml";
      case "sh":
        return "shell";
      case "dockerfile":
        return "dockerfile";
      default:
        if (name === "Dockerfile") return "dockerfile";
        return "plaintext";
    }
  };

  return (
    <div className="fixed inset-0 z-[100] flex items-center justify-center bg-black/80 backdrop-blur-md p-4 md:p-10 animate-in fade-in duration-300">
      <div className="w-full max-w-6xl h-full flex flex-col bg-[#09090b] border border-white/10 rounded-3xl shadow-[0_0_50px_-12px_rgba(0,0,0,0.5)] overflow-hidden">
        {/* Editor Header */}
        <div className="flex items-center justify-between px-6 py-4 border-b border-white/5 bg-zinc-900/40 backdrop-blur-xl">
          <div className="flex items-center gap-4">
            <div className="bg-violet-500/10 p-2 rounded-xl">
              <Code2 className="h-5 w-5 text-violet-500" />
            </div>
            <div>
              <div className="flex items-center gap-2">
                <span className="text-sm font-bold tracking-tight text-zinc-100">{filename.split('/').pop()}</span>
                <span className="text-[10px] uppercase font-bold tracking-widest text-zinc-500 bg-white/5 px-2 py-0.5 rounded-full border border-white/5 shrink-0">
                  {getLanguage(filename)}
                </span>
              </div>
              <p className="text-[10px] text-zinc-500 font-mono mt-0.5">{filename}</p>
            </div>
          </div>

          <div className="flex items-center gap-3">
            <div className="hidden md:flex items-center gap-1.5 px-3 py-1.5 rounded-xl bg-emerald-500/5 border border-emerald-500/10 mr-2">
              <Sparkles className="h-3.5 w-3.5 text-emerald-500" />
              <span className="text-[10px] font-bold text-emerald-500 uppercase tracking-widest">Autosave Ready</span>
            </div>
            <Button 
              onClick={handleSave} 
              disabled={saving}
              className="h-10 px-6 bg-gradient-to-r from-violet-600 to-indigo-600 hover:from-violet-700 hover:to-indigo-700 text-white border-none shadow-lg shadow-violet-500/20 rounded-xl transition-all active:scale-95"
            >
              {saving ? <Loader2 className="h-4 w-4 animate-spin mr-2" /> : <Save className="h-4 w-4 mr-2" />}
              <span className="font-bold">Save Changes</span>
            </Button>
            <div className="w-px h-6 bg-white/10 mx-1" />
            <Button 
              variant="ghost" 
              size="icon" 
              onClick={onClose}
              className="h-10 w-10 rounded-xl hover:bg-white/5 text-zinc-400 hover:text-white transition-colors"
            >
              <X className="h-5 w-5" />
            </Button>
          </div>
        </div>

        {/* Editor Instance */}
        <div className="flex-1 bg-[#09090b]">
          <Editor
            height="100%"
            language={getLanguage(filename)}
            theme="vs-dark"
            value={value}
            onChange={(val) => setValue(val || "")}
            loading={
              <div className="flex flex-col items-center justify-center h-full gap-4 text-zinc-500 animate-pulse">
                <Loader2 className="h-8 w-8 animate-spin" strokeWidth={1.5} />
                <span className="text-sm font-medium tracking-tight">Initializing Editor...</span>
              </div>
            }
            options={{
              fontSize: 14,
              fontFamily: 'var(--font-mono), ui-monospace, SFMono-Regular, Menlo, Monaco, Consolas, "Liberation Mono", "Courier New", monospace',
              minimap: { enabled: true },
              scrollBeyondLastLine: false,
              padding: { top: 20 },
              smoothScrolling: true,
              cursorBlinking: "smooth",
              cursorSmoothCaretAnimation: "on",
              formatOnPaste: true,
              formatOnType: true,
              automaticLayout: true,
              tabSize: 2,
              wordWrap: "on",
              lineNumbersMinChars: 3,
              bracketPairColorization: { enabled: true },
              guides: { indentation: true },
            }}
          />
        </div>
        
        {/* Footer info */}
        <div className="px-6 py-2 border-t border-white/5 bg-zinc-950 flex items-center justify-between text-[11px] text-zinc-500">
          <div className="flex items-center gap-4">
            <span className="flex items-center gap-1.5 font-medium">
              <span className="w-1.5 h-1.5 rounded-full bg-violet-500" />
              UTF-8
            </span>
            <span className="text-zinc-700 select-none">|</span>
            <span className="font-medium uppercase tracking-tight">{getLanguage(filename)}</span>
          </div>
          <div className="flex items-center gap-4 text-zinc-600 italic">
            Press Cmd/Ctrl + S to save manually
          </div>
        </div>
      </div>
    </div>
  );
}
