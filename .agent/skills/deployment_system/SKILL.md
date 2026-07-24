---
name: Deployment System
description: Comprehensive guide to how Docklift deploys and manages user applications.
---

# Deployment System Guide

This guide details the lifecycle of a deployment in Docklift, from source code to running container.

## Core Services

-   **`projects.ts`**: Manages Project CRUD and triggers deployments.
-   **`deployments.ts`**: Manages deployment history, logs, and streaming deploy/stop/restart/redeploy.
-   **`docker.ts`**: Wrapper for `dockerode` to control containers and stream compose operations.
-   **`buildResolver.ts` / `buildRunner.ts`**: Select Dockerfile or Railpack and build tagged images.
-   **`compose.ts`**: Scans Dockerfiles and writes DockLift-owned runtime Compose state.
-   **`git.ts`**: Handles cloning (`git clone`) and pulling (`fetch + hard reset + clean`) repositories.

## Deployment Lifecycle

1.  **Trigger**:
    -   Manual (UI button) or Webhook (GitHub push).
    -   `POST /api/deployments/:projectId/deploy`.

2.  **Preparation**:
    -   A unique deployment ID is created (`status: queued`).
    -   Source code is prepared:
        -   **GitHub**: `git clone` or `git fetch + reset --hard` into `deployments/<projectId>/source`.
        -   **Upload**: Unzip file into `deployments/<projectId>/source`.
    -   Temp upload files are always cleaned up via `try/finally` (even on extraction error).

    **Git Token Security** (GitHub projects):
    -   Installation token is refreshed just-in-time via `getInstallationToken()`.
    -   Token is set in git remote URL, used for pull, then **immediately scrubbed** in a `finally` block.
    -   If pull fails, token is still removed from the remote URL (prevents credential leakage).
    -   Uses `spawnSync()` with argument arrays for any shell commands (e.g., `docker rm -f`) — never string interpolation.

3.  **Build Resolution and Configuration**:
    -   Auto mode prefers repository Dockerfiles and falls back to Railpack.
    -   Dockerfile and Railpack may also be selected explicitly with a base directory.
    -   Each deployment builds a tagged image.
    -   Runtime Compose is written under `deployments/.docklift/<projectId>/compose.yml`; source files are never patched or overwritten.
    -   Runtime environment variables and configured external named volumes are attached to services.

4.  **Build & Run**:
    -   Command: `docker compose -f <runtime-compose> -p <projectName> up -d`
    -   Output is streamed via SSE (Server-Sent Events) to the frontend console.

5.  **Verification**:
    -   System checks if containers are running.
    -   Updates `Project` status to `running`.
    -   Updates `Deployment` status to `success`.

## Streaming Safety

All streaming endpoints (`deploy`, `stop`, `restart`, `redeploy`) use a `writeLog` helper with a **disconnection guard**:

```typescript
const writeLog = (text: string) => {
  try { if (!res.writableEnded) res.write(text); } catch {}
  logs.push(text);
};
```

This prevents server crashes if the client disconnects mid-stream and ensures the deployment status is always updated in the database regardless of client connection state.

## File Structure (Per Project)

```
deployments/
  <projectId>/        # Application source (clone/upload root; never modified by deployment generation)
  .docklift/
    <projectId>/
      compose.yml     # Generated runtime state
      *-railpack-*.json
    .env              # Runtime Environment Variables (when used)
```

## Naming Conventions

-   **Project Containers**: `dl_<shortId>_<serviceName>` (shortId = first 8 chars of projectId)
-   **Networks**: All containers (and Docklift itself) must join `docklift_network`.

## Troubleshooting Deployments

-   **Build Fails**: Check `docker compose build` logs in the UI. Common issues: missing Dockerfile, build args errors.
-   **Container Exited**: The app might have crashed. Check logs via `docker logs <container_name>`.
-   **Port Conflicts**: Docklift auto-assigns internal ports (3001+), but ensure the App *listens* on the port defined in `EXPOSE` or environment.
-   **Stuck "in_progress"**: If the client disconnected during deploy, the disconnection guard ensures status still updates. If truly stuck, check backend logs.
