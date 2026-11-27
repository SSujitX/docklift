---
name: Deployment System
description: Comprehensive guide to how Docklift builds and deploys user applications (Go backend).
---

# Deployment System Guide

This guide details the lifecycle of a deployment in Docklift, from source code to running container. The backend is a Go binary (`server/`); it drives Docker through the moby client (Engine API) and the `docker` CLI (`compose`, `buildx`, `pull`, `volume`, `builder prune`) installed in the `docklift-backend` image.

## Core Packages

-   **`server/internal/deploy`**: the service layer for deployments.
    -   `build_resolver.go`: `ResolveProjectBuild` decides *what* to build (Dockerfile vs Railpack, base directory, service list). `ResolveProjectPath` rejects absolute paths and `..`; `assertRealPathInside` rejects symlinks that resolve outside the project.
    -   `build_runner.go`: `BuildServiceImage` builds one image. Public build args vs `is_secret` (BuildKit `--secret`), Railpack plan + build, `CheckProjectSymlinks`, `SummarizeBuildFailure`.
    -   `compose.go`: `ScanDockerfiles`, `DedupeScannedServices`, `RenderRuntimeCompose` / `WriteRuntimeCompose`.
    -   `naming.go`: every Docker name (compose project, container, network, volume, reserved service names).
    -   `images.go`: image tags, keep-2 retention, rollback guard, `PostDeployCleanup`.
    -   `tracker.go`: in-memory per-project lock and cancel registry (`Tracker`).
    -   `status.go`: status constants, `SyncProjectStatus`, `AggregateProjectStatus`, `RecoverOnBoot`, `FailDeployment`.
    -   `ports.go`: `AllocatePort` / `ReleaseServicePort` (host port pool).
    -   `runtime.go`: `Runtime` (store + Docker + tracker + paths), `ComposeWorkDir`, `ComposeFiles`, proxy and network helpers, `ReattachProxyOnBoot`.
    -   `envvars.go`: `EnvForService` (shared + service-scoped variables).
    -   `dbengines.go`, `dblinks.go`, `hubtags.go`: managed databases (see the `managed_databases` skill).
-   **`server/internal/api/deployments`**: HTTP routes and the pipeline.
    -   `pipeline.go`: `StartDeploy` and the deploy steps (pull, build, compose, proxy attach, status, domains, cleanup).
    -   `lifecycle.go`: `Stop`, `Restart`, `Cancel`, teardown.
    -   `rollback.go`: `Rollback` (restore previous).
    -   `queue.go`: per-project auto-deploy queue used by the GitHub webhook.
    -   `logsink.go`: deploy log buffer, secret redaction, periodic DB sync, client stream.
-   **`server/internal/api/projects`**: project CRUD, `GET /:id/build/detect`, storage (persistent volumes), env variables, auto-deploy flag, and `DeleteProject` (`delete.go`).
-   **`server/internal/dockerx`**: `ComposeCmd` / `Compose` / `ComposeDown` (with the exact-label probe), `PullImage`, `VolumeCreate` / `VolumeRemove`, `BuilderPrune`, network helpers (`ConnectProxy`, `DisconnectProxy`, `TeardownNetwork`, `ConnectContainer`).
-   **`server/internal/execx`**: the only way to start external programs (see `docker_operations`).
-   **`server/internal/gitops`**: clone / pull / reset, origin scrub, credential verification.
-   **`server/internal/pathsec`, `safeunzip`**: path containment and ZIP extraction (rejects traversal entries and links).

## Deployment Lifecycle

1.  **Trigger**
    -   Manual (UI button) or GitHub push webhook.
    -   `POST /api/deployments/:projectId/deploy` (also `/redeploy`) streams plain-text progress. The deploy itself runs on the server's background context: a client that disconnects stops receiving output but never aborts the deploy.
    -   **Serialized per project**: the `Tracker` lock **and** an `in_progress` deployment row. A second deploy gets **409** ("A deployment is already running for this project"); during a cancel teardown **409** ("A cancel is in progress for this project").
    -   `stop` / `restart` return **409** while a deploy runs; cancel first. `DELETE /api/projects/:id` while a deploy runs returns **409** ("Cannot delete project while a deployment is in progress").
    -   **Webhook deploys are queued per project** (`Queue` in `queue.go`, `EnqueueDeploy` with trigger `webhook`): at most one running and one pending. Pushes that arrive while a deploy (or any other operation on the project) is running collapse into one more run, started after the current one finishes; it pulls the branch tip at that time, so it deploys the latest commit. A busy project is retried every 2 s or when an operation ends; the request is not dropped while the server runs.

