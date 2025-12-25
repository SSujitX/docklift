"use client";

import { useState } from "react";
import { Button } from "./ui/button";
import { API_URL } from "@/lib/utils";
import { Save, Loader2, X } from "lucide-react";

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
      await fetch(`${API_URL}/api/files/${projectId}/${filename}`, {
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

  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/50 backdrop-blur-sm p-4">
      <div className="w-full max-w-4xl bg-card border border-border rounded-2xl shadow-2xl overflow-hidden">
        <div className="flex items-center justify-between px-5 py-4 border-b border-border bg-secondary/30">
          <code className="font-mono text-sm text-muted-foreground bg-secondary px-3 py-1 rounded-lg">{filename}</code>
          <div className="flex items-center gap-2">
            <Button size="sm" onClick={handleSave} disabled={saving}>
              {saving ? <Loader2 className="h-4 w-4 animate-spin" /> : <Save className="h-4 w-4" />}
              Save
            </Button>
            <Button size="sm" variant="ghost" onClick={onClose}>
              <X className="h-4 w-4" />
            </Button>
          </div>
        </div>
        <textarea
          value={value}
          onChange={(e) => setValue(e.target.value)}
          className="w-full h-[500px] bg-zinc-950 font-mono text-sm p-5 resize-none focus:outline-none text-zinc-300"
          spellCheck={false}
        />
      </div>
    </div>
  );
}
