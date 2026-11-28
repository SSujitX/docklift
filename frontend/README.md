# Docklift dashboard

React 19 + Vite + Tailwind CSS 4 + React Router. The production build is embedded in the Go binary
(`server/web`), which serves it on the same port as the API and the WebSocket terminal.

## Development

```bash
# from the repository root: Go API on :8000 and this dev server together
make dev

# or by hand
cd server && go run ./cmd/docklift     # API on :8000
cd frontend
bun install --frozen-lockfile
bun run dev
```

Open http://127.0.0.1:3600 (set `PORT` in `.env` to change it). `/api` and `/ws` are proxied to the
backend on `127.0.0.1:8000`; leave `VITE_API_URL` empty.

## Checks

```bash
bun test                                # unit tests (src/**/*.test.ts)
bunx tsc -p tsconfig.app.json --noEmit  # typecheck
bun run build                           # typecheck + production build into dist/
```

## Production

The root `Dockerfile` builds this directory with Bun (`bunx vite build`; CI runs the typecheck), copies `dist/` into
`server/web/dist` and compiles it into the `docklift` binary with `go:embed`. There is no separate
frontend container. `make build` does the same without Docker and writes `bin/docklift`.
