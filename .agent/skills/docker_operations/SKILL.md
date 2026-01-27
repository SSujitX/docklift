---
name: Docker Operations
description: Guide for managing and debugging Docklift containers and deployments.
---

# Docker Operations Guide

Docklift is a container management platform, so understanding the underlying Docker operations is crucial.

## Core Containers

The main Docklift platform consists of:
-   `docklift-backend`: The API server.
-   `docklift-frontend`: The web UI.
-   `docklift-nginx`: The main entry point (port 8080) routing to frontend and backend.
-   `docklift-nginx-proxy`: Handles custom domain routing (port 80).

## viewing Logs

To view logs for the infrastructure:

```bash
# Backend logs
docker logs docklift-backend -f

# Frontend logs
docker logs docklift-frontend -f

# Nginx logs
docker logs docklift-nginx -f
```

## inspecting Deployed User Projects

User deployments follow the naming convention: `docklift_<projectId>_<serviceName>` or similar.

To see all running containers related to Docklift projects:
```bash
docker ps --filter "name=docklift_"
```

## Debugging Deployments

1.  **Check Build Logs**: In the UI, check the deployment logs.
2.  **Inspect Container**:
    ```bash
    docker inspect <container_name_or_id>
    ```
3.  **View Container Logs**:
    ```bash
    docker logs <container_name_or_id>
    ```
4.  **Enter Container Shell**:
    ```bash
    docker exec -it <container_name_or_id> /bin/sh
    # or /bin/bash if available
    ```

## Network

All Docklift containers + user deployments should be on the `docklift_network` bridge network to communicate.

## Volume Management

-   **Data**: `./data` maps to `/app/data` in backend.
-   **Deployments**: `./deployments` maps to `/deployments` in backend.

## Pruning

To clean up unused resources (careful in production!):
```bash
docker system prune -a
```
Docklift also has a built-in purge API.
