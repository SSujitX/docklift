---
name: Frontend Development
description: Guide for developing features in the Vite + React Router frontend.
---

# Frontend Development Guide

Docklift uses **Vite + React 19 + React Router 7** and **Tailwind CSS 4** for its dashboard. In
production it is a static SPA that is built into `frontend/dist` and **embedded in the Go binary**
(`server/web`, `go:embed`); the Go server serves it with SPA fallback on the same origin as `/api` and
`/ws`. There is no frontend container and no dashboard nginx.

## Directory Structure (`frontend/src/`)

| Route | File | Description |
|-------|------|-------------|
| `/` | `pages/Dashboard.tsx` | Projects list (mobile cards / desktop table, filters, pagination) |
| `/sign-in` | `pages/SignIn.tsx` | Login page |
| `/setup` | `pages/Setup.tsx` | First-run registration |
| `/projects/new` | `pages/NewProject.tsx` | Project creation wizard (PageHeader, brand chips/cards) |
| `/projects/:id` | `pages/ProjectDetail.tsx` | Project detail tabs |
| `/logs` | `pages/Logs.tsx` | System logs (SSE) — compact header, one-viewport fill |
| `/terminal` | `pages/Terminal.tsx` | Web terminal (xterm.js) — compact header, one-viewport fill |
| `/system` | `pages/System.tsx` | Host metrics (`SystemOverview`) — compact Operate header, **page scrolls** (not one-viewport) |
| `/ports` | `pages/Ports.tsx` | Docker port mapping — compact header, one-viewport fill |
| `/databases` | `pages/Databases.tsx` | Managed DBs (Projects-style filters, mobile cards / desktop table) |
| `/databases/new` | `pages/NewDatabase.tsx` | Full-width create + sticky CTA (matches New Project) |
| — | `components/databases/*` | Connection, link, attach panels |
| `/settings` | `pages/Settings.tsx` | Settings + GitHub |
| `/developer` | `pages/Developer.tsx` | API & MCP: API tokens (create with password, shown once; revoke with password), "Connect an AI" snippets, endpoint browser from `/api/openapi.json`. Pure helpers in `lib/developer.ts` (tested) |
| `/docs`, `/docs/*` | redirect | `DocsRedirect` sends the browser to https://docklift.dev (VitePress site in `website/`) |

Router: `src/app/router.tsx` (lazy route chunks). `/sign-in` and `/setup` render
bare; every other route is nested under `app/AppShell.tsx`.

## App shell (`components/shell/`)

All navigation lives in a fixed left rail. Pages render only their own content —
they must not add a page header bar, footer, or outer `container`/`min-h-screen`
wrapper, because `AppShell` already supplies the top bar, max width and padding.

| Component | Purpose |
|-----------|---------|
| `AppShell.tsx` (`app/`) | Fixed rail, mobile drawer, top bar, content `<main>` |
| `Sidebar.tsx` | Brand, New Project, grouped nav, Settings tree, version footer |
| `navigation.ts` | Nav groups + breadcrumbs — add new pages here once. Each item’s `section` prefixes keep the rail selected on nested routes (`/projects/*`, `/databases/new`, `/settings`, …) |
| `lib/settingsNav.ts` | Settings section ids/labels shared by rail tree + Settings page |
| `AccountMenu.tsx` | Header account menu: identity, profile, sign out (theme is TopBar toggle) |
| `SidebarStatus.tsx` | Version + upgrade prompt (star cache shared with TopBar) |
| `TopBar.tsx` | Breadcrumbs, search, GitHub star, theme toggle, account menu |
| `CommandPalette.tsx` | Ctrl/Cmd+K: pages, projects, actions |
| `PageHeader.tsx` | `PageHeader` + `StatChip` for consistent page titles |

Shortcuts: `Ctrl/Cmd+K` opens the palette, `Ctrl/Cmd+B` collapses the rail
(persisted in `docklift_sidebar_collapsed`).

Sticky page elements sit below the 3.5rem top bar — use `top-14` for flush
elements and `top-20` for spaced ones.

## Key Components

| Component | Location | Purpose |
|-----------|----------|---------|
| `LogViewer.tsx` | `components/` | Shared log viewer (search, copy, download, clear, fullscreen) — used for project/service runtime logs and System Logs |
| `SystemLogsPanel.tsx` | `components/` | SSE system logs |
| `TerminalView.tsx` | `components/` | xterm.js + WS terminal |
| `FileEditor.tsx` | `components/` | Full-screen IDE Monaco editor (dirty state, Ctrl/⌘S, Esc) |
| `AuthProvider.tsx` | `components/` | Auth + route redirects |
| `ServiceDomainCard.tsx` | `components/domains/` | Project service domain list + SSL (serial mutation queue) |
| `PanelDomainCard.tsx` | `components/domains/` | Settings → Domain panel hostname + SSL (same UX as service card) |
| `DnsGuideCard.tsx` | `components/domains/` | Shared “Point DNS at this server” guide |

## API calls (required)

