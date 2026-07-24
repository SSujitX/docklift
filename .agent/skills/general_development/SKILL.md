---
name: General Development
description: Guide for setting up, running, and developing the Docklift project.
---

# General Development Guide

Docklift is a self-hosted Docker deployment platform. This skill covers local setup and the
day-to-day workflow; the other skills go deep on individual subsystems.

## Prerequisites

- **Docker**: installed and running.
- **Bun**: [Bun](https://bun.sh/) is the package manager and script runner.

## Project Structure

```
backend/          Express + Prisma API (TypeScript, ESM)
  src/routes/     HTTP endpoints
  src/services/   Docker, git, build, nginx, certs, terminal
  src/lib/        Config, auth middleware, naming, path/origin security
  prisma/         schema.prisma (no migrations dir — db push on boot)
frontend/         Vite + React 19 dashboard
  src/app/        AppShell + router
  src/components/ shell/, ui/, feature components
  src/pages/      Route pages, incl. docs/
nginx.conf        Dashboard gateway config
nginx-proxy/      Public proxy config, snippets, generated vhosts, certbot state
data/             SQLite DB + uploads (mounted)
deployments/      Project sources + .docklift/ generated runtime state (mounted)
backups/          Backup archives (mounted)
*.sh              install / install-dev / upgrade / uninstall
```

## Quick Start (Development)

```bash
git clone https://github.com/SSujitX/docklift.git
cd docklift
```

**Backend** (terminal 1) — serves `http://localhost:8000`:
```bash
cd backend
cp .env.local.example .env.local   # gitignored; JWT auto-generates if left empty
bun install
bun run db:generate
bun run db:push
bun run dev                        # tsx watch
```

**Frontend** (terminal 2) — serves `http://localhost:3600`:
```bash
cd frontend
bun install
bun run dev
```

### Env file layout

| File | Committed | Purpose |
|------|-----------|---------|
| `backend/.env` | yes | Server/production defaults |
| `backend/.env.local.example` | yes | Template for local overrides |
| `backend/.env.local` | **no** | Your machine: Vite CORS origin, optional GitHub App creds |

Secrets (`JWT_SECRET`, `INTERNAL_API_SECRET`) auto-generate and persist under `data/.secrets` when
not supplied, so a fresh install needs no manual setup.

Because the Vite dev server runs on `:3600` while the API runs on `:8000`, the dev origin is
cross-origin and must be allowed via `CORS_ORIGIN` in `.env.local` — the production build is
same-origin behind `docklift-nginx` and needs nothing.

## Common Commands

### Backend
- `bun run dev` — tsx watch on `:8000`
- `bun run build` — `tsc` → `dist/`
- `bun run test` — `tsx --test` (build resolver)
- `bun run db:studio` / `db:push` / `db:generate`
- `bun run reset-password` — reset the admin password

### Frontend
- `bun run dev` — Vite on `:3600`
- `bun run build` — `tsc -b` + production build into `dist/`
- `bun run preview` — serve the production build

### Docker (full stack)
- `docker compose up -d --build` — production-like stack on `:8080`
- `docker compose logs -f` — all services

### Type-checking from an agent shell (Windows)
`cd` does not persist between calls and `npx tsc` may not resolve. Use one command with the local binary:
```powershell
cd backend; .\node_modules\.bin\tsc --noEmit
cd frontend; .\node_modules\.bin\tsc -b --noEmit
```

## Architecture Notes

- **UI shell**: every authenticated page renders inside `AppShell` — a collapsible left sidebar
  (nav, status, user menu), a breadcrumb top bar, and a `Ctrl+K` command palette. There is no
  global `Header`/`Footer` component any more. See `frontend_development` and `ui_design_system`.
- **Auth**: JWT in `localStorage`, short-lived separate tokens for SSE. See `authentication`.
- **Deployments**: the backend clones/unzips source, resolves Dockerfile vs Railpack, builds a tagged
  image, and writes its **own** compose file under `deployments/.docklift/<projectId>/` — repository
  files are never modified. See `deployment_system`.
- **Networking**: two nginx containers (dashboard gateway `:8080`, public proxy `:80`/`:443`) plus a
  certbot sidecar. See `networking_proxy`.

## Conventions

- Conventional commits (`feat:`, `fix:`, `docs:`…) — releases are derived from them. See `release_process`.
- Reuse `lib/naming.ts` for any Docker name; never build those strings inline.
- Reuse `LogViewer` for anything log-shaped rather than writing another console.
- Read the `security_hardening` checklist before adding an endpoint.

## Troubleshooting

- **"Session validation failed"**: expired/invalid JWT, or the password changed after it was issued.
- **CORS errors in dev**: the dev origin is not in `CORS_ORIGIN` in `backend/.env.local`.
- **Prisma type errors after a schema edit**: `bun run db:generate`.
- **Build errors**: confirm Bun/Node versions and reinstall dependencies.
