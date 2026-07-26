// Domain mapping for one service: add/remove hostnames, live Let's Encrypt
// activity, DNS verification and a concrete fix when issuance fails.
import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import {
  AlertTriangle,
  Check,
  Copy,
  ExternalLink,
  Globe,
  Loader2,
  Lock,
  Plus,
  RefreshCw,
  ScrollText,
  Search,
  Server,
  ShieldAlert,
  ShieldOff,
  Trash2,
} from "lucide-react";
import { toast } from "sonner";
import type { SslInfo } from "@/components/SslStatusBadge";
import { Button } from "@/components/ui/button";
import { Card } from "@/components/ui/card";
import { authFetch } from "@/lib/auth";
import {
  displayHostnameFromInput,
  dnsRecordHint,
  normalizeDomainInput,
} from "@/lib/domain";
import type { DomainDnsCheck, Service, SslEvent } from "@/lib/types";
import { API_URL, cn, copyToClipboard } from "@/lib/utils";
import { sslFixFor } from "./sslHelp";

type BusyAction = "add" | "remove" | "retry";

// A certificate that never finishes leaves the status at "pending" forever; stop
// auto-refreshing after this long so an abandoned tab does not poll all day.
const POLL_TIMEOUT_MS = 5 * 60 * 1000;

const STATUS_META: Record<
  SslInfo["status"],
  { label: string; className: string; icon: typeof Lock }
> = {
  active: {
    label: "HTTPS active",
    className: "bg-emerald-500/10 text-emerald-600 border-emerald-500/30",
    icon: Lock,
  },
  expiring: {
    label: "Renews soon",
    className: "bg-amber-500/10 text-amber-600 border-amber-500/30",
    icon: Lock,
  },
  expired: {
    label: "Certificate expired",
    className: "bg-red-500/10 text-red-600 border-red-500/30",
    icon: ShieldAlert,
  },
  failed: {
    label: "HTTPS setup failed",
    className: "bg-red-500/10 text-red-600 border-red-500/30",
    icon: ShieldAlert,
  },
  pending: {
    label: "Issuing certificate",
    className: "bg-brand/10 text-brand border-brand/30",
    icon: Loader2,
  },
  missing: {
    label: "HTTP only — no certificate yet",
    className: "bg-secondary text-muted-foreground border-border/60",
    icon: ShieldOff,
  },
};

const LEVEL_STYLES: Record<SslEvent["level"], string> = {
  info: "text-foreground/80",
  success: "text-emerald-600",
  warn: "text-amber-600",
  error: "text-red-500",
};

const DNS_STYLES: Record<DomainDnsCheck["status"], string> = {
  ok: "text-emerald-600",
  mismatch: "text-amber-600",
  missing: "text-red-500",
  unknown: "text-muted-foreground",
};

function splitDomains(value: string | null): string[] {
  return (value || "")
    .split(",")
    .map((d) => d.trim().toLowerCase())
    .filter(Boolean);
}

function SslPill({ ssl }: { ssl?: SslInfo | null }) {
  const status = ssl?.status || "missing";
  const meta = STATUS_META[status];
  const Icon = meta.icon;
  const expiry =
    ssl?.expiresAt && (status === "active" || status === "expiring")
      ? new Date(ssl.expiresAt).toLocaleDateString()
      : null;

  return (
    <span
      className={cn(
        "inline-flex items-center gap-1.5 rounded-md border px-2 py-0.5 text-[11px] font-medium",
        meta.className,
      )}
    >
      <Icon className={cn("h-3 w-3 shrink-0", status === "pending" && "animate-spin")} />
      {meta.label}
      {expiry && <span className="opacity-70">· until {expiry}</span>}
    </span>
  );
}

function CopyableCommand({ command }: { command: string }) {
  const [copied, setCopied] = useState(false);
  return (
    <div className="flex items-stretch gap-1">
      <code className="shell-scroll min-w-0 flex-1 overflow-x-auto whitespace-nowrap rounded bg-background px-2 py-1.5 text-[11px] select-all">
        {command}
      </code>
      <Button
        type="button"
        variant="ghost"
        size="icon"
        className="h-8 w-8 shrink-0"
        onClick={async () => {
          await copyToClipboard(command);
          setCopied(true);
          setTimeout(() => setCopied(false), 1500);
        }}
        aria-label="Copy command"
      >
        {copied ? (
          <Check className="h-3.5 w-3.5 text-emerald-500" />
        ) : (
          <Copy className="h-3.5 w-3.5" />
        )}
      </Button>
    </div>
  );
}

