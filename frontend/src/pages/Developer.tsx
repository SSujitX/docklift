// API & MCP: API tokens for scripts and AI agents, the endpoint browser fed by
// /api/openapi.json, and ready-to-copy MCP client snippets.

import { useCallback, useEffect, useMemo, useState, type FormEvent, type ReactNode } from "react";
import { toast } from "sonner";
import { Check, ChevronRight, Copy, Loader2, Plus, RefreshCw, Search, Trash2 } from "lucide-react";
import { PageHeader, StatChip } from "@/components/shell/PageHeader";
import { Button } from "@/components/ui/button";
import { Card } from "@/components/ui/card";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { Input } from "@/components/ui/input";
import { authFetch } from "@/lib/auth";
import { API_URL, cn, copyToClipboard } from "@/lib/utils";
import {
  type ApiToken,
  type Endpoint,
  type EndpointGroup,
  type TokenScope,
  EXPIRY_OPTIONS,
  TOKEN_PLACEHOLDER,
  claudeCodeCommand,
  countEndpoints,
  curlExample,
  describeTime,
  filterEndpoints,
  genericMcpConfig,
  mcpUrl,
  parseOpenApi,
} from "@/lib/developer";

async function errorOf(res: Response, fallback: string): Promise<string> {
  const data = (await res.json().catch(() => ({}))) as { error?: string };
  return data.error || `${fallback} (${res.status})`;
}

async function copy(text: string, what: string) {
  if (await copyToClipboard(text)) toast.success(`${what} copied`);
  else toast.error("Could not copy to the clipboard");
}

