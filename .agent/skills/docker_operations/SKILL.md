---
name: Docker Operations
description: Guide for managing and debugging Docklift containers and deployments.
---

# Docker Operations Guide

Docklift is a container management platform, so understanding the underlying Docker operations is crucial.

## Core Containers

The Docklift platform consists of exactly **5 containers** (defined in `docker-compose.yml`):

| Container | Host port | Purpose |
|-----------|-----------|---------|
| `docklift-backend` | — (8000 internal) | Express API server |
| `docklift-frontend` | — (3000 internal) | Vite SPA (static nginx) |
| `docklift-nginx` | `${DASHBOARD_BIND:-0.0.0.0}:8080:80` | Dashboard gateway |
| `docklift-nginx-proxy` | `80:80`, `443:443` | Public domains for user apps + panel domain |
| `docklift-certbot` | — | Let's Encrypt issuance + 12-hourly renewal loop |

> **Note**: There is NO `docklift-db` container. SQLite is a file (`/app/data/docklift.db`) inside
> the backend container.

When you add or rename a core container, update the `CORE_CONTAINERS` and
`LOG_SERVICE_CONTAINERS` lists in `backend/src/routes/system.ts`, the Logs page service list in
`frontend/src/pages/Logs.tsx`, and `uninstall.sh`.

## Viewing Logs

### CLI
```bash
docker logs docklift-backend -f --tail 100
docker logs docklift-frontend -f --tail 100
docker logs docklift-nginx -f --tail 100          # dashboard gateway (:8080)
docker logs docklift-nginx-proxy -f --tail 100    # public proxy (:80/:443)
docker logs docklift-certbot -f --tail 100        # certificate issuance/renewal
```

### UI
Navigate to `/logs` in the dashboard for real-time SSE-streamed logs with timestamps, ANSI/level
colouring, search, follow-mode and download. See the `logging_monitoring` skill.

## Inspecting Deployed User Projects

User containers are named `dl_<slug>_<shortId>_<service>` (see `backend/src/lib/naming.ts`).

```bash
# List all user containers
docker ps --filter "name=dl_"
# Or by ownership label
docker ps --filter "label=com.docklift.managed=true"

# View logs for a specific user container
docker logs dl_python-smoke_53b01966_app -f

# Everything Compose knows about one project
docker compose -p dl-python-smoke-53b01966 ps

# Project network
docker network inspect dl-net-53b01966
```

## Debugging Deployments

1. **Check build logs**: in the UI, under the project's Deployments tab.
2. **Inspect container**: `docker inspect <container>`
3. **View logs**: `docker logs <container>`
4. **Enter shell**: `docker exec -it <container> /bin/sh`
5. **Read generated runtime state**: `deployments/.docklift/<projectId>/compose.yml` — this, not any
   repository compose file, is what actually ran.
6. **Confirm proxy attachment**: `docklift-nginx-proxy` should appear on `dl-net-<shortId>` after deploy.

## Networks

| Network | Who | Purpose |
|---------|-----|---------|
| `docklift_network` | Core DockLift services only | Control plane |
| `dl-net-<shortId>` | One project's containers + edge proxy | App isolation |

```bash
docker network inspect docklift_network
docker network ls --filter label=com.docklift.managed=true
```

Do **not** put user apps back on `docklift_network` by default. Do **not** bind apps to `127.0.0.1`
as a substitute for isolation without redesigning proxy routing.

## Volume Management

Bind mounts used by the backend:

| Host Path | Container Path | Purpose |
|-----------|---------------|---------|
| `./data` | `/app/data` | SQLite DB + uploads |
| `./deployments` | `/deployments` | Project source + generated runtime state |
| `./nginx-proxy/conf.d` | `/nginx-conf` | Generated vhosts |
| `./nginx-proxy/certbot/conf` | `/etc/letsencrypt` | Certificates |
| `./backups` | `/data/backups` | Backups |

Named volumes for project data are created per configured mount as `dl-<shortId>-<slug>` and
labelled `com.docklift.project=<projectId>`:

```bash
docker volume ls --filter label=com.docklift.project
docker volume ls --filter label=com.docklift.project=<projectId>
```

These are **external** volumes from Compose's point of view, so `docker compose down` will not
delete them. They are removed when the project is deleted.

## Pruning

```bash
# Host-wide and indiscriminate — NEVER from DockLift product paths
docker system prune -a
```

Product paths may prune **only** `docklift-<project8>-*` app tags (keep-2 after success) and
BuildKit cache (`builder prune -f` auto; `-af` on System purge). Never `docker system prune`
and never delete foreign / managed DB upstream images from the panel.

Helpers: `backend/src/lib/imageCleanup.ts`.

For full platform removal use `uninstall.sh`, which targets DockLift-owned resources
(`com.docklift.*` labels / `dl-net-*` / core names).
