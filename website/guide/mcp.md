# AI Agents (MCP)

Docklift is also a [Model Context Protocol](https://modelcontextprotocol.io) server. Connect Claude Code, Claude Desktop, Cursor or any other MCP client and the agent can list, create, deploy and debug your projects, with exactly the permissions of the API token you give it.

- **Endpoint:** `https://<your panel domain>/mcp` (or `http://SERVER_IP:8080/mcp`)
- **Transport:** Streamable HTTP, stateless, JSON responses. Both current protocol revisions are supported: `2026-07-28` (per-request metadata, `server/discover`) and the older `initialize`-based revisions up to `2025-11-25`.
- **Auth:** `Authorization: Bearer dl_...` (an [API token](/guide/api#authentication)). A dashboard session token is not accepted; a missing or bad token gets 401 with `WWW-Authenticate: Bearer`.

## 1. Create a token

Dashboard → **API & MCP → Create token**, or on the server:

```bash
docker exec docklift-backend docklift api-token create claude --read      # look only
docker exec docklift-backend docklift api-token create claude             # can deploy and change things
```

Start with a **read** token. Give a **full** token when the agent should deploy, and tick **allow dangerous actions** only if it must roll back, restore, purge, reboot or run shell commands on the host.

## 2. Connect a client

**Claude Code:**

```bash
claude mcp add --transport http docklift https://panel.example.com/mcp \
  --header "Authorization: Bearer dl_..."
```

**Other clients** (Claude Desktop, Cursor, VS Code and others that read an `mcpServers` config):

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

The **API & MCP** page shows both snippets with your URL, and fills in a token right after you create it.

Use the HTTPS panel domain when the agent runs on another machine: the token travels in a header and plain `http://SERVER_IP:8080` sends it unencrypted.

## Tools

| Tool | What it does |
|---|---|
| `docklift_list_projects` / `docklift_get_project` | Projects with status, services and domains |
| `docklift_create_project` | Create from a GitHub repository (deploy it afterwards) |
| `docklift_delete_project` | Delete a project for good |
| `docklift_deploy` | Deploy or redeploy and return the build log |
| `docklift_stop` / `docklift_restart` | Lifecycle, with the log |
| `docklift_project_logs` | Last 100 lines of every service |
| `docklift_container_logs` | One container: past lines, then follows for a few seconds |
| `docklift_list_env` / `docklift_set_env` / `docklift_delete_env` | Environment variables (set = create or replace) |
| `docklift_list_databases` / `docklift_create_database` | Managed Postgres, MySQL, MariaDB, MongoDB, Redis |
| `docklift_list_domains` / `docklift_add_domain` / `docklift_remove_domain` | Domains of a project's services |
| `docklift_list_deployments` | Deployment history |
| `docklift_system_stats` | CPU, memory, disk |
| `docklift_create_backup` / `docklift_list_backups` | Backups |
| `docklift_list_endpoints` | Every HTTP endpoint, filterable by tag or word |
| `docklift_api_request` | Call any `/api/...` endpoint (method, path, query, JSON body) |

Anything without a dedicated tool is reachable through `docklift_list_endpoints` + `docklift_api_request`.

## How it is enforced

Every tool call is turned into ordinary HTTP requests that go **through the same router in the same process**, with your token and your client address. So a tool can never do more than the token could with `curl`:

- read tokens are refused on any write (the tool answers with the 403);
- step-up actions need **allow dangerous actions**;
- token management, password and profile changes, the terminal and the domain-only switch are never reachable;
- validation, rate limits, the maintenance gate and `[API-TOKEN]` / `[AUDIT]` log lines are the same as over HTTP.

`/mcp` sits behind the same front middleware as the dashboard: trusted proxies, "Only allow access through the domain" (with it on, `/mcp` works only through the panel domain), and a browser `Origin` must be the dashboard itself (else 403). Messages are limited to 4 MiB.

## Long output

Deploy, restart, backup and other streamed operations are followed until they finish or `waitSeconds` pass (default 600, max 1800). The tool returns the **last 64 KiB** and says when the start was cut or when it stopped waiting; a deploy keeps running on the server either way, so the agent can check `docklift_list_deployments` later. JSON answers are cut after 256 KiB.

## Troubleshooting

- **401 `Invalid API token`**: wrong or revoked token. **`API token revoked: ...`**: the password changed after the token was created; make a new one.
- **429**: 20 invalid tokens from your address within 15 minutes. Wait for `Retry-After`.
- **403 on `/mcp`** with a browser-based client: its `Origin` is not the dashboard. Add it to `CORS_ORIGIN` if you trust it.
- **403 `Dashboard access by IP is disabled`**: domain-only access is on; use `https://<panel domain>/mcp`.
