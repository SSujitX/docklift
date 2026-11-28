# API Reference

The Go backend serves both the dashboard and the API at **`http://SERVER_IP:8080`** (or your panel domain). In local development (`make dev`) the API listens on **`http://localhost:8000`** and the Vite dev server on `http://127.0.0.1:3600` proxies `/api` and `/ws` to it.

Everything the dashboard does goes through this API, so a script or an AI agent can do the same with an **API token**.

## The full endpoint map (OpenAPI)

Every route, with its path and query parameters, request body fields, whether it streams and whether it is a "dangerous" (step-up) action, is in an OpenAPI 3.1 document served by the server itself:

```bash
curl -s -H "Authorization: Bearer $DOCKLIFT_TOKEN" https://panel.example.com/api/openapi.json
```

In the dashboard, **API & MCP** (`/developer`) has a searchable endpoint browser built from the same document. The document is generated from one table in the server (`server/internal/api/openapi/routes.go`); a test fails when a route is added without an entry, so it cannot drift from the code.

## Authentication

Send a bearer token on every request:

```
Authorization: Bearer <token>
```

Two kinds of token are accepted:

| Token | Looks like | Lifetime | Get it from |
|---|---|---|---|
| Dashboard session | `eyJ...` (JWT) | 7 days | `POST /api/auth/login` |
| **API token** | `dl_` + 43 characters | until revoked, or the expiry you chose | Dashboard → **API & MCP**, or `docklift api-token create` |

Use API tokens for scripts, CI and AI agents. They do not need your password, can be read-only, can expire, and can be revoked one by one.

### Create an API token

**Dashboard:** **API & MCP → Create token**. Pick a name, the access (read only or full), whether dangerous actions are allowed, and an expiry, then confirm with your account password. The token is shown **once**; Docklift stores only its SHA-256 hash.

**Server:**

```bash
docker exec docklift-backend docklift api-token create ci-deploy                 # full access, never expires
docker exec docklift-backend docklift api-token create grafana --read --expires 90d
docker exec docklift-backend docklift api-token create ops-agent --allow-dangerous
docker exec docklift-backend docklift api-token list
docker exec docklift-backend docklift api-token revoke dl_AbC123x
```

The CLI creates tokens for the first admin. Note its default is **full** access (`--read` for read only); the dashboard and `POST /api/tokens` default to read only.

### What a token may do

| | Read token | Full token | Full token + allow dangerous |
|---|---|---|---|
| `GET` / `HEAD` requests | yes | yes | yes |
| Download a backup (`GET /api/backup/download/:file`) | no | yes | yes |
| `POST` / `PUT` / `PATCH` / `DELETE` | no (403) | yes | yes |
| Step-up actions: rollback, restore, clear restore seal, purge, reboot, reset, update-system, upgrade, `POST /api/system/execute` | no | no (403) | yes, without a password |
| Token management, profile, change password, terminal and SSE tokens, the WebSocket terminal, "Only allow access through the domain" | **never** | **never** | **never** |

In the dashboard, step-up actions ask for your password again. A token has no password, so it can perform them only when it was created with **allow dangerous actions**; otherwise the answer is 403:

```json
{ "error": "This action asks for the account password in the dashboard. An API token can perform it only when it was created with \"allow dangerous actions\".", "requirePassword": false }
```

Log streams (`GET /api/logs/:projectId/stream/:container`, `GET /api/system/logs/:service`) accept the bearer API token directly; a dashboard session still needs a 5 minute SSE token there.

### Errors

| Status | Error | Meaning |
|---|---|---|
| 401 | `Invalid API token` | Unknown, malformed or revoked token |
| 401 | `API token expired` | Past its expiry |
| 401 | `API token revoked: the account password changed after it was created` | The owner's password changed (also after `docklift reset-password`) |
| 403 | `This API token is read-only (...)` | A read token on a write request |
| 403 | `This endpoint is not available with an API token. ...` | A dashboard-only endpoint |
| 429 | `Too many invalid API tokens. Try again later.` | 20 invalid tokens from one address within 15 minutes; `Retry-After` says how long |

**Changing the account password revokes every API token created before the change.** Create new ones afterwards.

Every write request made with a token is logged by the backend as `[API-TOKEN] METHOD /path by token "name" (dl_prefix) from IP: ...`, and dangerous actions as `[AUDIT]`.

### Examples

```bash
export DOCKLIFT=https://panel.example.com
export DOCKLIFT_TOKEN=dl_...

# List projects
curl -s -H "Authorization: Bearer $DOCKLIFT_TOKEN" $DOCKLIFT/api/projects

# Add an environment variable, then redeploy (the build log streams back)
curl -s -H "Authorization: Bearer $DOCKLIFT_TOKEN" -H "Content-Type: application/json" \
  -d '{"key":"LOG_LEVEL","value":"debug"}' $DOCKLIFT/api/projects/$PROJECT_ID/env
curl -N -H "Authorization: Bearer $DOCKLIFT_TOKEN" -X POST $DOCKLIFT/api/deployments/$PROJECT_ID/redeploy
```

Deploy, redeploy, stop, restart, cancel, rollback, backup and restore answer with a `text/plain` log streamed until the operation ends (use `curl -N`). A deploy keeps running if the client disconnects.

## AI agents (MCP)

The same server is an MCP server at `/mcp`. See [AI Agents (MCP)](/guide/mcp).

## Common endpoints

The OpenAPI document is the complete list; these are the ones most scripts need.

| Method | Endpoint | Description |
|--------|----------|-------------|
| `GET` | `/api/projects` | List projects |
| `POST` | `/api/projects` | Create from GitHub (JSON) or a ZIP (multipart field `files`) |
| `GET` / `PATCH` / `DELETE` | `/api/projects/:id` | Get, update, delete |
| `GET` / `POST` | `/api/projects/:id/env` | List / create env vars |
| `POST` | `/api/projects/:id/env/bulk` | Import `KEY=VALUE` lines |
| `DELETE` | `/api/projects/:id/env/:envId` | Delete an env var |
| `POST` | `/api/deployments/:projectId/deploy` | Deploy (streams logs) |
| `POST` | `/api/deployments/:projectId/redeploy` | Rebuild and redeploy (streams logs) |
| `POST` | `/api/deployments/:projectId/stop`, `/restart`, `/cancel` | Lifecycle (streams logs) |
| `POST` | `/api/deployments/:projectId/rollback` | Roll back (step-up; streams logs) |
| `GET` | `/api/deployments/:projectId?meta=1&limit=10` | Deployment history |
| `GET` | `/api/deployments/:projectId/logs` | Last 100 lines of every service |
| `GET` | `/api/deployments/:projectId/services` | Services and their domains |
| `PUT` | `/api/deployments/:projectId/services/:serviceId` | Set a service's domains (`{"domain":"a.com,b.com"}`) |
| `GET` / `POST` | `/api/databases` | List / create managed databases |
| `POST` | `/api/databases/:id/links` | Inject a database URL into an app |
| `GET` | `/api/system/stats` | Host metrics |
| `GET` | `/api/backup` · `POST /api/backup/create` | List / create backups |
| `GET` / `POST` / `DELETE` | `/api/tokens` | API tokens (dashboard session only, step-up for create and revoke) |
| `GET` | `/api/openapi.json` | The full endpoint map |
| `GET` | `/api/health` | Health (no auth) |
