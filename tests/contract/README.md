# Docklift HTTP contract tests

Black-box tests of the Docklift HTTP + WebSocket API. They define "correct" for the
Go rewrite: the suite passes against the TypeScript backend (`backend/`) and must
pass unchanged against the Go backend (`server/`). It is its own Go module
(`docklift/tests/contract`, standard library + `gorilla/websocket`).

Every route is listed in [ROUTES.md](ROUTES.md) with its auth rule, TS source line and covering test.

## Running

From this directory:

```sh
go vet ./...
go test ./...                       # default: DOCKLIFT_TARGET=ts
go test -run TestFreshInstallFlow -v ./...
```

### Against the TypeScript backend (default)

Needs `node`, `backend/dist` and `backend/node_modules` (and `bunx` or the Prisma CLI in
`backend/node_modules`). For every booted instance the harness:

1. creates a temp dir (`data/ deployments/ backups/ nginx/ letsencrypt/`),
2. runs `bunx prisma migrate deploy` in `backend/` with `DATABASE_URL=file:<tmp>/data/smoke.db`,
3. starts `node dist/index.js` from a **copy** of `backend/dist` (+ `package.json`, with
   `node_modules` linked) on a free port, and waits for `GET /api/health`.

Running from a copy matters: `backend/src/lib/loadEnv.ts` loads `backend/.env` and
`backend/.env.local` with `override: true`, which would silently replace `CORS_ORIGIN`,
`DOCKLIFT_FRONTEND_URL`, etc. The copy has no `.env` files, so the TS and Go servers
receive exactly the same environment.

### Against the Go backend

```sh
DOCKLIFT_TARGET=go DOCKLIFT_GO_BIN=../../server/docklift go test ./...
```

The binary is started with no arguments (default `serve` subcommand), with the same
environment variables, cwd set to the instance temp dir. No migration step is run by the
harness — the Go binary must create/migrate the SQLite file itself. Some instances start on
`testdata/seed.db.gz`, a database written by the TS backend's Prisma client (existing-install
compatibility).

### Against a running server

```sh
DOCKLIFT_BASE_URL=https://panel.example.com DOCKLIFT_ADMIN_EMAIL=... DOCKLIFT_ADMIN_PASSWORD=... go test ./...
```

