---
name: Managed Databases
description: Coolify/Dokploy-style managed databases with Dokku-style app linking (Go backend).
---

# Managed Databases

Docklift databases are first-class managed projects (`project_type=database`, `source_type=managed`, `db_engine` set). They are **not** Git/ZIP apps: there is no checkout, no build and no Dockerfile. They run through the same deploy pipeline as apps, but the pipeline pulls an upstream image instead of building one.

Code map:

| Path | Role |
|------|------|
| `server/internal/deploy/dbengines.go` | Engine catalog, image/tag resolution, credentials, connection URLs, service marker, data mount rules |
| `server/internal/deploy/hubtags.go` | Live Docker Hub tag catalog (`HubTags`), tag filters |
| `server/internal/deploy/dblinks.go` | Linking: env ownership, network attach/detach, re-apply after deploys |
| `server/internal/api/databases/databases.go` | `/api/databases` routes (engines, list, create, connection, links) |
| `server/internal/api/deployments/pipeline.go` | `prepareManaged`: service row, image pull, volume mount fix, runtime compose |
| `server/internal/api/projects/env.go` | Locks credential env keys of a database project |
| `server/internal/api/projects/delete.go` | Delete: Docker teardown, then link cleanup, then row |

## Engines

Defined in `server/internal/deploy/dbengines.go` (`GetEngine`, `ListEngines`, `EngineIDs`):

| id | New-create default | Bare `[managed:id]` pin (`LegacyManagedImages`) | Container port | Data mount | Env key injected into apps |
|----|--------------------|-------------------------------------------------|----------------|------------|----------------------------|
| postgres | `postgres:18-alpine` | `postgres:16-alpine` | 5432 | `/var/lib/postgresql/data` (<= 17) or `/var/lib/postgresql` (18+, `latest`) | `DATABASE_URL` |
| mysql | `mysql:8.4` | `mysql:8.4` | 3306 | `/var/lib/mysql` | `DATABASE_URL` |
| mariadb | `mariadb:11` | `mariadb:11` | 3306 | `/var/lib/mysql` | `DATABASE_URL` |
| redis | `redis:8-alpine` | `redis:7-alpine` | 6379 | `/data` | `REDIS_URL` |
| mongodb | `mongo:8` | `mongo:7` | 27017 | `/data/db` | `MONGODB_URI` |

Every engine's service is named `db`, so the container is `dl_<project slug>_<id8>_db`.

-   **`GET /api/databases/engines`** returns the catalog with **live tags from Docker Hub** (`HubTags.ListEnginesWithLiveVersions`). Hub is queried at most 3 pages of 100 tags per engine, cached 6 h (5 min when the page set was incomplete), 10 s request timeout. When Hub is unreachable or yields nothing usable, the static `Versions` of the engine are returned.
-   Tags are filtered to operator-friendly majors (`MatchesEngineTagPattern`): no pre-releases (`alpha`, `beta`, `rc`, `preview`, `nightly`, `dev`), no distro or OS variants (`bookworm`, `slim`, `ubi`, ...); postgres/redis `N[.M][-alpine]`, mysql `N.M[-oracle]`, mariadb/mongodb `N[.M]`, plus `latest`. Minor tags are dropped when the major line exists (`16` beats `16.14`); sorted newest first, alpine before Debian, `latest` last; at most 36 versions.
-   `POST /api/databases` accepts `version` (or `tag`, or `image`; bare tag or `<repo>:<tag>`). `ResolveEngineImage` accepts a tag from the live list, the static list, or anything that matches the engine's tag pattern (so creation works while the live list is briefly stale); otherwise 400 "Invalid version for ...". The full image is stored on the service as `dockerfile_path = [managed:<engine>|<repo>:<tag>]` (`ManagedServiceMarker`).
-   Deploy pulls the **stored** image (`ImageForManagedService`); a bare `[managed:<engine>]` marker maps to `LegacyManagedImages` (never a silent major bump).
-   **Postgres mount is version-aware** (`VolumeMountForEngine`): <= 17 uses `/var/lib/postgresql/data`; 18+ and `latest` use `/var/lib/postgresql` (the official image contract). Create and deploy sync `persistent_volumes.mount_path`; a redeploy logs a warning when it has to change the old `/data` mount. Data that already landed on an anonymous volume under the old mount cannot be migrated: recreate and restore from backup.
-   **Credentials** are generated at create: user `docklift`, password 24 random bytes in base64url (`GenerateDatabasePassword`), database named after the project (slug of up to 16 characters, `-` becomes `_`; Redis uses only the password). Stored as project env (`EngineRuntimeEnv`: `POSTGRES_USER/PASSWORD/DB`, `MYSQL_USER/PASSWORD/DATABASE/ROOT_PASSWORD`, `REDIS_PASSWORD`, `MONGO_INITDB_ROOT_USERNAME/PASSWORD/DATABASE`); any key containing `password` is `is_secret`. Redis gets `command: redis-server --requirepass <password> --appendonly yes` in the runtime compose.
-   `publish_host_port` defaults to **false** and the service port is `null`; prefer linking. No `PORT` env is injected into database containers (`NoPortEnv`).

