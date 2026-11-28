---
name: GitHub Integration
description: Guide for setting up and managing Docklift's GitHub App integration in the Go backend.
---

# GitHub Integration Guide

Docklift integrates with GitHub through a GitHub App. The App gives access to private repositories (installation tokens) and sends `push` webhooks for auto-deploys.

## Architecture

-   **Routes**: `server/internal/api/github/` (`github.go` flow and status, `repos.go` repos/branches/tags, `webhook.go`, `origins.go` URL and return-URL rules), mounted at `/api/github`
-   **App credentials, JWTs, tokens, setup states**: `server/internal/githubapp/` (`app.go`, `client.go`, `state.go`)
-   **Git operations**: `server/internal/gitops/gitops.go` (clone, pull, scrub)
-   **Deploy queue**: `server/internal/api/deployments/queue.go`
-   **Callers that clone or pull with a token**: `server/internal/api/projects/create.go` (initial clone) and `server/internal/api/deployments/pipeline.go` (every deploy)
-   **Frontend helpers**: `startGithubInstallSession()` / `startGithubInstallAndNavigate()` in `frontend/src/lib/auth.ts`; UI in `frontend/src/components/GitHubConnect.tsx`, `BranchSelector.tsx` and `frontend/src/pages/NewProject.tsx`
-   **Callback base URL**: built from the request host (`publicBaseURL`), falling back to the origin of `DOCKLIFT_FRONTEND_URL`. Non-IP, non-loopback hosts are always `https`. So the panel must be reached on its public URL when you create the App.

## Authentication Model

-   **App auth**: an **RS256** JWT signed with the stored private key, valid 10 minutes (issued 60 s in the past for clock drift). `iss` is the numeric App ID.
-   **Installation auth**: the App JWT is exchanged for an installation token. Tokens are cached in memory per App ID and installation, reused until 5 minutes before expiry, and dropped on disconnect or when a new App is created (`ForgetTokens`).
-   Credentials come from the `settings` table first. `DOCKLIFT_GITHUB_APP_ID` and the key file at `DOCKLIFT_GITHUB_PRIVATE_KEY_PATH` (default `./github-app.pem`) are a fallback for older installs. `DOCKLIFT_GITHUB_*` variables are never passed to child processes.
-   `/api/github/*` is behind `RequireSession` except `/webhook`, `/callback`, `/manifest/callback` and `/setup`.

## Setup Flow (App Manifest)

1.  **Initiate**: the UI calls `POST /api/github/manifest` with `appName` and an optional `returnUrl`. The response is `{ manifest, action, serverUrl }`. The backend creates a one-time **state**, sets the `docklift_github_state` cookie, and puts the state on the GitHub form action (`https://github.com/settings/apps/new?state=...`).
2.  **Create**: the browser posts the manifest to GitHub. The App is created `public: true` with permissions `contents: read`, `metadata: read`, event `push`, hook URL `<base>/api/github/webhook`, setup URL `<base>/api/github/setup`.
3.  **Callback**: GitHub redirects to `/api/github/manifest/callback?code=...&state=...`. The backend exchanges the code for the App credentials (ID, slug, client id and secret, private key PEM, webhook secret) and stores them in `settings`.
4.  **Install**: a fresh state is created and the browser is redirected to the App's install page with `?state=`.
5.  **Setup**: GitHub redirects to `/api/github/setup?installation_id=...&state=...`. The backend fetches that installation with the App JWT, stores `github_installation_id`, username and avatar, clears the state and the cookie, and redirects to the saved return URL with `github=connected`.

`POST /api/github/install-session` (used by `startGithubInstallSession`) starts the same install step for an existing App: it sets the cookie and returns `{ installUrl }`.

### State and CSRF protection

The callbacks are public (GitHub calls them without a session), so they are guarded by a one-time `state` (`githubapp/state.go`, `requireState` in `github.go`):

-   State is 24 random bytes (48 hex chars), stored as `DATA_PATH/github-setup/<state>.json` (dir `0700`, file `0600`) with `{ createdAt, returnUrl }`. It expires after **1 hour**, and expired files are pruned on every use. The return URL is stored per state.
-   The cookie is `docklift_github_state`, `HttpOnly`, `SameSite=Lax`, `Path=/`, `Max-Age=3600`, `Secure` when the request was HTTPS.
-   **`/manifest/callback` and `/setup` require a `state` query parameter that equals the cookie** (constant-time compare) **and** names a valid, unexpired state file. The query alone or the cookie alone is refused with `github_error=invalid_state`.
-   After a successful step the state is deleted (single use). A legacy single `github_setup_state` settings key is still read for in-flight flows from old versions.
-   Return URLs are sanitised (`sanitizeReturnURL`): only relative paths, or http(s) URLs on the frontend origin, the request origin, or a `CORS_ORIGIN` entry; otherwise `<DOCKLIFT_FRONTEND_URL>/settings`.

### Legacy `/api/github/callback`

The old OAuth code exchange is gone. If `installation_id` is numeric it forwards to `/api/github/setup` (with `state`); otherwise it redirects to Settings with `github_error=legacy_oauth_disabled`.

### Errors shown in Settings

Redirects use `<DOCKLIFT_FRONTEND_URL>/settings?github_error=<code>`: `invalid_state`, `no_code`, `callback_failed`, `conversion_failed`, `invalid_installation`, `installation_verify_failed`, `setup_failed`.

## Installation Binding

The App is public, so anyone can install it. Only these installations are ever stored:

-   the one returned through a matching `/setup` state, or
-   in `POST /api/github/check-installation` (for localhost installs): the saved installation if still present, else the installation on the App owner's account. It never falls back to `installations[0]`.

