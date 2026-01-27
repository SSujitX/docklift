---
name: Deployment System
description: Comprehensive guide to how Docklift deploys and manages user applications.
---

# Deployment System Guide

This guide details the lifecycle of a deployment in Docklift, from source code to running container.

## Core Services

-   **`projects.ts`**: Manages Project CRUD and triggers deployments.
-   **`deployments.ts`**: Manages deployment history and logs.
-   **`docker.ts`**: wrapper for `dockerode` to control containers.
-   **`compose.ts`**: Generates `docker-compose.yml` files dynamically.
-   **`git.ts`**: Handles cloning and pulling repositories.

## Deployment Lifecycle

1.  **Trigger**:
    -   Manual (UI button) or Webhook (GitHub push).
    -   `POST /api/deployments/:projectId/deploy`.

2.  **Preparation**:
    -   A unique deployment ID is created (`status: queued`).
    -   Source code is prepared:
        -   **GitHub**: `git clone` or `git pull` into `deployments/<projectId>/source`.
        -   **Upload**: Unzip file into `deployments/<projectId>/source`.

3.  **Configuration Generation**:
    -   `compose.scanDockerfiles()` searches for Dockerfiles.
    -   `compose.generateCompose()` creates a `docker-compose.yml` in the project root.
    -   **Env Injection**: Environment variables (Build Args & Runtime) are injected into the compose file.

4.  **Build & Run**:
    -   Command: `docker compose up -d --build`
    -   Output is streamed via SSE (Server-Sent Events) to the frontend console.

5.  **Verification**:
    -   System checks if containers are running.
    -   Updates `Project` status to `running`.
    -   Updates `Deployment` status to `success`.

## File Structure (Per Project)

```
deployments/
  <projectId>/
    source/           # Application Source Code
    docker-compose.yml # Generated Config
    .env              # Runtime Environment Variables
```

## Naming Conventions

-   **Project Containers**: `docklift_<projectId>_<serviceName>`
-   **Networks**: All containers (and Docklift itself) must join `docklift_network`.

## Troubleshooting Deployments

-   **Build Fails**: Check `docker compose build` logs in the UI. Common issues: missing Dockerfile, build args errors.
-   **Container Exited**: The app might have crashed. Check logs via `docker logs <container_name>`.
-   **Port Conflicts**: Docklift auto-assigns internal ports (3001+), but ensure the App *listens* on the port defined in `EXPOSE` or environment.
