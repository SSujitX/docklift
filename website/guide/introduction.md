# Introduction

**Docklift** is a self-hosted PaaS that turns a Linux VPS into your own Docker deployment platform. Point it at a GitHub repo or upload a ZIP, and it builds an image (Dockerfile or [Railpack](https://railpack.com/)), runs the containers, wires custom domains with Let's Encrypt HTTPS, and can redeploy on every push — all from a web UI.

Think of it as a lighter Coolify / Dokku-style workflow with a real dashboard: git-to-deploy on hardware you control, without Kubernetes or a managed cloud account.

**Your server. Your rules. Your apps.**

## What you get

| Capability | Summary |
|------------|---------|
| One-click deploy | GitHub or ZIP → build → run on your VPS |
| Automatic builds | Prefer your `Dockerfile`; otherwise Railpack detects Node, Python, Go, and more |
| GitHub App | Private repos via an app you own; push-to-deploy webhooks |
| Auto-deploy | Webhook redeploys on push (10s debounce) |
| Custom domains + HTTPS | nginx reverse proxy and automatic Let's Encrypt |
| Multi-service projects | Multiple Dockerfiles with shared vs per-service env, domains, and logs |
| Managed databases | Postgres, MySQL, MariaDB, Redis, MongoDB — private by default |
| Env & secrets | Shared or per-service; build-time and runtime scopes |
| Host monitoring | CPU, RAM, GPU, disk, network, top processes |
| Web terminal | Root shell in the browser, behind step-up password |
| Backup & restore | Control DB, sources, vhosts, and certificates in one archive |

## How it works

Docklift is a small Docker Compose stack: a React dashboard, an Express backend that talks to the Docker socket, and two nginx containers with different jobs.

| Container | Role |
|-----------|------|
| `docklift-frontend` | Dashboard UI |
| `docklift-backend` | Deploy, auth, GitHub App, nginx/SSL orchestration |
| `docklift-nginx` | Dashboard gateway on **`:8080`** |
| `docklift-nginx-proxy` | Public **`:80` / `:443`** for project and panel domains |
| `docklift-certbot` | Issues and renews Let's Encrypt certificates (HTTP-01) |

### Deploy path

1. Create a project from a GitHub repo or ZIP. Source lands in `deployments/<project-id>/`.
2. Docklift uses your `Dockerfile` when present; otherwise Railpack builds an image.
3. By default the app runs on a **private project network** (no public host port). Prefer a **custom domain** so nginx-proxy serves it on `:80`/`:443`, or opt in to **Publish host ports** (`5500`–`5600`) for raw `IP:port`.
4. With a domain set, the backend writes an nginx vhost and certbot obtains a certificate.
5. With GitHub connected and Auto-Deploy on, a push webhook rebuilds and redeploys.

Docklift writes its own runtime Compose file under `deployments/.docklift/<project-id>/`. Your repository is never modified.

### Access model

- **Admin UI:** `http://SERVER_IP:8080` by default. Prefer an HTTPS panel domain under **Settings → Domain** when you can. Optional: `DASHBOARD_BIND=127.0.0.1` plus an SSH tunnel for localhost-only.
- **Your apps:** public hostnames on `:80`/`:443` via nginx-proxy. Host ports are opt-in.
- **First login:** a one-time bootstrap secret (setup code) is required before the first admin account can be created.

Raw HTTP on a public IP is convenient for first-run — it is **not** private or encrypted. Put Docklift behind a firewall, use HTTPS, or bind the dashboard to localhost.

## Next steps

1. [Install](./installation.md) on your VPS
2. Follow the [Quick Start](./quick-start.md)
3. Connect [GitHub](./github.md), [deploy](./deployment.md), and attach a [domain](./domains.md)
