# System Overview

Real-time monitoring of the host and Docker infrastructure from the **System** page — not only per-container stats.

## Metrics

| Metric | Details |
|--------|---------|
| **CPU** | Load averages (1 / 5 / 15), usage per core |
| **Memory (RAM)** | Used, free, cache, and swap |
| **Disk** | Capacity and read/write activity |
| **Network** | Upload and download rates |
| **GPU** | Shown when available on the host |
| **Processes** | Top processes on the machine |

Use this page to spot disk pressure, memory exhaustion, and runaway processes before deploys fail.

## Purge

**Purge** is a Docklift-scoped maintenance action that requires your **account password** (step-up auth). JWT alone is not enough.

Current behaviour is intentionally conservative on shared hosts:

- Does **not** run host-wide `docker prune`
- Does **not** restart foreign (non-Docklift) containers
- Does **not** wipe OS caches, journals, apt caches, or `/tmp`

Use the host shell or [Web Terminal](./terminal.md) for broader Docker cleanup when you know the host is dedicated to Docklift.

## Control plane actions

| Action | Effect |
|--------|--------|
| **Reset** | Restart Docklift platform services |
| **Reboot** | Restart the server |

Dangerous host actions from the terminal strip also require step-up password confirmation. Cancel aborts — the action does not continue.

## Logs

Dashboard → **Logs** streams platform containers over SSE:

- Backend
- Frontend
- Nginx (dashboard gateway on `:8080`)
- Nginx Proxy (public `:80`/`:443`)

From the host:

```bash
docker logs docklift-backend -f --tail 100
docker logs docklift-nginx -f --tail 100
docker logs docklift-nginx-proxy -f --tail 100
```

## Health

```bash
curl -s http://SERVER_IP:8080/api/health
```

On the server itself you can also use `http://127.0.0.1:8080/api/health`.
