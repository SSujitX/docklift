---
name: Authentication System
description: Guide to the authentication and authorization system in Docklift.
---

# Authentication System Guide

Docklift uses a token-based authentication system (JWT) to secure the API and frontend.

## Components

-   **Routes**: `backend/src/routes/auth.ts`
-   **Middleware**: `backend/src/lib/authMiddleware.ts`
-   **Origin checking**: `backend/src/lib/originCheck.ts` (CORS + WebSocket)
-   **Frontend Context**: `frontend/src/components/AuthProvider.tsx`
-   **Frontend API Helper**: `frontend/src/lib/auth.ts` (`getAuthHeaders`, `authFetch`, `fetchWithAuth`)
-   **Database Model**: `User` (email, password hash, role, `passwordChangedAt`).

## Auth Flow

1.  **Registration**:
    -   `POST /api/auth/register`
    -   Only allows registration if **zero** users exist (first user becomes admin).
    -   **Bootstrap secret required** (header `x-bootstrap-secret` or body) — printed by install /
        backend logs / `data/.bootstrap-secret`. Never returned by a public API.
    -   **Bootstrap claim**: exclusive `fs.rename` of `data/.bootstrap-secret` → `.claimed-<random>`
        (`lib/bootstrap.ts`). Only the winner may create the admin; losers get 403.
    -   Secret is consumed **only after** successful user create; failed validation restores the claim.
    -   Crash recovery: on boot with no users, `recoverStaleBootstrapClaims()` restores a leftover
        `.claimed-*` back to `.bootstrap-secret` so setup is not bricked.
    -   **Setup rate limit**: `/register` and `/setup-token` use a stricter limiter (**10 / 15 min**)
        in addition to the general auth limiter.

2.  **Login**:
    -   `POST /api/auth/login`
    -   Validates email/password (bcrypt, 12 salt rounds).
    -   Returns a JWT `token` (7-day expiry) that includes a **`pwdv`** claim.
    -   Rate limited via `express-rate-limit` on all `/api/auth` routes (100 / 15 min general).

3.  **Session Management**:
    -   Frontend stores the token in `localStorage` key `docklift_token`.
    -   **Always use `authFetch()`** for protected APIs — it attaches the Bearer header and on **401**
        clears storage + calls `logout()` (registered from `AuthProvider`). Prefer it over raw
        `fetch` + `getAuthHeaders()`.
    -   `AuthProvider.tsx` validates `/api/auth/me` on page load and clears invalid tokens.
    -   **Password change invalidates old sessions**: JWT must carry `pwdv === passwordChangedAt.getTime()`.
        Exact match (not `iat` comparison) — same-second reissue after password change works correctly.

## Protected Routes

All routes except the following require JWT via `authMiddleware`:
-   `/api/auth/register`, `/api/auth/login`, `/api/auth/status` (public, rate limited)
-   `/api/github/webhook`, `/manifest/callback`, `/setup` (GitHub App flow)
    -   Legacy `/api/github/callback` OAuth token exchange is **disabled** (install-id redirect only)
-   `/api/backup/restore-upload` with valid setup token (fresh install restore; token consumed only
    after successful restore — see `lib/setupRestoreAuth.ts`)

Routes `/me`, `/profile`, `/change-password` all use `authMiddleware` (not manual JWT decoding).

## SSE Authentication

Server-Sent Events (logs) use **short-lived tokens** instead of long-lived JWTs in URLs:
-   `POST /api/auth/sse-token` → returns a 5-minute JWT with `purpose: 'sse'` (requires Bearer session JWT)
-   Frontend uses this token as a query parameter: `?token=<sseToken>`
-   Middleware split:
    - **`authMiddleware`**: `Authorization: Bearer` session JWT only — **no query tokens**
    - **`sseAuthMiddleware`**: query `?token=` with `purpose === 'sse'` only
    - SSE middleware is mounted **only** on:
      - `GET /api/system/logs/:service`
      - `GET /api/logs/:projectId/stream/:containerName`

## Security Middleware

Located in `backend/src/lib/authMiddleware.ts`.

-   **`authMiddleware`**: Bearer session JWT; rejects SSE-purpose tokens; never reads `?token=`;
    rejects when `pwdv` is missing or does not match `passwordChangedAt`.
-   **`sseAuthMiddleware`**: Query SSE token only; rejects tokens without `purpose: 'sse'`; same `pwdv` check.
-   Setup-token path always uses `config.dataPath` (never a hardcoded `./data`).

## Security Hardening

-   **Error Sanitization**: All `catch` blocks in auth routes return generic messages (e.g., `'Login failed'`), never `error.message`.
-   **Security Headers**: Custom security headers in `index.ts` (`X-Content-Type-Options`, `X-Frame-Options`, `X-XSS-Protection`, `Referrer-Policy`, `Permissions-Policy`, plus HSTS when the request arrived over HTTPS). Helmet is not used.
-   **CORS / Origin**: Strict **same-origin** — scheme + host + port — via `isTrustedOrigin()`, plus any
    exact origins configured in `CORS_ORIGIN`. See the `security_hardening` skill for why hostname-only
    matching is unsafe here.
-   **Rate Limiting**: Applied to all `/api/auth` routes.
-   **Terminal**: WebSocket JWT + password re-verification (double auth). Session JWT only — SSE-purpose tokens are rejected on terminal upgrade.
-   **Backup Downloads**: Use `fetch` + `Authorization: Bearer` header + blob download pattern — **never** put JWTs in URL query parameters (prevents token leakage in browser history, server logs, and referrer headers).

## Passwords

-   **Hashing**: Uses `bcrypt` with 12 salt rounds.
-   **Reset**: Admin password can be reset via CLI:
    ```bash
    cd backend
    bun run reset-password
    ```

## Common Issues

-   **Infinite Redirects**: Often caused by invalid token storage or clock skew invalidating JWTs.
-   **"Unauthorized" Loop**: Frontend not clearing invalid token — clear `localStorage`.
