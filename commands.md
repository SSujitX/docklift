# 🛠️ Docklift Management Commands

Useful commands for debugging and maintaining your Docklift instance.

## 📋 Table of Contents
- [⬆️ Upgrade](#-upgrade)
- [📜 Check Infrastructure Logs](#-check-infrastructure-logs)
- [🛰️ Project Debugging](#-project-debugging)
- [🧹 Cleaning & Resetting](#-cleaning--resetting)
- [🌐 Network & Port Check](#-network--port-check)

---

### ⬆️ Upgrade
```bash
# Safe upgrade (preserves all data and containers)
curl -fsSL https://raw.githubusercontent.com/SSujitX/docklift/master/upgrade.sh | sudo bash
```

---

### 📜 Check Infrastructure Logs
```bash
# View Backend logs
docker logs docklift-backend --tail 50 -f

docker logs -f docklift-backend

# View Frontend logs
docker logs docklift-frontend --tail 50 -f

docker logs -f docklift-frontend

# Build & run
docker compose up -d --build

# View Nginx Proxy logs (useful for 502/404 errors)
docker logs docklift-nginx-proxy --tail 50 -f
docker logs -f docklift-nginx-proxy 
```

### 🛰️ project Debugging
```bash
# List all Docklift-related containers
docker ps --filter name=dl_ --filter name=docklift_

# View logs for a specific project container
# (Replace dl_uuid_name with your container name)
docker logs dl_30e99d03_multiscraper_api --tail 100 -f
```

### 🧹 Cleaning & Resetting
```bash
# Nuclear Uninstall (Force-kills everything & deletes all data)
curl -fsSL "https://raw.githubusercontent.com/SSujitX/docklift/master/uninstall.sh?nocache=5" | sudo bash -s -- -y

# Force-kill anything holding ports 3001-3050 (Ghost processes)
sudo fuser -k 3001/tcp
# OR for the whole range:
for port in {3001..3050}; do sudo fuser -k ${port}/tcp 2>/dev/null; done
```

### 🌐 Network & Port Check
```bash
# Check if a port is in use and by what process
sudo netstat -tulpn | grep 3001

# Inspect the Docklift internal network
docker network inspect docklift_network
```

---

### 🚀 Development Commands (Bun)
```bash
# Database management
bunx prisma studio              # Open DB GUI
bunx prisma db push             # Push schema changes
bunx prisma generate            # Regenerate client

# Frontend dev
bun run dev                     # Start Next.js dev server
bunx next dev -p 3001           # Custom port

# Backend dev  
bun run dev                     # Start with tsx watch

# Build & lint
bun run build                   
bunx tsc --noEmit
```

### 📦 Update & Version Management (Bun)

```bash
bun outdated
bun update

bun update next@latest react@latest react-dom@latest eslint-config-next@latest

```

```bash
# Update all packages to latest
bunx npm-check-updates -u
bun install

# Version bumps
npm version patch               # 0.1.5 → 0.1.6 (bug fixes)
npm version minor               # 0.1.5 → 0.2.0 (new features)
npm version major               # 0.1.5 → 1.0.0 (breaking changes)
npm version 0.1.6               # Set specific version
npm version patch --no-git-tag-version  # No git commit/tag

# Version bumps
bunx bumpp
bunx bumpp --patch
bunx bumpp --minor
bunx bumpp --major
```