export default function DeveloperPage() {
  const [tokens, setTokens] = useState<ApiToken[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [creating, setCreating] = useState(false);
  const [revoking, setRevoking] = useState<ApiToken | null>(null);
  const [created, setCreated] = useState<{ token: string; item: ApiToken } | null>(null);
  // The last token created on this page fills the snippets (never stored).
  const [snippetToken, setSnippetToken] = useState<string | null>(null);

  const load = useCallback(async () => {
    try {
      const res = await authFetch(`${API_URL}/api/tokens`);
      if (!res.ok) throw new Error(await errorOf(res, "Failed to load API tokens"));
      const data = (await res.json()) as { tokens?: ApiToken[] };
      setTokens(Array.isArray(data.tokens) ? data.tokens : []);
      setError(null);
    } catch (err) {
      setError(err instanceof Error ? err.message : "Failed to load API tokens");
    } finally {
      setLoading(false);
    }
  }, []);

  useEffect(() => {
    void load();
  }, [load]);

  const active = tokens.filter((t) => t.status === "active").length;

  return (
    <div className="space-y-6">
      <PageHeader
        title="API & MCP"
        description="Let scripts and AI agents do what you do in the dashboard, with a bearer token instead of your password."
        meta={
          <>
            <StatChip label="Tokens" value={tokens.length} />
            <StatChip label="Active" value={active} tone={active > 0 ? "success" : "neutral"} />
          </>
        }
        actions={
          <Button onClick={() => setCreating(true)}>
            <Plus />
            Create token
          </Button>
        }
      />

      <TokensPanel
        tokens={tokens}
        loading={loading}
        error={error}
        onRefresh={() => {
          setLoading(true);
          void load();
        }}
        onRevoke={setRevoking}
        onCreate={() => setCreating(true)}
      />

      <ConnectPanel token={snippetToken} />

      <EndpointsPanel />

      <CreateTokenDialog
        open={creating}
        onClose={() => setCreating(false)}
        onCreated={(token, item) => {
          setCreating(false);
          setCreated({ token, item });
          setSnippetToken(token);
          void load();
        }}
      />
      <CreatedTokenDialog created={created} onClose={() => setCreated(null)} />
      <RevokeTokenDialog
        token={revoking}
        onClose={() => setRevoking(null)}
        onRevoked={() => {
          setRevoking(null);
          void load();
        }}
      />
    </div>
  );
}

/* ---------- layout helpers ---------- */

function Panel({
  title,
  description,
  meta,
  actions,
  children,
}: {
  title: string;
  description?: ReactNode;
  meta?: ReactNode;
  actions?: ReactNode;
  children: ReactNode;
}) {
  return (
    <Card>
      <div className="flex items-start justify-between gap-4 border-b border-border px-4 py-3 sm:px-5">
        <div className="min-w-0">
          <h2 className="flex items-baseline gap-2 text-[15px] font-semibold leading-5">
            {title}
            {meta !== undefined && (
              <span className="font-mono text-[13px] font-normal text-subtle tabular-nums">{meta}</span>
            )}
          </h2>
          {description && <p className="mt-0.5 text-[13px] leading-5 text-muted-foreground">{description}</p>}
        </div>
        {actions && <div className="flex shrink-0 items-center gap-1">{actions}</div>}
      </div>
      {children}
    </Card>
  );
}

function Tag({ children, warn = false }: { children: ReactNode; warn?: boolean }) {
  return (
    <span
      className={cn(
        "inline-flex h-5 items-center rounded-sm border px-1.5 font-mono text-[11px] leading-none whitespace-nowrap",
        warn ? "border-warn/40 text-warn" : "border-border text-muted-foreground",
      )}
    >
      {children}
    </span>
  );
}

function CopyBlock({ label, value, what, wrap = false }: { label: string; value: string; what: string; wrap?: boolean }) {
  const [done, setDone] = useState(false);
  return (
    <div className="min-w-0 space-y-1.5">
      <p className="text-[13px] font-medium leading-5">{label}</p>
      <div className="flex items-start rounded-md border border-border bg-secondary/60">
        <pre
          className={cn(
            "shell-scroll min-w-0 flex-1 px-3 py-2 font-mono text-xs leading-5 text-foreground select-all",
            wrap ? "whitespace-pre-wrap [overflow-wrap:anywhere]" : "overflow-x-auto whitespace-pre",
          )}
        >
          {value}
        </pre>
        <Button
          variant="ghost"
          size="icon"
          className="m-0.5 shrink-0"
          title={`Copy ${what.toLowerCase()}`}
          aria-label={`Copy ${what.toLowerCase()}`}
          onClick={async () => {
            await copy(value, what);
            setDone(true);
            window.setTimeout(() => setDone(false), 1500);
          }}
        >
          {done ? <Check className="text-ok" /> : <Copy className="text-subtle" />}
        </Button>
      </div>
    </div>
  );
}

/* ---------- tokens ---------- */

function TokensPanel({
  tokens,
  loading,
  error,
  onRefresh,
  onRevoke,
  onCreate,
}: {
  tokens: ApiToken[];
  loading: boolean;
  error: string | null;
  onRefresh: () => void;
  onRevoke: (t: ApiToken) => void;
  onCreate: () => void;
}) {
  return (
    <Panel
      title="API tokens"
      description="Send one as Authorization: Bearer dl_… to the REST API or the MCP endpoint. Changing your password revokes every token."
      actions={
        <Button variant="ghost" size="icon" onClick={onRefresh} title="Refresh tokens" aria-label="Refresh tokens">
          <RefreshCw className="text-subtle" />
        </Button>
      }
    >
      {error && (
        <div className="m-4 rounded-md border border-danger/40 bg-secondary/60 px-3 py-2.5 text-[13px] text-foreground sm:mx-5">{error}</div>
      )}
      {loading ? (
        <div className="divide-y divide-border" aria-hidden="true">
          {[1, 2, 3].map((i) => (
            <div key={i} className="flex h-12 items-center px-4 sm:px-5">
              <div className="h-2.5 w-40 rounded-sm bg-secondary" />
            </div>
          ))}
        </div>
      ) : tokens.length === 0 ? (
        <div className="px-4 py-14 text-center">
          <p className="text-[15px] font-semibold">No API tokens yet</p>
          <p className="mx-auto mt-1 max-w-sm text-[13.5px] text-muted-foreground">
            Create one for a CI job, a script or an AI agent. It is shown once.
          </p>
          <Button className="mt-4" onClick={onCreate}>
            <Plus />
            Create token
          </Button>
        </div>
      ) : (
        <>
          {/* Mobile */}
          <ul className="divide-y divide-border md:hidden">
            {tokens.map((t) => (
              <li key={t.id} className="flex items-start gap-3 px-4 py-3">
                <div className="min-w-0 flex-1">
                  <p className={cn("truncate text-sm font-medium leading-5", t.status !== "active" && "text-muted-foreground")}>{t.name}</p>
                  <p className="mt-0.5 truncate text-xs leading-5 text-muted-foreground">
                    <span className="font-mono">{t.prefix}…</span> · <TokenAccess token={t} />
                  </p>
                  <p className="text-xs leading-5 text-muted-foreground">
                    Used {describeTime(t.last_used_at).toLowerCase()} · <TokenExpiry token={t} lower />
                  </p>
                </div>
                <Button variant="ghost" size="icon" onClick={() => onRevoke(t)} title={`Revoke ${t.name}`} aria-label={`Revoke ${t.name}`}>
                  <Trash2 className="text-subtle" />
                </Button>
              </li>
            ))}
          </ul>
          {/* Desktop */}
          <table className="hidden w-full text-left text-sm md:table">
            <thead>
              <tr className="h-9 border-b border-border text-xs text-muted-foreground">
                <th className="px-5 font-medium">Name</th>
                <th className="px-4 font-medium">Token</th>
                <th className="px-4 font-medium">Access</th>
                <th className="px-4 font-medium">Last used</th>
                <th className="px-4 font-medium">Expires</th>
                <th className="px-5 text-right font-medium">
                  <span className="sr-only">Revoke</span>
                </th>
              </tr>
            </thead>
            <tbody className="divide-y divide-border">
              {tokens.map((t) => (
                <tr key={t.id} className="h-11 hover:bg-secondary/50">
                  <td className={cn("max-w-56 truncate px-5 font-medium", t.status !== "active" && "text-muted-foreground")}>{t.name}</td>
                  <td className="px-4 font-mono text-xs text-muted-foreground">{t.prefix}…</td>
                  <td className="px-4 text-[13px] text-muted-foreground">
                    <TokenAccess token={t} />
                  </td>
                  <td className="px-4 text-[13px] text-muted-foreground" title={t.last_used_at ?? undefined}>
                    {describeTime(t.last_used_at)}
                  </td>
                  <td className="px-4 text-[13px] text-muted-foreground" title={t.expires_at ?? undefined}>
                    <TokenExpiry token={t} />
                  </td>
                  <td className="px-3 text-right">
                    <Button variant="ghost" size="sm" className="text-muted-foreground hover:text-danger" onClick={() => onRevoke(t)}>
                      Revoke
                    </Button>
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </>
      )}
    </Panel>
  );
}

/** Access in words. Only the permission to run dangerous actions carries colour. */
function TokenAccess({ token }: { token: ApiToken }) {
  return (
    <>
      {token.scope === "full" ? "Full access" : "Read only"}
      {token.allow_dangerous && <span className="text-warn"> + dangerous actions</span>}
    </>
  );
}

function TokenExpiry({ token, lower = false }: { token: ApiToken; lower?: boolean }) {
  if (token.status === "revoked") return <span className="text-danger">{lower ? "revoked" : "Revoked"}</span>;
  if (token.status === "expired") {
    return (
      <span className="text-danger">
        {lower ? "expired" : "Expired"} {describeTime(token.expires_at).toLowerCase()}
      </span>
    );
  }
  if (!token.expires_at) return <>{lower ? "no expiry" : "Never"}</>;
  return <>{lower ? `expires ${describeTime(token.expires_at)}` : describeTime(token.expires_at)}</>;
}

function CreateTokenDialog({
  open,
  onClose,
  onCreated,
}: {
  open: boolean;
  onClose: () => void;
  onCreated: (token: string, item: ApiToken) => void;
}) {
  const [name, setName] = useState("");
  const [scope, setScope] = useState<TokenScope>("read");
  const [dangerous, setDangerous] = useState(false);
  const [expiry, setExpiry] = useState<number | null>(90);
  const [password, setPassword] = useState("");
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    if (open) {
      setName("");
      setScope("read");
      setDangerous(false);
      setExpiry(90);
      setPassword("");
      setError(null);
    }
  }, [open]);

  const submit = async (e: FormEvent) => {
    e.preventDefault();
    if (!name.trim() || !password) return;
    setBusy(true);
    setError(null);
    try {
      const res = await authFetch(`${API_URL}/api/tokens`, {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({
          name: name.trim(),
          scope,
          allowDangerous: scope === "full" && dangerous,
          expiresInDays: expiry,
          password,
        }),
      });
      if (!res.ok) throw new Error(await errorOf(res, "Failed to create token"));
      const data = (await res.json()) as { token: string; apiToken: ApiToken };
      onCreated(data.token, data.apiToken);
    } catch (err) {
      setError(err instanceof Error ? err.message : "Failed to create token");
    } finally {
      setPassword("");
      setBusy(false);
    }
  };

  return (
    <Dialog open={open} onOpenChange={(o) => !o && !busy && onClose()}>
      <DialogContent>
        <DialogHeader>
          <DialogTitle>Create API token</DialogTitle>
          <DialogDescription>The token is shown once. Store it where your script or agent reads secrets.</DialogDescription>
        </DialogHeader>
        <form className="space-y-4" onSubmit={submit}>
          <label className="block space-y-1.5">
            <span className="block text-[13px] font-medium leading-5">Name</span>
            <Input value={name} onChange={(e) => setName(e.target.value)} maxLength={100} placeholder="e.g. claude-code, github-actions" autoFocus />
          </label>

          <fieldset className="space-y-1.5">
            <legend className="mb-1.5 text-[13px] font-medium leading-5">Access</legend>
            <div className="grid gap-2 sm:grid-cols-2">
              {(
                [
                  ["read", "Read only", "GET requests: list, inspect, read logs."],
                  ["full", "Full", "Everything an admin can do in the dashboard."],
                ] as const
              ).map(([value, label, help]) => (
                <label
                  key={value}
                  className={cn(
                    "cursor-pointer rounded-md border px-3 py-2 transition-colors duration-150",
                    scope === value ? "border-brand bg-brand/5" : "border-border hover:bg-secondary/60",
                  )}
                >
                  <input
                    type="radio"
                    name="token-scope"
                    value={value}
                    checked={scope === value}
                    onChange={() => setScope(value)}
                    className="sr-only"
                  />
                  <span className="block text-[13px] font-medium">{label}</span>
                  <span className="block text-xs leading-[1.125rem] text-muted-foreground">{help}</span>
                </label>
              ))}
            </div>
          </fieldset>

          <label className={cn("flex items-start gap-2.5", scope !== "full" && "opacity-50")}>
            <input
              type="checkbox"
              className="mt-0.5 size-4 accent-[hsl(var(--brand))]"
              checked={scope === "full" && dangerous}
              disabled={scope !== "full"}
              onChange={(e) => setDangerous(e.target.checked)}
            />
            <span className="space-y-0.5">
              <span className="block text-[13px] font-medium leading-5">Allow dangerous actions</span>
              <span className="block text-xs leading-[1.125rem] text-muted-foreground">
                Actions that ask for your password in the dashboard: rollback, restore, purge, reboot, upgrade and shell commands on the
                host. Leave off unless the agent needs them.
              </span>
            </span>
          </label>

          <label className="block space-y-1.5">
            <span className="block text-[13px] font-medium leading-5">Expires</span>
            <select
              className="flex h-9 w-full rounded-md border border-input bg-card px-3 text-sm text-foreground focus:border-brand focus:outline-hidden focus:ring-3 focus:ring-brand/20 pointer-coarse:h-10"
              value={expiry === null ? "never" : String(expiry)}
              onChange={(e) => setExpiry(e.target.value === "never" ? null : Number(e.target.value))}
            >
              {EXPIRY_OPTIONS.map((o) => (
                <option key={o.label} value={o.days === null ? "never" : String(o.days)}>
                  {o.label}
                </option>
              ))}
            </select>
          </label>

          <label className="block space-y-1.5">
            <span className="block text-[13px] font-medium leading-5">Confirm with account password</span>
            <Input
              type="password"
              value={password}
              onChange={(e) => setPassword(e.target.value)}
              placeholder="Your Docklift password"
              autoComplete="current-password"
            />
          </label>

          {error && <p className="text-[13px] text-danger">{error}</p>}

          <DialogFooter>
            <Button type="button" variant="outline" onClick={onClose} disabled={busy}>
              Cancel
            </Button>
            <Button type="submit" disabled={busy || !name.trim() || !password}>
              {busy && <Loader2 className="animate-spin" />}
              Create token
            </Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  );
}

function CreatedTokenDialog({ created, onClose }: { created: { token: string; item: ApiToken } | null; onClose: () => void }) {
  return (
    <Dialog open={created !== null} onOpenChange={(o) => !o && onClose()}>
      <DialogContent className="max-w-lg">
        <DialogHeader>
          <DialogTitle>Copy your token now</DialogTitle>
          <DialogDescription>
            This is the only time <span className="text-foreground">{created?.item.name}</span> is shown. Docklift keeps only a hash of it.
          </DialogDescription>
        </DialogHeader>
        {created && (
          <div className="space-y-3">
            <CopyBlock label="Token" value={created.token} what="Token" wrap />
            <p className="text-[13px] leading-5 text-muted-foreground">
              <TokenAccess token={created.item} /> · <TokenExpiry token={created.item} lower />
            </p>
            <p className="text-xs leading-[1.125rem] text-muted-foreground">
              The snippets under Connect an AI now include this token until you leave the page.
            </p>
          </div>
        )}
        <DialogFooter>
          <Button onClick={onClose}>Done</Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}

function RevokeTokenDialog({ token, onClose, onRevoked }: { token: ApiToken | null; onClose: () => void; onRevoked: () => void }) {
  const [password, setPassword] = useState("");
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    setPassword("");
    setError(null);
  }, [token]);

  const submit = async (e: FormEvent) => {
    e.preventDefault();
    if (!token || !password) return;
    setBusy(true);
    setError(null);
    try {
      const res = await authFetch(`${API_URL}/api/tokens/${encodeURIComponent(token.id)}`, {
        method: "DELETE",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ password }),
      });
      if (!res.ok) throw new Error(await errorOf(res, "Failed to revoke token"));
      toast.success(`Revoked ${token.name}`);
      onRevoked();
    } catch (err) {
      setError(err instanceof Error ? err.message : "Failed to revoke token");
    } finally {
      setPassword("");
      setBusy(false);
    }
  };

  return (
    <Dialog open={token !== null} onOpenChange={(o) => !o && !busy && onClose()}>
      <DialogContent>
        <DialogHeader>
          <DialogTitle>Revoke {token?.name}?</DialogTitle>
          <DialogDescription>
            <span className="font-mono">{token?.prefix}…</span> stops working at once, for every script and agent using it.
          </DialogDescription>
        </DialogHeader>
        <form className="space-y-4" onSubmit={submit}>
          <label className="block space-y-1.5">
            <span className="block text-[13px] font-medium leading-5">Confirm with account password</span>
            <Input
              type="password"
              value={password}
              onChange={(e) => setPassword(e.target.value)}
              placeholder="Your Docklift password"
              autoComplete="current-password"
              autoFocus
            />
          </label>
          {error && <p className="text-[13px] text-danger">{error}</p>}
          <DialogFooter>
            <Button type="button" variant="outline" onClick={onClose} disabled={busy}>
              Cancel
            </Button>
            <Button type="submit" variant="destructive" disabled={busy || !password}>
              {busy && <Loader2 className="animate-spin" />}
              Revoke token
            </Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  );
}

