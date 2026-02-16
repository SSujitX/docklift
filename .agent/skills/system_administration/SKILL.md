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

## Backup System

### Database Backup
- **API**: `POST /api/system/backup`
- Creates a copy of `docklift.db` in `/data/backups/`
- Filename format: `docklift-backup-YYYY-MM-DD-HH-MM-SS.db`

### Restore
- **API**: `POST /api/system/restore`
- Accepts a backup file and replaces the current database

## Ports Management (`/ports`)

Shows all ports in use by Docker containers:
- **API**: `GET /api/system/ports`
- Displays: container name, internal port, external port, protocol

## Settings (`/settings`)

- **GitHub App**: Connection status, app credentials
- **Domain Config**: Server IP, wildcard domain
- **Security**: JWT secret rotation, API secret management

## Server Access Requirements

The backend container needs these host-level permissions (defined in `docker-compose.yml`):
- `privileged: true` — For Docker-in-Docker operations
- `pid: host` — For host process visibility (reboot, system info)
- Docker socket mount: `/var/run/docker.sock`
- Host file mounts: `/etc/hostname`, `/etc/os-release`, `/proc` (read-only)
