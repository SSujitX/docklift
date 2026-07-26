# Port Management

Host ports are **opt-in**. By default an app can be **Running** but not public — Project → Overview shows **Private by default** until you add a domain/subdomain or enable **Publish host ports** and redeploy.

Prefer a [custom domain](./domains.md): raw `IP:port` reveals your origin server and is easier to scan. Preferred path: domain on nginx-proxy → project network → `container_name:internal_port`.

## Dashboard vs apps

| Port | Use |
|------|-----|
| `8080` | Docklift dashboard gateway (`docklift-nginx`) |
| `80` | Custom domains HTTP + ACME (`docklift-nginx-proxy`) |
| `443` | Custom domains HTTPS |
| `5500`–`5600` | Optional published app host ports (default pool) |
| `8000` | Backend (internal / local dev) |
| `3600` | Vite frontend (local `bun run dev`) |

Managed databases also keep host ports **off** by default — [link](./databases.md) them on the Docker network instead.

## Publish host ports

1. Open **Project → Build Settings**
2. Enable **Publish host ports**
3. **Redeploy**

Ports are taken from the pool below.

| Setting | Default | Description |
|---------|---------|-------------|
| `PORT_RANGE_START` | `5500` | First host port when publish is enabled |
| `PORT_RANGE_END` | `5600` | Last host port in the pool |

Configure in `/opt/docklift/.env` and recreate Compose services to apply.

## View allocations

Open `/ports` in the dashboard to see host ports reserved for projects that opted in.
Running apps/databases with host publish **off** stay private — they appear under
**Running without a host port**, not as Allocated pool rows.

## Host diagnostics

```bash
sudo ss -tulpn | grep -E ':(80|443|8080|550[0-9])\b'
```

```bash
# Free a stuck port (example)
sudo fuser -k 5500/tcp
```

```bash
# Free the whole default app pool
for port in {5500..5600}; do sudo fuser -k ${port}/tcp 2>/dev/null; done
```

```bash
docker network inspect docklift_network
docker network ls --filter label=com.docklift.managed=true
```
