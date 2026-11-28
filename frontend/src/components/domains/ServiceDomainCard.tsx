// Domain mapping for one service: add/remove hostnames, live Let's Encrypt
// activity, DNS verification and a concrete fix when issuance fails.
import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import {
  AlertTriangle,
  Check,
  Copy,
  ExternalLink,
  Loader2,
  RefreshCw,
  Search,
  Trash2,
} from "lucide-react";
import { toast } from "sonner";
import { SslStatusBadge, type SslInfo } from "@/components/SslStatusBadge";
import { Button } from "@/components/ui/button";
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

const LEVEL_STYLES: Record<SslEvent["level"], string> = {
  info: "text-foreground",
  success: "text-ok",
  warn: "text-warn",
  error: "text-danger",
};

const DNS_STYLES: Record<DomainDnsCheck["status"], string> = {
  ok: "text-ok",
  mismatch: "text-warn",
  missing: "text-danger",
  unknown: "text-muted-foreground",
};

function splitDomains(value: string | null): string[] {
  return (value || "")
    .split(",")
    .map((d) => d.trim().toLowerCase())
    .filter(Boolean);
}

function CopyableCommand({ command }: { command: string }) {
  const [copied, setCopied] = useState(false);
  return (
    <div className="flex items-center gap-1">
      <code className="shell-scroll min-w-0 flex-1 overflow-x-auto whitespace-nowrap rounded-sm bg-card px-2 py-1.5 font-mono text-xs text-foreground select-all">
        {command}
      </code>
      <Button
        type="button"
        variant="ghost"
        size="icon"
        className="shrink-0 text-subtle hover:text-foreground"
        onClick={async () => {
          await copyToClipboard(command);
          setCopied(true);
          setTimeout(() => setCopied(false), 1500);
        }}
        aria-label="Copy command"
      >
        {copied ? (
          <Check className="size-3.5 text-ok" />
        ) : (
          <Copy className="size-3.5" />
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
    <section className="rounded-lg border border-border bg-card">
      {/* Header */}
      <div className="flex flex-wrap items-start justify-between gap-3 border-b border-border px-4 py-3">
        <div className="min-w-0">
          <h3 className="truncate text-[15px] font-semibold leading-5">{service.name}</h3>
          <p className="mt-0.5 font-mono text-xs text-muted-foreground">
            {service.port ? `public port ${service.port}` : "no port assigned"}
            <span className="text-subtle"> · </span>
            container port {service.internal_port}
            <span className="text-subtle"> · </span>
            {domains.length === 0
              ? "no domains"
              : `${domains.length} domain${domains.length > 1 ? "s" : ""}`}
          </p>
        </div>

        {domains.length > 0 && (
          <Button
            type="button"
            variant="outline"
            size="sm"
            onClick={handleRetry}
            disabled={busy !== null || !service.container_name}
            title={
              service.container_name
                ? "Ask Let's Encrypt again for every domain on this service"
                : "Deploy the service first"
            }
          >
            {busy === "retry" ? (
              <Loader2 className="size-3.5 animate-spin" />
            ) : (
              <RefreshCw className="size-3.5" />
            )}
            Retry HTTPS
          </Button>
        )}
      </div>

      {/* Add a domain */}
      <form onSubmit={handleAdd} className="space-y-2 border-b border-border p-4">
        <label
          htmlFor={`domain-input-${service.id}`}
          className="text-[13px] font-medium"
        >
          Add a domain
        </label>
        <div className="flex flex-col gap-2 sm:flex-row">
          <div className="relative min-w-0 flex-1 sm:max-w-md">
            <span className="pointer-events-none absolute left-3 top-1/2 -translate-y-1/2 font-mono text-sm text-subtle">
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
              className="h-9 w-full rounded-md border border-input bg-card pl-[5.5rem] pr-3 font-mono text-sm transition-colors duration-150 placeholder:text-subtle focus-visible:border-brand focus-visible:outline-hidden focus-visible:ring-3 focus-visible:ring-brand/20 pointer-coarse:h-10"
            />
          </div>
          <Button
            type="submit"
            size="lg"
            disabled={!parsed.value || Boolean(parsed.error) || isDuplicate || busy !== null}
          >
            {busy === "add" && <Loader2 className="animate-spin" />}
            Add domain
          </Button>
        </div>

        {parsed.error ? (
          <p className="flex items-start gap-1.5 text-xs text-danger">
            <AlertTriangle className="mt-0.5 size-3.5 shrink-0" />
            {parsed.error}
          </p>
        ) : isDuplicate ? (
          <p className="flex items-start gap-1.5 text-xs text-warn">
            <AlertTriangle className="mt-0.5 size-3.5 shrink-0" />
            {parsed.value} is already mapped to this service.
          </p>
        ) : parsed.value ? (
          <p className="text-xs leading-5 text-muted-foreground">
            Saves as <code className="font-mono text-foreground">{parsed.value}</code>
            {hint && (
              <>
                {" "}
                — needs an <span className="font-medium text-foreground">A</span> record named{" "}
                <code className="font-mono text-foreground">{hint.name}</code> pointing at{" "}
                <code className="font-mono text-foreground">{serverIP}</code>
              </>
            )}
          </p>
        ) : (
          <p className="text-xs leading-5 text-muted-foreground">
            Paste a URL — only <code className="font-mono">https://</code> and a trailing{" "}
            <code className="font-mono">/</code> (or path) are removed. Subdomains stay as-is.
          </p>
        )}
      </form>

      {/* Mapped domains */}
      {domains.length === 0 ? (
        <div className="px-4 py-8 text-center">
          <p className="text-sm font-medium">No domain mapped yet</p>
          <p className="mx-auto mt-1 max-w-md text-[13px] leading-5 text-muted-foreground">
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
        <ul className="divide-y divide-border">
          {domains.map((domain) => {
            const ssl = sslMap[domain];
            const secure = ssl?.status === "active" || ssl?.status === "expiring";
            const dns = dnsChecks[domain];
            const fix =
              ssl?.status === "failed" || ssl?.status === "expired"
                ? sslFixFor(ssl?.error, serverIP)
                : null;

            return (
              <li key={domain} className="space-y-2.5 px-4 py-3">
                <div className="flex flex-wrap items-center justify-between gap-x-4 gap-y-2">
                  <div className="flex min-w-0 flex-wrap items-center gap-x-4 gap-y-1">
                    <a
                      href={`${secure ? "https" : "http"}://${domain}`}
                      target="_blank"
                      rel="noopener noreferrer"
                      className="inline-flex min-w-0 items-start gap-1 font-mono text-sm font-medium break-all underline-offset-4 hover:underline"
                    >
                      {domain}
                      <ExternalLink className="mt-0.5 size-3.5 shrink-0 text-subtle" />
                    </a>
                    {/* Error details live in the fix callout below, not twice. */}
                    <SslStatusBadge
                      ssl={fix && ssl ? { ...ssl, error: null, diagnosticCommand: null } : ssl}
                    />
                  </div>

                  {confirmRemove === domain ? (
                    <div className="flex items-center gap-1.5">
                      <span className="text-[13px] text-muted-foreground">Remove?</span>
                      <Button
                        type="button"
                        variant="destructive"
                        size="sm"
                        onClick={() => void handleRemove(domain)}
                        disabled={busy !== null}
                      >
                        {busy === "remove" ? (
                          <Loader2 className="size-3.5 animate-spin" />
                        ) : (
                          "Remove"
                        )}
                      </Button>
                      <Button
                        type="button"
                        variant="ghost"
                        size="sm"
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
                        className="text-muted-foreground"
                        onClick={() => void runDnsCheck([domain])}
                        disabled={checking !== null}
                      >
                        {checking === domain ? (
                          <Loader2 className="size-3.5 animate-spin" />
                        ) : (
                          <Search className="size-3.5" />
                        )}
                        Check DNS
                      </Button>
                      <Button
                        type="button"
                        variant="ghost"
                        size="icon"
                        className="text-subtle hover:text-danger"
                        onClick={() => setConfirmRemove(domain)}
                        aria-label={`Remove ${domain}`}
                      >
                        <Trash2 />
                      </Button>
                    </div>
                  )}
                </div>

                {dns && (
                  <p className={cn("text-xs wrap-break-word", DNS_STYLES[dns.status])}>
                    {dns.message}
                  </p>
                )}

                {fix && (
                  <div className="space-y-2 rounded-md border border-danger/40 bg-secondary/60 px-3 py-2.5 text-[13px]">
                    <p className="font-medium text-foreground">{fix.title}</p>
                    {ssl?.error && (
                      <p className="font-mono text-xs text-danger wrap-break-word">{ssl.error}</p>
                    )}
                    <ol className="space-y-1">
                      {fix.steps.map((step, i) => (
                        <li key={i} className="flex gap-2 leading-5 text-muted-foreground">
                          <span className="font-mono text-xs leading-5 text-subtle">{i + 1}.</span>
                          <span className="min-w-0">{step}</span>
                        </li>
                      ))}
                    </ol>
                    {ssl?.diagnosticCommand && (
                      <CopyableCommand command={ssl.diagnosticCommand} />
                    )}
                  </div>
                )}
              </li>
            );
          })}
        </ul>
      )}

      {/* Let's Encrypt activity */}
      {(events.length > 0 || busy !== null) && (
        <div className="border-t border-border">
          <div className="flex items-center justify-between gap-2 px-4 py-2">
            <span className="flex items-center gap-2 text-[13px] font-medium">
              {activityRunning && (
                <Loader2 className="size-3.5 animate-spin text-subtle" />
              )}
              Let&apos;s Encrypt activity
            </span>
            <div className="flex items-center gap-2">
              {pollExpired && (
                <span className="text-xs text-muted-foreground">
                  Auto-refresh paused
                </span>
              )}
              <Button
                type="button"
                variant="ghost"
                size="sm"
                className="text-muted-foreground"
                onClick={() => {
                  setPollExpired(false);
                  void fetchSsl(true);
                }}
              >
                <RefreshCw className="size-3.5" />
                Refresh
              </Button>
            </div>
          </div>
          <div
            ref={logRef}
            className="shell-scroll mx-4 mb-4 max-h-48 overflow-y-auto rounded-md bg-secondary/60 py-1.5"
          >
            {events.length === 0 ? (
              <p className="px-3 py-1.5 text-xs text-muted-foreground">
                Setting up routing and requesting a certificate…
              </p>
            ) : (
              events.map((event, i) => (
                <div key={`${event.at}-${i}`} className="flex gap-3 px-3 py-0.5 text-xs leading-5">
                  <span className="shrink-0 font-mono tabular-nums text-subtle">
                    {new Date(event.at).toLocaleTimeString([], {
                      hour: "2-digit",
                      minute: "2-digit",
                      second: "2-digit",
                    })}
                  </span>
                  <span className={cn("min-w-0 wrap-break-word", LEVEL_STYLES[event.level])}>
                    {event.message}
                  </span>
                </div>
              ))
            )}
          </div>
        </div>
      )}
    </section>
  );
}
