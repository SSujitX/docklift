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
            "group toast group-[.toaster]:bg-background group-[.toaster]:text-foreground group-[.toaster]:border-border group-[.toaster]:shadow-2xl group-[.toaster]:rounded-2xl group-[.toaster]:py-4 group-[.toaster]:px-5 group-[.toaster]:min-h-[60px] group-[.toaster]:backdrop-blur-xl",
          success:
            "group-[.toaster]:!bg-gradient-to-r group-[.toaster]:!from-emerald-500 group-[.toaster]:!via-green-500 group-[.toaster]:!to-teal-500 group-[.toaster]:!text-white group-[.toaster]:!border-emerald-400/50 group-[.toaster]:!shadow-[0_10px_40px_-10px_rgba(16,185,129,0.5)]",
          error:
            "group-[.toaster]:!bg-gradient-to-r group-[.toaster]:!from-rose-500 group-[.toaster]:!via-red-500 group-[.toaster]:!to-pink-500 group-[.toaster]:!text-white group-[.toaster]:!border-rose-400/50 group-[.toaster]:!shadow-[0_10px_40px_-10px_rgba(244,63,94,0.5)]",
          warning:
            "group-[.toaster]:!bg-gradient-to-r group-[.toaster]:!from-amber-500 group-[.toaster]:!via-orange-500 group-[.toaster]:!to-yellow-500 group-[.toaster]:!text-white group-[.toaster]:!border-amber-400/50 group-[.toaster]:!shadow-[0_10px_40px_-10px_rgba(245,158,11,0.5)]",
          info:
            "group-[.toaster]:!bg-gradient-to-r group-[.toaster]:!from-cyan-500 group-[.toaster]:!via-blue-500 group-[.toaster]:!to-indigo-500 group-[.toaster]:!text-white group-[.toaster]:!border-cyan-400/50 group-[.toaster]:!shadow-[0_10px_40px_-10px_rgba(6,182,212,0.5)]",
          title: "group-[.toast]:font-bold group-[.toast]:text-sm",
          description: "group-[.toast]:text-muted-foreground group-[.toast.success]:!text-white/90 group-[.toast.error]:!text-white/90 group-[.toast.warning]:!text-white/90 group-[.toast.info]:!text-white/90 group-[.toast]:text-xs",
          actionButton:
            "group-[.toast]:!bg-white/20 group-[.toast]:!text-white group-[.toast]:!font-bold group-[.toast]:!rounded-lg",
          cancelButton:
            "group-[.toast]:!bg-white/10 group-[.toast]:!text-white/80 group-[.toast]:!rounded-lg",
          closeButton:
            "group-[.toast]:!bg-white/20 group-[.toast]:!text-white group-[.toast]:!border-white/20 group-[.toast]:hover:!bg-white/30",
        },
      }}
      {...props}
    />
  )
}

export { Toaster }

