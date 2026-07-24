// Browser origin trust checks shared by the CORS layer and the terminal WebSocket handshake.
// DockLift publishes user apps on the host port pool (5500-5600), so a same-hostname check
// is not enough: the port is what separates the dashboard from untrusted deployed apps.
import type { IncomingHttpHeaders } from 'http';

interface ParsedOrigin {
  scheme: 'http:' | 'https:';
  hostname: string;
  port: string;
}

function defaultPort(scheme: string): string {
  return scheme === 'https:' ? '443' : '80';
}

export function parseOrigin(value: string): ParsedOrigin | null {
  try {
    const u = new URL(value);
    if (u.protocol !== 'http:' && u.protocol !== 'https:') return null;
    return {
      scheme: u.protocol,
      hostname: u.hostname.toLowerCase(),
      port: u.port || defaultPort(u.protocol),
    };
  } catch {
    return null;
  }
}

/** Canonical `scheme//host:port` form, or null when the value is not an http(s) origin. */
export function normalizeOrigin(value: string): string | null {
  const parsed = parseOrigin(value);
  return parsed ? `${parsed.scheme}//${parsed.hostname}:${parsed.port}` : null;
}

function firstHeader(headers: IncomingHttpHeaders, name: string): string {
  const raw = headers[name];
  const value = Array.isArray(raw) ? raw[0] : raw;
  return (value || '').toString().split(',')[0].trim();
}

/** The origin the browser addressed, reconstructed from the forwarded Host/proto. */
export function requestOrigin(
  headers: IncomingHttpHeaders,
  fallbackProto = 'http'
): ParsedOrigin | null {
  const host = firstHeader(headers, 'x-forwarded-host') || firstHeader(headers, 'host');
  if (!host) return null;
  const proto = firstHeader(headers, 'x-forwarded-proto') || fallbackProto;
  return parseOrigin(`${proto === 'https' ? 'https' : 'http'}://${host}`);
}

function sameOrigin(a: ParsedOrigin | null, b: ParsedOrigin | null): boolean {
  if (!a || !b) return false;
  if (a.hostname !== b.hostname) return false;
  if (a.scheme === b.scheme) return a.port === b.port;
  // A proxy that terminates TLS without a trustworthy X-Forwarded-Proto makes an https
  // panel look like http here. Tolerate the scheme mismatch only when both sides sit on
  // their own default port, which no deployed app in the host port pool ever does.
  return a.port === defaultPort(a.scheme) && b.port === defaultPort(b.scheme);
}

/**
 * True when `origin` is the dashboard itself (same scheme + host + port as this request)
 * or an operator-configured origin. Allowlist entries must match exactly — the scheme
 * there is a deliberate choice, not a proxy artifact.
 */
export function isTrustedOrigin(
  origin: string,
  headers: IncomingHttpHeaders,
  opts: { fallbackProto?: string; allow?: (string | undefined | null)[] } = {}
): boolean {
  const parsed = parseOrigin(origin);
  if (!parsed) return false;
  const normalized = normalizeOrigin(origin);

  const allowlist = [
    ...(process.env.CORS_ORIGIN ? process.env.CORS_ORIGIN.split(',') : []),
    ...(opts.allow || []),
  ];
  for (const entry of allowlist) {
    const trimmed = (entry || '').trim();
    if (trimmed && normalizeOrigin(trimmed) === normalized) return true;
  }

  return sameOrigin(parsed, requestOrigin(headers, opts.fallbackProto));
}
