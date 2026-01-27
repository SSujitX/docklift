---
name: Database Management
description: Guide for managing the Docklift SQLite database using Prisma.
---

# Database Management Guide

Docklift uses SQLite as its database, managed by Prisma ORM.

## Schema Location

The Prisma schema is located at:
`backend/prisma/schema.prisma`

## Core Commands

Run these commands from the `backend/` directory:

### View Data
-   **Open Studio**:
    ```bash
    bun run db:studio
    ```
    Opens a web GUI to view and edit the database content.

### Schema Changes
1.  **Edit Schema**: Modify `backend/prisma/schema.prisma`.
2.  **Generate Client**:
    ```bash
    bun run db:generate
    ```
    Updates the generated TypeScript client in `node_modules`.
3.  **Push Changes**:
    ```bash
    bun run db:push
    ```
    Applies the schema changes to the local SQLite database file (`backend/prisma/dev.db` or similar).

### Reset
-   **Reset Database**:
    ```bash
    bun run db:push --force-reset
    ```
    **WARNING**: This will delete all data.

## Key Models

-   **User**: Admin users for the application.
-   **Project**: Represents a deployed application (container).
-   **Deployment**: History of builds/deploys for a project.
-   **EnvVariable**: Environment variables for projects.
-   **Settings**: System-wide settings.

## Troubleshooting

-   If the database seems out of sync with the client, run `bun run db:generate`.
-   If you encounter migration errors, `db:push` usually resolves dev inconsistencies for SQLite.
