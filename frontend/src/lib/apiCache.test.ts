import { afterEach, describe, expect, test } from "bun:test";
import { createApiCache } from "./apiCache";
import { apiCache, authFetch, registerAuthUnauthorizedHandler } from "./auth";
import { dataForHref, API } from "./prefetch";
import { consumeProgressStream } from "./streamProgress";

/** A controllable fetcher: each call waits until the test resolves it. */
function deferredFetcher() {
  const calls: Array<{ url: string; resolve: (res: Response) => void; reject: (err: unknown) => void }> = [];
  const fetcher = (url: string) =>
    new Promise<Response>((resolve, reject) => {
      calls.push({ url, resolve, reject });
    });
  return { calls, fetcher };
}

const json = (body: unknown, status = 200) =>
  new Response(JSON.stringify(body), { status, headers: { "content-type": "application/json" } });

const tick = () => new Promise((r) => setTimeout(r, 0));

describe("createApiCache", () => {
  test("stale then fresh: a revisit reads the old body, the refresh replaces it", async () => {
    let n = 0;
    const cache = createApiCache(async () => json({ n: ++n }));

    expect(cache.peek("/a")).toBeUndefined();
    expect(await (await cache.fetch("/a")).json()).toEqual({ n: 1 });

    // Page mounts again: draws the stored body at once…
    expect(cache.peek("/a")).toEqual({ n: 1 });
    // …and its mount fetch still goes to the network.
    expect(await (await cache.fetch("/a")).json()).toEqual({ n: 2 });
    expect(cache.peek("/a")).toEqual({ n: 2 });
  });

  test("peek returns a fresh copy each time", async () => {
    const cache = createApiCache(async () => json([1, 2, 3]));
    await cache.fetch("/a");
    const first = cache.peek<number[]>("/a")!;
    first.push(4);
    expect(cache.peek("/a")).toEqual([1, 2, 3]);
  });

  test("identical in-flight GETs share one request; each caller gets its own body", async () => {
    const { calls, fetcher } = deferredFetcher();
    const cache = createApiCache(fetcher);

    const a = cache.fetch("/x");
    const b = cache.fetch("/x");
    const other = cache.fetch("/y");
    expect(calls.map((c) => c.url)).toEqual(["/x", "/y"]);

    calls[0].resolve(json({ ok: true }));
    calls[1].resolve(json({ y: true }));
    const [ra, rb] = await Promise.all([a, b]);
    expect(await ra.json()).toEqual({ ok: true });
    expect(await rb.json()).toEqual({ ok: true });
    expect(await (await other).json()).toEqual({ y: true });

    // Once settled, the next call is a new request (polling stays live).
    const c = cache.fetch("/x");
    expect(calls).toHaveLength(3);
    calls[2].resolve(json({ again: true }));
    expect(await (await c).json()).toEqual({ again: true });
  });

  test("invalidate drops entries and never stores a response that raced with it", async () => {
    const { calls, fetcher } = deferredFetcher();
    const cache = createApiCache(fetcher);

    const first = cache.fetch("/p");
    calls[0].resolve(json({ v: "before" }));
    await first;
    expect(cache.peek("/p")).toEqual({ v: "before" });

    // A read starts, then a mutation invalidates before the read answers.
    const racing = cache.fetch("/p");
    cache.invalidate();
    expect(cache.peek("/p")).toBeUndefined();

    // A read after the invalidation does not join the old in-flight request.
    const after = cache.fetch("/p");
    expect(calls).toHaveLength(3);

    calls[1].resolve(json({ v: "pre-mutation" }));
    expect(await (await racing).json()).toEqual({ v: "pre-mutation" }); // caller still gets it
    expect(cache.peek("/p")).toBeUndefined(); // but it is not stored

    calls[2].resolve(json({ v: "after" }));
    await after;
    expect(cache.peek("/p")).toEqual({ v: "after" });
  });

  test("clear (sign-out) forgets everything, including late responses of the old session", async () => {
    const { calls, fetcher } = deferredFetcher();
    const cache = createApiCache(fetcher);

    const a = cache.fetch("/a");
    calls[0].resolve(json({ user: "alice" }));
    await a;
    const late = cache.fetch("/b");
    cache.clear();
    calls[1].resolve(json({ user: "alice-late" }));
    await late;

    expect(cache.size()).toBe(0);
    expect(cache.peek("/a")).toBeUndefined();
    expect(cache.peek("/b")).toBeUndefined();
  });

  test("a failed refresh keeps the old data and hands the caller the error response", async () => {
    let mode: "ok" | "500" | "throw" = "ok";
    const cache = createApiCache(async () => {
      if (mode === "throw") throw new TypeError("network down");
      if (mode === "500") return json({ error: "boom" }, 500);
      return json({ items: [1] });
    });

    await cache.fetch("/list");
    mode = "500";
    const res = await cache.fetch("/list");
    expect(res.ok).toBe(false);
    expect(res.status).toBe(500);
    expect(await res.json()).toEqual({ error: "boom" }); // page builds its own error message
    expect(cache.peek("/list")).toEqual({ items: [1] });

    mode = "throw";
    await expect(cache.fetch("/list")).rejects.toThrow("network down");
    expect(cache.peek("/list")).toEqual({ items: [1] });

    // An error is never stored as data for a key that had none.
    mode = "500";
    await cache.fetch("/fresh-key");
    expect(cache.peek("/fresh-key")).toBeUndefined();
  });

  test("a caller's abort rejects only that caller; the shared request still lands", async () => {
    const { calls, fetcher } = deferredFetcher();
    const cache = createApiCache(fetcher);
    const controller = new AbortController();

    const aborted = cache.fetch("/h", { signal: controller.signal });
    const kept = cache.fetch("/h");
    controller.abort();
    await expect(aborted).rejects.toMatchObject({ name: "AbortError" });

    calls[0].resolve(json({ page: 0 }));
    expect(await (await kept).json()).toEqual({ page: 0 });
    expect(cache.peek("/h")).toEqual({ page: 0 });
  });

  test("prefetch skips fresh or loading data, refreshes stale data, and never throws", async () => {
    let now = 1_000;
    let calls = 0;
    let fail = false;
    const cache = createApiCache(
      async () => {
        calls += 1;
        if (fail) throw new Error("offline");
        return json({ calls });
      },
      { freshMs: 10_000, now: () => now },
    );

    cache.prefetch("/d");
    cache.prefetch("/d"); // already loading
    await tick();
    expect(calls).toBe(1);
    expect(cache.peek("/d")).toEqual({ calls: 1 });

    now += 5_000;
    cache.prefetch("/d"); // still fresh
    await tick();
    expect(calls).toBe(1);

    now += 6_000;
    cache.prefetch("/d"); // stale
    await tick();
    expect(calls).toBe(2);
    expect(cache.storedAt("/d")).toBe(now);

    fail = true;
    now += 20_000;
    cache.prefetch("/d");
    await tick();
    expect(cache.peek("/d")).toEqual({ calls: 2 });
  });

  test("keeps at most maxEntries, dropping the least recently stored", async () => {
    const cache = createApiCache(async (url) => json({ url }), { maxEntries: 2 });
    await cache.fetch("/1");
    await cache.fetch("/2");
    await cache.fetch("/1"); // refresh moves /1 to the newest slot
    await cache.fetch("/3");
    expect(cache.peek("/2")).toBeUndefined();
    expect(cache.peek("/1")).toEqual({ url: "/1" });
    expect(cache.peek("/3")).toEqual({ url: "/3" });
  });

  test("handles bodiless statuses", async () => {
    const cache = createApiCache(async () => new Response(null, { status: 204 }));
    const res = await cache.fetch("/empty");
    expect(res.status).toBe(204);
    expect(cache.peek("/empty")).toBeUndefined(); // not JSON
  });
});

