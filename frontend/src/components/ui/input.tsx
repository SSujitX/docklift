import * as React from "react";
import { cn } from "@/lib/utils";

/** 36px text input (40px on touch). Native <select> should use the same classes. */
const Input = React.forwardRef<HTMLInputElement, React.ComponentProps<"input">>(
  ({ className, type, ...props }, ref) => {
    return (
      <input
        type={type}
        className={cn(
          "flex h-9 w-full min-w-0 rounded-md border border-input bg-card px-3 py-1 text-sm text-foreground transition-[border-color,box-shadow] duration-150 pointer-coarse:h-10",
          "file:mr-3 file:border-0 file:bg-transparent file:text-sm file:font-medium file:text-foreground",
          "placeholder:text-subtle",
          "focus:border-brand focus:outline-hidden focus:ring-3 focus:ring-brand/20",
          "aria-invalid:border-danger aria-invalid:focus:ring-danger/20",
          "disabled:cursor-not-allowed disabled:opacity-50",
          className
        )}
        ref={ref}
        {...props}
      />
    );
  }
);
Input.displayName = "Input";

export { Input };
