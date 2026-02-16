---
name: Build & CI Pipeline
description: Guide to Dockerfiles, Docker Compose, and the production build pipeline.
---

# Build & CI Pipeline Guide

Docklift uses a multi-container Docker Compose setup for production, with separate Dockerfiles for backend and frontend.

## Docker Compose (`docker-compose.yml`)

### Services (4 containers)

| Service | Image | Port | Purpose |
|---------|-------|------|---------|
| `docklift-backend` | Custom (./backend) | 8000 (internal) | Express API server |
| `docklift-frontend` | Custom (./frontend) | 3000 (internal) | Next.js standalone |
| `docklift-nginx` | `nginx:stable-alpine` | 8080:80 | Dashboard gateway |
| `docklift-nginx-proxy` | `nginx:stable-alpine` | 80:80 | Domain proxy for user apps |

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

Uses a 3-stage build for optimal image size:

```dockerfile
# Stage 1: Install deps with Bun (fast)
FROM oven/bun:1-alpine AS deps
RUN bun install --frozen-lockfile

# Stage 2: Build with Node.js (stable on all CPUs)
FROM node:22-alpine AS builder
RUN npm run build

# Stage 3: Production runtime
FROM node:22-alpine AS runner
CMD ["node", "server.js"]
```

> **CRITICAL**: The build stage uses **Node.js, NOT Bun**.
> Bun crashes with `SIGILL (Illegal instruction)` on many VPS CPUs (especially those without
> AVX512 support). Bun is only used for fast dependency installation.

### Next.js Standalone Output

The frontend uses Next.js `standalone` output mode to minimize image size:
- `output: "standalone"` in `next.config.ts`
- Copies only `.next/standalone` and `.next/static` to the runner
- Final image is ~100MB instead of ~500MB

## Backend Dockerfile (`backend/Dockerfile`)

```dockerfile
FROM node:22-alpine
RUN npm ci --omit=dev
RUN npx prisma generate
RUN npm run build
CMD ["node", "dist/index.js"]
```

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
docker compose up -d --build docklift-frontend

# View build logs
docker compose logs -f docklift-frontend
```

### Type Checking
```bash
# Frontend (Next.js build includes TypeScript check)
cd frontend && npx next build

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