-   Protected APIs: **`authFetch()`** from `lib/auth.ts` (401 → logout). Do not use raw `fetch` +
    `getAuthHeaders()` for authenticated endpoints — check `res.ok` before treating data as success
    (Ports, Databases, GitHub disconnect, etc.).
-   Backup/restore/deploy streams: `lib/streamProgress.ts` (`consumeProgressStream`) — require
    `res.ok` and treat `[ERROR]` lines as failure before toasting success.
-   Restore + system purge: collect **account password** in the confirm dialog and send it in the
    body / FormData (`password`) for step-up auth.
-   Overlays: `lib/focusTrap.ts` (`useFocusTrap`) for command palette + mobile drawer.
-   Deploy history pagination: AbortController + generation counter (ignore stale pages).

## Project UI notes

-   Project status includes **`degraded`** (partial fleet) — `StatusBadge` / `ProjectCard` must show it;
    treat like running for stop/restart actions.
-   Build Settings: **`publish_host_port`** checkbox (default off) — host ports are opt-in.
-   Overview → Services & Endpoints: never show `IP:null` or link until `serverIP` is real.
    If no host port and no domain, show **Private by default** with clear copy: prefer a
    domain; avoid sharing `IP:port` (exposes origin IP / easier to scan). Key
    “awaiting host port / Redeploy” off **persisted** `project.publish_host_port` — never
    the unsaved Build checkbox (`publishHostPort` form state). Secondary CTA when not
    awaiting: **Build settings** (navigates; does not publish). If Publish is already
    saved on but `svc.port` is still null, say redeploy is required. No “Workspace” badge.
-   Domain tab (singular label): `DnsGuideCard` highlights “Point DNS at this server”
    with short Cloudflare tips (no accordion). Empty-state: private-by-default + domain
    preferred; mention Build → Publish host ports + redeploy as the opt-in `IP:port` path.
-   Settings → Domain (`/settings?tab=domain`): same guide + inline add (no popup),
    SSL status pills, Check DNS, Retry HTTPS, and Let’s Encrypt activity log via
    `PanelDomainCard` — mirror project Domain, not a table/dialog.
    Under each dashboard-port domain, `PanelAccessSwitch` (`components/domains/PanelAccess.tsx`)
    is "Only allow access through the domain": state and the disabled reason come only from
    the API (`access {domainOnly, canEnable, reason?}` in `GET /api/domains`; logic in
    `lib/panelAccess.ts`, tested). Turning on opens the step-up password dialog
    (`usePanelAccess`); a dismissible `PanelAccessSuggestion` appears when the page is viewed
    through an HTTPS panel domain and nothing is restricted yet.
-   Env manager: optional **BuildKit secret** flag (`is_secret`) when `is_build_arg` is on.

## Dev server

- `make dev` from the repo root runs the Go API and Vite together; or `cd frontend && bun run dev`
  with the API already running on `:8000`.
- Default port: **3600** (`PORT` in `frontend/.env`, bound to `127.0.0.1`, `strictPort`)
- Proxy (`vite.config.ts`): `/api` -> `http://127.0.0.1:8000`, `/ws` -> `ws://127.0.0.1:8000`; the
  `/api` proxy forwards `X-Forwarded-Host` / `X-Forwarded-Proto: http` so cookies and the GitHub
  `setup_url` stay on the Vite origin, and uses its own keep-alive `http.Agent` (without one the
  proxy sends `Connection: close` on a reused keep-alive socket: intermittent `ECONNRESET` / 502)
- Leave `VITE_API_URL` empty for same-origin / proxy (the production build does the same)
- `__APP_VERSION__` is injected from the root `VERSION` file (`vite.config.ts`; `DOCKLIFT_APP_VERSION` overrides it), shown in the sidebar footer
- Path alias: `@/` -> `frontend/src/`

## Checks

```bash
cd frontend
bun install --frozen-lockfile
bunx tsc -p tsconfig.app.json --noEmit    # typecheck (CI)
bunx vite build                           # production build into dist/ (CI)
bun audit --prod                          # must be clean (CI)
```

`bun run build` runs the typecheck and the build together. `tsc` is TypeScript 7's native compiler
(about a second for the app). `package.json` has no `overrides`: add one only to pin a transitive
security fix that `bun audit` reports, and drop it once the parent's own range includes the fix.

## Production

- The root `Dockerfile` runs `bunx vite build` (stage `web`, no typecheck: CI runs tsc) with
  `VITE_API_URL` empty, copies `dist/`
  into `server/web/dist`, and `go build` embeds it. `make build` does the same locally.
- `server/web/dist/index.html` is a tracked placeholder so the Go module always compiles; never
  commit real build output there (`dist/` contents other than the placeholder are gitignored).
- The Go server sets the Content-Security-Policy from the embedded build (see `server/web/web.go`):
  scripts are same-origin plus the pinned Monaco CDN path found in the bundle. Inline scripts in
  `index.html` are allowed by hash; a new third-party script origin needs a matching change there,
  or the browser blocks it in production.
- Hashed `/assets/` files are cached immutably; `index.html` is never cached.
