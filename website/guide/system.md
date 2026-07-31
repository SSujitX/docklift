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

What it does:

- Removes unused `docklift-*` app image tags that are outside each project’s **keep-2** set (current + previous successful deploy), skipping tags still used by running containers
- Clears **all** BuildKit cache (`docker builder prune -af`) so disk can recover after many redeploys
- Returns **409** if any deployment is still in progress (so in-flight build tags are not deleted)

What it never does:

- Host-wide `docker system prune`
- Delete non-`docklift-*` images (Postgres, nginx, etc.) or foreign containers
- Wipe OS caches, journals, apt caches, or `/tmp`

After every **successful** deploy, Docklift also keeps only two images per project service and prunes **unused** BuildKit cache automatically. See [Deployment](./deployment.md#disk-hygiene-and-restore).

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
