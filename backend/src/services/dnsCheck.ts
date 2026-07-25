// DNS preflight for custom domains: does the hostname actually point at this server?
// Used before asking Let's Encrypt for a certificate so a missing record fails fast
// with an actionable message instead of burning an ACME failure.
import dns from 'dns/promises';

export type DnsCheckStatus = 'ok' | 'mismatch' | 'missing' | 'unknown';

export interface DomainDnsCheck {
  domain: string;
  status: DnsCheckStatus;
  a: string[];
  aaaa: string[];
  serverIp: string | null;
  message: string;
}

const IP_CACHE_TTL = 5 * 60 * 1000;
let cachedServerIp: string | null = null;
let cachedServerIpAt = 0;

export async function getServerPublicIp(): Promise<string | null> {
  if (cachedServerIpAt && Date.now() - cachedServerIpAt < IP_CACHE_TTL) {
    return cachedServerIp;
  }
  // Stamp before the call so a server without internet access does not pay the
  // lookup timeout on every certificate attempt.
  cachedServerIpAt = Date.now();
  try {
    const controller = new AbortController();
    const timer = setTimeout(() => controller.abort(), 4000);
    const res = await fetch('https://api.ipify.org?format=json', { signal: controller.signal });
    clearTimeout(timer);
    if (res.ok) {
      const data = (await res.json()) as { ip?: string };
      if (data.ip) {
        cachedServerIp = data.ip;
      }
    }
  } catch {
    /* offline or blocked — callers treat a null IP as "cannot verify" */
  }
  return cachedServerIp;
}

const LOOKUP_TIMEOUT_MS = 5000;

async function lookup(
  hostname: string,
  family: 4 | 6
): Promise<{ records: string[]; noRecord: boolean; lookupFailed: boolean }> {
  let timer: NodeJS.Timeout | undefined;
  const timeout = new Promise<never>((_, reject) => {
    timer = setTimeout(
      () => reject(Object.assign(new Error('DNS lookup timed out'), { code: 'ETIMEOUT' })),
      LOOKUP_TIMEOUT_MS
    );
  });

  try {
    const query = family === 4 ? dns.resolve4(hostname) : dns.resolve6(hostname);
    const records = await Promise.race([query, timeout]);
    return { records, noRecord: records.length === 0, lookupFailed: false };
  } catch (error: any) {
    // ENOTFOUND/ENODATA mean the resolver answered: there is no such record.
    // Anything else (SERVFAIL, timeout, no resolver) is our problem, not the user's.
    const code = error?.code;
    if (code === 'ENOTFOUND' || code === 'ENODATA') {
      return { records: [], noRecord: true, lookupFailed: false };
    }
    return { records: [], noRecord: false, lookupFailed: true };
  } finally {
    if (timer) clearTimeout(timer);
  }
}

export async function checkDomainDns(hostname: string): Promise<DomainDnsCheck> {
  const domain = hostname.trim().toLowerCase();
  const [serverIp, v4, v6] = await Promise.all([
    getServerPublicIp(),
    lookup(domain, 4),
    lookup(domain, 6),
  ]);

  const base = { domain, a: v4.records, aaaa: v6.records, serverIp };

  if (v4.noRecord && v6.noRecord) {
    return {
      ...base,
      status: 'missing',
      message: `No A or AAAA record for ${domain}. Add an A record pointing at ${serverIp || 'this server'}, then retry.`,
    };
  }

  if (v4.records.length === 0 && v6.records.length === 0) {
    return {
      ...base,
      status: 'unknown',
      message: `Could not resolve ${domain} from this server (DNS lookup failed). Issuance will still be attempted.`,
    };
  }

  const all = [...v4.records, ...v6.records];

  if (!serverIp) {
    return {
      ...base,
      status: 'unknown',
      message: `${domain} resolves to ${all.join(', ')}. This server's public IP could not be confirmed.`,
    };
  }

  if (v4.records.includes(serverIp)) {
    return { ...base, status: 'ok', message: `${domain} points at this server (${serverIp}).` };
  }

  return {
    ...base,
    status: 'mismatch',
    message: `${domain} resolves to ${all.join(', ')}, not this server (${serverIp}). If Cloudflare's proxy is enabled this is expected — otherwise fix the A record.`,
  };
}

export async function checkDomainsDns(hostnames: string[]): Promise<DomainDnsCheck[]> {
  const checks: DomainDnsCheck[] = [];
  for (const host of hostnames) {
    checks.push(await checkDomainDns(host));
  }
  return checks;
}
