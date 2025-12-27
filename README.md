<p align="center">
  <a href="https://nextjs.org"><img src="https://img.shields.io/badge/Next.js-16-black" alt="Next.js"></a>
  <a href="https://expressjs.com"><img src="https://img.shields.io/badge/Express-Node.js-green" alt="Express"></a>
  <a href="https://docker.com"><img src="https://img.shields.io/badge/Docker-Compose-blue" alt="Docker"></a>
  <a href="LICENSE"><img src="https://img.shields.io/badge/License-MIT-yellow" alt="License"></a>
</p>

# 🐳 Docklift

**Docklift is an open-source & self-hostable alternative to Heroku / Netlify / Vercel / etc.**

It helps you deploy and manage your applications on your own hardware - you only need a VPS with Docker installed. Deploy from GitHub or upload files directly, manage custom domains, monitor system resources, and access your server terminal - all from a beautiful web interface.

**Imagine having the ease of a cloud platform but on your own servers. That is Docklift.**

No vendor lock-in, which means all your application data, configurations, and Docker containers live on your server. If you decide to stop using Docklift, your apps keep running - you just lose the beautiful dashboard and automations. 🪄

---

> 🚀 **Self-hosted deployment platform** • Deploy Docker containers from GitHub or ZIP upload • Real-time build logs • Automatic port management • Custom domains • System monitoring • Web terminal • Built with Next.js 16 & Express

![Docklift Dashboard](screenshots/home.png)

## ✨ Key Features

- **📦 One-Click Deployment** - Simply connect a GitHub repository or upload a ZIP file.
- **🐳 Docker Native** - Automatic generation of `Dockerfile` (if missing) and `docker-compose.yml`. 
- **🌐 Automatic Port Management** - No need to worry about port conflicts; Docklift handles them.
- **🔄 Zero-Downtime Redeploys** - Rebuild and restart containers seamlessly.
- **📜 Live Build Logs** - Watch your application build in real-time.
- **🐙 GitHub Integration** - Private repository support via GitHub Apps.

![Deployment In Progress](screenshots/deploy.png)


---

## 🛠️ Prerequisites

To run Docklift locally or on a server, you need:

1.  **Docker & Docker Compose** installed and running.
2.  **Node.js (v18+)** (for local development).

---

## 🚀 Getting Started

### 1. One-Command Install (Recommended)

Run this on your Ubuntu/Debian server to set up everything automatically:

#### Install

```bash
curl -fsSL https://raw.githubusercontent.com/SSujitX/docklift/master/install.sh | sudo bash
```

#### Uninstall

```bash
curl -fsSL "https://raw.githubusercontent.com/SSujitX/docklift/master/uninstall.sh?nocache=1" | sudo bash -s -- -y
```

### 2. Manual Installation (Clone Repo)

```bash
git clone https://github.com/SSujitX/docklift.git
cd docklift
```

### 2. Configure Environment

Docklift uses environment variables for configuration. **Do not commit `.env` files.**

```bash
# Backend Setup
cd backend
cp .env.example .env
# Edit .env if needed (default ports and DB paths are usually fine)
```

### 3. Run with Docker Compose (Recommended)

The easiest way to run Docklift is using the composed setup.

```bash
# From the root directory
docker compose up -d
```

Access the dashboard at: `http://localhost:3000`

### 4. Local Development (Manual Setup)

If you want to contribute or modify code:

**Backend:**
```bash
cd backend
npm install
# Ensure you created .env from .env.example
npm run dev
```
*Backend runs on port 8000.*

**Frontend:**
```bash
cd frontend
npm install
npm run dev
```
*Frontend runs on port 3000.*

---

## 🐳 Deploying Your First App

1.  Open Docklift (`http://localhost:3000`).
2.  Click **"New Project"**.
3.  Choose a source:
    *   **Public GitHub**: Paste a repo URL.
    *   **Private GitHub**: Connect the Docklift GitHub App.
    *   **Direct Upload**: Upload a ZIP file of your code. (Make sure it has a `Dockerfile` for best results!)
4.  Docklift will detect the branch (defaults to `main`).
5.  Click **Deploy**.
6.  Your app will be live at `http://localhost:<ASSIGNED_PORT>`.

---

## 📂 Project Structure Guide

Docklift is designed to be flexible. It automatically scans your project for `Dockerfile` files. 

### 1. Single-Service Project (Recommended for simple apps)
Use this for a standard Next.js, Python, or Go application.

```text
my-cool-app/
├── Dockerfile          <-- Required (at the root)
├── package.json        (or requirements.txt, main.go, etc.)
├── src/
└── ...
```

### 2. Multi-Service Project (Mono-repo style)
Docklift will detect each `Dockerfile` and create separate services for them within the same project.

```text
my-complex-app/
├── api/
│   ├── Dockerfile     <-- Service 1 (e.g. FastAPI)
│   ├── main.py
│   └── requirements.txt
├── dashboard/
│   ├── Dockerfile     <-- Service 2 (e.g. Next.js)
│   ├── package.json
│   └── ...
└── README.md
```

> [!TIP]
> **Docklift Tip**: Each subdirectory containing a `Dockerfile` becomes an independent container with its own internal port and optional custom domain.

---

## 🤝 Contributing

Contributions are welcome! Please open an issue to discuss proposed changes or features.

## 📄 License

MIT License - see [LICENSE](LICENSE)

***

**Made with ❤️ for developers who take control.**


## Star History

[![Star History Chart](https://api.star-history.com/svg?repos=SSujitX/docklift&type=date&legend=top-left)](https://www.star-history.com/#SSujitX/docklift&type=date&legend=top-left)

![Visitors](https://api.visitorbadge.io/api/visitors?path=https%3A%2F%2Fgithub.com%2FSSujitX%2Fdocklift&countColor=%23263759)