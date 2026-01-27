---
name: Frontend Development
description: Guide for developing features in the Next.js 16 frontend functionality.
---

# Frontend Development Guide

Docklift uses **Next.js 16 (App Router)** for its frontend.

## Directory Structure (`frontend/app/`)

-   `page.tsx`: Dashboard (Home).
-   `layout.tsx`: Root layout (includes ThemeProvider, AuthProvider, Toaster).
-   `projects/`: Project management routes.
    -   `[id]/`: Project detail view (nested layout).
    -   `new/`: Project creation wizard.
-   `settings/`: Global settings.
-   `sign-in/`: Login page.

## Key Architectures

### 1. Client vs Server Components
-   **"use client"**: Used extensively for interactive dashboards (`useState`, `useEffect`).
-   **Server Components**: Used for static layouts or initial data fetching where possible (though this app relies heavily on client-side API fetching due to auth token storage in localStorage).

### 2. State Management
-   **Local State**: `useState` for UI toggles.
-   **Global State**: Minimal. `AuthContext` handles user session.
-   **Data Fetching**: `useEffect` + `axios/fetch` pattern.
    -   *Standard*: Fetch data on mount, show loader, handle error.

### 3. Streaming Logs
-   Uses **Server-Sent Events (SSE)**.
-   Endpoint: `/api/deployments/:id/logs`.
-   Frontend: `EventSource` API handles real-time updates.

### 4. File Editor (Monaco)
-   Found in `components/FileEditor.tsx`.
-   Used for editing `docker-compose.yml` and project files directly in the browser.

## Common Tasks

### Adding a New Page
1.  Create `app/new-feature/page.tsx`.
2.  Add `"use client"` if it needs interaction.
3.  Import `Header` and `Footer`.

### Connecting to Backend
Use the helper in `lib/utils.ts` or `lib/auth.ts`:
```typescript
import { API_URL } from "@/lib/utils";
import { getAuthHeaders } from "@/lib/auth";

// Fetch
fetch(`${API_URL}/api/resource`, { headers: getAuthHeaders() });
```

## Debugging

-   **Hydration Errors**: Usually caused by malformed HTML (e.g., `<div>` inside `<p>`) or dates rendering differently on server/client.
-   **Auth Issues**: Check `Application > Local Storage` in devtools for `docklift_token`.
