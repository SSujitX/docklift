# Docklift Management Commands

End-to-end reference for install, day-to-day ops, logs, nginx, password reset, versioning and development.

**Compose services** → `backend` · `nginx-proxy` · `certbot`  
**Container names** → `docklift-backend` · `docklift-nginx-proxy` · `docklift-certbot`  
**Backend** → one Go binary (`docklift`) serving the API, the WebSocket terminal and the embedded dashboard on port 8000 (published as host `:8080`)  
**App containers** → `dl_<project-slug>_<id>_<service>` (e.g. `dl_python-smoke_53b01966_app`)  
**Dashboard** → `http://SERVER_IP:8080` · **App publish ports** → `5500–5600`  
**Fresh setup** → copy the **bootstrap secret** from `docker logs docklift-backend` (or `data/.bootstrap-secret`) into the Setup page

---

## Table of contents

1. [Install / start / stop](#1-install--start--stop)
2. [Upgrade & uninstall](#2-upgrade--uninstall)
3. [Logs (all services)](#3-logs-all-services)
4. [Nginx (app and panel domains)](#4-nginx-app-and-panel-domains)
5. [Reset admin password](#5-reset-admin-password)
6. [Version (check & release)](#6-version-check--release)
7. [Projects & containers](#7-projects--containers)
8. [Network & ports](#8-network--ports)
9. [Clean / reset](#9-clean--reset)
10. [Local development](#10-local-development)
11. [Configuration (environment)](#11-configuration-environment)

---

## 1. Install / start / stop

```bash
# Fresh install — latest GitHub release
curl -fsSL https://raw.githubusercontent.com/SSujitX/docklift/master/install.sh | sudo bash
```

```bash
# Fresh install — pin a release (argv; see github.com/SSujitX/docklift/releases)
curl -fsSL https://raw.githubusercontent.com/SSujitX/docklift/master/install.sh | sudo bash -s -- v=3.0.1
```

```bash
# Same pin via env
curl -fsSL https://raw.githubusercontent.com/SSujitX/docklift/master/install.sh | sudo DOCKLIFT_VERSION=3.0.1 bash
```

```bash
# Development build of master (not for production)
curl -fsSL https://raw.githubusercontent.com/SSujitX/docklift/master/install-dev.sh | sudo bash
```

```bash
# From a cloned repo
cd /opt/docklift   # or your clone path
docker compose up -d --build
```

```bash
# First boot: installer prints Dashboard URL + Setup code.
docker logs docklift-backend 2>&1 | grep -A8 'Fresh install'
```

```bash
sudo cat /opt/docklift/data/.bootstrap-secret
```

```bash
# Start / stop / restart (all)
docker compose start
docker compose stop
docker compose restart
```

```bash
# Rebuild / recreate one service (Compose service keys, not container names)
docker compose up -d --build backend
docker compose up -d --force-recreate nginx-proxy
```

```bash
# Status (backend has a Docker HEALTHCHECK on /api/health)
docker compose ps
docker ps --filter name=docklift --filter name=dl_
```

---

## 2. Upgrade & uninstall

```bash
# Safe upgrade to the latest release (keeps data + user app containers)
curl -fsSL https://raw.githubusercontent.com/SSujitX/docklift/master/upgrade.sh | sudo bash
```

```bash
# Upgrade to a specific release
curl -fsSL https://raw.githubusercontent.com/SSujitX/docklift/master/upgrade.sh | sudo bash -s -- v=3.0.1
```

What `upgrade.sh` does, in order:

1. Tags the running images `docklift-backend:pre-upgrade` (and `docklift-frontend:pre-upgrade` on 2.1.x), stops the backend and **verifies** it stopped, then snapshots the database to `backups/docklift_<stamp>.db.bak` and `nginx-proxy/conf.d` to `backups/nginx-conf_<stamp>.tar.gz`.
2. Checks out the release tag.
3. Builds the new `docklift-backend` image while `nginx-proxy` keeps serving app domains.
4. Removes the 2.1.x `docklift-frontend` and `docklift-nginx` containers (the Go backend now serves the dashboard on `:8080`) and starts the stack.
5. Waits up to 180 s for `/api/health` with every service running.
6. Removes the obsolete `docklift-frontend` image.

Any failure after the backend stopped rolls back automatically: previous git ref, database snapshot, nginx confs and pre-upgrade images, then waits for health again. If that also fails, the script prints the exact manual rollback commands. The log is `/opt/docklift/upgrade.log`.

> Upgrading **from 2.1.x** must use this (new) `upgrade.sh` from GitHub as shown above. The dashboard's
> "Upgrade" button on 2.1.x runs the old local script, which cannot switch to the new container layout
> (it fails safely and rolls back).

```bash
# Full uninstall (destructive — deletes Docklift data)
curl -fsSL "https://raw.githubusercontent.com/SSujitX/docklift/master/uninstall.sh?nocache=5" | sudo bash -s -- -y
```

All three scripts honour `DOCKLIFT_INSTALL_DIR` (default `/opt/docklift`).

---

## 3. Logs (all services)

### Platform containers

```bash
# Compose (all services, live)
docker compose logs -f
docker compose logs -f --tail 100 backend
docker compose logs -f --tail 100 nginx-proxy
docker compose logs -f --tail 100 certbot

# By container name
docker logs docklift-backend -f --tail 100        # API, terminal, dashboard (:8080)
docker logs docklift-nginx-proxy -f --tail 100    # app + panel domains (:80/:443)
docker logs docklift-certbot -f --tail 50         # Let's Encrypt renewals
```

The backend logs in `slog` text format (`time=… level=… msg=…`); it never logs query strings, so `?token=` values do not reach the logs.

### Deployed project apps

```bash
# List app containers
docker ps --filter name=dl_

# Logs for one app (replace with your container name)
docker logs dl_python-smoke_53b01966_app -f --tail 200
```

### In the UI

Dashboard → **Logs** (SSE live stream of the platform containers).

---

## 4. Nginx (app and panel domains)

| Container | Role | Host port | Config |
|-----------|------|-----------|--------|
| `docklift-backend` | Dashboard, `/api`, `/ws` (Go server) | **8080** → 8000 | environment (see §11) |
| `docklift-nginx-proxy` | User app domains, panel domain, HTTPS | **80**, **443** | `nginx-proxy/nginx.conf`, `nginx-proxy/conf.d/*.conf` |
| `docklift-certbot` | Let's Encrypt ACME | — | `nginx-proxy/certbot/` |

A panel domain (Settings → Domain) proxies to `docklift-backend.docklift_network:8000` (network-qualified, so a project network cannot answer for the name). Confs written by 2.1.x (upstream `docklift-nginx:80`) or by an earlier 3.0 build (`docklift-backend:8000`) are rewritten once when the new backend boots, then nginx is reloaded.

`docklift-nginx-proxy` has fixed addresses on `docklift_network` (`172.28.255.254`, `fd12:3456:789a::ffff:fffe`); the backend trusts `X-Forwarded-*` only from those and loopback (`TRUSTED_PROXIES`).

```bash
# Test config then reload
docker exec docklift-nginx-proxy nginx -t
docker exec docklift-nginx-proxy nginx -s reload

# List generated domain configs
ls -la /opt/docklift/nginx-proxy/conf.d/

# Follow proxy logs
docker logs docklift-nginx-proxy -f --tail 100
```

---

## 5. Reset admin password

### Production (Docker)

```bash
docker exec -it docklift-backend docklift reset-password
```

Prints a new random password for the first admin user. Change it after login.

### Local development

```bash
cd server
DATA_PATH=../data DATABASE_URL=file:$(pwd)/../data/docklift.db go run ./cmd/docklift reset-password
```

Other subcommands: `docklift serve` (default), `docklift migrate` (apply database migrations and exit),
`docklift panel-access` and `docklift api-token` (below).

### Dashboard access by IP (`panel-access`)

**Settings → Domain → Only allow access through the domain** makes the dashboard answer only through
its HTTPS panel domain: `http://SERVER_IP:8080` then redirects browsers to `https://<domain>` and
refuses API, terminal and live-log requests with `403` (also from the server itself and through SSH
tunnels). `/api/health` keeps answering `{"status":"ok"}`. The same switch from the server:

```bash
docker exec docklift-backend docklift panel-access status          # current state
docker exec docklift-backend docklift panel-access open            # allow http://SERVER_IP:8080 again
docker exec docklift-backend docklift panel-access close           # only through the HTTPS panel domain
docker exec docklift-backend docklift panel-access close panel.example.com   # when there are several
```

`open` is the way back in when the domain stops working. The running server picks up the change
within 2 seconds; no restart. `close` refuses when no panel domain with HTTPS is configured.

### API tokens (`api-token`)

Bearer tokens for scripts, CI and AI agents (the REST API and the MCP endpoint `/mcp`). The same
tokens as **API & MCP** in the dashboard; only a SHA-256 hash is stored and the token is printed once.

```bash
docker exec docklift-backend docklift api-token create ci-deploy                  # full access, no expiry
docker exec docklift-backend docklift api-token create grafana --read --expires 90d
docker exec docklift-backend docklift api-token create ops-agent --allow-dangerous # may roll back, restore, reboot, run commands
docker exec docklift-backend docklift api-token list                              # never shows the token itself
docker exec docklift-backend docklift api-token revoke <id|dl_prefix>              # stops working at once
```

`create` makes the token for the first admin; scope is full unless `--read` (GET requests only).
Changing the account password (also `reset-password`) revokes every token created before it.
Guides: [API Reference](https://docklift.dev/guide/api), [AI Agents (MCP)](https://docklift.dev/guide/mcp).

---

## 6. Version (check & release)

### See current version

```bash
# Source of truth in the repo: the root VERSION file
cat VERSION

# Running server
curl -s http://SERVER_IP:8080/api/health   # only {"status":"ok"} while "Only allow access through the domain" is on
# or from the server: docker exec docklift-backend wget -qO- http://127.0.0.1:8000/api/health
# Version compare against the latest release is in the UI (authenticated)
```

The version is stamped into the Go binary at build time (`-ldflags -X docklift/internal/version.Version=…`): the Docker build reads it from the root `VERSION` file (override with `--build-arg VERSION=…` or `DOCKLIFT_BUILD_VERSION=… docker compose build`), and `make build` does the same.

### How versions are changed

The root `VERSION` file is the single source of the version. Releases are a **manual** run of the
**Release** workflow; it bumps `VERSION` for you.

```bash
# 1) Commit with conventional messages on master (the subjects become the changelog lines)
git commit -m "fix(deploy): description"
# 2) Push — wait for CI + Install to go green
# 3) GitHub → Actions → "Release" → Run workflow: pick bump, run with dry_run first to preview
# Release bumps VERSION, syncs the copies, adds the CHANGELOG.md section, commits
# chore(release): vX.Y.Z, tags vX.Y.Z, publishes the GitHub Release "Docklift vX.Y.Z",
# then deploys Docs (changelog on docklift.dev). Do not edit released CHANGELOG.md sections by hand.
```

| `bump` | Demo (`3.0.0` →) |
|--------|------------------|
| `patch` | `3.0.1` |
| `minor` | `3.1.0` |
| `major` | `4.0.0` |
| `current` | `3.0.0`: releases `VERSION` as it is (after you changed it in a commit); fails if the tag exists |

`dry_run` does everything except the commit, tag, push, GitHub Release and Docs, and prints the diff
and the release notes.

SemVer reminder: **patch** = bug fix / small change, **minor** = new feature, **major** = breaking change.

Changing `VERSION` by hand (then release with `current`):

```bash
echo 3.1.0 > VERSION
./scripts/sync-version.sh          # writes it into package.json, frontend/package.json and the docs' pin examples
./scripts/sync-version.sh check    # what CI runs: fails when a copy drifts
```

Files: `VERSION` · `scripts/sync-version.sh` · `scripts/release-notes.sh` · Workflow: `.github/workflows/release.yml`

### After a release on a server

```bash
curl -fsSL https://raw.githubusercontent.com/SSujitX/docklift/master/upgrade.sh | sudo bash
```

---

## 7. Projects & containers

```bash
# All Docklift + app containers
docker ps -a --filter name=docklift --filter name=dl_

# Inspect one app
docker inspect dl_python-smoke_53b01966_app

# Shell into the backend (Alpine: sh and bash; docker, git, railpack, nsenter available)
docker exec -it docklift-backend bash

# Shell into an app container
docker exec -it dl_python-smoke_53b01966_app sh
```

Naming pattern for apps:

- Compose project / image prefix: `dl-<slug>-<8charId>`
- Container: `dl_<slug>_<8charId>_<service>`

---

## 8. Network & ports

```bash
# Docklift bridge network (172.28.0.0/16 + fd12:3456:789a::/64)
docker network inspect docklift_network

# What is listening (Linux)
sudo ss -tulpn | grep -E ':(80|443|8080|550[0-9])\b'

# Kill a stuck host port (example)
sudo fuser -k 5500/tcp

# App port pool (compose env; default avoids Windows Hyper-V 3000–5000)
# PORT_RANGE_START=5500
# PORT_RANGE_END=5600
```

| Port | Use |
|------|-----|
| `8080` | Dashboard + API + terminal (`docklift-backend`, container port 8000); only `/api/health` while "Only allow access through the domain" is on |
| `80` | Custom domains HTTP + ACME (nginx-proxy) |
| `443` | Custom domains HTTPS (Let's Encrypt) |
| `5500–5600` | Published app ports (default pool) |
| `8000` | Backend inside the network / local `make dev` |
| `3600` | Vite dev server (local `make dev`) |

To keep the port open on the network but answer only through the panel domain, use **Only allow
access through the domain** (section 5). Bind the dashboard to localhost only (then use a panel
domain or an SSH tunnel; with the switch on, an SSH tunnel to `:8080` is refused too):

```bash
cd /opt/docklift && DASHBOARD_BIND=127.0.0.1 docker compose up -d backend
```

---

## 9. Clean / reset

Prefer the dashboard **System → Purge** (step-up password) for Docklift-scoped cleanup: unused `docklift-*` app tags outside keep-2 + full BuildKit wipe. Successful deploys already keep only 2 images per service and prune unused BuildKit cache.

```bash
# Manual equivalents (host shell) — Docklift-scoped only; never system prune on shared hosts
docker images --format '{{.Repository}}:{{.Tag}}' | grep '^docklift-[a-f0-9]\{8\}-'
docker builder prune -f          # unused BuildKit cache (same as post-deploy)
docker builder prune -af         # all BuildKit cache (same as System Purge)

# Remove the pre-upgrade image once the new version is confirmed good
docker rmi docklift-backend:pre-upgrade

# Stop stack (keeps data dirs)
docker compose down

# Stop + remove containers/network (data dirs on disk remain: ./data, ./deployments, …)
docker compose down --remove-orphans

# Nuclear uninstall script (see §2)

# Free a range of stuck TCP ports (Linux) — adjust to your PORT_RANGE
for port in {5500..5600}; do sudo fuser -k ${port}/tcp 2>/dev/null; done
```

---

## 10. Local development

Requirements: Go (version in `server/go.mod`), Bun (`1.4.5`), bash + make; Docker for deploy features.

```bash
make dev        # Go API on :8000 + Vite on http://127.0.0.1:3600 (proxies /api and /ws to :8000)
make build      # dashboard + Go binary with the dashboard embedded → bin/docklift
make server     # Go binary only (placeholder dashboard; enough for API work)
make test       # go vet + go test ./... (server), go vet (contract suite)
make contract   # contract suite against bin/docklift
make frontend   # bun install --frozen-lockfile, typecheck, vite build, bun audit --prod
make lint       # gofmt, go vet, bash -n + shellcheck, actionlint, compose config
make vuln       # govulncheck
make docker     # docker build -t docklift-backend .
```

`make dev` keeps local state in `./data`, `./deployments`, `./backups` and `./nginx-proxy` (gitignored). Docker-dependent tests run with a daemon:

```bash
DOCKLIFT_DOCKER=1 make test contract
```

Without make:

```bash
# Backend
cd server
go run ./cmd/docklift                 # reads .env / .env.local from the working directory
go test ./...

# Frontend (other terminal)
cd frontend
bun install --frozen-lockfile
bun run dev                           # http://127.0.0.1:3600
bun run build                         # production → dist/ (embedded via server/web/dist)

# Contract suite
cd server && go build -o ../bin/docklift ./cmd/docklift
cd ../tests/contract && DOCKLIFT_GO_BIN=$(pwd)/../../bin/docklift go test ./...
```

Health check while developing:

```bash
curl -s http://127.0.0.1:8000/api/health
curl -s http://127.0.0.1:3600/api/auth/status   # via Vite proxy
```

The SQLite database (`data/docklift.db`, WAL mode) can be inspected with `sqlite3`; migrations are embedded in the binary (`server/internal/db/migrations`) and applied on start (`docklift migrate` applies them without serving).

---

## 11. Configuration (environment)

Set in `docker-compose.yml` (values in `${…}` can be overridden from the shell or a `.env` file next to it).

| Variable | Default (compose) | Meaning |
|---|---|---|
| `DASHBOARD_BIND` | `0.0.0.0` | Host address for the dashboard port 8080 |
| `PORT` | `8000` | Backend listen port inside the container |
| `DATABASE_URL` | `file:/app/data/docklift.db` | SQLite file (`file:` URL) |
| `DATA_PATH` | `/app/data` | `.secrets`, bootstrap secret, setup token, restore markers |
| `DEPLOYMENTS_PATH` | `/deployments` | Project sources and runtime compose files |
| `BACKUP_PATH` | `/data/backups` | Backup zips |
| `NGINX_CONF_PATH` | `/nginx-conf` | Generated nginx-proxy confs |
| `LETSENCRYPT_PATH` | `/etc/letsencrypt` | Certificates (shared with certbot / nginx-proxy) |
| `DOCKER_NETWORK` | `docklift_network` | Control-plane network |
| `PORT_RANGE_START` / `PORT_RANGE_END` | `5500` / `5600` | Host port pool for apps |
| `JWT_SECRET` | empty | Empty = generated and kept in `DATA_PATH/.secrets` |
| `INTERNAL_API_SECRET` | empty | Unused (the `X-Internal-Secret` header is no longer accepted); still kept in `.secrets` for compatibility |
| `CORS_ORIGIN` | empty | Extra allowed browser origins (comma-separated) |
| `TRUSTED_PROXIES` | loopback + nginx-proxy's fixed IPs | Peers whose `X-Forwarded-*` headers are believed (`none` = never) |
| `DOCKLIFT_FRONTEND_URL` | `http://localhost:8080` | Dashboard URL used for GitHub App redirects |
| `CERTBOT_EMAIL` / `CERTBOT_STAGING` | empty / `false` | Let's Encrypt account e-mail / staging CA |
| `CERTBOT_CONTAINER` / `NGINX_PROXY_CONTAINER` | `docklift-certbot` / `docklift-nginx-proxy` | Sidecar container names |
| `DOCKLIFT_GITHUB_APP_ID`, `DOCKLIFT_GITHUB_CLIENT_ID`, `DOCKLIFT_GITHUB_CLIENT_SECRET`, `DOCKLIFT_GITHUB_PRIVATE_KEY_PATH` | unset | GitHub App (normally configured from the dashboard) |
| `TERMINAL_ALLOW_NO_ORIGIN` | unset | `true` allows terminal WebSocket clients without an `Origin` header |
| `DOCKLIFT_BUILD_VERSION` | root `VERSION` file | Version stamped into the image at build time |

---

## Quick cheat sheet

| Task | Command |
|------|---------|
| Start stack | `docker compose up -d --build` |
| Upgrade server | `curl -fsSL …/upgrade.sh \| sudo bash` |
| Backend logs | `docker logs docklift-backend -f` |
| Domain proxy logs | `docker logs docklift-nginx-proxy -f` |
| Reload domain nginx | `docker exec docklift-nginx-proxy nginx -s reload` |
| Reset password | `docker exec -it docklift-backend docklift reset-password` |
| Reopen `:8080` (domain-only on) | `docker exec docklift-backend docklift panel-access open` |
| New API token (scripts, MCP) | `docker exec docklift-backend docklift api-token create <name>` |
| Check version | `cat VERSION` / `docker exec docklift-backend wget -qO- http://127.0.0.1:8000/api/health` |
| Run everything CI runs | `make lint test contract frontend docker` |
| Ship a release | green CI + Install → Actions → **Release** |
| List apps | `docker ps --filter name=dl_` |
