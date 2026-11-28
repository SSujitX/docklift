// FileTree component - hierarchical file browser with folder expansion

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
  const cls = "size-4 shrink-0 text-subtle";
  switch (ext) {
    case "py":
    case "js":
    case "ts":
    case "tsx":
    case "jsx":
    case "yml":
    case "yaml":
      return <FileCode className={cls} />;
    case "json":
      return <FileJson className={cls} />;
    case "md":
    case "txt":
      return <FileText className={cls} />;
    default:
      if (name === "Dockerfile") {
        return <FileCode className={cls} />;
      }
      return <File className={cls} />;
  }
};

const formatSize = (bytes: number) => {
  if (bytes < 1024) return `${bytes} B`;
  if (bytes < 1024 * 1024) return `${(bytes / 1024).toFixed(1)} KB`;
  return `${(bytes / (1024 * 1024)).toFixed(1)} MB`;
};

function TreeItem({ item, level, onFileEdit }: TreeItemProps) {
  // Folders start collapsed — expand via the chevron
  const [isOpen, setIsOpen] = useState(false);

  if (item.type === "folder") {
    return (
      <div>
        <button
          onClick={() => setIsOpen(!isOpen)}
          className="flex h-9 w-full items-center gap-2 pr-4 text-left text-[13px] transition-colors duration-150 hover:bg-secondary/50 focus-visible:outline-2 focus-visible:-outline-offset-2 focus-visible:outline-ring pointer-coarse:h-10"
          style={{ paddingLeft: `${level * 16 + 12}px` }}
        >
          {isOpen ? (
            <ChevronDown className="size-4 shrink-0 text-subtle" />
          ) : (
            <ChevronRight className="size-4 shrink-0 text-subtle" />
          )}
          {isOpen ? (
            <FolderOpen className="size-4 shrink-0 text-subtle" />
          ) : (
            <Folder className="size-4 shrink-0 text-subtle" />
          )}
          <span className="truncate font-mono font-medium">{item.name}</span>
          <span className="ml-auto shrink-0 text-xs text-subtle">
            <span className="font-mono">{item.children?.length || 0}</span> items
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
        "group flex h-9 items-center gap-2 pr-2 text-[13px] transition-colors duration-150 hover:bg-secondary/50 pointer-coarse:h-10",
      )}
      style={{ paddingLeft: `${level * 16 + 36}px` }}
    >
      {getFileIcon(item.name)}
      <span className="min-w-0 truncate font-mono">{item.name}</span>
      <span className="shrink-0 font-mono text-xs text-subtle">
        {item.size !== undefined && formatSize(item.size)}
      </span>
      {item.editable && onFileEdit && (
        <Button
          size="sm"
          variant="ghost"
          className="ml-auto text-muted-foreground opacity-60 group-hover:opacity-100 focus-visible:opacity-100"
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
      <div className="py-12 text-center">
        <p className="text-sm font-medium">No files uploaded</p>
        <p className="mt-1 text-[13px] text-muted-foreground">
          Files appear here after the source is uploaded or cloned.
        </p>
      </div>
    );
  }

  return (
    <div className="divide-y divide-border">
      <div className="px-4 py-2 text-xs text-muted-foreground">
        <span className="font-mono">{countFiles(files)}</span> files in{" "}
        <span className="font-mono">{files.filter((f) => f.type === "folder").length}</span> folders
      </div>
      <div className="max-h-[500px] overflow-y-auto py-1">
        {files.map((item) => (
          <TreeItem key={item.path} item={item} level={0} onFileEdit={onFileEdit} />
        ))}
      </div>
    </div>
  );
}
