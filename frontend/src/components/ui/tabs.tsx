import * as React from "react";
import * as TabsPrimitive from "@radix-ui/react-tabs";
import { cn } from "@/lib/utils";

const Tabs = TabsPrimitive.Root;

/** Underline tabs: left aligned, full-width hairline, scrolls sideways when it overflows. */
const TabsList = React.forwardRef<
  React.ElementRef<typeof TabsPrimitive.List>,
  React.ComponentPropsWithoutRef<typeof TabsPrimitive.List>
>(({ className, ...props }, ref) => (
  <TabsPrimitive.List
    ref={ref}
    className={cn(
      "hairline-b scrollbar-none flex w-full items-stretch justify-start gap-5 overflow-x-auto text-muted-foreground",
      className
    )}
    {...props}
  />
));
TabsList.displayName = TabsPrimitive.List.displayName;

const TabsTrigger = React.forwardRef<
  React.ElementRef<typeof TabsPrimitive.Trigger>,
  React.ComponentPropsWithoutRef<typeof TabsPrimitive.Trigger>
>(({ className, ...props }, ref) => (
  <TabsPrimitive.Trigger
    ref={ref}
    className={cn(
      "inline-flex h-10 shrink-0 items-center gap-2 whitespace-nowrap border-b-2 border-transparent px-0.5 text-sm font-medium text-muted-foreground transition-colors duration-150",
      "hover:text-foreground",
      "focus-visible:outline-2 focus-visible:-outline-offset-2 focus-visible:outline-ring",
      "disabled:pointer-events-none disabled:opacity-50",
      "data-[state=active]:border-brand data-[state=active]:text-foreground",
      "[&_svg]:size-4 [&_svg]:shrink-0 [&_svg]:text-subtle data-[state=active]:[&_svg]:text-foreground",
      className
    )}
    {...props}
  />
));
TabsTrigger.displayName = TabsPrimitive.Trigger.displayName;

const TabsContent = React.forwardRef<
  React.ElementRef<typeof TabsPrimitive.Content>,
  React.ComponentPropsWithoutRef<typeof TabsPrimitive.Content>
>(({ className, ...props }, ref) => (
  <TabsPrimitive.Content
    ref={ref}
    className={cn(
      "mt-5 rounded-sm focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-ring data-[state=active]:animate-in data-[state=active]:fade-in-0 data-[state=active]:duration-150",
      className
    )}
    {...props}
  />
));
TabsContent.displayName = TabsPrimitive.Content.displayName;

export { Tabs, TabsList, TabsTrigger, TabsContent };
