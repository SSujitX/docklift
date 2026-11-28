---
outline: 2
---

# Full API Reference

::: info Generated page
This page is generated from the route table in `server/internal/api/openapi/routes.go`. Do not edit it by hand: change the table, then run `go run ./cmd/gen-docs` in `server/`. A test fails when this page and the table differ.
:::

Every HTTP route of the Docklift server: **109 routes** in 14 groups. The running server serves the same data as an OpenAPI 3.1 document at `GET /api/openapi.json`. Authentication, token scopes, errors and worked `curl` examples are in the [API guide](/guide/api).

Paths are relative to the panel URL (for example `https://panel.example.com`). Path parameters are written `{name}` and are always required. Bodies are JSON unless noted. Errors are JSON `{"error": "..."}`.

## Markers

| Marker | Meaning |
|---|---|
| **dangerous** | Step-up action. A dashboard session must send the account `password` in the body; an API token must have been created with **allow dangerous actions** (else 403). |
| **session only** | Refuses every API token with 403. Use a dashboard session. |
| **streams** | The response is a stream: a `text/plain` progress log until the operation ends, or Server-Sent Events (`text/event-stream`) that follow until the client disconnects. Use `curl -N`. |
| **no read tokens** | A read-scope API token gets 403. Every write method behind the login, plus the GET routes marked so. |

## Groups

