---
name: Logging & Monitoring
description: Guide to the real-time logging system, LogViewer component, and system monitoring.
---

# Logging & Monitoring Guide

Docklift streams logs for both platform containers and user apps over Server-Sent Events (SSE).

## Architecture

```
Docker Container → stdout/stderr → Dockerode → SSE → EventSource → LogViewer UI
```

### Backend (SSE Streaming)

- **System logs**: `backend/src/routes/system.ts` → `GET /api/system/logs/:service`
- **Container logs**: `backend/src/services/docker.ts` → `streamContainerLogs()`
- **Deployment build logs**: `backend/src/routes/deployments.ts` → SSE during build + `docker compose up`

### Frontend (Display)

- **Shared component**: `frontend/src/components/LogViewer.tsx` — single source of truth for rendering
- **System logs panel**: `frontend/src/components/SystemLogsPanel.tsx` — SSE wrapper for one service
- **System logs page**: `frontend/src/pages/Logs.tsx` — tabbed UI for platform containers
- **Project logs**: `frontend/src/pages/ProjectDetail.tsx` → `ContainerLogsPanel` — uses `LogViewer`

## Service → Container Mapping

These are the **only** valid system services. They are defined once, in `LOG_SERVICE_CONTAINERS`
(`backend/src/routes/system.ts`), and must match `docker-compose.yml`:

| Service key | Container | UI label | Role |
|-------------|-----------|----------|------|
| `backend` | `docklift-backend` | Backend | API server |
| `frontend` | `docklift-frontend` | Frontend | Dashboard SPA |
| `nginx` | `docklift-nginx` | **Dashboard Gateway** | Serves the panel on `:8080` |
| `proxy` | `docklift-nginx-proxy` | **Public Proxy** | Public `:80`/`:443` for app domains |
| `certbot` | `docklift-certbot` | Certbot | Certificate issuance + renewal |

> **Naming matters.** There are two nginx containers and calling both "nginx" confuses everyone.
> `docklift-nginx` is the *internal dashboard gateway*; `docklift-nginx-proxy` is the *public
> edge* that terminates TLS for user domains. The UI uses the role names above, not the image name.

> There is NO `docklift-db` or `docklift-redis`. SQLite is a file inside the backend container.

## LogViewer Component

`frontend/src/components/LogViewer.tsx`, used by both system and project logs.

### Props

| Prop | Type | Default | Description |
|------|------|---------|-------------|
| `logs` | `string[]` | — | Raw log lines from SSE |
| `connected` | `boolean` | — | Whether the SSE connection is alive |
| `title` | `string` | — | Header title (role name) |
| `subtitle` | `string` | — | Monospace subtitle (real container name) |
| `onClear` | `() => void` | — | Clear log buffer |
| `onDownload` | `() => void` | auto | Custom download handler |
| `height` | `string` | `"h-[600px]"` | Tailwind height class |
| `downloadFilename` | `string` | `"logs.txt"` | Download filename |

### Features

- **Timestamps**: Docker timestamps parsed into `2026-Feb-05 12:06:06.047`
- **ANSI colours**: full escape-code parsing (30–37, 90–97, bold)
- **Smart colours** when no ANSI codes are present:
  - Red: `error`, `fatal`, `panic`, `fail` · Amber: `warn`
  - Green: `success`, `ready`, `loaded`, `✓` · Blue: `info`, `starting`, `🚀`
- **Search**: inline Ctrl+F with match highlighting
- **Fullscreen**: toggle
- **Follow mode**: auto-scroll pauses when you scroll up; a **Follow** button re-attaches to the tail
- **Download**: joins lines with `\n` (not `""`)

### Auto-scroll: scroll the container, not the page

Use direct `scrollTop` assignment inside `requestAnimationFrame` — **not** `scrollIntoView()`:

```tsx
const el = scrollRef.current;
if (el) requestAnimationFrame(() => { el.scrollTop = el.scrollHeight; });
```

`scrollIntoView()` scrolls the *page* as well as the log pane, which fights the sidebar shell layout,
and a `smooth` animation never lands at the bottom while lines are still streaming — every append
restarts it and it stalls part-way down.

## SSE Connection Pattern

```typescript
const es = new EventSource(`/api/system/logs/backend?tail=500&token=${sseToken}`);

es.onmessage = (event) => {
  const data = JSON.parse(event.data);   // { type: 'log', message: '...' }
  setLogs(prev => [...prev, data.message]);
};
```

SSE needs a short-lived token from `POST /api/auth/sse-token`, never the session JWT — see the
`authentication` skill.

### Backend SSE Response Format

```json
{ "type": "log",    "message": "Prisma schema loaded from prisma/schema.prisma" }
{ "type": "error",  "message": "Container not found" }
{ "type": "status", "message": "Container is not running" }
{ "type": "end",    "message": "Stream closed" }
```

### Reconnect Backoff: reset on output, not on connect

`SystemLogsPanel` retries with backoff when the stream drops. Reset the retry counter only when a
`type: 'log'` frame arrives — **not** in `onopen` and not on `status`/`error` frames.

A missing or stopped container still "opens" the stream successfully and immediately emits an error
frame. Resetting the counter there produces a tight reconnect loop that hammers the API forever.
Repeated status lines are also deduplicated (`appendStatus`) so a stopped container does not fill the
buffer with the same sentence.

## Tab Switching

`SystemLogsPanel` must receive `key={service}` so switching tabs fully unmounts and remounts it.
Without the key, the previous service's lines stay in state and appear interleaved.

```tsx
<SystemLogsPanel
  key={activeService}
  service={activeService}
  label="Public Proxy"
  container="docklift-nginx-proxy"
  isActive
/>
```

## Max Log Lines

- Frontend buffers up to **10,000** lines per service/container
- Backend `tail` defaults to **200** (`?tail=` up to the UI's 500 for initial load)
- Docker timestamps are enabled (`timestamps: true` in `streamContainerLogs`)

## Certbot Log Visibility

`docker-compose.yml` runs `certbot renew` **without** `--quiet` and echoes a timestamped
`[certbot] … checking renewals` line each cycle. With `--quiet`, a healthy renewal loop produces no
output at all, which is indistinguishable from a broken container in the Logs UI.

## Troubleshooting

- **"Container not found" loop**: the service key is not in `LOG_SERVICE_CONTAINERS`, or the
  container is genuinely absent. Check the backoff reset rule above before assuming an API bug.
- **Logs not appearing**: DevTools → Network → EventStream; also check the SSE token has not expired (5 min).
- **Mixed logs between tabs**: missing `key` prop on `SystemLogsPanel`.
- **Page jumps while logs stream**: something reintroduced `scrollIntoView()`.
- **Download has no newlines**: use `logs.join("\n")`, not `logs.join("")`.
