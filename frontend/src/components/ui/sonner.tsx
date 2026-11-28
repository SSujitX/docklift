"use client";

import type { CSSProperties } from "react";
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
 * Neutral popover surface for every type; the status colour is on the icon only
 * (see [data-sonner-toast][data-type] in globals.css).
 */
const Toaster = ({ ...props }: ToasterProps) => {
  const { theme = "system" } = useTheme();

  return (
    <Sonner
      theme={theme as ToasterProps["theme"]}
      position="top-center"
      expand
      closeButton
      duration={5200}
      gap={8}
      offset={16}
      visibleToasts={4}
      className="toaster group"
      style={
        {
          "--normal-bg": "hsl(var(--popover))",
          "--normal-text": "hsl(var(--popover-foreground))",
          "--normal-border": "hsl(var(--border))",
          "--border-radius": "8px",
        } as CSSProperties
      }
      icons={{
        success: <CheckCircle2 className="size-4 shrink-0" />,
        error: <CircleAlert className="size-4 shrink-0" />,
        warning: <TriangleAlert className="size-4 shrink-0" />,
        info: <Info className="size-4 shrink-0" />,
        loading: <Loader2 className="size-4 shrink-0 animate-spin" />,
        close: <X className="size-3.5" />,
      }}
      toastOptions={{
        classNames: {
          toast:
            "group toast docklift-toast group-[.toaster]:pointer-events-auto " +
            "group-[.toaster]:w-[min(100vw-1.5rem,26rem)]! " +
            "group-[.toaster]:rounded-lg! group-[.toaster]:border! group-[.toaster]:border-border! " +
            "group-[.toaster]:bg-popover! group-[.toaster]:text-popover-foreground! " +
            "group-[.toaster]:px-3.5! group-[.toaster]:py-3! group-[.toaster]:gap-2.5! " +
            "group-[.toaster]:font-sans group-[.toaster]:text-[13.5px] " +
            "group-[.toaster]:shadow-popover!",
          title: "group-[.toast]:text-[13.5px] group-[.toast]:font-medium group-[.toast]:leading-5",
          description:
            "group-[.toast]:text-[13px] group-[.toast]:leading-5 group-[.toast]:text-muted-foreground!",
          actionButton:
            "group-[.toast]:h-7! group-[.toast]:rounded-md! group-[.toast]:bg-brand! group-[.toast]:px-2.5! " +
            "group-[.toast]:text-[13px]! group-[.toast]:font-medium! group-[.toast]:text-brand-foreground!",
          cancelButton:
            "group-[.toast]:h-7! group-[.toast]:rounded-md! group-[.toast]:border group-[.toast]:border-input " +
            "group-[.toast]:bg-card! group-[.toast]:px-2.5! group-[.toast]:text-[13px]! " +
            "group-[.toast]:font-medium! group-[.toast]:text-foreground!",
          closeButton:
            "group-[.toast]:border-border! group-[.toast]:bg-popover! " +
            "group-[.toast]:text-subtle! hover:group-[.toast]:text-foreground! " +
            "hover:group-[.toast]:bg-secondary!",
          icon: "group-[.toast]:mt-0.5 group-[.toast]:self-start",
        },
      }}
      {...props}
    />
  );
};

export { Toaster };
