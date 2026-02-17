"use client";

import { useEffect, useState, useRef } from "react";
import { LogViewer } from "@/components/LogViewer";
import { API_URL } from "@/lib/utils";

interface SystemLogsPanelProps {
  service: string; // 'backend' | 'frontend' | 'database' | 'redis' | 'proxy' | 'nginx'
  isActive: boolean;
}

const MAX_LOG_LINES = 10000;

export function SystemLogsPanel({ service, isActive }: SystemLogsPanelProps) {
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

    const connect = async () => {
      // Close existing
      eventSourceRef.current?.close();

      // Fetch short-lived SSE token instead of using main JWT
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
        } else {
          // Fallback to main token if SSE token endpoint not available
          sseToken = typeof window !== "undefined" ? localStorage.getItem("docklift_token") || "" : "";
        }
      } catch {
        // Fallback to main token
        sseToken = typeof window !== "undefined" ? localStorage.getItem("docklift_token") || "" : "";
      }
      
      // SSE URL: use same-origin in browser when not on localhost (production behind proxy)
      const isDev = process.env.NODE_ENV === "development";
      let sseBase = API_URL || (isDev && typeof window !== "undefined"
        ? `${window.location.protocol}//${window.location.hostname}:8000`
        : "");
      if (typeof window !== "undefined" && window.location.hostname !== "localhost" && window.location.hostname !== "127.0.0.1") {
        sseBase = ""; // same-origin so /api is proxied correctly
      }
      
      const url = `${sseBase}/api/system/logs/${service}?tail=500&token=${encodeURIComponent(sseToken)}`;

      const es = new EventSource(url);
      eventSourceRef.current = es;

      es.onopen = () => {
        setConnected(true);
        retryCount = 0; // Reset on successful connection
      };

      es.onmessage = (event) => {
        try {
          const data = JSON.parse(event.data);
          // Backend sends { type: 'log', message: "..." }; support legacy data.log too
          const line = data.log ?? (data.type === "log" || data.type === "error" ? data.message : data.message);
          if (line != null && String(line).trim() !== "") {
            setLogs(prev => {
              const updated = [...prev, String(line)];
              return updated.length > MAX_LOG_LINES ? updated.slice(-MAX_LOG_LINES) : updated;
            });
          }
        } catch {
         // Raw text fallback
         setLogs(prev => {
           const updated = [...prev, event.data];
           return updated.length > MAX_LOG_LINES ? updated.slice(-MAX_LOG_LINES) : updated;
         });
        }
      };

      es.onerror = () => {
        setConnected(false);
        es.close();
        if (isActive) {
          // Exponential backoff with jitter to prevent reconnect storms
          const delay = Math.min(30000, 1000 * Math.pow(2, retryCount)) + Math.random() * 1000;
          retryCount++;
          setTimeout(connect, delay);
        }
      };
    };

    connect();

    return () => {
      eventSourceRef.current?.close();
    };
  }, [isActive, service]);

  if (!isActive) return null;

  return (
    <LogViewer
      logs={logs}
      connected={connected}
      title={`${service.charAt(0).toUpperCase() + service.slice(1)} Logs`}
      subtitle={`docklift-${service}`}
      onClear={() => setLogs([])}
      downloadFilename={`docklift-${service}-logs.txt`}
      height="h-[650px]"
    />
  );
}
