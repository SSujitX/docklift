// Panel (Settings → Domain) mapping: add/remove hostnames, live Let's Encrypt
// activity, DNS verification — same interaction model as ServiceDomainCard.
import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import {
  AlertTriangle,
  Check,
  Copy,
  ExternalLink,
  Loader2,
  Plus,
  RefreshCw,
  ScrollText,
  Search,
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
import {
  dismissSuggestion,
  panelAccessSuggestion,
  suggestionDismissed,
  type PanelAccess,
} from "@/lib/panelAccess";
import type { DomainDnsCheck, SslEvent } from "@/lib/types";
import { API_URL, cn, copyToClipboard } from "@/lib/utils";
import { PanelAccessSuggestion, PanelAccessSwitch, usePanelAccess } from "./PanelAccess";
import { sslFixFor } from "./sslHelp";

export interface PanelDomain {
  domain: string;
  port: number;
  ssl?: SslInfo | null;
  access?: PanelAccess;
}

type BusyAction = "add" | "remove" | "retry";

const POLL_TIMEOUT_MS = 5 * 60 * 1000;
const DEFAULT_PANEL_PORT = 8080;

const STATUS_META: Record<
  SslInfo["status"],
  { label: string; dot: string; text: string }
> = {
  active: { label: "HTTPS active", dot: "bg-ok", text: "text-foreground" },
  expiring: { label: "Renews soon", dot: "bg-warn", text: "text-foreground" },
  expired: { label: "Certificate expired", dot: "bg-danger", text: "text-danger" },
  failed: { label: "HTTPS setup failed", dot: "bg-danger", text: "text-danger" },
  pending: {
    label: "Issuing certificate",
    dot: "bg-warn animate-pulse motion-reduce:animate-none",
    text: "text-foreground",
  },
  missing: {
    label: "HTTP only — no certificate yet",
    dot: "bg-subtle",
    text: "text-muted-foreground",
  },
};

const LEVEL_STYLES: Record<SslEvent["level"], string> = {
  info: "text-foreground/85",
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

function SslPill({ ssl }: { ssl?: SslInfo | null }) {
  const status = ssl?.status || "missing";
  const meta = STATUS_META[status];
  const expiry =
    ssl?.expiresAt && (status === "active" || status === "expiring")
      ? new Date(ssl.expiresAt).toLocaleDateString()
      : null;

  return (
    <span className={cn("inline-flex items-center gap-1.5 text-[13px] font-medium", meta.text)}>
      <span aria-hidden="true" className={cn("size-2 shrink-0 rounded-full", meta.dot)} />
      {meta.label}
      {expiry && (
        <span className="font-normal text-muted-foreground">· until {expiry}</span>
      )}
    </span>
  );
}

function CopyableCommand({ command }: { command: string }) {
  const [copied, setCopied] = useState(false);
  return (
    <div className="flex items-center gap-1 rounded-md border border-input bg-card pl-2.5">
      <code className="shell-scroll min-w-0 flex-1 overflow-x-auto whitespace-nowrap py-1.5 font-mono text-xs select-all">
        {command}
      </code>
      <Button
        type="button"
        variant="ghost"
        size="icon"
        className="shrink-0 text-subtle"
        onClick={async () => {
          await copyToClipboard(command);
          setCopied(true);
          setTimeout(() => setCopied(false), 1500);
        }}
        aria-label="Copy command"
      >
        {copied ? <Check className="text-ok" /> : <Copy />}
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
  const panelAccess = usePanelAccess(serverIP, onUpdate);
  const [dismissed, setDismissed] = useState<string[]>([]);

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
  const suggestion =
    busy === null
      ? panelAccessSuggestion(domains, (d) => dismissed.includes(d) || suggestionDismissed(d))
      : null;

  return (
    <Card>
      <div className="flex flex-wrap items-start justify-between gap-3 border-b border-border px-4 py-3 sm:px-5">
        <div className="min-w-[16rem] flex-1">
          <h3 className="text-[15px] font-semibold leading-5">Panel domain</h3>
          <p className="mt-0.5 text-[13px] leading-5 text-muted-foreground">
            Maps a hostname to this Docklift dashboard, wires the reverse proxy, and requests a
            certificate.
          </p>
          <p className="mt-1 font-mono text-xs text-muted-foreground">
            Docklift dashboard · port {DEFAULT_PANEL_PORT} by default ·{" "}
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
            onClick={() => void handleRetry()}
            disabled={busy !== null}
            title="Ask Let's Encrypt again for every panel domain"
          >
            {busy === "retry" ? (
              <Loader2 className="animate-spin" />
            ) : (
              <RefreshCw className="text-subtle" />
            )}
            Retry HTTPS
          </Button>
        )}
      </div>

      {suggestion && (
        <div className="px-4 pt-4 sm:px-5">
          <PanelAccessSuggestion
            domain={suggestion}
            serverIP={serverIP}
            busy={panelAccess.busy}
            onEnable={panelAccess.requestEnable}
            onDismiss={(d) => {
              dismissSuggestion(d);
              setDismissed((prev) => [...prev, d]);
            }}
          />
        </div>
      )}

      <form onSubmit={(e) => void handleAdd(e)} className="space-y-2 p-4 sm:p-5">
        <label htmlFor="panel-domain-input" className="block text-[13px] font-medium">
          Add a domain
        </label>
        <div className="flex flex-col gap-2 sm:flex-row">
          <div className="relative min-w-0 flex-1">
            <span className="pointer-events-none absolute left-3 top-1/2 -translate-y-1/2 font-mono text-sm text-subtle">
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
              className="h-9 w-full rounded-md border border-input bg-card pl-[4.75rem] pr-3 font-mono text-sm text-foreground transition-[border-color,box-shadow] duration-150 placeholder:font-sans placeholder:text-subtle focus:border-brand focus:outline-hidden focus:ring-3 focus:ring-brand/20 pointer-coarse:h-10"
            />
          </div>
          <div className="flex gap-2">
            <input
              type="number"
              min={1}
              max={65535}
              value={portInput}
              onChange={(e) => setPortInput(e.target.value)}
              title="Docklift panel port"
              aria-label="Panel port"
              className="h-9 w-24 min-w-0 rounded-md border border-input bg-card px-3 font-mono text-sm text-foreground tabular-nums transition-[border-color,box-shadow] duration-150 focus:border-brand focus:outline-hidden focus:ring-3 focus:ring-brand/20 pointer-coarse:h-10 max-sm:flex-1"
            />
            <Button
              type="submit"
              size="lg"
              disabled={
                !parsed.value ||
                Boolean(parsed.error) ||
                Boolean(portError) ||
                isDuplicate ||
                busy !== null
              }
              className="shrink-0"
            >
              {busy === "add" ? <Loader2 className="animate-spin" /> : <Plus />}
              Add domain
            </Button>
          </div>
        </div>

        {parsed.error ? (
          <p className="flex items-start gap-1.5 text-xs leading-[1.125rem] text-danger">
            <AlertTriangle className="mt-px size-3.5 shrink-0" />
            {parsed.error}
          </p>
        ) : portError && (parsed.value || portInput.trim() !== String(DEFAULT_PANEL_PORT)) ? (
          <p className="flex items-start gap-1.5 text-xs leading-[1.125rem] text-danger">
            <AlertTriangle className="mt-px size-3.5 shrink-0" />
            {portError}
          </p>
        ) : isDuplicate ? (
          <p className="flex items-start gap-1.5 text-xs leading-[1.125rem] text-warn">
            <AlertTriangle className="mt-px size-3.5 shrink-0" />
            {parsed.value} is already mapped to the panel.
          </p>
        ) : parsed.value && panelPort != null ? (
          <p className="text-xs leading-[1.125rem] text-muted-foreground">
            Saves as <code className="font-mono text-foreground">{parsed.value}</code>
            {hint && (
              <>
                {" "}
                — needs an <strong className="font-medium text-foreground">A</strong> record named{" "}
                <code className="font-mono text-foreground">{hint.name}</code> pointing at{" "}
                <code className="font-mono text-foreground">{serverIP}</code>
              </>
            )}
            {" "}
            · proxies to panel port{" "}
            <code className="font-mono text-foreground">{panelPort}</code>
          </p>
        ) : (
          <p className="text-xs leading-[1.125rem] text-muted-foreground">
            Paste a URL — only <code className="font-mono">https://</code> and a trailing{" "}
            <code className="font-mono">/</code> (or path) are removed. Subdomains stay as-is.
            Change the port only if Docklift is not on {DEFAULT_PANEL_PORT}.
          </p>
        )}
      </form>

      {domains.length === 0 ? (
        <div className="border-t border-border">
          <div className="px-4 pb-6 pt-8 text-center">
            <p className="text-[15px] font-semibold">No domain mapped yet</p>
            <p className="mx-auto mt-1 max-w-sm text-[13.5px] text-muted-foreground">
              Access the panel at a hostname like{" "}
              <code className="font-mono text-foreground">panel.example.com</code> instead of
              IP:port.
            </p>
          </div>
          <div className="px-4 pb-4 sm:px-5 sm:pb-5">
            <PanelAccessSwitch
              domain={null}
              serverIP={serverIP}
              busy={panelAccess.busy}
              onEnable={panelAccess.requestEnable}
              onDisable={(d) => void panelAccess.disable(d)}
            />
          </div>
        </div>
      ) : (
        <ul className="divide-y divide-border border-t border-border">
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
              <li key={domain} className="space-y-3 px-4 py-3.5 sm:px-5">
                <div className="flex flex-wrap items-start justify-between gap-x-3 gap-y-2">
                  <div className="min-w-0 space-y-1">
                    <a
                      href={`${secure ? "https" : "http"}://${domain}`}
                      target="_blank"
                      rel="noopener noreferrer"
                      className="inline-flex items-start gap-1.5 font-mono text-sm font-medium break-all text-foreground transition-colors hover:text-brand"
                    >
                      {domain}
                      <ExternalLink className="mt-0.5 size-3.5 shrink-0 text-subtle" />
                    </a>
                    <div className="flex flex-wrap items-center gap-x-3 gap-y-1">
                      <SslPill ssl={ssl} />
                      <span className="font-mono text-xs text-muted-foreground">
                        → port <span className="text-foreground">{entry.port}</span>
                      </span>
                    </div>
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
                        {busy === "remove" ? <Loader2 className="animate-spin" /> : "Remove"}
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
                        variant="outline"
                        size="sm"
                        onClick={() => void runDnsCheck([domain])}
                        disabled={checking !== null}
                      >
                        {checking === domain ? (
                          <Loader2 className="animate-spin" />
                        ) : (
                          <Search className="text-subtle" />
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

                {entry.port === DEFAULT_PANEL_PORT && (
                  <PanelAccessSwitch
                    domain={domain}
                    access={entry.access}
                    serverIP={serverIP}
                    busy={panelAccess.busy || busy !== null}
                    onEnable={panelAccess.requestEnable}
                    onDisable={(d) => void panelAccess.disable(d)}
                  />
                )}

                {dns && (
                  <p className={cn("text-xs leading-[1.125rem] wrap-break-word", DNS_STYLES[dns.status])}>
                    {dns.message}
                  </p>
                )}

                {fix && (
                  <div className="space-y-2 rounded-md border border-danger/40 bg-secondary/60 px-3 py-2.5">
                    <p className="text-[13px] font-medium leading-5 text-danger">{fix.title}</p>
                    {ssl?.error && (
                      <p className="font-mono text-xs leading-[1.125rem] text-muted-foreground wrap-break-word">
                        {ssl.error}
                      </p>
                    )}
                    <ol className="list-decimal space-y-1 pl-4 text-[13px] leading-5 text-foreground/85 marker:font-mono marker:text-xs marker:text-subtle">
                      {fix.steps.map((step, i) => (
                        <li key={i} className="min-w-0 pl-1">
                          {step}
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

      {(events.length > 0 || busy !== null) && (
        <div className="border-t border-border">
          <div className="flex h-9 items-center justify-between gap-2 px-4 sm:px-5">
            <span className="flex items-center gap-2 text-[13px] font-medium">
              {activityRunning ? (
                <Loader2 className="size-3.5 animate-spin text-subtle" />
              ) : (
                <ScrollText className="size-3.5 text-subtle" />
              )}
              Let&apos;s Encrypt activity
            </span>
            {pollExpired && (
              <span className="ml-auto text-xs text-muted-foreground">auto-refresh paused</span>
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
          <div
            ref={logRef}
            className="shell-scroll max-h-48 overflow-y-auto rounded-b-lg border-t border-border bg-secondary/40 px-4 py-2 font-mono text-xs leading-5 sm:px-5"
          >
            {events.length === 0 ? (
              <p className="py-1 text-muted-foreground">Setting up routing and requesting a certificate…</p>
            ) : (
              events.map((event, i) => (
                <div key={`${event.at}-${i}`} className="flex gap-3 py-0.5">
                  <span className="shrink-0 text-subtle tabular-nums">
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
      {panelAccess.dialog}
    </Card>
  );
}
