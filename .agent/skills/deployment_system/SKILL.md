---
name: Deployment System
description: Comprehensive guide to how Docklift builds and deploys user applications.
---

# Deployment System Guide

This guide details the lifecycle of a deployment in Docklift, from source code to running container.

## Core Services

-   **`routes/projects.ts`**: Project CRUD, build settings, persistent volumes, domain assignment.
-   **`routes/deployments.ts`**: Deployment history plus the streaming `deploy` / `stop` / `restart` / `redeploy` / `cancel` handlers.
-   **`lib/runCompose.ts`**: Shared `docker` spawn with mandatory `error` + `close` handlers (never leave unhandled spawn errors).
-   **`lib/deploymentState.ts`**: In-memory “deploying” lock per project (`isProjectDeploying` / `setProjectDeploying`).
-   **`lib/projectStatusSync.ts`**: Inspect **all** service containers; aggregate project status.
-   **`lib/deploymentRecovery.ts`**: On boot, mark stale `in_progress` failed and stuck `building` projects corrected.
-   **`lib/portAllocation.ts`**: Transactional host-port claim (no upsert steal).
-   **`services/docker.ts`**: `dockerode` wrapper for inspect/logs.
-   **`services/buildResolver.ts`**: Decides *what* to build (Dockerfile vs Railpack, base directory, service list).
-   **`services/buildRunner.ts`**: Actually builds the image and summarizes failures.
-   **`services/compose.ts`**: Scans Dockerfiles (**dedupes colliding service names** with path hash) and writes runtime Compose.
-   **`services/git.ts`**: Clone / pull + `scrubOriginRemote`.
-   **`lib/naming.ts`**: Compose project, container, image, and `storageVolumeComposeKey` names.

## Deployment Lifecycle

1.  **Trigger**
    -   Manual (UI button) or GitHub push webhook.
    -   `POST /api/deployments/:projectId/deploy` (streaming response).
    -   **Serialized per project**: memory lock **and** DB `deployment.status = in_progress` → **409**.
        `stop` / `restart` also 409 while deploying; caller must cancel first.
    -   **Delete project** while deploying → **409** (`projects.ts` + `isProjectDeploying`).

2.  **Preparation**
    -   A deployment row is created (`status: in_progress`).
    -   Source is prepared **in place** at `deployments/<projectId>/`:
        -   **GitHub**: `git clone`, or `git fetch` + `reset --hard` + `clean` for an existing checkout.
        -   **Upload**: ZIP extracted via `lib/safeUnzip.ts` (rejects traversal entries).
    -   Temp upload files are always removed in a `finally` block, even when extraction throws.

    **Git Token Security** (GitHub projects):
    -   Installation token is refreshed just-in-time via `getInstallationToken()`.
    -   After pull/clone, origin is scrubbed (`scrubOriginRemote`) and verified clean.
    -   **If scrub fails, the deployment/create fails** (do not continue with credentials in `.git/config`).
    -   Shell work uses `spawn` / `spawnSync` with argument arrays — never string interpolation.

3.  **Build Resolution**
    -   `build_type` on the project is `auto` (default), `dockerfile`, or `railpack`.
    -   **`auto`** prefers a repository `Dockerfile` and falls back to Railpack when none is found.
    -   `base_directory` (default `.`) scopes detection to a monorepo subdirectory. It is resolved
        with `resolveProjectPath()`, which rejects any path escaping the deployment root.
    -   Dockerfile mode may also pin an explicit `dockerfile_path`.

4.  **Image Build** (`buildServiceImage`)
    -   Every deployment builds an explicitly tagged image, prefixed by the compose project name.
    -   **Dockerfile**: `docker build` with validated build args — `validateDockerBuildArgs()` only
        passes args the Dockerfile actually declares via `ARG`, so unrelated env vars are not leaked
        into build layers.
    -   **Railpack**: `docker buildx build` driven by a generated Railpack plan JSON. `PORT` is passed
        both as `--build-arg` and as `--secret id=PORT,env=PORT`, because Railpack reads it as a secret.
    -   Cancellation kills the detached process group, so compose/buildkit children die too.
    -   Failures go through `summarizeBuildFailure()`, which turns common toolchain noise into one
        actionable line (e.g. an out-of-sync `package-lock.json` on `npm ci`).

