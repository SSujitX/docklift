---
name: Authentication System
description: Guide to the authentication and authorization system in the Docklift Go backend and dashboard.
---

# Authentication System Guide

Docklift authenticates API calls with HS256 JWTs signed by the Go backend (`server/`). The dashboard is a React app embedded in the same binary, so the API, the WebSocket terminal and the UI share one origin.

## Components

-   **Token and password code**: `server/internal/auth` (`jwt.go`, `password.go`, `secrets.go`, `service.go`, `sessions.go`, `bootstrap.go`)
-   **Routes**: `server/internal/api/auth/auth.go`, mounted at `/api/auth`
-   **Router, rate limiters and per-group guards**: `server/internal/app/router/router.go`
-   **Origin and proxy trust**: `server/internal/httpx` (`origin.go`, `proxy.go`)
-   **Terminal upgrade**: `server/internal/api/terminal/terminal.go`
-   **CLI**: `server/cmd/docklift` (`docklift reset-password`)
-   **Frontend context**: `frontend/src/components/AuthProvider.tsx`
-   **Frontend API helper**: `frontend/src/lib/auth.ts` (`getAuthHeaders`, `authFetch`, `fetchWithAuth`)
-   **Database**: SQLite `users` table (email, bcrypt hash, role, `passwordChangedAt`), accessed through `server/internal/store`.

## Secrets

`LoadOrCreateSecrets` (`secrets.go`) resolves the signing secret at startup:

1.  `JWT_SECRET` and `INTERNAL_API_SECRET` both set in the environment: use them.
2.  Else `DATA_PATH/.secrets` (JSON, mode 0600) when it holds both values.
3.  Else generate 64 random bytes (hex) for each missing value and write `.secrets`. An env value still wins for its half. If the write fails, sessions reset on restart.

`INTERNAL_API_SECRET` is still read and generated so existing `.secrets` files load and a rollback to 2.x works, but **it authenticates nothing**: the `X-Internal-Secret` header is no longer accepted anywhere (the webhook queues deploys in process).

Child processes never inherit `JWT_SECRET`, `INTERNAL_API_SECRET` or any `DOCKLIFT_GITHUB_*` variable (`server/internal/execx/env.go`). This covers docker, compose, git, railpack, `/api/system/execute` and the terminal shell.

## Tokens

-   **Algorithm**: HS256 only. The parser rejects every other algorithm (`jwt.WithValidMethods`).
-   **Claims**: `userId`, `email`, `role`, optional `purpose`, `pwdv`, `iat`, `exp`, and `jti` on terminal tokens.
-   **Session token**: 7 days, no `purpose`. Issued by `register`, `login`, and `change-password`.
-   **Purpose tokens**: 5 minutes. `purpose: "sse"` for log streams, `purpose: "terminal"` for the WebSocket. They never authenticate normal API calls.
-   **`pwdv`**: the user's `passwordChangedAt` in Unix ms (0 when unset). Every guard compares it with the stored value and answers 401 "Session expired. Please log in again." on mismatch or when the claim is missing. Database errors fail closed.

## Passwords

-   bcrypt cost 12 (`BcryptCost`). Hashes are written with the `$2b$` prefix and `$2a$`/`$2b$`/`$2y$` hashes verify. Input is truncated at bcrypt's 72 byte limit.
-   Minimum length 8 (UTF-16 units) on register and change-password.
-   Login for an unknown email still runs one bcrypt comparison (`EqualizeLoginTiming`) so response time does not reveal which emails exist. Both failures return 401 "Invalid email or password".
-   Emails are lower-cased on register, login and profile update.

## Auth Flow

1.  **Registration**: `POST /api/auth/register`
    -   Allowed only while the `users` table is empty. The first user is `admin`.
    -   Requires the **bootstrap secret** in header `X-Bootstrap-Secret` (or body `bootstrapSecret`). It is printed to the backend logs on a fresh install and stored in `DATA_PATH/.bootstrap-secret` (32 random bytes, hex, mode 0600). No public API returns it. `GET /api/auth/status` creates it but never returns it.
    -   **Claim**: `TryClaim` renames `.bootstrap-secret` to `.claimed-<random>`. Only the rename winner may create the admin. The claimed file is compared again in constant time, then, on any failure, `AbortClaim` puts the secret back. On success the claim, the secret and the setup token are deleted.
    -   Crash recovery: `RecoverStaleClaims` (run from `LogIfNeeded` at boot with no users) restores the newest `.claimed-*` file. With an admin present, leftover claims and the setup token are deleted.
2.  **Login**: `POST /api/auth/login` returns `{ message, token, user }`.
3.  **Profile**: `GET /api/auth/me`, `PATCH /api/auth/profile` (name and email).
4.  **Change password**: `POST /api/auth/change-password` with `currentPassword` and `newPassword`. It sets `passwordChangedAt`, so every older session token stops working, returns a fresh token for the caller, and **closes the user's open log streams and terminal sessions** (`RevokeUser`). The current password is checked with a plain bcrypt compare, not the step-up throttle; the route is covered by the general auth limiter.
5.  **Reset from the host**: `docker exec docklift-backend docklift reset-password` (inside a dev checkout: `go run ./cmd/docklift reset-password` from `server/`). It sets a random 16 character password for the first admin, prints it, and bumps `passwordChangedAt` so old sessions end. A running server also notices this: bound connections recheck the password version every 30 s (`SessionRecheckInterval`).