## API

All routes are under `/api/databases` and need authentication.

| Method | Path | Purpose |
|--------|------|---------|
| GET | `/api/databases/engines` | Catalog (+ live Hub `versions`, `imageRepo`) |
| GET / POST | `/api/databases` | List managed database projects / create (201, credentials returned once, `Cache-Control: no-store`) |
| GET | `/api/databases/:id/connection` | Host, port, credentials, connection URL, `internal_only`, note |
| GET / POST | `/api/databases/:id/links` | List / create links |
| DELETE | `/api/databases/:id/links/:linkId` | Unlink |
| GET | `/api/databases/links/by-app/:appProjectId` | Links on an app project |

Start, stop, redeploy, cancel and delete are **not** routes of this group; they use `/api/deployments/:id/...` and `DELETE /api/projects/:id` like any project (rollback is refused with 400 for databases).

### Create (`POST /api/databases`)

Body: `name` (1-120 characters), `engine`, optional `version`. Steps (`create` in `databases.go`): resolve the image, generate credentials, create the project row (`status pending`, `build_type dockerfile`, `internal_port` = engine port), write `deployments/<id>/README.docklift-db.md`, create the `db` service row (container name from `ServiceContainerName`), the credential env rows, the data `persistent_volumes` row (`dl-<id8>-data-<hash>`, display name `data`) and the Docker volume (`docker volume create` with labels `com.docker.compose.project`, `com.docklift.project`, `com.docklift.role=managed-database`). Any failure rolls the project row and directory back; Docker unreachable answers 503. **Create does not start the container**: the dashboard triggers a deploy next.

### Deploy (`prepareManaged` in the pipeline)

-   Pulls the image with `docker pull` (`dockerx.PullImage`): output streams into the deployment log, 10 minute hard timeout, cancellable. The pull runs as the project's registered command (`Tracker.StartProcess`), so **Cancel kills it**. Pulls never run in the request path synchronously; they must not block the API.
-   The project lock is owned by the deployment id (`Tracker.Acquire`), so a cancelled pull cannot unlock a newer deploy.
-   Writes the runtime compose with one service (`image`, `container_name`, `command` for Redis, data volume as an external named volume) and runs `docker compose -f <runtime compose> -p <project> up -d --remove-orphans` from `deployments/.docklift/.compose-cwd`.
-   Edge proxy attach after `up` only warns for databases ("OK for internal databases"); then `ReapplyDatabaseLinksForDatabase` refreshes injected URLs and re-attaches the database to every linked app network.
-   Deployments tab streams the pull; `?tab=deployments` links from `/databases` show it.

## Linking