- A fresh remote server is set up by the suite: provide `DOCKLIFT_BOOTSTRAP_SECRET`, or
  `DOCKLIFT_DATA_DIR` (the server's `DATA_PATH`, readable by the test process).
- `DOCKLIFT_DATA_DIR` also enables tests that read `.secrets` (internal-secret header,
  self-signed JWTs).
- Tests that need a dedicated instance (fresh install, seeded DB, restore, critical seal) and
  allowlist-specific CORS checks are skipped.
- Every client sends its own `X-Forwarded-For` so rate-limit buckets do not collide. Behind a
  reverse proxy that appends the real IP, all clients share one bucket; the rate-limit tests
  and repeated runs within 15 minutes can then hit 429s. The terminal brute-force counter
  (20 failures / 15 min per socket IP + user) also accumulates across runs on a remote server.

## Docker

The harness always boots with `DOCKER_HOST=tcp://127.0.0.1:1`, so no Docker daemon is ever
touched. Docker-dependent endpoints are still covered for validation, auth and the exact
"Docker unreachable" responses (`requireNoDocker(t)`).

- `DOCKLIFT_DOCKER=1` declares that the target server *can* reach Docker: no-Docker assertions
  skip and `requireDocker(t)` tests (deploy success, stop/restart/rollback, storage volumes,
  database create, delete) run. When booting with `DOCKLIFT_DOCKER=1` the harness passes the
  caller's `DOCKER_HOST` through instead of the unreachable one. Use this on the Linux test server.
- `DOCKLIFT_NETWORK=1` enables tests whose server-side code calls the public internet (DNS
  checks, GitHub manifest code exchange).

## Other knobs

| Variable | Meaning |
|---|---|
| `DOCKLIFT_TARGET` | `ts` (default) or `go` |
| `DOCKLIFT_GO_BIN` | Go server binary (required for `go`) |
| `DOCKLIFT_BASE_URL` | use a running server instead of booting |
| `DOCKLIFT_DATA_DIR` | that server's `DATA_PATH` (optional) |
| `DOCKLIFT_ADMIN_EMAIL` / `DOCKLIFT_ADMIN_PASSWORD` | admin of an already set-up remote server |
| `DOCKLIFT_BOOTSTRAP_SECRET` | bootstrap secret of a fresh remote server |
| `DOCKLIFT_BACKEND_DIR` | TS backend dir (default `../../backend`) |
| `DOCKLIFT_DOCKER=1` | server has Docker (see above) |
| `DOCKLIFT_NETWORK=1` | allow server-side internet calls |
| `DOCKLIFT_KEEP_TMP=1` | keep the temp dir (server logs, data dirs) |

## Environment passed to booted servers

The inherited environment is stripped of every variable the backend reads, then:

```
NODE_ENV=production
PORT=<free port>
DATABASE_URL=file:<tmp>/<instance>/data/smoke.db
DATA_PATH=<tmp>/<instance>/data
DEPLOYMENTS_PATH=<tmp>/<instance>/deployments
BACKUP_PATH=<tmp>/<instance>/backups
NGINX_CONF_PATH=<tmp>/<instance>/nginx
LETSENCRYPT_PATH=<tmp>/<instance>/letsencrypt
DOCKER_HOST=tcp://127.0.0.1:1
JWT_SECRET=                       (empty: secrets come from DATA_PATH/.secrets)
INTERNAL_API_SECRET=
CORS_ORIGIN=http://cors-allowed.test:4000
DOCKLIFT_FRONTEND_URL=http://frontend.test:3000
DOCKLIFT_GITHUB_APP_ID=
DOCKLIFT_GITHUB_CLIENT_ID=
DOCKLIFT_GITHUB_CLIENT_SECRET=
DOCKLIFT_GITHUB_PRIVATE_KEY_PATH=<tmp>/<instance>/github-app.pem   (does not exist)
DOCKER_NETWORK=docklift_contract_network
PORT_RANGE_START=5500
PORT_RANGE_END=5510
CERTBOT_CONTAINER=docklift-contract-certbot
CERTBOT_EMAIL=
CERTBOT_STAGING=true
NGINX_PROXY_CONTAINER=docklift-contract-nginx-proxy
TERMINAL_ALLOW_NO_ORIGIN=
```

## Instances

| Name | Booted | Purpose |
|---|---|---|
| `main` | always (TestMain) | shared server; first admin registered by the harness |
| `fresh` | `TestFreshInstallFlow` | bootstrap secret, setup token, setup-token restore, first registration, profile, password change |
| `seeded` | `TestSeeded`, `TestACMEEmailDefaultsToAdmin` | `testdata/seed.db.gz` + known `.secrets`: webhook HMAC with a known secret, configured GitHub App, managed DB links, rollback guards |
| `critical` | `TestCriticalRestoreGate` | data dir carrying a `.restore-critical` seal |
| `restore` | `TestAuthenticatedRestore` | a committed restore (server exits, harness restarts it) |

Processes are killed as a tree (`taskkill /T /F` on Windows, process group on Unix). On any
failure every instance's server log is printed at the end of the run.

## Seed database

`testdata/seed.db.gz` is generated by `testdata/seed/generate.mjs` using the TS backend's own
migrations and Prisma client (IDs mirrored in `seed_test.go`):

```sh
node testdata/seed/generate.mjs ../../backend testdata/seed.db.gz
```

It is gzipped (and not named `*.db`) because the repository `.gitignore` ignores `*.db`.

## Conventions

- `Resp.Expect(t, status, Obj{...})` asserts the exact status and a structural body: exact key
  sets, literal values, and matchers (`UUID`, `ISOms`, `JWT`, `Hex(n)`, `Str`, `Num`, `Int`,
  `Bool`, `Prefix`, `Contains`, `Re`, `Either`, `OrNull`, `ArrayOf`, `Optional`).
- Error bodies are compared exactly (`ExpectError`). Where the text comes from Docker/OS
  output, only the stable prefix is asserted.
- Test data uses unique names per test (`uniq`), so tests are order-independent and re-runnable;
  the dedicated instances' ordered flows are single tests with ordered subtests.
- Local HTTPS-intercepting tools (e.g. AdGuard) may rewrite HTML responses; assertions on the
  Express HTML 404 use `Contains`.
