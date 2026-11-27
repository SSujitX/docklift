---
name: System Administration
description: Guide for server management, system APIs, backups, restore, and maintenance operations in the Docklift Go stack.
---

# System Administration Guide

Docklift ships server management features in the dashboard and API. The Go backend (`server/`) implements them; the three-container stack is defined in `docker-compose.yml`.

## Stack

| Container | Role | Ports |
|-----------|------|-------|
| `docklift-backend` | Go binary: API, WebSocket terminal, embedded dashboard | container 8000, host `:8080` (`DASHBOARD_BIND` selects the host address) |
| `docklift-nginx-proxy` | Public proxy for app domains and the optional panel domain | `:80`, `:443` |
| `docklift-certbot` | `certbot renew` every 12 h (webroot); the backend triggers issuance with `docker exec` | none |

There is no frontend container, no gateway nginx, and no separate database container: the SQLite file is `DATABASE_URL` (default `file:/app/data/docklift.db`), migrated by `docklift serve` at start.

## Backend Server Access Requirements

From `docker-compose.yml` (the backend container runs as root):

-   `privileged: true` and `pid: host`: `nsenter` into host PID 1 for system update, upgrade and host process listing.
-   `/var/run/docker.sock`: all Docker work.
-   Mounts: `./data:/app/data`, `./deployments:/deployments`, `./nginx-proxy/conf.d:/nginx-conf`, `./nginx-proxy/certbot/conf:/etc/letsencrypt` (RW so restores can put certificates back; nginx-proxy mounts it read-only), `./backups:/data/backups`, and read-only `/etc/hostname`, `/etc/os-release`, `/proc` under `/host/`.
-   The image (`Dockerfile`) is Alpine with `docker-cli` (+ buildx, compose), `git`, `bash`, `procps`, `util-linux` (`nsenter`, `script`) and a pinned, checksum-verified Railpack.

### Environment

Read once at start by `server/internal/config/config.go` (`.env` and `.env.local` in the working directory are loaded first):

| Variable | Default | Meaning |
|----------|---------|---------|
| `PORT` | `8000` | listen port inside the container |
| `DATA_PATH` | `./data` | database dir, secrets, tokens, restore markers |
| `DATABASE_URL` | `file:/app/data/docklift.db` | SQLite file |
| `DEPLOYMENTS_PATH` | `./deployments` | project sources and runtime compose files |
| `BACKUP_PATH` | `<DATA_PATH>/backups` (compose sets `/data/backups`) | backups and `uploads/` |
| `NGINX_CONF_PATH` | `./nginx-proxy/conf.d` | generated vhosts |
| `LETSENCRYPT_PATH` | `./nginx-proxy/certbot/conf` | certificates |
| `DOCKER_NETWORK` | `docklift_network` | control-plane network |
| `PORT_RANGE_START` / `PORT_RANGE_END` | `5500` / `5600` | host ports for deployed apps |
| `JWT_SECRET`, `INTERNAL_API_SECRET` | generated into `DATA_PATH/.secrets` | see `authentication` |
| `CORS_ORIGIN`, `DOCKLIFT_FRONTEND_URL` | empty / `http://localhost:3000` (compose: `http://localhost:8080`) | trusted origins, callback fallback |
| `TRUSTED_PROXIES` | loopback + nginx-proxy's fixed address | see `security_hardening` |
| `TERMINAL_ALLOW_NO_ORIGIN` | `false` | allow terminal upgrade without `Origin` |
| `CERTBOT_EMAIL`, `CERTBOT_STAGING`, `CERTBOT_CONTAINER`, `NGINX_PROXY_CONTAINER` | | TLS issuance |

Data and backup directories are `0700` and their files `0600`; `TightenModes` repairs older installs at boot.

## CLI

The binary is `/usr/local/bin/docklift` in the container (working dir `/app`):

```bash
docker exec docklift-backend docklift reset-password   # new random admin password, prints it
docker exec docklift-backend docklift migrate          # apply migrations and exit
docker exec docklift-backend docklift serve            # the container entrypoint (default)
```

Local development, from the repo root: `make dev` (Vite dev server plus `go run ./cmd/docklift serve` in `server/`), `make build` (dashboard embedded into `bin/docklift`), `make server` (placeholder dashboard, enough for API work), `make test`, `make contract`, `make lint`.

