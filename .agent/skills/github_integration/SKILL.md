---
name: GitHub Integration
description: Guide for setting up and managing Docklift's GitHub App integration.
---

# GitHub Integration Guide

Docklift integrates with GitHub using a GitHub App. This allows for accessing private repositories and receiving webhook events (push) for auto-deployments.

## Architecture

-   **Routes**: `backend/src/routes/github.ts`
-   **Service**: `backend/src/services/git.ts` (for cloning/pulling)
-   **Authentication**: Uses JWT signed with a private key to authenticate as the GitHub App.

## Setup Flow (Manifest Flow)

1.  **Initiation**: User clicks "Connect GitHub" in UI.
2.  **Manifest Generation**: `POST /api/github/manifest` generates a GitHub App manifest.
3.  **Redirect**: User is redirected to GitHub to create the app.
4.  **Callback**: GitHub redirects back to `/api/github/manifest/callback` with a code.
5.  **Exchange**: Backend exchanges code for App Credentials (ID, Client ID, Secret, Private Key, Webhook Secret).
6.  **Storage**: Credentials are stored in the `Settings` table in the database.
7.  **Installation**: User is redirected to install the newly created app on their account/orgs.

## Key Components

### Repository Listing
-   **Endpoint**: `GET /api/github/repos`
-   **Logic**: Fetches repositories from **all** installations accessible to the App.
-   **Important**: Uses pagination to ensure all repositories are retrieved (recursively fetching all pages).

### Webhooks (Auto-Deploy)
-   **Endpoint**: `POST /api/github/webhook`
-   **Event**: Listen for `push` events.
-   **Logic**: 
    1.  Verifies signature using `github_webhook_secret`.
    2.  Matches repository URL from payload with `Project` database entries.
    3.  Triggers deployment for matching projects with `auto_deploy: true`.

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
