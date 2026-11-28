---
name: Build & CI Pipeline
description: Guide to the Dockerfile, Docker Compose, the Makefile and the CI workflows that build and test Docklift.
---

# Build & CI Pipeline Guide

Docklift ships as one image (`docklift-backend`, built from the root `Dockerfile`) plus two stock
images (nginx, certbot), run by `docker-compose.yml` as three containers.

Do not confuse the two build systems:

- **This guide** covers building *Docklift itself* (the platform image).
- **`deployment_system`** covers building *user projects* (Dockerfile / Railpack per deployment).

## Docker Compose (`docker-compose.yml`)

Compose project name: `docklift`.

### Services (3 containers)

| Compose service | Image | Container name | Host port | Purpose |
|-----------------|-------|----------------|-----------|---------|
| `backend` | `docklift-backend` (built from `Dockerfile`) | `docklift-backend` | `${DASHBOARD_BIND:-0.0.0.0}:8080` -> container `8000` | Go server: API, WebSocket terminal, embedded dashboard |
| `nginx-proxy` | `nginx:1.30.8-alpine` | `docklift-nginx-proxy` | `80:80`, `443:443` | Project and panel domains |
| `certbot` | `certbot/certbot:v5.10.0` | `docklift-certbot` | none | Let's Encrypt renewal loop (every 12h); issuance is triggered by the backend via `docker exec` |

The nginx and certbot tags are pinned on purpose; bump them deliberately (`upgrade.sh` pulls them).
The `docklift-frontend` and `docklift-nginx` containers of 2.x no longer exist; `upgrade.sh`
removes them (`remove_obsolete_containers`).

`DASHBOARD_BIND` defaults to **`0.0.0.0`** so install can open `http://SERVER_IP:8080`. Operators may
set `127.0.0.1` for localhost-only. The first account still requires the bootstrap secret.

Control-plane network: `docklift_network` (bridge, IPv6 enabled, subnets `172.28.0.0/16` and
`fd12:3456:789a::/64`). `nginx-proxy` has fixed addresses at the top of each subnet
(`172.28.255.254`, `fd12:3456:789a::ffff:fffe`); those are the only non-loopback peers in the default
`TRUSTED_PROXIES`, so a request from any other container cannot spoof `X-Forwarded-*`. User apps use
**per-project** `dl-net-*` networks (see `networking_proxy` / `deployment_system`).

### Volume mounts (backend)

| Host path | Container path | Purpose |
|-----------|----------------|---------|
| `/var/run/docker.sock` | `/var/run/docker.sock` | Docker API access |
| `./data` | `/app/data` | SQLite database, secrets, uploads |
| `./deployments` | `/deployments` | Project source + generated runtime state |
| `./nginx-proxy/conf.d` | `/nginx-conf` | Generated vhosts |
| `./nginx-proxy/certbot/conf` | `/etc/letsencrypt` | Certificates (RW so restores can write them) |
| `./backups` | `/data/backups` | Backup archives |
| `/etc/hostname`, `/etc/os-release`, `/proc` | `/host/*` (ro) | Host metrics |

The backend runs `privileged: true` and `pid: host` so it can `nsenter` into host PID 1 for
host-level actions (system update, reboot). `nginx-proxy` mounts `conf.d`, `nginx.conf`, `snippets`,
`certbot/www` and `/etc/letsencrypt` **read-only**; only the backend and certbot write there.

### Environment variables (backend)

Defaults live in `server/internal/config/config.go`; compose sets these:

