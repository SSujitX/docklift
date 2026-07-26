# Installation

Install Docklift on any Linux VPS with Docker. Production installs land under `/opt/docklift` by default.

## Production install (recommended)

Latest GitHub release:

```bash
curl -fsSL https://raw.githubusercontent.com/SSujitX/docklift/master/install.sh | sudo bash
```

Pin a specific release (see [Releases](https://github.com/SSujitX/docklift/releases)). Pass the version to bash, not in the curl URL:

```bash
curl -fsSL https://raw.githubusercontent.com/SSujitX/docklift/master/install.sh | sudo bash -s -- v=2.0.2
```

```bash
curl -fsSL https://raw.githubusercontent.com/SSujitX/docklift/master/install.sh | sudo DOCKLIFT_VERSION=2.0.2 bash
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

Prerequisites: Docker and [Bun](https://bun.sh/).

```bash
git clone https://github.com/SSujitX/docklift.git
cd docklift
```

**Backend** (`http://localhost:8000`):

```bash
cd backend
cp .env.local.example .env.local
bun install
bun run db:generate && bun run db:push
bun run dev
```

**Frontend** (separate terminal, `http://127.0.0.1:3600`):

```bash
cd frontend
bun install
bun run dev
```

`backend/.env` is server/production config (committed). Copy `.env.local.example` → `.env.local` (gitignored) for the Vite origin in `CORS_ORIGIN` and optional GitHub App credentials.

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
| `JWT_SECRET` / `INTERNAL_API_SECRET` | auto-generated | Override only if you manage secrets yourself |

Secrets auto-generate and persist under `data/.secrets`.
