import * as React from "react";
import { Slot } from "@radix-ui/react-slot";
import { cva, type VariantProps } from "class-variance-authority";
import { cn } from "@/lib/utils";

/*
  Heights: sm 28px, default 32px, lg 36px, icon 32px square. On touch (pointer: coarse) default,
  lg and icon grow to 40px; sm keeps its 28px box but its hit area is extended to 40px.
  Icons inside default to 16px (see [data-slot='button'] in globals.css); a size utility on the
  icon overrides that.
*/
const buttonVariants = cva(
  "relative inline-flex items-center justify-center gap-2 whitespace-nowrap rounded-md text-sm font-medium transition-[color,background-color,border-color,opacity] duration-150 focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-ring disabled:pointer-events-none disabled:opacity-50",
  {
    variants: {
      variant: {
        default: "bg-brand text-brand-foreground hover:bg-brand/90",
        destructive:
          "bg-destructive text-destructive-foreground hover:bg-destructive/90",
        outline: "border border-input bg-card text-foreground hover:bg-secondary",
        secondary: "border border-input bg-card text-foreground hover:bg-secondary",
        ghost: "hover:bg-secondary hover:text-foreground",
        link: "text-brand underline-offset-4 hover:underline",
        success: "bg-brand text-brand-foreground hover:bg-brand/90",
        warning: "border border-input bg-card text-warn hover:bg-secondary",
        /** Solid surface styling comes entirely from className */
        bare: "bg-transparent shadow-none hover:bg-transparent",
      },
      size: {
        default: "h-8 px-3 pointer-coarse:h-10",
        sm: "h-7 gap-1.5 px-2.5 text-[13px] pointer-coarse:after:absolute pointer-coarse:after:inset-x-0 pointer-coarse:after:-inset-y-1.5",
        lg: "h-9 px-4 pointer-coarse:h-10",
        icon: "size-8 pointer-coarse:size-10",
      },
    },
    defaultVariants: {
      variant: "default",
      size: "default",
    },
  }
);

export interface ButtonProps
  extends React.ButtonHTMLAttributes<HTMLButtonElement>,
    VariantProps<typeof buttonVariants> {
  asChild?: boolean;
}

const Button = React.forwardRef<HTMLButtonElement, ButtonProps>(
  ({ className, variant, size, asChild = false, ...props }, ref) => {
    const Comp = asChild ? Slot : "button";
    return (
      <Comp
        data-slot="button"
        className={cn(buttonVariants({ variant, size }), className)}
        ref={ref}
        {...props}
      />
    );
  }
);
Button.displayName = "Button";

export { Button, buttonVariants };
