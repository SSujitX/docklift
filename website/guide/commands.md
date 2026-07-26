# Commands Reference

End-to-end reference for install, day-to-day ops, logs, nginx, password reset, networking, and versioning.

| Item | Value |
|------|--------|
| Compose services | `backend` · `frontend` · `nginx` · `nginx-proxy` · `certbot` |
| Container names | `docklift-backend` · `docklift-frontend` · `docklift-nginx` · `docklift-nginx-proxy` · `docklift-certbot` |
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
curl -fsSL https://raw.githubusercontent.com/SSujitX/docklift/master/install.sh | sudo bash -s -- v=2.0.2
```

```bash
# Same pin via env
curl -fsSL https://raw.githubusercontent.com/SSujitX/docklift/master/install.sh | sudo DOCKLIFT_VERSION=2.0.2 bash
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
# Rebuild one service (Compose service keys, not container names)
docker compose up -d --build backend
docker compose up -d --build frontend
docker compose up -d --build nginx
docker compose up -d --build nginx-proxy
```

```bash
# Status
docker compose ps
docker ps --filter name=docklift --filter name=dl_
```

## 2. Upgrade & uninstall

```bash
# Safe upgrade (keeps data + user app containers)
curl -fsSL https://raw.githubusercontent.com/SSujitX/docklift/master/upgrade.sh | sudo bash
```

```bash
# Full uninstall (destructive — deletes Docklift data)
curl -fsSL "https://raw.githubusercontent.com/SSujitX/docklift/master/uninstall.sh" | sudo bash -s -- -y
```

```bash
# Development build from master (unreleased)
curl -fsSL https://raw.githubusercontent.com/SSujitX/docklift/master/install-dev.sh | sudo bash
```

## 3. Logs (all services)

### Platform containers

```bash
# Compose (all services, live)
docker compose logs -f
docker compose logs -f --tail 100 backend
docker compose logs -f --tail 100 frontend
docker compose logs -f --tail 100 nginx
docker compose logs -f --tail 100 nginx-proxy

