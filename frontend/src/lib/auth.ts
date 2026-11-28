// Utility hook for making authenticated API calls

import { API_URL } from "./utils";
import { createApiCache } from "./apiCache";

let unauthorizedHandler: (() => void) | null = null;

/** Called once from AuthProvider so 401 responses clear session app-wide. */
export function registerAuthUnauthorizedHandler(handler: () => void): void {
  unauthorizedHandler = handler;
}

function handleUnauthorized(response: Response): void {
  if (response.status === 401 && unauthorizedHandler) {
    unauthorizedHandler();
  }
}

// Get auth headers for API calls
export function getAuthHeaders(): HeadersInit {
  if (typeof window === "undefined") return {};
  
  const token = localStorage.getItem("docklift_token");
  return token ? { Authorization: `Bearer ${token}` } : {};
}

/** Start GitHub App install: sets HttpOnly nonce cookie, returns GitHub URL to open/navigate. */
export async function startGithubInstallSession(returnUrl?: string): Promise<string> {
  const res = await authFetch(`${API_URL}/api/github/install-session`, {
    method: "POST",
    headers: {
      "Content-Type": "application/json",
    },
    credentials: "include",
    body: JSON.stringify({ returnUrl: returnUrl || window.location.href }),
  });
  const data = await res.json().catch(() => ({}));
  if (!res.ok || !data.installUrl) {
    throw new Error(data.error || "Failed to start GitHub install");
  }
  return data.installUrl as string;
}

/**
 * Start install session then open GitHub.
 * Opens a blank tab synchronously (before await) so popup blockers do not kill it.
 * Pass `{ sameTab: true }` to navigate the current tab instead.
 */
export async function startGithubInstallAndNavigate(
  returnUrl?: string,
  opts?: { sameTab?: boolean }
): Promise<void> {
  const sameTab = opts?.sameTab === true;
  const popup = sameTab ? null : window.open("about:blank", "_blank");
  try {
    const installUrl = await startGithubInstallSession(returnUrl);
    if (popup && !popup.closed) {
      try {
        popup.opener = null;
      } catch {
        /* ignore */
      }
      popup.location.href = installUrl;
      return;
    }
    window.location.href = installUrl;
  } catch (error) {
    if (popup && !popup.closed) popup.close();
    throw error;
  }
}

/**
 * App-wide GET cache (memory only). Pages call `apiCache.fetch(url)` for primary data and
 * `apiCache.peek(url)` to draw the last known data at once. See `lib/apiCache.ts`.
 */
export const apiCache = createApiCache((url) => authFetch(url));

// Another tab signed in or out: whatever this tab cached may belong to another session.
if (typeof window !== "undefined") {
  window.addEventListener("storage", (event) => {
    if (event.key === null || event.key === "docklift_token") apiCache.clear();
  });
}

/** POSTs that only mint short-lived tokens or run read-only checks; they change no cached data. */
const READ_ONLY_POST = [
  /\/api\/auth\/(sse|terminal|setup)-token(\?|$)/,
  /\/dns-check(\?|$)/,
  /\/api\/github\/install-session(\?|$)/,
];

function isMutation(url: string, options: RequestInit): boolean {
  const method = (options.method || "GET").toUpperCase();
  if (method === "GET" || method === "HEAD" || method === "OPTIONS") return false;
  return !READ_ONLY_POST.some((pattern) => pattern.test(url));
}

// Authenticated fetch wrapper
export async function authFetch(url: string, options: RequestInit = {}): Promise<Response> {
  const headers = {
    ...getAuthHeaders(),
    ...options.headers,
  };

  // Any change can touch more than one resource (a deploy moves project status, ports and
  // stats), so a mutation drops the whole GET cache before it is sent and again once it answers,
  // which also discards reads that raced with it.
  const mutation = isMutation(url, options);
  if (mutation) apiCache.invalidate();
  let response: Response;
  try {
    response = await fetch(url, {
      ...options,
      headers,
    });
  } finally {
    if (mutation) apiCache.invalidate();
  }
  if (response.status === 401) apiCache.clear();
  handleUnauthorized(response);
  return response;
}

// Typed fetch that handles auth and returns JSON
export async function fetchWithAuth<T>(endpoint: string, options: RequestInit = {}): Promise<T> {
  const url = endpoint.startsWith("http") ? endpoint : `${API_URL}${endpoint}`;
  
  const response = await authFetch(url, {
    ...options,
    headers: {
      "Content-Type": "application/json",
      ...options.headers,
    },
  });

  if (!response.ok) {
    if (response.status === 401) {
      throw new Error("Session expired");
    }
    const error = await response.json().catch(() => ({ error: "Request failed" }));
    throw new Error(error.error || `HTTP ${response.status}`);
  }

  return response.json();
}