export function ServiceDomainCard({
  service,
  projectId,
  serverIP,
  onUpdate,
}: {
  service: Service;
  projectId: string;
  serverIP: string;
  onUpdate: () => void;
}) {
  const propDomains = useMemo(() => splitDomains(service.domain), [service.domain]);
  const [domains, setDomains] = useState<string[]>(propDomains);
  const domainsRef = useRef(propDomains);
  const mutationQueue = useRef(Promise.resolve());
  const [busy, setBusy] = useState<BusyAction | null>(null);

  useEffect(() => {
    domainsRef.current = domains;
  }, [domains]);

  useEffect(() => {
    if (busy === null) {
      setDomains(propDomains);
    }
  }, [propDomains, busy]);
  const [input, setInput] = useState("");
  const [sslMap, setSslMap] = useState<Record<string, SslInfo>>({});
  const [events, setEvents] = useState<SslEvent[]>([]);
  const [dnsChecks, setDnsChecks] = useState<Record<string, DomainDnsCheck>>({});
  const [checking, setChecking] = useState<string | null>(null);
  const [confirmRemove, setConfirmRemove] = useState<string | null>(null);
  const [pollExpired, setPollExpired] = useState(false);
  const logRef = useRef<HTMLDivElement>(null);

  const parsed = normalizeDomainInput(input);
  const isDuplicate = Boolean(parsed.value) && domains.includes(parsed.value);
  const serviceEndpoint = `${API_URL}/api/deployments/${projectId}/services/${service.id}`;

  // `force` keeps polling alive while a save is in flight: the first domain is already
  // written server-side, but this component's `service` prop has not refreshed yet.
  const fetchSsl = useCallback(async (force = false) => {
    if (!service.domain && !force) {
      setSslMap({});
      setEvents([]);
      return;
    }
    try {
      const res = await authFetch(`${serviceEndpoint}/ssl`);
      if (!res.ok) return;
      const data = await res.json();
      setSslMap(data.ssl || {});
      setEvents(data.events || []);
    } catch {
      /* transient — the next poll retries */
    }
  }, [serviceEndpoint, service.domain]);

  useEffect(() => {
    void fetchSsl();
  }, [fetchSsl]);

  // Issuance runs inside the save request, so keep pulling activity while it works.
  const activityRunning =
    busy !== null || Object.values(sslMap).some((s) => s?.status === "pending");

  useEffect(() => {
    if (!activityRunning) {
      setPollExpired(false);
      return;
    }
    if (pollExpired) return;

    const startedAt = Date.now();
    const interval = setInterval(() => {
      if (Date.now() - startedAt > POLL_TIMEOUT_MS) {
        setPollExpired(true);
        return;
      }
      void fetchSsl(true);
    }, 2000);
    return () => clearInterval(interval);
  }, [activityRunning, pollExpired, fetchSsl]);

  useEffect(() => {
    const el = logRef.current;
    if (el) el.scrollTop = el.scrollHeight;
  }, [events]);

  const persist = (next: string[], action: BusyAction): Promise<boolean> => {
    const run = async (): Promise<boolean> => {
      const previous = domainsRef.current;
      setDomains(next);
      setBusy(action);
      setPollExpired(false);
      try {
        const res = await authFetch(serviceEndpoint, {
          method: "PUT",
          headers: { "Content-Type": "application/json" },
          body: JSON.stringify({ domain: next.join(",") }),
        });
        const data = await res.json().catch(() => ({}));
        if (!res.ok) throw new Error(data.error || "Failed to save domains");

        if (typeof data.domain === "string") {
          setDomains(splitDomains(data.domain));
        }
        if (data.ssl) setSslMap(data.ssl);
        if (data.events) setEvents(data.events);
        onUpdate();
        return true;
      } catch (err: any) {
        setDomains(previous);
        toast.error(err?.message || "Failed to save domains");
        return false;
      } finally {
        setBusy(null);
        void fetchSsl(true);
      }
    };

    const result = mutationQueue.current.then(run);
    mutationQueue.current = result.then(
      () => undefined,
      () => undefined,
    );
    return result;
  };

  const handleAdd = async (e: React.FormEvent) => {
    e.preventDefault();
    if (!parsed.value || parsed.error) return;
    if (isDuplicate) {
      toast.error(`${parsed.value} is already mapped to this service`);
      return;
    }
    const hostname = parsed.value;
    setInput("");
    const ok = await persist([...domains, hostname], "add");
    if (ok) {
      toast.success(`${hostname} added — requesting HTTPS`);
    } else {
      // Give the value back rather than making them retype it
      setInput((current) => current || hostname);
    }
  };

  const handleRemove = async (hostname: string) => {
    setConfirmRemove(null);
    const ok = await persist(
      domains.filter((d) => d !== hostname),
      "remove",
    );
    if (ok) toast.success(`${hostname} removed`);
  };

  const handleRetry = async () => {
    setBusy("retry");
    setPollExpired(false);
    try {
      const res = await authFetch(`${serviceEndpoint}/ssl/retry`, {
        method: "POST",
      });
      const data = await res.json().catch(() => ({}));
      if (!res.ok) throw new Error(data.error || "SSL retry failed");
      if (data.ssl) setSslMap(data.ssl);
      if (data.events) setEvents(data.events);
      toast.success("Retry finished — check the status per domain");
    } catch (err: any) {
      toast.error(err?.message || "SSL retry failed");
    } finally {
      setBusy(null);
      void fetchSsl(true);
    }
  };

  const runDnsCheck = async (hostnames: string[]) => {
    setChecking(hostnames[0] ?? null);
    try {
      const res = await authFetch(`${serviceEndpoint}/dns-check`, {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ domains: hostnames }),
      });
      const data = await res.json().catch(() => ({}));
      if (!res.ok) throw new Error(data.error || "DNS check failed");
      const checks: DomainDnsCheck[] = data.checks || [];
      setDnsChecks((prev) => {
        const merged = { ...prev };
        for (const check of checks) merged[check.domain] = check;
        return merged;
      });
    } catch (err: any) {
      toast.error(err?.message || "DNS check failed");
    } finally {
      setChecking(null);
    }
  };

  const hint = parsed.value ? dnsRecordHint(parsed.value) : null;

  return (
    <Card className="border-border/50 p-4 sm:p-6">
      <div className="space-y-5">
        <div className="flex flex-wrap items-start justify-between gap-3">
          <div className="flex min-w-0 items-start gap-3">
            <div className="grid h-10 w-10 shrink-0 place-items-center rounded-xl bg-brand/10">
              <Server className="h-5 w-5 text-brand" />
            </div>
            <div className="min-w-0">
              <h4 className="truncate text-base font-bold">{service.name}</h4>
              <p className="mt-0.5 text-xs text-muted-foreground">
                {service.port ? `Public port ${service.port}` : "No port assigned"} · container
                port {service.internal_port} ·{" "}
                {domains.length === 0
                  ? "no domains"
                  : `${domains.length} domain${domains.length > 1 ? "s" : ""}`}
              </p>
            </div>
          </div>

          {domains.length > 0 && (
            <Button
              type="button"
              variant="outline"
              size="sm"
              className="h-9 gap-1.5 text-xs"
              onClick={handleRetry}
              disabled={busy !== null || !service.container_name}
              title={
                service.container_name
                  ? "Ask Let's Encrypt again for every domain on this service"
                  : "Deploy the service first"
              }
            >
              {busy === "retry" ? (
                <Loader2 className="h-3.5 w-3.5 animate-spin" />
              ) : (
                <RefreshCw className="h-3.5 w-3.5" />
              )}
              Retry HTTPS
            </Button>
          )}
        </div>

        <form onSubmit={handleAdd} className="space-y-2">
          <label
            htmlFor={`domain-input-${service.id}`}
            className="text-[11px] font-bold uppercase tracking-widest text-muted-foreground"
          >
            Add a domain
          </label>
          <div className="flex flex-col gap-2 sm:flex-row">
            <div className="relative min-w-0 flex-1">
              <span className="pointer-events-none absolute left-3 top-1/2 -translate-y-1/2 font-mono text-sm text-muted-foreground">
                https://
              </span>
              <input
                id={`domain-input-${service.id}`}
                value={input}
                onChange={(e) => setInput(displayHostnameFromInput(e.target.value))}
                onPaste={(e) => {
                  const pasted = e.clipboardData.getData("text");
                  if (!pasted) return;
                  const cleaned = displayHostnameFromInput(pasted);
                  if (cleaned !== pasted) {
                    e.preventDefault();
                    setInput(cleaned);
                  }
                }}
                onBlur={() => setInput((v) => displayHostnameFromInput(v))}
                placeholder="app.example.com"
                spellCheck={false}
                autoComplete="off"
                className="h-11 w-full rounded-lg border border-input bg-background pl-[4.75rem] pr-3 font-mono text-sm ring-offset-background transition-colors placeholder:font-sans placeholder:text-muted-foreground focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-brand focus-visible:ring-offset-2"
              />
            </div>
            <Button
              type="submit"
              disabled={!parsed.value || Boolean(parsed.error) || isDuplicate || busy !== null}
              className="h-11 gap-2 font-semibold"
            >
              {busy === "add" ? (
                <Loader2 className="h-4 w-4 animate-spin" />
              ) : (
                <Plus className="h-4 w-4" />
              )}
              Add domain
            </Button>
          </div>

          {parsed.error ? (
            <p className="flex items-start gap-1.5 text-xs text-red-500">
              <AlertTriangle className="mt-0.5 h-3.5 w-3.5 shrink-0" />
              {parsed.error}
            </p>
          ) : isDuplicate ? (
            <p className="flex items-start gap-1.5 text-xs text-amber-600">
              <AlertTriangle className="mt-0.5 h-3.5 w-3.5 shrink-0" />
              {parsed.value} is already mapped to this service.
            </p>
          ) : parsed.value ? (
            <div className="space-y-1 text-xs">
              <p className="text-muted-foreground">
                Saves as <code className="font-mono text-foreground">{parsed.value}</code>
                {hint && (
                  <>
                    {" "}
                    — needs an <strong className="text-foreground">A</strong> record named{" "}
                    <code className="font-mono text-foreground">{hint.name}</code> pointing at{" "}
                    <code className="font-mono text-foreground">{serverIP}</code>
                  </>
                )}
              </p>
            </div>
          ) : (
            <p className="text-xs text-muted-foreground">
              Paste a URL — only <code className="font-mono">https://</code> and a trailing{" "}
              <code className="font-mono">/</code> (or path) are removed. Subdomains stay as-is.
            </p>
          )}
        </form>

        {domains.length === 0 ? (
          <div className="rounded-xl border border-dashed border-border/60 bg-secondary/20 p-5 text-center">
            <Globe className="mx-auto h-6 w-6 text-muted-foreground/40" />
            <p className="mt-2 text-sm font-medium">No domain mapped yet</p>
            <p className="mt-1 text-xs text-muted-foreground">
              {typeof service.port === "number" &&
              service.port > 0 &&
              serverIP &&
              serverIP !== "..." &&
              serverIP !== "N/A"
                ? `Also reachable at http://${serverIP}:${service.port} (host port published).`
                : "Private by default — add a domain for HTTPS (preferred). Avoid sharing IP:port: it reveals your origin server. Raw host ports are opt-in via Build → Publish host ports, then redeploy."}
            </p>
          </div>
        ) : (
          <div className="space-y-2">
            {domains.map((domain) => {
              const ssl = sslMap[domain];
              const secure = ssl?.status === "active" || ssl?.status === "expiring";
              const dns = dnsChecks[domain];
              const fix =
                ssl?.status === "failed" || ssl?.status === "expired"
                  ? sslFixFor(ssl?.error, serverIP)
                  : null;

              return (
                <div
                  key={domain}
                  className="space-y-3 rounded-xl border border-border/50 bg-secondary/10 p-3 sm:p-4"
                >
                  <div className="flex flex-wrap items-start justify-between gap-2">
                    <div className="min-w-0 space-y-1.5">
                      <a
                        href={`${secure ? "https" : "http"}://${domain}`}
                        target="_blank"
                        rel="noopener noreferrer"
                        className="inline-flex items-start gap-1.5 font-mono text-sm font-semibold break-all transition-colors hover:text-brand"
                      >
                        {domain}
                        <ExternalLink className="mt-0.5 h-3.5 w-3.5 shrink-0 opacity-60" />
                      </a>
                      <div>
                        <SslPill ssl={ssl} />
                      </div>
                    </div>

                    {confirmRemove === domain ? (
                      <div className="flex items-center gap-1.5">
                        <span className="text-xs text-muted-foreground">Remove?</span>
                        <Button
                          type="button"
                          variant="destructive"
                          size="sm"
                          className="h-8 text-xs"
                          onClick={() => void handleRemove(domain)}
                          disabled={busy !== null}
                        >
                          {busy === "remove" ? (
                            <Loader2 className="h-3.5 w-3.5 animate-spin" />
                          ) : (
                            "Remove"
                          )}
                        </Button>
                        <Button
                          type="button"
                          variant="ghost"
                          size="sm"
                          className="h-8 text-xs"
                          onClick={() => setConfirmRemove(null)}
                        >
                          Cancel
                        </Button>
                      </div>
                    ) : (
                      <div className="flex shrink-0 items-center gap-1">
                        <Button
                          type="button"
                          variant="ghost"
                          size="sm"
                          className="h-8 gap-1.5 text-xs"
                          onClick={() => void runDnsCheck([domain])}
                          disabled={checking !== null}
                        >
                          {checking === domain ? (
                            <Loader2 className="h-3.5 w-3.5 animate-spin" />
                          ) : (
                            <Search className="h-3.5 w-3.5" />
                          )}
                          Check DNS
                        </Button>
                        <Button
                          type="button"
                          variant="ghost"
                          size="icon"
                          className="h-8 w-8 text-muted-foreground hover:bg-red-500/10 hover:text-red-500"
                          onClick={() => setConfirmRemove(domain)}
                          aria-label={`Remove ${domain}`}
                        >
                          <Trash2 className="h-4 w-4" />
                        </Button>
                      </div>
                    )}
                  </div>

                  {dns && (
                    <p className={cn("text-[11px] break-words", DNS_STYLES[dns.status])}>
                      {dns.message}
                    </p>
                  )}

                  {fix && (
                    <div className="space-y-2 rounded-lg border border-red-500/30 bg-red-500/5 p-3">
                      <p className="flex items-start gap-1.5 text-xs font-semibold text-red-500">
                        <AlertTriangle className="mt-0.5 h-3.5 w-3.5 shrink-0" />
                        {fix.title}
                      </p>
                      {ssl?.error && (
                        <p className="text-[11px] text-muted-foreground break-words">{ssl.error}</p>
                      )}
                      <ol className="space-y-1">
                        {fix.steps.map((step, i) => (
                          <li key={i} className="flex gap-2 text-[11px] text-foreground/80">
                            <span className="font-bold text-red-500/70">{i + 1}.</span>
                            <span className="min-w-0">{step}</span>
                          </li>
                        ))}
                      </ol>
                      {ssl?.diagnosticCommand && (
                        <CopyableCommand command={ssl.diagnosticCommand} />
                      )}
                    </div>
                  )}
                </div>
              );
            })}
          </div>
        )}

        {(events.length > 0 || busy !== null) && (
          <div className="overflow-hidden rounded-xl border border-border/50">
            <div className="flex items-center justify-between gap-2 bg-secondary/40 px-3 py-2">
              <span className="flex items-center gap-2 text-[11px] font-bold uppercase tracking-widest text-muted-foreground">
                {activityRunning ? (
                  <Loader2 className="h-3 w-3 animate-spin text-brand" />
                ) : (
                  <ScrollText className="h-3 w-3" />
                )}
                Let&apos;s Encrypt activity
              </span>
              {pollExpired && (
                <span className="ml-auto text-[11px] text-muted-foreground">
                  auto-refresh paused
                </span>
              )}
              <Button
                type="button"
                variant="ghost"
                size="sm"
                className="h-7 gap-1.5 text-[11px]"
                onClick={() => {
                  setPollExpired(false);
                  void fetchSsl(true);
                }}
              >
                <RefreshCw className="h-3 w-3" />
                Refresh
              </Button>
            </div>
            <div
              ref={logRef}
              className="shell-scroll max-h-48 divide-y divide-border/30 overflow-y-auto bg-background/60"
            >
              {events.length === 0 ? (
                <p className="px-3 py-3 text-[11px] text-muted-foreground">
                  Setting up routing and requesting a certificate…
                </p>
              ) : (
                events.map((event, i) => (
                  <div key={`${event.at}-${i}`} className="flex gap-2 px-3 py-1.5 text-[11px]">
                    <span className="shrink-0 font-mono text-muted-foreground/70">
                      {new Date(event.at).toLocaleTimeString([], {
                        hour: "2-digit",
                        minute: "2-digit",
                        second: "2-digit",
                      })}
                    </span>
                    <span className={cn("min-w-0 break-words", LEVEL_STYLES[event.level])}>
                      {event.message}
                    </span>
                  </div>
                ))
              )}
            </div>
          </div>
        )}
      </div>
    </Card>
  );
}
