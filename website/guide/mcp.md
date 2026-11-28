# AI Agents (MCP)

Docklift is also a [Model Context Protocol](https://modelcontextprotocol.io) (MCP) server. Connect Claude Code, Claude Desktop, Cursor, VS Code, Codex CLI or any other MCP client, and the agent can list, create, configure, deploy and debug your projects with exactly the permissions of the [API token](/guide/api#api-tokens) you give it.

What an agent can do through it:

- see every project, its services, status, domains and deployment history;
- create a project from a GitHub repository, deploy or redeploy it and read the build log;
- stop and restart projects, read the last log lines of every service or follow one container;
- list, set and delete environment variables;
- create managed databases (Postgres, MySQL, MariaDB, MongoDB, Redis);
- add and remove domains of a service (Docklift writes the nginx config and requests HTTPS);
- read host metrics, create and list backups;
- call **any other** API endpoint through `docklift_list_endpoints` and `docklift_api_request`.

| | |
|---|---|
| **Endpoint** | `https://panel.example.com/mcp` (your panel domain), or `http://SERVER_IP:8080/mcp` |
| **Transport** | Streamable HTTP, stateless, JSON responses (`POST` only; `GET` and `DELETE` answer 405) |
| **Protocol** | `2026-07-28` (per-request metadata, `server/discover`) and the older `initialize`-based revisions `2025-11-25`, `2025-06-18`, `2025-03-26`, `2024-11-05` |
| **Auth** | `Authorization: Bearer dl_...` (an API token). A dashboard session token is refused. |

## 1. Create a token

**Dashboard:** open **API & MCP** (`/developer`) and click **Create token**. Choose a name, **Read only** or **Full access**, whether **allow dangerous actions** is on (full access only), and an expiry, then confirm with your account password. The token is shown **once**; Docklift keeps only its SHA-256 hash. Right after you create it, the MCP card on the same page fills the token into the copy-ready Claude Code command and MCP config.

**Server CLI** (no password needed; it runs on the host and creates the token for the first admin):

```bash
docker exec docklift-backend docklift api-token create claude --read              # look only
docker exec docklift-backend docklift api-token create claude                     # full: deploy and change things
docker exec docklift-backend docklift api-token create ops-agent --allow-dangerous # full + dangerous actions
docker exec docklift-backend docklift api-token create claude --read --expires 30d
```

Note the defaults differ: the CLI creates **full** tokens unless you pass `--read`; the dashboard and `POST /api/tokens` default to **read**.

Start with a **read** token. Give a **full** token when the agent should deploy or change settings, and turn on **allow dangerous actions** only if it must roll back, restore backups, purge images, reboot or upgrade the host, or run shell commands. See [Permissions](#permissions).

Keep the token out of files you commit: the examples below read it from an environment variable where the client supports that.

```bash
export DOCKLIFT_TOKEN=dl_...   # the token you just created
```

## 2. Connect a client

The URL is your panel URL plus `/mcp`. Use the **HTTPS panel domain** whenever the agent is not on the server itself: the token travels in a header, and plain `http://SERVER_IP:8080` sends it unencrypted.

### Claude Code

```bash
claude mcp add --transport http docklift https://panel.example.com/mcp \
  --header "Authorization: Bearer $DOCKLIFT_TOKEN"
```

Add `--scope user` to make it available in every project, or `--scope project` to write it to the project's `.mcp.json` (shared with the repository). For a shared `.mcp.json`, reference the variable instead of pasting the token; Claude Code expands `${VAR}` in `url` and `headers`:

```json
{
  "mcpServers": {
    "docklift": {
      "type": "http",
      "url": "https://panel.example.com/mcp",
      "headers": { "Authorization": "Bearer ${DOCKLIFT_TOKEN}" }
    }
  }
}
```

Check it with `claude mcp list`, or `/mcp` inside a session. Source: [Claude Code MCP docs](https://code.claude.com/docs/en/mcp).

### Claude Desktop

Claude Desktop has two ways to reach a remote server:

**A. Custom connector** (Settings → Connectors / Customize → Connectors → **Add custom connector**). Enter the URL `https://panel.example.com/mcp`, choose **No sign in** under Authentication, and under **Request headers** add `Authorization` with the value `Bearer dl_...`. Custom connectors are opened **from Anthropic's cloud**, not from your computer, so this only works when the panel domain is reachable from the public internet over HTTPS on port 443. Source: [Getting started with custom connectors](https://support.claude.com/en/articles/11175166-getting-started-with-custom-connectors-using-remote-mcp).

**B. Local bridge with `mcp-remote`** (works when the panel is only reachable from your machine, for example over a VPN). `claude_desktop_config.json` only starts local (stdio) servers, so run the [`mcp-remote`](https://github.com/geelen/mcp-remote) bridge with `npx` (needs Node.js):

```json
{
  "mcpServers": {
    "docklift": {
      "command": "npx",
      "args": [
        "-y",
        "mcp-remote",
        "https://panel.example.com/mcp",
        "--header",
        "Authorization:${AUTH_HEADER}"
      ],
      "env": { "AUTH_HEADER": "Bearer dl_..." }
    }
  }
}
```

Write `Authorization:${AUTH_HEADER}` exactly like that, without spaces: Claude Desktop on Windows does not escape spaces inside `args`, so the `Bearer ...` value goes in `env`, where spaces are fine. Restart Claude Desktop after editing the file.

### Cursor

`~/.cursor/mcp.json` (every project) or `.cursor/mcp.json` (one project). Cursor sends `headers` to remote servers and expands `${env:NAME}`:

```json
{
  "mcpServers": {
    "docklift": {
      "url": "https://panel.example.com/mcp",
      "headers": { "Authorization": "Bearer ${env:DOCKLIFT_TOKEN}" }
    }
  }
}
```

Source: [Cursor MCP docs](https://cursor.com/docs/context/mcp).

### VS Code (GitHub Copilot)

Workspace-root `.mcp.json` (or your Copilot user `~/.copilot/mcp-config.json`) uses the `mcpServers` format and expands `${NAME}` from the environment in header values:

```json
{
  "mcpServers": {
    "docklift": {
      "type": "http",
      "url": "https://panel.example.com/mcp",
      "headers": { "Authorization": "Bearer ${DOCKLIFT_TOKEN}" }
    }
  }
}
```

An existing editor-managed `.vscode/mcp.json` (top-level `servers`, now marked deprecated by VS Code) can prompt for the token instead of storing it:

```json
{
  "inputs": [
    { "type": "promptString", "id": "docklift-token", "description": "Docklift API token", "password": true }
  ],
  "servers": {
    "docklift": {
      "type": "http",
      "url": "https://panel.example.com/mcp",
      "headers": { "Authorization": "Bearer ${input:docklift-token}" }
    }
  }
}
```

Source: [VS Code MCP configuration reference](https://code.visualstudio.com/docs/agents/reference/mcp-configuration).

### Codex CLI

In `~/.codex/config.toml`, `bearer_token_env_var` names the environment variable whose value Codex sends as `Authorization: Bearer ...`:

```toml
[mcp_servers.docklift]
url = "https://panel.example.com/mcp"
bearer_token_env_var = "DOCKLIFT_TOKEN"
```

Start Codex from a shell where `DOCKLIFT_TOKEN` is set. Source: [Codex MCP docs](https://learn.chatgpt.com/docs/extend/mcp?surface=cli).

### Any other client

Most clients read an `mcpServers` object like the one the dashboard shows under **Other MCP clients**:

```json
{
  "mcpServers": {
    "docklift": {
      "type": "http",
      "url": "https://panel.example.com/mcp",
      "headers": { "Authorization": "Bearer dl_..." }
    }
  }
}
```

Field names vary between clients (some want `type: "http"`, some only `url`), so check your client's docs. If a client cannot send a custom header to a remote server, use the `mcp-remote` bridge shown under [Claude Desktop](#claude-desktop). To test by hand:

```bash
curl -s https://panel.example.com/mcp \
  -H "Authorization: Bearer $DOCKLIFT_TOKEN" \
  -H "Content-Type: application/json" \
  -H "Accept: application/json, text/event-stream" \
  -H "MCP-Protocol-Version: 2025-11-25" \
  -d '{"jsonrpc":"2.0","id":1,"method":"tools/list"}'
```

## Tool reference

::: info Generated section
This section is generated from `server/internal/api/mcp/tools.go` (23 tools). Do not edit it by hand: run `go run ./cmd/gen-docs` in `server/`. The rest of this page is written by hand.
:::

Every tool call becomes one or more ordinary API requests (see [How it is enforced](#how-it-is-enforced)), so **Scope** is the API token scope those requests need. Tools that follow streamed output accept `waitSeconds` (default 600, max 1800).

| Tool | Scope | What it does |
|---|---|---|
| [`docklift_list_projects`](#docklift-list-projects) | read | List every project (apps and managed databases) with status, source, domains and services. |
| [`docklift_get_project`](#docklift-get-project) | read | Get one project with its services, build settings, status and domains. |
| [`docklift_create_project`](#docklift-create-project) | full | Create a project from a GitHub repository (https://github.com/owner/repo). The repository is cloned now; call docklift_deploy afterwards to build and start it. Private repositories need the GitHub App installed in Docklift. |
| [`docklift_delete_project`](#docklift-delete-project) | full | Delete a project for good: its containers, images, volumes records, files and history. This cannot be undone. |
| [`docklift_deploy`](#docklift-deploy) | full | Build and start a project (pull/clone source, build the image, start containers) and return the build output. Set redeploy to rebuild the current source. Output is followed until the deploy ends or waitSeconds pass; only the last 64 KiB is returned. |
| [`docklift_stop`](#docklift-stop) | full | Stop every container of a project (returns the log). |
| [`docklift_restart`](#docklift-restart) | full | Restart every container of a project (returns the log). |
| [`docklift_project_logs`](#docklift-project-logs) | read | The last 100 log lines of every running service of a project. |
| [`docklift_container_logs`](#docklift-container-logs) | read | Read one container's logs: the last `tail` lines, then follows new lines for `seconds` (default 3, max 60). Container names come from docklift_get_project (services[].container_name). |
| [`docklift_list_env`](#docklift-list-env) | read | List a project's environment variables (values included). |
| [`docklift_set_env`](#docklift-set-env) | full | Create or replace an environment variable of a project (replacing deletes the old one with the same key and scope first). Redeploy for running containers to see it. |
| [`docklift_delete_env`](#docklift-delete-env) | full | Delete an environment variable of a project by key (and service scope). |
| [`docklift_list_databases`](#docklift-list-databases) | read | List managed databases (Postgres, MySQL, MariaDB, MongoDB, Redis). |
| [`docklift_create_database`](#docklift-create-database) | full | Create and start a managed database. Use docklift_api_request POST /api/databases/{id}/links to inject its URL into an app. |
| [`docklift_list_domains`](#docklift-list-domains) | read | List the services of a project with their domains (comma-separated) and ids. |
| [`docklift_add_domain`](#docklift-add-domain) | full | Add a domain to a service of a project. Writes the nginx config and requests an HTTPS certificate (point DNS at the server first). |
| [`docklift_remove_domain`](#docklift-remove-domain) | full | Remove a domain from a service of a project. |
| [`docklift_system_stats`](#docklift-system-stats) | read | Host metrics: CPU, memory, disk, network, uptime, Docker. |
| [`docklift_list_deployments`](#docklift-list-deployments) | read | Deployment history of a project, newest first, with status, trigger, commit and logs. |
| [`docklift_create_backup`](#docklift-create-backup) | full | Create a backup of the database, project files and settings, and return the log. |
| [`docklift_list_backups`](#docklift-list-backups) | read | List backups on the server. |
| [`docklift_list_endpoints`](#docklift-list-endpoints) | read | List every Docklift HTTP endpoint (method, path, summary, body fields, flags) for use with docklift_api_request. Filter by tag or a search word. The full OpenAPI document is GET /api/openapi.json. |
| [`docklift_api_request`](#docklift-api-request) | depends on the endpoint | Call any Docklift API endpoint as this token (see docklift_list_endpoints). Read-scope tokens may only use GET. Endpoints marked step-up need a token created with allow_dangerous; token management, password, terminal and the domain-only switch are never available. Streamed responses are followed up to waitSeconds and the tail is returned. |

### `docklift_list_projects` {#docklift-list-projects}

List every project (apps and managed databases) with status, source, domains and services.

- **Scope:** read
- **Arguments:** none

### `docklift_get_project` {#docklift-get-project}

Get one project with its services, build settings, status and domains.

- **Scope:** read
- **Arguments:**
  - `projectId` (`string`, required): Project id (from docklift_list_projects).

### `docklift_create_project` {#docklift-create-project}

Create a project from a GitHub repository (https://github.com/owner/repo). The repository is cloned now; call docklift_deploy afterwards to build and start it. Private repositories need the GitHub App installed in Docklift.

- **Scope:** full
- **Arguments:**
  - `name` (`string`, required): Project name (max 120 characters).
  - `github_url` (`string`, required): https://github.com/owner/repo
  - `base_directory` (`string`): Subdirectory to build from (monorepos), default ".".
  - `build_type` (`string`): How to build (default auto: Dockerfile/compose if present, else Railpack). One of `auto`, `dockerfile`, `railpack`.
  - `description` (`string`): Optional description.
  - `dockerfile_path` (`string`): Explicit Dockerfile path inside the project.
  - `domain` (`string`): Optional comma-separated domains for the first service.
  - `github_branch` (`string`): Branch or tag (default: the repository's default branch).
  - `internal_port` (`integer`): Port the app listens on inside the container (default 3000).

### `docklift_delete_project` {#docklift-delete-project}

Delete a project for good: its containers, images, volumes records, files and history. This cannot be undone.

- **Scope:** full
- **Destructive:** yes (clients may ask you to confirm)
- **Arguments:**
  - `projectId` (`string`, required): Project id (from docklift_list_projects).

### `docklift_deploy` {#docklift-deploy}

Build and start a project (pull/clone source, build the image, start containers) and return the build output. Set redeploy to rebuild the current source. Output is followed until the deploy ends or waitSeconds pass; only the last 64 KiB is returned.

- **Scope:** full
- **Arguments:**
  - `projectId` (`string`, required): Project id (from docklift_list_projects).
  - `commit_message` (`string`): Optional note stored on the deployment.
  - `redeploy` (`boolean`): Use the redeploy route instead of deploy.
  - `waitSeconds` (`integer`): How long to follow the streamed output, in seconds (default 600, max 1800). The operation keeps running on the server after this.

### `docklift_stop` {#docklift-stop}

Stop every container of a project (returns the log).

- **Scope:** full
- **Arguments:**
  - `projectId` (`string`, required): Project id (from docklift_list_projects).
  - `waitSeconds` (`integer`): How long to follow the streamed output, in seconds (default 600, max 1800). The operation keeps running on the server after this.

### `docklift_restart` {#docklift-restart}

Restart every container of a project (returns the log).

- **Scope:** full
- **Arguments:**
  - `projectId` (`string`, required): Project id (from docklift_list_projects).
  - `waitSeconds` (`integer`): How long to follow the streamed output, in seconds (default 600, max 1800). The operation keeps running on the server after this.

### `docklift_project_logs` {#docklift-project-logs}

The last 100 log lines of every running service of a project.

- **Scope:** read
- **Arguments:**
  - `projectId` (`string`, required): Project id (from docklift_list_projects).

### `docklift_container_logs` {#docklift-container-logs}

Read one container's logs: the last `tail` lines, then follows new lines for `seconds` (default 3, max 60). Container names come from docklift_get_project (services[].container_name).

- **Scope:** read
- **Arguments:**
  - `projectId` (`string`, required): Project id (from docklift_list_projects).
  - `containerName` (`string`, required): Container name of a service of the project.
  - `seconds` (`integer`): How long to follow new output (default 3, max 60).
  - `tail` (`integer`): Past lines to include (default 200).

### `docklift_list_env` {#docklift-list-env}

List a project's environment variables (values included).

- **Scope:** read
- **Arguments:**
  - `projectId` (`string`, required): Project id (from docklift_list_projects).
  - `service` (`string`): Only variables of this service scope ("" = shared).

### `docklift_set_env` {#docklift-set-env}

Create or replace an environment variable of a project (replacing deletes the old one with the same key and scope first). Redeploy for running containers to see it.

- **Scope:** full
- **Arguments:**
  - `projectId` (`string`, required): Project id (from docklift_list_projects).
  - `key` (`string`, required): Variable name: letters, digits and _, not starting with a digit.
  - `value` (`string`, required): Value.
  - `is_build_arg` (`boolean`): Also pass to the image build (default false).
  - `is_runtime` (`boolean`): Set in the running container (default true).
  - `is_secret` (`boolean`): Pass a build variable as a BuildKit secret instead of a build arg.
  - `service_name` (`string`): Service scope; empty (default) means every service.

### `docklift_delete_env` {#docklift-delete-env}

Delete an environment variable of a project by key (and service scope).

- **Scope:** full
- **Destructive:** yes (clients may ask you to confirm)
- **Arguments:**
  - `projectId` (`string`, required): Project id (from docklift_list_projects).
  - `key` (`string`, required): Variable name.
  - `service_name` (`string`): Service scope; empty (default) means the shared scope.

### `docklift_list_databases` {#docklift-list-databases}

List managed databases (Postgres, MySQL, MariaDB, MongoDB, Redis).

- **Scope:** read
- **Arguments:** none

### `docklift_create_database` {#docklift-create-database}

Create and start a managed database. Use docklift_api_request POST /api/databases/{id}/links to inject its URL into an app.

- **Scope:** full
- **Arguments:**
  - `name` (`string`, required): Name (max 120 characters).
  - `engine` (`string`, required): Engine. One of `postgres`, `mysql`, `mariadb`, `mongodb`, `redis`.
  - `version` (`string`): Image tag (default: the recommended version, see GET /api/databases/engines).

### `docklift_list_domains` {#docklift-list-domains}

List the services of a project with their domains (comma-separated) and ids.

- **Scope:** read
- **Arguments:**
  - `projectId` (`string`, required): Project id (from docklift_list_projects).

### `docklift_add_domain` {#docklift-add-domain}

Add a domain to a service of a project. Writes the nginx config and requests an HTTPS certificate (point DNS at the server first).

- **Scope:** full
- **Arguments:**
  - `projectId` (`string`, required): Project id (from docklift_list_projects).
  - `domain` (`string`, required): Hostname, e.g. app.example.com.
  - `service` (`string`): Service id or name; optional when the project has one service.

### `docklift_remove_domain` {#docklift-remove-domain}

Remove a domain from a service of a project.

- **Scope:** full
- **Destructive:** yes (clients may ask you to confirm)
- **Arguments:**
  - `projectId` (`string`, required): Project id (from docklift_list_projects).
  - `domain` (`string`, required): Hostname, e.g. app.example.com.
  - `service` (`string`): Service id or name; optional when the project has one service.

### `docklift_system_stats` {#docklift-system-stats}

Host metrics: CPU, memory, disk, network, uptime, Docker.

- **Scope:** read
- **Arguments:** none

### `docklift_list_deployments` {#docklift-list-deployments}

Deployment history of a project, newest first, with status, trigger, commit and logs.

- **Scope:** read
- **Arguments:**
  - `projectId` (`string`, required): Project id (from docklift_list_projects).
  - `limit` (`integer`): Page size (default 10).
  - `offset` (`integer`): Rows to skip.
  - `status` (`string`): Only this status (e.g. success, failed, building).

### `docklift_create_backup` {#docklift-create-backup}

Create a backup of the database, project files and settings, and return the log.

- **Scope:** full
- **Arguments:**
  - `name` (`string`): Optional label for the file name.
  - `waitSeconds` (`integer`): How long to follow the streamed output, in seconds (default 600, max 1800). The operation keeps running on the server after this.

### `docklift_list_backups` {#docklift-list-backups}

List backups on the server.

- **Scope:** read
- **Arguments:** none

### `docklift_list_endpoints` {#docklift-list-endpoints}

List every Docklift HTTP endpoint (method, path, summary, body fields, flags) for use with docklift_api_request. Filter by tag or a search word. The full OpenAPI document is GET /api/openapi.json.

- **Scope:** read
- **Arguments:**
  - `search` (`string`): Only endpoints whose path or summary contains this text.
  - `tag` (`string`): Only this tag (Projects, Deployments, Databases, Domains, Files, Logs, Ports, GitHub, System, Backup, Auth, API tokens, Health, Meta).

### `docklift_api_request` {#docklift-api-request}

Call any Docklift API endpoint as this token (see docklift_list_endpoints). Read-scope tokens may only use GET. Endpoints marked step-up need a token created with allow_dangerous; token management, password, terminal and the domain-only switch are never available. Streamed responses are followed up to waitSeconds and the tail is returned.

- **Scope:** read for `GET`, full otherwise; **dangerous** endpoints need allow dangerous actions
- **Destructive:** yes (clients may ask you to confirm)
- **Arguments:**
  - `method` (`string`, required): HTTP method. One of `GET`, `POST`, `PUT`, `PATCH`, `DELETE`.
  - `path` (`string`, required): Path starting with /api/, e.g. /api/projects/abc/env.
  - `body` (`object`): JSON body.
  - `query` (`object`): Query parameters.
  - `waitSeconds` (`integer`): How long to follow the streamed output, in seconds (default 600, max 1800). The operation keeps running on the server after this.

## Examples

What you type, and what the agent does with the tools:

**"What is running on my Docklift server and is anything unhealthy?"**
`docklift_list_projects`, then `docklift_system_stats`; for a project that is not running, `docklift_project_logs`. Works with a read token.

**"Deploy github.com/acme/shop on shop.example.com, it listens on port 8080."**
`docklift_create_project` with `name`, `github_url` and `internal_port: 8080` (the repository is cloned now), `docklift_add_domain` with `domain: shop.example.com`, then `docklift_deploy`, which follows the build log and returns its last 64 KiB. Needs a full token, and DNS for the domain pointing at the server before HTTPS can be issued.

**"The api project crashes on start. Find out why and fix the config."**
`docklift_get_project` for the container names, `docklift_container_logs` for the failing container, `docklift_list_env` to compare variables, `docklift_set_env` to fix one, then `docklift_deploy` with `redeploy: true` so the containers get the new value.

**"Give the shop project a Postgres database."**
`docklift_create_database` with `engine: postgres`, then `docklift_api_request` `POST /api/databases/{id}/links` with `{"app_project_id": "..."}`, which injects the connection URL (`DATABASE_URL` by default) into the app, then a redeploy.

**"Make a backup before you change anything."**
`docklift_create_backup`, which returns the backup log; `docklift_list_backups` shows the file.

**"Roll shop back to yesterday's deployment."**
`docklift_list_deployments` for the deployment id, then `docklift_api_request` `POST /api/deployments/{projectId}/rollback` with `{"deploymentId": "..."}`. Rollback is a dangerous (step-up) action: without **allow dangerous actions** the tool returns the 403 and nothing happens.

## Permissions

| | Read token | Full token | Full + allow dangerous |
|---|---|---|---|
| Read tools (`list_*`, `get_project`, logs, stats) and `GET` through `docklift_api_request` | yes | yes | yes |
| Write tools (create, deploy, stop, restart, env, domains, databases, backups) and other methods | no (403) | yes | yes |
| `GET /api/backup/download/{filename}` | no (403) | yes | yes |
| Dangerous actions: rollback, restore, clear the restore seal, purge, reboot, reset, update-system, upgrade, `POST /api/system/execute` | no | no (403) | yes, without a password |
| Token management, profile, password change, SSE and terminal tokens, the WebSocket terminal, "Only allow access through the domain" | never | never | never |

The [full API reference](/guide/api-reference) marks every route that is dangerous, session only or not for read tokens.

## How it is enforced

Every tool call is turned into ordinary HTTP requests that go **through the same router in the same process**, with your token, client address, `User-Agent` and forwarded headers. A tool can never do more than the token could with `curl`:

- read tokens are refused on any write (the tool returns the 403);
- dangerous actions need **allow dangerous actions**;
- token management, password and profile changes, the terminal and the domain-only switch are never reachable;
- validation, rate limits, the maintenance gate and the `[API-TOKEN]` / `[AUDIT]` log lines are the same as over HTTP.

`/mcp` sits behind the same front middleware as the dashboard: trusted proxies, and "Only allow access through the domain" (with it on, `/mcp` works only through the panel domain). A request with a browser `Origin` header must come from the dashboard itself or an origin in `CORS_ORIGIN`, else 403 `Origin not allowed`.

## Security

- **Use the HTTPS panel domain.** Over `http://SERVER_IP:8080` the token crosses the network in clear text.
- **A full token is close to root.** It can deploy any repository as a container on your server, change environment variables and delete projects. With **allow dangerous actions** it can also run shell commands on the host, reboot it and restore backups. Give it only to agents you would trust with SSH.
- **A read token is not harmless.** It can read every environment variable value, database connection credentials (`GET /api/databases/{id}/connection`), project files and logs. Treat it as a secret too.
- **Tokens die on password change.** Changing the account password (also `docklift reset-password`) revokes every token created before the change. Create new ones afterwards.
- **Revoke on leak.** Dashboard → **API & MCP** → revoke, or `docker exec docklift-backend docklift api-token revoke <id|dl_prefix>`. It stops working at once.
- Prefer a separate token per agent or machine, with an expiry, so the **Last used** column and the backend log (`[API-TOKEN] ... by token "name"`) tell you which one acted.

## Limits

| Limit | Value |
|---|---|
| Following a streamed response (deploy, redeploy, stop, restart, backup, restore) | `waitSeconds`, default **600 s** (10 min), max **1800 s** (30 min). A deploy, backup or restore keeps running on the server after that; check `docklift_list_deployments` later. |
| Streamed output returned | the **last 64 KiB**; the tool says when the start was cut |
| JSON and other responses returned | the first **256 KiB**; the tool says when it was cut |
| Non-streaming request | 5 minutes |
| One MCP message (request body) | **4 MiB** |
| `docklift_container_logs` follow time | `seconds`, default 3, max 60 |
| Invalid tokens | 20 per client address within 15 minutes, then 429 until the window ends |

## Troubleshooting

| Symptom | Cause and fix |
|---|---|
| 401 `API token required (Authorization: Bearer dl_...)` | No header, or not a `dl_` token (a dashboard session token is refused here). Check the client sends the header to `/mcp`. |
| 401 `Invalid API token` | Wrong, malformed or revoked token. Create a new one. |
| 401 `API token expired` | Past its expiry. Create a new one. |
| 401 `API token revoked: the account password changed after it was created` | The password changed. Create a new token. |
| Tool result 403 `This API token is read-only (...)` | A read token tried a write. Use a full token. |
| Tool result 403 `This action asks for the account password in the dashboard. ...` | A dangerous action with a token created without **allow dangerous actions**. Create one with it if you really want the agent to do this. |
| Tool result 403 `This endpoint is not available with an API token. ...` | A dashboard-only endpoint. Do it in the dashboard. |
| 429 `Too many invalid API tokens. Try again later.` | 20 invalid tokens from your address within 15 minutes. Wait for `Retry-After`, then fix the token. |
| 403 `Dashboard access by IP is disabled. Use https://...` | "Only allow access through the domain" is on. Use `https://<panel domain>/mcp`. |
| 403 `Origin not allowed` | A browser-based client sent an `Origin` that is not the dashboard. Add it to `CORS_ORIGIN` only if you trust it. |
| 405 on `GET /mcp` | Expected: the server is stateless and has no server-to-client stream. Clients fall back to `POST`. |
| Tool result 503 with `"maintenance": true` | The API is in maintenance (a restore is running, or a failed restore left the restore-critical seal). Try again when it ends; see [Backup & Restore](/guide/backup). |
| Claude Desktop connector cannot connect | Custom connectors connect from Anthropic's cloud; the panel domain must be public on HTTPS port 443. Otherwise use the `mcp-remote` bridge. |
