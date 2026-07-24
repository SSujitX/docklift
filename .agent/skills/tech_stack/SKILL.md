---
name: Technology Stack
description: Comprehensive list of all technologies and libraries used in Docklift.
---

# Technology Stack

Docklift is built using a modern, lightweight, and performance-oriented stack.

## Frontend (User Interface)

-   **Bundler / Dev**: [Vite](https://vite.dev/) (dev server on `:3600`)
-   **UI**: [React 19](https://react.dev/) + [React Router](https://reactrouter.com/)
-   **Language**: [TypeScript](https://www.typescriptlang.org/)
-   **Styling**:
    -   [Tailwind CSS v3](https://tailwindcss.com/)
    -   [Shadcn UI](https://ui.shadcn.com/) (Radix UI primitives)
    -   `tailwindcss-animate`
-   **App shell**: Custom sidebar shell in `frontend/src/components/shell/` + `frontend/src/app/AppShell.tsx`
-   **State/Data**:
    -   React Hooks (`useState`, `useEffect`)
    -   Native `fetch` via `authFetch()` / `fetchWithAuth()` helpers (`frontend/src/lib/auth.ts`)
    -   [EventSource](https://developer.mozilla.org/en-US/docs/Web/API/EventSource) (SSE logs)
-   **Editor**: [Monaco Editor](https://microsoft.github.io/monaco-editor/)
-   **Terminal**: [xterm.js](https://xtermjs.org/) over WebSocket
-   **Icons**: [Lucide React](https://lucide.dev/)
-   **Notifications**: [Sonner](https://sonner.emilkowal.ski/)
-   **Theming**: Custom `ThemeProvider` (`frontend/src/lib/theme.tsx`)

## Backend (API & Orchestration)

-   **Runtime**: [Node.js 24](https://nodejs.org/) (production Docker images; Bun used for install/build)
-   **Package Manager**: [Bun](https://bun.sh/) (fast install & script runner)
-   **Dev Runner**: [tsx](https://tsx.is/) (`bun run dev` → `tsx watch src/index.ts`, serves `:8000`)
-   **Framework**: [Express 5.2](https://expressjs.com/)
-   **Language**: [TypeScript](https://www.typescriptlang.org/)
-   **Database**:
    -   [SQLite](https://www.sqlite.org/) (local file-based DB)
    -   [Prisma ORM 6](https://www.prisma.io/) (data access)
-   **Docker Control**: [Dockerode](https://github.com/apocas/dockerode) for inspect/logs, `docker` CLI for compose/build
-   **App Builder**: [Railpack](https://railpack.com/) (pinned version, invoked through `docker buildx`)
-   **Compose Generation**: [js-yaml](https://github.com/nodeca/js-yaml)
-   **Git Operations**: [Simple-Git](https://github.com/steveukx/git-js)
-   **Monitoring**: [Systeminformation](https://systeminformation.io/) (CPU, RAM, usage stats)
-   **Archives**: `archiver` (backups), `unzipper` (uploads, wrapped by `lib/safeUnzip.ts`)
-   **Auth**:
    -   `jsonwebtoken` (JWT)
    -   `bcrypt` (password hashing, 12 rounds)
    -   `express-rate-limit`
-   **WebSocket**: `ws` (web terminal)

## Infrastructure

-   **Containers**: Docker + Docker Compose (+ BuildKit/`buildx` for Railpack builds)
-   **Images**: `docklift-backend`, `docklift-frontend` (explicit names; containers `docklift-*`)
-   **Dashboard gateway**: `docklift-nginx` → host `:8080` (`nginx.conf`)
-   **App domains proxy**: `docklift-nginx-proxy` → host `:80` + `:443`
-   **Certificates**: `docklift-certbot` (Let's Encrypt HTTP-01, renewal loop every 12h)