## System Dashboard (`/system`)

`GET /api/system/stats` (3 s cache), `GET /api/system/quick` (CPU and memory percent for the header widget), `GET /api/system/ip` (public IP via `api.ipify.org`, 5 min cache, `N/A` when unknown). Code: `server/internal/api/system/system.go` and `server/internal/sysinfo`.

-   **CPU, memory, disk, network, GPU, load**: read with gopsutil and the `/host/proc`, `/host/os-release`, `/host/hostname` mounts. Load averages are the real system values.
-   **Processes**: top 10 by CPU. The host process table comes from `nsenter --target 1 ... ps aux --sort=-%cpu`, with `top` inside the container as fallback.
-   **Server info**: hostname, distro, kernel, uptime, public IP.
-   On Windows and macOS (development) host readings that need Linux return what is available.

## Maintenance Operations

All of these need a session token **and** the account password in the body (step-up, throttled at 10 wrong attempts per user and address per 15 minutes). Each logs an `[AUDIT]` line with the client address.

| API | Effect |
|-----|--------|
| `POST /api/system/purge` | Docklift-scoped cleanup, below |
| `POST /api/system/reset` | Restart the stack containers: `docklift-nginx-proxy`, `docklift-certbot`, then `docklift-backend` last, after 1 s. A missing container is skipped. |
| `POST /api/system/reboot` | `reboot -f` on the host after 1 s |
| `POST /api/system/update-system` | `apt update`, `apt upgrade`, `apt autoremove` on the host via `nsenter`, 15 min timeout |
| `POST /api/system/upgrade` | Starts `/opt/docklift/upgrade.sh` on the host, detached (`systemd-run`, falling back to `nohup`) |
| `POST /api/system/execute` | Run one shell command in the container with the password re-verified; 30 s timeout, 1 MiB output cap |

On Windows and macOS (dev machines) `reboot`, `update-system` and `upgrade` are simulated, and `reset` is simulated on Windows only. When Docker is unreachable, Docker routes answer 503 "Docker is not available".

### Purge

`POST /api/system/purge` never touches anything outside Docklift:

1.  **409** if any deployment is `in_progress`, checked before and again after the keep set is computed.
2.  Remove `docklift-*` application image tags that are outside each project's **keep-2 set** (images of the last two successful deployments) and not used by **any** container on the host, running or stopped.
3.  `docker builder prune -af`, skipped (and reported) if a deployment started meanwhile. The BuildKit cache is shared by all projects.
4.  Response `{ message, details[], removedImages, keptImages, memory* }`.

It never runs `docker system prune`, removes foreign images, containers or volumes, or does OS maintenance (swap, `drop_caches`, journal, apt, `/tmp`).

Automatic hygiene: after a **successful** deploy only, the keep-2 image set is retained and unused builder cache is pruned. A failed deploy never strips the last good release. Project, Deployments, **Restore** (step-up, `POST /api/deployments/:projectId/rollback`) pairs with keep-2.

## Interactive Web Terminal

-   **Route**: `/terminal`; WebSocket `/ws/terminal` on the same origin as the dashboard (nginx-proxy passes it for a panel domain).
-   **Frontend**: xterm.js in `frontend/src/components/TerminalView.tsx` and `Terminal.tsx`.
-   **Backend**: `server/internal/api/terminal` (upgrade checks) and `server/internal/terminal` (sessions). The shell is `/bin/bash` in `/root` on a PTY (`creack/pty`), Linux only. The container has host access through privileged mode and `pid: host`.
-   **Connect flow**: `POST /api/auth/terminal-token` (5 min, **single use**), open `/ws/terminal?token=...`, receive `auth_required`, send `{ type: "auth", password, cols, rows }` within **60 s**, receive `auth_success`, then `input` / `resize` messages and `output` / `exit` messages back.
-   **Limits**: 3 concurrent sessions; 5 wrong passwords per session and 20 per address and user per 15 minutes; 30 minute idle timeout; resize accepts integers `0 < n < 500`; 1 MiB per client message; ping every 30 s.
-   **Ends when**: the shell exits, the idle or auth timeout hits (`exit` with `reason` `idle_timeout` / `auth_timeout`), the server stops, or the user's **password changes** (`session_revoked`).
-   The shell environment has `JWT_SECRET`, `INTERNAL_API_SECRET` and `DOCKLIFT_GITHUB_*` removed.
-   **Host strip UI**: text actions above the shell for package update, Docklift upgrade, purge, reset stack, reboot. Upgrade and package dialogs confirm first and then show a wait dialog; the sidebar "Upgrade now" opens `/terminal?confirm=upgrade` and defers the shell password prompt so the operator is not asked twice.