/* ---------- connect an AI ---------- */

function ConnectPanel({ token }: { token: string | null }) {
  const url = mcpUrl(window.location.origin, API_URL);
  const t = token ?? TOKEN_PLACEHOLDER;
  const [tab, setTab] = useState<"claude" | "json" | "curl">("claude");
  const tabs = [
    { id: "claude", label: "Claude Code" },
    { id: "json", label: "Other MCP clients" },
    { id: "curl", label: "REST (curl)" },
  ] as const;
  return (
    <Panel
      title="Connect an AI"
      description="Docklift speaks MCP (Streamable HTTP) at the URL below. Every tool call runs through the API with the token's permissions."
    >
      <div className="space-y-4 p-4 sm:p-5">
        <CopyBlock label="MCP URL" value={url} what="MCP URL" />
        <div className="hairline-b flex gap-5 overflow-x-auto scrollbar-none" role="tablist" aria-label="Client">
          {tabs.map((x) => (
            <button
              key={x.id}
              type="button"
              role="tab"
              aria-selected={tab === x.id}
              onClick={() => setTab(x.id)}
              className={cn(
                "inline-flex h-10 shrink-0 items-center border-b-2 text-[13.5px] transition-colors duration-150 focus-visible:-outline-offset-2",
                tab === x.id ? "border-brand font-medium text-foreground" : "border-transparent text-muted-foreground hover:text-foreground",
              )}
            >
              {x.label}
            </button>
          ))}
        </div>
        {tab === "claude" && <CopyBlock label="Run in your project" value={claudeCodeCommand(url, t)} what="Command" wrap />}
        {tab === "json" && <CopyBlock label="MCP config (mcpServers)" value={genericMcpConfig(url, t)} what="Config" />}
        {tab === "curl" && <CopyBlock label="Call the REST API" value={curlExample(API_URL || window.location.origin, t)} what="Command" wrap />}
        <p className="text-xs leading-[1.125rem] text-muted-foreground">
          {token ? "The snippet contains the token you just created." : `Replace ${TOKEN_PLACEHOLDER} with a token.`} Use a read-only token
          for agents that only need to look. Guide:{" "}
          <a className="text-brand underline-offset-4 hover:underline" href="https://docklift.dev/guide/mcp" target="_blank" rel="noreferrer">
            docklift.dev/guide/mcp
          </a>
        </p>
      </div>
    </Panel>
  );
}

