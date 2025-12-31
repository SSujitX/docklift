"use client";

import { useState } from "react";
import { Copy, Check, Terminal, FileCode } from "lucide-react";
import { copyToClipboard, cn } from "@/lib/utils";

export const CopyButton = ({ text, className }: { text: string, className?: string }) => {
  const [copied, setCopied] = useState(false);
  const handleCopy = () => {
    copyToClipboard(text);
    setCopied(true);
    setTimeout(() => setCopied(false), 2000);
  };
  return (
    <button
      onClick={handleCopy}
      className={cn(
        "flex items-center gap-2 px-3 py-1.5 rounded-lg bg-white/5 hover:bg-white/10 text-zinc-400 hover:text-white transition-all border border-white/5 shrink-0 active:scale-95",
        className
      )}
    >
      {copied ? <Check className="h-3.5 w-3.5 text-emerald-500" /> : <Copy className="h-3.5 w-3.5" />}
      <span className="text-[10px] font-bold uppercase tracking-tight hidden sm:inline">
        {copied ? "Copied" : "Copy"}
      </span>
    </button>
  );
};

export const CommandBlock = ({ command, label, color = "cyan" }: { command: string, label?: string, color?: string }) => {
  return (
    <div className="group mb-4 last:mb-0">
      {label && <p className="text-xs font-bold text-muted-foreground/80 uppercase tracking-wider mb-2 ml-1 selection:bg-slate-700 selection:text-white"># {label}</p>}
      <div className="relative group/btn">
        <div className="flex items-center gap-3 bg-zinc-900 border border-white/5 rounded-xl px-4 py-2.5 group-hover/btn:border-white/10 transition-all shadow-inner">
          <Terminal className="h-4 w-4 text-zinc-600 shrink-0" />
          <div className="overflow-x-auto no-scrollbar flex-1 font-mono text-sm py-0.5">
            <code className={cn(
              "whitespace-nowrap",
              color === "cyan" ? "text-cyan-400" : 
              color === "red" ? "text-red-400" : 
              color === "emerald" ? "text-emerald-400" : 
              color === "amber" ? "text-amber-400" : 
              color === "violet" ? "text-violet-400" : "text-blue-400"
            )}>
              {command}
            </code>
          </div>
          <CopyButton text={command} />
        </div>
      </div>
    </div>
  );
};

export const TerminalWindow = ({ title, items, color = "cyan" }: { title: string, items: { cmd?: string, comment?: string }[], color?: string }) => {
  return (
    <div className="bg-zinc-950 rounded-2xl border border-white/5 overflow-hidden mb-8 shadow-2xl text-left">
      <div className="flex items-center justify-between px-5 py-3.5 bg-white/5 border-b border-white/5">
        <div className="flex items-center gap-4">
          <div className="flex gap-2">
            <div className="w-3 h-3 rounded-full bg-red-500/30 border border-red-500/20" />
            <div className="w-3 h-3 rounded-full bg-amber-500/30 border border-amber-500/20" />
            <div className="w-3 h-3 rounded-full bg-emerald-500/30 border border-emerald-500/20" />
          </div>
          <div className="h-4 w-[1px] bg-white/10 mx-1" />
          <span className="text-[11px] font-black text-zinc-500 uppercase tracking-[0.2em]">{title}</span>
        </div>
      </div>
      <div className="p-6 space-y-4">
        {items.map((item, i) => (
          <div key={i} className="group/item">
            {item.comment && (
              <p className="text-xs text-zinc-500 font-medium italic mb-2 ml-1"># {item.comment}</p>
            )}
            {item.cmd && (
              <div className="flex items-center justify-between gap-4 bg-white/5 rounded-xl px-4 py-2.5 border border-white/5 group-hover/item:border-white/10 transition-all">
                <div className="flex items-center gap-3 min-w-0 flex-1">
                  <span className="text-zinc-600 text-xs font-mono select-none">$</span>
                  <div className="overflow-x-auto no-scrollbar font-mono text-sm py-0.5 w-full">
                    <code className={cn(
                      "whitespace-nowrap",
                      color === "cyan" ? "text-cyan-400" : 
                      color === "emerald" ? "text-emerald-400" : 
                      color === "amber" ? "text-amber-400" : "text-zinc-300"
                    )}>
                      {item.cmd}
                    </code>
                  </div>
                </div>
                <CopyButton text={item.cmd} />
              </div>
            )}
          </div>
        ))}
      </div>
    </div>
  );
};

export const StaticCodeBlock = ({ code, icon: Icon = FileCode, title, color = "cyan" }: { code: string, icon?: any, title?: string, color?: string }) => {
  return (
    <div className="relative bg-zinc-950 rounded-2xl border border-white/5 overflow-hidden mb-6 group text-left">
      <div className="flex items-center justify-between px-5 py-3.5 bg-white/5 border-b border-white/5">
        <div className="flex items-center gap-3">
          <Icon className={cn("h-4 w-4", color === "cyan" ? "text-cyan-500" : "text-zinc-500")} />
          <span className="text-[11px] font-bold text-zinc-500 uppercase tracking-wider">{title || "Code Example"}</span>
        </div>
        <CopyButton text={code} className="bg-transparent border-transparent hover:bg-white/5" />
      </div>
      <div className="p-6 overflow-x-auto no-scrollbar text-sm text-zinc-300 font-mono leading-relaxed">
        <pre>{code}</pre>
      </div>
    </div>
  );
};
