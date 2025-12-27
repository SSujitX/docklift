# 🐳 Docklift

**Open-source, self-hosted PaaS for Docker deployments.**  
Deploy any application to your own server with one click. A free alternative to Heroku, Vercel, and Netlify. 
Similar to Coolify, CapRover, and Dokku, but simpler.

[![Next.js](https://img.shields.io/badge/Next.js-15-black)](https://nextjs.org)
[![Express](https://img.shields.io/badge/Express-Node.js-green)](https://expressjs.com)
[![Docker](https://img.shields.io/badge/Docker-Compose-blue)](https://docker.com)
[![License](https://img.shields.io/badge/License-MIT-yellow)](LICENSE)

> 🚀 **Self-hosted deployment platform** • Deploy Docker containers from GitHub or ZIP upload • Real-time build logs • Automatic port management • Built with Next.js 15 & Express

## ✨ Key Features

- **📦 One-Click Deployment** - Simply connect a GitHub repository or upload a ZIP file.
- **🐳 Docker Native** - Automatic generation of `Dockerfile` (if missing) and `docker-compose.yml`. 
- **🌐 Automatic Port Management** - No need to worry about port conflicts; Docklift handles them.
- **🔄 Zero-Downtime Redeploys** - Rebuild and restart containers seamlessly.
- **📜 Live Build Logs** - Watch your application build in real-time.
- **🐙 GitHub Integration** - Private repository support via GitHub Apps.

---

## 🛠️ Prerequisites

To run Docklift locally or on a server, you need:

1.  **Docker & Docker Compose** installed and running.
2.  **Node.js (v18+)** (for local development).

---

## 🚀 Getting Started

### 1. Clone the Repository

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

## 🤝 Contributing

Contributions are welcome! Please open an issue to discuss proposed changes or features.

## 📄 License

MIT License - see [LICENSE](LICENSE)

***

**Made with ❤️ for developers who take control.**
