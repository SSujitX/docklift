---
name: Logging & Monitoring
description: Guide to the real-time logging system, LogViewer component, backend logs, and system monitoring in the Docklift Go stack.
---

# Logging & Monitoring Guide

Docklift streams logs for the platform containers and for user apps over Server-Sent Events (SSE). Build and restore progress is streamed as plain text.

## Architecture

```
Docker container -> stdout/stderr -> Docker Engine API (Go client) -> SSE -> EventSource -> LogViewer
```

### Backend

-   **SSE stream (shared)**: `server/internal/api/logs/stream.go` (`Stream`). Used by both routes below.
-   **Project container logs**: `GET /api/logs/:projectId/stream/:containerName` (`server/internal/api/logs/logs.go`). The container must belong to the project, else 404 "Container not found for this project". `GET /api/logs/:projectId/containers` lists services with live state.
-   **System logs**: `GET /api/system/logs/:service` (`server/internal/api/system/system.go`).
-   **Deployment, stop, restart, rollback progress**: `POST /api/deployments/:projectId/...` answer with a `text/plain` stream (`server/internal/api/deployments/logsink.go`). Known secret values are masked as `***` in deploy logs.
-   **Snapshots**: `GET /api/deployments/:projectId/logs` returns the last log lines of each service container as JSON; `/stats` returns container stats.
-   **Backup and restore progress**: plain text stream with `[ERROR]` lines on failure (see `system_administration`).
-   **The backend's own log**: structured `slog` text on stderr, visible with `docker logs docklift-backend` or the Backend tab. Tags in messages: `[AUDIT]` (host actions, with client address), `[SECURITY]` (suspicious auth events), `[TERMINAL]`, `[Webhook]`, `[Auto-Deploy]`, `[CRITICAL]` (restore seal), `[bootstrap]`.

### Authentication for streams

The two SSE routes use a **short-lived SSE token**, never the session JWT:

-   `POST /api/auth/sse-token` (Bearer session token) returns a 5 minute token with `purpose: "sse"`.
-   The client opens `EventSource("/api/.../stream/...?token=<sseToken>")`. Exactly one `token` parameter is accepted.
-   The guard (`RequireSSE`) applies only to `GET` requests on `/api/system/logs/*` and `/api/logs/*/stream/*`. The connection is **closed when the user's password changes**, and the token cannot call any other API.
-   nginx access logs record `$uri`, not the query string, so tokens do not reach the logs.

See the `authentication` skill.

### Frontend

-   **Shared viewer**: `frontend/src/components/LogViewer.tsx`, the single place that renders logs.
-   **System logs panel**: `frontend/src/components/SystemLogsPanel.tsx`, one SSE connection for one service.
-   **System logs page**: `frontend/src/pages/Logs.tsx`, tabs for the platform containers.
-   **Project logs**: `ContainerLogsPanel` in `frontend/src/pages/ProjectDetail.tsx`, one `EventSource` per service container, requested with `tail=5000`.

## Service to Container Mapping

System services are defined in `logServices` in `server/internal/api/system/system.go` and listed in `frontend/src/pages/Logs.tsx`. They must match `docker-compose.yml`:

| Service key | Container | UI label | Role |
|-------------|-----------|----------|------|
| `backend` | `docklift-backend` | Backend | Go server: API, terminal, embedded dashboard |
| `proxy` | `docklift-nginx-proxy` | Public Proxy | Public `:80`/`:443` for app domains and the panel domain |
| `certbot` | `docklift-certbot` | Certbot | Certificate renewals |

Any other key is rejected with 400 and the list of valid names. There is no frontend container and no gateway nginx container in 3.x, and no database container (SQLite is a file under `data/`). When adding a container to the stack, add it in three places: `docker-compose.yml`, `logServices`, and `SERVICES` in `Logs.tsx`.

## SSE Event Format

Each event is `data: <json>\n\n`:

```json
{ "type": "connected", "container": "docklift-backend" }
{ "type": "log",       "message": "2026-02-05T12:06:06.047Z msg=..." }
{ "type": "status",    "message": "Container is not running" }
{ "type": "error",     "message": "Container not found: ..." }
{ "type": "end",       "message": "Log stream ended" }
```

-   Docker problems are sent as `error` / `status` events on a **200** stream (so the browser shows the message), not as HTTP errors. Only auth and unknown-service/project failures are HTTP errors.
-   If the container is not running, one `status` event is sent and the stream closes.
-   Otherwise logs are followed with `Timestamps: true`, stdout and stderr merged, one `log` event per Docker frame (whitespace-only frames are skipped). TTY containers are copied raw.
-   `?tail=N` sets the initial line count. `parseInt(tail) || 200`, so invalid or zero values give **200**; a negative value is passed to Docker, which treats it as all lines. Dashboard requests use 500 for system logs and 5000 for project logs.
-   Each event write has a 30 s deadline; a stalled client ends the stream. Headers include `X-Accel-Buffering: no` so nginx does not buffer the stream.

