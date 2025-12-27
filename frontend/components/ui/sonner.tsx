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
      className="toaster group"
      toastOptions={{
        classNames: {
          toast:
            "group toast group-[.toaster]:bg-background group-[.toaster]:text-foreground group-[.toaster]:border-border group-[.toaster]:shadow-2xl group-[.toaster]:rounded-xl",
          success:
            "group-[.toaster]:!bg-gradient-to-r group-[.toaster]:!from-emerald-500 group-[.toaster]:!to-green-500 group-[.toaster]:!text-white group-[.toaster]:!border-emerald-600",
          error:
            "group-[.toaster]:!bg-gradient-to-r group-[.toaster]:!from-red-500 group-[.toaster]:!to-rose-500 group-[.toaster]:!text-white group-[.toaster]:!border-red-600",
          description: "group-[.toast]:text-muted-foreground group-[.toast.success]:text-white/90 group-[.toast.error]:text-white/90",
          actionButton:
            "group-[.toast]:bg-primary group-[.toast]:text-primary-foreground",
          cancelButton:
            "group-[.toast]:bg-muted group-[.toast]:text-muted-foreground",
        },
      }}
      {...props}
    />
  )
}

export { Toaster }