### Graceful shutdown

On SIGINT/SIGTERM `docklift serve` cancels background work (which ends streams), drains the HTTP server for up to 10 s, closes every terminal session, runs shutdown hooks and closes the database.

## System Logs

`GET /api/system/logs/:service` (SSE, `?tail=` default 200). Mapping in `server/internal/api/system/system.go` (`logServices`), UI in `frontend/src/pages/Logs.tsx`:

| Service key | Container | UI label |
|-------------|-----------|----------|
| `backend` | `docklift-backend` | Backend |
| `proxy` | `docklift-nginx-proxy` | Public Proxy |
| `certbot` | `docklift-certbot` | Certbot |

Any other key gives 400. See the `logging_monitoring` skill for the stream format and viewer.

## Version Check

`GET /api/system/version` returns `{ current, latest, updateAvailable, githubOk, checkedAt }`. `current` is the `docklift/internal/version.Version` set at build time (from the root `package.json`). It compares against GitHub `releases/latest`. TTL about 15 min when an update exists, 2 min when current, 30 s when GitHub fails (a failed probe is never cached as "no update"). Concurrent callers share one fetch; `?refresh=1` re-probes only if the cache is older than 30 s. The UI polls from the sidebar footer only.

## Backup and Restore

Handlers: `server/internal/api/backup/`; archive and restore logic: `server/internal/backup/`; locks and seal: `server/internal/maintenance/`. Mounted at `/api/backup`, session token required (except fresh-install `restore-upload`, below).

### Archive layout

| Entry | Source |
|-------|--------|
| `database/docklift.db` | consistent SQLite snapshot (`VACUUM INTO`), never a raw copy of the live file |
| `deployments/` | `DEPLOYMENTS_PATH`, including `.docklift/<projectId>/compose.yml` |
| `nginx-conf/` | `NGINX_CONF_PATH` |
| `letsencrypt/` | `LETSENCRYPT_PATH` (links stored as links) |
| `github-app.pem` | `DOCKLIFT_GITHUB_PRIVATE_KEY_PATH`, when it exists |

Named volume contents are **not** archived: volumes are re-created empty if missing. Snapshot application data separately.

### Backup API

| API | Purpose |
|-----|---------|
| `GET /api/backup` | list server-created backups, newest first |
| `POST /api/backup/create` | create a backup; streamed `text/plain` progress; optional `name` (sanitised to `[A-Za-z0-9_-]`, 50 chars) |
| `GET /api/backup/download/:filename` | download (fetch with Bearer + blob; no URL tokens) |
| `DELETE /api/backup/:filename` | delete |
| `POST /api/backup/upload` | store an uploaded ZIP (field `backup`, 500 MiB max, ZIP only) without restoring |
| `GET /api/backup/uploads`, `DELETE /api/backup/uploads/:filename` | manage uploaded files |

Create writes to a hidden `.partial` file, fsyncs, then renames, so a crash never leaves a truncated `.zip`. Failures appear as `[ERROR]` lines in the stream while the HTTP status stays 200. The frontend (`lib/streamProgress.ts`) requires `res.ok` and treats any `[ERROR]` line as failure. Filenames must match `^[a-zA-Z0-9._-]+\.zip$` and stay inside the backup directory after symlink resolution.

### Restore API

| API | Purpose |
|-----|---------|
| `POST /api/backup/restore/:filename` | restore a server-side backup (step-up) |
| `POST /api/backup/restore-from-upload/:filename` | restore a previously uploaded file (step-up) |
| `POST /api/backup/restore-upload` | upload and restore in one request (step-up, or `X-Setup-Token` on a fresh install) |
| `GET /api/backup/critical-status` | is the restore seal set |
| `POST /api/backup/clear-critical-restore` | clear the seal (step-up) |

### Restore safety

