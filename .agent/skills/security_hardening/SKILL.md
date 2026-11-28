---
name: Security Hardening
description: Security patterns, guards, and limits enforced in the Docklift Go backend. Follow them when adding features.
---

# Security Hardening Guide

This skill lists the security controls of the Go backend (`server/`) and the stack around it. Follow these conventions when adding endpoints or features. Token and password details are in the `authentication` skill; the GitHub App flow is in `github_integration`.

## Stack and Trust Boundaries

Three containers (`docker-compose.yml`):

| Container | Role |
|-----------|------|
| `docklift-backend` | Go binary: API, WebSocket terminal, embedded dashboard. Listens on 8000 in the container, published as host `:8080`. Runs as root, `privileged: true`, `pid: host`, with the Docker socket mounted. |
| `docklift-nginx-proxy` | Public `:80`/`:443` for app domains and the optional panel domain. Fixed address `172.28.255.254` / `fd12:3456:789a::ffff:fffe` on `docklift_network`. |
| `docklift-certbot` | Let's Encrypt renewals. |

The backend can control the host, so anything that reaches the API with a valid session is effectively root on the host. Treat every route accordingly.

**Build trust**: all projects are built by the same Docker daemon and share one BuildKit cache. A hostile repository can influence what other projects build. Deploy only repositories you trust.

## Authentication Summary

-   JWT HS256 only, `JWT_SECRET` from the environment or `DATA_PATH/.secrets`. Sessions last 7 days; SSE and terminal tokens last 5 minutes.
-   `pwdv` must equal the user's `passwordChangedAt` (Unix ms). A password change ends all older tokens.
-   `RequireSession` accepts only `Authorization: Bearer <session JWT>`. Query tokens exist only for log streams (`RequireSSE`) and the terminal upgrade, and only with the matching `purpose`.
-   **`X-Internal-Secret` is no longer accepted.** `INTERNAL_API_SECRET` is still read and generated for compatibility and grants nothing.
-   **Request bodies are parsed only after rate limiting and authentication.** Each route group in `server/internal/app/router/router.go` applies limiter, guard, then body parser. Never add a body parser in front of a guard.
-   Backup downloads use `fetch` with the Bearer header and a blob. Never put a token in a URL except the 5 minute purpose tokens.

## Password Step-up

`Service.RequireStepUpPassword` (`server/internal/auth/service.go`) re-verifies the account password from the JSON body.

-   Required for: all backup restore routes and `clear-critical-restore`, `POST /api/system/purge`, `reboot`, `reset`, `update-system`, `upgrade`, `POST /api/system/execute`, and `POST /api/deployments/:projectId/rollback`.
-   **Throttled to 10 wrong passwords per user and client address per 15 minutes**, then 429 with `Retry-After`. A correct password resets the count.
-   The frontend must abort when the operator cancels the prompt.
-   A fresh-install restore authorised by the setup token skips step-up because no admin exists yet.

## Rate Limits and Client Address

All limits are per client address, fixed 15 minute windows, in memory (`server/internal/httpx/ratelimit.go`, bounded at 50,000 tracked clients).

| Scope | Limit |
|-------|-------|
| `/api/auth/*` | 100 requests |
| `/api/auth/register`, `/api/auth/setup-token` | also 10 requests |
| `/api/auth/me`, `/sse-token`, `/terminal-token` | own limiter, 1000 requests (not in the shared auth limit) |
| step-up password failures | 10 per user and address |
| terminal password failures | 5 per session, 20 per address and user |

IPv6 clients are keyed by their `/56`.

### Trusted proxy model (`TRUSTED_PROXIES`)

`X-Forwarded-For`, `X-Forwarded-Proto` and `X-Forwarded-Host` are honoured **only when the socket peer is trusted** (`server/internal/httpx/proxy.go`, `server/internal/config/proxies.go`).

-   Default: `127.0.0.0/8,::1/128,172.28.255.254,fd12:3456:789a::ffff:fffe`, which is loopback plus the single fixed address of `docklift-nginx-proxy`. Deployed apps on `docklift_network` and the network gateway (which fronts clients of `:8080`) are **not** trusted, so they cannot forge their address, scheme or host.
-   Syntax: comma-separated IPs or CIDRs, `!entry` excludes, the single value `none` trusts no peer.
-   With a trusted peer the client address is the **last** `X-Forwarded-For` entry (the one nginx appended).
-   If you put another reverse proxy in front, add it to `TRUSTED_PROXIES`, otherwise every user shares that proxy's rate-limit bucket.
-   Do not widen the default to the whole Docker subnet.

