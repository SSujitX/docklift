// Consume a text streaming response (backup/restore) and surface failures
// that arrive as HTTP errors or `[ERROR]` markers after a 200 status.

export type StreamProgressResult = {
  ok: boolean;
  lines: string[];
  error?: string;
};

/**
 * Read a fetch Response body as text lines.
 * - Non-OK HTTP status → failure (JSON error body preferred when present)
 * - Any line containing `[ERROR]` → failure even if status is 200
 */
export async function consumeProgressStream(
  res: Response,
  onLine?: (line: string) => void,
): Promise<StreamProgressResult> {
  const lines: string[] = [];

  if (!res.ok) {
    const contentType = res.headers.get("content-type") || "";
    if (contentType.includes("application/json")) {
      const data = await res.json().catch(() => ({} as { error?: string }));
      return {
        ok: false,
        lines,
        error: data.error || `Request failed (${res.status})`,
      };
    }
    const text = await res.text().catch(() => "");
    for (const line of text.split("\n")) {
      if (!line) continue;
      lines.push(line);
      onLine?.(line);
    }
    return {
      ok: false,
      lines,
      error: lines.find((l) => /\[ERROR\]/i.test(l)) || `Request failed (${res.status})`,
    };
  }

  if (!res.body) {
    return { ok: false, lines, error: "Empty response body" };
  }

  const reader = res.body.getReader();
  const decoder = new TextDecoder();
  let buffer = "";
  let sawError = false;
  let errorLine = "";

  while (true) {
    const { done, value } = await reader.read();
    if (done) break;
    buffer += decoder.decode(value, { stream: true });
    const parts = buffer.split("\n");
    buffer = parts.pop() || "";
    for (const line of parts) {
      lines.push(line);
      onLine?.(line);
      if (/\[ERROR\]/i.test(line)) {
        sawError = true;
        errorLine = line;
      }
    }
  }
  if (buffer) {
    lines.push(buffer);
    onLine?.(buffer);
    if (/\[ERROR\]/i.test(buffer)) {
      sawError = true;
      errorLine = buffer;
    }
  }

  if (sawError) {
    return { ok: false, lines, error: errorLine.replace(/^.*\[ERROR\]\s*/i, "").trim() || errorLine };
  }
  return { ok: true, lines };
}
