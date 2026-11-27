# Installation

Install Docklift on any Linux VPS with Docker. Production installs land under `/opt/docklift` by default.

## Production install (recommended)

Latest GitHub release:

```bash
curl -fsSL https://raw.githubusercontent.com/SSujitX/docklift/master/install.sh | sudo bash
```

Pin a specific release (see [Releases](https://github.com/SSujitX/docklift/releases)). Pass the version to bash, not in the curl URL:

```bash
curl -fsSL https://raw.githubusercontent.com/SSujitX/docklift/master/install.sh | sudo bash -s -- v=3.0.0
```

```bash
curl -fsSL https://raw.githubusercontent.com/SSujitX/docklift/master/install.sh | sudo DOCKLIFT_VERSION=3.0.0 bash
```

Tags may include or omit a leading `v`. Omitting the pin installs latest.

::: warning
Pinning or downgrading with `install.sh` rebuilds that tag and does **not** run `upgrade.sh`'s DB snapshot. Prefer `upgrade.sh` to move forward; backup before installing an older tag.
:::

### After install

The installer prints:

```text
Dashboard: http://YOUR_SERVER_IP:8080
Setup code: <bootstrap-secret>
```

1. Open that URL
2. Paste the setup code
3. Create the first admin account (or restore a backup)

Raw HTTP on a public IP is intentional for first-run convenience — it is not encrypted. Later options:

- **Settings → Domain** for an HTTPS panel hostname
- Firewall rules limiting who can reach `:8080`
- `DASHBOARD_BIND=127.0.0.1` in `/opt/docklift/.env` for localhost-only access (then use an SSH tunnel)

Recover the bootstrap secret if needed:

```bash
docker logs docklift-backend | grep -A8 "Fresh install"
```

```bash
sudo cat /opt/docklift/data/.bootstrap-secret
```

## Manual Compose install

```bash
git clone https://github.com/SSujitX/docklift.git
cd docklift
docker compose up -d
```

## Upgrade (preserves data)

Safe upgrade to the latest GitHub release. Keeps the database, projects, and user app containers; takes an auto-backup and can roll back on build/health failure.

```bash
curl -fsSL https://raw.githubusercontent.com/SSujitX/docklift/master/upgrade.sh | sudo bash
```

| Preserved | Notes |
|-----------|--------|
| Database | Control-plane SQLite and settings |
| Projects | Sources under `deployments/` |
| Certificates | nginx-proxy / certbot data |
| App containers | User workloads stay running where possible |

## Uninstall (destructive)

```bash
curl -fsSL "https://raw.githubusercontent.com/SSujitX/docklift/master/uninstall.sh" | sudo bash -s -- -y
```

This removes every Docklift container, image, volume and network, the build cache, and `/opt/docklift` (database, deployments, backups, certificates). Other Docker workloads on the same host are left alone, as are Docker Engine and git.

Manual teardown of a clone:

```bash
cd docklift
docker compose down -v
cd .. && rm -rf docklift
```

## Development build (latest master)

```bash
curl -fsSL https://raw.githubusercontent.com/SSujitX/docklift/master/install-dev.sh | sudo bash
```

Installs unreleased code from `master`. Use the production install for stable deployments.

## Local development

Prerequisites: Go (version in `server/go.mod`), [Bun](https://bun.sh/) 1.4.5, bash and make. Docker is needed for deploy features.

```bash
git clone https://github.com/SSujitX/docklift.git
cd docklift
make dev
```

`make dev` runs the Go API on `http://localhost:8000` and the Vite dev server on `http://127.0.0.1:3600`, which proxies `/api` and `/ws` to the API. Local state lives in `./data`, `./deployments`, `./backups`, and `./nginx-proxy`.

Without make, use two terminals:

```bash
cd server
go run ./cmd/docklift          # reads .env / .env.local from the working directory
```

```bash
cd frontend
bun install --frozen-lockfile
bun run dev
```

The SQLite file defaults to `data/docklift.db`. Migrations are embedded in the binary and applied on start; `docklift migrate` applies them and exits. See [Commands](./commands.md#_10-local-development) for the other `make` targets (`build`, `test`, `contract`, `lint`, `docker`).

## Upgrading from 2.x

Run the new `upgrade.sh` from GitHub:

```bash
curl -fsSL https://raw.githubusercontent.com/SSujitX/docklift/master/upgrade.sh | sudo bash
```

- The old `docklift-frontend` and `docklift-nginx` containers and the `docklift-frontend` image are removed. The stack is now `docklift-backend`, `docklift-nginx-proxy`, and `docklift-certbot`.
- The dashboard stays on `:8080`; the Go backend serves it directly.
- A panel domain nginx conf is rewritten once when the new backend boots (its upstream becomes `docklift-backend.docklift_network:8000`), then nginx is reloaded.
- Do not use the **Upgrade** button on a 2.1.x dashboard for this step. It runs the old local script, which cannot switch the container layout (it fails safely and rolls back).
- The database is snapshotted before the upgrade and restored automatically if the upgrade fails.

## Optional configuration

Create `/opt/docklift/.env` and re-run `docker compose up -d` from that directory:

| Variable | Default | Purpose |
|----------|---------|---------|
| `DASHBOARD_BIND` | `0.0.0.0` | Panel listen address. Default allows `http://SERVER_IP:8080`. Set `127.0.0.1` for localhost-only. |
| `PORT_RANGE_START` / `PORT_RANGE_END` | `5500` / `5600` | Host port pool when Publish host ports is enabled |
| `CERTBOT_EMAIL` | — | Let's Encrypt account email |
| `CERTBOT_STAGING` | `false` | Staging CA while testing (avoid rate limits) |
| `CORS_ORIGIN` | — | Extra allowed browser origins |
| `DOCKLIFT_FRONTEND_URL` | `http://localhost:8080` | Public dashboard URL (GitHub App callbacks) |
| `JWT_SECRET` | auto-generated | Override only if you manage secrets yourself |
| `INTERNAL_API_SECRET` | auto-generated | Unused (the `X-Internal-Secret` header is no longer accepted); still kept in `.secrets` for compatibility |
| `TRUSTED_PROXIES` | loopback + nginx-proxy's fixed IPs | Peers whose `X-Forwarded-*` headers are trusted (default `127.0.0.0/8,::1/128,172.28.255.254,fd12:3456:789a::ffff:fffe`; `none` = never) |

Secrets auto-generate and persist under `data/.secrets`.