| Var | Purpose |
|-----|---------|
| `DATABASE_URL` | `file:/app/data/docklift.db` |
| `PORT` | Listen port inside the container (8000) |
| `DEPLOYMENTS_PATH`, `NGINX_CONF_PATH`, `BACKUP_PATH`, `DATA_PATH`, `LETSENCRYPT_PATH` | Container-side paths for the mounts above |
| `DOCKER_NETWORK` | `docklift_network` |
| `PORT_RANGE_START` / `_END` | Host port pool when `publish_host_port` is on (default `5500`-`5600`) |
| `JWT_SECRET` / `INTERNAL_API_SECRET` | Empty: generated and persisted in `DATA_PATH/.secrets` on first run (`INTERNAL_API_SECRET` is kept for compatibility, not a credential) |
| `CORS_ORIGIN` | Extra browser origins (comma-separated) when the panel is not same-origin |
| `TRUSTED_PROXIES` | Peers whose `X-Forwarded-*` headers are trusted (loopback + the fixed nginx-proxy addresses) |
| `DOCKLIFT_FRONTEND_URL` | Public dashboard URL, used for GitHub App callbacks |
| `CERTBOT_EMAIL` / `CERTBOT_STAGING` / `CERTBOT_CONTAINER` | Let's Encrypt registration, staging toggle, sidecar name |
| `NGINX_PROXY_CONTAINER` | Name of the proxy container to reload |
| `DASHBOARD_BIND` | Compose-level only: panel listen address (default `0.0.0.0`) |

### Project app env (multi-service)

`env_variables.service_name`: `""` is shared by every service; otherwise it is the Docker service
name (folder). Deploy merges shared + service-scoped values (service keys win). UI: multi-service
projects use a **Workspace** rail; **All services** owns Deploy / Build / Source / shared Env, a
**service** workspace owns that service's Overview, Env, Domains, Storage and runtime Logs.
Single-service projects keep flat tabs. All-services actions (Redeploy / Restart / Stop / Delete)
always affect the whole compose stack.

## Dockerfile (root `Dockerfile`)

One image, three stages plus the runtime stage:

```dockerfile
FROM --platform=$BUILDPLATFORM oven/bun:1.4.5-alpine AS web      # bun install --frozen-lockfile; bunx vite build
FROM --platform=$BUILDPLATFORM golang:1.27.4-alpine3.24 AS server # go build -> /out/docklift
FROM alpine:3.24.4                                                # runtime
```

- **web**: builds `frontend/` with `VITE_API_URL` empty (same-origin API and WebSocket).
- **server**: `CGO_ENABLED=0`, `GOTOOLCHAIN=local`, `-mod=readonly`. It deletes the tracked placeholder
  `server/web/dist`, copies the real `frontend/dist` in, and builds
  `go build -trimpath -ldflags "-s -w -X docklift/internal/version.Version=${v}" ./cmd/docklift`.
  `v` is the `VERSION` build arg, or the `version` of the root `package.json` when the arg is empty.
  Both stages run on the build platform and cross-compile for `TARGETOS`/`TARGETARCH`
  (linux/amd64, linux/arm64), so multi-arch builds do not emulate Go or Bun.
- **runtime** (`alpine:3.24.4`): `apk add ca-certificates tzdata docker-cli docker-cli-buildx
  docker-cli-compose git bash procps util-linux`, the Railpack binary, and `/usr/local/bin/docklift`.
  Runs as root. `ENV PORT=8000 DATABASE_URL=file:/app/data/docklift.db`, `EXPOSE 8000`,
  `HEALTHCHECK` on `/api/health`, `ENTRYPOINT ["/usr/local/bin/docklift"]`, `CMD ["serve"]`
  (`serve` applies the migrations before listening).

> **Key details**:
> - `docker-cli`, `docker-cli-compose` **and `docker-cli-buildx`** are all required. Railpack builds
>   go through `docker buildx build`, so a missing buildx plugin breaks every Railpack deployment
>   while Dockerfile deployments keep working.
> - Railpack is pinned (`ARG RAILPACK_VERSION=0.33.0`) and verified with per-arch SHA-256 args
>   (`RAILPACK_SHA256_AMD64`, `RAILPACK_SHA256_ARM64`) plus `railpack --version`. When bumping, update
>   the version and both checksums; never float it.
> - `util-linux` provides `nsenter`; `procps` gives accurate `ps`/`top` for host process listing.
> - Runs as **root**: the Docker socket and `nsenter` into host PID 1 require it.
> - The dashboard is not served by nginx any more; `server/web` embeds `frontend/dist` with
>   `go:embed`. A plain `go build` without copying `frontend/dist` into `server/web/dist` embeds only
>   the placeholder `index.html`.

## Makefile

`make build` (dashboard + binary into `bin/docklift`), `make server` (binary only, placeholder
dashboard), `make test`, `make contract`, `make frontend`, `make lint`, `make vuln`, `make docker`,
`make dev`, `make clean`. `VERSION` defaults to the root `package.json` version; `GOVULNCHECK_VERSION`
(`v1.9.0`) matches `ci.yml`. The targets mirror the CI jobs.

