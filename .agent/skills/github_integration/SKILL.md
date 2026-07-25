---
name: GitHub Integration
description: Guide for setting up and managing Docklift's GitHub App integration.
---

# GitHub Integration Guide

Docklift integrates with GitHub using a GitHub App. This allows for accessing private repositories and receiving webhook events (push) for auto-deployments.

## Architecture

-   **Routes**: `backend/src/routes/github.ts`
-   **Service**: `backend/src/services/git.ts` (for cloning/pulling)
-   **Frontend helpers**: `startGithubInstallSession()` / `startGithubInstallAndNavigate()` in `frontend/src/lib/auth.ts`
-   **Authentication**: Uses JWT signed with a private key to authenticate as the GitHub App.
-   **Callback URL**: derived from `DOCKLIFT_FRONTEND_URL` — it must be the public dashboard URL, or
    GitHub redirects the user somewhere unreachable.

## Setup Flow (Manifest Flow)

1.  **Initiation**: User clicks "Connect GitHub" in UI.
2.  **Manifest Generation**: `POST /api/github/manifest` generates a GitHub App manifest.
3.  **Redirect**: User is redirected to GitHub to create the app.
4.  **Callback**: GitHub redirects back to `/api/github/manifest/callback` with a code.
5.  **Exchange**: Backend exchanges code for App Credentials (ID, Client ID, Secret, Private Key, Webhook Secret).
6.  **Storage**: Credentials are stored in the `Settings` table in the database.
7.  **Installation**: User is redirected to install the newly created app on their account/orgs.

### CSRF Protection on the Setup Flow

The manifest and install callbacks are **public** endpoints (GitHub calls them unauthenticated), so
they are guarded by a one-time nonce instead:

-   Each setup attempt writes a **per-state file** under `data/github-setup/<state>.json`
    (`{ createdAt, returnUrl? }` — return URL lives **per state**, not a global `github_return_url`).
-   Also sets an `HttpOnly`, `SameSite=Lax` cookie (`docklift_github_state`, `Secure` over HTTPS).
-   Verification uses `timingSafeEqual`, enforces TTL, and clears the used state — single-use.
-   Legacy Settings-key fallback may still be read for older in-flight flows.

### Legacy `/api/github/callback`

Unsafe OAuth code→token exchange is **removed**. The route may still redirect `installation_id` to
`/api/github/setup`; otherwise it sends the user to Settings with `github_error=legacy_oauth_disabled`.

## Key Components

### Repository Listing
-   **Endpoint**: `GET /api/github/repos`
-   **Logic**: Fetches repositories from **all** installations accessible to the App.
-   **Important**: Uses pagination to ensure all repositories are retrieved (recursively fetching all pages).

### Webhooks (Auto-Deploy)
-   **Endpoint**: `POST /api/github/webhook` (global — project matching is by repo URL, not `/webhook/:projectId`).
-   UI / API should advertise `/api/github/webhook`.
-   **Event**: Listen for `push` events.
-   **Logic**: 
    1.  Requires `github_webhook_secret` to be configured — requests are **rejected (401)** if the secret is missing (fail closed).
    2.  Verifies HMAC signature **FIRST** — before any database queries (prevents unauthenticated DB lookups). Uses `req.rawBody` captured via `express.json({ verify })` callback for accurate comparison.
    3.  **Skip** when `payload.deleted === true` or `payload.head_commit == null` (deleted-branch pushes).
    4.  Matches repository URL from payload with `Project` database entries.
    5.  Triggers deployment for matching projects with `auto_deploy: true`.
    6.  Debounced via `recentDeploys` Map with 10-second cooldown per project (no global concurrency cap — careful with many projects on one repo).

### Authentication
-   **App Auth**: Uses `jsonwebtoken` to sign a JWT with the stored Private Key (`RS256`).
-   **Installation Auth**: Uses the App JWT to request an "Installation Token" for specific API acts (like cloning or fetching repos).

## Troubleshooting

-   **"Repositories not loading"**: Check if the App is installed on the specific GitHub account.
-   **"Webhook not triggering"**: Verify the `webhook_secret` in DB matches GitHub. Check if `auto_deploy` is enabled for the project.
-   **"Authentication Failed"**: The Private Key might be missing or invalid in the database.

## Useful Database Settings (`Settings` table)
-   `github_app_id`
-   `github_private_key`
-   `github_webhook_secret`
-   `github_installation_id` (Default installation ID)