describe("app wiring (authFetch + apiCache)", () => {
  const realFetch = globalThis.fetch;
  afterEach(() => {
    globalThis.fetch = realFetch;
    apiCache.clear();
    registerAuthUnauthorizedHandler(() => {});
  });

  function mockServer(handler: (url: string, init?: RequestInit) => Response) {
    const seen: string[] = [];
    globalThis.fetch = (async (input: RequestInfo | URL, init?: RequestInit) => {
      const url = String(input);
      seen.push(`${(init?.method || "GET").toUpperCase()} ${url}`);
      return handler(url, init);
    }) as typeof fetch;
    return seen;
  }

  test("a 401 clears the cache and signs out", async () => {
    let status = 200;
    mockServer(() => json({ projects: [] }, status));
    let signedOut = false;
    registerAuthUnauthorizedHandler(() => {
      signedOut = true;
    });

    await apiCache.fetch(API.projects);
    expect(apiCache.peek(API.projects)).toBeDefined();

    status = 401;
    await apiCache.fetch(API.ports);
    expect(signedOut).toBe(true);
    expect(apiCache.size()).toBe(0);
  });

  test("a mutation through authFetch invalidates cached reads", async () => {
    mockServer(() => json([{ id: "p1" }]));
    await apiCache.fetch(API.projects);
    expect(apiCache.peek(API.projects)).toBeDefined();

    await authFetch(`${API.projects}/p1`, { method: "DELETE" });
    expect(apiCache.peek(API.projects)).toBeUndefined();
  });

  test("a mutation discards a read that was in flight while it ran", async () => {
    let release: (() => void) | undefined;
    globalThis.fetch = (async (_input: RequestInfo | URL, init?: RequestInit) => {
      if ((init?.method || "GET") === "GET") {
        await new Promise<void>((r) => (release = r));
        return json({ status: "running" });
      }
      return json({ ok: true });
    }) as typeof fetch;

    const read = apiCache.fetch(API.projects);
    await tick();
    await authFetch(`${API.projects}/p1/stop`, { method: "POST" });
    release!();
    await read;
    expect(apiCache.peek(API.projects)).toBeUndefined();
  });

  test("read-only POSTs (SSE / terminal tokens, DNS checks) keep the cache", async () => {
    mockServer(() => json({ token: "short-lived" }));
    await apiCache.fetch(API.projects);
    await authFetch("/api/auth/sse-token", { method: "POST" });
    await authFetch("/api/domains/dns-check", { method: "POST" });
    expect(apiCache.peek(API.projects)).toBeDefined();
  });

  test("the end of a progress stream (backup, restore, deploy) invalidates", async () => {
    mockServer(() => json({ backups: [] }));
    await apiCache.fetch(API.backups);
    // Something refreshed the list while the stream was still running.
    const result = await consumeProgressStream(new Response("step 1\ndone\n"));
    expect(result.ok).toBe(true);
    expect(apiCache.peek(API.backups)).toBeUndefined();
  });
});

describe("dataForHref", () => {
  test("maps nav targets to their primary data", () => {
    expect(dataForHref("/")).toEqual([API.projects, API.systemQuick]);
    expect(dataForHref("/ports")).toEqual([API.ports]);
    expect(dataForHref("/settings?tab=domain")).toEqual([API.domains, API.sslEmail, API.systemIP]);
    expect(dataForHref("/settings?tab=github")).toEqual([API.githubStatus]);
  });

  test("streams, tabs without data and external links prefetch nothing", () => {
    expect(dataForHref("/logs")).toEqual([]);
    expect(dataForHref("/terminal")).toEqual([]);
    expect(dataForHref("/settings?tab=profile")).toEqual([]);
    expect(dataForHref("https://docklift.dev")).toEqual([]);
  });
});
