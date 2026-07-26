<h1 align="center">🐳 Docklift</h1>

<p align="center">
  <strong>Self-hosted PaaS for Docker on your VPS</strong><br>
  Open-source Coolify alternative — and a practical Dokku, CapRover, Dokploy,<br>
  Heroku, Vercel &amp; Netlify alternative when you want git-to-deploy without the cloud bill.
</p>

<p align="center">
  <a href="https://github.com/SSujitX/docklift/stargazers"><img src="https://img.shields.io/github/stars/SSujitX/docklift?style=flat-square&color=cyan" alt="Stars"></a>
  <a href="https://github.com/SSujitX/docklift/releases"><img src="https://img.shields.io/github/v/release/SSujitX/docklift?style=flat-square&color=green" alt="Release"></a>
  <a href="https://vite.dev"><img src="https://img.shields.io/badge/Vite-React-646CFF" alt="Vite"></a>
  <a href="https://expressjs.com"><img src="https://img.shields.io/badge/Express-Node.js-green" alt="Express"></a>
  <img src="https://img.shields.io/badge/Docker-Compose-blue?style=flat-square" alt="Docker">
</p>

<p align="center">
  <a href="https://docklift.dev">Docs</a> •
  <a href="https://docklift.dev/guide/quick-start">Quick Start</a> •
  <a href="https://docklift.dev/guide/commands">Commands</a> •
  <a href="#-features">Features</a> •
  <a href="#-how-it-works">How it Works</a> •
  <a href="#-contributing">Contributing</a>
</p>

---

## 📖 What is Docklift?

Docklift is a **self-hosted PaaS** (platform-as-a-service) that turns a plain Linux VPS into your
own Docker deployment platform. Point it at a GitHub repo or upload a ZIP, and it builds a Docker
image, runs the containers, wires custom domains with automatic Let's Encrypt HTTPS, and redeploys
on every push — all from a web UI, with no vendor lock-in.

If you are searching for a **Coolify alternative**, a lighter **Dokku** / **CapRover** style
workflow with a real dashboard, or a **self-hosted Heroku / Vercel alternative** that stays on
hardware you control, Docklift is built for that job: deploy Docker apps on a VPS without
Kubernetes, Swarm, or a managed cloud account.

**Your server. Your rules. Your apps.**

![Docklift Dashboard](screenshots/project.png)

<details>
<summary><b>More screenshots</b></summary>

**Project detail** — All-services workspace (deploy / build / shared env) and per-service workspace (env, domains, storage, runtime logs); single-service projects stay flat
![Project detail](screenshots/inside%20projects.png)

**Live logs** — every platform container, streamed over SSE
![Logs](screenshots/logs.png)

**System monitoring** — CPU, RAM, GPU, disk, network, processes
![System](screenshots/system.png)

</details>

---

## 📑 Table of Contents

