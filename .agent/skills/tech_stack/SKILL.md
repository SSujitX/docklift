---
name: Technology Stack
description: Comprehensive list of all technologies and libraries used in Docklift.
---

# Technology Stack

Docklift is built using a modern, lightweight, and performance-oriented stack.

## Frontend (User Interface)

-   **Bundler / Dev**: [Vite](https://vite.dev/)
-   **UI**: [React 19](https://react.dev/) + [React Router](https://reactrouter.com/)
-   **Language**: [TypeScript](https://www.typescriptlang.org/)
-   **Styling**:
    -   [Tailwind CSS v3](https://tailwindcss.com/)
    -   [Shadcn UI](https://ui.shadcn.com/) (Radix UI primitives)
    -   `tailwindcss-animate`
-   **State/Data**:
    -   React Hooks (`useState`, `useEffect`)
    -   Native `fetch` via `authFetch()` / `fetchWithAuth()` helpers (`frontend/src/lib/auth.ts`)
    -   [EventSource](https://developer.mozilla.org/en-US/docs/Web/API/EventSource) (SSE logs)
-   **Editor**: [Monaco Editor](https://microsoft.github.io/monaco-editor/)
-   **Icons**: [Lucide React](https://lucide.dev/)
-   **Notifications**: [Sonner](https://sonner.emilkowal.ski/)
-   **Theming**: Custom `ThemeProvider` (`frontend/src/lib/theme.tsx`)

## Backend (API & Orchestration)

-   **Runtime**: [Node.js 24](https://nodejs.org/) (production Docker images; Bun used for install/scripts)
-   **Package Manager**: [Bun](https://bun.sh/) (Fast install & script runner)
-   **Framework**: [Express 5.2](https://expressjs.com/)
-   **Language**: [TypeScript](https://www.typescriptlang.org/)
-   **Database**:
    -   [SQLite](https://www.sqlite.org/) (Local file-based DB)
    -   [Prisma ORM 6](https://www.prisma.io/) (Data access)
-   **Docker Control**: [Dockerode](https://github.com/apocas/dockerode)
-   **Git Operations**: [Simple-Git](https://github.com/steveukx/git-js)
-   **Monitoring**: [Systeminformation](https://systeminformation.io/) (CPU, RAM, usage stats)
-   **Auth**:
    -   `jsonwebtoken` (JWT)
    -   `bcryptjs` (password hashing)

## Infrastructure

-   **Containers**: Docker + Docker Compose
-   **Images**: `docklift-backend`, `docklift-frontend` (explicit names; containers `docklift-*`)
-   **Dashboard gateway**: `docklift-nginx` → host `:8080` (`nginx.conf`)
-   **App domains proxy**: `docklift-nginx-proxy` → host `:80`