## Security Headers and CSP

-   `httpx.SecurityHeaders` sets `X-Content-Type-Options: nosniff`, `X-Frame-Options: SAMEORIGIN`, `X-XSS-Protection`, `Referrer-Policy: strict-origin-when-cross-origin`, `Permissions-Policy` (camera, microphone, geolocation off). `Strict-Transport-Security` (1 year, includeSubDomains) only when the request itself was HTTPS (TLS, or `X-Forwarded-Proto: https` from a trusted proxy).
-   The dashboard responses (`server/web/web.go`) add a `Content-Security-Policy`: scripts from `'self'`, the one pinned Monaco path on jsDelivr found in the bundle, and hashes of inline scripts (no `'unsafe-inline'` for scripts); `connect-src` is same origin, the request host over `ws:`/`wss:`, and `api.github.com`; `object-src 'none'`, `frame-ancestors 'self'`, `base-uri 'self'`. `/api/*` and `/ws/*` never fall through to the SPA; unknown API paths return JSON 404.
-   If the frontend starts loading a new third-party origin, the CSP must be updated in `web.go` in the same change.

## Origin Validation (CORS and WebSocket)

`server/internal/httpx/origin.go` is the single implementation, used by the CORS middleware and the terminal upgrade.

**Rule**: an origin is trusted only when scheme + host + port match the request the browser made, or it matches **exactly** an entry in `CORS_ORIGIN` or `DOCKLIFT_FRONTEND_URL`.

-   The request origin is rebuilt from `Host` (or, from a trusted proxy, `X-Forwarded-Host` / `X-Forwarded-Proto`).
-   A scheme mismatch is tolerated only when both sides use their default port (TLS terminated at the proxy).
-   Requests without `Origin` pass CORS (the JWT is still the gate). The terminal upgrade is stricter: no `Origin` is refused unless `TERMINAL_ALLOW_NO_ORIGIN=true`.
-   Never compare hostnames alone. Apps deployed on the same host under other ports or subdomains would then pass as the dashboard.
-   Every nginx panel template sets `proxy_set_header X-Forwarded-Host $http_host;` from the connection (`server/internal/nginx/templates.go`) so a client cannot supply its own. Keep that when adding templates.

## Maintenance Gate and Restore Seal

`httpx.MaintenanceGate` (`server/internal/httpx/gate.go`) with state from `server/internal/maintenance`:

-   During a restore (maintenance on) **every** `/api/*` request except `/api/health` and `/api/backup/restore*` gets 503.
-   While the **restore-critical seal** (`DATA_PATH/.restore-critical`, mode 0600) exists, only `/api/health`, `GET /api/backup/critical-status` and `POST /api/backup/clear-critical-restore` pass. The seal is re-applied at boot (`LoadCriticalOnBoot`), blocks all restores, and is removed only by `clear-critical-restore` with step-up. If the file cannot be deleted, maintenance stays on.
-   A global restore lock is taken before any upload byte is read; a second restore gets 409.
-   **Restore only starts Docklift-generated runtime compose files** (`DEPLOYMENTS_PATH/.docklift/<projectId>/compose.yml`), run from that directory. A repository's own `docker-compose.yml` is never started by a restore. Projects without a runtime file are skipped and need a manual deploy.

See `system_administration` for the full restore flow.

## File Modes

-   Data and backup directories (`DATA_PATH`, the database directory, `BACKUP_PATH`, `BACKUP_PATH/uploads`) are `0700`; files holding secrets or data are `0600`: the SQLite database and `-wal`/`-shm`, `.secrets`, `.bootstrap-secret`, `.setup-token`, `.restore-critical`, backup and uploaded archives, the generated runtime compose files, GitHub setup state files, and the GitHub App key file.
-   `backup.TightenModes` fixes older installs on every boot (`server/internal/backup/modes.go`). nginx and Let's Encrypt directories are excluded because other containers read them.
-   New code that writes secrets must create files with `0o600` and directories with `0o700`.

## Child Processes

All programs (docker, compose, buildx, railpack, git, the terminal shell, `/api/system/execute`) run through `server/internal/execx`:

