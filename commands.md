# Docklift Management Commands

End-to-end reference for install, day-to-day ops, logs, nginx, password reset, and versioning.

**Compose services** → `backend` · `frontend` · `nginx` · `nginx-proxy` · `certbot`  
**Container names** → `docklift-backend` · `docklift-frontend` · `docklift-nginx` · `docklift-nginx-proxy` · `docklift-certbot`  
**App containers** → `dl_<project-slug>_<id>_<service>` (e.g. `dl_python-smoke_53b01966_app`)  
**Dashboard** → `http://SERVER_IP:8080` · **App publish ports** → `5500–5600`  
**Fresh setup** → copy **bootstrap secret** from `docker logs docklift-backend` (or `data/.bootstrap-secret`) into the Setup page

---

## Table of contents

1. [Install / start / stop](#1-install--start--stop)
2. [Upgrade & uninstall](#2-upgrade--uninstall)
3. [Logs (all services)](#3-logs-all-services)
4. [Nginx (gateway + app proxy)](#4-nginx-gateway--app-proxy)
5. [Reset admin password](#5-reset-admin-password)
6. [Version (check & release)](#6-version-check--release)
7. [Projects & containers](#7-projects--containers)
8. [Network & ports](#8-network--ports)
9. [Clean / reset](#9-clean--reset)
10. [Local development](#10-local-development)

---

## 1. Install / start / stop

```bash
# Fresh install (server)
curl -fsSL https://raw.githubusercontent.com/SSujitX/docklift/master/install.sh | sudo bash

# From a cloned repo
cd /opt/docklift   # or your clone path
docker compose up -d --build

# Dashboard on :8080 (default all interfaces → http://SERVER_IP:8080).
# Optional lockdown: DASHBOARD_BIND=127.0.0.1 docker compose up -d

# First boot: installer prints Dashboard URL + Setup code.
# Or: docker logs docklift-backend 2>&1 | grep -A8 'Fresh install'
# Or: cat data/.bootstrap-secret

# Start / stop / restart (all)
docker compose start
docker compose stop
docker compose restart

# Rebuild one service (Compose service keys, not container names)
docker compose up -d --build backend
docker compose up -d --build frontend
docker compose up -d --build nginx
docker compose up -d --build nginx-proxy

# Status
docker compose ps
docker ps --filter name=docklift --filter name=dl_
```

---

## 2. Upgrade & uninstall

```bash
# Safe upgrade (keeps data + user app containers)
curl -fsSL https://raw.githubusercontent.com/SSujitX/docklift/master/upgrade.sh | sudo bash

# Full uninstall (destructive — deletes Docklift data)
curl -fsSL "https://raw.githubusercontent.com/SSujitX/docklift/master/uninstall.sh?nocache=5" | sudo bash -s -- -y
```

---

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
# List app containers
docker ps --filter name=dl_

# Logs for one app (replace with your container name)
docker logs dl_python-smoke_53b01966_app -f --tail 200
```

### In the UI

Dashboard → **Logs** → tabs: Backend · Frontend · Nginx Proxy · Nginx (SSE live stream).

---

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
ls -la nginx-proxy/conf.d/
# or on server:
ls -la /opt/docklift/nginx-proxy/conf.d/

# Follow access/error style output (container logs)
docker logs docklift-nginx -f --tail 100
docker logs docklift-nginx-proxy -f --tail 100
```

Edit gateway routing: change `nginx.conf`, then:

```bash
docker compose up -d --force-recreate nginx
# or: docker exec docklift-nginx nginx -s reload  (if only conf mounted)
```

---

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

---

## 6. Version (check & release)

### See current version

```bash
# From package.json (source of truth in image/repo)
grep '"version"' backend/package.json frontend/package.json

# Running API
curl -s http://SERVER_IP:8080/api/health
# or from the server: curl -s http://127.0.0.1:8080/api/health
# Version compare is in the UI (authenticated)
```

UI also shows the version in the header / footer.

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

# 2) Push, then GitHub → Actions → "Release & Test" → Run workflow
# semantic-release bumps package.json (root/frontend/backend), CHANGELOG.md, tag + GitHub Release
# You do NOT edit CHANGELOG.md by hand — it is generated from these commits.
```

| Commit signal | Release | Demo (`1.3.21` →) |
|---------------|---------|-------------------|
| `feat:`, `fix:`, `perf:`, `refactor:`, `chore:` | Patch | `1.3.22` |
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

# Shell into backend
docker exec -it docklift-backend sh

# Shell into an app container
docker exec -it dl_python-smoke_53b01966_app sh
```

Naming pattern for apps:

- Compose project / image prefix: `dl-<slug>-<8charId>`
- Container: `dl_<slug>_<8charId>_<service>`

---

## 8. Network & ports

```bash
# Docklift bridge network
docker network inspect docklift_network

# What is listening (Linux)
sudo ss -tulpn | grep -E ':(80|8080|550[0-9]|8000)\b'
# or
sudo netstat -tulpn | grep -E ':(80|8080|5500)'

# Kill a stuck host port (example)
sudo fuser -k 5500/tcp

# App port pool (compose env; default avoids Windows Hyper-V 3000–5000)
# PORT_RANGE_START=5500
# PORT_RANGE_END=5600
```

| Port | Use |
|------|-----|
| `8080` | Dashboard (gateway nginx) |
| `80` | Custom domains HTTP + ACME (nginx-proxy) |
| `443` | Custom domains HTTPS (Let's Encrypt) |
| `5500–5600` | Published app ports (default pool) |
| `8000` | Backend (internal / local dev) |
| `3600` | Vite frontend (local `bun run dev`) |

---

## 9. Clean / reset

```bash
# Stop stack (keeps volumes/data dirs)
docker compose down

# Stop + remove containers/network (data dirs on disk remain: ./data, ./deployments, …)
docker compose down --remove-orphans

# Nuclear uninstall script (see §2)

# Free a range of stuck TCP ports (Linux) — adjust to your PORT_RANGE
for port in {5500..5600}; do sudo fuser -k ${port}/tcp 2>/dev/null; done
```

---

## 10. Local development

```bash
# Backend — .env (server, committed) + .env.local (from .env.local.example, gitignored)
cd backend
cp .env.local.example .env.local
bun install
bun run db:generate
bun run db:push
bun run dev            # http://localhost:8000

# Frontend (other terminal)
cd frontend
bun install
bun run dev            # http://127.0.0.1:3600  (PORT in .env)
bun run build          # production → dist/

# Database
cd backend
bun run db:studio      # Prisma GUI
bun run db:push
bun run db:generate

# Dep updates (frontend or backend folder)
bun outdated
bun update
# or
bunx npm-check-updates -u && bun install
```

Health check while developing:

```bash
curl -s http://127.0.0.1:8000/api/health
curl -s http://127.0.0.1:3600/api/auth/status   # via Vite proxy
```

---

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
| Ship a release | conventional commit → Actions → **Release & Test** |
| List apps | `docker ps --filter name=dl_` |
