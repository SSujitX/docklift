// Pure helpers of the API & MCP page: token rows, MCP client snippets and the
// endpoint browser built from /api/openapi.json.

export type TokenScope = "read" | "full";

export interface ApiToken {
  id: string;
  name: string;
  prefix: string;
  user_id: string;
  scope: TokenScope;
  allow_dangerous: boolean;
  created_at: string;
  last_used_at: string | null;
  expires_at: string | null;
  status: "active" | "expired" | "revoked";
}

/** Placeholder shown in snippets until a token is created on this page. */
export const TOKEN_PLACEHOLDER = "dl_YOUR_TOKEN";

/** The MCP endpoint of this dashboard (same origin as the API). */
export function mcpUrl(origin: string, apiUrl = ""): string {
  const base = (apiUrl || origin).replace(/\/+$/, "");
  return `${base}/mcp`;
}

/** `claude mcp add` command for Claude Code. */
export function claudeCodeCommand(url: string, token: string): string {
  return `claude mcp add --transport http docklift ${url} --header "Authorization: Bearer ${token}"`;
}

/** Generic MCP client config (Streamable HTTP with a bearer header). */
export function genericMcpConfig(url: string, token: string): string {
  return JSON.stringify(
    {
      mcpServers: {
        docklift: {
          type: "http",
          url,
          headers: { Authorization: `Bearer ${token}` },
        },
      },
    },
    null,
    2,
  );
}

/** curl call against the REST API with the token. */
export function curlExample(origin: string, token: string): string {
  return `curl -H "Authorization: Bearer ${token}" ${origin.replace(/\/+$/, "")}/api/projects`;
}

export const EXPIRY_OPTIONS: { label: string; days: number | null }[] = [
  { label: "30 days", days: 30 },
  { label: "90 days", days: 90 },
  { label: "1 year", days: 365 },
  { label: "Never", days: null },
];

/** Relative description of a timestamp ("never" for null). */
export function describeTime(iso: string | null, now = Date.now(), empty = "Never"): string {
  if (!iso) return empty;
  const t = Date.parse(iso);
  if (Number.isNaN(t)) return empty;
  const diff = t - now;
  const abs = Math.abs(diff);
  const units: [number, string][] = [
    [365 * 24 * 3600e3, "year"],
    [30 * 24 * 3600e3, "month"],
    [24 * 3600e3, "day"],
    [3600e3, "hour"],
    [60e3, "minute"],
  ];
  for (const [ms, name] of units) {
    if (abs >= ms) {
      const n = Math.floor(abs / ms);
      const label = `${n} ${name}${n === 1 ? "" : "s"}`;
      return diff < 0 ? `${label} ago` : `in ${label}`;
    }
  }
  return diff < 0 ? "just now" : "in under a minute";
}

/* ---------- endpoint browser ---------- */

export interface EndpointField {
  name: string;
  type: string;
  required: boolean;
  description?: string;
  in?: string;
}

export interface Endpoint {
  method: string;
  path: string;
  tag: string;
  summary: string;
  description?: string;
  stepUp: boolean;
  sessionOnly: boolean;
  readScope: boolean;
  stream?: string;
  auth: string;
  params: EndpointField[];
  body: EndpointField[];
}

export interface EndpointGroup {
  tag: string;
  description?: string;
  endpoints: Endpoint[];
}

type Json = Record<string, unknown>;

function asObj(v: unknown): Json {
  return v && typeof v === "object" && !Array.isArray(v) ? (v as Json) : {};
}

function schemaType(s: Json): string {
  const t = s.type;
  if (Array.isArray(t)) return t.join(" | ");
  if (t === "array") return `${schemaType(asObj(s.items)) || "any"}[]`;
  if (t === "string" && s.format === "binary") return "file";
  return typeof t === "string" ? t : "any";
}

const METHOD_ORDER = ["get", "post", "put", "patch", "delete"];

/** Flattens an OpenAPI document into endpoint groups in tag order. */
export function parseOpenApi(doc: unknown): EndpointGroup[] {
  const root = asObj(doc);
  const paths = asObj(root.paths);
  const tagMeta = Array.isArray(root.tags) ? (root.tags as Json[]) : [];
  const groups = new Map<string, EndpointGroup>();
  for (const t of tagMeta) {
    const name = String(t.name ?? "");
    if (name) groups.set(name, { tag: name, description: t.description as string | undefined, endpoints: [] });
  }
  for (const [path, item] of Object.entries(paths)) {
    const ops = asObj(item);
    for (const method of METHOD_ORDER) {
      const op = asObj(ops[method]);
      if (!Object.keys(op).length) continue;
      const tag = Array.isArray(op.tags) && op.tags.length ? String(op.tags[0]) : "Other";
      const params = (Array.isArray(op.parameters) ? (op.parameters as Json[]) : []).map((p) => ({
        name: String(p.name),
        in: String(p.in),
        type: schemaType(asObj(p.schema)),
        required: p.required === true,
        description: p.description as string | undefined,
      }));
      const content = asObj(asObj(op.requestBody).content);
      const schema = asObj(asObj(content["application/json"] ?? content["multipart/form-data"]).schema);
      const required = new Set(Array.isArray(schema.required) ? (schema.required as string[]) : []);
      const body = Object.entries(asObj(schema.properties)).map(([name, raw]) => {
        const s = asObj(raw);
        return { name, type: schemaType(s), required: required.has(name), description: s.description as string | undefined };
      });
      const endpoint: Endpoint = {
        method: method.toUpperCase(),
        path,
        tag,
        summary: String(op.summary ?? ""),
        description: op.description as string | undefined,
        stepUp: op["x-docklift-step-up"] === true,
        sessionOnly: op["x-docklift-session-only"] === true,
        readScope: op["x-docklift-read-scope"] === true,
        stream: op["x-docklift-stream"] as string | undefined,
        auth: String(op["x-docklift-auth"] ?? "session"),
        params,
        body,
      };
      if (!groups.has(tag)) groups.set(tag, { tag, endpoints: [] });
      groups.get(tag)!.endpoints.push(endpoint);
    }
  }
  return [...groups.values()].filter((g) => g.endpoints.length > 0);
}

/** Groups narrowed to endpoints whose method, path, summary or tag match every word of `query`. */
export function filterEndpoints(groups: EndpointGroup[], query: string): EndpointGroup[] {
  const words = query.toLowerCase().split(/\s+/).filter(Boolean);
  if (!words.length) return groups;
  return groups
    .map((g) => ({
      ...g,
      endpoints: g.endpoints.filter((e) => {
        const hay = `${e.method} ${e.path} ${e.summary} ${e.tag}`.toLowerCase();
        return words.every((w) => hay.includes(w));
      }),
    }))
    .filter((g) => g.endpoints.length > 0);
}

export function countEndpoints(groups: EndpointGroup[]): number {
  return groups.reduce((n, g) => n + g.endpoints.length, 0);
}
