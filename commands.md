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
curl -fsSL https://raw.githubusercontent.com/SSujitX/docklift/master/install.sh | sudo bash -s -- v=2.2.0
```

```bash
# Same pin via env
curl -fsSL https://raw.githubusercontent.com/SSujitX/docklift/master/install.sh | sudo DOCKLIFT_VERSION=2.2.0 bash
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
curl -fsSL https://raw.githubusercontent.com/SSujitX/docklift/master/upgrade.sh | sudo bash -s -- v=2.2.0
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

Other subcommands: `docklift serve` (default) and `docklift migrate` (apply database migrations and exit).

---

## 6. Version (check & release)

### See current version

```bash
# Source of truth in the repo: the root package.json
grep '"version"' package.json

# Running server
curl -s http://SERVER_IP:8080/api/health
# or from the server: docker exec docklift-backend wget -qO- http://127.0.0.1:8000/api/health
# Version compare against the latest release is in the UI (authenticated)
```

The version is stamped into the Go binary at build time (`-ldflags -X docklift/internal/version.Version=…`): the Docker build reads it from the root `package.json` (override with `--build-arg VERSION=…` or `DOCKLIFT_BUILD_VERSION=… docker compose build`), and `make build` does the same.

### How versions are changed (do not hand-edit)

Versioning is **automatic** via [semantic-release](https://github.com/semantic-release/semantic-release).  
Do **not** use `npm version`, `bumpp`, or manual bumps unless you know you are breaking the pipeline.

```bash
# Assume current version is 1.3.21 (root package.json; frontend/package.json follows it)

# 1) Commit with conventional messages on master
git commit -m "fix(deploy): description"       # → patch  → 1.3.22
git commit -m "feat(api): description"          # → patch  → 1.3.22
git commit -m "chore: cleanup"                  # → patch  → 1.3.22
git commit -m "feat: something *force minor*"   # → minor  → 1.4.0
git commit -m "feat: something *force major*"   # → major  → 2.0.0
git commit -m "chore: docs *skip release*"      # → none   → stays 1.3.21

# 2) Push — wait for CI + Install to go green
# 3) GitHub → Actions → "Release" → Run workflow
# semantic-release bumps package.json (root + frontend), CHANGELOG.md, tag + GitHub Release
# Release then deploys Docs (changelog on docklift.dev). Do not edit CHANGELOG.md by hand.
```

| Commit signal | Release | Demo (`1.3.21` →) |
|---------------|---------|-------------------|
| `feat:`, `fix:`, `perf:`, `refactor:`, `docs:`, `test:`, `ci:`, `chore:` … | Patch | `1.3.22` |
| `*force minor*` in subject | Minor | `1.4.0` |
| `*force major*` / `BREAKING CHANGE` | Major | `2.0.0` |
| `*skip release*` in subject | None | `1.3.21` (unchanged) |

SemVer reminder: **patch** = bugfix / small change, **minor** = new feature (forced here), **major** = breaking change.

Config: `release.config.cjs` · Workflow: `.github/workflows/release.yml`

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
| `8080` | Dashboard + API + terminal (`docklift-backend`, container port 8000) |
| `80` | Custom domains HTTP + ACME (nginx-proxy) |
| `443` | Custom domains HTTPS (Let's Encrypt) |
| `5500–5600` | Published app ports (default pool) |
| `8000` | Backend inside the network / local `make dev` |
| `3600` | Vite dev server (local `make dev`) |

Bind the dashboard to localhost only (then use a panel domain or an SSH tunnel):

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
make contract   # contract suite against bin/docklift (DOCKLIFT_TARGET=go)
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
cd ../tests/contract && DOCKLIFT_TARGET=go DOCKLIFT_GO_BIN=$(pwd)/../../bin/docklift go test ./...
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
| `DOCKLIFT_BUILD_VERSION` | root `package.json` | Version stamped into the image at build time |

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
| Check version | `grep '"version"' package.json` / `curl -s localhost:8080/api/health` |
| Run everything CI runs | `make lint test contract frontend docker` |
| Ship a release | green CI + Install → Actions → **Release** |
| List apps | `docker ps --filter name=dl_` |
