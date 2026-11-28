// DNS setup reference: A-record target, common record shapes, short Cloudflare notes.
import { Check, Copy } from "lucide-react";
import { useState } from "react";
import { Button } from "@/components/ui/button";
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
    <Card>
      <div className="border-b border-border px-4 py-3 sm:px-5">
        <h3 className="text-[15px] font-semibold leading-5">Point DNS at this server</h3>
        <p className="mt-0.5 text-[13px] leading-5 text-muted-foreground">
          Create the DNS record first. Let&apos;s Encrypt checks HTTP before issuing a
          certificate.
        </p>
      </div>

      <div className="space-y-5 p-4 sm:p-5">
        <div className="flex flex-wrap items-center justify-between gap-x-4 gap-y-2">
          <div className="min-w-0">
            <p className="text-xs text-muted-foreground">A record target</p>
            <code className="mt-0.5 block font-mono text-[15px] font-medium text-foreground">
              {serverIP || "N/A"}
            </code>
          </div>
          <Button
            type="button"
            variant="outline"
            size="sm"
            onClick={handleCopy}
            disabled={!hasIP}
          >
            {copied ? <Check className="text-ok" /> : <Copy className="text-subtle" />}
            {copied ? "Copied" : "Copy IP"}
          </Button>
        </div>

        <div className="-mx-4 overflow-x-auto sm:-mx-5">
          <table className="w-full whitespace-nowrap text-left text-[13px]">
            <thead>
              <tr className="h-9 border-y border-border text-xs font-medium text-muted-foreground">
                <th className="pl-4 pr-2 font-medium sm:pl-5 sm:pr-3">Hostname</th>
                <th className="px-2 font-medium sm:px-3">Type</th>
                <th className="px-2 font-medium sm:px-3">Name</th>
                <th className="pl-2 pr-4 font-medium sm:pl-3 sm:pr-5">Value</th>
              </tr>
            </thead>
            <tbody className="font-mono text-xs">
              {RECORDS.map((record) => (
                <tr key={record.label} className="h-10 border-b border-border">
                  <td className="pl-4 pr-2 text-muted-foreground sm:pl-5 sm:pr-3">{record.host}</td>
                  <td className="px-2 text-foreground sm:px-3">{record.type}</td>
                  <td className="px-2 text-foreground sm:px-3">{record.name}</td>
                  <td className="pl-2 pr-4 text-foreground sm:pl-3 sm:pr-5">
                    {record.type === "CNAME" ? "example.com" : serverIP || "server IP"}
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>

        <ul className="space-y-1.5 text-[13px] leading-5 text-muted-foreground">
          {TIPS.map((tip) => (
            <li key={tip} className="flex gap-2.5">
              <span aria-hidden="true" className="mt-2 h-px w-2 shrink-0 bg-subtle" />
              <span>{tip}</span>
            </li>
          ))}
        </ul>
      </div>
    </Card>
  );
}
