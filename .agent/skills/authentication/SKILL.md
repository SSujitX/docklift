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
    -   Only allows registration if **zero** users exist in the database (first user becomes admin).

2.  **Login**:
    -   `POST /api/auth/login`
    -   Validates email/password (bcrypt, 12 salt rounds).
    -   Returns a JWT `token` (7-day expiry).
    -   Rate limited via `express-rate-limit` on all `/api/auth` routes.

3.  **Session Management**:
    -   Frontend stores the token in `localStorage` key `docklift_token`.
    -   Token is sent in `Authorization: Bearer <token>` header via `getAuthHeaders()` in `frontend/src/lib/auth.ts`.
    -   `AuthProvider.tsx` validates the token against `/api/auth/me` on page load and clears invalid tokens.
    -   **Password change invalidates old sessions**: `User.passwordChangedAt` is compared against the
        JWT `iat`, so tokens issued before a password change are rejected even though they have not expired.

## Protected Routes

All routes except the following require JWT via `authMiddleware`:
-   `/api/auth/register`, `/api/auth/login`, `/api/auth/status` (public, rate limited)
-   `/api/github/webhook`, `/callback`, `/manifest/callback`, `/setup` (GitHub flow)
-   `/api/backup/restore-upload` with valid one-time setup token (fresh install restore)

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

-   **`authMiddleware`**: Bearer session JWT; rejects SSE-purpose tokens; never reads `?token=`.
-   **`sseAuthMiddleware`**: Query SSE token only; rejects tokens without `purpose: 'sse'`.

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
