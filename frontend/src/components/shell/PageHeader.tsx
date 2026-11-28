// Consistent page title block: title, one-line description, meta row and actions.
// `eyebrow` and `icon` are accepted for compatibility but intentionally not rendered.

import type { ReactNode } from "react";
import { cn } from "@/lib/utils";
import type { IconComponent } from "./navigation";

interface PageHeaderProps {
  title: string;
  description?: string;
  /** Accepted but not rendered. */
  eyebrow?: string;
  /** Accepted but not rendered. */
  icon?: IconComponent;
  actions?: ReactNode;
  /** Rendered under the title, e.g. a row of StatChip filter items. */
  meta?: ReactNode;
  className?: string;
}

export function PageHeader({
  title,
  description,
  actions,
  meta,
  className,
}: PageHeaderProps) {
  return (
    <div
      className={cn(
        "mb-6 flex flex-col gap-3 sm:flex-row sm:items-start sm:justify-between sm:gap-6",
        className,
      )}
    >
      <div className="min-w-0 flex-1">
        <h1 className="text-xl font-semibold leading-7 tracking-[-0.01em] text-foreground sm:truncate">
          {title}
        </h1>
        {description && (
          <p className="mt-0.5 max-w-2xl text-[13.5px] leading-5 text-muted-foreground">
            {description}
          </p>
        )}
        {meta && (
          <div className="mt-3 flex flex-wrap items-center gap-x-5 gap-y-2">{meta}</div>
        )}
      </div>

      {actions && (
        <div className="flex shrink-0 flex-wrap items-center gap-2 sm:justify-end">
          {actions}
        </div>
      )}
    </div>
  );
}

/**
 * Quiet "label count" text item, used as a filter/summary row under a page title.
 * Tone only colours the count when it carries state (success → ok, warning → warn).
 * `pulse` shows a pulsing dot for something in progress.
 */
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
  const countTone = {
    neutral: "text-subtle",
    success: "text-ok",
    warning: "text-warn",
    info: "text-subtle",
  } as const;

  const dotTone = {
    neutral: "bg-subtle",
    success: "bg-ok",
    warning: "bg-warn",
    info: "bg-info",
  } as const;

  return (
    <span className="inline-flex items-center gap-1.5 whitespace-nowrap text-[13.5px] leading-5 text-muted-foreground">
      {pulse && (
        <span
          aria-hidden="true"
          className={cn(
            "size-1.5 shrink-0 rounded-full animate-pulse motion-reduce:animate-none",
            dotTone[tone],
          )}
        />
      )}
      <span>{label}</span>
      <span className={cn("font-mono text-[13px] tabular-nums", countTone[tone])}>{value}</span>
    </span>
  );
}
