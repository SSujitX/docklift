# Commands Reference

End-to-end reference for install, day-to-day ops, logs, nginx, password reset, networking, versioning, and local development.

| Item | Value |
|------|--------|
| Compose services | `backend` · `nginx-proxy` · `certbot` |
| Container names | `docklift-backend` · `docklift-nginx-proxy` · `docklift-certbot` |
| Backend | One Go binary (`docklift`) serving the API, the WebSocket terminal, and the embedded dashboard on port 8000 (published as host `:8080`) |
| App containers | `dl_<project-slug>_<id>_<service>` (e.g. `dl_python-smoke_53b01966_app`) |
| Dashboard | `http://SERVER_IP:8080` |
| App publish ports | `5500`–`5600` (opt-in) |
| Fresh setup | Bootstrap secret from `docker logs docklift-backend` or `data/.bootstrap-secret` |

Run Compose commands from `/opt/docklift` on production installs (or your clone path).

## 1. Install / start / stop

```bash
# Fresh install — latest GitHub release
curl -fsSL https://raw.githubusercontent.com/SSujitX/docklift/master/install.sh | sudo bash
```

```bash
# Fresh install — pin a release (argv)
curl -fsSL https://raw.githubusercontent.com/SSujitX/docklift/master/install.sh | sudo bash -s -- v=3.0.0
```

```bash
# Same pin via env
curl -fsSL https://raw.githubusercontent.com/SSujitX/docklift/master/install.sh | sudo DOCKLIFT_VERSION=3.0.0 bash
```

```bash
# From a cloned repo
cd /opt/docklift
docker compose up -d --build
```