5.  **Run**
    -   Runtime Compose is generated at `deployments/.docklift/<projectId>/compose.yml`.
        **Source files are never patched** — no repository `Dockerfile` or `docker-compose.yml` is
        rewritten, which means user-committed compose files stay intact.
    -   Command: `docker compose -f <runtime-compose> -p <composeProject> up -d --remove-orphans`
    -   `--remove-orphans` plus per-alias `down` cleans up containers left behind by renamed or
        removed services (and by the legacy UUID-based compose project name).
    -   Runtime env vars and configured named volumes are attached to each service.
    -   Output streams to the UI console over SSE.

6.  **Verification**
    -   `syncProjectStatusFromContainers()` updates each service from Docker, then aggregates project status
        (`running` only if all running; `error` if any error; else mixed rules).
    -   Deploy lock (`setProjectDeploying`) is held through status write **and** nginx/SSL activation.
    -   Final status write uses `updateMany` with `status ≠ cancelled` so cancel cannot be overwritten as success.
    -   `failDeploymentState` likewise never overwrites `cancelled`.

7.  **Cancel (anytime)**
    -   Product rule: cancel tears everything down so the user can start fresh.
    -   Marks in-progress (and latest success/failed) deployments `cancelled`, `compose down`, project/services `stopped`.
    -   Stop/cancel only mark `stopped` when compose exit code is **0**.

## Build Types at a Glance

| `build_type` | Behaviour |
|--------------|-----------|
| `auto` | Repository `Dockerfile` if present, else Railpack detection |
| `dockerfile` | Requires a Dockerfile; fails loudly instead of silently falling back |
| `railpack` | Always Railpack, even when a Dockerfile exists |

`detectManifests()` reports which framework manifests were seen (e.g. `package.json`,
`requirements.txt`, `pyproject.toml`), which is what the UI shows as the detected stack.

## Persistent Storage

Rebuilds replace containers, so anything written to a container's filesystem is lost unless mounted.

-   Configured per service in the project **Storage** tab → `PersistentVolume` rows
    (`service_name`, `mount_path`, `display_name`).
-   Docker volume name: `dl-<shortId>-<slug>-<hash(label)>` so labels like `a-b` vs `a_b` never collide.
    Labelled `com.docklift.project=<projectId>` for cleanup.
-   Compose volume keys use `storageVolumeComposeKey(service, index, volumeName)`.
-   Volumes are created as **external** named volumes and referenced by the generated compose file,
    so `docker compose down` never deletes user data.
-   They are removed only when the project is deleted (or when a failed create is rolled back).
-   External databases reached through `DATABASE_URL` need no volume — only in-container state
    (SQLite files, uploads, caches) does.

## Streaming Safety

All streaming endpoints (`deploy`, `stop`, `restart`, `redeploy`) use a `writeLog` helper with a
**disconnection guard**:

```typescript
const writeLog = (text: string) => {
  try { if (!res.writableEnded) res.write(text); } catch {}
  logs.push(text);
};
```

This prevents crashes when the client disconnects mid-stream and guarantees the deployment status
is persisted regardless of connection state.

## File Structure (Per Project)

```
deployments/
  <projectId>/            # Application source (clone/upload root; never modified by Docklift)
  .docklift/
    <projectId>/
      compose.yml         # Generated runtime state
      *-railpack-*.json   # Generated Railpack build plans
    .env                  # Runtime environment variables (when used)
```

## Naming Conventions

Always use `lib/naming.ts` — never hand-build these strings.

| Thing | Format | Example |
|-------|--------|---------|
| Compose project (`-p`) | `dl-<slug>-<shortId>` | `dl-python-smoke-53b01966` |
| Container | `dl_<slug>_<shortId>_<svc>` | `dl_python-smoke_53b01966_app` |
| Volume | `dl-<shortId>-<slug>` | `dl-53b01966-data` |

`shortId` is the first 8 hex chars of the project UUID (dashes stripped). `composeProjectAliases()`
also returns the bare project UUID, the legacy compose project name, so older deployments can still
be torn down.

Every container (and Docklift itself) joins `docklift_network`.

## Troubleshooting Deployments

-   **Build fails**: read the summarized error at the end of the UI log; the raw output is above it.
-   **Container exited**: the app crashed — `docker logs <container>`.
-   **Port conflicts**: host ports come from the `5500`–`5600` pool (`PORT_RANGE_START/END`). The app
    must listen on the port Docklift injects as `PORT` / the service's `internal_port`.
-   **409 on deploy/delete**: a deployment is already in flight. `POST /:projectId/cancel` first.
-   **Stuck `in_progress` / `building` after backend restart**: `recoverDeploymentStateOnBoot()` marks
    interrupted deployments failed and reconciles project status from containers.
-   **Data lost after redeploy**: no persistent volume for that path — configure one in Storage.
