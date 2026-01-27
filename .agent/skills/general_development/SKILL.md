---
name: General Development
description: Guide for setting up, running, and developing the Docklift project.
---

# General Development Guide

This skill provides instructions for developing the Docklift project, a self-hosted Docker deployment platform.

## Prerequisites

- **Docker**: Ensure Docker is installed and running.
- **Bun**: This project uses [Bun](https://bun.sh/) as the package manager and runtime for scripts.

## Project Structure

- **backend/**: Node.js Express API server.
- **frontend/**: Next.js 16 React application.
- **nginx-proxy/**: Reverse proxy configuration for deployed projects.
- **data/**: SQLite database storage (mounted volume).
- **deployments/**: Storage for project files (mounted volume).

## Quick Start (Development)

1.  **Clone the repository:**
    ```bash
    git clone https://github.com/SSujitX/docklift.git
    cd docklift
    ```

2.  **Backend Setup:**
    Open a terminal and run:
    ```bash
    cd backend
    cp .env.example .env
    bun install
    bun run db:generate
    bun run db:push
    bun run dev
    ```
    The backend will start on `http://localhost:4000`.

3.  **Frontend Setup:**
    Open a **new** terminal and run:
    ```bash
    cd frontend
    bun install
    bun run dev
    ```
    The frontend will start on `http://localhost:3000`.

## Common Commands

### Backend
-   `bun run build`: Compile TypeScript.
-   `bun run db:studio`: Open Prisma Studio GUI to view/edit database data.
-   `bun run reset-password`: Reset the admin password.

### Frontend
-   `bun run build`: Create a production build of the Next.js app.
-   `bun run lint`: Run ESLint checks.

### Docker (Infrastructure)
-   `docker compose up -d`: Start the production-like environment (nginx, backend, frontend).
-   `docker compose logs -f`: View logs for all services.

## Architecture Notes

-   **Authentication**: detailed in `CODEBASE_SUMMARY.md`. Uses JWT, stored in localStorage.
-   **Deployments**: The backend manages Docker deployments. It clones repos/unzips files, generates `docker-compose.yml`, and runs `docker compose up`.
-   **Nginx**: Used for routing custom domains to the appropriate containers.

## Troubleshooting

-   **"Session validation failed"**: Check if the JWT token is expired or valid.
-   **Build errors**: ensure you are using the correct Node/Bun versions and dependencies are installed.