/* ---------- endpoint browser ---------- */

function EndpointsPanel() {
  const [groups, setGroups] = useState<EndpointGroup[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [query, setQuery] = useState("");

  useEffect(() => {
    let cancelled = false;
    (async () => {
      try {
        const res = await authFetch(`${API_URL}/api/openapi.json`);
        if (!res.ok) throw new Error(await errorOf(res, "Failed to load the endpoint list"));
        const doc = await res.json();
        if (!cancelled) setGroups(parseOpenApi(doc));
      } catch (err) {
        if (!cancelled) setError(err instanceof Error ? err.message : "Failed to load the endpoint list");
      } finally {
        if (!cancelled) setLoading(false);
      }
    })();
    return () => {
      cancelled = true;
    };
  }, []);

  const visible = useMemo(() => filterEndpoints(groups, query), [groups, query]);
  const total = countEndpoints(groups);
  const shown = countEndpoints(visible);

  return (
    <Panel
      title="Endpoints"
      meta={query ? `${shown}/${total}` : total}
      description={
        <>
          Every route of this server, from{" "}
          <a className="font-mono text-brand underline-offset-4 hover:underline" href={`${API_URL}/api/openapi.json`} onClick={(e) => {
            e.preventDefault();
            void openSpec();
          }}>
            /api/openapi.json
          </a>{" "}
          (OpenAPI 3.1).
        </>
      }
    >
      <div className="border-b border-border p-4 sm:px-5">
        <div className="relative">
          <Search className="pointer-events-none absolute left-3 top-1/2 size-4 -translate-y-1/2 text-subtle" />
          <Input
            value={query}
            onChange={(e) => setQuery(e.target.value)}
            placeholder="Search: deploy, env, POST /api/projects…"
            aria-label="Search endpoints"
            className="pl-9"
          />
        </div>
        <p className="mt-2.5 text-xs leading-[1.125rem] text-muted-foreground">
          <span className="font-mono text-foreground">read</span> works with a read-only token.{" "}
          <span className="font-mono text-warn">dangerous</span> needs a token that allows dangerous actions.{" "}
          <span className="font-mono text-foreground">dashboard only</span> never accepts a token.
        </p>
      </div>
      {error && <div className="m-4 rounded-md border border-danger/40 bg-secondary/60 px-3 py-2.5 text-[13px] sm:mx-5">{error}</div>}
      {loading ? (
        <div className="flex justify-center py-10">
          <Loader2 className="size-5 animate-spin text-subtle" />
        </div>
      ) : visible.length === 0 ? (
        <p className="px-4 py-10 text-center text-[13.5px] text-muted-foreground">No endpoint matches.</p>
      ) : (
        <div className="divide-y divide-border">
          {visible.map((g) => (
            <details key={g.tag} open={Boolean(query)} className="group/tag">
              <summary className="flex cursor-pointer list-none items-center gap-2 px-4 py-2.5 hover:bg-secondary/50 sm:px-5 [&::-webkit-details-marker]:hidden">
                <ChevronRight className="size-4 shrink-0 text-subtle transition-transform duration-150 group-open/tag:rotate-90" />
                <span className="text-[13.5px] font-medium">{g.tag}</span>
                <span className="font-mono text-[13px] text-subtle tabular-nums">{g.endpoints.length}</span>
                {g.description && <span className="hidden truncate text-[13px] text-muted-foreground sm:inline">· {g.description}</span>}
              </summary>
              <ul className="divide-y divide-border border-t border-border bg-secondary/20">
                {g.endpoints.map((e) => (
                  <EndpointRow key={`${e.method} ${e.path}`} endpoint={e} />
                ))}
              </ul>
            </details>
          ))}
        </div>
      )}
    </Panel>
  );
}

async function openSpec() {
  const res = await authFetch(`${API_URL}/api/openapi.json`);
  if (!res.ok) {
    toast.error(await errorOf(res, "Failed to load the document"));
    return;
  }
  const blob = new Blob([JSON.stringify(await res.json(), null, 2)], { type: "application/json" });
  const href = URL.createObjectURL(blob);
  const a = document.createElement("a");
  a.href = href;
  a.download = "docklift-openapi.json";
  a.click();
  window.setTimeout(() => URL.revokeObjectURL(href), 1000);
}

function EndpointRow({ endpoint: e }: { endpoint: Endpoint }) {
  const fields = [...e.params.filter((p) => p.in !== "header"), ...e.body.map((b) => ({ ...b, in: "body" }))];
  return (
    <li>
      <details className="group/ep">
        <summary className="flex cursor-pointer list-none flex-wrap items-center gap-x-3 gap-y-1 px-4 py-2 hover:bg-secondary/50 sm:pl-11 sm:pr-5 [&::-webkit-details-marker]:hidden">
          <span className={cn("w-14 shrink-0 font-mono text-xs font-medium", e.method === "DELETE" ? "text-danger" : "text-muted-foreground")}>{e.method}</span>
          <span className="min-w-0 break-all font-mono text-[13px] text-foreground">{e.path}</span>
          <span className="min-w-0 flex-1 truncate text-[13px] text-muted-foreground">{e.summary}</span>
          <span className="flex flex-wrap gap-1">
            {e.stepUp && <Tag warn>dangerous</Tag>}
            {e.sessionOnly && <Tag>dashboard only</Tag>}
            {e.readScope && <Tag>read</Tag>}
            {e.stream && <Tag>stream</Tag>}
            {e.auth === "public" && <Tag>public</Tag>}
          </span>
        </summary>
        <div className="space-y-2 px-4 pb-3 sm:pl-11 sm:pr-5">
          {e.description && <p className="text-xs leading-[1.125rem] whitespace-pre-line text-muted-foreground">{e.description}</p>}
          {fields.length > 0 ? (
            <table className="w-full text-left text-xs">
              <tbody className="divide-y divide-border">
                {fields.map((f) => (
                  <tr key={`${f.in}:${f.name}`} className="align-top">
                    <td className="py-1.5 pr-3 font-mono text-foreground whitespace-nowrap">
                      {f.name}
                      {f.required && <span className="text-danger">*</span>}
                    </td>
                    <td className="py-1.5 pr-3 font-mono text-subtle whitespace-nowrap">{f.in}</td>
                    <td className="py-1.5 pr-3 font-mono text-subtle whitespace-nowrap">{f.type}</td>
                    <td className="py-1.5 text-muted-foreground">{f.description}</td>
                  </tr>
                ))}
              </tbody>
            </table>
          ) : (
            <p className="text-xs text-subtle">No parameters.</p>
          )}
        </div>
      </details>
    </li>
  );
}
