---
name: Docker Operations
description: Guide for managing and debugging Docklift containers, Docker access from the Go backend, and deployments.
---

# Docker Operations Guide

Docklift is a container management platform, so understanding the underlying Docker operations is crucial.

## Core Containers

The Docklift platform consists of exactly **3 containers** (services in `docker-compose.yml`):

| Container | Compose service | Host port | Purpose |
|-----------|-----------------|-----------|---------|
| `docklift-backend` | `backend` | `${DASHBOARD_BIND:-0.0.0.0}:8080` -> container `8000` | Go binary: API, WebSocket terminal, embedded dashboard |
| `docklift-nginx-proxy` | `nginx-proxy` | `80:80`, `443:443` | Public domains for user apps + the optional panel domain |
| `docklift-certbot` | `certbot` | none | Let's Encrypt renewals (every 12 h); issuance runs through `docker exec` from the backend |

> **Note**: There is NO `docklift-db`, `docklift-frontend` or `docklift-nginx` container (the last two existed in 2.x). SQLite is a file (`/app/data/docklift.db`) inside the backend container, and the React dashboard is compiled into the Go binary with `go:embed` (`server/web/`).

Image pins: `nginx:1.30.8-alpine` and `certbot/certbot:v5.10.0` (bump deliberately; `upgrade.sh` pulls them). The backend image is built from the root `Dockerfile` (bun build of `frontend/`, Go build of `server/`, Alpine runtime with `docker-cli`, `docker-cli-buildx`, `docker-cli-compose`, `git`, `bash`, `procps`, `util-linux` and the pinned `railpack` binary). It runs as root, `privileged: true` and `pid: host` (nsenter into host PID 1 for system update / reboot / upgrade) and mounts `/var/run/docker.sock`.

When you add or rename a core container, update `coreContainers` and the log service list in `server/internal/api/system/system.go`, the Logs page service list in `frontend/src/pages/Logs.tsx`, `uninstall.sh`, and the `docker-compose.yml` environment (`NGINX_PROXY_CONTAINER`, `CERTBOT_CONTAINER`).

## How the Backend Talks to Docker

Two channels, both in `server/internal/dockerx` (names and rules for *what* to create live in `server/internal/deploy`):

| Channel | Used for |
|---------|----------|
| Engine API, official moby client (`github.com/moby/moby/client`, built from the environment: `DOCKER_HOST`, default `/var/run/docker.sock`) | ping, container inspect/list/logs/stats, network create/inspect/connect/disconnect/remove, image list/inspect/remove |
| `docker` CLI via `execx` | `docker compose` (`up`, `down`, `restart`, `kill`), `docker build` / `docker buildx build`, `docker pull`, `docker volume create/rm`, `docker builder prune`, `docker rm -f`, `docker exec` (nginx reload, certbot) |

Rules:

-   Both sit behind interfaces (`dockerx.Engine`, `dockerx.Runner`, `deploy.DockerOps`) so logic is unit-tested without a daemon. Tests that need a daemon are gated by `DOCKLIFT_DOCKER=1`.
-   `Docker.Engine == nil` or an unreachable daemon yields `dockerx.ErrUnavailable` (`IsUnavailable`); routes answer **503** `{"error":"Docker is not available"}`. The API still starts without Docker (the startup network step only logs a warning).
-   `ContainerStatus` inspects by exact name, then falls back to a name-filtered list; any failure is `{not_found, running:false}`.

### `execx`: the only way to start programs

`server/internal/execx` replaces shell invocations. `execx.Run(ctx, execx.Cmd{...})`:

-   No shell: arguments are passed as a list, so user input cannot inject syntax. Errors name the program, never its arguments (arguments can carry secrets).
-   `Timeout`, `Dir`, `Stdin`, streamed or captured `Stdout` / `Stderr`, `OnStart(pid)`.
-   Cancelling the context (or a timeout) stops the **whole process tree** (process group on Unix, `taskkill /T` on Windows) after `KillGrace` (default 3 s) and `Run` returns only after the program exited.
-   Children never inherit the backend's secrets: `JWT_SECRET`, `INTERNAL_API_SECRET` and `DOCKLIFT_GITHUB_*` are removed from the base environment (`execx.IsSensitiveEnv`). `ExtraEnv` is what the caller passes on purpose (build variables as BuildKit secrets).
-   Errors: `*ExitError` (non-zero exit), `*StartError` (cannot start), `ErrTimeout`, or the context error. `ComposeCmd` runs report a non-zero exit through the result, not the error.

### Compose runs

`dockerx.ComposeCmd` builds `docker compose [-f file] -p <project> <args>`.

