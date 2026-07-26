"use client";

import { useTheme } from "@/lib/theme";
import {
  CheckCircle2,
  CircleAlert,
  Info,
  Loader2,
  TriangleAlert,
  X,
} from "lucide-react";
import { Toaster as Sonner } from "sonner";

type ToasterProps = React.ComponentProps<typeof Sonner>;

/**
 * Global notifications — top-center so they stay visible above sticky footers
 * and work from any page (create, deploy, errors, copy, etc.).
 */
const Toaster = ({ ...props }: ToasterProps) => {
  const { theme = "system" } = useTheme();

  return (
    <Sonner
      theme={theme as ToasterProps["theme"]}
      position="top-center"
      expand
      richColors
      closeButton
      duration={5200}
      gap={10}
      offset={16}
      visibleToasts={4}
      className="toaster group"
      icons={{
        success: <CheckCircle2 className="h-5 w-5 shrink-0" strokeWidth={2.25} />,
        error: <CircleAlert className="h-5 w-5 shrink-0" strokeWidth={2.25} />,
        warning: <TriangleAlert className="h-5 w-5 shrink-0" strokeWidth={2.25} />,
        info: <Info className="h-5 w-5 shrink-0" strokeWidth={2.25} />,
        loading: <Loader2 className="h-5 w-5 shrink-0 animate-spin" strokeWidth={2.25} />,
        close: <X className="h-3.5 w-3.5" strokeWidth={2.25} />,
      }}
      toastOptions={{
        classNames: {
          toast:
            "group toast docklift-toast group-[.toaster]:pointer-events-auto " +
            "group-[.toaster]:w-[min(100vw-1.5rem,26rem)] " +
            "group-[.toaster]:rounded-2xl group-[.toaster]:border group-[.toaster]:p-4 " +
            "group-[.toaster]:gap-3 group-[.toaster]:font-sans " +
            "group-[.toaster]:bg-background group-[.toaster]:text-foreground " +
            "group-[.toaster]:border-border/70 " +
            "group-[.toaster]:shadow-[0_12px_40px_-12px_rgba(0,0,0,0.35)] " +
            "group-[.toaster]:ring-1 group-[.toaster]:ring-black/5 " +
            "dark:group-[.toaster]:ring-white/10",
          success:
            "group-[.toaster]:!bg-emerald-50 group-[.toaster]:dark:!bg-emerald-950/90 " +
            "group-[.toaster]:!border-emerald-500/40 group-[.toaster]:!text-emerald-900 " +
            "group-[.toaster]:dark:!text-emerald-50 " +
            "group-[.toaster]:!shadow-[0_12px_36px_-10px_rgba(16,185,129,0.45)]",
          error:
            "group-[.toaster]:!bg-red-50 group-[.toaster]:dark:!bg-red-950/90 " +
            "group-[.toaster]:!border-red-500/45 group-[.toaster]:!text-red-950 " +
            "group-[.toaster]:dark:!text-red-50 " +
            "group-[.toaster]:!shadow-[0_12px_36px_-10px_rgba(239,68,68,0.45)]",
          warning:
            "group-[.toaster]:!bg-amber-50 group-[.toaster]:dark:!bg-amber-950/90 " +
            "group-[.toaster]:!border-amber-500/45 group-[.toaster]:!text-amber-950 " +
            "group-[.toaster]:dark:!text-amber-50 " +
            "group-[.toaster]:!shadow-[0_12px_36px_-10px_rgba(245,158,11,0.4)]",
          info:
            "group-[.toaster]:!bg-sky-50 group-[.toaster]:dark:!bg-sky-950/90 " +
            "group-[.toaster]:!border-sky-500/40 group-[.toaster]:!text-sky-950 " +
            "group-[.toaster]:dark:!text-sky-50 " +
            "group-[.toaster]:!shadow-[0_12px_36px_-10px_rgba(14,165,233,0.4)]",
          loading:
            "group-[.toaster]:!bg-secondary/95 group-[.toaster]:!border-brand/35 " +
            "group-[.toaster]:!text-foreground",
          title:
            "group-[.toast]:text-[13px] group-[.toast]:font-semibold group-[.toast]:leading-snug " +
            "group-[.toast]:tracking-tight",
          description:
            "group-[.toast]:text-xs group-[.toast]:leading-relaxed group-[.toast]:opacity-90",
          actionButton:
            "group-[.toast]:rounded-lg group-[.toast]:bg-foreground group-[.toast]:px-3 " +
            "group-[.toast]:text-xs group-[.toast]:font-semibold group-[.toast]:text-background",
          cancelButton:
            "group-[.toast]:rounded-lg group-[.toast]:bg-secondary group-[.toast]:px-3 " +
            "group-[.toast]:text-xs group-[.toast]:font-medium group-[.toast]:text-muted-foreground",
          closeButton:
            "group-[.toast]:border-border/60 group-[.toast]:bg-background/90 " +
            "group-[.toast]:text-muted-foreground group-[.toast]:hover:text-foreground " +
            "group-[.toast]:hover:bg-secondary",
          icon: "group-[.toast]:mt-0.5",
        },
      }}
      {...props}
    />
  );
};

export { Toaster };
