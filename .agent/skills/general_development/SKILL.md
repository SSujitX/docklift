---
name: General Development
description: Guide for setting up, running, and developing the Docklift project.
---

# General Development Guide

Docklift is a self-hosted Docker deployment platform. The backend is a Go module in `server/`;
the dashboard is a React app in `frontend/` that is embedded into the Go binary. This skill covers
local setup and the day-to-day workflow; the other skills go deep on individual subsystems.

## Prerequisites

- **Go**: version from `server/go.mod` (1.27, toolchain 1.27.4).
- **Bun**: 1.4.5 (the version CI and the `Dockerfile` use). Package manager and script runner for `frontend/`.
- **Docker** (daemon reachable via `DOCKER_HOST` or the default socket): only for Docker-backed features and the Docker-gated tests. API-only work and the contract suite run without it.
- **bash** and `make`: the `Makefile` uses bash. On Windows use Git Bash.

## Project Structure

```
server/                  Go module `docklift` (go.mod)
  cmd/docklift/          main.go: subcommands serve (default), migrate, reset-password, panel-access, api-token
  internal/
    api/<group>/         One package per route group (auth, projects, deployments, databases,
                         domains, files, github, health, logs, ports, system, terminal, backup, tokens)
    api/openapi/         The route table (every route, request shapes) and GET /api/openapi.json
    api/mcp/             The MCP server at /mcp (tool calls dispatched through the router)
    app/                 Deps (shared dependencies) and lifecycle; app/router/router.go is the mount table
    auth/                JWT, passwords, persisted secrets, bootstrap/setup tokens
    backup/              Backup and restore archives
    certs/ nginx/ dnscheck/   Let's Encrypt, nginx vhost generation, DNS checks
    panelaccess/         "Only allow access through the domain": cached setting + root middleware
    config/              Every environment variable the server reads (config.go) + .env loading
    db/                  SQLite open (WAL, writer/reader pools), embedded migrations, ensure/repair
    deploy/              Build resolver, compose generation, ports, env vars, deploy status
    dockerx/ execx/      Docker Engine client wrapper + docker CLI runner
    gitops/ githubapp/   git operations, GitHub App
    httpx/               Middleware, JSON/body helpers, rate limiter, trusted proxies
    store/               Hand-written SQL, one file per table
    terminal/ sysinfo/ maintenance/ pathsec/ safeunzip/ fscopy/ version/ ...
  web/                   go:embed of the dashboard (web/dist holds a tracked placeholder index.html)
frontend/                Vite + React 19 dashboard
  src/app/               AppShell + router
  src/components/        shell/, ui/, domains/, databases/, project/, feature components
  src/pages/             Route pages
  src/lib/               auth, theme, streamProgress, focusTrap, ...
tests/contract/          Black-box HTTP suite against the real binary (own Go module)
tests/e2e/               End-to-end suite against a real install (own Go module)
nginx-proxy/             nginx config, snippets, generated vhosts, certbot state (mounted)
data/                    SQLite DB + uploads (mounted)
deployments/             Project sources + .docklift/ generated runtime state (mounted)
backups/                 Backup archives (mounted)
scripts/                 smoke.sh and shell-script tests
website/                 VitePress docs site (https://docklift.dev)
*.sh                     install / install-dev / upgrade / uninstall
```

The old TypeScript backend (`backend/`: Express, Prisma, Bun) no longer exists. Do not recreate
patterns from it (`lib/*.ts`, `prisma/schema.prisma`, `bun run db:*`).

## Quick Start (Development)

```bash
git clone https://github.com/SSujitX/docklift.git
cd docklift
make dev
```

`make dev` starts both processes and stops them together:

- Go API (`go run ./cmd/docklift serve` from `server/`) on `http://localhost:8000`. State goes
  to `./data`, `./deployments`, `./backups` and `./nginx-proxy` (all gitignored);
  `DOCKLIFT_FRONTEND_URL` is set to `http://127.0.0.1:3600`.
- Vite dev server on `http://127.0.0.1:3600`, proxying `/api` and `/ws` to `127.0.0.1:8000`.
  Open the dashboard on `:3600`.

Or run them by hand in two terminals:

```bash
cd server && go run ./cmd/docklift serve                      # :8000 (PORT changes it)
cd frontend && bun install --frozen-lockfile && bun run dev   # :3600
```

