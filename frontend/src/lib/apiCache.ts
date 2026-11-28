// In-memory stale-while-revalidate cache for authenticated GET requests.
//
// Pages keep their own state and polling. They read the last known body with `peek()` to draw
// immediately on a revisit, and load through `fetch()`, which always goes to the network (so polls
// stay live) but shares one request between identical GETs that are in flight at the same time.
//
// Correctness rules:
// - Only 2xx responses are stored. A failed refresh leaves the previous body in place and hands the
//   caller the failed response, so the page shows the error state it already had.
// - `invalidate()` drops every entry and forgets in-flight requests; a response that started before
//   an invalidation is returned to its caller but never stored. The app invalidates on every
//   mutating request (see `authFetch`), on sign-in, sign-out and on a 401.
// - Memory only. Nothing is written to localStorage or sessionStorage. Endpoints that return
//   secrets (env values, database credentials, source files) are not loaded through this cache.

export type CacheFetcher = (url: string) => Promise<Response>;

interface Snapshot {
  status: number;
  statusText: string;
  contentType: string | null;
  body: string;
}

interface Entry extends Snapshot {
  storedAt: number;
}

export interface ApiCacheOptions {
  /** Most entries kept; the least recently used one is dropped first. */
  maxEntries?: number;
  /** `prefetch()` does nothing while an entry is younger than this. */
  freshMs?: number;
  now?: () => number;
}

export interface ApiCache {
  /** GET `url`: network every time, de-duplicated while in flight; 2xx bodies are stored. */
  fetch(url: string, init?: { signal?: AbortSignal | null }): Promise<Response>;
  /** Last stored JSON body for `url`, parsed fresh on each call, or undefined. */
  peek<T = unknown>(url: string): T | undefined;
  /** When the body for `url` was stored (ms since epoch), or undefined. */
  storedAt(url: string): number | undefined;
  /** Warm `url` unless it is fresh or already loading. Never throws. */
  prefetch(url: string): void;
  /** Forget everything (after a mutation). In-flight responses are not stored. */
  invalidate(): void;
  /** Same as invalidate; used on sign-in, sign-out and 401 to make the intent explicit. */
  clear(): void;
  /** Number of stored entries (tests). */
  size(): number;
}

const NULL_BODY_STATUSES = new Set([101, 204, 205, 304]);

function toResponse(snap: Snapshot): Response {
  const headers = new Headers();
  if (snap.contentType) headers.set("content-type", snap.contentType);
  return new Response(NULL_BODY_STATUSES.has(snap.status) ? null : snap.body, {
    status: snap.status,
    statusText: snap.statusText,
    headers,
  });
}

function abortError(): Error {
  try {
    return new DOMException("The operation was aborted.", "AbortError");
  } catch {
    const error = new Error("The operation was aborted.");
    error.name = "AbortError";
    return error;
  }
}

/** Resolve with `promise` unless `signal` aborts first; the shared request itself keeps going. */
function withSignal<T>(promise: Promise<T>, signal?: AbortSignal | null): Promise<T> {
  if (!signal) return promise;
  if (signal.aborted) return Promise.reject(abortError());
  return new Promise<T>((resolve, reject) => {
    const onAbort = () => reject(abortError());
    signal.addEventListener("abort", onAbort, { once: true });
    promise.then(
      (value) => {
        signal.removeEventListener("abort", onAbort);
        resolve(value);
      },
      (error) => {
        signal.removeEventListener("abort", onAbort);
        reject(error);
      },
    );
  });
}

export function createApiCache(fetcher: CacheFetcher, options: ApiCacheOptions = {}): ApiCache {
  const maxEntries = options.maxEntries ?? 100;
  const freshMs = options.freshMs ?? 10_000;
  const now = options.now ?? Date.now;

  const entries = new Map<string, Entry>();
  const inflight = new Map<string, Promise<Snapshot>>();
  /** Bumped by invalidate/clear; a response is stored only if the generation did not change. */
  let generation = 0;

  function store(url: string, snap: Snapshot) {
    entries.delete(url);
    entries.set(url, { ...snap, storedAt: now() });
    while (entries.size > maxEntries) {
      const oldest = entries.keys().next().value;
      if (oldest === undefined) break;
      entries.delete(oldest);
    }
  }

  function load(url: string): Promise<Snapshot> {
    const existing = inflight.get(url);
    if (existing) return existing;

    const startGeneration = generation;
    const request = (async () => {
      const res = await fetcher(url);
      const body = await res.text();
      const snap: Snapshot = {
        status: res.status,
        statusText: res.statusText,
        contentType: res.headers.get("content-type"),
        body,
      };
      if (res.ok && startGeneration === generation) store(url, snap);
      return snap;
    })();

    inflight.set(url, request);
    const settle = () => {
      if (inflight.get(url) === request) inflight.delete(url);
    };
    request.then(settle, settle);
    return request;
  }

  function invalidate() {
    generation += 1;
    entries.clear();
    inflight.clear();
  }

  return {
    async fetch(url, init) {
      const snap = await withSignal(load(url), init?.signal);
      return toResponse(snap);
    },

    peek<T>(url: string): T | undefined {
      const entry = entries.get(url);
      if (!entry) return undefined;
      try {
        return JSON.parse(entry.body) as T;
      } catch {
        return undefined;
      }
    },

    storedAt(url) {
      return entries.get(url)?.storedAt;
    },

    prefetch(url) {
      if (inflight.has(url)) return;
      const entry = entries.get(url);
      if (entry && now() - entry.storedAt < freshMs) return;
      load(url).catch(() => {
        /* prefetch is best effort; the page reports its own errors */
      });
    },

    invalidate,
    clear: invalidate,

    size() {
      return entries.size;
    },
  };
}
