---
name: Frontend Development
description: Guide for developing features in the Vite + React Router frontend.
---

# Frontend Development Guide

Docklift uses **Vite + React + React Router** for its dashboard (static SPA in production).

## Directory Structure (`frontend/src/`)

| Route | File | Description |
|-------|------|-------------|
| `/` | `pages/Dashboard.tsx` | Dashboard (project list, system stats) |
| `/sign-in` | `pages/SignIn.tsx` | Login page |
| `/setup` | `pages/Setup.tsx` | First-run registration |
| `/projects/new` | `pages/NewProject.tsx` | Project creation wizard |
| `/projects/:id` | `pages/ProjectDetail.tsx` | Project detail tabs |
| `/logs` | `pages/Logs.tsx` | System logs (SSE) |
| `/terminal` | `pages/Terminal.tsx` | Web terminal (xterm.js) |
| `/system` | `pages/System.tsx` | System health |
| `/ports` | `pages/Ports.tsx` | Docker port mapping |
| `/databases` | `pages/Databases.tsx` | Database projects |
| `/settings` | `pages/Settings.tsx` | Settings + GitHub |
| `/docs/*` | `pages/docs/*` | Built-in documentation |

Router: `src/app/router.tsx` (lazy route chunks).

## Key Components

| Component | Location | Purpose |
|-----------|----------|---------|
| `Header.tsx` | `components/` | Global navigation |
| `Footer.tsx` | `components/` | Global footer |
| `LogViewer.tsx` | `components/` | Shared log viewer |
| `SystemLogsPanel.tsx` | `components/` | SSE system logs |
| `TerminalView.tsx` | `components/` | xterm.js + WS terminal |
| `FileEditor.tsx` | `components/` | Monaco editor |
| `AuthProvider.tsx` | `components/` | Auth + route redirects |

## Dev server

- Default port: **3600** (`PORT` in `frontend/.env`)
- Proxy: `/api` and `/ws` → `http://127.0.0.1:8000`
- Leave `VITE_API_URL` empty for same-origin / proxy

## Production

- `bun run build` → `dist/`
- Docker image `docklift-frontend` serves static files with nginx on **3000**
- Gateway `nginx.conf` proxies `/` → `docklift-frontend:3000` and keeps `/api`, `/ws/`, SSE rules