Other CLI subcommands: `docklift serve` (default, runs migrations first), `docklift migrate` (apply migrations and exit), `docklift panel-access` and `docklift api-token` (see API Tokens).

## Guards (`server/internal/auth/service.go`)

| Guard | Credential | Used for |
|-------|-----------|----------|
| `RequireSession` | `Authorization: Bearer <session JWT>` or `Bearer dl_<API token>` | everything under `/api` except the public routes below |
| `RequireAPIToken` | `Authorization: Bearer dl_<API token>` only | `/mcp` |
| `SessionOnly` / `FullScopeOnly` | after `RequireSession`: refuse every API token / read-scope tokens | see API Tokens |
| `RequireSSE` | query `?token=<sse token>`, exactly one `token` parameter | `GET /api/system/logs/*` and `GET /api/logs/*/stream/*` |
| `VerifyPurposeToken` | query `?token=<terminal token>` | `/ws/terminal` upgrade, single use |
| setup token | header `X-Setup-Token` | `POST /api/backup/restore-upload` on a fresh install |

-   `RequireSession` never reads query tokens and rejects `sse` and `terminal` purpose tokens.
-   The SSE guard is selected per request in `router.go` (`sseOr`): only GET requests on the two stream path shapes use it. Any other method or path on those mounts needs a session token.
-   SSE tokens may be reused (an `EventSource` reconnects with the same URL) until they expire after 5 minutes. The connection is bound to the session, so it ends when the password changes.
-   **Terminal tokens are single use.** `VerifyPurposeToken` remembers the SHA-256 of each consumed token until it expires (bounded at 10,000 entries; when full, new tokens are refused). Each token also carries a random `jti`, so two issued in the same second differ.

Token endpoints: `POST /api/auth/sse-token` and `POST /api/auth/terminal-token` (both need a session token; API tokens are refused).

## API Tokens (`server/internal/auth/apitokens.go`)

Long-lived bearer credentials for scripts, CI and AI agents: `Authorization: Bearer dl_<43 base64url chars>` (32 random bytes).

-   **Storage**: table `api_tokens` (migration `20261128120000_api_tokens`): `id`, `name`, `token_hash` (hex SHA-256, unique), `prefix` (first 11 characters, for display), `user_id` (FK to `users`, cascade), `scope` (`read`|`full`), `allow_dangerous`, `created_at`, `last_used_at`, `expires_at` (nullable). The plain token is returned once, on creation, and never stored or logged.
-   **Lookup**: `RequireSession` sends a Bearer value starting with `dl_` to `serveAPIToken`; a JWT never starts with that prefix, so session handling is unchanged. Format check, then an indexed equality lookup on the hash (no byte-by-byte compare of the secret), then: expired → 401 `API token expired`; owner's `passwordChangedAt` later than `created_at` → 401 `API token revoked: ...` (so **a password change or `reset-password` revokes every older token**, with no extra write); owner missing → 401.
-   **Scope**: `read` allows GET/HEAD only (403 otherwise). `auth.FullScopeOnly` refuses read tokens on GET routes that must not be read-only (`GET /api/backup/download/:file`).
-   **Never with a token** (`auth.SessionOnly`, 403): `/api/tokens/*`, `PATCH /api/auth/profile`, `POST /api/auth/change-password`, `/sse-token`, `/terminal-token`, `PUT /api/domains/:domain/access`. The WebSocket terminal needs a terminal purpose token, which only a session can get.
-   **Step-up**: `TokenStepUp` runs first in `RequireStepUpPassword` and `VerifyStepUpPassword` (and in `/api/system/execute` before its password check). A token without `allow_dangerous` gets 403 `{error, requirePassword: false}`; with it the action proceeds without a password and an `[AUDIT]` line is logged. `CheckAccountPassword` (change-password) always fails for a token.
-   **Throttle**: 20 invalid tokens (`APITokenFailureLimit`) per client address per 15 minutes, then 429 with `Retry-After`; the check runs before the lookup. A valid token does not reset the count.
-   **last_used_at** is written at most once a minute per token (`APITokenTouchInterval`).
-   **Log streams**: on the `sseOr` paths a request with `Bearer dl_...` is authenticated as a token (no SSE token needed). Such a stream is not bound to the session registry: revoking the token does not cut a stream that is already open.
-   **Audit**: every non-GET token request logs `[API-TOKEN] METHOD path by token "name" (prefix) from IP: ...`.
-   **Management**: `GET/POST /api/tokens`, `DELETE /api/tokens/:id` (`server/internal/api/tokens`), session only; create and revoke need the step-up password. `POST` body: `name` (1-100), `scope` (default `read`), `allowDangerous` (needs `full`), `expiresInDays` (1-3650 or null). CLI: `docklift api-token create <name> [--read] [--allow-dangerous] [--expires 90d] | list | revoke <id|prefix>` (`server/cmd/docklift/apitoken.go`; creates for the first admin, default scope full).
-   **MCP**: `/mcp` uses `RequireAPIToken` (API token only, no scope check on the transport POST itself, 401 with `WWW-Authenticate: Bearer`). See `security_hardening`.

