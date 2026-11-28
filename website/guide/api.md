# API Guide

The Go backend serves both the dashboard and the API at **`http://SERVER_IP:8080`** (or your panel domain, for example `https://panel.example.com`). In local development (`make dev`) the API listens on **`http://localhost:8000`** and the Vite dev server on `http://127.0.0.1:3600` proxies `/api` and `/ws` to it.

Everything the dashboard does goes through this API, so a script, a CI job or an AI agent can do the same with an **API token**.

- **[Full API Reference](/guide/api-reference)**: every route (method, path, parameters, body fields and markers), generated from the server's route table.
- **OpenAPI 3.1 document**: `GET /api/openapi.json` on your server (see [below](#the-openapi-document)).
- **[AI Agents (MCP)](/guide/mcp)**: the same API as MCP tools at `/mcp`.

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

Use API tokens for scripts, CI and AI agents. They do not need your password, can be read-only, can expire, and can be revoked one by one. Session tokens are what the dashboard uses; a script that logs in with the password is possible but not recommended.

Routes without authentication: `GET /api/health`, the setup and login routes under `/api/auth`, the GitHub App callbacks and the signed GitHub webhook. The reference shows the auth of every route.

## API tokens

### Create

**Dashboard:** **API & MCP** (`/developer`) → **Create token**. Pick a name, the access (**Read only** or **Full access**), whether **allow dangerous actions** is on (full access only), and an expiry, then confirm with your account password. The token is shown **once**; Docklift stores only its SHA-256 hash and a display prefix (`dl_` plus 8 characters).

**Server CLI** (runs on the host, needs no password, creates the token for the first admin):

```bash
docker exec docklift-backend docklift api-token create ci-deploy                  # full access, never expires
docker exec docklift-backend docklift api-token create grafana --read --expires 90d
docker exec docklift-backend docklift api-token create ops-agent --allow-dangerous
docker exec docklift-backend docklift api-token list                              # never shows the token itself
docker exec docklift-backend docklift api-token revoke dl_AbC123x                 # by id or prefix
```

The CLI defaults to **full** access (`--read` for read only); the dashboard and `POST /api/tokens` default to **read**. `--expires` takes days (`90d` or `90`, 1 to 3650) or `never`.

**API** (dashboard session only, with the step-up password):

```bash
curl -s -X POST https://panel.example.com/api/tokens \
  -H "Authorization: Bearer $SESSION_JWT" -H "Content-Type: application/json" \
  -d '{"name":"ci-deploy","scope":"full","allowDangerous":false,"expiresInDays":90,"password":"..."}'
```

The answer (201) is `{"token": "dl_...", "apiToken": {...}}`. `name` is 1 to 100 characters, `scope` is `read` (default) or `full`, `allowDangerous` needs `full`, `expiresInDays` is 1 to 3650 or `null` for no expiry.

### Lifecycle

| Event | Effect |
|---|---|
| Used | `last_used_at` is updated (at most once a minute); every write request is logged as `[API-TOKEN] METHOD /path by token "name" (dl_prefix) from IP: ...` |
| Expiry passes | 401 `API token expired`; the list shows it as **expired** |
| Account password changes (also `docklift reset-password`) | every token created **before** the change stops: 401 `API token revoked: ...`; the list shows **revoked** |
| Revoked (dashboard, `DELETE /api/tokens/{id}` with the password, or `docklift api-token revoke`) | deleted, stops working at once: 401 `Invalid API token` |
| Owner deleted | the owner's tokens are deleted with it |

No token can list, create or revoke tokens: token management is dashboard session only.

### Scopes

| | Read token | Full token | Full token + allow dangerous |
|---|---|---|---|
| `GET` / `HEAD` requests | yes | yes | yes |
| Download a backup (`GET /api/backup/download/{filename}`) | no (403) | yes | yes |
| `POST` / `PUT` / `PATCH` / `DELETE` | no (403) | yes | yes |
| Dangerous (step-up) actions: rollback, restore (three routes), clear the restore seal, purge, reboot, reset, update-system, upgrade, `POST /api/system/execute` | no | no (403) | yes, without a password |
| Token management, profile, change password, SSE and terminal tokens, the WebSocket terminal, "Only allow access through the domain" | **never** | **never** | **never** |

The [reference](/guide/api-reference) marks these as **no read tokens**, **dangerous** and **session only**.

In the dashboard, dangerous actions ask for your password again. A token has no password, so it can perform them only when it was created with **allow dangerous actions**. A dashboard session sends the account `password` in the JSON body of those routes instead.

Log streams (`GET /api/logs/{projectId}/stream/{containerName}`, `GET /api/system/logs/{service}`) accept the bearer API token directly; a dashboard session still needs a 5 minute SSE token (`?token=`) there.

A read token is still powerful: it can read every environment variable value, database credentials (`GET /api/databases/{id}/connection`), project files and logs. Keep every token secret, and send it only over HTTPS.

## Errors

Errors are JSON with an `error` message and the HTTP status:

```json
{ "error": "API token expired" }
```

| Status | Error | Meaning |
|---|---|---|
| 400 | (varies) | Validation failed, for example `name is required (max 100 characters)` |
| 401 | `Invalid API token` | Unknown, malformed or revoked token |
| 401 | `API token expired` | Past its expiry |
| 401 | `API token revoked: the account password changed after it was created` | The owner's password changed |
| 403 | `This API token is read-only (scope "read" allows GET requests only)` | A read token on a write request |
| 403 | `This endpoint needs an API token with scope "full"` | A read token on a GET route that needs full (backup download) |
| 403 | `This endpoint is not available with an API token. Sign in to the dashboard instead.` | A session-only endpoint |
| 403 | `This action asks for the account password in the dashboard. ...` | A dangerous action with a token without allow dangerous; the body also has `"requirePassword": false` |
| 403 | `Password confirmation required for this operation` / `Invalid password` | A dashboard session on a dangerous action without, or with a wrong, `password` (`"requirePassword": true`) |
| 403 | `Dashboard access by IP is disabled. Use https://...` | "Only allow access through the domain" is on; use the panel domain |
| 404 | `Not found` | Unknown route, or the object does not exist |
| 429 | (see below) | Rate limited; `Retry-After` says how many seconds to wait |
| 503 | (reason) with `"maintenance": true` | A restore is running, or a failed restore left the restore-critical seal |

## Rate and size limits

| Limit | Value |
|---|---|
| Invalid API tokens | 20 per client address within 15 minutes, then 429 `Too many invalid API tokens. Try again later.` until the window ends. A valid token does not reset the count; other addresses and sessions are not affected. |
| Wrong step-up passwords | 10 per user and client within 15 minutes, then 429 `Too many failed password attempts. Try again later.` |
| `/api/auth/*` (login, register, profile) | 100 requests per address within 15 minutes; register and setup-token 10 |
| JSON body | 10 MiB |
| ZIP upload (`POST /api/projects`, field `files`) | 100 MiB |
| Backup upload | 500 MiB |
| Shell command (`POST /api/system/execute`) | 30 seconds, 1 MiB output |

## Streamed responses

Deploy, redeploy, stop, restart, cancel, rollback, backup create and the three restore routes answer with a `text/plain` log streamed until the operation ends. Use `curl -N` so the output shows as it arrives. A deploy keeps running if the client disconnects; read its result from the deployment history. Container and system log routes are Server-Sent Events (`text/event-stream`) that follow until you disconnect. The reference marks these routes **streams**.

## Examples

Set these once. The examples use [`jq`](https://jqlang.org) to pick ids out of JSON answers.

```bash
export DOCKLIFT=https://panel.example.com
export DOCKLIFT_TOKEN=dl_...
auth=(-H "Authorization: Bearer $DOCKLIFT_TOKEN")
json=(-H "Content-Type: application/json")
```

### List projects

```bash
curl -s "${auth[@]}" $DOCKLIFT/api/projects | jq '.[] | {id, name, status}'
```

### Create a project from GitHub

The repository is cloned now; nothing is built until you deploy. Private repositories need the [GitHub App](/guide/github) installed in Docklift.

```bash
PROJECT_ID=$(curl -s "${auth[@]}" "${json[@]}" -X POST $DOCKLIFT/api/projects -d '{
  "name": "shop",
  "source_type": "github",
  "github_url": "https://github.com/acme/shop",
  "github_branch": "main",
  "internal_port": 8080
}' | jq -r .id)
```

Other fields: `build_type` (`auto`, `dockerfile`, `railpack`), `base_directory`, `dockerfile_path`, `domain`, `description`. To upload a ZIP instead, send `multipart/form-data` with `name` and the file in `files`.

### Set an environment variable

```bash
curl -s "${auth[@]}" "${json[@]}" -X POST $DOCKLIFT/api/projects/$PROJECT_ID/env \
  -d '{"key":"LOG_LEVEL","value":"debug"}'

# or many at once from .env text
curl -s "${auth[@]}" "${json[@]}" -X POST $DOCKLIFT/api/projects/$PROJECT_ID/env/bulk \
  -d '{"content":"LOG_LEVEL=debug\nFEATURE_X=1"}'
```

Add `"service_name"` to scope a variable to one service, `"is_build_arg": true` to pass it to the build. Running containers see new values after the next deploy.

### Deploy and read the log

```bash
curl -N "${auth[@]}" "${json[@]}" -X POST $DOCKLIFT/api/deployments/$PROJECT_ID/deploy \
  -d '{"trigger":"ci","commit_message":"release 1.4"}'

# the result, from the history (newest first)
curl -s "${auth[@]}" "$DOCKLIFT/api/deployments/$PROJECT_ID?meta=1&limit=1" | jq '.items[0].status'
```

Use `/redeploy` to rebuild the current source. A successful deploy log contains `DEPLOY SUCCESSFUL!`.

### Create a database and link it

```bash
DB_ID=$(curl -s "${auth[@]}" "${json[@]}" -X POST $DOCKLIFT/api/databases \
  -d '{"name":"shop-db","engine":"postgres"}' | jq -r .id)

# inject its connection URL (DATABASE_URL by default for Postgres) into the app
curl -s "${auth[@]}" "${json[@]}" -X POST $DOCKLIFT/api/databases/$DB_ID/links \
  -d "{\"app_project_id\":\"$PROJECT_ID\"}"
```

Engines and versions: `GET /api/databases/engines`. The create answer contains the credentials and `connection_url`; `GET /api/databases/{id}/connection` returns them again.

### Add a domain

Domains belong to a service. `PUT` replaces the whole comma-separated list, so read it first:

```bash
curl -s "${auth[@]}" $DOCKLIFT/api/deployments/$PROJECT_ID/services | jq '.[] | {id, name, domain}'
SERVICE_ID=...   # from the list above

curl -s "${auth[@]}" "${json[@]}" -X PUT $DOCKLIFT/api/deployments/$PROJECT_ID/services/$SERVICE_ID \
  -d '{"domain":"shop.example.com,www.shop.example.com"}'
```

Docklift writes the nginx config and requests HTTPS certificates; point DNS at the server first (`POST /api/deployments/{projectId}/services/{serviceId}/dns-check` checks it). See [Custom Domains](/guide/domains).

### Create a backup

```bash
curl -N "${auth[@]}" "${json[@]}" -X POST $DOCKLIFT/api/backup/create -d '{"name":"before-upgrade"}'
curl -s "${auth[@]}" $DOCKLIFT/api/backup
curl -s "${auth[@]}" -o backup.zip $DOCKLIFT/api/backup/download/FILENAME.zip   # full token
```

## The OpenAPI document

Every route, with its path and query parameters, request body fields, whether it streams and whether it is a dangerous (step-up) action, is in an OpenAPI 3.1 document served by the server itself (any valid token, read included):

```bash
curl -s "${auth[@]}" $DOCKLIFT/api/openapi.json -o docklift-openapi.json
```

Import that file into Postman (**Import** → file), Insomnia, Bruno or any OpenAPI tool to get a ready collection; set the collection's auth to **Bearer Token** with your `dl_...` token and the base URL to your panel. The document lists `servers: [{"url": "/"}]`, so set the base URL yourself. Response schemas are deliberately loose; request bodies and parameters are exact. Docklift-specific flags are in `x-docklift-*` extensions (`x-docklift-step-up`, `x-docklift-stream`, `x-docklift-session-only`, `x-docklift-read-scope`, `x-docklift-auth`).

In the dashboard, **API & MCP** has a searchable endpoint browser built from the same document. The document, the [Full API Reference](/guide/api-reference) and the MCP tool `docklift_list_endpoints` all come from one table in the server (`server/internal/api/openapi/routes.go`); a router test fails when a route is added without an entry, so they cannot drift from the code.

## AI agents (MCP)

The same server is an MCP server at `/mcp`. See [AI Agents (MCP)](/guide/mcp).