2.  **Preparation**
    -   A deployment row is created (`status: in_progress`; `trigger` comes from the request body and defaults to `manual`, the webhook queue uses `webhook`), project and services go to `building`.
    -   Source lives **in place** at `deployments/<projectId>/` (`DEPLOYMENTS_PATH`; `/deployments` in the container):
        -   **GitHub**: clone, or fetch + reset + clean for an existing checkout (`gitops`).
        -   **Upload**: ZIP extracted through `safeunzip` (rejects traversal entries).
    -   The commit SHA is stored on the row for GitHub/public projects.

    **Git token security** (GitHub projects):
    -   The installation token is requested just in time (`githubapp`) and sent as a request header for the pull; it is not stored in the checkout. The origin remote is reset to the stored repository URL before the token is requested.
    -   After the pull the origin is scrubbed (`ScrubOrigin`) and `VerifyNoCredentials` runs. **If the scrub fails the deployment fails** ("Failed to scrub Git credentials after pull").
    -   Programs run through `execx` with argument lists, never a shell. `execx` removes `JWT_SECRET`, `INTERNAL_API_SECRET` and `DOCKLIFT_GITHUB_*` from every child environment.

3.  **Build Resolution** (`ResolveProjectBuild`)
    -   `build_type` is `auto` (default), `dockerfile` or `railpack` (anything else normalizes to `auto`).
    -   **`auto`**: scans for files named exactly `Dockerfile` up to 2 directories below `base_directory` (skipping symlinks, `node_modules`, `.git`, `.github`, `vendor`, `__pycache__`, `.venv`, `venv`, `.next`, `dist`, `build`, `.cache`); every hit is a service. With none, it falls back to one Railpack service named `app`.
    -   **`dockerfile`**: one service `app`, `dockerfile_path` (default `Dockerfile`) relative to the base directory; fails with "Dockerfile not found" instead of falling back.
    -   **`railpack`**: always one Railpack service `app`, even when a Dockerfile exists.
    -   `base_directory` (default `.`) scopes detection to a monorepo subdirectory; it must exist and stay inside the project (including through symlinks).
    -   Scanned service names come from the Dockerfile's directory (`app` at the root, `a/b` becomes `a-b`); name collisions are suffixed with a 4-character path hash (`ShortPathHash`).
    -   **Reserved names**: a scanned service name equal to or starting with `docklift` (case-insensitive) is refused (`ReservedServiceNameError`). The service key is a DNS alias on the project network that `docklift-nginx-proxy` joins, so a project must not answer for `docklift-backend` or `docklift-backend.docklift_network`. `GET /api/projects/:id/build/detect` returns **400** with the message; during a deploy the error fails the deployment. `RenderRuntimeCompose` repeats the check for stored service rows (rollback, redeploy).
    -   `DetectManifests` reports which manifests exist (`package.json`, `requirements.txt`, `pyproject.toml`, `go.mod`, ...); the UI shows them as the detected stack. `detect` does not build.

4.  **Image Build** (`BuildServiceImage`, one image per service, built sequentially)
    -   Every image gets an explicit tag: `docklift-<project8>-<service slug>:<deployment8>` (`DockliftImageTag`). `project8` is the first 8 characters of the project UUID, `deployment8` the first 8 of the deployment id.
    -   **Dockerfile builds**: `docker build --progress plain -t <tag> -f <Dockerfile> <context>`.
        -   Public build args: `--build-arg` only for variables with `is_build_arg` and not `is_secret`. `ValidateDockerBuildArgs` warns (does not fail) when the Dockerfile has no matching `ARG`.
        -   Secrets: `is_secret` + `is_build_arg` switch the build to `docker buildx build --load` with `--secret id=KEY,env=KEY`; the value reaches the process only through its environment, never `--build-arg`. A missing `RUN --mount=type=secret,id=KEY` in the Dockerfile is a **preflight failure** (the deploy aborts before building).
    -   **Railpack builds**: `railpack prepare` writes `<service>-railpack-plan.json` and `-railpack-info.json` into `deployments/.docklift/<projectId>/` (build variables are replaced by a placeholder during prepare), then `docker buildx build --load -f <plan>` with `BUILDKIT_SYNTAX=` the Railpack frontend. Every build variable is a BuildKit secret, plus a `secrets-hash` build arg so cached layers are invalidated when a secret changes.
        -   The frontend is **pinned by digest**: `RailpackFrontend = ghcr.io/railwayapp/railpack-frontend:v<RailpackVersion>@sha256:...` (`RailpackVersion` 0.33.0). Update the version and `railpackFrontendDigest` together; the `railpack` CLI in the `Dockerfile` (`RAILPACK_VERSION`, per-arch SHA-256) must match.
        -   **Symlink guard**: before `railpack prepare`, `CheckProjectSymlinks` walks the whole checkout and fails the build if a symlink resolves outside the project directory (a dangling link is allowed only with a relative target that never climbs with `..`). `railpack prepare` runs inside the backend container and would follow such a link into the plan and the image.
    -   Build variables named `PATH`, `HOME`, `PORT`, `NODE_OPTIONS`, `LD_PRELOAD`, `LD_LIBRARY_PATH`, `DOCKER_*` or `BUILDKIT_*` are rejected ("reserved by DockLift").
    -   The last 128 KiB of output is kept; `SummarizeBuildFailure` turns an out-of-sync `package-lock.json` or lockfile into one actionable line, otherwise the error is `<program> exited with code N`.
    -   Cancelling kills the whole process tree (process group on Unix), so BuildKit and compose children die too.
    -   **Shared builder cache**: no builder is created or selected, so every project builds with the Docker daemon's default BuildKit builder and shares its cache. BuildKit cache mounts (`RUN --mount=type=cache`, used by Railpack and many Dockerfiles) are shared by id across all builds on the host. Only deploy repositories you trust on one server.
    -   Secret values are registered with the deploy log redactor (`***`); values shorter than 4 characters are not redacted.

