// Maps a certbot error summary to the fix the user actually has to perform.

export interface SslFix {
  title: string;
  steps: string[];
}

export function sslFixFor(error: string | null | undefined, serverIP?: string): SslFix | null {
  if (!error) return null;
  const text = error.toLowerCase();
  const target = serverIP && serverIP !== "N/A" ? serverIP : "this server's IP";

  if (text.includes("nxdomain") || text.includes("dns problem") || text.includes("dns record missing")) {
    return {
      title: "DNS does not resolve yet",
      steps: [
        `Create an A record for this hostname pointing at ${target}.`,
        "If the record exists, wait for propagation (usually 1–5 minutes, up to an hour).",
        "Then press Retry HTTPS.",
      ],
    };
  }

  if (text.includes("rate limit") || text.includes("too many")) {
    return {
      title: "Let's Encrypt rate limit hit",
      steps: [
        "Wait an hour before retrying — failed orders are throttled per account.",
        "Fix DNS first so the next attempt succeeds.",
        "For repeated testing, set CERTBOT_STAGING=true to use the staging CA.",
      ],
    };
  }

  if (text.includes("acme email") || text.includes("no acme")) {
    return {
      title: "No ACME account email configured",
      steps: [
        "Open Settings and set the SSL contact email (or set CERTBOT_EMAIL).",
        "Then press Retry HTTPS.",
      ],
    };
  }

  if (
    text.includes("timeout") ||
    text.includes("timed out") ||
    text.includes("connection") ||
    text.includes("refused") ||
    text.includes("unauthorized") ||
    text.includes("403") ||
    text.includes("404") ||
    text.includes("invalid response")
  ) {
    return {
      title: "Let's Encrypt could not reach the challenge",
      steps: [
        "Open ports 80 and 443 on the server firewall — HTTP-01 validation uses port 80.",
        "Confirm DNS points at this server and no other service answers on port 80.",
        "On Cloudflare, set SSL mode to Full (strict) and do not block /.well-known/acme-challenge/.",
      ],
    };
  }

  if (text.includes("no such container") || text.includes("docker") || text.includes("certbot")) {
    return {
      title: "Certbot container is unavailable",
      steps: [
        "Check the sidecar: docker ps --filter name=docklift-certbot",
        "Start the stack again: docker compose up -d",
        "Then press Retry HTTPS.",
      ],
    };
  }

  return {
    title: "Certificate could not be issued",
    steps: [
      "Read the certbot log with the command below for the full reason.",
      `Verify DNS for this hostname resolves to ${target}.`,
      "Fix the cause, then press Retry HTTPS.",
    ],
  };
}