## CI (`.github/workflows/`)

`ci.yml` (push to master and pull requests, `website/**` ignored) has these jobs:

| Job | What it runs |
|-----|--------------|
| `go` | `gofmt -l server tests/contract`, `go vet` in `server` and `tests/contract`, `go test -count=1 ./...` in `server`, `govulncheck` |
| `contract` | Builds the binary, runs `tests/contract` with `DOCKLIFT_GO_BIN` |
| `frontend` | Bun 1.4.5: `bun install --frozen-lockfile`, `bunx tsc -p tsconfig.app.json --noEmit`, `bunx vite build`, `bun audit --prod` |
| `shell` | `bash -n` + shellcheck on the install/upgrade/uninstall/`scripts` shell files, the `upgrade.sh` run-state test, `docker compose config`, compose-layout greps (exactly `backend certbot nginx-proxy`, `DASHBOARD_BIND:-0.0.0.0}:8080:8000`, fixed proxy addresses, `TRUSTED_PROXIES` default), release-blocker greps (`docker image prune` / `docker system prune` forbidden in non-test server code, Bun and Go base image pins) |
| `docker` | `docker build`, smoke test (`/api/health`, `<div id="root">` served at `/`, health status, `docker`/`git`/`bash`/`nsenter`/`ps`/`railpack` present, `docker compose version`, `docker buildx version`), then Docker-gated `go test` (`DOCKLIFT_DOCKER=1`) for `server` and `tests/contract` |

`install.yml` (push, pull request, manual, callable) runs `install.sh` on a clean runner, then
`upgrade.sh` over it, in two scenarios: `fresh` (this commit) and `from-release` (latest GitHub
release upgraded to this commit). The `fresh` scenario also runs the end-to-end suite
(`tests/e2e`, see its README) against the install. `release.yml` and `docs.yml` are covered in
`release_process`.

All third-party actions are pinned by commit SHA. Keep `actions/checkout` at v6 (the shell job
greps for it).

## Build Commands

### Local development
```bash
make dev                    # Go API on :8000 + Vite on :3600
```

### Production (Docker)
```bash
docker compose up -d --build              # build and start everything
docker compose up -d --build backend      # rebuild the Docklift image only
docker compose logs -f backend            # follow logs
docker build --build-arg VERSION=2.2.0 -t docklift-backend .   # explicit version
```

### Checks
```bash
make test && make contract && make frontend && make lint
cd frontend && bunx tsc -p tsconfig.app.json --noEmit   # fast typecheck
cd server && go vet ./... && go test ./...
```

## Common Build Issues

| Error | Cause | Fix |
|-------|-------|-----|
| `docker buildx` not found during deploy | Image missing the buildx plugin | Keep `docker-cli-buildx` in the runtime stage |
| `railpack: not found` or checksum failure | Railpack download failed or the checksum does not match | Check that `RAILPACK_VERSION` publishes a musl tarball for that arch and update the matching `RAILPACK_SHA256_*` |
| `bun install --frozen-lockfile` fails | `bun.lock` out of sync with `package.json` | Run `bun install` in `frontend/` and commit the lockfile |
| `bun audit --prod` fails | Vulnerable transitive dependency | Bump it or pin a fixed version in `frontend/package.json` `overrides` |
| `cannot determine version` in the Docker build | Root `package.json` has no `version` and no `VERSION` arg | Pass `--build-arg VERSION=...` |
| Dashboard is the placeholder page | `frontend/dist` was not copied into `server/web/dist` before `go build` | Use `make build` or the Docker build |
| `go: updates to go.mod needed` | Dependency change not committed in `go.mod`/`go.sum` | `cd server && go mod tidy`, commit both |

## Dev vs Production Architecture

```
Development:
  Browser -> Vite (:3600) -> /api and /ws proxied -> Go server (:8000)

Production:
  Browser -> SERVER_IP:8080 -> docklift-backend (:8000: API, /ws/terminal, embedded dashboard)
  Public domains -> :80/:443 -> docklift-nginx-proxy -> (project network) -> container_name:internal_port
```