```bash
# First boot: Dashboard URL + Setup code
docker logs docklift-backend 2>&1 | grep -A8 'Fresh install'
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

## 2. Upgrade & uninstall

```bash
# Safe upgrade to the latest release (keeps data + user app containers)
curl -fsSL https://raw.githubusercontent.com/SSujitX/docklift/master/upgrade.sh | sudo bash
```

```bash
# Upgrade to a specific release
curl -fsSL https://raw.githubusercontent.com/SSujitX/docklift/master/upgrade.sh | sudo bash -s -- v=3.0.0
```

What `upgrade.sh` does, in order:

1. Tags the running images `docklift-backend:pre-upgrade` (and `docklift-frontend:pre-upgrade` on 2.1.x), stops the backend and verifies it stopped, then snapshots the database to `backups/docklift_<stamp>.db.bak` and `nginx-proxy/conf.d` to `backups/nginx-conf_<stamp>.tar.gz`.
2. Checks out the release tag.
3. Builds the new `docklift-backend` image while `nginx-proxy` keeps serving app domains.
4. Removes the 2.1.x `docklift-frontend` and `docklift-nginx` containers (the Go backend now serves the dashboard on `:8080`) and starts the stack.
5. Waits up to 180 s for `/api/health` with every service running.
6. Removes the obsolete `docklift-frontend` image.

Any failure after the backend stopped rolls back automatically: previous git ref, database snapshot, nginx confs, and pre-upgrade images, then waits for health again. If that also fails, the script prints the exact manual rollback commands. The log is `/opt/docklift/upgrade.log`.

::: warning Upgrading from 2.1.x
Use the new `upgrade.sh` from GitHub as shown above. The dashboard's **Upgrade** button on 2.1.x runs the old local script, which cannot switch to the new container layout (it fails safely and rolls back).
:::

```bash
# Full uninstall (destructive — deletes Docklift data)
curl -fsSL "https://raw.githubusercontent.com/SSujitX/docklift/master/uninstall.sh" | sudo bash -s -- -y
```

```bash
# Development build from master (unreleased)
curl -fsSL https://raw.githubusercontent.com/SSujitX/docklift/master/install-dev.sh | sudo bash
```

All install, upgrade, and uninstall scripts honour `DOCKLIFT_INSTALL_DIR` (default `/opt/docklift`).

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

The backend logs in `slog` text format (`time=… level=… msg=…`). It never logs query strings, so `?token=` values do not reach the logs.

### Deployed project apps

```bash
docker ps --filter name=dl_
docker logs dl_python-smoke_53b01966_app -f --tail 200
```

### In the UI

Dashboard → **Logs** streams the platform containers (SSE live stream).

## 4. Nginx (app and panel domains)

| Container | Role | Host port | Config |
|-----------|------|-----------|--------|
| `docklift-backend` | Dashboard, `/api`, `/ws` (Go server) | **8080** → 8000 | environment (see §11) |
| `docklift-nginx-proxy` | User app domains, panel domain, HTTPS | **80**, **443** | `nginx-proxy/nginx.conf`, `nginx-proxy/conf.d/*.conf` |
| `docklift-certbot` | Let's Encrypt ACME | — | `nginx-proxy/certbot/` |

A panel domain (**Settings → Domain**) proxies to `docklift-backend.docklift_network:8000`. The name is network-qualified so a project network cannot answer for it. Confs written by 2.1.x (upstream `docklift-nginx:80`) are rewritten once when the new backend boots, then nginx is reloaded.

`docklift-nginx-proxy` has fixed addresses on `docklift_network` (`172.28.255.254`, `fd12:3456:789a::ffff:fffe`). The backend trusts `X-Forwarded-*` headers only from those and loopback (`TRUSTED_PROXIES`).

```bash
# Test config then reload
docker exec docklift-nginx-proxy nginx -t
docker exec docklift-nginx-proxy nginx -s reload

# List generated domain configs
ls -la /opt/docklift/nginx-proxy/conf.d/

# Follow proxy logs
docker logs docklift-nginx-proxy -f --tail 100
```

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

## 6. Version (check & release)

### See current version

```bash
# Source of truth in the repo: the root VERSION file
cat VERSION
```

```bash
curl -s http://SERVER_IP:8080/api/health   # only {"status":"ok"} while "Only allow access through the domain" is on
# or from the server:
docker exec docklift-backend wget -qO- http://127.0.0.1:8000/api/health
```

The UI also shows the version in the header / footer (authenticated compare against the latest release). The version is stamped into the Go binary at build time: the Docker build reads it from the root `VERSION` file (override with `--build-arg VERSION=…` or `DOCKLIFT_BUILD_VERSION=… docker compose build`), and `make build` does the same.

### How versions are changed

The root `VERSION` file is the single source of the version. Releases are a **manual** run of the
**Release** workflow; it bumps `VERSION` for you.

```bash
# 1) Commit with conventional messages on master (the subjects become the changelog lines)
git commit -m "fix(deploy): description"
# 2) Push — wait for CI + Install (Ubuntu) to go green
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

Files: [`VERSION`](https://github.com/SSujitX/docklift/blob/master/VERSION) · [`scripts/sync-version.sh`](https://github.com/SSujitX/docklift/blob/master/scripts/sync-version.sh) · [`scripts/release-notes.sh`](https://github.com/SSujitX/docklift/blob/master/scripts/release-notes.sh) · Workflow: [`.github/workflows/release.yml`](https://github.com/SSujitX/docklift/blob/master/.github/workflows/release.yml)

### After a release on a server

```bash
curl -fsSL https://raw.githubusercontent.com/SSujitX/docklift/master/upgrade.sh | sudo bash
```

## 7. Projects & containers

```bash
docker ps -a --filter name=docklift --filter name=dl_
docker inspect dl_python-smoke_53b01966_app
docker exec -it docklift-backend bash
docker exec -it dl_python-smoke_53b01966_app sh
```

Naming:

- Compose project / image prefix: `dl-<slug>-<8charId>`
- Container: `dl_<slug>_<8charId>_<service>`

## 8. Network & ports

```bash
docker network inspect docklift_network
docker network ls --filter label=com.docklift.managed=true

sudo ss -tulpn | grep -E ':(80|443|8080|550[0-9])\b'
sudo fuser -k 5500/tcp
```

| Port | Use |
|------|-----|
| `8080` | Dashboard + API + terminal (`docklift-backend`, container port 8000); only `/api/health` while "Only allow access through the domain" is on |
| `80` | Custom domains HTTP + ACME (nginx-proxy) |
| `443` | Custom domains HTTPS |
| `5500`–`5600` | Published app ports (opt-in pool) |
| `8000` | Backend inside the network / local `make dev` |
| `3600` | Vite dev server (local `make dev`) |

Compose env defaults:

```bash
# PORT_RANGE_START=5500
# PORT_RANGE_END=5600
# DASHBOARD_BIND=0.0.0.0
```

To keep the port open on the network but answer only through the panel domain, use **Only allow
access through the domain** (section 5). Bind the dashboard to localhost only (then use a panel
domain or an SSH tunnel; with the switch on, an SSH tunnel to `:8080` is refused too):

```bash
cd /opt/docklift && DASHBOARD_BIND=127.0.0.1 docker compose up -d backend
```

## 9. Clean / reset

```bash
# Stop stack (keeps volumes/data dirs)
docker compose down

# Stop + remove containers/network (data dirs on disk remain)
docker compose down --remove-orphans

# Remove the pre-upgrade image once the new version is confirmed good
docker rmi docklift-backend:pre-upgrade

# Free stuck app port pool
for port in {5500..5600}; do sudo fuser -k ${port}/tcp 2>/dev/null; done
```

Nuclear remove: see uninstall one-liner in §2.

Volumes labelled for a project:

```bash
docker volume ls --filter label=com.docklift.project
```

## 10. Local development

Requirements: Go (version in `server/go.mod`), Bun (`1.4.5`), bash + make. Docker is needed for deploy features.

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

`make dev` keeps local state in `./data`, `./deployments`, `./backups`, and `./nginx-proxy` (gitignored). Docker-dependent tests run when a daemon is available:

```bash
DOCKLIFT_DOCKER=1 make test contract
```

Without make:

```bash
# Backend
cd server
go run ./cmd/docklift                 # reads .env / .env.local from the working directory
go test ./...
```

```bash
# Frontend (other terminal)
cd frontend
bun install --frozen-lockfile
bun run dev                           # http://127.0.0.1:3600
bun run build                         # production → dist/ (embedded via server/web/dist)
```

```bash
curl -s http://127.0.0.1:8000/api/health
curl -s http://127.0.0.1:3600/api/auth/status   # via Vite proxy
```

The SQLite database (`data/docklift.db`, WAL mode) can be inspected with `sqlite3`. Migrations are embedded in the binary and applied on start; `docklift migrate` applies them without serving.

## 11. Configuration (environment)

Set in `docker-compose.yml`. Values written as `${…}` can be overridden from the shell or a `.env` file next to it.

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
| List apps | `docker ps --filter name=dl_` |
| Ship a release | green CI + Install → Actions → **Release** |
