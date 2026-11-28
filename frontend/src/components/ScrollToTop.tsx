import { useState, useEffect } from "react";
import { ChevronUp } from "lucide-react";
import { Button } from "@/components/ui/button";
import { cn } from "@/lib/utils";

export function ScrollToTop() {
  const [isVisible, setIsVisible] = useState(false);

  useEffect(() => {
    const toggleVisibility = () => {
      if (window.scrollY > 300) {
        setIsVisible(true);
      } else {
        setIsVisible(false);
      }
    };

    window.addEventListener("scroll", toggleVisibility);
    return () => window.removeEventListener("scroll", toggleVisibility);
  }, []);

  const scrollToTop = () => {
    window.scrollTo({
      top: 0,
      behavior: "smooth",
    });
  };

  return (
    <div
      className={cn(
        "fixed bottom-4 right-4 z-50 transition-[opacity,translate] duration-150 motion-reduce:transition-none sm:bottom-6 sm:right-6",
        isVisible ? "translate-y-0 opacity-100" : "pointer-events-none translate-y-1 opacity-0",
      )}
    >
      <Button
        onClick={scrollToTop}
        variant="outline"
        size="icon"
        title="Scroll to top"
        tabIndex={isVisible ? undefined : -1}
        className="size-9 text-muted-foreground shadow-popover hover:text-foreground pointer-coarse:size-10"
      >
        <ChevronUp className="size-4" />
      </Button>
    </div>
  );
}
