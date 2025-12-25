"use client";

import { useState } from "react";
import { ProjectFile } from "@/lib/types";
import { Button } from "./ui/button";
import { cn } from "@/lib/utils";
import {
  ChevronRight,
  ChevronDown,
  Folder,
  FolderOpen,
  FileCode,
  FileText,
  FileJson,
  File,
} from "lucide-react";

interface FileTreeProps {
  files: ProjectFile[];
  onFileEdit?: (path: string) => void;
}

interface TreeItemProps {
  item: ProjectFile;
  level: number;
  onFileEdit?: (path: string) => void;
}

const getFileIcon = (name: string) => {
  const ext = name.split(".").pop()?.toLowerCase();
  switch (ext) {
    case "py":
    case "js":
    case "ts":
    case "tsx":
    case "jsx":
      return <FileCode className="h-4 w-4 text-violet-500" />;
    case "json":
      return <FileJson className="h-4 w-4 text-amber-500" />;
    case "md":
    case "txt":
      return <FileText className="h-4 w-4 text-blue-500" />;
    case "yml":
    case "yaml":
      return <FileCode className="h-4 w-4 text-pink-500" />;
    default:
      if (name === "Dockerfile") {
        return <FileCode className="h-4 w-4 text-cyan-500" />;
      }
      return <File className="h-4 w-4 text-muted-foreground" />;
  }
};

const formatSize = (bytes: number) => {
  if (bytes < 1024) return `${bytes} B`;
  if (bytes < 1024 * 1024) return `${(bytes / 1024).toFixed(1)} KB`;
  return `${(bytes / (1024 * 1024)).toFixed(1)} MB`;
};

function TreeItem({ item, level, onFileEdit }: TreeItemProps) {
  const [isOpen, setIsOpen] = useState(level < 2);

  if (item.type === "folder") {
    return (
      <div>
        <button
          onClick={() => setIsOpen(!isOpen)}
          className={cn(
            "flex items-center gap-2 w-full px-3 py-2 hover:bg-secondary/50 transition-colors text-left",
            "text-sm"
          )}
          style={{ paddingLeft: `${level * 16 + 12}px` }}
        >
          {isOpen ? (
            <ChevronDown className="h-4 w-4 text-muted-foreground shrink-0" />
          ) : (
            <ChevronRight className="h-4 w-4 text-muted-foreground shrink-0" />
          )}
          {isOpen ? (
            <FolderOpen className="h-4 w-4 text-amber-500 shrink-0" />
          ) : (
            <Folder className="h-4 w-4 text-amber-500 shrink-0" />
          )}
          <span className="font-medium truncate">{item.name}</span>
          <span className="text-xs text-muted-foreground ml-auto">
            {item.children?.length || 0} items
          </span>
        </button>
        {isOpen && item.children && (
          <div>
            {item.children.map((child) => (
              <TreeItem
                key={child.path}
                item={child}
                level={level + 1}
                onFileEdit={onFileEdit}
              />
            ))}
          </div>
        )}
      </div>
    );
  }

  return (
    <div
      className={cn(
        "flex items-center gap-2 px-3 py-2 hover:bg-secondary/50 transition-colors",
        "text-sm group"
      )}
      style={{ paddingLeft: `${level * 16 + 36}px` }}
    >
      {getFileIcon(item.name)}
      <span className="font-mono truncate">{item.name}</span>
      <span className="text-xs text-muted-foreground">
        {item.size !== undefined && formatSize(item.size)}
      </span>
      {item.editable && onFileEdit && (
        <Button
          size="sm"
          variant="ghost"
          className="ml-auto opacity-0 group-hover:opacity-100 transition-opacity h-7"
          onClick={() => onFileEdit(item.path)}
        >
          Edit
        </Button>
      )}
    </div>
  );
}

export function FileTree({ files, onFileEdit }: FileTreeProps) {
  const countFiles = (items: ProjectFile[]): number => {
    return items.reduce((count, item) => {
      if (item.type === "file") return count + 1;
      return count + (item.children ? countFiles(item.children) : 0);
    }, 0);
  };

  if (files.length === 0) {
    return (
      <div className="text-muted-foreground text-sm py-12 text-center">
        No files uploaded
      </div>
    );
  }

  return (
    <div className="divide-y divide-border">
      <div className="px-4 py-3 bg-secondary/30 text-sm text-muted-foreground">
        {countFiles(files)} files in {files.filter((f) => f.type === "folder").length} folders
      </div>
      <div className="max-h-[500px] overflow-y-auto">
        {files.map((item) => (
          <TreeItem key={item.path} item={item} level={0} onFileEdit={onFileEdit} />
        ))}
      </div>
    </div>
  );
}