## LogViewer Component

`frontend/src/components/LogViewer.tsx`, used by system and project logs.

### Props

| Prop | Type | Default | Description |
|------|------|---------|-------------|
| `logs` | `string[]` | required | Raw log lines |
| `connected` | `boolean` | required | Whether the stream is alive |
| `title` | `string` | required | Header title (role name) |
| `subtitle` | `string` | none | Monospace subtitle (real container name) |
| `onClear` | `() => void` | required | Clear the buffer |
| `onDownload` | `() => void` | built-in | Custom download handler |
| `height` | `string` | `"h-[600px]"` | Tailwind height class (`h-full` lets the parent size it) |
| `downloadFilename` | `string` | `"logs.txt"` | Download name |

### Features

-   **Timestamps**: the Docker RFC 3339 prefix is parsed and shown as `2026-Feb-05 12:06:06.047` (local time).
-   **ANSI colours**: SGR codes 30-37 and 90-97 plus bold.
-   **Smart colours** when a line has no ANSI codes: red for `error`, `fatal`, `panic`, `fail`; amber for `warn`; green for `success`, `ready`, `loaded`; blue for `info`, `starting`.
-   **Search**: Ctrl/Cmd+F while the viewer is focused, with match highlighting; Esc closes it.
-   **Fullscreen** toggle, **copy**, **download** (lines joined with `\n`).
-   **Follow mode**: auto-scroll pauses when the user scrolls away from the bottom (80 px threshold); the Follow button re-attaches.

### Auto-scroll: scroll the container, not the page

Assign `scrollTop = scrollHeight` on the log container (inside `requestAnimationFrame`). Do not use `scrollIntoView()`: it scrolls the page too, fights the shell layout, and a smooth animation stalls while lines keep arriving.

## SSE Connection Pattern

`SystemLogsPanel` does, in order:

1.  `POST /api/auth/sse-token` with the session token.
2.  `new EventSource(`/api/system/logs/${service}?tail=500&token=${sseToken}`)`.
3.  On each message parse JSON; `log` events are appended, `status` / `error` / `end` messages are shown once as status lines, `connected` is ignored.
4.  On error, close and reconnect with backoff (below) and a **new** SSE token.

### Reconnect backoff: reset on output, not on connect

The delay is `min(30 s, 1 s * 2^retries)` plus up to 1 s of jitter. Reset the retry counter only when a `type: "log"` frame arrives, **not** in `onopen` and not on `status` or `error` frames. A missing or stopped container still opens the stream successfully and then sends one error or status event; resetting on open would reconnect once a second forever. Repeated identical status lines are collapsed (`appendStatus`).

## Tab Switching

`SystemLogsPanel` must be rendered with `key={service}` so switching tabs unmounts the old stream and clears its lines. Without the key the previous service's lines stay in state and mix with the new ones.

```tsx
<SystemLogsPanel
  key={active.id}
  service={active.id}
  label={active.label}
  container={active.container}
  isActive
/>
```

## Buffer Limits

-   The frontend keeps at most **10,000** lines per service or container (`MAX_LOG_LINES`).
-   Backend `tail` defaults to **200** when the parameter is absent or invalid.

## Certbot Log Visibility

The `certbot` service in `docker-compose.yml` runs `certbot renew --webroot -w /var/www/certbot` every 12 hours **without** `--quiet` and prints a timestamped `[certbot] ... checking renewals` line each cycle. A healthy idle container would otherwise log nothing, which looks the same as a dead one in the Logs UI.

## Host Monitoring

CPU, memory, disk, network, processes and server info come from `GET /api/system/stats` (3 s cache) and `GET /api/system/quick`; see `system_administration`. Container state comes from `GET /api/logs/:projectId/containers`, and container stats from `GET /api/deployments/:projectId/stats`. `GET /api/health` (public) returns `{ status, timestamp, version, instanceId }` and is the Docker healthcheck.

## Troubleshooting

-   **"Container not found" loop**: the service key is not in `logServices`, or the container really is absent (`docker ps -a`). Check the backoff rule above before suspecting the API.
-   **Logs not appearing**: DevTools, Network, EventStream. A 401 means the SSE token expired (5 minutes) or the password changed; the panel fetches a new one on reconnect. Behind another reverse proxy, make sure it does not buffer `text/event-stream`.
-   **Streams close after a password change**: expected; the connection is bound to the session and ends when the password version changes.
-   **Mixed logs between tabs**: missing `key` prop on `SystemLogsPanel`.
-   **Page jumps while logs stream**: `scrollIntoView()` was reintroduced.
-   **Download has no newlines**: use `logs.join("\n")`.
-   **Backend log level**: the Go server logs with `slog` at info and above. Add `slog` fields (`"error", err`) instead of formatting errors into the message, and never log tokens, passwords or URLs with credentials (`gitops.Redact`).