`POST /api/github/disconnect` deletes the installation id, username and avatar (the App stays) and clears the token cache.

## Endpoints

| Endpoint | Purpose |
|----------|---------|
| `POST /manifest` | Build the manifest and start the setup state |
| `GET /manifest/callback` | Public. Exchange the manifest code |
| `GET /app-status` | Configured, installed, App info, install URL |
| `POST /check-installation` | Find the installation without waiting for `/setup` |
| `POST /install-session`, `GET /install` | Start an install with a fresh state |
| `GET /setup` | Public. Post-install redirect |
| `GET /callback` | Public. Legacy forwarder |
| `GET /status` | Connection status for Settings |
| `GET /installations` | All accounts the App is installed on |
| `GET /repos` | Repositories of all installations |
| `GET /branches`, `GET /tags` | Refs of one repository |
| `POST /disconnect` | Forget the installation |
| `POST /webhook` | Public. Push events |

## Repositories, Branches, Tags

-   **Repos**: `GET /api/github/repos` (optional `?owner=login`). Response `{ repositories, failedInstallations, fallbackSingle }`. It lists every installation (up to 8 fetched in parallel), de-duplicates by repo id, sorts by last activity. A failed installation is reported in `failedInstallations`, never silently dropped; if all fail the answer is 502. `fallbackSingle: true` means the installation list failed and only the saved installation was queried.
-   **Branches**: `GET /api/github/branches?repo=owner/name&type=public|private`.
-   **Tags**: `GET /api/github/tags?repo=owner/name&type=public|private`. Up to **500** tags (5 pages of 100).
-   A `private` repository without a resolvable installation is refused; `public` falls back to unauthenticated access.
-   UI: New Project has a Private tab with an "All" chip and one chip per connected account, and a Branch | Tag switcher. Push auto-deploy does not follow a tag pin.

## Cloning and Pulling

-   Only `https://github.com/...` repositories get an installation token (`GitHubOwnerRepo`; other hosts, ports, userinfo are refused). Stored URLs from older versions (`http`, `www.github.com`) are normalised by `NormalizeGitHubURL`.
-   The token is sent as an `Authorization` header via `GIT_CONFIG_*`, scoped to the origin, never written to `.git/config` or the arguments. On a pull, the checkout's origin must equal the project's stored URL or git is not run with credentials.
-   After clone and after every deploy pull, the origin is scrubbed and verified credential-free. A failure fails the deploy (or rolls back project creation).
-   Pull = `fetch --tags --prune --prune-tags --force`, then `reset --hard` to `origin/<branch>` or to the tag, then `clean -f -d`. A ref that is neither a remote branch nor a tag fails closed.
-   Details of the git hardening are in the `security_hardening` skill.

## Webhooks (Auto-Deploy)

`POST /api/github/webhook` is global (one URL for all projects); projects are matched by repository URL.

1.  `X-GitHub-Event` other than `push`: 200, ignored.
2.  No `github_webhook_secret` stored: **401** (fail closed). No `X-Hub-Signature-256`: 401.
3.  The body is read only now (limit 10 MiB, from the budget shared by unauthenticated routes), and the HMAC-SHA256 is computed over the **raw bytes**, compared with `hmac.Equal`. Mismatch: 401. JSON is decoded only after the signature is valid. This happens before any database access.
4.  Skip when `deleted` is true or `head_commit` is null (branch deletions, empty pushes).
5.  Match projects with `source_type = github`, `auto_deploy` on, and `github_url` equal to `clone_url` (or `html_url`) with or without `.git`.
6.  For each project: the pushed `refs/heads/*` or `refs/tags/*` name must equal the project's pinned branch or tag (else "ref mismatch"); other ref kinds are skipped; a project deployed in the last **10 seconds** is skipped ("cooldown").
7.  Matching projects are **queued per project** (`EnqueueDeploy`, trigger `webhook`): one deploy running plus at most one pending, a newer request replaces the pending one, and a busy project (manual deploy, rollback, stop) waits instead of dropping the request. The webhook answers GitHub immediately with `{ message, triggered, skipped, branch, commit }`.

There is no global concurrency cap across projects; many projects on one repository build in parallel.

## Troubleshooting

-   **Settings shows `github_error=invalid_state`**: the cookie was missing or expired (over 1 hour), the browser blocked cookies, or the flow started on another host name than it returned to. Start again from the Docklift UI on the same URL.
-   **Callback URL wrong or unreachable**: the App was created while the panel was opened on a different host. Set `DOCKLIFT_FRONTEND_URL` to the public URL and recreate the App.
-   **"Repositories not loading"**: check the App is installed on the target account; look at `failedInstallations`.
-   **"Webhook not triggering"**: compare the webhook secret in GitHub with the stored one, check Recent Deliveries for 401, confirm `auto_deploy` is on and the branch pin matches the pushed branch. The backend logs `[Webhook]` lines (`docker logs docklift-backend`).
-   **"Authentication failed"**: the private key or App ID is missing or invalid in `settings`.

## Settings Keys (`settings` table)

`github_app_id`, `github_app_slug`, `github_app_name`, `github_client_id`, `github_client_secret`, `github_private_key`, `github_webhook_secret`, `github_username`, `github_avatar_url`, `github_installation_id` (default installation), `github_pending_app_name`.

These live in the SQLite database, which is `0600`, and are included in backups (database snapshot). The private key is also copied from `DOCKLIFT_GITHUB_PRIVATE_KEY_PATH` into backups when that file exists.
