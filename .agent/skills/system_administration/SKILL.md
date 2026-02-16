---
name: System Administration
description: Guide for server management, system APIs, backups, and maintenance operations.
---

# System Administration Guide

Docklift includes built-in system management features accessible through the UI and API.

## System Dashboard (`/system`)

The system page shows real-time server health metrics:
- **CPU**: Usage percentage, model, core count
- **Memory**: Used/total/percentage
- **Disk**: Used/total/percentage
- **Uptime**: System uptime
- **OS Info**: Distribution, kernel version, hostname

### API: `GET /api/system/stats`
Returns system metrics using the `systeminformation` library.

## Maintenance Operations

### Purge Resources

| API | Purpose |
|-----|---------|
| `POST /api/system/purge/containers` | Remove stopped containers |
| `POST /api/system/purge/images` | Remove unused images |
| `POST /api/system/purge/volumes` | Remove unused volumes |
| `POST /api/system/purge/networks` | Remove unused networks |
| `POST /api/system/purge/all` | Remove all unused Docker resources |
| `POST /api/system/purge/build-cache` | Clear Docker build cache |

### Reboot

- **API**: `POST /api/system/reboot`
- Executes `reboot` command on the host (requires privileged mode)
- The backend container runs with `privileged: true` and `pid: host`

## Backup & Restore System

All backup/restore routes are in `backend/src/routes/backup.ts`, mounted at `/api/backup`.

### Backup

| API | Purpose |
|-----|---------|
| `POST /api/backup/create` | Create a full backup (DB, deployments, Nginx configs, GitHub key) |
| `GET /api/backup/list` | List available backups |
| `GET /api/backup/download/:filename` | Download a backup file |
| `DELETE /api/backup/:filename` | Delete a backup |

### Restore

| API | Purpose |
|-----|---------|
| `POST /api/backup/restore/:filename` | Restore from a server-side backup |
| `POST /api/backup/restore-upload` | Upload and immediately restore |
| `POST /api/backup/restore-from-upload/:filename` | Restore from a previously uploaded file |

### Auto-Restore (reconcileSystem)

After restoring files, the system **automatically**:

1. **Reads restored database** — Creates a fresh `PrismaClient` to read the restored DB
2. **Auto-redeploys all projects** — Runs `docker compose -p <projectId> up -d --build` for each
3. **Reloads Nginx proxy** — `docker exec docklift-nginx-proxy nginx -s reload`
4. **Self-restarts backend** — `process.exit(0)` triggers Docker's `restart: unless-stopped` policy

> This eliminates the need for manual redeployment after a restore.

### What's Backed Up

| Item | Path | Description |
|------|------|-------------|
| Database | `/app/data/docklift.db` | SQLite database |
| Deployments | `/deployments/` | All project source code and configs |
| Nginx configs | `/nginx-conf/` | Generated proxy configurations |
| GitHub key | `github-app.pem` | GitHub App private key |

## Ports Management (`/ports`)

Shows all ports in use by Docker containers:
- **API**: `GET /api/system/ports`
- Displays: container name, internal port, external port, protocol

## Settings (`/settings`)

- **GitHub App**: Connection status, app credentials
- **Domain Config**: Server IP, wildcard domain
- **Security**: JWT secret rotation, API secret management
- **Backup & Restore**: Create, upload, download, and restore backups

## Server Access Requirements

The backend container needs these host-level permissions (defined in `docker-compose.yml`):
- `privileged: true` — For Docker-in-Docker operations
- `pid: host` — For host process visibility (reboot, system info)
- Docker socket mount: `/var/run/docker.sock`
- Host file mounts: `/etc/hostname`, `/etc/os-release`, `/proc` (read-only)