-   Arguments are passed as an array, never through `sh -c` with user input. (`/api/system/execute` is the one deliberate shell: it runs the given command with `/bin/bash -c`, after step-up, a 30 s timeout, 1 MiB output cap, and kills the whole process group.)
-   `JWT_SECRET`, `INTERNAL_API_SECRET` and every `DOCKLIFT_GITHUB_*` variable are removed from the child environment (`IsSensitiveEnv`). Add new backend secrets to that list.
-   Errors name the program, not its arguments, because arguments can carry secrets.

## Build Inputs

-   Only variables the user marked as build variables are passed to `docker build`; secret build variables go as BuildKit secrets, never `--build-arg` (`server/internal/deploy/build_runner.go`). Forwarding every project variable would bake runtime secrets into image layers.
-   Reserved names (`PATH`, `HOME`, `PORT`, `NODE_OPTIONS`, `LD_PRELOAD`, `LD_LIBRARY_PATH`, `DOCKER_*`, `BUILDKIT_*`) are rejected as build variables.
-   `base_directory` and `dockerfile_path` go through `ResolveProjectPath` (`build_resolver.go`): absolute paths and `..` are refused.
-   User apps run on per-project networks (`ProjectNetworkName`). Do not put them on `docklift_network` by default.

## Paths and Files

`server/internal/pathsec`:

-   `IsInside(base, candidate)`: resolves both to absolute clean paths, rejects `..`, absolute escapes and sibling prefixes. Never use `strings.HasPrefix` on paths.
-   `TouchesGitDir`: the file editor never reads or writes `.git` (git runs hooks and config on the next pull). On Windows, spellings that map to `.git` (trailing dots, `::$DATA`, `GIT~1`) count too.
-   `RealPath` / `FinalPath`: resolve symlinks and junctions from the open handle, so a swap between check and use cannot escape. `OpenOverwrite` never follows a final symlink.
-   The file editor opens the file, resolves the handle's real path, and requires it to stay inside the real project directory and outside `.git`. Editable files are capped at 10 MiB.
-   Backup and upload names must match `^[a-zA-Z0-9._-]+\.zip$` without `..`, and their real path must stay inside the backup or uploads directory. Uploaded names are replaced by server-chosen `restore-<ms>-<hex>.zip`.

## Archives (`server/internal/safeunzip`)

-   Every archive goes through `safeunzip.Extract`. It rejects empty, absolute, NUL, `..` and (on Windows) drive or stream paths, re-checks the real parent directory against the destination (also through links already present), and never creates symlinks (a link entry is written as a regular file holding the link text).
-   Quotas count real bytes written: default 50,000 files and 2 GiB (restore); project ZIP uploads allow 100 MiB compressed, 20,000 files, 512 MiB expanded. Backup uploads allow 500 MiB, aligned with nginx `client_max_body_size 500M`.
-   Never extract with `archive/zip` directly.

## Git (`server/internal/gitops`)

-   git runs without a shell, with a timeout (10 min network, 2 min local), hooks disabled (`core.hooksPath=/dev/null`), no fsmonitor, no credential helper, no prompts, `GIT_*` variables from the parent dropped, and `GIT_CEILING_DIRECTORIES` so discovery cannot climb out of the project.
-   Only `https` is allowed (`GIT_ALLOW_PROTOCOL`): `http`, `ssh`, `file://`, `ext::` and local paths are refused. URLs starting with `-` or containing whitespace are refused; refs are validated (`ValidateRef`) and passed after `--` or `--end-of-options`.
-   **Credentials** (a GitHub App installation token) are sent as an `Authorization` header through `GIT_CONFIG_*` scoped to the remote origin, so the token is never written to `.git/config` or the process arguments. They are only sent to `https://github.com`, and on a pull only when the checkout's origin equals the project's stored URL; otherwise `ErrCredentialsOrigin`.
-   After clone and pull the origin is scrubbed (`ScrubOrigin`) and verified (`VerifyNoCredentials`). If that fails, fail the deploy or roll back project creation. Logs and errors go through `Redact` (URL userinfo and the token).

## Webhooks

`POST /api/github/webhook` (public):

