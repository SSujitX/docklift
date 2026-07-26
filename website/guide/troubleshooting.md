# Troubleshooting

Common issues and fixes. Pair with [Commands](./commands.md) for log and nginx helpers.

## Can't get past Setup

The bootstrap secret is required before the first admin account can be created. Finding the IP alone is not enough.

```bash
docker logs docklift-backend | grep -A8 "Fresh install"
```

```bash
sudo cat /opt/docklift/data/.bootstrap-secret
```

Paste the setup code into the Setup page. After a successful register/restore it is consumed and deleted.

## Can't open `SERVER_IP:55xx`

1. Host ports are **off by default** — use your custom domain, or enable **Publish host ports** in Build Settings and redeploy
2. Check `/ports` for reserved host ports
3. Confirm the container is running: `docker ps --filter name=dl_`
4. Overview may still show **Private by default** until you publish or attach a domain

Prefer domains over raw host ports.

## Domain returns 502

1. Confirm the app container is running and listening on its **internal** port (`0.0.0.0`, not only localhost)
2. Verify DNS: `nslookup yourdomain.com`
3. Ensure nginx-proxy is attached to the project network after deploy
4. Reload proxy:

```bash
docker exec docklift-nginx-proxy nginx -t
docker exec docklift-nginx-proxy nginx -s reload
```

5. Cloudflare: SSL mode **Full (strict)** once HTTPS is active (not Flexible)

## Domain returns nothing / NXDOMAIN

DNS record missing or not propagated. Flush local resolver cache and recheck the A record points at the server IP.

## Certificate order fails

A certificate request fails entirely if **any** hostname in the order lacks DNS. `www` is not auto-added — create DNS first, then add `www.example.com`.

```bash
docker logs docklift-certbot --tail 200
```

During first issue behind Cloudflare, prefer DNS-only (gray cloud) until Active, then Full (strict).

## Build failures

1. Check deployment logs in Project → Deployments
2. If "no Dockerfile", set build mode to **Railpack**, or fix **Base directory** / **Dockerfile path** for monorepos
3. Verify Dockerfile syntax and `EXPOSE` when using Dockerfile mode
4. Confirm build-time env vars and BuildKit secrets are marked correctly

## Container keeps stopping

```bash
docker logs dl_<slug>_<id>_<service> -f --tail 200
```

- Ensure the main process does not exit immediately
- Listen on `0.0.0.0` inside the container
- Match the configured internal port / `EXPOSE`

## Data disappears after redeploy

Anything written only inside the container filesystem is lost on replace. Add a mount in **Storage**, or use a [managed database](./databases.md) / external `DATABASE_URL`.

```bash
docker volume ls --filter label=com.docklift.project
```

## "A deployment is already running"

Cancel the in-flight build first. Deployments are serialized per project.

## Dashboard not reachable

```bash
docker compose ps
docker logs docklift-nginx -f --tail 100
curl -s http://127.0.0.1:8080/api/health
```

Default bind is `0.0.0.0` so `http://SERVER_IP:8080` works. If you set `DASHBOARD_BIND=127.0.0.1`, use an SSH tunnel or a panel domain on 80/443.

## Bypassing Cloudflare / scraping blocks

If a deployed app is blocked by Cloudflare but works from your SSH shell, Docker's network fingerprint may be detected. Run a lightweight SOCKS proxy on the host network and point the app at it (example):

```bash
docker run -d --name local-host-proxy --network host --restart unless-stopped \
  -e PROXY_USER=user -e PROXY_PASSWORD=pass serjs/go-socks5-proxy
```

Route app traffic through `socks5://user:pass@172.28.0.1:1080` (adjust to your bridge gateway). Traffic then leaves through the physical host network.
