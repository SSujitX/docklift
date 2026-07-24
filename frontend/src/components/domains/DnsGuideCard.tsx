// DNS setup reference for custom domains: the A-record target plus the three
// record shapes people actually need, and the Cloudflare caveats behind a fold.
import { Check, ChevronDown, Copy, Globe2 } from "lucide-react";
import { useState } from "react";
import { Card } from "@/components/ui/card";
import { copyToClipboard } from "@/lib/utils";
import { toast } from "sonner";

const RECORDS = [
  { label: "Root domain", host: "example.com", type: "A", name: "@" },
  { label: "Subdomain", host: "app.example.com", type: "A", name: "app" },
  { label: "WWW", host: "www.example.com", type: "CNAME", name: "www" },
] as const;

export function DnsGuideCard({ serverIP }: { serverIP: string }) {
  const [copied, setCopied] = useState(false);
  const hasIP = Boolean(serverIP) && serverIP !== "N/A" && serverIP !== "...";

  const handleCopy = async () => {
    if (!hasIP) return;
    await copyToClipboard(serverIP);
    setCopied(true);
    toast.success("Server IP copied");
    setTimeout(() => setCopied(false), 1500);
  };

  return (
    <Card className="border-border/50 p-4 sm:p-6">
      <div className="flex flex-col gap-4 sm:gap-5">
        <div className="flex flex-wrap items-start justify-between gap-3">
          <div className="min-w-0">
            <h3 className="flex items-center gap-2 text-base font-bold sm:text-lg">
              <Globe2 className="h-4 w-4 text-brand sm:h-5 sm:w-5" />
              Point DNS at this server
            </h3>
            <p className="mt-1 text-xs text-muted-foreground sm:text-sm">
              Create the record before adding the domain — Let&apos;s Encrypt verifies over
              HTTP before it issues a certificate.
            </p>
          </div>

          <button
            type="button"
            onClick={handleCopy}
            disabled={!hasIP}
            className="group flex items-center gap-3 rounded-xl border border-border/60 bg-secondary/40 px-3 py-2 text-left transition-colors hover:border-brand/40 disabled:cursor-default disabled:opacity-70"
          >
            <span className="flex flex-col">
              <span className="text-[10px] font-bold uppercase tracking-widest text-muted-foreground">
                A record target
              </span>
              <code className="font-mono text-sm font-bold text-brand sm:text-base">
                {serverIP || "N/A"}
              </code>
            </span>
            {hasIP &&
              (copied ? (
                <Check className="h-4 w-4 text-emerald-500" />
              ) : (
                <Copy className="h-4 w-4 text-muted-foreground group-hover:text-foreground" />
              ))}
          </button>
        </div>

        <div className="grid gap-2 sm:grid-cols-3">
          {RECORDS.map((record) => (
            <div
              key={record.label}
              className="min-w-0 rounded-xl border border-border/50 bg-secondary/20 p-3"
            >
              <p className="text-[10px] font-bold uppercase tracking-widest text-muted-foreground">
                {record.label}
              </p>
              <p className="mt-1 truncate font-mono text-xs text-foreground/90">{record.host}</p>
              <div className="mt-2 flex flex-wrap items-center gap-1 text-[10px] font-mono">
                <span className="rounded bg-background px-1.5 py-0.5 border border-border/50">
                  {record.type}
                </span>
                <span className="rounded bg-background px-1.5 py-0.5 border border-border/50">
                  {record.name}
                </span>
                <span className="truncate rounded bg-background px-1.5 py-0.5 border border-border/50">
                  {record.type === "CNAME" ? "example.com" : serverIP || "server IP"}
                </span>
              </div>
            </div>
          ))}
        </div>

        <details className="group rounded-xl border border-border/50 bg-secondary/20 [&_summary::-webkit-details-marker]:hidden">
          <summary className="flex cursor-pointer items-center justify-between gap-2 px-3 py-2.5 text-xs font-semibold sm:text-sm">
            Cloudflare and propagation notes
            <ChevronDown className="h-4 w-4 shrink-0 text-muted-foreground transition-transform group-open:rotate-180" />
          </summary>
          <ul className="space-y-2 border-t border-border/40 px-3 py-3 text-xs text-muted-foreground">
            <li>
              Set Cloudflare SSL mode to <strong className="text-foreground">Full (strict)</strong>{" "}
              once HTTPS is active. Flexible causes redirect loops.
            </li>
            <li>
              With the orange cloud on, Cloudflare must still let{" "}
              <code className="rounded bg-background px-1 py-0.5">/.well-known/acme-challenge/</code>{" "}
              through over plain HTTP.
            </li>
            <li>
              Ports <strong className="text-foreground">80</strong> and{" "}
              <strong className="text-foreground">443</strong> must be open on the server firewall.
            </li>
            <li>
              Add <code className="rounded bg-background px-1 py-0.5">www</code> as a separate domain
              only when its DNS record exists — one missing record fails the whole certificate.
            </li>
          </ul>
        </details>
      </div>
    </Card>
  );
}