## Public Routes

-   `GET /api/health`
-   `GET /api/auth/status`, `POST /api/auth/register`, `POST /api/auth/login`, `GET /api/auth/setup-token` (needs the bootstrap secret, refused once a user exists)
-   `/api/github/webhook` (HMAC), `/api/github/manifest/callback`, `/api/github/setup`, `/api/github/callback` (these need a matching `state`, see the `github_integration` skill)
-   `POST /api/backup/restore-upload` with a valid `X-Setup-Token`, only while no admin exists

All other `/api/*` routes need `RequireSession`.

## Step-up Password

Dangerous operations re-verify the account password in the JSON body (`password`), on top of the session token. `RequireStepUpPassword` answers 403 `{ error, requirePassword: true }` when the password is missing or wrong.

-   **Throttle**: 10 wrong passwords per user **and client address** per 15 minutes (`StepUpFailureLimit`, `StepUpFailureWindow`). Then 429 with `Retry-After` and the hash is not compared. A correct password resets the count. At most 10,000 pairs are tracked.
-   Used by: backup restore routes and `clear-critical-restore`, `POST /api/system/purge|reboot|reset|update-system|upgrade`, `POST /api/deployments/:projectId/rollback`, `POST /api/system/execute` (through `VerifyStepUpPassword`), turning on domain-only access, and creating or revoking API tokens.
-   With an API token there is no password: only a token created with `allow_dangerous` passes (see API Tokens). A new step-up call site must go through `RequireStepUpPassword`, `VerifyStepUpPassword` or `TokenStepUp`, never compare the password itself.
-   The frontend must abort when the operator cancels the prompt; it must never call these APIs without a password.

## Terminal Authentication

1.  `POST /api/auth/terminal-token`, then connect to `wss://host/ws/terminal?token=...`.
2.  The upgrade checks the `Origin` first (403), then the token (401). A request with no `Origin` is refused unless `TERMINAL_ALLOW_NO_ORIGIN=true`.
3.  The server sends `auth_required`; the client must answer `{ "type": "auth", "password": ... }` within **60 seconds** (`AuthTimeout`) or the session ends with `exit` / `auth_timeout`.
4.  5 wrong passwords per session, and 20 wrong passwords per `address:user` per 15 minutes across reconnects.
5.  Max 3 concurrent sessions, 30 minute idle timeout.

## Session Handling in the Browser

-   The token lives in `localStorage` key `docklift_token`.
-   Use `authFetch()` for protected APIs. It adds the Bearer header and on **401** clears the session through the handler registered by `AuthProvider`.
-   `AuthProvider.tsx` validates `/api/auth/me` on page load and clears invalid tokens.
-   Backup downloads use `authFetch` plus a blob. Never put a session token in a URL.

## Security Behaviour

-   **Request bodies are parsed only after rate limiting and authentication.** `router.go` mounts the body parser inside each group, after the limiter and the guard. Unauthenticated routes get a 64 KiB parser (`publicBodyLimit`) and a separate memory budget.
-   **Rate limits** (per client address, 15 minute window): `/api/auth/*` 100 requests; `/api/auth/register` and `/api/auth/setup-token` are also counted against a stricter limiter of 10; `/api/auth/me`, `/sse-token`, `/terminal-token` skip the shared auth limiter and have their own 1000 request limiter; `/api/auth/status` is not limited. 429 responses carry `Retry-After` and `RateLimit-*` headers.
-   **Client address**: `X-Forwarded-For` is believed only when the socket peer is in `TRUSTED_PROXIES` (default `127.0.0.0/8,::1/128,172.28.255.254,fd12:3456:789a::ffff:fffe`, which is loopback plus the fixed address of `docklift-nginx-proxy`). Otherwise the socket address is used, so a direct client of `:8080` cannot forge it.
-   **Error messages** from auth routes are fixed strings (`Login failed`, `Registration failed`); internal errors are logged, not returned.
-   **Origin checks** use scheme + host + port, see the `security_hardening` skill.

## Common Issues

-   **Redirect loop or "Unauthorized" loop**: the stored token is invalid or the password changed. Clear `docklift_token` from `localStorage`.
-   **Every user from one office gets 429 on login**: all of them share one client address. If Docklift sits behind another reverse proxy, the limiter sees that proxy's address instead of the user's: add the proxy to `TRUSTED_PROXIES`.
-   **Lost the setup code**: `cat data/.bootstrap-secret` on the host (only exists before the first admin is created).
-   **Lost the admin password**: `docker exec docklift-backend docklift reset-password`.
