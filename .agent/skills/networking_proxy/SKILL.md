---
name: Networking & Proxy
description: Guide to Docklift's networking, custom domains, and Nginx reverse proxy.
---

# Networking & Proxy Guide

Docklift uses a dedicated Nginx container (`docklift-nginx-proxy`) to handle routing for user applications.

## Architecture

-   **Main Nginx**: Routes standard traffic (dashboard port 8080).
-   **Proxy Nginx**: Routes **Tenant/Project traffic** (port 80/443).
-   **Docker Network**: `docklift_network` (bridge).

## How Routing Works

1.  **Project Creation**:
    -   User assigns a `domain` (e.g., `app.example.com`) or port.
    -   Backend assigns an internal container port.

2.  **Config Generation**:
    -   Code: `backend/src/services/nginx.ts`
    -   Generates a config file in `nginx-proxy/conf.d/<projectId>.conf`.

3.  **Nginx Reload**:
    -   Backend runs `docker exec docklift-nginx-proxy nginx -s reload`.
    -   Nginx loads the new config.

## Config Template

A typical generated config looks like:
```nginx
server {
    listen 80;
    server_name app.example.com;

    location / {
        proxy_pass http://docklift_<projectId>_<serviceName>:<internal_port>;
        proxy_set_header Host $host;
        # ... standard proxy headers
    }
}
```

## Troubleshooting

-   **502 Bad Gateway**:
    -   The application container is not running.
    -   The application is not listening on the expected port.
    -   The container is not connected to `docklift_network`.
-   **404 Not Found**:
    -   The domain does not match `server_name` in any config.
    -   DNS is not pointing to the Docklift server IP.
-   **Config Errors**:
    -   Check valid syntax: `docker exec docklift-nginx-proxy nginx -t`
