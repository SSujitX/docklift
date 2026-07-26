// DNS setup reference: A-record target, common record shapes, short Cloudflare notes.
import { Check, Copy, Globe2 } from "lucide-react";
import { useState } from "react";
import { Card } from "@/components/ui/card";
import { copyToClipboard } from "@/lib/utils";
import { toast } from "sonner";

const RECORDS = [
  { label: "Root", host: "example.com", type: "A", name: "@" },
  { label: "Subdomain", host: "app.example.com", type: "A", name: "app" },
  { label: "WWW", host: "www.example.com", type: "CNAME", name: "www" },
] as const;

const TIPS = [
  "Cloudflare SSL: Full (strict) after HTTPS works — never Flexible.",
  "Orange cloud: allow /.well-known/acme-challenge/ over plain HTTP.",
  "Open ports 80 and 443 on the server firewall.",
  "Add www only when its DNS record exists — one miss fails the cert.",
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
    <Card className="overflow-hidden border-border/50">
      {/* Highlighted Point DNS header */}
      <div className="border-b border-brand/15 bg-gradient-to-br from-brand/10 via-brand/5 to-transparent px-4 py-4 sm:px-6 sm:py-5">
        <div className="flex flex-wrap items-start justify-between gap-3">
          <div className="min-w-0">
            <h3 className="flex items-center gap-2 text-base font-bold tracking-tight sm:text-lg">
              <span className="flex h-8 w-8 items-center justify-center rounded-xl border border-brand/20 bg-brand/15">
                <Globe2 className="h-4 w-4 text-brand" />
              </span>
              Point DNS at this server
            </h3>
            <p className="mt-1.5 max-w-xl text-xs leading-relaxed text-muted-foreground sm:text-sm">
              Create the DNS record first. Let&apos;s Encrypt checks HTTP before
              issuing a certificate.
            </p>
          </div>

          <button
            type="button"
            onClick={handleCopy}
            disabled={!hasIP}
            className="group flex items-center gap-3 rounded-xl border border-brand/25 bg-background/80 px-3 py-2 text-left shadow-sm transition-colors hover:border-brand/50 disabled:cursor-default disabled:opacity-70"
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
      </div>

      <div className="space-y-4 p-4 sm:p-6">
        <div className="grid gap-2 sm:grid-cols-3">
          {RECORDS.map((record) => (
            <div
              key={record.label}
              className="min-w-0 rounded-xl border border-border/50 bg-secondary/20 p-3"
            >
              <p className="text-[10px] font-bold uppercase tracking-widest text-muted-foreground">
                {record.label}
              </p>
              <p className="mt-1 truncate font-mono text-xs text-foreground/90">
                {record.host}
              </p>
              <div className="mt-2 flex flex-wrap items-center gap-1 font-mono text-[10px]">
                <span className="rounded border border-border/50 bg-background px-1.5 py-0.5">
                  {record.type}
                </span>
                <span className="rounded border border-border/50 bg-background px-1.5 py-0.5">
                  {record.name}
                </span>
                <span className="truncate rounded border border-border/50 bg-background px-1.5 py-0.5">
                  {record.type === "CNAME" ? "example.com" : serverIP || "server IP"}
                </span>
              </div>
            </div>
          ))}
        </div>

        <ul className="space-y-1.5 rounded-xl border border-border/40 bg-secondary/15 px-3 py-3 text-xs leading-relaxed text-muted-foreground sm:text-[13px]">
          {TIPS.map((tip) => (
            <li key={tip} className="flex gap-2">
              <span className="mt-1.5 h-1 w-1 shrink-0 rounded-full bg-brand/70" />
              <span>{tip}</span>
            </li>
          ))}
        </ul>
      </div>
    </Card>
  );
}
