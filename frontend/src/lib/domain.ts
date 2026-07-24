// Domain input handling: users paste full URLs ("https://app.example.com/"), so
// normalize to a bare hostname and tell them exactly what was cleaned up.

export interface DomainInputResult {
  /** Bare lowercase hostname, or "" when the input cannot be used. */
  value: string;
  /** Non-blocking clean-ups we applied, shown as hints under the field. */
  notices: string[];
  /** Blocking problem — the domain cannot be added. */
  error: string | null;
}

const HOSTNAME_REGEX =
  /^[a-z0-9]([a-z0-9-]*[a-z0-9])?(\.[a-z0-9]([a-z0-9-]*[a-z0-9])?)+$/;

const IPV4_REGEX = /^\d{1,3}(\.\d{1,3}){3}$/;

export function normalizeDomainInput(raw: string): DomainInputResult {
  const notices: string[] = [];
  let value = (raw || "").trim();

  if (!value) return { value: "", notices, error: null };

  const schemeMatch = /^([a-z][a-z0-9+.-]*):\/\//i.exec(value);
  if (schemeMatch) {
    notices.push(`Removed "${schemeMatch[1].toLowerCase()}://"`);
    value = value.slice(schemeMatch[0].length);
  } else if (value.startsWith("//")) {
    notices.push('Removed "//"');
    value = value.slice(2);
  }

  const atIndex = value.indexOf("@");
  if (atIndex !== -1) {
    notices.push("Removed credentials before @");
    value = value.slice(atIndex + 1);
  }

  const pathIndex = value.search(/[/?#]/);
  if (pathIndex !== -1) {
    const tail = value.slice(pathIndex);
    notices.push(tail === "/" ? "Removed trailing slash" : `Removed path "${tail}"`);
    value = value.slice(0, pathIndex);
  }

  const portMatch = /:(\d+)$/.exec(value);
  if (portMatch) {
    notices.push(`Removed port ":${portMatch[1]}" — routing uses the service port`);
    value = value.slice(0, portMatch.index);
  }

  if (value.endsWith(".")) {
    value = value.slice(0, -1);
  }

  if (value !== value.toLowerCase()) {
    notices.push("Lowercased");
    value = value.toLowerCase();
  }

  // DNS and ACME only speak ASCII: let the URL parser punycode international names.
  if (value && /[^\x20-\x7e]/.test(value)) {
    try {
      const ascii = new URL(`http://${value}`).hostname;
      if (ascii && ascii !== value) {
        notices.push(`Converted to punycode (${ascii})`);
        value = ascii;
      }
    } catch {
      /* fall through to the hostname validation below */
    }
  }

  if (!value) {
    return { value: "", notices, error: "Enter a hostname like app.example.com" };
  }

  if (value.startsWith("*.")) {
    return {
      value: "",
      notices,
      error: "Wildcard domains need DNS-01 validation, which Docklift does not issue.",
    };
  }

  if (IPV4_REGEX.test(value) || value.includes(":")) {
    return {
      value: "",
      notices,
      error: "Use a hostname — Let's Encrypt cannot issue certificates for IP addresses.",
    };
  }

  if (!value.includes(".")) {
    return {
      value: "",
      notices,
      error: "Use a full hostname with a public suffix, like app.example.com",
    };
  }

  if (value === "localhost" || value.endsWith(".localhost") || value.endsWith(".local")) {
    return {
      value: "",
      notices,
      error: "Local-only hostnames cannot get a public certificate.",
    };
  }

  if (value.length > 253) {
    return { value: "", notices, error: "Hostname is longer than 253 characters." };
  }

  if (!HOSTNAME_REGEX.test(value)) {
    return {
      value: "",
      notices,
      error: "Invalid hostname. Use letters, digits and hyphens, like app.example.com",
    };
  }

  return { value, notices, error: null };
}

// Second-level labels that sit under a two-letter ccTLD (example.co.uk,
// example.com.au), so the registrable domain is three labels, not two.
const CC_SECOND_LEVEL = new Set([
  "co",
  "com",
  "net",
  "org",
  "gov",
  "edu",
  "ac",
  "or",
  "ne",
  "go",
  "in",
]);

/** Suggested DNS record for a hostname, relative to its registrable domain. */
export function dnsRecordHint(hostname: string): { type: "A"; name: string } {
  const labels = hostname.split(".");
  const tld = labels[labels.length - 1] || "";
  const sld = labels[labels.length - 2] || "";
  const registrableLabels =
    labels.length >= 3 && tld.length === 2 && CC_SECOND_LEVEL.has(sld) ? 3 : 2;

  return {
    type: "A",
    name:
      labels.length <= registrableLabels
        ? "@"
        : labels.slice(0, labels.length - registrableLabels).join("."),
  };
}
