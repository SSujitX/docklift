---
name: Build & CI Pipeline
description: Guide to Dockerfiles, Docker Compose, and the production build pipeline.
---

# Build & CI Pipeline Guide

Docklift ships as a multi-container Docker Compose stack, with separate Dockerfiles for backend and frontend.

Do not confuse the two build systems:

- **This guide** covers building *Docklift itself* (the platform images).
- **`deployment_system`** covers building *user projects* (Dockerfile / Railpack per deployment).

## Docker Compose (`docker-compose.yml`)

Compose project name: `docklift`.

### Services (5 containers)

| Compose service | Image | Container name | Host port | Purpose |
|-----------------|-------|----------------|-----------|---------|
| `backend` | `docklift-backend` | `docklift-backend` | — (expose 8000) | Express API |
| `frontend` | `docklift-frontend` | `docklift-frontend` | — (expose 3000) | Vite SPA (nginx) |
| `nginx` | `nginx:stable-alpine` | `docklift-nginx` | `${DASHBOARD_BIND:-0.0.0.0}:8080:80` | Dashboard gateway |
| `nginx-proxy` | `nginx:stable-alpine` | `docklift-nginx-proxy` | `80:80`, `443:443` | Project & panel domains |
| `certbot` | `certbot/certbot` | `docklift-certbot` | — | Let's Encrypt issue/renew loop |

`DASHBOARD_BIND` defaults to **`0.0.0.0`** so install can open `http://SERVER_IP:8080`. Operators may
set `127.0.0.1` for localhost-only. First account still requires the bootstrap setup code.

Control-plane network: `docklift_network` (bridge, IPv6 enabled). User apps use **per-project**
`dl-net-*` networks — see `networking_proxy` / `deployment_system`.

### Volume Mounts (Backend)

| Host Path | Container Path | Purpose |
|-----------|---------------|---------|
| `/var/run/docker.sock` | `/var/run/docker.sock` | Docker API access |
| `./data` | `/app/data` | SQLite database + uploads |
| `./deployments` | `/deployments` | Project source + generated runtime state |
| `./nginx-proxy/conf.d` | `/nginx-conf` | Generated vhosts |
| `./nginx-proxy/certbot/conf` | `/etc/letsencrypt` | Certificates (RW so backups can restore them) |
| `./backups` | `/data/backups` | Database/deployment backups |
| `/etc/hostname`, `/etc/os-release`, `/proc` | `/host/*` (ro) | Host metrics |

The backend also runs `privileged: true` and `pid: host` so it can `nsenter` into host PID 1
for host-level actions (system update, reboot, cache drop).

`nginx-proxy` mounts `conf.d`, `snippets`, `certbot/www` and `/etc/letsencrypt` **read-only** —
only the backend and certbot write there.

### Environment Variables

| Var | Purpose |
|-----|---------|
| `JWT_SECRET` | Auth token signing (auto-generated + persisted on first run if empty) |
| `INTERNAL_API_SECRET` | Backend-to-backend auth (webhook → deploy) |
| `DATABASE_URL` | `file:/app/data/docklift.db` |
| `PORT_RANGE_START` / `_END` | Host port pool when `publish_host_port` is enabled (default `5500`–`5600`) |
| `DASHBOARD_BIND` | Panel listen address (default `0.0.0.0`) |
| `CORS_ORIGIN` | Extra browser origins (comma-separated) when the panel is not same-origin |
| `DOCKLIFT_FRONTEND_URL` | Public dashboard URL used for GitHub App callbacks |
| `CERTBOT_EMAIL` / `CERTBOT_STAGING` | Let's Encrypt registration + staging toggle |

### Project app env (multi-service)

`EnvVariable.service_name`: `""` = shared to every service; otherwise the Docker service name (folder). Deploy merges shared + service-scoped (service keys win). UI: multi-service projects use a **Workspace** rail — **All services** owns Deploy / Build / Source / shared Env; a **service** workspace owns that service’s Overview, Env, Domains, Storage, and runtime Logs. Single-service projects keep flat tabs (no rail) with lifecycle actions under the title. **All-services actions** (Redeploy / Restart / Stop / Delete) always affect the whole compose stack — not inside Env/Domains/Storage/Logs tabs.

## Frontend Dockerfile (`frontend/Dockerfile`)