5.  **Run**
    -   `WriteRuntimeCompose` writes `deployments/.docklift/<projectId>/compose.yml` (mode 0600, atomic rename). **Repository files are never patched**; a committed `docker-compose.yml` or `.env` is never read.
    -   **Compose never runs inside the checkout.** Every `docker compose` command runs with working directory `deployments/.docklift/.compose-cwd` (`Runtime.ComposeWorkDir`, an empty directory Docklift owns) and an explicit `-f <runtime compose>`. Without that, compose would load a repository's own compose file and `.env`, and `down --rmi` could remove images that file names.
    -   Per service the generated file sets: `image` (the tag built above), `container_name`, `restart: unless-stopped`, network `project`, labels `com.docklift.managed=true`, `com.docklift.project=<projectId>`, `com.docklift.service=<name>`, `environment` (runtime variables plus `PORT=<internal port>`), `security_opt: no-new-privileges:true`. `mem_limit` / `cpus` only when compose options set them (not applied by default; database images need room). `ports: ["<host>:<internal>"]` only when `publish_host_port` is true. Values are double-quoted so no YAML parser turns them into numbers or booleans.
    -   Network: top-level `networks.project.name = dl-net-<id8>` with ownership labels. User apps are never on the control-plane `docklift_network`.
    -   Command: `docker compose -f <runtime compose> -p <composeProject> up -d --remove-orphans` (output streamed to the log). Before it, legacy UUID-named compose projects (`ComposeProjectAliases`) get a best-effort `down --remove-orphans`.
    -   After `up` succeeds: `ConnectProxyToProjectNetwork` attaches `docklift-nginx-proxy` to the project network. On failure (non-database project) the deploy is marked **failed** and domains are **not** activated. A managed database only warns.
    -   Linked databases are re-attached (`ReapplyDatabaseLinksForApp`; for a database project `ReapplyDatabaseLinksForDatabase`).
    -   Stop / cancel / delete: `DisconnectProxyFromProjectNetwork` (and linked databases) runs *before* `compose down`, because the proxy endpoint otherwise blocks network removal. If teardown fails, the proxy and database links are **reconnected** so running apps keep routing.
    -   Teardown verification: after `compose down`, `ProbeComposeProject` lists containers and networks with the exact label `com.docker.compose.project=<alias>` (`docker ps -aq`, `docker network ls -q`). A failed probe counts as "not gone". Stderr text such as "not found" is never trusted. Stop retries once after detaching the proxy.
    -   **Disk hygiene after a successful deploy** (`PostDeployCleanup`):
        1. Persist `deployments.commit_sha` and `image_tags` (JSON map service name to tag) on success.
        2. **Images: keep 2 per project.** `ProjectKeepSet` reads the newest 100 successful rows, skips rows without tags (stop and restart rows are `success` too) and keeps the tags of the first two. Images used by the project's containers are never removed. Only tags matching `^docklift-[a-f0-9]{8}-...` (`IsDockliftAppImage`) are considered; never host-wide prune, never foreign or upstream images.
        3. **BuildKit**: `docker builder prune -f` (unused cache only) after success, even when no app image was built. `-af` runs only in System **Purge**.
        4. Failed or cancelled deploys do not prune. Cleanup errors are soft (logged, deploy stays successful).
    -   **Restore previous** (`POST /api/deployments/:projectId/rollback`, body `{deploymentId}`; requires the step-up password, `RequireStepUpPassword`):
        -   Target must be a `success` row, not the current success, with stored `image_tags` (else 404 / 400 / 409). Managed databases cannot roll back (400).
        -   Takes the project lock, then checks that every tagged image still exists locally (409 "Image missing for ..." otherwise).
        -   Resets the checkout to the target commit, **rewrites the runtime compose** with the stored tags, and runs `up -d --remove-orphans`. No rebuild. Restore only ever starts the Docklift-generated runtime compose file. The same holds for backup restore: it runs `docker compose -f <runtime compose> -p <project> up -d` from the runtime directory and skips a project without one ("deploy manually").
        -   If compose never succeeded, git HEAD is put back. If compose succeeded and a later step failed, the row is still marked success and git stays at the target.
        -   The new row has trigger `rollback` and copies the target's tags and SHA. The tag rolled away from is removed only by a later keep-2 pass.
    -   Output streams to the UI console as plain text.