`database_links` rows: database project to app project, optional `service_name` (`""` = shared env for all services), `env_key`.

`POST /api/databases/:id/links` body: `app_project_id` (required, must not be a database), optional `service_name` (must exist on the app), `env_key` (default is the engine's key; `^[A-Za-z_][A-Za-z0-9_]*$`, at most 64 characters), `overwrite`.

On link (in this order):

1. Requires working credentials (400 "deploy the database first") and database `status` **running or degraded** (409 otherwise).
2. `AssertEnvKeyAvailable`: one owner per `(app_project_id, service_name, env_key)` (unique index `database_links_app_project_id_service_name_env_key_key` plus this gate). Another database already owns the key: 409. An existing unlinked env value with the same key: 409 unless `overwrite=true`.
3. Link row (`UpsertLink`), then `UpsertLinkedEnv`: the connection URL is written as a **runtime secret** env var (`is_runtime`, `is_secret`, not a build arg), scoped or shared.
4. `EnsureDBOnAppNetwork` attaches the database container to the app's `dl-net-<id8>` network. Failure does not fail the request: `network_attached: false` and a note.
5. The operator **redeploys the app** so containers pick up the env; nothing is redeployed automatically.

URLs use the database container name as host (Docker DNS on the shared project network):

| Engine | URL |
|--------|-----|
| postgres | `postgresql://user:pass@dl_..._db:5432/db` |
| mysql / mariadb | `mysql://user:pass@dl_..._db:3306/db` |
| redis | `redis://:pass@dl_..._db:6379` |
| mongodb | `mongodb://user:pass@dl_..._db:27017/db?authSource=admin` |

User, password and database are `encodeURIComponent`-encoded.

After any DB or app deploy, links are re-applied (`ReapplyDatabaseLinksForDatabase` / `ReapplyDatabaseLinksForApp`: network join + URL refresh). `DELETE .../links/:linkId` (`UnlinkDatabaseLink`) removes the link, then the env var if no other link owns that key, then the network attachment if no link between the two projects remains.

## UI

-   `/databases`: list. `/databases/new`: engine picker + create & deploy.
-   Project detail (database): only the **Overview** (Connection + Linked apps, `ManagedDatabasePanel`), **Deployments** and **Logs** content; no Environment / Build / Storage / Source / Domains tabs. Credentials are shown on Overview and locked; recreate the database to rotate them.
-   Project detail (app): **Attach database** (`AttachDatabasePanel`): the scope picker appears only on **All services** and is locked to the current service in the service workspace.
-   Logs: same LogViewer as apps (service runtime logs).

## Rules

-   One owner per `(app_project_id, service_name, env_key)`.
-   Credential env keys of a database project are locked (`ManagedCredentialEnvKeys`): single edit and bulk import return 400 "Managed database credentials are set at create time ... recreate the database to rotate." Official images apply them only on first volume init.
-   App stop / cancel / delete must `DisconnectLinkedDatabasesFromApp` before `compose down`; on a failed teardown, `ReapplyDatabaseLinksForApp` re-attaches (same as proxy restore).
-   Database delete: Docker teardown first (`compose down --remove-orphans --volumes`, verified), then volumes, then `CleanupLinksForDatabaseProject` (strips peer apps' env and networks), then the row.
-   Project delete removes only Docklift-built `docklift-<id8>-*` images; the upstream database image (`postgres:...`) is never removed.
-   Prefer linking over advertising raw `IP:port`. A published host port comes from the same pool as apps (`PORT_RANGE_START`-`PORT_RANGE_END`) and shows in the connection note.
-   A service name that starts with `docklift` is reserved for scanned app services; database service names are fixed (`db`).

## Testing

```bash
cd server && go test ./internal/deploy/... ./internal/api/databases/... -count=1
DOCKLIFT_DOCKER=1 go test ./internal/api/databases/... -count=1   # integration against a real daemon
```
