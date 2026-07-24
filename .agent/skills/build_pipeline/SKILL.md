---
name: Build & CI Pipeline
description: Guide to Dockerfiles, Docker Compose, and the production build pipeline.
---

# Build & CI Pipeline Guide

Docklift uses a multi-container Docker Compose setup for production, with separate Dockerfiles for backend and frontend.

## Docker Compose (`docker-compose.yml`)

### Services (4 containers)

| Compose service | Image | Container name | Port | Purpose |
|-----------------|-------|----------------|------|---------|
| `backend` | `docklift-backend` | `docklift-backend` | 8000 (internal) | Express API |
| `frontend` | `docklift-frontend` | `docklift-frontend` | 3000 (internal) | Vite SPA (nginx) |
| `nginx` | `nginx:stable-alpine` | `docklift-nginx` | 8080:80 | Dashboard gateway |
| `nginx-proxy` | `nginx:stable-alpine` | `docklift-nginx-proxy` | 80:80 | App domain proxy |

### Volume Mounts (Backend)

| Host Path | Container Path | Purpose |
|-----------|---------------|---------|
| `/var/run/docker.sock` | `/var/run/docker.sock` | Docker API access |
| `./data` | `/app/data` | SQLite database |
| `./deployments` | `/deployments` | Project source files |
| `./nginx-proxy/conf.d` | `/nginx-conf` | Generated Nginx configs |
| `./backups` | `/data/backups` | Database backups |

### Environment Variables

- `JWT_SECRET`: Auth token signing (auto-generated on first run if empty)
- `INTERNAL_API_SECRET`: Backend-to-backend auth
- `DATABASE_URL`: `file:/app/data/docklift.db`

## Frontend Dockerfile (`frontend/Dockerfile`)

3-stage: Bun install → Vite build → `nginx:stable-alpine` serving `dist/` on port **3000** (SPA `try_files`).

Browser calls stay same-origin behind `docklift-nginx` (`VITE_API_URL` empty at build time).

## Backend Dockerfile (`backend/Dockerfile`)

Uses a 4-stage build:

```dockerfile
# Stage 1: Install ALL deps with Bun (fast)
FROM oven/bun:1-alpine AS deps
RUN bun install --frozen-lockfile
RUN bunx prisma generate

# Stage 2: Install PRODUCTION deps only
FROM oven/bun:1-alpine AS prod-deps
RUN bun install --production --frozen-lockfile

# Stage 3: Build with Bun
FROM oven/bun:1-alpine AS builder
RUN bun run build

# Stage 4: Production runtime
FROM node:22-alpine AS runner
RUN apk add --no-cache docker-cli docker-cli-compose git procps
CMD ["sh", "-c", "npx prisma db push --skip-generate && node dist/index.js"]
```

> **Key details**:
> - Docker CLI + Compose are installed so the backend can manage containers
> - `prisma db push` runs on every startup to auto-apply schema migrations
> - Runtime is Node.js (not Bun) for stability
> - Runs as **root** (Docker socket requires it)

## Build Commands

### Local Development
```bash
# Frontend
cd frontend && npm run dev    # Dev server at :3000

# Backend
cd backend && npm run dev     # Dev server at :4000 (nodemon)
```

### Production (Docker)
```bash
# Build and start all containers
docker compose up -d --build

# Rebuild a single service
docker compose up -d --build frontend

# View build logs
docker compose logs -f frontend
```

### Type Checking
```bash
# Frontend (tsc + Vite)
cd frontend && bun run build

# Backend
cd backend && npm run build   # runs tsc
```

## Common Build Issues

| Error | Cause | Fix |
|-------|-------|-----|
| `SIGILL` / `Segmentation fault` in Bun | Server CPU lacks AVX instructions | Use Node.js for build stage (already fixed) |
| `Cannot use namespace 'X' as a type` | TypeScript incompatibility | Replace with `React.SVGProps<SVGSVGElement>` |
| Lockfile conflicts | Multiple lockfiles | Delete root `package-lock.json` or set `turbopack.root` |
| `standalone` missing files | Public folder empty | `RUN mkdir -p public` before build |

## Dev vs Production Architecture

```
Development:
  Frontend (:3000) → direct API calls → Backend (:4000)

Production:
  Browser → :8080 → docklift-nginx → Frontend (:3000) + Backend (:8000)
  User domains → :80 → docklift-nginx-proxy → user containers
```
