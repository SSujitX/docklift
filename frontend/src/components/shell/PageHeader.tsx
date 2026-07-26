// Consistent page title block: eyebrow, title, supporting copy and actions.

import type { ReactNode } from "react";
import { cn } from "@/lib/utils";
import type { IconComponent } from "./navigation";

interface PageHeaderProps {
  title: string;
  description?: string;
  eyebrow?: string;
  icon?: IconComponent;
  actions?: ReactNode;
  /** Chips rendered under the title, e.g. running/stopped counts. */
  meta?: ReactNode;
  className?: string;
}

export function PageHeader({
  title,
  description,
  eyebrow,
  icon: Icon,
  actions,
  meta,
  className,
}: PageHeaderProps) {
  return (
    <div
      className={cn(
        "mb-6 flex flex-col gap-4 sm:mb-8 sm:gap-5 lg:flex-row lg:items-start lg:justify-between",
        className,
      )}
    >
      <div className="flex min-w-0 items-start gap-3 sm:gap-4">
        {Icon && (
          <span className="hidden h-12 w-12 shrink-0 items-center justify-center rounded-2xl border border-brand/20 bg-brand/10 sm:flex">
            <Icon className="h-5.5 w-5.5 text-brand" />
          </span>
        )}
        <div className="min-w-0 flex-1">
          {eyebrow && (
            <p className="text-[10px] font-semibold uppercase tracking-[0.2em] text-brand">
              {eyebrow}
            </p>
          )}
          <h1 className="mt-1 text-xl font-bold tracking-tight sm:truncate sm:text-2xl md:text-3xl">
            {title}
          </h1>
          {description && (
            <p className="mt-1.5 max-w-2xl text-sm leading-relaxed text-muted-foreground">
              {description}
            </p>
          )}
          {meta && <div className="mt-3 flex flex-wrap gap-2 sm:mt-4">{meta}</div>}
        </div>
      </div>

      {actions && (
        <div className="flex w-full shrink-0 flex-wrap items-center gap-2 sm:w-auto">
          {actions}
        </div>
      )}
    </div>
  );
}

/** Small labelled statistic used in page headers. */
export function StatChip({
  label,
  value,
  tone = "neutral",
  pulse = false,
}: {
  label: string;
  value: ReactNode;
  tone?: "neutral" | "success" | "warning" | "info";
  pulse?: boolean;
}) {
  const tones = {
    neutral: "border-border/60 bg-secondary/50 text-foreground",
    success: "border-emerald-500/25 bg-emerald-500/10 text-emerald-600 dark:text-emerald-400",
    warning: "border-amber-500/25 bg-amber-500/10 text-amber-600 dark:text-amber-400",
    info: "border-brand/25 bg-brand/10 text-brand",
  } as const;

  return (
    <span
      className={cn(
        "inline-flex items-center gap-2 rounded-xl border px-3 py-1.5 text-sm font-medium",
        tones[tone],
        pulse && "animate-pulse",
      )}
    >
      <span className="text-xs uppercase tracking-wider opacity-70">{label}</span>
      <span className="font-bold">{value}</span>
    </span>
  );
}
