
import { useEffect, useState, useRef } from "react";
import { LogViewer } from "@/components/LogViewer";
import { API_URL } from "@/lib/utils";

interface SystemLogsPanelProps {
  /** Service key accepted by GET /api/system/logs/:service. */
  service: string;
  isActive: boolean;
  /** Display name; defaults to the capitalised service key. */
  label?: string;
  /** Real container name, shown beside the title. */
  container?: string;
}

const MAX_LOG_LINES = 10000;

export function SystemLogsPanel({ service, isActive, label, container }: SystemLogsPanelProps) {
  const [logs, setLogs] = useState<string[]>([]);
  const [connected, setConnected] = useState(false);
  const eventSourceRef = useRef<EventSource | null>(null);

  // Connect to SSE
  useEffect(() => {
    if (!isActive) {
      eventSourceRef.current?.close();
      eventSourceRef.current = null;
      setConnected(false);
      return;
    }

    let retryCount = 0;
    let retryTimer: ReturnType<typeof setTimeout> | null = null;
    let cancelled = false;

    const connect = async () => {
      if (cancelled) return;
      // Close existing
      eventSourceRef.current?.close();

      // Fetch short-lived SSE token — never put the session JWT in the URL
      let sseToken = "";
      try {
        const mainToken = typeof window !== "undefined" ? localStorage.getItem("docklift_token") || "" : "";
        const tokenRes = await fetch(`${API_URL || ""}/api/auth/sse-token`, {
          method: "POST",
          headers: {
            "Content-Type": "application/json",
            ...(mainToken ? { Authorization: `Bearer ${mainToken}` } : {}),
          },
        });
        if (tokenRes.ok) {
          const data = await tokenRes.json();
          sseToken = data.token || "";
        }
      } catch {
        /* leave empty */
      }

      if (!sseToken) {
        setConnected(false);
        if (isActive && !cancelled) {
          const delay = Math.min(30000, 1000 * Math.pow(2, retryCount)) + Math.random() * 1000;
          retryCount++;
          retryTimer = setTimeout(connect, delay);
        }
        return;
      }
      
      // SSE URL: use same-origin in browser when not on localhost (production behind proxy)
      const isDev = import.meta.env.DEV;
      let sseBase = API_URL || (isDev && typeof window !== "undefined"
        ? `${window.location.protocol}//${window.location.hostname}:8000`
        : "");
      if (typeof window !== "undefined" && window.location.hostname !== "localhost" && window.location.hostname !== "127.0.0.1") {
        sseBase = ""; // same-origin so /api is proxied correctly
      }
      
      const url = `${sseBase}/api/system/logs/${service}?tail=500&token=${encodeURIComponent(sseToken)}`;

      const es = new EventSource(url);
      eventSourceRef.current = es;

      const append = (line: string) => {
        if (!line.trim()) return;
        setLogs(prev => {
          const updated = [...prev, line];
          return updated.length > MAX_LOG_LINES ? updated.slice(-MAX_LOG_LINES) : updated;
        });
      };

      // Status lines repeat on every reconnect (e.g. a stopped container), so only
      // show them when they are not already the last thing on screen.
      const appendStatus = (line: string) => {
        if (!line.trim()) return;
        setLogs(prev => (prev[prev.length - 1] === line ? prev : [...prev, line]));
      };

      es.onopen = () => {
        setConnected(true);
      };

      es.onmessage = (event) => {
        let data: { type?: string; message?: string; log?: string };
        try {
          data = JSON.parse(event.data);
        } catch {
          append(event.data); // Raw text fallback
          return;
        }

        if (data.type === "log" || data.log != null) {
          // Real output means the stream is healthy, so restart the backoff here
          // rather than in onopen: the connection also "opens" for a container
          // that does not exist, which otherwise reconnects once a second forever.
          retryCount = 0;
          append(String(data.log ?? data.message ?? ""));
          return;
        }

        if (data.type === "connected") return;

        // status | error | end — container missing, stopped, or stream closed
        if (data.message) appendStatus(String(data.message));
      };

      es.onerror = () => {
        setConnected(false);
        es.close();
        if (isActive && !cancelled) {
          // Exponential backoff with jitter to prevent reconnect storms
          const delay = Math.min(30000, 1000 * Math.pow(2, retryCount)) + Math.random() * 1000;
          retryCount++;
          retryTimer = setTimeout(connect, delay);
        }
      };
    };

    connect();

    return () => {
      cancelled = true;
      if (retryTimer) clearTimeout(retryTimer);
      eventSourceRef.current?.close();
    };
  }, [isActive, service]);

  if (!isActive) return null;

  const title = label || service.charAt(0).toUpperCase() + service.slice(1);
  const containerName = container || `docklift-${service}`;

  return (
    <LogViewer
      logs={logs}
      connected={connected}
      title={`${title} Logs`}
      subtitle={containerName}
      onClear={() => setLogs([])}
      downloadFilename={`${containerName}-logs.txt`}
      height="h-[650px]"
    />
  );
}