| Group | Routes | Description |
|---|---|---|
| [Health](#health) | 1 | Liveness. |
| [Auth](#auth) | 9 | Setup, login, the account and short-lived purpose tokens. |
| [API tokens](#api-tokens) | 3 | Bearer tokens for scripts and AI agents. Dashboard session only. |
| [Projects](#projects) | 15 | Applications: create, configure, environment variables, persistent storage. |
| [Deployments](#deployments) | 14 | Deploy, redeploy, stop, restart, cancel, rollback, history, service domains. |
| [Databases](#databases) | 8 | Managed databases and links to apps. |
| [Domains](#domains) | 9 | Panel (dashboard) domains, SSL and the ACME email. |
| [Files](#files) | 3 | The project file editor. |
| [Logs](#logs) | 2 | Container logs. |
| [Ports](#ports) | 2 | The host port pool. |
| [GitHub](#github) | 15 | GitHub App setup, repositories and the push webhook. |
| [System](#system) | 11 | Host metrics, logs, version and host maintenance. |
| [Backup](#backup) | 12 | Backups and restore. |
| [Meta](#meta) | 5 | This document, the WebSocket terminal and the MCP endpoint. |

## Health {#health}

Liveness.

| Method | Path | Summary | Markers |
|---|---|---|---|
| `GET` | [`/api/health`](#get-api-health) | Health check |  |

### `GET /api/health` {#get-api-health}

Health check.

- **Auth:** none

## Auth {#auth}

Setup, login, the account and short-lived purpose tokens.

| Method | Path | Summary | Markers |
|---|---|---|---|
| `GET` | [`/api/auth/status`](#get-api-auth-status) | Setup status (whether an admin exists) |  |
| `POST` | [`/api/auth/register`](#post-api-auth-register) | Create the first admin |  |
| `POST` | [`/api/auth/login`](#post-api-auth-login) | Log in and get a 7 day session token |  |
| `GET` | [`/api/auth/me`](#get-api-auth-me) | The signed-in user |  |
| `PATCH` | [`/api/auth/profile`](#patch-api-auth-profile) | Change name and email | session only |
| `POST` | [`/api/auth/change-password`](#post-api-auth-change-password) | Change the account password | session only |
| `GET` | [`/api/auth/setup-token`](#get-api-auth-setup-token) | Setup token for a fresh-install restore |  |
| `POST` | [`/api/auth/sse-token`](#post-api-auth-sse-token) | 5 minute token for log streams (EventSource) | session only |
| `POST` | [`/api/auth/terminal-token`](#post-api-auth-terminal-token) | 5 minute single-use token for the WebSocket terminal | session only |

### `GET /api/auth/status` {#get-api-auth-status}

Setup status (whether an admin exists).

- **Auth:** none

### `POST /api/auth/register` {#post-api-auth-register}

Create the first admin.

Only while no user exists. Needs the bootstrap secret (header or body).

- **Auth:** none
- **Headers:**
  - `X-Bootstrap-Secret` (`string`): Bootstrap secret from the backend logs or data/.bootstrap-secret.
- **Body** (JSON):
  - `name` (`string`, required)
  - `email` (`string`, required)
  - `password` (`string`, required): At least 8 characters.
  - `bootstrapSecret` (`string`): Alternative to the X-Bootstrap-Secret header.

### `POST /api/auth/login` {#post-api-auth-login}

Log in and get a 7 day session token.

- **Auth:** none
- **Body** (JSON):
  - `email` (`string`, required)
  - `password` (`string`, required)

### `GET /api/auth/me` {#get-api-auth-me}

The signed-in user.

- **Auth:** `Authorization: Bearer` session or API token

### `PATCH /api/auth/profile` {#patch-api-auth-profile}

Change name and email.

- **Auth:** `Authorization: Bearer` session or API token
- **Markers:** session only
- **Body** (JSON):
  - `name` (`string`, required)
  - `email` (`string`, required)

### `POST /api/auth/change-password` {#post-api-auth-change-password}

Change the account password.

Ends every older session and revokes every API token created before the change.

- **Auth:** `Authorization: Bearer` session or API token
- **Markers:** session only
- **Body** (JSON):
  - `currentPassword` (`string`, required)
  - `newPassword` (`string`, required): At least 8 characters.

### `GET /api/auth/setup-token` {#get-api-auth-setup-token}

Setup token for a fresh-install restore.

Refused once a user exists.

- **Auth:** none
- **Headers:**
  - `X-Bootstrap-Secret` (`string`): Bootstrap secret from the backend logs or data/.bootstrap-secret.

### `POST /api/auth/sse-token` {#post-api-auth-sse-token}

5 minute token for log streams (EventSource).

- **Auth:** `Authorization: Bearer` session or API token
- **Markers:** session only

### `POST /api/auth/terminal-token` {#post-api-auth-terminal-token}

5 minute single-use token for the WebSocket terminal.

- **Auth:** `Authorization: Bearer` session or API token
- **Markers:** session only

## API tokens {#api-tokens}

Bearer tokens for scripts and AI agents. Dashboard session only.

| Method | Path | Summary | Markers |
|---|---|---|---|
| `GET` | [`/api/tokens`](#get-api-tokens) | List API tokens | session only |
| `POST` | [`/api/tokens`](#post-api-tokens) | Create an API token (returned once) | dangerous, session only |
| `DELETE` | [`/api/tokens/{id}`](#delete-api-tokens-id) | Revoke an API token | dangerous, session only |

### `GET /api/tokens` {#get-api-tokens}

List API tokens.

- **Auth:** `Authorization: Bearer` session or API token
- **Markers:** session only

### `POST /api/tokens` {#post-api-tokens}

Create an API token (returned once).

- **Auth:** `Authorization: Bearer` session or API token
- **Markers:** dangerous, session only
- **Body** (JSON):
  - `name` (`string`, required): 1-100 characters.
  - `scope` (`string`): read (default): GET only. full: every method. One of `read`, `full`.
  - `allowDangerous` (`boolean`): Allow actions that ask for the password in the dashboard. Needs scope full.
  - `expiresInDays` (`integer|null`): 1-3650; null or absent for no expiry.
  - `password` (`string`): Account password (step-up). Not needed with an API token created with allow_dangerous.

### `DELETE /api/tokens/{id}` {#delete-api-tokens-id}

Revoke an API token.

- **Auth:** `Authorization: Bearer` session or API token
- **Markers:** dangerous, session only
- **Path parameters:** `id`
- **Body** (JSON):
  - `password` (`string`): Account password (step-up). Not needed with an API token created with allow_dangerous.

## Projects {#projects}

Applications: create, configure, environment variables, persistent storage.

| Method | Path | Summary | Markers |
|---|---|---|---|
| `GET` | [`/api/projects`](#get-api-projects) | List projects |  |
| `POST` | [`/api/projects`](#post-api-projects) | Create a project from a GitHub repository or a ZIP upload | no read tokens |
| `GET` | [`/api/projects/{id}`](#get-api-projects-id) | Get a project with its services |  |
| `PATCH` | [`/api/projects/{id}`](#patch-api-projects-id) | Update project settings | no read tokens |
| `DELETE` | [`/api/projects/{id}`](#delete-api-projects-id) | Delete a project, its containers, images and files | no read tokens |
| `GET` | [`/api/projects/{id}/build/detect`](#get-api-projects-id-build-detect) | Detect how the project would be built |  |
| `GET` | [`/api/projects/{id}/storage`](#get-api-projects-id-storage) | Persistent volumes |  |
| `POST` | [`/api/projects/{id}/storage`](#post-api-projects-id-storage) | Add a persistent volume | no read tokens |
| `DELETE` | [`/api/projects/{id}/storage/{storageId}`](#delete-api-projects-id-storage-storageid) | Delete a persistent volume and its data | no read tokens |
| `GET` | [`/api/projects/{id}/env`](#get-api-projects-id-env) | List environment variables |  |
| `POST` | [`/api/projects/{id}/env`](#post-api-projects-id-env) | Create an environment variable | no read tokens |
| `POST` | [`/api/projects/{id}/env/bulk`](#post-api-projects-id-env-bulk) | Import KEY=VALUE lines | no read tokens |
| `DELETE` | [`/api/projects/{id}/env/{envId}`](#delete-api-projects-id-env-envid) | Delete an environment variable | no read tokens |
| `PATCH` | [`/api/projects/{id}/auto-deploy`](#patch-api-projects-id-auto-deploy) | Turn auto-deploy on push on or off (GitHub projects) | no read tokens |
| `GET` | [`/api/projects/{id}/auto-deploy`](#get-api-projects-id-auto-deploy) | Auto-deploy settings and webhook URL |  |

### `GET /api/projects` {#get-api-projects}

List projects.

- **Auth:** `Authorization: Bearer` session or API token

### `POST /api/projects` {#post-api-projects}

Create a project from a GitHub repository or a ZIP upload.

A GitHub repository is cloned now; a ZIP (multipart field `files`) is extracted now. Deploy it afterwards.

- **Auth:** `Authorization: Bearer` session or API token
- **Markers:** no read tokens
- **Body** (JSON or multipart/form-data):
  - `name` (`string`, required): Max 120 characters.
  - `source_type` (`string`): Default: upload when a file is sent, else github when github_url is set. One of `github`, `upload`.
  - `github_url` (`string`): https://github.com/owner/repo
  - `github_branch` (`string`): Branch or tag.
  - `description` (`string`)
  - `domain` (`string`): Comma-separated domains.
  - `project_type` (`string`): Default app.
  - `files` (`file`): ZIP of the source (multipart only, max 100 MiB).
  - `build_type` (`string`): auto (default), dockerfile or railpack; anything else means auto. One of `auto`, `dockerfile`, `railpack`.
  - `base_directory` (`string`): Subdirectory to build from (default ".").
  - `dockerfile_path` (`string|null`): Explicit Dockerfile path inside the project.
  - `internal_port` (`integer|string`): Port the app listens on (default 3000, clamped to 1-65535).

### `GET /api/projects/{id}` {#get-api-projects-id}

Get a project with its services.

- **Auth:** `Authorization: Bearer` session or API token
- **Path parameters:** `id`

### `PATCH /api/projects/{id}` {#patch-api-projects-id}

Update project settings.

- **Auth:** `Authorization: Bearer` session or API token
- **Markers:** no read tokens
- **Path parameters:** `id`
- **Body** (JSON):
  - `name` (`string`)
  - `description` (`string|null`)
  - `github_url` (`string|null`)
  - `domain` (`string|null`)
  - `project_type` (`string`)
  - `publish_host_port` (`boolean`): Publish host ports from the pool (opt-in).
  - `build_type` (`string`): auto (default), dockerfile or railpack; anything else means auto. One of `auto`, `dockerfile`, `railpack`.
  - `base_directory` (`string`): Subdirectory to build from (default ".").
  - `dockerfile_path` (`string|null`): Explicit Dockerfile path inside the project.
  - `internal_port` (`integer|string`): Port the app listens on (default 3000, clamped to 1-65535).

### `DELETE /api/projects/{id}` {#delete-api-projects-id}

Delete a project, its containers, images and files.

- **Auth:** `Authorization: Bearer` session or API token
- **Markers:** no read tokens
- **Path parameters:** `id`

### `GET /api/projects/{id}/build/detect` {#get-api-projects-id-build-detect}

Detect how the project would be built.

- **Auth:** `Authorization: Bearer` session or API token
- **Path parameters:** `id`

### `GET /api/projects/{id}/storage` {#get-api-projects-id-storage}

Persistent volumes.

- **Auth:** `Authorization: Bearer` session or API token
- **Path parameters:** `id`

### `POST /api/projects/{id}/storage` {#post-api-projects-id-storage}

Add a persistent volume.

- **Auth:** `Authorization: Bearer` session or API token
- **Markers:** no read tokens
- **Path parameters:** `id`
- **Body** (JSON):
  - `name` (`string`, required): 1-32 letters, numbers, _ or -.
  - `service_name` (`string`, required)
  - `mount_path` (`string`, required): Absolute path in the container.

### `DELETE /api/projects/{id}/storage/{storageId}` {#delete-api-projects-id-storage-storageid}

Delete a persistent volume and its data.

- **Auth:** `Authorization: Bearer` session or API token
- **Markers:** no read tokens
- **Path parameters:** `id`, `storageId`
- **Query parameters:**
  - `removeVolume` (`string`, required): Confirms permanent data deletion. One of `true`.

### `GET /api/projects/{id}/env` {#get-api-projects-id-env}

List environment variables.

- **Auth:** `Authorization: Bearer` session or API token
- **Path parameters:** `id`
- **Query parameters:**
  - `service` (`string`): Only this service scope.

### `POST /api/projects/{id}/env` {#post-api-projects-id-env}

Create an environment variable.

- **Auth:** `Authorization: Bearer` session or API token
- **Markers:** no read tokens
- **Path parameters:** `id`
- **Body** (JSON):
  - `key` (`string`, required): Letters, digits and _, not starting with a digit.
  - `value` (`string`)
  - `is_secret` (`boolean`): Pass a build variable as a BuildKit secret.
  - `service_name` (`string|null`): Service the variable applies to; empty or absent means all services.
  - `is_build_arg` (`boolean|null`): Pass to the image build.
  - `is_runtime` (`boolean|null`): Set in the running container (default true).

### `POST /api/projects/{id}/env/bulk` {#post-api-projects-id-env-bulk}

Import KEY=VALUE lines.

- **Auth:** `Authorization: Bearer` session or API token
- **Markers:** no read tokens
- **Path parameters:** `id`
- **Body** (JSON):
  - `content` (`string`, required): .env text.
  - `service_name` (`string|null`): Service the variable applies to; empty or absent means all services.
  - `is_build_arg` (`boolean|null`): Pass to the image build.
  - `is_runtime` (`boolean|null`): Set in the running container (default true).

### `DELETE /api/projects/{id}/env/{envId}` {#delete-api-projects-id-env-envid}

Delete an environment variable.

- **Auth:** `Authorization: Bearer` session or API token
- **Markers:** no read tokens
- **Path parameters:** `id`, `envId`

### `PATCH /api/projects/{id}/auto-deploy` {#patch-api-projects-id-auto-deploy}

Turn auto-deploy on push on or off (GitHub projects).

- **Auth:** `Authorization: Bearer` session or API token
- **Markers:** no read tokens
- **Path parameters:** `id`
- **Body** (JSON):
  - `enabled` (`boolean|null`)

### `GET /api/projects/{id}/auto-deploy` {#get-api-projects-id-auto-deploy}

Auto-deploy settings and webhook URL.

- **Auth:** `Authorization: Bearer` session or API token
- **Path parameters:** `id`

## Deployments {#deployments}

Deploy, redeploy, stop, restart, cancel, rollback, history, service domains.

| Method | Path | Summary | Markers |
|---|---|---|---|
| `GET` | [`/api/deployments/{projectId}`](#get-api-deployments-projectid) | Deployment history, newest first |  |
| `GET` | [`/api/deployments/{projectId}/services`](#get-api-deployments-projectid-services) | Services of a project |  |
| `GET` | [`/api/deployments/{projectId}/services/{serviceId}/ssl`](#get-api-deployments-projectid-services-serviceid-ssl) | SSL status of a service's domains |  |
| `POST` | [`/api/deployments/{projectId}/services/{serviceId}/dns-check`](#post-api-deployments-projectid-services-serviceid-dns-check) | Check DNS of a service's domains | no read tokens |
| `POST` | [`/api/deployments/{projectId}/services/{serviceId}/ssl/retry`](#post-api-deployments-projectid-services-serviceid-ssl-retry) | Retry HTTPS certificates for a service | no read tokens |
| `PUT` | [`/api/deployments/{projectId}/services/{serviceId}`](#put-api-deployments-projectid-services-serviceid) | Set a service's domains | no read tokens |
| `POST` | [`/api/deployments/{projectId}/deploy`](#post-api-deployments-projectid-deploy) | Deploy (streamed build log) | streams, no read tokens |
| `POST` | [`/api/deployments/{projectId}/redeploy`](#post-api-deployments-projectid-redeploy) | Redeploy (streamed build log) | streams, no read tokens |
| `POST` | [`/api/deployments/{projectId}/stop`](#post-api-deployments-projectid-stop) | Stop the project's containers (streamed log) | streams, no read tokens |
| `POST` | [`/api/deployments/{projectId}/restart`](#post-api-deployments-projectid-restart) | Restart the project's containers (streamed log) | streams, no read tokens |
| `POST` | [`/api/deployments/{projectId}/rollback`](#post-api-deployments-projectid-rollback) | Roll back to an earlier deployment's images (streamed log) | dangerous, streams, no read tokens |
| `POST` | [`/api/deployments/{projectId}/cancel`](#post-api-deployments-projectid-cancel) | Cancel the running build (streamed log) | streams, no read tokens |
| `GET` | [`/api/deployments/{projectId}/logs`](#get-api-deployments-projectid-logs) | Last 100 log lines of every service |  |
| `GET` | [`/api/deployments/{projectId}/stats`](#get-api-deployments-projectid-stats) | CPU and memory of every service |  |

### `GET /api/deployments/{projectId}` {#get-api-deployments-projectid}

Deployment history, newest first.

- **Auth:** `Authorization: Bearer` session or API token
- **Path parameters:** `projectId`
- **Query parameters:**
  - `limit` (`integer`): Page size (default 10).
  - `offset` (`integer`)
  - `status` (`string`): Filter by status.
  - `meta` (`string`): 1 or true: answer {items, total} instead of an array. One of `1`, `true`.

### `GET /api/deployments/{projectId}/services` {#get-api-deployments-projectid-services}

Services of a project.

- **Auth:** `Authorization: Bearer` session or API token
- **Path parameters:** `projectId`

### `GET /api/deployments/{projectId}/services/{serviceId}/ssl` {#get-api-deployments-projectid-services-serviceid-ssl}

SSL status of a service's domains.

- **Auth:** `Authorization: Bearer` session or API token
- **Path parameters:** `projectId`, `serviceId`

### `POST /api/deployments/{projectId}/services/{serviceId}/dns-check` {#post-api-deployments-projectid-services-serviceid-dns-check}

Check DNS of a service's domains.

- **Auth:** `Authorization: Bearer` session or API token
- **Markers:** no read tokens
- **Path parameters:** `projectId`, `serviceId`
- **Body** (JSON):
  - `domains` (`array<string>`): Domains to check (max 10); default the service's domains.

### `POST /api/deployments/{projectId}/services/{serviceId}/ssl/retry` {#post-api-deployments-projectid-services-serviceid-ssl-retry}

Retry HTTPS certificates for a service.

- **Auth:** `Authorization: Bearer` session or API token
- **Markers:** no read tokens
- **Path parameters:** `projectId`, `serviceId`

### `PUT /api/deployments/{projectId}/services/{serviceId}` {#put-api-deployments-projectid-services-serviceid}

Set a service's domains.

Writes the nginx vhost and requests certificates. An empty string or null removes every domain.

- **Auth:** `Authorization: Bearer` session or API token
- **Markers:** no read tokens
- **Path parameters:** `projectId`, `serviceId`
- **Body** (JSON):
  - `domain` (`string|null`): Comma-separated domain list.

### `POST /api/deployments/{projectId}/deploy` {#post-api-deployments-projectid-deploy}

Deploy (streamed build log).

The deploy keeps running if the client disconnects.

- **Auth:** `Authorization: Bearer` session or API token
- **Markers:** streams, no read tokens
- **Path parameters:** `projectId`
- **Body** (JSON):
  - `trigger` (`string`): Label stored on the deployment (default manual).
  - `commit_message` (`string`)
- **Response:** streamed `text/plain`

### `POST /api/deployments/{projectId}/redeploy` {#post-api-deployments-projectid-redeploy}

Redeploy (streamed build log).

- **Auth:** `Authorization: Bearer` session or API token
- **Markers:** streams, no read tokens
- **Path parameters:** `projectId`
- **Body** (JSON):
  - `trigger` (`string`)
  - `commit_message` (`string`)
- **Response:** streamed `text/plain`

### `POST /api/deployments/{projectId}/stop` {#post-api-deployments-projectid-stop}

Stop the project's containers (streamed log).

- **Auth:** `Authorization: Bearer` session or API token
- **Markers:** streams, no read tokens
- **Path parameters:** `projectId`
- **Response:** streamed `text/plain`

### `POST /api/deployments/{projectId}/restart` {#post-api-deployments-projectid-restart}

Restart the project's containers (streamed log).

- **Auth:** `Authorization: Bearer` session or API token
- **Markers:** streams, no read tokens
- **Path parameters:** `projectId`
- **Response:** streamed `text/plain`

### `POST /api/deployments/{projectId}/rollback` {#post-api-deployments-projectid-rollback}

Roll back to an earlier deployment's images (streamed log).

- **Auth:** `Authorization: Bearer` session or API token
- **Markers:** dangerous, streams, no read tokens
- **Path parameters:** `projectId`
- **Body** (JSON):
  - `deploymentId` (`string`, required)
  - `password` (`string`): Account password (step-up). Not needed with an API token created with allow_dangerous.
- **Response:** streamed `text/plain`

### `POST /api/deployments/{projectId}/cancel` {#post-api-deployments-projectid-cancel}

Cancel the running build (streamed log).

- **Auth:** `Authorization: Bearer` session or API token
- **Markers:** streams, no read tokens
- **Path parameters:** `projectId`
- **Response:** streamed `text/plain`

### `GET /api/deployments/{projectId}/logs` {#get-api-deployments-projectid-logs}

Last 100 log lines of every service.

- **Auth:** `Authorization: Bearer` session or API token
- **Path parameters:** `projectId`

### `GET /api/deployments/{projectId}/stats` {#get-api-deployments-projectid-stats}

CPU and memory of every service.

- **Auth:** `Authorization: Bearer` session or API token
- **Path parameters:** `projectId`

## Databases {#databases}

Managed databases and links to apps.

| Method | Path | Summary | Markers |
|---|---|---|---|
| `GET` | [`/api/databases/engines`](#get-api-databases-engines) | Supported database engines and versions |  |
| `GET` | [`/api/databases`](#get-api-databases) | List managed databases |  |
| `GET` | [`/api/databases/links/by-app/{appProjectId}`](#get-api-databases-links-by-app-appprojectid) | Database links of an app project |  |
| `POST` | [`/api/databases`](#post-api-databases) | Create (and start) a managed database | no read tokens |
| `GET` | [`/api/databases/{id}/connection`](#get-api-databases-id-connection) | Connection details and credentials |  |
| `GET` | [`/api/databases/{id}/links`](#get-api-databases-id-links) | Apps linked to a database |  |
| `POST` | [`/api/databases/{id}/links`](#post-api-databases-id-links) | Link a database to an app (injects a connection URL env var) | no read tokens |
| `DELETE` | [`/api/databases/{id}/links/{linkId}`](#delete-api-databases-id-links-linkid) | Remove a database link | no read tokens |

### `GET /api/databases/engines` {#get-api-databases-engines}

Supported database engines and versions.

- **Auth:** `Authorization: Bearer` session or API token

### `GET /api/databases` {#get-api-databases}

List managed databases.

- **Auth:** `Authorization: Bearer` session or API token

### `GET /api/databases/links/by-app/{appProjectId}` {#get-api-databases-links-by-app-appprojectid}

Database links of an app project.

- **Auth:** `Authorization: Bearer` session or API token
- **Path parameters:** `appProjectId`

### `POST /api/databases` {#post-api-databases}

Create (and start) a managed database.

- **Auth:** `Authorization: Bearer` session or API token
- **Markers:** no read tokens
- **Body** (JSON):
  - `name` (`string`, required): Max 120 characters.
  - `engine` (`string`, required): An engine id from GET /api/databases/engines. One of `postgres`, `mysql`, `mariadb`, `redis`, `mongodb`.
  - `version` (`string`): Image tag; default is the recommended version. `tag` and `image` are accepted as aliases.

### `GET /api/databases/{id}/connection` {#get-api-databases-id-connection}

Connection details and credentials.

- **Auth:** `Authorization: Bearer` session or API token
- **Path parameters:** `id`

### `GET /api/databases/{id}/links` {#get-api-databases-id-links}

Apps linked to a database.

- **Auth:** `Authorization: Bearer` session or API token
- **Path parameters:** `id`

### `POST /api/databases/{id}/links` {#post-api-databases-id-links}

Link a database to an app (injects a connection URL env var).

- **Auth:** `Authorization: Bearer` session or API token
- **Markers:** no read tokens
- **Path parameters:** `id`
- **Body** (JSON):
  - `app_project_id` (`string`, required)
  - `service_name` (`string`): Service of the app that gets the variable; empty means every service.
  - `env_key` (`string`): Variable name (default from the engine, e.g. DATABASE_URL).
  - `overwrite` (`boolean`): Replace an existing variable with that key.

### `DELETE /api/databases/{id}/links/{linkId}` {#delete-api-databases-id-links-linkid}

Remove a database link.

- **Auth:** `Authorization: Bearer` session or API token
- **Markers:** no read tokens
- **Path parameters:** `id`, `linkId`

## Domains {#domains}

Panel (dashboard) domains, SSL and the ACME email.

| Method | Path | Summary | Markers |
|---|---|---|---|
| `GET` | [`/api/domains/ssl/email`](#get-api-domains-ssl-email) | ACME account email |  |
| `PUT` | [`/api/domains/ssl/email`](#put-api-domains-ssl-email) | Set the ACME account email | no read tokens |
| `GET` | [`/api/domains`](#get-api-domains) | Panel domains with SSL status and access state |  |
| `POST` | [`/api/domains`](#post-api-domains) | Add a panel domain | no read tokens |
| `POST` | [`/api/domains/dns-check`](#post-api-domains-dns-check) | Check DNS of hostnames | no read tokens |
| `GET` | [`/api/domains/{domain}/ssl`](#get-api-domains-domain-ssl) | SSL status of a panel domain |  |
| `POST` | [`/api/domains/{domain}/ssl/retry`](#post-api-domains-domain-ssl-retry) | Retry HTTPS for a panel domain | no read tokens |
| `PUT` | [`/api/domains/{domain}/access`](#put-api-domains-domain-access) | Only allow dashboard access through this domain | session only |
| `DELETE` | [`/api/domains/{domain}`](#delete-api-domains-domain) | Remove a panel domain | no read tokens |

### `GET /api/domains/ssl/email` {#get-api-domains-ssl-email}

ACME account email.

- **Auth:** `Authorization: Bearer` session or API token

### `PUT /api/domains/ssl/email` {#put-api-domains-ssl-email}

Set the ACME account email.

- **Auth:** `Authorization: Bearer` session or API token
- **Markers:** no read tokens
- **Body** (JSON):
  - `email` (`string|null`)

### `GET /api/domains` {#get-api-domains}

Panel domains with SSL status and access state.

- **Auth:** `Authorization: Bearer` session or API token

### `POST /api/domains` {#post-api-domains}

Add a panel domain.

- **Auth:** `Authorization: Bearer` session or API token
- **Markers:** no read tokens
- **Body** (JSON):
  - `domain` (`string`, required)
  - `port` (`integer|string`, required): Target port 1-65535 (the dashboard port for the panel).

### `POST /api/domains/dns-check` {#post-api-domains-dns-check}

Check DNS of hostnames.

- **Auth:** `Authorization: Bearer` session or API token
- **Markers:** no read tokens
- **Body** (JSON):
  - `domains` (`array<string>`, required)

### `GET /api/domains/{domain}/ssl` {#get-api-domains-domain-ssl}

SSL status of a panel domain.

- **Auth:** `Authorization: Bearer` session or API token
- **Path parameters:** `domain`

### `POST /api/domains/{domain}/ssl/retry` {#post-api-domains-domain-ssl-retry}

Retry HTTPS for a panel domain.

- **Auth:** `Authorization: Bearer` session or API token
- **Markers:** no read tokens
- **Path parameters:** `domain`

### `PUT /api/domains/{domain}/access` {#put-api-domains-domain-access}

Only allow dashboard access through this domain.

Turning it on needs the step-up password and a request through the proxy over HTTPS for that domain.

- **Auth:** `Authorization: Bearer` session or API token
- **Markers:** session only
- **Path parameters:** `domain`
- **Body** (JSON):
  - `domainOnly` (`boolean`, required)
  - `password` (`string`): Required when domainOnly is true.

### `DELETE /api/domains/{domain}` {#delete-api-domains-domain}

Remove a panel domain.

- **Auth:** `Authorization: Bearer` session or API token
- **Markers:** no read tokens
- **Path parameters:** `domain`

## Files {#files}

The project file editor.

| Method | Path | Summary | Markers |
|---|---|---|---|
| `GET` | [`/api/files/{projectId}`](#get-api-files-projectid) | File tree of a project |  |
| `GET` | [`/api/files/{projectId}/content`](#get-api-files-projectid-content) | Read a file |  |
| `PUT` | [`/api/files/{projectId}/content`](#put-api-files-projectid-content) | Write a file | no read tokens |

### `GET /api/files/{projectId}` {#get-api-files-projectid}

File tree of a project.

- **Auth:** `Authorization: Bearer` session or API token
- **Path parameters:** `projectId`

### `GET /api/files/{projectId}/content` {#get-api-files-projectid-content}

Read a file.

- **Auth:** `Authorization: Bearer` session or API token
- **Path parameters:** `projectId`
- **Query parameters:**
  - `path` (`string`, required): Path relative to the project.

### `PUT /api/files/{projectId}/content` {#put-api-files-projectid-content}

Write a file.

- **Auth:** `Authorization: Bearer` session or API token
- **Markers:** no read tokens
- **Path parameters:** `projectId`
- **Query parameters:**
  - `path` (`string`, required): Path relative to the project.
- **Body** (JSON):
  - `content` (`string`, required): Whole new content (max 10 MiB).

## Logs {#logs}

Container logs.

| Method | Path | Summary | Markers |
|---|---|---|---|
| `GET` | [`/api/logs/{projectId}/containers`](#get-api-logs-projectid-containers) | Containers of a project and their state |  |
| `GET` | [`/api/logs/{projectId}/stream/{containerName}`](#get-api-logs-projectid-stream-containername) | Follow a container's logs (SSE) | streams |

### `GET /api/logs/{projectId}/containers` {#get-api-logs-projectid-containers}

Containers of a project and their state.

- **Auth:** `Authorization: Bearer` session or API token
- **Path parameters:** `projectId`

### `GET /api/logs/{projectId}/stream/{containerName}` {#get-api-logs-projectid-stream-containername}

Follow a container's logs (SSE).

- **Auth:** `Authorization: Bearer` API token, or `?token=` (5 minute SSE token) for a dashboard session
- **Markers:** streams
- **Path parameters:** `projectId`, `containerName`
- **Query parameters:**
  - `token` (`string`): SSE token from POST /api/auth/sse-token (not needed with a Bearer API token).
  - `tail` (`integer`): Number of past lines (default 200; negative means all).
- **Response:** streamed `text/event-stream`

## Ports {#ports}

The host port pool.

| Method | Path | Summary | Markers |
|---|---|---|---|
| `GET` | [`/api/ports`](#get-api-ports) | Host port pool and Docker port mappings |  |
| `DELETE` | [`/api/ports/{port}`](#delete-api-ports-port) | Release a port from the pool | no read tokens |

### `GET /api/ports` {#get-api-ports}

Host port pool and Docker port mappings.

- **Auth:** `Authorization: Bearer` session or API token

### `DELETE /api/ports/{port}` {#delete-api-ports-port}

Release a port from the pool.

- **Auth:** `Authorization: Bearer` session or API token
- **Markers:** no read tokens
- **Path parameters:** `port`

## GitHub {#github}

GitHub App setup, repositories and the push webhook.

| Method | Path | Summary | Markers |
|---|---|---|---|
| `POST` | [`/api/github/manifest`](#post-api-github-manifest) | Start creating the GitHub App (manifest flow) | no read tokens |
| `GET` | [`/api/github/manifest/callback`](#get-api-github-manifest-callback) | GitHub App manifest callback (browser redirect) |  |
| `GET` | [`/api/github/app-status`](#get-api-github-app-status) | Whether the GitHub App is configured |  |
| `POST` | [`/api/github/check-installation`](#post-api-github-check-installation) | Look up the App installation | no read tokens |
| `POST` | [`/api/github/install-session`](#post-api-github-install-session) | Start an install session (returns the install URL) | no read tokens |
| `GET` | [`/api/github/install`](#get-api-github-install) | Redirect to the App install page |  |
| `GET` | [`/api/github/setup`](#get-api-github-setup) | GitHub App setup callback (browser redirect) |  |
| `GET` | [`/api/github/callback`](#get-api-github-callback) | Legacy GitHub callback (browser redirect) |  |
| `GET` | [`/api/github/status`](#get-api-github-status) | GitHub connection status |  |
| `GET` | [`/api/github/installations`](#get-api-github-installations) | App installations |  |
| `GET` | [`/api/github/repos`](#get-api-github-repos) | Repositories the App can access |  |
| `GET` | [`/api/github/branches`](#get-api-github-branches) | Branches of a repository |  |
| `GET` | [`/api/github/tags`](#get-api-github-tags) | Tags of a repository |  |
| `POST` | [`/api/github/disconnect`](#post-api-github-disconnect) | Forget the App installation | no read tokens |
| `POST` | [`/api/github/webhook`](#post-api-github-webhook) | GitHub push webhook |  |

### `POST /api/github/manifest` {#post-api-github-manifest}

Start creating the GitHub App (manifest flow).

- **Auth:** `Authorization: Bearer` session or API token
- **Markers:** no read tokens
- **Body** (JSON):
  - `appName` (`string`, required)
  - `returnUrl` (`string`): Dashboard URL to return to.

### `GET /api/github/manifest/callback` {#get-api-github-manifest-callback}

GitHub App manifest callback (browser redirect).

- **Auth:** none
- **Query parameters:**
  - `code` (`string`)
  - `state` (`string`)
  - `installation_id` (`string`)
- **Response:** `text/html`

### `GET /api/github/app-status` {#get-api-github-app-status}

Whether the GitHub App is configured.

- **Auth:** `Authorization: Bearer` session or API token

### `POST /api/github/check-installation` {#post-api-github-check-installation}

Look up the App installation.

- **Auth:** `Authorization: Bearer` session or API token
- **Markers:** no read tokens

### `POST /api/github/install-session` {#post-api-github-install-session}

Start an install session (returns the install URL).

- **Auth:** `Authorization: Bearer` session or API token
- **Markers:** no read tokens
- **Body** (JSON):
  - `returnUrl` (`string`): Alias: return_url.

### `GET /api/github/install` {#get-api-github-install}

Redirect to the App install page.

- **Auth:** `Authorization: Bearer` session or API token
- **Query parameters:**
  - `return_url` (`string`)
- **Response:** `text/html`

### `GET /api/github/setup` {#get-api-github-setup}

GitHub App setup callback (browser redirect).

- **Auth:** none
- **Query parameters:**
  - `installation_id` (`string`)
  - `state` (`string`)
- **Response:** `text/html`

### `GET /api/github/callback` {#get-api-github-callback}

Legacy GitHub callback (browser redirect).

- **Auth:** none
- **Query parameters:**
  - `installation_id` (`string`)
  - `state` (`string`)
- **Response:** `text/html`

### `GET /api/github/status` {#get-api-github-status}

GitHub connection status.

- **Auth:** `Authorization: Bearer` session or API token

### `GET /api/github/installations` {#get-api-github-installations}

App installations.

- **Auth:** `Authorization: Bearer` session or API token

### `GET /api/github/repos` {#get-api-github-repos}

Repositories the App can access.

- **Auth:** `Authorization: Bearer` session or API token
- **Query parameters:**
  - `owner` (`string`): Only this account.

### `GET /api/github/branches` {#get-api-github-branches}

Branches of a repository.

- **Auth:** `Authorization: Bearer` session or API token
- **Query parameters:**
  - `repo` (`string`, required): owner/name
  - `type` (`string`): Repository visibility hint used to pick the installation token.

### `GET /api/github/tags` {#get-api-github-tags}

Tags of a repository.

- **Auth:** `Authorization: Bearer` session or API token
- **Query parameters:**
  - `repo` (`string`, required): owner/name
  - `type` (`string`): Repository visibility hint used to pick the installation token.

### `POST /api/github/disconnect` {#post-api-github-disconnect}

Forget the App installation.

- **Auth:** `Authorization: Bearer` session or API token
- **Markers:** no read tokens

### `POST /api/github/webhook` {#post-api-github-webhook}

GitHub push webhook.

Signed by GitHub (X-Hub-Signature-256 over the raw body). Queues auto-deploys.

- **Auth:** GitHub webhook signature (`X-Hub-Signature-256`)
- **Headers:**
  - `X-Hub-Signature-256` (`string`, required)
  - `X-GitHub-Event` (`string`, required)
- **Body** (JSON):
  - `ref` (`string`)
  - `repository` (`object`)
  - `head_commit` (`object|null`)
  - `pusher` (`object`)
  - `deleted` (`boolean`)

## System {#system}

Host metrics, logs, version and host maintenance.

| Method | Path | Summary | Markers |
|---|---|---|---|
| `GET` | [`/api/system/stats`](#get-api-system-stats) | Host metrics |  |
| `GET` | [`/api/system/logs/{service}`](#get-api-system-logs-service) | Follow a Docklift container's logs (SSE) | streams |
| `GET` | [`/api/system/quick`](#get-api-system-quick) | CPU and memory percent |  |
| `GET` | [`/api/system/ip`](#get-api-system-ip) | Server IP addresses |  |
| `POST` | [`/api/system/purge`](#post-api-system-purge) | Remove unused Docklift images and build cache | dangerous, no read tokens |
| `POST` | [`/api/system/reboot`](#post-api-system-reboot) | Reboot the host | dangerous, no read tokens |
| `POST` | [`/api/system/reset`](#post-api-system-reset) | Restart the Docklift containers | dangerous, no read tokens |
| `POST` | [`/api/system/execute`](#post-api-system-execute) | Run a shell command on the host (30 s, 1 MiB output) | dangerous, no read tokens |
| `GET` | [`/api/system/version`](#get-api-system-version) | Version and update status |  |
| `POST` | [`/api/system/update-system`](#post-api-system-update-system) | Update the host OS packages | dangerous, no read tokens |
| `POST` | [`/api/system/upgrade`](#post-api-system-upgrade) | Upgrade Docklift | dangerous, no read tokens |

### `GET /api/system/stats` {#get-api-system-stats}

Host metrics.

- **Auth:** `Authorization: Bearer` session or API token

### `GET /api/system/logs/{service}` {#get-api-system-logs-service}

Follow a Docklift container's logs (SSE).

- **Auth:** `Authorization: Bearer` API token, or `?token=` (5 minute SSE token) for a dashboard session
- **Markers:** streams
- **Path parameters:** `service`
- **Query parameters:**
  - `token` (`string`): SSE token from POST /api/auth/sse-token (not needed with a Bearer API token).
  - `tail` (`integer`): Number of past lines (default 200; negative means all).
- **Response:** streamed `text/event-stream`

### `GET /api/system/quick` {#get-api-system-quick}

CPU and memory percent.

- **Auth:** `Authorization: Bearer` session or API token

### `GET /api/system/ip` {#get-api-system-ip}

Server IP addresses.

- **Auth:** `Authorization: Bearer` session or API token

### `POST /api/system/purge` {#post-api-system-purge}

Remove unused Docklift images and build cache.

- **Auth:** `Authorization: Bearer` session or API token
- **Markers:** dangerous, no read tokens
- **Body** (JSON):
  - `password` (`string`): Account password (step-up). Not needed with an API token created with allow_dangerous.

### `POST /api/system/reboot` {#post-api-system-reboot}

Reboot the host.

- **Auth:** `Authorization: Bearer` session or API token
- **Markers:** dangerous, no read tokens
- **Body** (JSON):
  - `password` (`string`): Account password (step-up). Not needed with an API token created with allow_dangerous.

### `POST /api/system/reset` {#post-api-system-reset}

Restart the Docklift containers.

- **Auth:** `Authorization: Bearer` session or API token
- **Markers:** dangerous, no read tokens
- **Body** (JSON):
  - `password` (`string`): Account password (step-up). Not needed with an API token created with allow_dangerous.

### `POST /api/system/execute` {#post-api-system-execute}

Run a shell command on the host (30 s, 1 MiB output).

- **Auth:** `Authorization: Bearer` session or API token
- **Markers:** dangerous, no read tokens
- **Body** (JSON):
  - `command` (`string`, required)
  - `password` (`string`): Account password (step-up). Not needed with an API token created with allow_dangerous.

### `GET /api/system/version` {#get-api-system-version}

Version and update status.

- **Auth:** `Authorization: Bearer` session or API token
- **Query parameters:**
  - `refresh` (`string`): Check for a new release now. One of `1`, `true`.

### `POST /api/system/update-system` {#post-api-system-update-system}

Update the host OS packages.

- **Auth:** `Authorization: Bearer` session or API token
- **Markers:** dangerous, no read tokens
- **Body** (JSON):
  - `password` (`string`): Account password (step-up). Not needed with an API token created with allow_dangerous.

### `POST /api/system/upgrade` {#post-api-system-upgrade}

Upgrade Docklift.

- **Auth:** `Authorization: Bearer` session or API token
- **Markers:** dangerous, no read tokens
- **Body** (JSON):
  - `password` (`string`): Account password (step-up). Not needed with an API token created with allow_dangerous.

## Backup {#backup}

Backups and restore.

| Method | Path | Summary | Markers |
|---|---|---|---|
| `GET` | [`/api/backup`](#get-api-backup) | List backups |  |
| `GET` | [`/api/backup/uploads`](#get-api-backup-uploads) | List uploaded backup files |  |
| `DELETE` | [`/api/backup/uploads/{filename}`](#delete-api-backup-uploads-filename) | Delete an uploaded backup file | no read tokens |
| `POST` | [`/api/backup/create`](#post-api-backup-create) | Create a backup (streamed log) | streams, no read tokens |
| `POST` | [`/api/backup/restore/{filename}`](#post-api-backup-restore-filename) | Restore a server-created backup (streamed log) | dangerous, streams, no read tokens |
| `DELETE` | [`/api/backup/{filename}`](#delete-api-backup-filename) | Delete a backup | no read tokens |
| `GET` | [`/api/backup/download/{filename}`](#get-api-backup-download-filename) | Download a backup ZIP | no read tokens |
| `POST` | [`/api/backup/upload`](#post-api-backup-upload) | Upload a backup ZIP without restoring it | no read tokens |
| `POST` | [`/api/backup/restore-upload`](#post-api-backup-restore-upload) | Upload and restore a backup in one request (streamed log) | dangerous, streams, no read tokens |
| `POST` | [`/api/backup/restore-from-upload/{filename}`](#post-api-backup-restore-from-upload-filename) | Restore a previously uploaded backup (streamed log) | dangerous, streams, no read tokens |
| `POST` | [`/api/backup/clear-critical-restore`](#post-api-backup-clear-critical-restore) | Clear the restore-critical seal | dangerous, no read tokens |
| `GET` | [`/api/backup/critical-status`](#get-api-backup-critical-status) | Restore-critical seal status |  |

### `GET /api/backup` {#get-api-backup}

List backups.

- **Auth:** `Authorization: Bearer` session or API token

### `GET /api/backup/uploads` {#get-api-backup-uploads}

List uploaded backup files.

- **Auth:** `Authorization: Bearer` session or API token

### `DELETE /api/backup/uploads/{filename}` {#delete-api-backup-uploads-filename}

Delete an uploaded backup file.

- **Auth:** `Authorization: Bearer` session or API token
- **Markers:** no read tokens
- **Path parameters:** `filename`

### `POST /api/backup/create` {#post-api-backup-create}

Create a backup (streamed log).

- **Auth:** `Authorization: Bearer` session or API token
- **Markers:** streams, no read tokens
- **Body** (JSON):
  - `name` (`string`): Label for the file name (default docklift).
- **Response:** streamed `text/plain`

### `POST /api/backup/restore/{filename}` {#post-api-backup-restore-filename}

Restore a server-created backup (streamed log).

Replaces the database and project state; the server restarts after a committed restore.

- **Auth:** `Authorization: Bearer` session or API token
- **Markers:** dangerous, streams, no read tokens
- **Path parameters:** `filename`
- **Body** (JSON):
  - `password` (`string`): Account password (step-up). Not needed with an API token created with allow_dangerous.
- **Response:** streamed `text/plain`

### `DELETE /api/backup/{filename}` {#delete-api-backup-filename}

Delete a backup.

- **Auth:** `Authorization: Bearer` session or API token
- **Markers:** no read tokens
- **Path parameters:** `filename`

### `GET /api/backup/download/{filename}` {#get-api-backup-download-filename}

Download a backup ZIP.

- **Auth:** `Authorization: Bearer` session or API token
- **Markers:** no read tokens
- **Path parameters:** `filename`
- **Response:** `application/zip`

### `POST /api/backup/upload` {#post-api-backup-upload}

Upload a backup ZIP without restoring it.

- **Auth:** `Authorization: Bearer` session or API token
- **Markers:** no read tokens
- **Body** (multipart/form-data):
  - `backup` (`file`, required): The backup ZIP (max 500 MiB).

### `POST /api/backup/restore-upload` {#post-api-backup-restore-upload}

Upload and restore a backup in one request (streamed log).

Before the first admin exists it is authorised by the X-Setup-Token header instead.

- **Auth:** `Authorization: Bearer` session or API token; before the first admin exists, the `X-Setup-Token` header instead
- **Markers:** dangerous, streams, no read tokens
- **Headers:**
  - `X-Setup-Token` (`string`): Fresh install only.
- **Body** (multipart/form-data):
  - `backup` (`file`, required): The backup ZIP (max 500 MiB).
  - `password` (`string`): Account password (step-up). Not needed with an API token created with allow_dangerous.
- **Response:** streamed `text/plain`

### `POST /api/backup/restore-from-upload/{filename}` {#post-api-backup-restore-from-upload-filename}

Restore a previously uploaded backup (streamed log).

- **Auth:** `Authorization: Bearer` session or API token
- **Markers:** dangerous, streams, no read tokens
- **Path parameters:** `filename`
- **Body** (JSON):
  - `password` (`string`): Account password (step-up). Not needed with an API token created with allow_dangerous.
- **Response:** streamed `text/plain`

### `POST /api/backup/clear-critical-restore` {#post-api-backup-clear-critical-restore}

Clear the restore-critical seal.

- **Auth:** `Authorization: Bearer` session or API token
- **Markers:** dangerous, no read tokens
- **Body** (JSON):
  - `password` (`string`): Account password (step-up). Not needed with an API token created with allow_dangerous.

### `GET /api/backup/critical-status` {#get-api-backup-critical-status}

Restore-critical seal status.

- **Auth:** `Authorization: Bearer` session or API token

## Meta {#meta}

This document, the WebSocket terminal and the MCP endpoint.

| Method | Path | Summary | Markers |
|---|---|---|---|
| `GET` | [`/api/openapi.json`](#get-api-openapi-json) | This OpenAPI document |  |
| `GET` | [`/ws/terminal`](#get-ws-terminal) | WebSocket host terminal (upgrade) | session only |
| `POST` | [`/mcp`](#post-mcp) | MCP endpoint (Streamable HTTP, stateless JSON-RPC) |  |
| `GET` | [`/mcp`](#get-mcp) | MCP endpoint: no server stream (405) |  |
| `DELETE` | [`/mcp`](#delete-mcp) | MCP endpoint: no sessions (405) |  |

### `GET /api/openapi.json` {#get-api-openapi-json}

This OpenAPI document.

- **Auth:** `Authorization: Bearer` session or API token

### `GET /ws/terminal` {#get-ws-terminal}

WebSocket host terminal (upgrade).

Needs a trusted Origin and ?token= from POST /api/auth/terminal-token, then the account password inside the socket.

- **Auth:** single-use terminal token (`?token=` from `POST /api/auth/terminal-token`)
- **Markers:** session only
- **Query parameters:**
  - `token` (`string`, required)

### `POST /mcp` {#post-mcp}

MCP endpoint (Streamable HTTP, stateless JSON-RPC).

Model Context Protocol for AI agents. Bearer API token only. See https://docklift.dev/guide/mcp

- **Auth:** `Authorization: Bearer dl_...` API token only
- **Headers:**
  - `MCP-Protocol-Version` (`string`)
  - `Accept` (`string`, required): application/json, text/event-stream
- **Body** (JSON):
  - `jsonrpc` (`string`, required)
  - `id` (`integer|string`)
  - `method` (`string`, required)
  - `params` (`object`)

### `GET /mcp` {#get-mcp}

MCP endpoint: no server stream (405).

- **Auth:** `Authorization: Bearer dl_...` API token only

### `DELETE /mcp` {#delete-mcp}

MCP endpoint: no sessions (405).

- **Auth:** `Authorization: Bearer dl_...` API token only
