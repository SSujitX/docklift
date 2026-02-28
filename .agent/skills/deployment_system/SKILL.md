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
-   **`compose.ts`**: Generates `docker-compose.yml` files dynamically (scans Dockerfiles, detects ports).
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

3.  **Configuration Generation**:
    -   `compose.scanDockerfiles()` searches for Dockerfiles.
    -   `compose.generateCompose()` creates a `docker-compose.yml` in the project root.
    -   **Env Injection**: Environment variables (Build Args & Runtime) are injected into the compose file.
    -   **Middleware Bypass**: `middlewareBypass.ts` patches Next.js `allowedHosts` if detected.

4.  **Build & Run**:
    -   Command: `docker compose -p <projectId> up -d --build`
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
  <projectId>/
    source/           # Application Source Code
    docker-compose.yml # Generated Config
    .env              # Runtime Environment Variables
```

## Naming Conventions

-   **Project Containers**: `dl_<shortId>_<serviceName>` (shortId = first 8 chars of projectId)
-   **Networks**: All containers (and Docklift itself) must join `docklift_network`.

## Troubleshooting Deployments

-   **Build Fails**: Check `docker compose build` logs in the UI. Common issues: missing Dockerfile, build args errors.
-   **Container Exited**: The app might have crashed. Check logs via `docker logs <container_name>`.
-   **Port Conflicts**: Docklift auto-assigns internal ports (3001+), but ensure the App *listens* on the port defined in `EXPOSE` or environment.
-   **Stuck "in_progress"**: If the client disconnected during deploy, the disconnection guard ensures status still updates. If truly stuck, check backend logs.
