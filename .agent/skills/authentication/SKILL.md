---
name: Authentication System
description: Guide to the authentication and authorization system in Docklift.
---

# Authentication System Guide

Docklift uses a token-based authentication system (JWT) to secure the API and frontend.

## Components

-   **Routes**: `backend/src/routes/auth.ts`
-   **Middleware**: `backend/src/lib/authMiddleware.ts`
-   **Frontend Context**: `frontend/components/AuthProvider.tsx`
-   **Frontend API Helper**: `frontend/lib/auth.ts`
-   **Database Model**: `User` (email, password hash, role).

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
    -   Token is sent in `Authorization: Bearer <token>` header via `getAuthHeaders()` in `frontend/lib/auth.ts`.
    -   `AuthProvider.tsx` validates the token against `/api/auth/me` on page load and clears invalid tokens.

## Protected Routes

All routes except the following require JWT via `authMiddleware`:
-   `/api/auth/register`, `/api/auth/login`, `/api/auth/status` (public, rate limited)
-   `/api/github/webhook`, `/callback`, `/manifest/callback`, `/setup` (GitHub flow)
-   `/api/backup/restore-upload` with valid one-time setup token (fresh install restore)

Routes `/me`, `/profile`, `/change-password` all use `authMiddleware` (not manual JWT decoding).

## SSE Authentication

Server-Sent Events (logs, deployment streams) use **short-lived tokens** instead of long-lived JWTs in URLs:
-   `POST /api/auth/sse-token` → returns a 5-minute JWT with `purpose: 'sse'`
-   Frontend uses this token as a query parameter: `?token=<sseToken>`
-   Backend validates `purpose === 'sse'` before allowing SSE connections

## Security Middleware

Located in `backend/src/lib/authMiddleware.ts`.

-   **`authenticateToken`**: Verifies JWT signature, attaches `req.user`, returns 401 if invalid.

## Security Hardening

-   **Error Sanitization**: All `catch` blocks in auth routes return generic messages (e.g., `'Login failed'`), never `error.message`.
-   **Security Headers**: Applied globally via `helmet()` middleware in `index.ts`.
-   **CORS**: Configured from `CORS_ORIGIN` environment variable.
-   **Rate Limiting**: Applied to all `/api/auth` routes.
-   **Terminal**: WebSocket JWT + password re-verification (double auth).
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