1.  Only `push` events are processed; others get 200 "Ignoring".
2.  **Fail closed**: no webhook secret stored gives 401.
3.  A missing `X-Hub-Signature-256` gives 401 before the body is read.
4.  The HMAC-SHA256 is computed over the **raw body bytes** and compared with `hmac.Equal`. The JSON is decoded only after the signature matches.
5.  Body limit 10 MiB from the budget reserved for unauthenticated routes.
6.  Matching projects are queued, not deployed inline: per project at most one running and one pending deploy, the latest request replaces the pending one. A 10 second cooldown per project suppresses push bursts.

## Terminal

`/ws/terminal` (`server/internal/api/terminal`, `server/internal/terminal`):

-   Handled before the HTTP middleware stack, so only the upgrade checks run: path, `Origin`, then the **single-use terminal token** (5 min, random `jti`).
-   After connecting, the client has **60 seconds** (`AuthTimeout`) to send its password; 5 wrong passwords per session and 20 per `address:user` per 15 minutes.
-   Max 3 sessions, 30 minute idle timeout, 1 MiB client frame limit, 30 s ping with 75 s pong wait.
-   **Changing the password closes the user's terminals and SSE log streams.** The context is bound with `BindSession` and rechecked every 30 s for changes made by another process (`reset-password`).
-   Resize accepts only integers with `0 < cols,rows < 500`.
-   The shell is `/bin/bash` in `/root` on Linux, with the backend secrets removed from its environment. It is host root: keep every auth layer.

## Streaming Safety

-   SSE writes use a 30 s write deadline per event; after the first failed write, further writes are dropped and the Docker log connection is closed when the request context ends.
-   Docker errors are sent as events on a 200 stream so the browser shows the message.
-   Backup and restore keep running if the client disconnects; their progress writer ignores a dead client.

## Error Handling

-   Return fixed messages for failures (`Login failed`, `Failed to list backups`). Log the real error with `d.Log`. Panics are caught by `httpx.Recover` and answered `500 {"error":"Something went wrong!"}`.
-   Write `[AUDIT]` log lines for destructive host actions (reboot, reset, update, upgrade, purge) with `httpx.ClientIP(r)`; `[SECURITY]` for suspicious auth events.
-   Some operator-facing routes still echo `err.Error()` in a `details` field (system actions). Do not copy that into new routes.

## Infrastructure

-   `nginx-proxy/conf.d/default.conf` answers unknown hostnames with `444`, so the dashboard is never served on an unconfigured domain.
-   `nginx-proxy/nginx.conf` logs `$uri`, not the full request line, which keeps `?token=` query values out of access logs.
-   Keep the dashboard bind in mind: `:8080` is published on `0.0.0.0` by default. With an HTTPS panel domain, **Only allow access through the domain** (`internal/panelaccess`, settings row `panel_domain_only`) refuses or redirects every request whose socket peer is not a trusted proxy; it never trusts a header for this. Exceptions while it is on: `/api/health` (`{"status":"ok"}` only) and the HMAC-signed `POST /api/github/webhook`. Recovery is `docker exec docklift-backend docklift panel-access open` (applies within 2 s). `DASHBOARD_BIND=127.0.0.1` closes the port entirely.
-   Graceful shutdown on SIGINT/SIGTERM: the HTTP server drains for up to 10 s, terminal sessions are closed, shutdown hooks run.

## Checklist for New Features

-   [ ] The route sits in a group whose guard is `RequireSession` (or has a written reason to be public) and a rate limiter if public.
-   [ ] No body parsing before the guard; unauthenticated bodies use `publicBody`.
-   [ ] Dangerous host or data operations call `RequireStepUpPassword`.
-   [ ] Error responses are fixed strings; details are logged.
-   [ ] Paths use `pathsec` and real-path checks; archives use `safeunzip`.
-   [ ] Child processes go through `execx` with an argument array; new secrets are added to `IsSensitiveEnv`.
-   [ ] Files with secrets are `0600`, directories `0700`.
-   [ ] Cross-origin decisions use `httpx.IsTrustedOrigin`, forwarded headers are read only through the trusted proxy helpers (`ClientIP`, `RequestProtocol`).
-   [ ] Tokens go in headers; only 5 minute purpose tokens may appear in a query string, and only on the routes that accept them.
-   [ ] Long streams bind to the session (`BindSession`) so a password change ends them.
-   [ ] New nginx templates set `X-Forwarded-Host` from `$http_host`.
-   [ ] No host-wide prune or OS maintenance in normal product APIs; purge stays Docklift-scoped.
-   [ ] Behaviour changes update `.agent/skills`, README and `commands.md` in the same PR.
