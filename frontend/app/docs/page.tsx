"use client";

import { useEffect } from "react";
import { useRouter } from "next/navigation";

export default function DocsPage() {
  const router = useRouter();

  useEffect(() => {
    router.replace("/docs/introduction");
  }, [router]);

  return (
    <div className="flex items-center justify-center min-h-[50vh]">
      <div className="animate-pulse text-muted-foreground font-medium">Loading documentation...</div>
    </div>
  );
}