The server creates and migrates the SQLite file at startup. JWT and internal secrets are generated
and persisted under `DATA_PATH/.secrets` when `JWT_SECRET` / `INTERNAL_API_SECRET` are empty. The
first admin needs the bootstrap secret, stored in `DATA_PATH/.bootstrap-secret` and printed at startup.

### Environment

All variables are read in `server/internal/config/config.go` (single source of truth). At startup
the server also loads `.env` (never overrides the real environment) and `.env.local` (overrides)
from its working directory (`config.LoadDotenv`). `frontend/.env` holds the Vite dev `PORT`
(default 3600); `VITE_API_URL` stays empty so the browser calls the same origin.

Because Vite proxies `/api` and `/ws`, local development needs no `CORS_ORIGIN`. Set it only when
the dashboard is served from an origin other than the API's.

## Common Commands

Run from the repository root.

| Command | What it does |
|---------|--------------|
| `make build` | Dashboard build + Go binary with the dashboard embedded -> `bin/docklift` |
| `make server` | Go binary with the placeholder dashboard (enough for API work and the contract suite) |
| `make test` | `go vet` + `go test -count=1 ./...` in `server`, `go vet` in `tests/contract` |
| `make contract` | Contract suite against the Go binary (`DOCKLIFT_GO_BIN`) |
| `make frontend` | `bun install --frozen-lockfile`, typecheck, `vite build`, `bun audit --prod` |
| `make lint` | gofmt, go vet, `bash -n` + shellcheck, actionlint, upgrade-script test, compose config |
| `make vuln` | govulncheck |
| `make docker` | `docker build -t docklift-backend .` |
| `make dev` | API + Vite dev server |

Docker-gated tests are skipped by default; run them with a daemon available:
`DOCKLIFT_DOCKER=1 make test contract`.

Server subcommands (also usable in the container):

```bash
docklift serve            # default; applies migrations, then listens
docklift migrate          # apply migrations and exit
docker exec -it docklift-backend docklift reset-password   # new random password for the first admin
```

Frontend (from `frontend/`): `bun run dev`, `bun run build` (typecheck + production build into
`dist/`), `bun run preview`.

Full stack in Docker: `docker compose up -d --build` (dashboard on `:8080`), `docker compose logs -f`.

### Type-checking from an agent shell (Windows)

`cd` does not persist between calls. Use one command per check:

```bash
cd server && go vet ./...
cd frontend && bunx tsc -p tsconfig.app.json --noEmit
```

`tsc` is TypeScript 7 (native); it has no `tsserver.js`, so an editor pointed at the workspace
TypeScript needs the TypeScript 7 language server (VS Code: "TypeScript Native Preview").

## Architecture Notes

- **One binary**: `docklift serve` serves `/api`, the `/ws/terminal` WebSocket and the embedded
  dashboard (SPA fallback, `server/web`). There is no separate frontend or dashboard-gateway container.
- **UI shell**: every authenticated page renders inside `AppShell`: collapsible left sidebar, breadcrumb
  top bar and a `Ctrl+K` command palette. See `frontend_development` and `ui_design_system`.
- **Auth**: JWT in `localStorage`, short-lived separate tokens for SSE and terminal. See `authentication`.
- **Deployments**: the backend clones/unzips source, resolves Dockerfile vs Railpack, builds a tagged
  image, and writes its **own** compose file under `deployments/.docklift/<projectId>/`; repository
  files are never modified. Apps land on **per-project networks**; host ports are opt-in. See
  `deployment_system`.
- **Networking**: `docklift-backend` on host `:8080` (default `0.0.0.0`), `docklift-nginx-proxy` on
  `:80`/`:443`, and a certbot sidecar. The proxy attaches to each project network and routes to
  `container_name:internal_port`. See `networking_proxy`.
- **Onboarding**: install prints `http://SERVER_IP:8080` + a bootstrap setup code; the first account
  cannot be claimed without it. See `authentication`.

## Adding a route

1. Put the group in `server/internal/api/<group>/` (a new package, or a new file in an existing one).
   The package exposes `Mount(r chi.Router, d *app.Deps)`, which registers handlers on a router that
   is already scoped to `/api/<group>` and already wrapped in the group's auth middleware.