6.  **Verification**
    -   `SyncProjectStatus` inspects every service container (Engine API, exact name) and writes service statuses, then aggregates: no services or all stopped is `stopped`; all running is `running`; any `dead` is `error`; some running is **`degraded`** (never a full `running` for a mixed fleet).
    -   The lock is held through the final status write **and** nginx/SSL activation (`activateServiceDomains` writes a vhost for each service that has a domain and container).
    -   The final row update is conditional on `status != cancelled`, so a cancel cannot be overwritten as success. `FailDeployment` likewise never overwrites `cancelled`.

7.  **Cancel (anytime)**
    -   `POST /api/deployments/:projectId/cancel` takes over the lock (`BeginCancel`) for the whole teardown, kills the registered command (`KillActive`), marks only `in_progress` rows `cancelled` (an idle cancel does not rewrite past `success` / `failed` rows), detaches databases and proxy, runs `compose kill`, then `compose down --remove-orphans` for every alias, verified by the label probe.
    -   Project and services become `stopped` only when the teardown is verified; vhosts are then removed.

## Build Types at a Glance

| `build_type` | Behaviour |
|--------------|-----------|
| `auto` | Every repository `Dockerfile` (depth 2), else one Railpack service |
| `dockerfile` | Requires a Dockerfile (`dockerfile_path`); fails instead of falling back |
| `railpack` | Always Railpack, even when a Dockerfile exists |

## Persistent Storage

Rebuilds replace containers, so anything written to a container's filesystem is lost unless mounted.

-   Configured per service in the project **Storage** tab (`/api/projects/:id/storage`) as `persistent_volumes` rows (`service_name`, `mount_path`, `display_name`, `name`).
-   Storage name: 1-32 characters `[A-Za-z0-9_-]`, starting alphanumeric. Mount path: absolute, no `:`, not `/`, not under `/proc`, `/sys`, `/dev`, `/etc`, `/var/run`.
-   Docker volume name: `dl-<id8>-<label slug (28)>-<hash(label)>` (`PersistentVolumeName`), so `a-b` and `a_b` never collide. Created with `docker volume create` and labels `com.docker.compose.project=<compose project>` and `com.docklift.project=<projectId>`. If creation fails the row is removed again.
-   Compose volume keys: `storage_<service slug>_<index>_<hash>` (`StorageVolumeComposeKey`).
-   Volumes are declared **external** in the generated compose file, so `docker compose down` never deletes user data.
-   They are removed only when the project is deleted, or when the storage entry is deleted (`DELETE /api/projects/:id/storage/:storageId?removeVolume=true`; without that flag the request is a 400). Both answer 409 when Docker cannot remove the volume (for example a container still uses it).
-   External databases reached through `DATABASE_URL` need no volume; only in-container state (SQLite files, uploads, caches) does.

## Project Delete

`DeleteProject` (`api/projects/delete.go`): holds the deploy lock; refuses while a deploy runs (409); needs Docker (503 when unreachable); detaches linked databases and the proxy; `compose down --remove-orphans --volumes` for every alias (runtime compose file for the current alias, `.compose-cwd` as working directory), verified by label probe (409 with the Docker detail if resources remain, after restoring proxy and links); removes the project network; removes persistent volumes; deletes `deployments/<id>` and `deployments/.docklift/<id>`; cleans database links when deleting a database; removes service vhosts; deletes the row.

Images are removed **afterwards and by tag only**: `RemoveUnusedDockliftImages` for `docklift-<id8>-*` tags built by Docklift. Upstream images (a managed database's `postgres`, or any image a repository's own compose file might name) are never removed.

