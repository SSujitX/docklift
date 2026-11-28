import { Loader2, RefreshCw } from "lucide-react";
import { Button } from "@/components/ui/button";
import { cn } from "@/lib/utils";

export type SslInfo = {
  status: "missing" | "pending" | "active" | "expiring" | "expired" | "failed";
  expiresAt?: string | null;
  error?: string | null;
  diagnosticCommand?: string | null;
};

// Dot colour carries the state; the label stays neutral.
const DOTS: Record<SslInfo["status"], string> = {
  active: "bg-ok",
  expiring: "bg-warn",
  expired: "bg-danger",
  failed: "bg-danger",
  pending: "bg-info animate-pulse motion-reduce:animate-none",
  missing: "bg-subtle",
};

const LABELS: Record<SslInfo["status"], string> = {
  active: "HTTPS active",
  expiring: "Expiring soon",
  expired: "Expired",
  failed: "SSL failed",
  pending: "Issuing…",
  missing: "No cert",
};

export function SslStatusBadge({
  ssl,
  onRetry,
  retrying,
}: {
  ssl?: SslInfo | null;
  onRetry?: () => void;
  retrying?: boolean;
}) {
  const status = ssl?.status || "missing";
  const expiry =
    ssl?.expiresAt && (status === "active" || status === "expiring")
      ? new Date(ssl.expiresAt).toLocaleDateString()
      : null;

  return (
    <div className="flex min-w-0 flex-col gap-1">
      <div className="flex flex-wrap items-center gap-x-2 gap-y-1">
        <span
          className={cn(
            "inline-flex items-center gap-2 whitespace-nowrap text-[13px] font-medium leading-5",
            status === "missing" ? "text-muted-foreground" : "text-foreground",
          )}
        >
          <span aria-hidden="true" className={cn("size-2 shrink-0 rounded-full", DOTS[status])} />
          {LABELS[status]}
        </span>
        {expiry && (
          <span className="font-mono text-xs text-muted-foreground">until {expiry}</span>
        )}
        {(status === "failed" || status === "expired" || status === "missing") &&
          onRetry && (
            <Button
              type="button"
              variant="ghost"
              size="sm"
              className="px-2 text-muted-foreground"
              onClick={onRetry}
              disabled={retrying}
            >
              {retrying ? (
                <Loader2 className="size-3.5 animate-spin" />
              ) : (
                <RefreshCw className="size-3.5" />
              )}
              Retry SSL
            </Button>
          )}
      </div>
      {status === "failed" && ssl?.error && (
        <div className="space-y-1.5">
          <p className="wrap-break-word text-xs text-danger">{ssl.error}</p>
          {ssl.diagnosticCommand && (
            <div className="space-y-1">
              <p className="text-xs text-muted-foreground">Full error on server:</p>
              <code className="block max-w-full select-all overflow-x-auto rounded-sm bg-secondary/60 px-2 py-1.5 font-mono text-xs text-foreground">
                {ssl.diagnosticCommand}
              </code>
            </div>
          )}
        </div>
      )}
    </div>
  );
}
