---
name: Technology Stack
description: Comprehensive list of all technologies and libraries used in Docklift.
---

# Technology Stack

Docklift is built using a modern, lightweight, and performance-oriented stack.

## Frontend (User Interface)

-   **Framework**: [Next.js 16](https://nextjs.org/) (App Router)
-   **Language**: [TypeScript](https://www.typescriptlang.org/)
-   **Styling**: 
    -   [Tailwind CSS v3](https://tailwindcss.com/)
    -   [Shadcn UI](https://ui.shadcn.com/) (Radix UI primitives)
    -   `tailwindcss-animate`
-   **State/Data**: 
    -   React Hooks (`useState`, `useEffect`)
    -   [Axios](https://axios-http.com/) (HTTP requests)
    -   [SWR](https://swr.vercel.app/) / EventSource (Real-time updates)
-   **Editor**: [Monaco Editor](https://microsoft.github.io/monaco-editor/) (VS Code logic for browser)
-   **Icons**: [Lucide React](https://lucide.dev/)
-   **Notifications**: [Sonner](https://sonner.emilkowal.ski/)
-   **Theming**: `next-themes` (Dark mode)

## Backend (API & Orchestration)

-   **Runtime**: [Node.js 22](https://nodejs.org/)
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
    -   `bcrypt` (Password hashing)

## Infrastructure

-   **Container Engine**: [Docker](https://www.docker.com/)
-   **Orchestration**: Docker Compose
-   **Web Server / Proxy**: [Nginx](https://nginx.org/)
    -   `docklift-nginx`: Main entry point (Dashboard acting as reverse proxy)
    -   `docklift-nginx-proxy`: Dedicated proxy for routing user deployments (Custom domains)

## Integration

-   **GitHub App**: Custom GitHub App for private repo access & push webhooks.

## Development Tools

-   **Linting**: ESLint
-   **Formatting**: Prettier
