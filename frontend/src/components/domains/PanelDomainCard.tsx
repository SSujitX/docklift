// Panel (Settings → Domain) mapping: add/remove hostnames, live Let's Encrypt
// activity, DNS verification — same interaction model as ServiceDomainCard.
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
import type { DomainDnsCheck, SslEvent } from "@/lib/types";
import { API_URL, cn, copyToClipboard } from "@/lib/utils";
import { sslFixFor } from "./sslHelp";

export interface PanelDomain {
  domain: string;
  port: number;
  ssl?: SslInfo | null;
}

type BusyAction = "add" | "remove" | "retry";

const POLL_TIMEOUT_MS = 5 * 60 * 1000;
const DEFAULT_PANEL_PORT = 8080;

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

function mergeEvents(batches: SslEvent[][]): SslEvent[] {
  return batches
    .flat()
    .sort((a, b) => new Date(a.at).getTime() - new Date(b.at).getTime());
}

function parsePanelPort(value: string): number | null {
  const raw = value.trim();
  if (!/^\d+$/.test(raw)) return null;
  const n = Number(raw);
  if (!Number.isInteger(n) || n < 1 || n > 65535) return null;
  return n;
}

export function PanelDomainCard({
  domains: propDomains,
  serverIP,
  onUpdate,
}: {
  domains: PanelDomain[];
  serverIP: string;
  /** Return false when the parent list refresh failed (keeps local confirmed state). */
  onUpdate: () => void | boolean | Promise<void | boolean>;
}) {
  const [domains, setDomains] = useState<PanelDomain[]>(propDomains);
  const domainsRef = useRef(propDomains);
  const mutationQueue = useRef(Promise.resolve());
  const [busy, setBusy] = useState<BusyAction | null>(null);
  const [input, setInput] = useState("");
  const [portInput, setPortInput] = useState(String(DEFAULT_PANEL_PORT));
  const [sslMap, setSslMap] = useState<Record<string, SslInfo>>({});
  const [events, setEvents] = useState<SslEvent[]>([]);
  const [dnsChecks, setDnsChecks] = useState<Record<string, DomainDnsCheck>>({});
  const [checking, setChecking] = useState<string | null>(null);
  const [confirmRemove, setConfirmRemove] = useState<string | null>(null);
  const [pollExpired, setPollExpired] = useState(false);
  const logRef = useRef<HTMLDivElement>(null);
  // After a successful mutation, ignore stale parent props until GET catches up.
  const expectHostnames = useRef<string[] | null>(null);

  useEffect(() => {
    domainsRef.current = domains;
  }, [domains]);

  useEffect(() => {
    if (busy !== null) return;
    const propNames = propDomains.map((d) => d.domain).slice().sort();
    if (expectHostnames.current) {
      const expected = expectHostnames.current.slice().sort();
      if (propNames.join("\0") !== expected.join("\0")) return;
      expectHostnames.current = null;
    }
    setDomains(propDomains);
    const nextSsl: Record<string, SslInfo> = {};
    for (const d of propDomains) {
      if (d.ssl) nextSsl[d.domain] = d.ssl;
    }
    setSslMap((prev) => ({ ...prev, ...nextSsl }));
  }, [propDomains, busy]);

  const hostnames = useMemo(() => domains.map((d) => d.domain), [domains]);
  const parsed = normalizeDomainInput(input);
  const isDuplicate = Boolean(parsed.value) && hostnames.includes(parsed.value);
  const panelPort = parsePanelPort(portInput);
  const portError =
    portInput.trim() === ""
      ? "Enter the panel port"
      : panelPort == null
        ? "Port must be an integer from 1 to 65535"
        : null;

  const fetchSsl = useCallback(async (force = false) => {
    const list = domainsRef.current;
    if (list.length === 0 && !force) {
      setSslMap({});
      setEvents([]);
      return;
    }
    try {
      const results = await Promise.all(
        list.map(async (d) => {
          const res = await authFetch(
            `${API_URL}/api/domains/${encodeURIComponent(d.domain)}/ssl`,
          );
          if (!res.ok) return { domain: d.domain, ssl: d.ssl || null, events: [] as SslEvent[] };
          const data = await res.json();
          return {
            domain: d.domain,
            ssl: (data.ssl as SslInfo) || null,
            events: (data.events || []) as SslEvent[],
          };
        }),
      );
      const nextSsl: Record<string, SslInfo> = {};
      for (const r of results) {
        if (r.ssl) nextSsl[r.domain] = r.ssl;
      }
      setSslMap(nextSsl);
      setEvents(mergeEvents(results.map((r) => r.events)));
    } catch {
      /* transient — the next poll retries */
    }
  }, []);

  const hostnamesKey = hostnames.join(",");
  useEffect(() => {
    void fetchSsl();
  }, [fetchSsl, hostnamesKey]);

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

  const handleAdd = async (e: React.FormEvent) => {
    e.preventDefault();
    if (!parsed.value || parsed.error || panelPort == null) return;
    if (isDuplicate) {
      toast.error(`${parsed.value} is already mapped to the panel`);
      return;
    }
    const hostname = parsed.value;
    const port = panelPort;
    setInput("");

    const run = async (): Promise<boolean> => {
      const previous = domainsRef.current;
      const optimistic: PanelDomain = {
        domain: hostname,
        port,
        ssl: { status: "pending" },
      };
      setDomains([...previous, optimistic]);
      setBusy("add");
      setPollExpired(false);
      try {
        const res = await authFetch(`${API_URL}/api/domains`, {
          method: "POST",
          headers: { "Content-Type": "application/json" },
          body: JSON.stringify({ domain: hostname, port }),
        });
        const data = await res.json().catch(() => ({}));
        if (!res.ok) throw new Error(data.error || "Failed to add domain");

        const confirmed: PanelDomain = {
          domain: data.domain || hostname,
          port: typeof data.port === "number" ? data.port : port,
          ssl: data.ssl || { status: "pending" },
        };
        const confirmedList = [...previous, confirmed];
        // Apply server result before clearing busy so prop sync cannot clobber with stale list.
        expectHostnames.current = confirmedList.map((d) => d.domain);
        setDomains(confirmedList);
        if (data.ssl) {
          setSslMap((prev) => ({ ...prev, [confirmed.domain]: data.ssl }));
        }
        if (data.events) setEvents((prev) => mergeEvents([prev, data.events]));

        const status = data.ssl?.status as SslInfo["status"] | undefined;
        if (status === "active" || status === "expiring") {
          toast.success(`${confirmed.domain} added — HTTPS active`);
        } else if (status === "failed" || status === "expired") {
          toast.message(`${confirmed.domain} added — SSL: ${status}`, {
            description: data.ssl?.error || "Check DNS, then Retry HTTPS.",
          });
        } else {
          toast.success(`${confirmed.domain} added — requesting HTTPS`);
        }

        // Keep expectHostnames if refresh fails — stale props must not wipe a successful add.
        const refreshed = await Promise.resolve(onUpdate());
        if (refreshed === false) {
          toast.message("Domain saved — list refresh failed", {
            description: "Reload the Domain tab if the list looks wrong.",
          });
        }
        return true;
      } catch (err: any) {
        expectHostnames.current = null;
        setDomains(previous);
        toast.error(err?.message || "Failed to add domain");
        setInput((current) => current || hostname);
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
    await result;
  };

  const handleRemove = async (hostname: string) => {
    setConfirmRemove(null);
    const run = async (): Promise<boolean> => {
      const previous = domainsRef.current;
      const next = previous.filter((d) => d.domain !== hostname);
      setDomains(next);
      setBusy("remove");
      try {
        const res = await authFetch(
          `${API_URL}/api/domains/${encodeURIComponent(hostname)}`,
          { method: "DELETE" },
        );
        const data = await res.json().catch(() => ({}));
        if (!res.ok) throw new Error(data.error || "Failed to remove domain");
        expectHostnames.current = next.map((d) => d.domain);
        setDomains(next);
        setSslMap((prev) => {
          const copy = { ...prev };
          delete copy[hostname];
          return copy;
        });
        toast.success(`${hostname} removed`);
        // Keep expectHostnames if refresh fails — stale props must not resurrect a removed domain.
        const refreshed = await Promise.resolve(onUpdate());
        if (refreshed === false) {
          toast.message("Domain removed — list refresh failed", {
            description: "Reload the Domain tab if the list looks wrong.",
          });
        }
        return true;
      } catch (err: any) {
        expectHostnames.current = null;
        setDomains(previous);
        toast.error(err?.message || "Failed to remove domain");
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
    await result;
  };

  const handleRetry = async () => {
    const list = domainsRef.current;
    if (list.length === 0) return;
    setBusy("retry");
    setPollExpired(false);
    try {
      for (const d of list) {
        const res = await authFetch(
          `${API_URL}/api/domains/${encodeURIComponent(d.domain)}/ssl/retry`,
          { method: "POST" },
        );
        const data = await res.json().catch(() => ({}));
        if (!res.ok) throw new Error(data.error || `SSL retry failed for ${d.domain}`);
        if (data.ssl) {
          setSslMap((prev) => ({ ...prev, [d.domain]: data.ssl }));
        }
        if (data.events) setEvents((prev) => mergeEvents([prev, data.events]));
      }
      toast.success("Retry finished — check the status per domain");
      await Promise.resolve(onUpdate());
    } catch (err: any) {
      toast.error(err?.message || "SSL retry failed");
    } finally {
      setBusy(null);
      void fetchSsl(true);
    }
  };

  const runDnsCheck = async (names: string[]) => {
    setChecking(names[0] ?? null);
    try {
      const res = await authFetch(`${API_URL}/api/domains/dns-check`, {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ domains: names }),
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
              <Globe className="h-5 w-5 text-brand" />
            </div>
            <div className="min-w-0">
              <h4 className="truncate text-base font-bold">Panel domain</h4>
              <p className="mt-0.5 text-xs text-muted-foreground">
                Docklift dashboard · port {DEFAULT_PANEL_PORT} by default ·{" "}
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
              onClick={() => void handleRetry()}
              disabled={busy !== null}
              title="Ask Let's Encrypt again for every panel domain"
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

        <form onSubmit={(e) => void handleAdd(e)} className="space-y-2">
          <label
            htmlFor="panel-domain-input"
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
                id="panel-domain-input"
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
                placeholder="panel.example.com"
                spellCheck={false}
                autoComplete="off"
                className="h-11 w-full rounded-lg border border-input bg-background pl-[4.75rem] pr-3 font-mono text-sm ring-offset-background transition-colors placeholder:font-sans placeholder:text-muted-foreground focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-brand focus-visible:ring-offset-2"
              />
            </div>
            <input
              type="number"
              min={1}
              max={65535}
              value={portInput}
              onChange={(e) => setPortInput(e.target.value)}
              title="Docklift panel port"
              aria-label="Panel port"
              className="h-11 w-full rounded-lg border border-input bg-background px-3 font-mono text-sm sm:w-28"
            />
            <Button
              type="submit"
              disabled={
                !parsed.value ||
                Boolean(parsed.error) ||
                Boolean(portError) ||
                isDuplicate ||
                busy !== null
              }
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
          ) : portError && (parsed.value || portInput.trim() !== String(DEFAULT_PANEL_PORT)) ? (
            <p className="flex items-start gap-1.5 text-xs text-red-500">
              <AlertTriangle className="mt-0.5 h-3.5 w-3.5 shrink-0" />
              {portError}
            </p>
          ) : isDuplicate ? (
            <p className="flex items-start gap-1.5 text-xs text-amber-600">
              <AlertTriangle className="mt-0.5 h-3.5 w-3.5 shrink-0" />
              {parsed.value} is already mapped to the panel.
            </p>
          ) : parsed.value && panelPort != null ? (
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
                {" "}
                · proxies to panel port{" "}
                <code className="font-mono text-foreground">{panelPort}</code>
              </p>
            </div>
          ) : (
            <p className="text-xs text-muted-foreground">
              Paste a URL — only <code className="font-mono">https://</code> and a trailing{" "}
              <code className="font-mono">/</code> (or path) are removed. Subdomains stay as-is.
              Change the port only if Docklift is not on {DEFAULT_PANEL_PORT}.
            </p>
          )}
        </form>

        {domains.length === 0 ? (
          <div className="rounded-xl border border-dashed border-border/60 bg-secondary/20 p-5 text-center">
            <Globe className="mx-auto h-6 w-6 text-muted-foreground/40" />
            <p className="mt-2 text-sm font-medium">No domain mapped yet</p>
            <p className="mt-1 text-xs text-muted-foreground">
              Access the panel at a hostname like{" "}
              <code className="font-mono">panel.example.com</code> instead of IP:port.
            </p>
          </div>
        ) : (
          <div className="space-y-2">
            {domains.map((entry) => {
              const domain = entry.domain;
              const ssl = sslMap[domain] || entry.ssl;
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
                      <div className="flex flex-wrap items-center gap-2">
                        <SslPill ssl={ssl} />
                        <span className="text-[11px] text-muted-foreground">
                          → port{" "}
                          <code className="font-mono text-foreground/80">{entry.port}</code>
                        </span>
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