## Streaming Safety

Deploy, redeploy, stop, restart, cancel and rollback respond with a plain-text stream. The operation writes to a non-blocking in-memory buffer (`clientStream`); the handler copies it to the client (`pump`). A disconnected client stops receiving output; the operation continues and the log is persisted to `deployments.logs` periodically (`startSync`) and at the end. Panics are recovered and the row is forced to a terminal status (`ensureTerminal`).

## File Structure (Per Project)

```
deployments/                       # DEPLOYMENTS_PATH (/deployments in the container)
  <projectId>/                     # Application source (clone/upload root; never modified by Docklift)
  .docklift/
    .compose-cwd/                  # Empty working directory for every docker compose command
    <projectId>/
      compose.yml                  # Generated runtime compose (0600)
      <service>-railpack-plan.json # Generated Railpack plan
      <service>-railpack-info.json
```

## Naming Conventions

Always use `server/internal/deploy/naming.go`; never hand-build these strings. They must stay stable because existing installs have Docker objects with these names.

| Thing | Format | Example |
|-------|--------|---------|
| Compose project (`-p`) | `dl-<slug>-<id8>` | `dl-python-smoke-53b01966` |
| Container | `dl_<slug>_<id8>_<svc>` (service slug uses `_`; max 63 chars) | `dl_python-smoke_53b01966_app` |
| Project network | `dl-net-<id8>` | `dl-net-53b01966` |
| Volume | `dl-<id8>-<slug>-<hash>` | `dl-53b01966-data-1a2b` |
| Built image | `docklift-<project8>-<service slug>:<deployment8>` | `docklift-53b01966-app:9f8e7d6c` |

`slug` (`DockerSlug`): lowercase, runs of other characters become `-`, trimmed, at most 24 characters for projects and 20 for services, `app` when empty. `id8` is the first 8 hex characters of the project UUID with dashes stripped. `ComposeProjectAliases` also returns the bare project UUID (the pre-slug compose project name) so older deployments can still be torn down.

**Control plane** (`docklift-*`) is on `docklift_network`. **User apps** are on their own `dl-net-*` network only; `docklift-nginx-proxy` is attached after `up`.

## Project Create Guards

-   Name required; source must be a valid GitHub URL **or** a ZIP upload; no empty shell projects.
-   Env keys: `deploy.IsValidEnvKey` (`^[A-Za-z_][A-Za-z0-9_]*$`, at most 128 characters); `(project_id, service_name, key)` is unique (migration `20260725160000_env_service_scope`) and startup removes duplicate rows left by older installs (`DedupeEnvVariables`).
-   Env scope: `service_name = ""` is shared; a service-scoped key overrides the shared key (`EnvForService`).

## Troubleshooting Deployments

-   **Build fails**: read the summarized error at the end of the UI log; the raw output is above it.
-   **"contains a symlink that points outside the project directory"**: Railpack build; remove the link or replace it with a regular file.
-   **"Service name ... is reserved"**: rename the directory that holds the Dockerfile; names may not start with `docklift`.
-   **Container exited**: the app crashed; `docker logs <container>`.
-   **No host port / cannot open SERVER_IP:55xx**: host publish is off by default. Enable **Publish host ports** (pool `PORT_RANGE_START`-`PORT_RANGE_END`, default 5500-5600) or use a domain via nginx-proxy.
-   **502 via domain**: check `docklift-nginx-proxy` is on `dl-net-<id8>` and the container listens on `internal_port`.
-   **409 on deploy/delete**: a deployment is already in flight. `POST /api/deployments/:projectId/cancel` first.
-   **Stuck `in_progress` / `building` after a backend restart**: `Runtime.RecoverOnBoot` (startup hook `deployment-recovery`) marks every `in_progress` deployment failed with "interrupted by restart" and re-syncs `building` projects from their containers (`error` when none runs).
-   **Domains answer 502 after the proxy container was recreated**: `ReattachProxyOnBoot` (startup hook `proxy-reattach`) re-attaches the proxy to every project network; a project that was never deployed or is stopped has no network and is skipped.
-   **Data lost after redeploy**: no persistent volume for that path; configure one in Storage.

## Development

```bash
cd server && go vet ./... && go test -count=1 ./...    # or: make test
DOCKLIFT_DOCKER=1 make test contract                    # Docker-gated tests (need a daemon)
```

The `deploy`, `api/deployments`, `api/projects` and `dockerx` packages are unit-tested against fakes (`DockerOps`, `dockerx.Runner`, `deploy.CommandRunner`); `integration_test.go` files run against a real daemon only with `DOCKLIFT_DOCKER=1`.
