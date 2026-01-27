---
name: Authentication System
description: Guide to the authentication and authorization system in Docklift.
---

# Authentication System Guide

Docklift uses a token-based authentication system (JWT) to secure the API and frontend.

## Components

-   **Routes**: `backend/src/routes/auth.ts`
-   **Middleware**: `backend/src/lib/authMiddleware.ts`
-   **Frontend Context**: `frontend/lib/auth.tsx`
-   **Database Model**: `User` (email, password hash).

## Auth Flow

1.  **Registration**:
    -   `POST /api/auth/register`
    -   Only allows registration if **zero** users exist in the database.
    -   First user becomes the admin.

2.  **Login**:
    -   `POST /api/auth/login`
    -   Validates email/password (bcrypt).
    -   Returns a `token` (JWT).

3.  **Session Management**:
    -   Frontend stores the token in `localStorage` key `docklift_token`.
    -   Token is sent in `Authorization: Bearer <token>` header for all API requests.
    -   Token expiry is set to 7 days.

## Security Middleware

Located in `backend/src/lib/authMiddleware.ts`.

-   **`authenticateToken`**:
    -   Verifies the JWT signature.
    -   If valid, attaches `req.user` and allows request.
    -   If invalid/missing, returns 401 Unauthorized.

## Passwords

-   **Hashing**: Uses `bcrypt` with 12 salt rounds.
-   **Reset**: Admin password can be reset via CLI if locked out:
    ```bash
    cd backend
    bun run reset-password
    # Follow prompts
    ```

## Common Issues

-   **Infinite Redirects**: Often caused by invalid token storage or clock skew issues invalidating the JWT.
-   **"Unauthorized" Loop**: Frontend might not be clearing the invalid token. storage needs to be cleared.
