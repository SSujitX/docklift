# Deployment

Docklift prefers your Dockerfile and automatically falls back to [Railpack](https://railpack.com/) when one is not present. Builds run through BuildKit; live logs stream to the browser over SSE.

## Actions

Redeploy, Restart, Stop, and Delete live on the dashboard list and under Workspace as **All-services actions**. They always affect every service in that one project — not a single app tab.

| Action | Effect |
|--------|--------|
| **Deploy** | Detect, build, and start a new image |
| **Stop** | Stop running containers gracefully |
| **Restart** | Restart without rebuilding the image |
| **Redeploy** | Rebuild from source and deploy a new version |
| **Cancel** | Abort an in-flight build; tears containers down for a fresh start |
| **Restore** | On Deployments history: reinstate a previous successful image set (step-up; no rebuild) |

Single-service projects skip the Workspace rail and show the same actions under the project title. Do not expect per-service redeploy from Env, Domains, Storage, or Logs — deploy still rebuilds the whole project.

## Deployment process

1. Pull latest code from GitHub (if applicable)
2. Use a repository Dockerfile, or let Railpack detect the framework
3. Build a tagged image (BuildKit secrets for marked build vars)
4. Write runtime compose under `deployments/.docklift/<id>/` on a per-project network
5. Start containers (host ports only if **Publish host ports** is enabled)
6. Attach nginx-proxy to the project network; update domain vhosts
7. Stream logs to the browser in real time

Partial fleets show status **degraded**. Past success/failed history is not rewritten when you cancel.

## Disk hygiene and Restore

After a **successful** deploy only:

1. Docklift stores `commit_sha` and per-service `image_tags` on that deployment row
2. Keeps **at most two** successful `docklift-<project>-<service>:*` tags (current + previous); older unused tags are removed
3. Prunes **unused** BuildKit cache (`docker builder prune -f`) so redeploys cannot unbounded-grow the root disk

Failed or cancelled deploys do **not** prune images or cache needed for the last good release. Managed database upstream images (`postgres:`, etc.) are never deleted.

On **Project → Deployments**, historical successful rows that still have stored image tags show **Restore**. That action requires your account password, resets git to the stored commit when present, rewrites runtime compose to those images, and runs `compose up` — it does not rebuild. If an image was already pruned, restore returns an error and you must redeploy from git.

Manual full BuildKit wipe plus unused `docklift-*` image cleanup across all Docklift projects is on the [System](./system.md#purge) page (**Purge**). It never runs host-wide `docker system prune`.

## Build modes

Configurable in project settings:

| Mode | Behaviour |
|------|-----------|
| **Auto** (default) | Uses a repository `Dockerfile` if present, otherwise Railpack |
| **Dockerfile** | Always your Dockerfile — fails instead of silently falling back |
| **Railpack** | Always Railpack, even if a Dockerfile exists |

Helpful settings for non-trivial repos:

- **Base directory** — build from a subdirectory (monorepos)
- **Dockerfile path** — point at a Dockerfile that is not at the repo root

Build-time variables are passed only when the Dockerfile declares them with `ARG` (or via BuildKit secrets when marked). Runtime secrets should not be baked into image layers.

## Multi-service projects

Dockerfile projects can contain multiple services:

```text
my-project/
├── api/
│   └── Dockerfile
├── frontend/
│   └── Dockerfile
└── worker/
    └── Dockerfile
```

- Use **All services** for deploy, build, source, and shared env
- Open one service for that app's endpoints, env, domains, storage, and runtime logs
- Deploy still rebuilds the whole project
- Single-service projects keep flat tabs (no Workspace rail)
- Railpack projects currently resolve to one application service

Prefer [custom domains](./domains.md). Host ports are optional — see [Port Management](./ports.md).

## First deploy checklist

1. **New Project** → GitHub or ZIP
2. Optional env vars; build mode **Auto**
3. **Deploy** → watch logs
4. Overview shows **Private by default** until you add a domain or enable Publish host ports + redeploy
5. Prefer a domain; raw `IP:port` exposes your origin server

## Runtime compose

Docklift writes Compose under `deployments/.docklift/<project-id>/`. A `docker-compose.yml` you committed yourself stays untouched.

App container naming:

- Image / compose prefix: `dl-<slug>-<8charId>`
- Container: `dl_<slug>_<8charId>_<service>`
