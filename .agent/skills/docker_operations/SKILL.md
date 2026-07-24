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
| `docklift-nginx` | `8080:80` | Dashboard gateway (routes to frontend + backend) |
| `docklift-nginx-proxy` | `80:80`, `443:443` | Public domains for user apps + panel domain |
| `docklift-certbot` | — | Let's Encrypt issuance + 12-hourly renewal loop |

> **Note**: There is NO `docklift-db` container. SQLite is a file (`/app/data/docklift.db`) inside
> the backend container.

When you add or rename a core container, update the `CORE_CONTAINERS` and
`LOG_SERVICE_CONTAINERS` lists in `backend/src/routes/system.ts`, the Logs page service list in
`frontend/src/pages/Logs.tsx`, and `uninstall.sh`. Missing it from `CORE_CONTAINERS` makes
`/api/system/purge` treat the container as a *user* workload and restart it.

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

# View logs for a specific user container
docker logs dl_python-smoke_53b01966_app -f

# Everything Compose knows about one project
docker compose -p dl-python-smoke-53b01966 ps
```

## Debugging Deployments

1. **Check build logs**: in the UI, under the project's Deployments tab.
2. **Inspect container**: `docker inspect <container>`
3. **View logs**: `docker logs <container>`
4. **Enter shell**: `docker exec -it <container> /bin/sh`
5. **Read generated runtime state**: `deployments/.docklift/<projectId>/compose.yml` — this, not any
   repository compose file, is what actually ran.

## Network

All Docklift containers and user deployments share the `docklift_network` bridge
(IPv4 `172.28.0.0/16`, IPv6 `fd12:3456:789a::/64`).

```bash
docker network inspect docklift_network
```

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
# All DockLift-managed app volumes
docker volume ls --filter label=com.docklift.project

# Volumes for one project
docker volume ls --filter label=com.docklift.project=<projectId>
```

These are **external** volumes from Compose's point of view, so `docker compose down` will not
delete them. They are removed when the project is deleted.

## Pruning

```bash
# Host-wide and indiscriminate — avoid on shared servers
docker system prune -a
```

Prefer the built-in `POST /api/system/purge`, which prunes images/networks, restarts only
*non-core* containers, and leaves volumes alone. For full removal use `uninstall.sh`, which targets
DockLift-labelled resources instead of pruning the whole host.
