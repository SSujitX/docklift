---
name: Logging & Monitoring
description: Guide to the real-time logging system, LogViewer component, and system monitoring.
---

# Logging & Monitoring Guide

Docklift provides real-time log streaming for both system containers and user-deployed applications via Server-Sent Events (SSE).

## Architecture

```
Docker Container → stdout/stderr → Dockerode → SSE → EventSource → LogViewer UI
```

### Backend (SSE Streaming)

- **System Logs**: `backend/src/routes/system.ts` → `GET /api/system/logs/:service`
- **Container Logs**: `backend/src/services/docker.ts` → `streamContainerLogs()`
- **Deployment Build Logs**: `backend/src/routes/deployments.ts` → SSE during `docker compose up`

### Frontend (Display)

- **Shared Component**: `frontend/components/LogViewer.tsx` — single source of truth for all log rendering
- **System Logs Panel**: `frontend/components/SystemLogsPanel.tsx` — SSE wrapper for system services
- **System Logs Page**: `frontend/app/logs/page.tsx` — tabbed UI for system containers
- **Project Logs**: `frontend/app/projects/[id]/page.tsx` → `ContainerLogsPanel` — uses `LogViewer`

## Service → Container Mapping

These are the **only** valid system services (must match `docker-compose.yml`):

| Service | Container Name | Description |
|---------|---------------|-------------|
| `backend` | `docklift-backend` | API server |
| `frontend` | `docklift-frontend` | Next.js dashboard |
| `proxy` | `docklift-nginx-proxy` | Custom domain routing (port 80) |
| `nginx` | `docklift-nginx` | Dashboard gateway (port 8080) |

> **IMPORTANT**: There is NO `docklift-db` or `docklift-redis` container.
> SQLite is a file inside the backend container, not a separate service.

## LogViewer Component

Located at `frontend/components/LogViewer.tsx`. Used by both system logs and project logs.

### Props

| Prop | Type | Default | Description |
|------|------|---------|-------------|
| `logs` | `string[]` | — | Raw log lines from SSE |
| `connected` | `boolean` | — | Whether SSE connection is alive |
| `title` | `string` | — | Header title |
| `subtitle` | `string` | — | Monospace subtitle (container name) |
| `onClear` | `() => void` | — | Clear log buffer |
| `onDownload` | `() => void` | auto | Custom download handler |
| `height` | `string` | `"h-[600px]"` | Tailwind height class |
| `downloadFilename` | `string` | `"logs.txt"` | Download filename |

### Features

- **Timestamps**: Parses Docker timestamps into human-readable format (`2026-Feb-05 12:06:06.047`)
- **ANSI Colors**: Fully parses ANSI escape codes (30-37, 90-97 color range, bold)
- **Smart Colors**: Auto-detects log levels when no ANSI codes present:
  - Red: `error`, `fatal`, `panic`, `fail`
  - Amber: `warn`
  - Green: `success`, `ready`, `loaded`, `✓`
  - Blue: `info`, `starting`, `🚀`
- **Search**: Ctrl+F inline search with match highlighting
- **Fullscreen**: Toggle fullscreen mode
- **Auto-scroll**: Pauses on manual scroll-up, FAB to jump back down
- **Download**: Joins lines with `\n` (not empty string)

## SSE Connection Pattern

```typescript
// Frontend pattern for connecting to SSE logs
const es = new EventSource(`/api/system/logs/backend?tail=500&token=...`);

es.onmessage = (event) => {
  const data = JSON.parse(event.data);
  // data = { type: 'log', message: "log line here" }
  setLogs(prev => [...prev, data.message]);
};
```

### Backend SSE Response Format

```json
{ "type": "log", "message": "Prisma schema loaded from prisma/schema.prisma" }
{ "type": "error", "message": "Container not found" }
{ "type": "status", "message": "Container is not running" }
```

## Tab Switching

When switching between service tabs on `/logs`, the `SystemLogsPanel` must be given a React `key={service}` to force a full unmount/remount. Without this, logs from the previous service persist in state and appear mixed.

```tsx
<SystemLogsPanel key={activeService} service={activeService} isActive={true} />
```

## Max Log Lines

- Frontend buffers up to **10,000** lines per service/container
- Backend `tail` parameter defaults to **500** for initial load (configurable via query param)
- Docker timestamps are enabled (`timestamps: true` in `streamContainerLogs`)

## Troubleshooting

- **"Container not found" loop**: Service name doesn't match any container in `docker-compose.yml`
- **Logs not appearing**: Check SSE connection in browser DevTools → Network → EventStream
- **Mixed logs between tabs**: Missing `key` prop on `SystemLogsPanel` (see Tab Switching above)
- **Download has no newlines**: Ensure `logs.join("\n")` not `logs.join("")`
