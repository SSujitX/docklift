<h1 align="center">🐳 Docklift</h1>

<p align="center">
  <strong>Self-hosted Docker deployment platform</strong><br>
  Open-source alternative to Coolify, Dokploy, Dokku, CapRover, Vercel, Netlify & Heroku.
</p>

<p align="center">
  <a href="https://github.com/SSujitX/docklift/stargazers"><img src="https://img.shields.io/github/stars/SSujitX/docklift?style=flat-square&color=cyan" alt="Stars"></a>
  <a href="https://github.com/SSujitX/docklift/releases"><img src="https://img.shields.io/github/v/release/SSujitX/docklift?style=flat-square&color=green" alt="Release"></a>
  <a href="https://vite.dev"><img src="https://img.shields.io/badge/Vite-React-646CFF" alt="Vite"></a>
  <a href="https://expressjs.com"><img src="https://img.shields.io/badge/Express-Node.js-green" alt="Express"></a>
  <img src="https://img.shields.io/badge/Docker-Compose-blue?style=flat-square" alt="Docker">
</p>

<p align="center">
  <a href="#-quick-start">Quick Start</a> •
  <a href="#-how-it-works">How it Works</a> •
  <a href="#-features">Features</a> •
  <a href="#-commands">Commands</a> •
  <a href="#-contributing">Contributing</a>
</p>

---

## 📖 What is Docklift?

Docklift lets you deploy and manage Docker containers on your own server with a beautiful web UI. Connect GitHub repos or upload files, get auto-deployments, custom domains, and full system monitoring — without vendor lock-in.

**Your server. Your rules. Your apps.**

![Docklift Dashboard](screenshots/project.png)

---

## 🆚 Why Docklift?

There are great tools out there like **Coolify**, **Dokploy**, **Dokku**, and **CapRover** — but they often come with a learning curve, complex configurations, or feel heavy for simple use cases. 

Docklift is built to be **lightweight, minimal, and easy to understand**. It focuses purely on Docker deployments without the bloat, while offering features others don't — like full **system monitoring** (CPU, RAM, GPU, disk, network) and a **web terminal** right in your browser. 

If you want to deploy Docker containers quickly without wrestling with configurations, **Docklift is for you**.

---

## 📑 Table of Contents

