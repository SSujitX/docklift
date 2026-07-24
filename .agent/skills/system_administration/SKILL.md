---
name: System Administration
description: Guide for server management, system APIs, backups, and maintenance operations.
---

# System Administration Guide

Docklift includes built-in system management features accessible through the UI and API.

## System Dashboard (`/system`)

The system page shows real-time server health metrics:
- **CPU**: Usage percentage, model, core count, temperature
- **Memory**: Used/total/percentage (reads host `/proc/meminfo` for accuracy)
- **GPU**: Model, VRAM, temperature, utilization (if available)
- **Disk**: Mount points, used/total/percentage
- **Network**: Bytes sent/received, speeds
- **Processes**: Top 10 by CPU (uses `nsenter` to read host processes)
- **Server Info**: Hostname, distro, kernel, uptime, public IP, location

### API Endpoints
| API | Purpose |
|-----|---------|
| `GET /api/system/stats` | Full system metrics (3s cache) |
| `GET /api/system/quick` | CPU + memory only (for header widget) |
| `GET /api/system/ip` | Server's public IP (5-min cache) |

## Maintenance Operations

### Purge Resources
**API**: `POST /api/system/purge`

Single endpoint that performs a comprehensive cleanup sequence:
1. **Docker cleanup**: `docker system prune -af` (removes unused images/networks, NOT volumes)
2. **Container restart**: Restarts all user containers to free memory, skipping anything in
   `CORE_CONTAINERS`. A core container missing from that list would be restarted here as if it were
   a user workload.
3. **Swap clear**: Clears swap if ≥30% free RAM available (safety check)
4. **Host cache**: Clears page cache via `nsenter` (`echo 3 > /proc/sys/vm/drop_caches`)
5. **Journal logs**: Vacuums systemd journals to 3 days
6. **APT cache**: Clears package manager cache
7. **Temp files**: Removes `/tmp` files older than 7 days

Returns before/after memory usage for comparison.

### Server Control

| API | Purpose | Notes |
|-----|---------|-------|
| `POST /api/system/reboot` | Reboot the host server | Uses `reboot -f`, simulated on Windows/Mac |
| `POST /api/system/reset` | Restart all Docklift containers | `docker restart` on `CORE_CONTAINERS` (all 5) |
| `POST /api/system/update-system` | Run `apt update && upgrade` on host | Via `nsenter`, 15-min timeout |
| `POST /api/system/upgrade` | Run Docklift upgrade script | Executes `/opt/docklift/upgrade.sh` on host |

## Interactive Web Terminal

**Route**: `/terminal`
**WebSocket**: `ws://host:8000/ws/terminal` (proxied via Nginx `/ws/`)

A full-featured xterm.js-based interactive terminal providing direct root access to the host.

### Architecture
- **Frontend**: xterm.js + WebSocket
- **Backend**: `ws` server + `child_process.spawn('script', ...)`
- **PTY**: Uses Linux `script` command for TTY emulation (zero native dependencies)
- **Container**: Runs inside `docklift-backend` (Alpine) but has host access via Docker privileged mode & PID host.

### Features
- **Real-time PTY**: Supports tab completion, history, colors, ncurses (htop/nano).
- **Root Access**: Session starts in `/root` with full host privileges.
- **Resizing**: Bi-directional resize sync between frontend/backend. Resize inputs are validated (cols: 1–500, rows: 1–200) to prevent injection.
- **Persistence**: Auto-reconnect on network drops.
- **Security**:
  - **Double Authentication**: JWT (connect) + Password (interactive).
  - **Rate Limiting**: Max 5 logins/minute.
  - **Session Limits**: Max 3 concurrent connections per user.
  - **Idle Timeout**: Auto-disconnect after 30 minutes of inactivity.

### Graceful Shutdown

The backend handles SIGTERM/SIGINT signals for clean exit:
- Stops accepting new HTTP connections.
- Cleans up all active terminal PTY sessions via `cleanupAllSessions()`.
- Disconnects Prisma database client.
- Applied in: `index.ts`.

## System Logs

**API**: `GET /api/system/logs/:service` (SSE stream)

Mapping lives in `LOG_SERVICE_CONTAINERS` (`backend/src/routes/system.ts`):

| Service | Container | UI label |
|---------|-----------|----------|
| `backend` | `docklift-backend` | Backend |
| `frontend` | `docklift-frontend` | Frontend |
| `nginx` | `docklift-nginx` | Dashboard Gateway (:8080) |
| `proxy` | `docklift-nginx-proxy` | Public Proxy (:80/:443) |
| `certbot` | `docklift-certbot` | Certbot |

## Version Check

**API**: `GET /api/system/version`
- Compares local `package.json` version against latest GitHub release
- 1-hour cache
- Returns `{ current, latest, updateAvailable }`

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
2. **Recreates persistent volumes** — Each `PersistentVolume` row is re-created as an external
   labelled Docker volume before the project starts, so mounts resolve
3. **Brings every project back up** — `docker compose -f <runtime-compose> -p <composeProject> up -d`,
   where the compose file is `deployments/.docklift/<projectId>/compose.yml` and the project name comes
   from `composeProjectName()`. Older backups without generated runtime state fall back to a
   `docker-compose.yml` at the source root; if neither exists the project is skipped and must be
   deployed manually
4. **Reloads Nginx proxy** — `docker exec docklift-nginx-proxy nginx -s reload`
5. **Self-restarts backend** — `process.exit(0)` triggers Docker's `restart: unless-stopped` policy

> The `-p` name must match deploy-time naming, or restore creates a *second* set of containers and
> images alongside the originals.

### What's Backed Up

| Item | Path | Description |
|------|------|-------------|
| Database | `/app/data/docklift.db` | SQLite database |
| Deployments | `/deployments/` | Project source + generated runtime compose |
| Nginx configs | `/nginx-conf/` | Generated proxy configurations |
| Certificates | `/etc/letsencrypt/` | Let's Encrypt certs (backend mounts this RW for restore) |
| GitHub key | `github-app.pem` | GitHub App private key |

Named volume *contents* are not archived — the volumes are re-created empty if missing. Snapshot
application data separately if it matters.

## Install / Upgrade / Uninstall Scripts

| Script | Purpose |
|--------|---------|
| `install.sh` | Production install of the latest release into `/opt/docklift` |
| `install-dev.sh` | Same, but from `master` (unreleased code) |
| `upgrade.sh` | Pull new images and recreate the stack, preserving `data/`, `deployments/`, certs |
| `uninstall.sh` | Remove DockLift containers, images, volumes, network, build cache and `/opt/docklift` |

`uninstall.sh` targets **DockLift-owned resources only** (core container names + the
`com.docklift.project` volume label). It must never run a host-wide `docker system prune` or remove
Docker Engine/git — other workloads on the same server have to survive an uninstall.

## Server Access Requirements

The backend container needs these host-level permissions (defined in `docker-compose.yml`):
- `privileged: true` — required for `nsenter` into host PID 1 (system update, reboot, cache drop)
- `pid: host` — For host process visibility (reboot, system info)
- Docker socket mount: `/var/run/docker.sock`
- Host file mounts: `/etc/hostname`, `/etc/os-release`, `/proc` (read-only)
- `./nginx-proxy/certbot/conf:/etc/letsencrypt` — read-write, so restores can put certificates back
  (the proxy mounts the same path read-only)