3-stage: Bun install → Vite build → `nginx:stable-alpine` serving `dist/` on port **3000** (SPA `try_files`).

Browser calls stay same-origin behind `docklift-nginx` (`VITE_API_URL` empty at build time).

## Backend Dockerfile (`backend/Dockerfile`)

4 stages, built on `oven/bun:1-alpine`, with a Node runtime:

```dockerfile
FROM base AS deps          # bun install --frozen-lockfile + prisma generate
FROM base AS prod-deps     # bun install --production
FROM base AS builder       # bun run build (tsc → dist/)
FROM node:24-alpine AS runner
RUN apk add --no-cache docker-cli docker-cli-buildx docker-cli-compose git procps bash util-linux
# + pinned Railpack binary (RAILPACK_VERSION, musl build, amd64/arm64)
CMD ["sh", "-c", "node dist/scripts/ensureDb.js && node dist/index.js"]
```

> **Key details**:
> - `docker-cli`, `docker-cli-compose` **and `docker-cli-buildx`** are all required. Railpack
>   builds go through `docker buildx build`, so a missing buildx plugin breaks every
>   Railpack deployment while Dockerfile deployments keep working.
> - Railpack is version-pinned in the Dockerfile (`ARG RAILPACK_VERSION`) and verified with
>   `railpack --version` at build time — bump it deliberately, never float it.
> - `util-linux` provides `nsenter`; `procps` gives accurate `ps`/`top` for host process listing.
> - **`ensureDb.js`**: dedupe env rows → `prisma migrate deploy` (checked-in migrations) → legacy repair.
>   Never boot with `db push --accept-data-loss`.
> - Runtime is Node.js, not Bun (Bun segfaults on CPUs without AVX).
> - Runs as **root** — the Docker socket requires it (the `docklift` user exists but is not used).

## CI (`.github/workflows/ci.yml`)

- Backend: frozen lockfile, pinned Bun, `tsc`, `bun test src`, `prisma validate` + migrations present
- Frontend: production build
- Compose config validate
- Regressions: `DASHBOARD_BIND` default `0.0.0.0`; proxy disconnect/setup-restore/secret preflight;
  no host image prune / `system prune -af` / default `cap_drop ALL` in product paths
- Backend `bun audit --prod` must be clean (CI-enforced via overrides for transitive deps)
- Frontend audit may still warn until `react-router-dom` ships a fixed 8.x (SPA does not use RSC mode)

## Build Commands

### Local Development
```bash
# Backend — tsx watch on :8000
cd backend && bun install && bun run db:generate && bun run db:push && bun run dev

# Frontend — Vite on :3600
cd frontend && bun install && bun run dev
```

### Production (Docker)
```bash
docker compose up -d --build              # build and start everything
docker compose up -d --build frontend     # rebuild one service
docker compose logs -f frontend           # follow build/run logs
```

### Type Checking
```bash
cd frontend && bun run build          # tsc -b + vite build
cd backend  && bun run build          # tsc
cd backend  && bun run test           # bun test src (all *.test.ts)
```

On Windows, `cd` does not persist between agent shell calls and `npx tsc` may not resolve.
Prefer the local binary in one command: `cd backend; .\node_modules\.bin\tsc --noEmit`.

## Common Build Issues

| Error | Cause | Fix |
|-------|-------|-----|
| `SIGILL` / `Segmentation fault` in Bun | Server CPU lacks AVX | Node.js is used for the runtime stage already |
| `docker buildx` not found during deploy | Backend image missing buildx plugin | Add `docker-cli-buildx` to the runner stage |
| `railpack: not found` | Railpack download failed for `TARGETARCH` | Check the pinned release publishes a musl binary for that arch |
| `bun install --frozen-lockfile` fails | `bun.lock` out of sync with `package.json` | Run `bun install` locally and commit the lockfile |
| Prisma client type errors after schema edit | Client not regenerated | `bun run db:generate` |

## Dev vs Production Architecture

```
Development:
  Browser → Vite (:3600) → direct API calls → Backend (:8000)

Production:
  Browser → SERVER_IP:8080 → docklift-nginx → Frontend (:3000) + Backend (:8000)
  Public domains → :80/:443 → docklift-nginx-proxy → (project network) → container_name:internal_port
```
