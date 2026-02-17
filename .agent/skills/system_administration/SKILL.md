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
2. **Container restart**: Restarts all user containers (excludes Docklift containers) to free memory
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
| `POST /api/system/reset` | Restart all Docklift containers | `docker restart` on the 4 core containers |
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
- **Root Access**: Sesssion starts in `/root` with full host privileges.
- **Resizing**: Bi-directional resize sync between frontend/backend.
- **Persistence**: Auto-reconnect on network drops.
- **Security**:
  - **Double Authentication**: JWT (connect) + Password (interactive).
  - **Rate Limiting**: Max 5 logins/minute.
  - **Session Limits**: Max 3 concurrent connections per user.
  - **Idle Timeout**: Auto-disconnect after 15 minutes of inactivity.

### Command Execution (Legacy One-Shot)

**API**: `POST /api/system/execute`
- Executes arbitrary shell commands on the host via `nsenter`
- **Requires password re-verification** (user must send current password)
- Has a 30-second timeout
- Audit-logged with IP address

## System Logs

**API**: `GET /api/system/logs/:service` (SSE stream)

| Service | Container |
|---------|-----------|
| `backend` | `docklift-backend` |
| `frontend` | `docklift-frontend` |
| `proxy` | `docklift-nginx-proxy` |
| `nginx` | `docklift-nginx` |

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
2. **Auto-redeploys all projects** — Runs `docker compose -p <projectId> up -d --build` for each
3. **Reloads Nginx proxy** — `docker exec docklift-nginx-proxy nginx -s reload`
4. **Self-restarts backend** — `process.exit(0)` triggers Docker's `restart: unless-stopped` policy

### What's Backed Up

| Item | Path | Description |
|------|------|-------------|
| Database | `/app/data/docklift.db` | SQLite database |
| Deployments | `/deployments/` | All project source code and configs |
| Nginx configs | `/nginx-conf/` | Generated proxy configurations |
| GitHub key | `github-app.pem` | GitHub App private key |

## Server Access Requirements

The backend container needs these host-level permissions (defined in `docker-compose.yml`):
- `privileged: true` — For Docker-in-Docker operations
- `pid: host` — For host process visibility (reboot, system info)
- Docker socket mount: `/var/run/docker.sock`
- Host file mounts: `/etc/hostname`, `/etc/os-release`, `/proc` (read-only)
