---
name: Technology Stack
description: Comprehensive list of all technologies and libraries used in Docklift.
---

# Technology Stack

Docklift is one Go binary (API, WebSocket terminal and embedded dashboard) plus two stock
containers for the public edge. Versions below come from `server/go.mod`,
`frontend/package.json`, `Dockerfile` and `docker-compose.yml`; re-read those files before
quoting a version elsewhere.

## Backend (`server/`)

-   **Language**: [Go](https://go.dev/) 1.27 (`go 1.27`, `toolchain go1.27.4`); the image builds with `golang:1.27.4-alpine3.24`, `CGO_ENABLED=0`
-   **HTTP router**: [chi](https://github.com/go-chi/chi) v5 (`github.com/go-chi/chi/v5`)
-   **Database**: SQLite through the pure-Go [`modernc.org/sqlite`](https://pkg.go.dev/modernc.org/sqlite) driver (no CGO). Plain `database/sql` with hand-written SQL in `internal/store` (no ORM). WAL mode, one writer connection plus a reader pool (`internal/db`)
-   **Migrations**: embedded SQL files in `internal/db/migrations`, tracked in the `_prisma_migrations` table (kept so 2.x databases upgrade in place)
-   **Auth**: [`golang-jwt/jwt/v5`](https://github.com/golang-jwt/jwt) for tokens, bcrypt from `golang.org/x/crypto`
-   **WebSocket**: [`gorilla/websocket`](https://github.com/gorilla/websocket) (web terminal)
-   **Terminal PTY**: [`creack/pty`](https://github.com/creack/pty)
-   **Docker Engine API**: the moby Go client (`github.com/moby/moby/client`, `github.com/moby/moby/api`) for inspect, logs and stats
-   **Docker CLI**: `docker` with the compose and buildx plugins, run through `internal/dockerx` and `internal/execx` for builds and `compose up/down`
-   **App builder**: [Railpack](https://railpack.com/) 0.33.0, checksum-pinned in the `Dockerfile`, run through `docker buildx`
-   **Compose generation**: [`gopkg.in/yaml.v3`](https://pkg.go.dev/gopkg.in/yaml.v3)
-   **Git**: the `git` binary, wrapped by `internal/gitops`
-   **Host metrics**: [`gopsutil/v4`](https://github.com/shirou/gopsutil) (`internal/sysinfo`)
-   **Logging**: standard library `log/slog`
-   **Version stamp**: `docklift/internal/version.Version`, set at link time from the root `package.json` version

## Frontend (`frontend/`)

Built with Bun 1.4.5 (`oven/bun:1.4.5-alpine` in the `Dockerfile`, same pin in CI) and embedded in
the Go binary (`server/web`, `go:embed`).

-   **Bundler / dev server**: [Vite](https://vite.dev/) 6 (dev server on `:3600`, proxies `/api` and `/ws` to `127.0.0.1:8000`)
-   **UI**: [React](https://react.dev/) 19 + [React Router](https://reactrouter.com/) 7 (`react-router-dom`)
-   **Language**: [TypeScript](https://www.typescriptlang.org/) 5.9 (`tsconfig.app.json` is the app project)
-   **Styling**: [Tailwind CSS](https://tailwindcss.com/) 4 through `@tailwindcss/vite`, configured in CSS (`@theme` in `src/styles/globals.css`, no `tailwind.config.ts`); `tw-animate-css`, `class-variance-authority`, `clsx`, `tailwind-merge`
-   **Components**: shadcn-style files in `src/components/ui/` on [Radix UI](https://www.radix-ui.com/) (`react-dialog`, `react-slot`, `react-tabs`)
-   **Data**: React hooks, native `fetch` through `authFetch()` in `src/lib/auth.ts`, `EventSource` for SSE logs
-   **Editor**: [Monaco](https://microsoft.github.io/monaco-editor/) via `@monaco-editor/react`
-   **Terminal**: [xterm.js](https://xtermjs.org/) (`@xterm/xterm` 6 and its fit, clipboard and web-links addons) over WebSocket
-   **Icons**: [Lucide React](https://lucide.dev/)
-   **Notifications**: [Sonner](https://sonner.emilkowal.ski/)
-   **Theming**: custom `ThemeProvider` (`src/lib/theme.tsx`)

## Edge and runtime

Three containers, from `docker-compose.yml` (project name `docklift`):

| Container | Image | Role |
|-----------|-------|------|
| `docklift-backend` | `docklift-backend` (built from the root `Dockerfile`, final stage `alpine:3.24.4`) | Go server on container port 8000, host `:8080` |
| `docklift-nginx-proxy` | `nginx:1.30.8-alpine` | Public `:80`/`:443`, generated vhosts for app and panel domains |
| `docklift-certbot` | `certbot/certbot:v5.10.0` | Let's Encrypt webroot renewals every 12h; the backend triggers issuance with `docker exec` |

The backend image also carries `docker-cli`, `docker-cli-buildx`, `docker-cli-compose`, `git`,
`bash`, `procps`, `util-linux` (for `nsenter`) and the Railpack binary. The backend runs
`privileged` with `pid: host`.

## Tooling

-   **Go**: `gofmt`, `go vet`, `go test`, [`govulncheck`](https://pkg.go.dev/golang.org/x/vuln/cmd/govulncheck) (pinned in the `Makefile` and `ci.yml`)
-   **Tests**: package tests next to the code; black-box HTTP suite in `tests/contract` (own Go module); end-to-end suite against a real install in `tests/e2e`
-   **CI**: GitHub Actions in `.github/workflows/` (`ci.yml`, `install.yml`, `release.yml`, `docs.yml`); actions are pinned by commit SHA
-   **Frontend checks**: `bunx tsc -p tsconfig.app.json --noEmit`, `bunx vite build`, `bun audit --prod`
-   **Shell scripts**: `bash -n` and shellcheck on `install.sh`, `install-dev.sh`, `upgrade.sh`, `uninstall.sh`, `scripts/*.sh`
-   **Releases**: [semantic-release](https://semantic-release.gitbook.io/) (Node 24 in the workflow), configured in `release.config.cjs`
-   **Docs site**: [VitePress](https://vitepress.dev/) in `website/`, deployed to GitHub Pages (https://docklift.dev)
-   **Local entry points**: `Makefile` (`make build`, `test`, `contract`, `frontend`, `lint`, `vuln`, `docker`, `dev`)