2. Handlers are methods on a small `handlers` struct holding `d *app.Deps` (see
   `server/internal/api/ports/ports.go`): `r.Get("/", h.list)`, `r.Delete("/{port}", h.release)`.
   Read path params with `chi.URLParam`, write responses with `httpx.JSON(w, status, v)` and
   `httpx.Error(w, status, msg)`, and report unexpected failures with `httpx.InternalError` (logs the
   cause, returns a generic 500). The request body is already parsed by the group's `body`
   middleware (`httpx.BodyFrom(r)`).
3. Data access goes through `d.Store` (add a method in `internal/store`, see below); Docker through
   `d.Docker` (Engine client) or `d.DockerX`.
4. Wire the group in `server/internal/app/router/router.go`, inside `apiRouter`:
   `group("/name", pkg.Mount, session, body)` (`session` is `d.Auth.RequireSession`). Public routes,
   SSE-token routes and step-up routes follow the guards already defined in that file; read the
   `security_hardening` skill before adding one.
5. Add the route to `server/internal/api/openapi/routes.go` with its real request shape (body keys,
   query parameters, step-up, streaming, `SessionOnly`/`FullScope`): `TestOpenAPITableMatchesRouter`
   fails until the router and the table agree. That table is `GET /api/openapi.json`, the dashboard's
   endpoint browser and the MCP `docklift_list_endpoints` tool. It is also the website's
   **generated** page `website/guide/api-reference.md`: after changing the table, run
   `go run ./cmd/gen-docs` in `server/` and commit the result (never edit that page by hand). The
   same command rewrites the "## Tool reference" section of `website/guide/mcp.md` (up to the next
   `## ` heading; the rest of that page is hand-written) from `server/internal/api/mcp/tools.go`, so
   run it after changing a tool too. `TestGeneratedDocsUpToDate` (`server/cmd/gen-docs`, part of `go test ./...`) fails while
   either page is stale. Both are pinned to LF in `.gitattributes`; the test compares bytes.
6. Decide what an API token may do: nothing to add for normal routes (read tokens are GET-only); add
   `r.With(auth.SessionOnly)` for account, token or lock-out routes, `auth.FullScopeOnly` for a GET
   that returns every secret. A dangerous route calls `RequireStepUpPassword`.
7. Add a handler test using `apptest.NewDeps(t)` (`server/internal/app/apptest`), list the route in
   `tests/contract/ROUTES.md`, and cover it in `tests/contract`.

## Adding a store query

`internal/store` has one file per table (`projects.go`, `ports.go`, ...). Methods hang off `*Store`,
use `s.reader()` for reads and `s.writer()` for writes, and call the helpers in `store.go`
(`queryList`, `queryOne`, `execAffected`, `execOne`; `InTx` for transactions). Write plain SQL with
quoted identifiers and `?` placeholders. Unique violations surface as `ErrConflict`, missing rows as
`ErrNotFound`. Schema changes need a migration: see `database_management`.

## Conventions

- Run `gofmt` (CI fails on unformatted files in `server` and `tests/contract`) and `go vet`.
- Conventional commits (`feat:`, `fix:`, `docs:`...). Releases are derived from them. See `release_process`.
- Build Docker names with the helpers in `server/internal/deploy/naming.go`; never build those strings inline.
- Reuse `LogViewer` for anything log-shaped rather than writing another console.
- Read the `security_hardening` checklist before adding an endpoint.
- **Always keep docs in sync with code.** For any major behavior change (bind defaults, networking,
  deploy lifecycle, auth/setup, purge/restore, schema, install/upgrade scripts), update in the
  **same change**:
  1. matching `.agent/skills/*/SKILL.md`
  2. operator docs (`README.md`, `commands.md`)
  3. install/upgrade/uninstall `.sh` scripts when behavior changes
  4. public docs under `website/guide/` (VitePress, https://docklift.dev)
  Do not ship code-only updates and leave skills/docs/scripts stale.

## Troubleshooting

- **"Session validation failed"**: expired/invalid JWT, or the password changed after it was issued.
- **Dashboard shows a placeholder page**: the binary was built without the dashboard. Use
  `make build` (or the Docker image), or open the Vite dev server on `:3600`.
- **Docker features fail locally**: no reachable daemon; set `DOCKER_HOST` or start Docker.
- **`gofmt needed` in CI**: run `gofmt -l server tests/contract` and `gofmt -w` the listed files.
- **Migration failure at startup**: see `database_management`.