-   **Never inside a checkout.** Callers pass `Runtime.ComposeWorkDir()` (`deployments/.docklift/.compose-cwd`, empty, owned by Docklift) as `Dir` and a `-f` pointing at the Docklift-generated runtime compose. Docklift never reads a repository's compose file or `.env`.
-   `ComposeDown` runs `down` (default timeout 60 s) and then verifies with `ProbeComposeProject`: containers (`docker ps -aq --filter label=com.docker.compose.project=<name>`) and networks (`docker network ls -q --filter label=...`) must both be empty. A failed probe is "not gone"; stderr text is never trusted. `IsComposeTeardownOK` returns false when the command did not run to completion.
-   Project delete uses `down --remove-orphans --volumes` (persistent volumes are external and removed separately by name), then removes only Docklift-built images.

### Edge proxy attachment

`docklift-nginx-proxy` is attached to every project network after `compose up` so it can resolve `container_name` through Docker DNS:

-   `ConnectProxy(network)` inspects the network first (a missing network is an error), treats "already connected" as success, logs and returns other failures.
-   `DisconnectProxy` (force, best-effort) must run before `compose down` or the network cannot be removed. `TeardownNetwork` also removes the network.
-   `Runtime.ReattachProxyOnBoot` re-attaches the proxy to every project network at startup (hook `proxy-reattach`), because the attachment belongs to the proxy container and is lost when it is recreated.

## Startup Sequence (`server/cmd/docklift/serve.go`)

`docklift serve` runs the database migrations, loads/creates secrets, creates `DATA_PATH`, `DEPLOYMENTS_PATH`, `BACKUP_PATH`, then runs hooks (`app.Lifecycle`) in this order:

1.  `PhaseDockerNetwork` (before the listener opens): `docker-network` = `EnsureNetwork` creates the bridge network `DOCKER_NETWORK` (default `docklift_network`, labels `com.docklift.managed=true`, `com.docklift.role=control-plane`) if it does not exist; also `backup-restore-recovery`. A failure only logs "Docker unavailable".
2.  Listener on `:PORT` (default 8000).
3.  `PhaseAfterListen` (background): `proxy-reattach`, then `nginx-sync` (`Stack.Boot`: `MigratePanelUpstreams`, then `SyncConfigs`), plus route-group hooks.
4.  Bootstrap secret notice (fresh install).
5.  `PhaseRecovery`: `deployment-recovery` (`Runtime.RecoverOnBoot`). Then `DedupeEnvVariables`.
6.  `PhaseWatchers`: `cert-renew-watcher` (`Stack.StartWatchers`).

Shutdown (SIGINT/SIGTERM): background context cancelled (ends streams), HTTP server drained (10 s), then hooks (`deployments` waits for running operations, `terminal sessions` closes).

## Viewing Logs

### CLI
```bash
docker logs docklift-backend -f --tail 100         # API, deploy engine, startup hooks
docker logs docklift-nginx-proxy -f --tail 100     # public proxy (:80/:443)
docker logs docklift-certbot -f --tail 100         # certificate renewal loop
docker exec docklift-certbot tail -n 100 /var/log/letsencrypt/letsencrypt.log   # issuance details
```

### UI
Navigate to `/logs` in the dashboard for real-time streamed logs with timestamps, ANSI/level colouring, search, follow-mode and download. See the `logging_monitoring` skill.

## Inspecting Deployed User Projects

User containers are named `dl_<slug>_<id8>_<service>` (see `server/internal/deploy/naming.go`).

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

1. **Check build logs**: in the UI, under the project's Deployments tab (stored in `deployments.logs`, secrets redacted).
2. **Inspect container**: `docker inspect <container>`
3. **View logs**: `docker logs <container>`
4. **Enter shell**: `docker exec -it <container> /bin/sh`
5. **Read generated runtime state**: `deployments/.docklift/<projectId>/compose.yml` (on the host, relative to the install directory); this, not any repository compose file, is what actually ran. Compose is always run with `-f` that file.
6. **Confirm proxy attachment**: `docklift-nginx-proxy` should appear on `dl-net-<id8>` after deploy (`docker network inspect dl-net-<id8>`).
7. **Backend from the host**: `curl http://127.0.0.1:8080/api/health` (container healthcheck uses `wget` against `http://127.0.0.1:${PORT}/api/health`).

## Images and Build Cache

-   Built images are tagged `docklift-<project8>-<service slug>:<deployment8>`. Only these tags are ever pruned, and only by keep-2 retention after a successful deploy (`PostDeployCleanup`), by project delete (that project's tags), or by System **Purge** (every project's keep-2 set is kept).
-   BuildKit: `docker builder prune -f` (unused cache) after a successful deploy; `-af` only in System Purge. All projects share the daemon's default BuildKit builder and its cache; Docklift does not create per-project builders.
-   Images pulled for managed databases (`postgres`, `mysql`, ...) and the platform images are never removed by Docklift.

