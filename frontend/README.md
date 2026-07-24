# DockLift frontend

Vite + React + React Router dashboard.

## Dev

```bash
# Backend on :8000
cd backend && bun run dev

cd frontend
bun install
bun run dev
```

Open http://127.0.0.1:3600 (set `PORT` in `.env` to change).  
`/api` and `/ws` proxy to the backend — leave `VITE_API_URL` empty.

## Build

```bash
bun run build    # → dist/
bun run preview
```

## Production

Built into the `docklift-frontend` image (static nginx on port 3000).  
Gateway `nginx.conf` on `:8080` proxies `/` → frontend and keeps `/api` + `/ws` + SSE rules.