- [Quick Start](#-quick-start)
- [How it Works](#-how-it-works)
- [Why Docklift](#-why-docklift)
- [Features](#-features)
- [Installation](#-installation)
- [Development Setup](#-development-setup)
- [Deploy Your First App](#-deploy-your-first-app)
- [Commands Reference](#-commands-reference)
- [Contributing](#-contributing)
- [License](#-license)


---

## 🚀 Quick Start

One-command install on Ubuntu/Debian:

```bash
curl -fsSL https://raw.githubusercontent.com/SSujitX/docklift/master/install.sh | sudo bash
```

Access the dashboard at `http://YOUR_SERVER_IP:8080`. For HTTPS on a hostname, add a panel domain under **Settings → Domain**. Unknown hostnames on ports 80/443 do not fall through to the dashboard.

---

## ⚙️ How it Works

Docklift is a small Docker Compose stack that turns your VPS into a PaaS: a web UI for projects, a backend that talks to the Docker socket, and nginx that routes public traffic to your apps.

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

| Service | Role |
|---------|------|
| **Frontend** | React (Vite) dashboard — projects, logs, settings, terminal |
| **Backend** | Express + Prisma/SQLite — deploy, auth, GitHub App, nginx/SSL |
| **docklift-nginx** | Dashboard gateway on **`:8080`** |
| **nginx-proxy** | Public **`:80` / `:443`** for project & panel domains |
| **certbot** | Issues/renews Let's Encrypt certs (HTTP-01) |

### Deploy path

1. You create a project (GitHub repo or ZIP). Source lands under `deployments/<project-id>/`.
2. Backend builds the image from your `Dockerfile` and runs a container on `docklift_network`.
3. A host port from the pool (`5500`–`5600` by default) is published so you can hit the app by IP:port.
4. If you set a **custom domain**, backend writes an nginx vhost and asks certbot for HTTPS.
5. With GitHub connected, a push webhook triggers rebuild/redeploy automatically.

### Access model

- **Admin UI:** `http://SERVER_IP:8080`, or a domain under **Settings → Domain**.
- **Your apps:** public hostnames on `:80`/`:443` via nginx-proxy (unknown hosts are rejected — they do not fall through to the dashboard).
- **Secrets:** JWT and internal keys auto-persist under `data/.secrets`; local overrides use `backend/.env.local`.

---

## ✨ Features

| Feature | Description |
|---------|-------------|
| 📦 **One-Click Deploy** | Push code → Docklift builds & runs it |
| 🐙 **GitHub Integration** | Connect private repos via GitHub Apps |
| 🔄 **Auto-Deploy** | Webhook-triggered redeploys on push |
| 🌐 **Custom Domains** | Nginx proxy + automatic Let's Encrypt HTTPS (Full strict ready) |
| 📊 **System Monitoring** | CPU, RAM, GPU, disk & network stats |
| 💻 **Web Terminal** | SSH-like access in your browser |
| 📜 **Live Build Logs** | Real-time streaming output |
| 🔐 **Env Variables** | Secure build-time & runtime secrets |

---

## 📦 Installation

### Production (Recommended)

**Install:**
```bash
curl -fsSL https://raw.githubusercontent.com/SSujitX/docklift/master/install.sh | sudo bash
```

**Upgrade (preserves data):**
```bash
curl -fsSL https://raw.githubusercontent.com/SSujitX/docklift/master/upgrade.sh | sudo bash
```

**Uninstall:**
```bash
curl -fsSL https://raw.githubusercontent.com/SSujitX/docklift/master/uninstall.sh | sudo bash -s -- -y
```

Removes every DockLift container, image, volume and network, the build cache, and
`/opt/docklift` (database, deployments, backups, certificates). Other Docker workloads on
the same host are left alone, as are Docker Engine and git.

### Development Build (Latest Master)

For testing the latest features before release:

```bash
curl -fsSL https://raw.githubusercontent.com/SSujitX/docklift/master/install-dev.sh | sudo bash
```

> ⚠️ **Warning:** This installs unreleased code from master branch. Use production install for stable deployments.

### Docker Compose

```bash
git clone https://github.com/SSujitX/docklift.git
cd docklift
docker compose up -d
```

---

## 💻 Development Setup

**Prerequisites:** Docker, [Bun](https://bun.sh/)

**Clone & setup:**
```bash
git clone https://github.com/SSujitX/docklift.git
cd docklift
```

**Backend:**
```bash
cd backend
cp .env.local.example .env.local   # your machine only (gitignored)
# edit .env.local if needed — GitHub optional; or use Settings → Create App
bun install
bun run db:generate && bun run db:push
bun run dev
```

**Frontend (new terminal):**
```bash
cd frontend && bun install && bun run dev
```

**Dev URLs:** Frontend `localhost:3600` | Backend `localhost:8000`  

**Env files:** `backend/.env` = server/production (committed). `backend/.env.local.example` = local template (committed) → copy to `.env.local` (gitignored) for Vite CORS / optional GitHub.

---

## 🐳 Deploy Your First App

1. Open Docklift → Click **New Project**
2. Choose source: GitHub URL, Private Repo, or ZIP Upload
3. Add environment variables (optional)
4. Click **Deploy** → Watch live build logs
5. Access at `http://your-ip:<assigned-port>`

> **Requirement:** Your project must have a `Dockerfile`

### 🛡️ Bypassing Cloudflare / Web Scraping Blocks
If your deployed apps are blocked by Cloudflare (but work on your server terminal), Docker's network fingerprint is being detected. Run a lightweight proxy on your host network to bypass this:
```bash
docker run -d --name local-host-proxy --network host --restart unless-stopped -e PROXY_USER=user -e PROXY_PASSWORD=pass serjs/go-socks5-proxy
```
Then, update your app's code (like `curl_cffi` or Python `requests`) to route through `socks5://user:pass@172.28.0.1:1080`. This sends traffic through your physical host network, perfectly replicating your SSH terminal.

---

## 📋 Commands Reference

### 🔍 Logs

| Command | Description |
|---------|-------------|
| `docker logs docklift-backend -f` | Backend logs |
| `docker logs docklift-frontend -f` | Frontend logs |
| `docker logs docklift-nginx -f` | Dashboard gateway (:8080) |
| `docker logs docklift-nginx-proxy -f` | App domains proxy (:80) |
| `docker ps --filter name=dl_` | List project containers |

### 🗄️ Database

| Command | Description |
|---------|-------------|
| `bun run db:studio` | Open Prisma Studio GUI |
| `bun run db:push` | Push schema changes |
| `bun run db:generate` | Regenerate Prisma client |

### 🏷️ Auto Release

Docklift uses [semantic-release](https://github.com/semantic-release/semantic-release) to automate versioning, changelogs, and GitHub Releases.

**How to release:**
1. Push commits to `master` using [conventional commits](#commit-convention) (e.g. `fix:`, `feat:`)
2. Go to **GitHub → Actions → "Release & Test"** → Click **"Run workflow"**
3. Done! semantic-release automatically:
   - Determines the next version from commit messages
   - Bumps `package.json` (root, frontend, backend)
   - Updates `CHANGELOG.md`
   - Creates a git tag and GitHub Release

| Commit Type | Release |
|-------------|---------|
| `feat:`, `fix:`, `perf:`, `refactor:` | Patch (1.3.10 → 1.3.11) |
| `*force minor*` in subject | Minor (1.3.10 → 1.4.0) |
| `BREAKING CHANGE` | Major (1.3.10 → 2.0.0) |
| `*skip release*` in subject | No release |

> 📖 Config: [release.config.cjs](release.config.cjs) | Workflow: [.github/workflows/release.yml](.github/workflows/release.yml)

### 🧹 Cleanup

| Command | Description |
|---------|-------------|
| `for port in {5500..5600}; do sudo fuser -k ${port}/tcp 2>/dev/null; done` | Kill app port pool |
| `docker exec -it docklift-backend node dist/scripts/reset-password.js` | Reset admin password |

> 📖 Full commands guide: [COMMANDS.md](COMMANDS.md)

---

## 🤝 Contributing

Contributions welcome! Please:

1. Fork the repo
2. Create a feature branch (`git checkout -b feature/amazing`)
3. Commit changes (`git commit -m 'feat: add amazing feature'`)
4. Push to branch (`git push origin feature/amazing`)
5. Open a Pull Request

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