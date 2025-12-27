# 🛠️ Docklift Management Commands

Useful commands for debugging and maintaining your Docklift instance.

### 📜 Check Infrastructure Logs
```bash
# View Backend logs
docker logs docklift-backend --tail 50 -f

# View Nginx Proxy logs (useful for 502/404 errors)
docker logs docklift-nginx-proxy --tail 50 -f
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