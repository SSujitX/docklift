---
name: Frontend Development
description: Guide for developing features in the Next.js 16 frontend functionality.
---

# Frontend Development Guide

Docklift uses **Next.js 16 (App Router)** for its frontend.

## Directory Structure (`frontend/app/`)

| Route | File | Description |
|-------|------|-------------|
| `/` | `page.tsx` | Dashboard (project list, system stats) |
| `/sign-in` | `sign-in/page.tsx` | Login page |
| `/setup` | `setup/page.tsx` | First-run registration |
| `/projects/new` | `projects/new/page.tsx` | Project creation wizard |
| `/projects/[id]` | `projects/[id]/page.tsx` | Project detail (tabs: overview, deployments, env, logs, files) |
| `/logs` | `logs/page.tsx` | System logs (real-time SSE, 4 service tabs) |
| `/terminal` | `terminal/page.tsx` | Web terminal (xterm.js) |
| `/system` | `system/page.tsx` | System health (CPU, RAM, disk) |
| `/ports` | `ports/page.tsx` | Docker port mapping viewer |
| `/databases` | `databases/page.tsx` | Database management |
| `/settings` | `settings/page.tsx` | App settings, GitHub integration |
| `/docs` | `docs/page.tsx` | Built-in documentation |

## Key Components

| Component | Location | Purpose |
|-----------|----------|---------|
| `Header.tsx` | `components/` | Global navigation bar |
| `Footer.tsx` | `components/` | Global footer |
| `LogViewer.tsx` | `components/` | **Shared** real-time log viewer with ANSI, search, colors |
| `SystemLogsPanel.tsx` | `components/` | SSE wrapper for system service logs |
| `Terminal.tsx` | `components/` | Terminal page wrapper (mounts TerminalView) |
| `TerminalView.tsx` | `components/` | Full xterm.js terminal emulator with system controls |
| `FileEditor.tsx` | `components/` | Monaco editor for in-browser file editing |
| `FileTree.tsx` | `components/` | Directory tree navigator |
| `EnvVarsManager.tsx` | `components/` | Environment variable CRUD |
| `StatusBadge.tsx` | `components/` | Container status indicator |

## Architecture Patterns

### 1. Client vs Server Components
- **"use client"**: Used for interactive dashboards (`useState`, `useEffect`)
- **Server Components**: Minimal — app relies on client-side API fetching (auth token in localStorage)

### 2. State Management
- **Local State**: `useState` for UI toggles
- **Global State**: `AuthContext` for user session
- **Data Fetching**: `useEffect` + native `fetch` via `authFetch()` / `fetchWithAuth()` from `lib/auth.ts`

### 3. Real-Time Streaming (SSE)
- Uses **Server-Sent Events (SSE)** via `EventSource` API
- System logs: `/api/system/logs/:service`
- Container logs: `/api/containers/:name/logs`
- Build logs: `/api/deployments/:id/logs`
- Frontend component: `LogViewer.tsx` renders all log types

### 4. Tab Switching Pattern
When switching between tabs that share a component, use React `key` to force remount:
```tsx
// Forces clean state when switching services
<SystemLogsPanel key={activeService} service={activeService} isActive={true} />
```

### 5. Polling with Dynamic Intervals (useRef Pattern)
When polling interval depends on component state, use a ref to avoid infinite useEffect re-registration:
```tsx
const buildingCountRef = useRef(buildingCount);
buildingCountRef.current = buildingCount;

useEffect(() => {
  fetchProjects();
  const interval = setInterval(() => {
    fetchProjects();
  }, buildingCountRef.current > 0 ? 3000 : 10000);
  return () => clearInterval(interval);
}, [fetchProjects]); // NO state-derived values in deps
```

### 6. Event Listener Cleanup (useRef Pattern)
When adding event listeners inside async `useEffect` functions, store handlers in refs for cleanup:
```tsx
const handlerRef = useRef<((e: Event) => void) | null>(null);
useEffect(() => {
  async function init() {
    const handler = (e: Event) => { ... };
    handlerRef.current = handler;
    element.addEventListener('contextmenu', handler);
  }
  init();
  return () => {
    if (element && handlerRef.current) {
      element.removeEventListener('contextmenu', handlerRef.current);
    }
  };
}, []);

## Common Tasks

### Adding a New Page
1. Create `app/new-feature/page.tsx`
2. Add `"use client"` if it needs interaction
3. Import `Header` and `Footer`
4. Add navigation link in `components/Header.tsx`

### Connecting to Backend
```typescript
import { fetchWithAuth, authFetch } from "@/lib/auth";

// Typed JSON response
const data = await fetchWithAuth<MyType>("/api/resource");

// Raw response (for non-JSON or custom handling)
const res = await authFetch(`${API_URL}/api/resource`, { method: "POST", body: ... });
```

### SSE Connection (with production proxy support)
```typescript
// In production, use same-origin (empty base) so /api is proxied correctly
const sseBase = window.location.hostname === "localhost" ? API_URL : "";
const es = new EventSource(`${sseBase}/api/system/logs/backend?token=...`);
```

## Debugging

- **Hydration Errors**: Caused by server/client mismatch (dates, conditional rendering)
- **Auth Issues**: Check `localStorage.getItem("docklift_token")` in devtools
- **SSE not connecting**: Check Network → EventStream tab in devtools
- **Build errors**: Run `npx next build` locally first to catch TypeScript issues