```bash
docker images --filter "reference=docklift-*"
docker builder du
```

## Networks

| Network | Who | Purpose |
|---------|-----|---------|
| `docklift_network` | `docklift-backend`, `docklift-nginx-proxy`, `docklift-certbot` | Control plane |
| `dl-net-<id8>` | One project's containers + the edge proxy (+ linked database containers) | App isolation |

`docklift_network` (compose: `name: docklift_network`, bridge, IPv6 enabled) uses subnets `172.28.0.0/16` and `fd12:3456:789a::/64`. `docklift-nginx-proxy` has **fixed addresses** at the top of each subnet: `172.28.255.254` and `fd12:3456:789a::ffff:fffe`. Docker allocates dynamic addresses from the bottom, so these never collide with other containers. The backend's default `TRUSTED_PROXIES` is loopback plus exactly those two addresses, so only the proxy may set `X-Forwarded-*`; app containers and the network gateway (which fronts `:8080` clients) cannot spoof client IPs. If you change the subnet or the addresses, change `TRUSTED_PROXIES` too.

```bash
docker network inspect docklift_network
docker network ls --filter label=com.docklift.managed=true
```

Do **not** put user apps back on `docklift_network` by default. Do **not** bind apps to `127.0.0.1` as a substitute for isolation without redesigning proxy routing.

## Volume Management

Bind mounts of `docklift-backend` (from `docker-compose.yml`, relative to the install directory):

| Host Path | Container Path | Purpose |
|-----------|---------------|---------|
| `./data` | `/app/data` | SQLite DB (`docklift.db`), secrets, SSL watcher state |
| `./deployments` | `/deployments` | Project source + generated runtime state (`.docklift/`) |
| `./nginx-proxy/conf.d` | `/nginx-conf` | Generated vhosts (mounted read-only into nginx-proxy) |
| `./nginx-proxy/certbot/conf` | `/etc/letsencrypt` | Certificates (read-write here so backups can restore them; read-only in nginx-proxy, read-write in certbot) |
| `./backups` | `/data/backups` | Backups |
| `/var/run/docker.sock` | same | Docker daemon |
| `/etc/hostname`, `/etc/os-release`, `/proc` | `/host/...` (read-only) | Host info for the System page |

Key environment (compose `environment:` of `backend`): `DATABASE_URL=file:/app/data/docklift.db`, `PORT=8000`, `DEPLOYMENTS_PATH`, `NGINX_CONF_PATH`, `DOCKER_NETWORK`, `BACKUP_PATH`, `DATA_PATH`, `PORT_RANGE_START` / `PORT_RANGE_END` (default 5500-5600), `JWT_SECRET`, `INTERNAL_API_SECRET`, `CORS_ORIGIN`, `TRUSTED_PROXIES`, `DOCKLIFT_FRONTEND_URL`, `CERTBOT_EMAIL`, `CERTBOT_STAGING`, `LETSENCRYPT_PATH`, `CERTBOT_CONTAINER`, `NGINX_PROXY_CONTAINER`. Defaults live in `server/internal/config/config.go`.

Named volumes for project data are created per configured mount as `dl-<id8>-<slug>-<hash>` with labels `com.docker.compose.project=<compose project>` and `com.docklift.project=<projectId>`:

```bash
docker volume ls --filter label=com.docklift.project
docker volume ls --filter label=com.docklift.project=<projectId>
```

These are **external** volumes from Compose's point of view, so `docker compose down` will not delete them. They are removed when the project (or the storage entry) is deleted.

## Pruning

```bash
# Host-wide and indiscriminate: NEVER from Docklift product paths
docker system prune -a
```

Product paths may prune **only** `docklift-<project8>-*` app tags (keep-2 after success; a project's own tags on delete) and BuildKit cache (`builder prune -f` after success, `-af` on System Purge). Never `docker system prune`, and never delete foreign or managed-database upstream images from the panel.

Helpers: `server/internal/deploy/images.go` (`RemoveUnusedDockliftImages`, `ProjectKeepSet`, `GlobalKeepSet`), `dockerx.BuilderPrune`, `server/internal/api/system/purge.go`.

For full platform removal use `uninstall.sh`, which targets Docklift-owned resources (names `docklift*`, `dl_*`, `dl-*`; `com.docklift.*` labels; `dl-net-*` networks).

## Development

```bash
make build        # dashboard + Go binary with the dashboard embedded -> bin/docklift
make test         # go vet + go test (server), go vet (contract suite)
make contract     # contract suite against the Go binary
make docker       # docker build -t docklift-backend .
DOCKLIFT_DOCKER=1 make test contract   # Docker-gated tests (needs a daemon)
```

Backend commands run in `server/` with the standard Go toolchain (`go test ./...`, `go vet ./...`); the frontend uses `bun` (`make frontend`).
