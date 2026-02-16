---
name: Docker Operations
description: Guide for managing and debugging Docklift containers and deployments.
---

# Docker Operations Guide

Docklift is a container management platform, so understanding the underlying Docker operations is crucial.

## Core Containers

The main Docklift platform consists of exactly **4 containers** (defined in `docker-compose.yml`):

| Container | Port | Purpose |
|-----------|------|---------|
| `docklift-backend` | 8000 (internal) | Express API server |
| `docklift-frontend` | 3000 (internal) | Next.js dashboard |
| `docklift-nginx` | 8080:80 | Dashboard gateway (routes to frontend + backend) |
| `docklift-nginx-proxy` | 80:80 | Custom domain proxy for user-deployed apps |

> **Note**: There is NO `docklift-db` container. SQLite is a file (`/app/data/docklift.db`) inside the backend container.

## Viewing Logs

### CLI
```bash
docker logs docklift-backend -f --tail 100
docker logs docklift-frontend -f --tail 100
docker logs docklift-nginx -f --tail 100
docker logs docklift-nginx-proxy -f --tail 100
```

### UI
Navigate to `/logs` in the Docklift dashboard. This provides real-time SSE-streamed logs with:
- Timestamps per line
- Color-coded output (errors=red, warnings=amber, success=green)
- Search functionality
- Download capability

## Inspecting Deployed User Projects

User deployments follow the naming convention: `dl_<shortId>_<serviceName>`.

```bash
# List all user containers
docker ps --filter "name=dl_"

# View logs for a specific user container
docker logs dl_2d165805_app -f
```

## Debugging Deployments

1. **Check Build Logs**: In the UI, check the deployment logs under the project's Deployments tab.
2. **Inspect Container**:
    ```bash
    docker inspect <container_name_or_id>
    ```
3. **View Container Logs**:
    ```bash
    docker logs <container_name_or_id>
    ```
4. **Enter Container Shell**:
    ```bash
    docker exec -it <container_name_or_id> /bin/sh
    ```

## Network

All Docklift containers + user deployments are on the `docklift_network` bridge network.

```bash
# Inspect the network
docker network inspect docklift_network
```

## Volume Management

| Host Path | Container Path | Used By |
|-----------|---------------|---------|
| `./data` | `/app/data` | Backend (SQLite DB) |
| `./deployments` | `/deployments` | Backend (project files) |
| `./nginx-proxy/conf.d` | `/nginx-conf` | Backend (generates proxy configs) |
| `./backups` | `/data/backups` | Backend (DB backups) |

## Pruning

```bash
# Clean up unused resources (careful in production!)
docker system prune -a
```

Docklift also has built-in purge APIs at `/api/system/purge/*`.
