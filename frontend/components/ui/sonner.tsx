"use client"

import { useTheme } from "next-themes"
import { Toaster as Sonner } from "sonner"

type ToasterProps = React.ComponentProps<typeof Sonner>

const Toaster = ({ ...props }: ToasterProps) => {
  const { theme = "system" } = useTheme()

  return (
    <Sonner
      theme={theme as ToasterProps["theme"]}
      position="bottom-center"
      expand={true}
      richColors
      closeButton
      duration={4000}
      gap={12}
      offset={24}
      visibleToasts={5}
      className="toaster group"
      toastOptions={{
        classNames: {
          toast:
            "group toast group-[.toaster]:bg-background/95 group-[.toaster]:text-foreground group-[.toaster]:border-border/40 group-[.toaster]:shadow-2xl group-[.toaster]:shadow-black/20 group-[.toaster]:rounded-xl group-[.toaster]:p-4 group-[.toaster]:gap-3 group-[.toaster]:font-sans group-[.toaster]:backdrop-blur-xl transition-all",
          success:
            "group-[.toaster]:!bg-emerald-500/5 group-[.toaster]:dark:!bg-emerald-500/10 group-[.toaster]:!border-emerald-500/20 group-[.toaster]:!text-emerald-600 group-[.toaster]:dark:!text-emerald-400 group-[.toaster]:!shadow-[0_8px_30px_-8px_rgba(16,185,129,0.1)]",
          error:
            "group-[.toaster]:!bg-red-500/5 group-[.toaster]:dark:!bg-red-500/10 group-[.toaster]:!border-red-500/20 group-[.toaster]:!text-red-600 group-[.toaster]:dark:!text-red-400 group-[.toaster]:!shadow-[0_8px_30px_-8px_rgba(239,68,68,0.1)]",
          warning:
            "group-[.toaster]:!bg-amber-500/5 group-[.toaster]:dark:!bg-amber-500/10 group-[.toaster]:!border-amber-500/20 group-[.toaster]:!text-amber-600 group-[.toaster]:dark:!text-amber-400 group-[.toaster]:!shadow-[0_8px_30px_-8px_rgba(245,158,11,0.1)]",
          info:
            "group-[.toaster]:!bg-blue-500/5 group-[.toaster]:dark:!bg-blue-500/10 group-[.toaster]:!border-blue-500/20 group-[.toaster]:!text-blue-600 group-[.toaster]:dark:!text-blue-400 group-[.toaster]:!shadow-[0_8px_30px_-8px_rgba(59,130,246,0.1)]",
          title: "group-[.toast]:font-bold group-[.toast]:text-sm group-[.toast]:text-foreground",
          description: "group-[.toast]:text-muted-foreground group-[.toast]:text-xs group-[.toast]:opacity-90",
          actionButton:
            "group-[.toast]:bg-primary group-[.toast]:text-primary-foreground group-[.toast]:font-semibold group-[.toast]:rounded-lg group-[.toast]:text-xs",
          cancelButton:
            "group-[.toast]:bg-muted group-[.toast]:text-muted-foreground group-[.toast]:font-medium group-[.toast]:rounded-lg group-[.toast]:text-xs",
          closeButton:
            "group-[.toast]:bg-transparent group-[.toast]:text-muted-foreground group-[.toast]:hover:text-foreground group-[.toast]:hover:bg-accent group-[.toast]:transition-colors",
        },
      }}
      {...props}
    />
  )
}

export { Toaster }