# By container name
docker logs docklift-backend -f --tail 100
docker logs docklift-frontend -f --tail 100
docker logs docklift-nginx -f --tail 100          # dashboard gateway (:8080)
docker logs docklift-nginx-proxy -f --tail 100    # custom domains (:80/:443)
docker logs docklift-certbot -f --tail 50         # Let's Encrypt renewals
```

### Deployed project apps

```bash
docker ps --filter name=dl_
docker logs dl_python-smoke_53b01966_app -f --tail 200
```

### In the UI

Dashboard → **Logs** → Backend · Frontend · Nginx Proxy · Nginx (SSE live stream).

## 4. Nginx (gateway + app proxy)

| Container | Role | Host port | Config |
|-----------|------|-----------|--------|
| `docklift-nginx` | Dashboard gateway (`/`, `/api`, `/ws`) | **8080** | repo `nginx.conf` |
| `docklift-nginx-proxy` | User app domains + HTTPS | **80**, **443** | `nginx-proxy/conf.d/*.conf` |
| `docklift-certbot` | Let's Encrypt ACME | — | `nginx-proxy/certbot/` |

```bash
# Test config then reload (gateway)
docker exec docklift-nginx nginx -t
docker exec docklift-nginx nginx -s reload

# Test config then reload (app domain proxy)
docker exec docklift-nginx-proxy nginx -t
docker exec docklift-nginx-proxy nginx -s reload

# List generated domain configs
ls -la /opt/docklift/nginx-proxy/conf.d/
```

Edit gateway routing: change `nginx.conf`, then:

```bash
docker compose up -d --force-recreate nginx
# or: docker exec docklift-nginx nginx -s reload  (if only conf mounted)
```

## 5. Reset admin password

### Production (Docker)

```bash
docker exec -it docklift-backend node dist/scripts/reset-password.js
```

Prints a new random password for the admin user. Change it after login.

### Local development

```bash
cd backend
bun run reset-password
```

## 6. Version (check & release)

### See current version

```bash
grep '"version"' backend/package.json frontend/package.json
```

```bash
curl -s http://SERVER_IP:8080/api/health
# or from the server:
curl -s http://127.0.0.1:8080/api/health
```

UI also shows the version in the header / footer (authenticated compare).

### How versions are changed (do not hand-edit)

Versioning is **automatic** via [semantic-release](https://github.com/semantic-release/semantic-release).
Do **not** use `npm version`, `bumpp`, or manual bumps unless you know you are breaking the pipeline.

```bash
# Assume current version is 1.3.21 (root + frontend + backend stay in sync)

# 1) Commit with conventional messages on master
git commit -m "fix(deploy): description"       # → patch  → 1.3.22
git commit -m "feat(api): description"          # → patch  → 1.3.22
git commit -m "chore: cleanup"                  # → patch  → 1.3.22
git commit -m "feat: something *force minor*"   # → minor  → 1.4.0
git commit -m "feat: something *force major*"   # → major  → 2.0.0
git commit -m "chore: docs *skip release*"      # → none   → stays 1.3.21

# 2) Push — wait for CI + Install (Ubuntu) to go green
# 3) GitHub → Actions → "Release" → Run workflow
# semantic-release bumps package.json (root/frontend/backend), CHANGELOG.md, tag + GitHub Release
# Release then deploys Docs (changelog on docklift.dev). Do not edit CHANGELOG.md by hand.
```

| Commit signal | Release | Demo (`1.3.21` →) |
|---------------|---------|-------------------|
| `feat:`, `fix:`, `perf:`, `refactor:`, `docs:`, `test:`, `ci:`, `chore:` … | Patch | `1.3.22` |
| `*force minor*` in subject | Minor | `1.4.0` |
| `*force major*` / `BREAKING CHANGE` | Major | `2.0.0` |
| `*skip release*` in subject | None | `1.3.21` (unchanged) |

SemVer reminder: **patch** = bug fix / small change, **minor** = new feature (forced here), **major** = breaking change.

Config: [`release.config.cjs`](https://github.com/SSujitX/docklift/blob/master/release.config.cjs) · Workflow: [`.github/workflows/release.yml`](https://github.com/SSujitX/docklift/blob/master/.github/workflows/release.yml)

### After a release on a server

```bash
curl -fsSL https://raw.githubusercontent.com/SSujitX/docklift/master/upgrade.sh | sudo bash
```

## 7. Projects & containers

```bash
docker ps -a --filter name=docklift --filter name=dl_
docker inspect dl_python-smoke_53b01966_app
docker exec -it docklift-backend sh
docker exec -it dl_python-smoke_53b01966_app sh
```

Naming:

- Compose project / image prefix: `dl-<slug>-<8charId>`
- Container: `dl_<slug>_<8charId>_<service>`

## 8. Network & ports

```bash
docker network inspect docklift_network
docker network ls --filter label=com.docklift.managed=true

sudo ss -tulpn | grep -E ':(80|443|8080|550[0-9]|8000)\b'
sudo fuser -k 5500/tcp
```

| Port | Use |
|------|-----|
| `8080` | Dashboard (gateway nginx) |
| `80` | Custom domains HTTP + ACME |
| `443` | Custom domains HTTPS |
| `5500`–`5600` | Published app ports (opt-in pool) |
| `8000` | Backend (internal / local dev) |
| `3600` | Vite frontend (local `bun run dev`) |

Compose env defaults:

```bash
# PORT_RANGE_START=5500
# PORT_RANGE_END=5600
# DASHBOARD_BIND=0.0.0.0
```

## 9. Clean / reset

```bash
# Stop stack (keeps volumes/data dirs)
docker compose down

# Stop + remove containers/network (data dirs on disk remain)
docker compose down --remove-orphans

# Free stuck app port pool
for port in {5500..5600}; do sudo fuser -k ${port}/tcp 2>/dev/null; done
```

Nuclear remove: see uninstall one-liner in §2.

Volumes labelled for a project:

```bash
docker volume ls --filter label=com.docklift.project
```

## 10. Local development

```bash
cd backend
cp .env.local.example .env.local
bun install
bun run db:generate
bun run db:push
bun run dev            # http://localhost:8000
```

```bash
cd frontend
bun install
bun run dev            # http://127.0.0.1:3600
bun run build          # production → dist/
```

```bash
cd backend
bun run db:studio
bun run db:push
bun run db:generate
```

```bash
curl -s http://127.0.0.1:8000/api/health
curl -s http://127.0.0.1:3600/api/auth/status   # via Vite proxy
```

### Database scripts (from `backend/`)

| Command | Description |
|---------|-------------|
| `bun run db:studio` | Prisma Studio GUI |
| `bun run db:migrate` | Apply checked-in Prisma migrations |
| `bun run db:ensure` | Production DB bootstrap (dedupe + migrate + repair) |
| `bun run db:generate` | Regenerate Prisma client |
| `bun run db:push` | Local-only schema sync (not used on container boot) |

## Quick cheat sheet

| Task | Command |
|------|---------|
| Start stack | `docker compose up -d --build` |
| Upgrade server | `curl -fsSL …/upgrade.sh \| sudo bash` |
| Backend logs | `docker logs docklift-backend -f` |
| Frontend logs | `docker logs docklift-frontend -f` |
| Gateway nginx logs | `docker logs docklift-nginx -f` |
| Domain proxy logs | `docker logs docklift-nginx-proxy -f` |
| Reload domain nginx | `docker exec docklift-nginx-proxy nginx -s reload` |
| Reset password | `docker exec -it docklift-backend node dist/scripts/reset-password.js` |
| Check version file | `grep version backend/package.json` |
| Health | `curl -s http://SERVER_IP:8080/api/health` |
| List apps | `docker ps --filter name=dl_` |
| Ship a release | green CI + Install → Actions → **Release** |