-   **Restore lock** (`TryAcquireRestoreLock`) is taken before any upload byte is read. A concurrent restore gets **409**. The lock also fails while the critical seal exists. Uploaded names are replaced by `restore-<ms>-<random>.zip`.
-   **Maintenance mode** is on for the whole restore: every `/api/*` request except `/api/health` and `/api/backup/restore*` gets 503.
-   Extraction uses `safeunzip` (50,000 files, 2 GiB), with a free-space check, into `BACKUP_PATH/temp-restore-<ms>`. The staged database is integrity-checked and migrated before anything live is touched.
-   Stages: extract, validate, stop user containers, **marker** (the point of no return), `.pre-restore` copy of the live database, database swap, migrations, deployments, nginx-conf, letsencrypt, GitHub key, reconcile, commit. Directory areas are replaced by content inside the target (`fscopy.ReplaceDirContents`), never by renaming bind-mounted roots, with the previous content kept in `.docklift-restore-prev-*` for rollback.
-   **Restore-critical seal**: before the first live change a durable marker is written. A crash or a failed rollback leaves `DATA_PATH/.restore-critical` (mode 0600), which is loaded at boot, keeps maintenance and the restore lock on, and blocks every `/api/backup/restore*` call. Only `POST /api/backup/clear-critical-restore` (step-up) removes it, and if deletion fails maintenance stays on.
-   **Commit policy** (`DecideCommit`): a restored database with no `admin` user is never committed; a setup-token restore whose reconcile failed is rolled back from `.pre-restore`; an authenticated restore keeps its data even if reconcile partly failed. The setup token and bootstrap secret are consumed only on a successful setup-token commit.
-   After a committed restore the backend sends itself SIGTERM after 1 s (`restartProcess`), and Docker's `restart: unless-stopped` starts it on the restored data. A 30 s watchdog forces exit.
-   Restore does not delete other backup ZIPs.

### Reconcile

After the files are in place, `reconcile` starts every project:

1.  Read projects from the restored database.
2.  Re-create each project's persistent volumes (external, labelled).
3.  Start the project **only from the Docklift-generated runtime compose file** `DEPLOYMENTS_PATH/.docklift/<projectId>/compose.yml`, run from that directory, project name `ComposeProjectName(name, id)`. A repository's own `docker-compose.yml` is **never** started by a restore. If the runtime file is missing the project is skipped and must be deployed manually. Re-attach the project's containers to nginx-proxy.
4.  Reload nginx-proxy.

Per-project failures produce `[ERROR]` lines and the stream ends as incomplete, so the UI does not show success. The compose project name must match deploy-time naming or restore creates a second set of containers.

## Install, Upgrade, Uninstall

| Script | Purpose |
|--------|---------|
| `install.sh` | Production install into `/opt/docklift` (default **latest** release). Pin with `bash -s -- v=2.0.2` or `DOCKLIFT_VERSION`. Fails closed if the release tag cannot be resolved. Waits for health, prints `Dashboard: http://SERVER_IP:8080` and the **setup code** from `data/.bootstrap-secret`. Removes obsolete 2.x containers (`docklift-frontend`, `docklift-nginx`). |
| `install-dev.sh` | Same, from `master` |
| `upgrade.sh` | Stops the backend, takes an SQLite backup (`sqlite3 .backup`, or a file copy with the backend stopped), tags the running image `docklift-backend:pre-upgrade`, rebuilds, health-checks `/api/health`, and rolls back (tagged images plus database restore) on failure. Migrates a 2.x layout to the Go backend and removes the obsolete containers and the `docklift-frontend` image once the new backend is healthy. |
| `uninstall.sh` | Removes Docklift containers, images, volumes, networks, build cache and `/opt/docklift`, restricted to Docklift-owned resources (`docklift*`, `dl_*`, `dl-*`, `dl-net-*`, `com.docklift.*` labels). Never a host-wide prune, never Docker Engine or git. |

The Docker `HEALTHCHECK` calls `GET /api/health` (public), which returns `{ status, timestamp, version, instanceId }`; the dashboard uses `instanceId` to detect restarts.

## Pitfalls

-   Changing a path variable in compose without moving the mount makes backups, restores and nginx sync act on the wrong directory.
-   The panel purge must stay Docklift-scoped. Do not add host-wide prune or OS maintenance to product APIs.
-   `reset-password` runs as a separate process: running sessions notice within 30 s because bound connections recheck the password version.