- [**Documentation site**](https://docklift.dev) — quick start, full guides, commands
- [Quick Start](#-quick-start)
- [Features](#-features)
- [How it Works](#-how-it-works)
- [Deploy Your First App](#-deploy-your-first-app)
- [Builds: Dockerfile or Railpack](#-builds-dockerfile-or-railpack)
- [Managed Databases](#-managed-databases)
- [Persistent Storage](#-persistent-storage)
- [Domains & HTTPS](#-domains--https)
- [Configuration](#-configuration)
- [Installation](#-installation)
- [Development Setup](#-development-setup)
- [Commands Reference](#-commands-reference)
- [Troubleshooting](#-troubleshooting)
- [Why Docklift (vs Coolify, Dokku, CapRover…)](#-why-docklift-vs-coolify-dokku-caprover)
- [FAQ](#-faq)
- [Contributing](#-contributing)
- [License](#-license)

---

## 🚀 Quick Start

Full walkthrough: **[docklift.dev/guide/quick-start](https://docklift.dev/guide/quick-start)** · Commands: **[docklift.dev/guide/commands](https://docklift.dev/guide/commands)**

One-command install on Ubuntu/Debian (latest GitHub release):

```bash
curl -fsSL https://raw.githubusercontent.com/SSujitX/docklift/master/install.sh | sudo bash
```

Pin a specific release (see [Releases](https://github.com/SSujitX/docklift/releases)) — pass the
version to bash, not in the curl URL:

```bash
curl -fsSL https://raw.githubusercontent.com/SSujitX/docklift/master/install.sh | sudo bash -s -- v=2.0.2
```

```bash
curl -fsSL https://raw.githubusercontent.com/SSujitX/docklift/master/install.sh | sudo DOCKLIFT_VERSION=2.0.2 bash
```

The installer prints your public URL, for example:

```text
Dashboard: http://YOUR_SERVER_IP:8080
Setup code: <bootstrap-secret>
```

Open that URL, paste the setup code, and create the first admin account. You can keep using the
IP:port panel, or later add an HTTPS panel domain under **Settings → Domain**.

> Raw HTTP on a public IP is intentional for first-run convenience — it is **not** private or
> encrypted. Put DockLift behind a firewall, use HTTPS, or set `DASHBOARD_BIND=127.0.0.1` if you
> want localhost-only access.

### First login needs the bootstrap secret

A fresh install prints a one-time **bootstrap secret** (setup code) to the install output and
backend logs. The Setup page requires it before the first admin account can be created, so finding
the IP alone is not enough to claim the panel. It is never exposed through any public API.

```bash
docker logs docklift-backend | grep -A8 "Fresh install"
```

```bash
sudo cat /opt/docklift/data/.bootstrap-secret
```

Paste it into the Setup page, register (or restore a backup), and the secret is consumed and deleted.

> For HTTPS on a hostname instead of an IP, add a panel domain under **Settings → Domain**.
> Unknown hostnames on ports 80/443 are rejected — they never fall through to the dashboard.

---

## ✨ Features

| Feature | Description |
|---------|-------------|
| 📦 **One-Click Deploy** | GitHub or ZIP → build → run on your VPS (Dockerfile or Railpack) |
| 🧱 **Automatic Builds** | Prefer your `Dockerfile`; otherwise [Railpack](https://railpack.com/) detects Node, Python, Go, and more |
| 🐙 **GitHub Integration** | Private repos via a GitHub App you own — push-to-deploy webhooks |
| 🔄 **Auto-Deploy** | Webhook-triggered redeploys on push, with a 10s debounce |
| 🌐 **Custom Domains + HTTPS** | Nginx reverse proxy and automatic Let's Encrypt (Cloudflare Full-strict ready) |
| 💾 **Persistent Storage** | Named volumes per service so SQLite and uploads survive rebuilds |
| 🧩 **Multi-Service Projects** | Multi-Dockerfile apps with all-services vs per-service env, domains, and logs |
| 📜 **Live Build Logs** | Real-time streaming output, with cancel mid-build |
| 🔐 **Env Variables** | Shared or per-service secrets; build-time and runtime scopes |
| 📊 **Host Monitoring** | CPU, RAM, GPU, disk, network, and top processes — not just container stats |
| 💻 **Web Terminal** | Root shell in the browser (xterm.js), behind double auth |
| 🗄️ **Backup & Restore** | Database, sources, vhosts, and certificates in one archive |
| 🧭 **Sidebar Workspace** | Collapsible rail plus `Ctrl+K` command palette |

---

## ⚙️ How it Works

Docklift is a small Docker Compose stack: a web UI, a backend that talks to the Docker socket, and
nginx to route public traffic to your apps.

```mermaid
flowchart LR
  Browser -->|:8080| DashNginx[docklift-nginx]
  DashNginx --> UI[Frontend]
  DashNginx --> API[Backend]
  Internet -->|:80 / :443| AppProxy[nginx-proxy]
  AppProxy -->|panel domain| DashNginx
  AppProxy -->|project domain| AppCtr[Your app containers]
  API -->|docker.sock| Docker[Docker Engine]
  API -->|write vhosts| AppProxy
  Certbot[certbot] -->|Let's Encrypt| AppProxy
  GitHub -->|webhook / push| API
```

### Stack pieces

| Container | Role |
|-----------|------|
| **docklift-frontend** | React (Vite) dashboard — projects, logs, settings, terminal |
| **docklift-backend** | Express + Prisma/SQLite — deploy, auth, GitHub App, nginx/SSL |
| **docklift-nginx** | Dashboard gateway on **`:8080`** |
| **docklift-nginx-proxy** | Public **`:80` / `:443`** for project and panel domains |
| **docklift-certbot** | Issues and renews Let's Encrypt certificates (HTTP-01) |

> Two nginx containers, two different jobs: `docklift-nginx` serves *the panel*,
> `docklift-nginx-proxy` serves *your apps*. The Logs page labels them **Dashboard Gateway** and
> **Public Proxy** for that reason.

### Deploy path

1. You create a project from a GitHub repo or a ZIP. Source lands in `deployments/<project-id>/`.
2. Docklift uses your `Dockerfile` when there is one, otherwise Railpack detects the stack and builds an image.
3. By default the app runs on a private project network (no public host port). Add a **custom domain** (preferred) so nginx-proxy serves it on `:80`/`:443`, or opt in to **Publish host ports** (`5500`–`5600` pool) for raw `IP:port`.
4. When you set a domain, the backend writes an nginx vhost, then asks certbot for a certificate.
5. With GitHub connected, a push webhook rebuilds and redeploys automatically.

Docklift writes its own runtime Compose file under `deployments/.docklift/<project-id>/`. Your
repository is **never modified** — a `docker-compose.yml` you committed yourself stays untouched.

### Access model

- **Admin UI:** `http://SERVER_IP:8080` by default. Prefer an HTTPS panel domain in **Settings → Domain** when you can. Optional: `DASHBOARD_BIND=127.0.0.1` + SSH tunnel.
- **Your apps:** public hostnames on `:80`/`:443` via nginx-proxy (project networks; host ports opt-in).
- **Secrets:** `JWT_SECRET` and internal keys auto-generate and persist under `data/.secrets`. First account requires the bootstrap secret.

---

## 🐳 Deploy Your First App

1. Open Docklift → **New Project**
2. Choose a source: GitHub URL, private repo, or ZIP upload
3. Add environment variables (optional) and pick a build mode (default: **Auto**)
4. **Deploy** → watch live build logs (cancel any time)
5. Add a domain (preferred) or enable **Publish host ports** + redeploy — Overview shows **Private by default** until then. Prefer domains: raw `IP:port` exposes your origin server.

---

## 🧱 Builds: Dockerfile or Railpack

Every project has a build mode, configurable in project settings.

| Mode | Behaviour |
|------|-----------|
| **Auto** (default) | Uses a repository `Dockerfile` if present, otherwise falls back to Railpack |
| **Dockerfile** | Always your Dockerfile — fails loudly instead of silently falling back |
| **Railpack** | Always Railpack, even if a Dockerfile exists |

**Railpack** (the builder behind Railway) detects Node, Python, Go, PHP, Ruby, Java, Rust and more
from manifests like `package.json`, `requirements.txt` or `pyproject.toml`, then builds an image with
BuildKit — no Dockerfile required.

Two settings help with non-trivial repos:

- **Base directory** — build from a subdirectory, for monorepos.
- **Dockerfile path** — point at a Dockerfile that isn't at the root.

Build-time variables are passed only when the Dockerfile actually declares them with `ARG`, so
runtime secrets don't get baked into image layers.

---

## 🗄️ Managed Databases

**Databases → New Database** creates Postgres, MySQL, MariaDB, Redis, or MongoDB from official
images (no Git/ZIP). Version tags load from Docker Hub automatically (majors / Alpine where
available). Host ports stay **off** by default. Database detail tabs are Overview, Deployments, and Logs
(credentials on Overview → Connection; no Environment / Build / Source / Domains).

**Ports** lists the opt-in host pool only. A Running app or database with publish
off will not show as Allocated — use domain/linking, or enable **Publish host ports**
and redeploy.

1. Create & deploy the database.
2. Open it → **Link** to a project (or one service), or from an app → **Attach database**.
3. DockLift joins the DB container to the app’s Docker network and injects `DATABASE_URL` /
   `REDIS_URL` / `MONGODB_URI` as a runtime secret.
4. **Redeploy the app** so containers pick up the new env.

Prefer linking over publishing host ports. Credentials are set at create time — **recreate** the
database to rotate passwords (env edits are blocked so Connection URLs cannot lie about the server).

---

## 💾 Persistent Storage

Redeploying replaces containers, so anything written inside a container's filesystem is lost unless
it's on a volume. If your app keeps a SQLite database, uploaded files, or a cache **inside** the
container, add a mount in the project's **Storage** tab.

- Volumes are external and labelled `com.docklift.project=<id>`, so `docker compose down` and
  redeploys never delete them.
- They are removed only when you delete the project.
- External databases reached over `DATABASE_URL` (Postgres, MySQL, managed services) need nothing here.
  For DockLift-managed databases, use **Databases** + link/attach instead of hand-pasting URLs.

```bash
# Inspect what Docklift manages
docker volume ls --filter label=com.docklift.project
```

---

## 🌐 Domains & HTTPS

1. Point an **A record** at your server's IP.
2. Add the domain to a service in Docklift.
3. Docklift writes an HTTP vhost, certbot solves the ACME HTTP-01 challenge, and the vhost is
   rewritten with HTTPS plus an HTTP→HTTPS redirect. Renewals run automatically every 12 hours.

**`www` is not added for you.** A certificate order fails *entirely* if one of its hostnames has no
DNS record, so silently adding `www.` would break the apex domain too. Create the `www` DNS record
first, then add `www.example.com` as an additional domain on the service.

**Behind Cloudflare:** use SSL mode **Full (strict)** once a certificate is issued, and make sure
Cloudflare can reach `/.well-known/acme-challenge/` over plain HTTP during issuance.

If issuance fails, the UI shows the specific certbot error plus a copyable command for the full log:

```bash
docker logs docklift-certbot --tail 200
```

---

## 🔧 Configuration

All of these are optional. Create `/opt/docklift/.env` (Compose picks it up automatically) and
re-run `docker compose up -d` from that directory to apply changes:

| Variable | Default | Purpose |
|----------|---------|---------|
| `DASHBOARD_BIND` | `0.0.0.0` | Panel listen address. Default allows `http://SERVER_IP:8080`. Set `127.0.0.1` for localhost-only, or use an HTTPS panel domain. |
| `PORT_RANGE_START` / `PORT_RANGE_END` | `5500` / `5600` | Host port pool when Publish host ports is enabled |
| `CERTBOT_EMAIL` | — | Let's Encrypt account email (expiry notices) |
| `CERTBOT_STAGING` | `false` | Use the staging CA while testing, to avoid rate limits |
| `CORS_ORIGIN` | — | Extra allowed browser origins, if the panel isn't same-origin |
| `DOCKLIFT_FRONTEND_URL` | `http://localhost:8080` | Public dashboard URL, used for GitHub App callbacks |
| `JWT_SECRET` / `INTERNAL_API_SECRET` | auto-generated | Override only if you manage secrets yourself |

---

## 📦 Installation

### Production (recommended)

Install (latest release):

```bash
curl -fsSL https://raw.githubusercontent.com/SSujitX/docklift/master/install.sh | sudo bash
```

Install a specific release (`bash -s -- v=2.0.2` or `DOCKLIFT_VERSION=2.0.2`):

```bash
curl -fsSL https://raw.githubusercontent.com/SSujitX/docklift/master/install.sh | sudo bash -s -- v=2.0.2
```

```bash
curl -fsSL https://raw.githubusercontent.com/SSujitX/docklift/master/install.sh | sudo DOCKLIFT_VERSION=2.0.2 bash
```

> Pinning/downgrading with `install.sh` rebuilds that tag and does **not** run `upgrade.sh`’s DB snapshot.
> Prefer `upgrade.sh` to move forward; backup before installing an older tag.

Upgrade to latest release (preserves data; always targets GitHub latest, not a pin):

```bash
curl -fsSL https://raw.githubusercontent.com/SSujitX/docklift/master/upgrade.sh | sudo bash
```

Uninstall:

```bash
curl -fsSL https://raw.githubusercontent.com/SSujitX/docklift/master/uninstall.sh | sudo bash -s -- -y
```

Uninstall removes every DockLift container, image, volume and network, the build cache, and
`/opt/docklift` (database, deployments, backups, certificates). Other Docker workloads on the same
host are left alone, as are Docker Engine and git.

### Development build (latest master)

```bash
curl -fsSL https://raw.githubusercontent.com/SSujitX/docklift/master/install-dev.sh | sudo bash
```

> ⚠️ Installs unreleased code from `master`. Use the production install for stable deployments.

### Docker Compose (manual)

```bash
git clone https://github.com/SSujitX/docklift.git
```

```bash
cd docklift
```

```bash
docker compose up -d
```

---

## 💻 Development Setup

**Prerequisites:** Docker and [Bun](https://bun.sh/).

```bash
git clone https://github.com/SSujitX/docklift.git
cd docklift
```

**Backend** — `localhost:8000`:

```bash
cd backend
cp .env.local.example .env.local   # your machine only (gitignored)
bun install
bun run db:generate && bun run db:push
bun run dev
```

**Frontend** (new terminal) — `localhost:3600`:

```bash
cd frontend && bun install && bun run dev
```

**Env files:** `backend/.env` is server/production config (committed).
`backend/.env.local.example` is the local template (committed) → copy to `.env.local` (gitignored)
for the Vite dev origin in `CORS_ORIGIN` and optional GitHub App credentials.

The `.agent/skills/` directory documents each subsystem (deployments, networking, security, UI) and
is the fastest way to get oriented in the codebase.

---

## 📋 Commands Reference

### 🔍 Logs

| Command | Description |
|---------|-------------|
| `docker logs docklift-backend -f` | Backend / API |
| `docker logs docklift-frontend -f` | Dashboard SPA |
| `docker logs docklift-nginx -f` | Dashboard gateway (`:8080`) |
| `docker logs docklift-nginx-proxy -f` | Public proxy (`:80`/`:443`) |
| `docker logs docklift-certbot -f` | Certificate issuance and renewals |
| `docker ps --filter name=dl_` | List project containers |

### 🗄️ Database

Run from `backend/`:

| Command | Description |
|---------|-------------|
| `bun run db:studio` | Open Prisma Studio GUI |
| `bun run db:migrate` | Apply checked-in Prisma migrations |
| `bun run db:ensure` | Production DB bootstrap (dedupe + migrate + repair) |
| `bun run db:generate` | Regenerate the Prisma client |
| `bun run db:push` | Local-only schema sync (not used on container boot) |

### 🧹 Maintenance

| Command | Description |
|---------|-------------|
| `docker exec -it docklift-backend node dist/scripts/reset-password.js` | Reset admin password |
| `docker exec docklift-nginx-proxy nginx -t` | Validate generated vhosts |
| `for port in {5500..5600}; do sudo fuser -k ${port}/tcp 2>/dev/null; done` | Free the app port pool |

> 📖 Full command guide: [commands.md](commands.md)

### 🏷️ Releases

Docklift uses [semantic-release](https://github.com/semantic-release/semantic-release) for
versioning, changelogs and GitHub Releases.

1. Push commits to `master` using [conventional commits](#commit-convention)
2. **GitHub → Actions → "Release & Test"** → **Run workflow**
3. semantic-release determines the version, bumps `package.json` (root, frontend, backend), updates
   `CHANGELOG.md`, tags, and publishes the release

| Commit subject | Release |
|----------------|---------|
| `feat:`, `fix:`, `perf:`, `refactor:`, `docs:`, `chore:` … | Patch (1.3.10 → 1.3.11) |
| contains `*force minor*` | Minor (1.3.10 → 1.4.0) |
| contains `BREAKING CHANGE` or `*force major*` | Major (1.3.10 → 2.0.0) |
| contains `*skip release*` | No release |

> 📖 Config: [release.config.cjs](release.config.cjs) · Workflow: [.github/workflows/release.yml](.github/workflows/release.yml)

---

## 🩺 Troubleshooting

| Symptom | Likely cause / fix |
|---------|--------------------|
| Can't get past Setup | Bootstrap secret required — see [First login](#first-login-needs-the-bootstrap-secret) |
| Build fails with "no Dockerfile" | Set build mode to **Railpack**, or fix **Base directory** for a monorepo |
| `502 Bad Gateway` on a domain | Container not running, or the app isn't listening on the configured internal port |
| Domain returns nothing / `NXDOMAIN` | DNS record missing or not propagated; flush your local resolver cache |
| Certificate order fails | One hostname in the request has no DNS record — check `docker logs docklift-certbot` |
| Data disappears after redeploy | Add a mount in the project's [Storage](#-persistent-storage) tab |
| "A deployment is already running" | Cancel the in-flight build first; deployments are serialized per project |

### 🛡️ Bypassing Cloudflare / scraping blocks

If a deployed app is blocked by Cloudflare but works from your SSH shell, Docker's network
fingerprint is being detected. Run a lightweight proxy on the host network:

```bash
docker run -d --name local-host-proxy --network host --restart unless-stopped \
  -e PROXY_USER=user -e PROXY_PASSWORD=pass serjs/go-socks5-proxy
```

Then route your app's requests (e.g. `curl_cffi`, Python `requests`) through
`socks5://user:pass@172.28.0.1:1080`. Traffic then leaves through the physical host network, exactly
like your SSH session.

---

## 🆚 Why Docklift (vs Coolify, Dokku, CapRover…)?

People comparing **self-hosted PaaS** options usually land on Coolify, Dokku, CapRover, or Dokploy.
Those are solid projects. Docklift aims at a narrower brief: a readable Docker deployment panel for
a single VPS (or a small fleet of apps on one host), without Swarm/Kubernetes ceremony.

| You want… | Typical pick | Why Docklift may fit instead |
|-----------|--------------|------------------------------|
| Polished UI + huge one-click catalog | Coolify / Dokploy | Smaller surface: GitHub/ZIP → Docker → domain → HTTPS |
| CLI `git push` Heroku clone | Dokku | Full web UI, live logs, host metrics, in-browser terminal |
| Swarm / one-click app store | CapRover | Plain Docker Compose under the hood — no Swarm required |
| Managed DX (Vercel / Netlify / Heroku) | Cloud PaaS | Same “push and get a URL” feel on **your** VPS and Docker host |

**What Docklift optimizes for**

- **Self-hosted Docker deploys** with a real dashboard (not CLI-only)
- **Dockerfile-first** builds, with Railpack when you do not have one
- **Automatic HTTPS** and custom domains via nginx + Let's Encrypt
- **Multi-service** projects (shared vs service env, domains, storage, runtime logs)
- **Ops visibility** built in: host monitoring and a gated web terminal

**Honest trade-offs:** Docklift is not trying to be the largest template marketplace or a multi-node
orchestrator. If you need 200+ one-click apps or Docker Swarm clustering on day one, Coolify,
Dokploy, or CapRover may match better. If you want a focused open-source **Coolify alternative**
(or a Dokku-like deploy model with a UI) to run containers on a VPS you control, start here.

---

## ❓ FAQ

### Is Docklift a Coolify alternative?

Yes — in the sense that both are open-source, self-hosted platforms for deploying apps with Docker,
domains, and HTTPS on your own server. Coolify is broader (templates, multi-server stories).
Docklift stays lighter and pairs deploys with host monitoring and a web terminal.

### Is Docklift a Dokku or CapRover alternative?

Dokku is CLI-first and buildpack-oriented; CapRover leans on Docker Swarm and one-click apps.
Docklift gives you a web UI, Dockerfile/Railpack builds, nginx HTTPS, and Compose-backed projects
without requiring Swarm.

### Can I use Docklift as a self-hosted Heroku or Vercel alternative?

For many apps, yes: connect GitHub (or upload a ZIP), build, attach a domain, get TLS, and
auto-redeploy on push. You keep the VPS, Docker socket, and data — no Heroku/Vercel account
required.

### Do I need Kubernetes?

No. Docklift talks to the Docker engine on the host and runs a small Compose stack for the panel
plus your app containers.

### What server do I need?

A typical Ubuntu/Debian VPS with Docker installed is enough to [install](#-quick-start) and deploy
your first app. Size the box for your workloads; the panel itself is a few containers, not a
full cluster control plane.

---

## 🤝 Contributing

Contributions are welcome:

1. Fork the repo
2. Create a feature branch (`git checkout -b feature/amazing`)
3. Commit your changes (`git commit -m 'feat: add amazing feature'`)
4. Push the branch (`git push origin feature/amazing`)
5. Open a Pull Request

Before opening a PR, please run:

```bash
cd backend  && bun run build && bun run test
cd frontend && bun run build
```

### Commit Convention

```
feat:     New feature
fix:      Bug fix
docs:     Documentation
style:    Formatting (no code change)
refactor: Code restructure
test:     Add tests
chore:    Maintenance
```

---

## 📄 License

MIT License — see [LICENSE](LICENSE)

---

<p align="center">
  <b>Made with ❤️ for developers who take control</b>
</p>

<p align="center">
  <a href="https://www.star-history.com/#SSujitX/docklift&Date">
    <img src="https://api.star-history.com/svg?repos=SSujitX/docklift&type=Date" width="500" alt="Star History">
  </a>
</p>

<p align="center">
  <img src="https://api.visitorbadge.io/api/visitors?path=https%3A%2F%2Fgithub.com%2FSSujitX%2Fdocklift&countColor=%23263759" alt="Visitors">
</p>
