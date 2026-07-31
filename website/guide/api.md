# API Reference

The dashboard talks to the backend through the gateway at **`http://SERVER_IP:8080`**. Browser clients use same-origin `/api/...` paths. In local development the API listens on **`http://localhost:8000`**.

Most endpoints require an authenticated admin session (cookie/JWT). Destructive operations may also require **step-up password** verification.

Health (no project auth):

```bash
curl -s http://SERVER_IP:8080/api/health
```

## Projects

| Method | Endpoint | Description |
|--------|----------|-------------|
| `GET` | `/api/projects` | List all projects |
| `GET` | `/api/projects/:id` | Get project details |
| `POST` | `/api/projects` | Create project |
| `PATCH` | `/api/projects/:id` | Update project |
| `DELETE` | `/api/projects/:id` | Delete project |
| `GET` | `/api/projects/:id/build/detect` | Detect build / services |
| `GET` | `/api/projects/:id/env` | List env vars |
| `POST` | `/api/projects/:id/env` | Create env var |
| `POST` | `/api/projects/:id/env/bulk` | Bulk upsert env |
| `DELETE` | `/api/projects/:id/env/:envId` | Delete env var |
| `GET` | `/api/projects/:id/storage` | List storage mounts |
| `POST` | `/api/projects/:id/storage` | Add storage mount |
| `DELETE` | `/api/projects/:id/storage/:storageId` | Remove storage mount |
| `GET` | `/api/projects/:id/auto-deploy` | Auto-deploy status |
| `PATCH` | `/api/projects/:id/auto-deploy` | Enable/disable auto-deploy |

## Deployments

| Method | Endpoint | Description |
|--------|----------|-------------|
| `POST` | `/api/deployments/:projectId/deploy` | Deploy (streams logs) |
| `POST` | `/api/deployments/:projectId/redeploy` | Rebuild and redeploy |
| `POST` | `/api/deployments/:projectId/stop` | Stop containers |
| `POST` | `/api/deployments/:projectId/restart` | Restart containers |
| `POST` | `/api/deployments/:projectId/cancel` | Cancel in-flight deploy |
| `POST` | `/api/deployments/:projectId/rollback` | Restore previous success (step-up; streams logs) |
| `GET` | `/api/deployments/:projectId` | Deployment info / history |
| `GET` | `/api/deployments/:projectId/logs` | Deployment logs |
| `GET` | `/api/deployments/:projectId/services` | Service list |
| `PUT` | `/api/deployments/:projectId/services/:serviceId` | Update service settings |

## Databases

| Method | Endpoint | Description |
|--------|----------|-------------|
| `GET` | `/api/databases/engines` | Supported engines |
| `GET` | `/api/databases` | List managed databases |
| `POST` | `/api/databases` | Create database |
| `GET` | `/api/databases/:id/connection` | Connection URL panel data |
| `GET` | `/api/databases/:id/links` | Linked apps |
| `POST` | `/api/databases/:id/links` | Link to project/service |
| `DELETE` | `/api/databases/:id/links/:linkId` | Unlink |
| `GET` | `/api/databases/links/by-app/:appProjectId` | Links for an app |

## System

| Method | Endpoint | Description |
|--------|----------|-------------|
| `GET` | `/api/system/stats` | Full system stats |
| `GET` | `/api/system/quick` | Lightweight stats |
| `GET` | `/api/system/ip` | Server public IP |
| `GET` | `/api/system/version` | Running version info |
| `GET` | `/api/system/logs/:service` | Platform log stream helper |
| `POST` | `/api/system/purge` | Unused Docklift images (keep-2) + full BuildKit wipe (step-up) |
| `POST` | `/api/system/reset` | Restart Docklift services |
| `POST` | `/api/system/reboot` | Reboot server (step-up) |
| `POST` | `/api/system/execute` | Execute shell command (terminal / gated) |
| `POST` | `/api/system/upgrade` | Trigger Docklift upgrade flow |
| `POST` | `/api/system/update-system` | Host package update flow |

## GitHub

| Method | Endpoint | Description |
|--------|----------|-------------|
| `GET` | `/api/github/app-status` | App status |
| `POST` | `/api/github/manifest` | Create app via manifest |
| `POST` | `/api/github/install-session` | Install session |
| `POST` | `/api/github/check-installation` | Poll installation |
| `GET` | `/api/github/repos` | List repositories |
| `GET` | `/api/github/branches` | List branches (`?repo=owner/name&type=public\|private`) |
| `GET` | `/api/github/tags` | List tags newest-first (`?repo=owner/name&type=public\|private`) |
| `POST` | `/api/github/disconnect` | Disconnect app |
| `POST` | `/api/github/webhook` | Webhook receiver |

## Auth, domains, ports, files, backup

| Method | Endpoint | Description |
|--------|----------|-------------|
| `GET` | `/api/auth/status` | Setup / auth status |
| `POST` | `/api/auth/login` | Login |
| `POST` | `/api/auth/register` | First admin (setup) |
| `GET` | `/api/auth/me` | Current user |
| `PATCH` | `/api/auth/profile` | Update profile |
| `POST` | `/api/auth/change-password` | Change password |
| `GET` | `/api/domains` | Panel/service domain configs |
| `POST` | `/api/domains` | Add domain |
| `DELETE` | `/api/domains/:domain` | Remove domain |
| `GET` | `/api/ports` | Host port pool + projects running without a host port |
| `GET` | `/api/files/:projectId` | File tree |
| `GET` | `/api/files/:projectId/content` | Read file |
| `PUT` | `/api/files/:projectId/content` | Write file |
| `POST` | `/api/backup/create` | Create backup |
| `POST` | `/api/backup/restore/:filename` | Restore (step-up / locks) |

This is a practical operator map, not an OpenAPI dump. Paths and payloads evolve with releases — inspect the running UI network tab or backend routes under `backend/src/routes/` when integrating.
