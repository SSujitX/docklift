---
name: Networking & Proxy
description: Guide to Docklift's networking, custom domains, Nginx reverse proxy, and Let's Encrypt SSL.
---

# Networking & Proxy Guide

Docklift runs **two** nginx containers. Keep their roles straight:

| Container | Listens | Role |
|-----------|---------|------|
| `docklift-nginx` | `:8080` (host) → `:80` | **Dashboard gateway** — serves the panel SPA + API, same-origin |
| `docklift-nginx-proxy` | `:80`, `:443` (host) | **Public edge** — user app domains, the panel domain, TLS termination |
| `docklift-certbot` | — | ACME HTTP-01 issuance + renewal every 12h |

## Dashboard bind (`DASHBOARD_BIND`)

Default in `docker-compose.yml`: **`0.0.0.0:8080`** so a fresh install is reachable at
`http://SERVER_IP:8080`. The installer prints that URL plus the one-time **setup code**
(`data/.bootstrap-secret`). First account creation still requires that secret — finding the IP alone
must not claim the panel.

Optional hardening (operator choice):
- Add an HTTPS panel domain under **Settings → Domain**
- Firewall / VPN in front of `:8080`
- Set `DASHBOARD_BIND=127.0.0.1` for localhost-only access

Do **not** market raw HTTP as private or encrypted. Do **not** force localhost by default — that
breaks intended server install UX.

## Network isolation (user projects)

Control-plane services stay on **`docklift_network`**.

Each deployed project gets its **own bridge network** named `dl-net-<shortProjectId>`
(`projectNetworkName()` in `lib/naming.ts`). Generated runtime compose attaches app containers only
to that network, with labels:

- `com.docklift.managed=true`
- `com.docklift.project=<projectId>`
- `com.docklift.service=<serviceName>`

After `compose up`, the backend calls `connectProxyToProjectNetwork(projectId)` so
`docklift-nginx-proxy` can resolve `container_name` via Docker DNS. That call **throws** on failure —
deploy must not report success or activate domains if attach failed. Control-plane containers are
**not** joined to project networks.

Before stop / cancel / delete `compose down`, call `disconnectProxyFromProjectNetwork()` so Docker
can remove the project network. On delete, `teardownProjectNetwork()` also removes the network.

### Upstream routing (apps)

Proxy location templates in `nginxSsl.ts` → `buildServiceProxyLocation()` use:

```nginx
set $target_<id> <container_name>;
proxy_pass http://$target_<id>:<internal_port>;
```

with `resolver 127.0.0.11` (Docker embedded DNS). **Do not** route user apps through
`host.docker.internal:<published-port>` for normal domain traffic.

### Host ports (opt-in)

`Project.publish_host_port` defaults to **`false`**. When false, runtime compose publishes **no**
host ports — traffic reaches apps via domain → nginx-proxy → project network.

When true, ports are allocated from `PORT_RANGE_*` and published as `host:internal`. Toggle lives in
project Build Settings. Never bind user apps to `127.0.0.1` as a substitute for isolation — that
breaks gateway routing unless the proxy model is redesigned.

## Key Files

| Path | Purpose |
|------|---------|
| `nginx.conf` | Dashboard gateway config (mounted into `docklift-nginx`) |
| `nginx-proxy/nginx.conf` | Public proxy top-level config: TLS defaults, log format, `$connection_upgrade` map |
| `nginx-proxy/snippets/acme.conf` | Shared ACME challenge location, included by every `listen 80` block |
| `nginx-proxy/conf.d/service-<serviceId>.conf` | **Generated** per-service vhost — never hand-edit |
| `nginx-proxy/certbot/www` | ACME webroot (shared with certbot) |
| `nginx-proxy/certbot/conf` | `/etc/letsencrypt` — certificates (RO in proxy, RW in backend) |
| `backend/src/services/nginx.ts` | Writes vhosts, reloads nginx, reconciles orphans |
| `backend/src/services/nginxSsl.ts` | Server-block templates (`buildHttpHttpsServers`) |
| `backend/src/services/compose.ts` | Runtime compose: project network, labels, limits, opt-in ports |
| `backend/src/services/docker.ts` | `connectProxyToProjectNetwork` (throws) / `disconnectProxyFromProjectNetwork` / `teardownProjectNetwork` |
| `backend/src/services/certs.ts` | Issuance, status, renewal watcher, error summarizing |

## How Routing Works

1. **Domain assigned** to a service (a project can have several services, each with its own domain).
   The `domain` field accepts a comma-separated list; the **first** entry is the primary/cert name.
2. **Config generated** at `nginx-proxy/conf.d/service-<serviceId>.conf` via `updateServiceDomain()`.
   A config is written only when the service has a domain, a container name, and status
   `running` or `starting`; otherwise an existing file is removed.
3. **HTTP first, then HTTPS**: the initial write is an HTTP-only block that includes the ACME snippet,
   so the challenge can succeed. After a certificate exists, the file is rewritten with the HTTPS
   server block and an HTTP→HTTPS redirect.
4. **Reload**: `reloadNginx()` in `nginx.ts` — spawn reload, optional self-heal, then **throws** on
   failure. Callers must not treat a failed reload as success (domain PUT rolls DB back on throw).
5. **Duplicate hostnames**: `lib/domainOwnership.ts` rejects a hostname already owned by another
   service or panel conf before save.
6. **Reconciliation**: `syncNginxConfigs()` deletes `service-*.conf` files whose service no longer
   exists, so stale vhosts do not keep hijacking a hostname.

## SSL / Let's Encrypt

- Issuance runs `certbot certonly --webroot` inside `docklift-certbot` (shared webroot volume).
- `CERTBOT_EMAIL` / the `ssl_acme_email` setting is the ACME account email; `CERTBOT_STAGING=true`
  switches to the staging CA for testing without burning rate limits.
- Public IP discovery for DNS checks uses **HTTPS** (`api.ipify.org`), never plaintext IP APIs.
- `startCertRenewWatcher()` reloads the proxy after the certbot sidecar renews something.
- Certificates live at `/etc/letsencrypt/live/<primaryDomain>/{fullchain,privkey}.pem`
  (`nginxCertPaths()`); `resolveCertName()` maps an arbitrary hostname back to its cert directory.

### `www` is never added implicitly

Only hostnames the user explicitly entered are put in the vhost and in the certificate SANs.
Auto-adding `www.` was a real bug: if the `www` DNS record does not exist, the ACME order for the
whole certificate fails, so the apex domain gets **no** certificate either. Users who want `www` add
it as another comma-separated domain — and must create the DNS record first.

### DNS preflight

`issueCertificate()` resolves every requested hostname first (`services/dnsCheck.ts`). When a
resolver answers "no such record" (ENOTFOUND/ENODATA) issuance is **skipped** with an actionable
error instead of calling certbot: ACME fails the whole order anyway, and failed orders count against
the account rate limit. Lookup failures that are our fault (SERVFAIL, timeout) do not block.

### Activity log

`appendSslEvent()` / `getSslEvents()` keep the last 40 issuance events per hostname in memory —
preflight results, certbot start, success or failure, nginx reloads. `GET
/api/deployments/:projectId/services/:serviceId/ssl` returns `{ ssl, events }`, and the project
Domains tab polls it every 2s while work is in flight so users watch issuance happen. Events are
narration only; PEMs remain the source of truth.

### Error reporting

`summarizeCertbotError()` reduces certbot's very verbose output to one actionable line, filtering
boilerplate like "An unexpected error occurred". `components/domains/ServiceDomainCard.tsx` shows
that summary with numbered remediation steps (`domains/sslHelp.ts`), plus a copyable command for the
raw logs:

```bash
docker logs docklift-certbot --tail 200
```

### Cloudflare

Cloudflare's SSL mode must be **Full (strict)** once a real certificate is issued. With the orange
cloud enabled, Cloudflare must still be able to reach `/.well-known/acme-challenge/` over plain HTTP
for issuance to work.

## Config Template

Generated configs look like this (simplified):

```nginx
server {
    listen 80;
    server_name app.example.com;
    include /etc/nginx/snippets/acme.conf;
    return 301 https://$host$request_uri;     # only once a cert exists
}

server {
    listen 443 ssl;
    http2 on;
    server_name app.example.com;

    ssl_certificate     /etc/letsencrypt/live/app.example.com/fullchain.pem;
    ssl_certificate_key /etc/letsencrypt/live/app.example.com/privkey.pem;

    resolver 127.0.0.11 valid=30s ipv6=off;

    location / {
        set $target_svc dl_myapp_53b01966_app;
        proxy_pass http://$target_svc:3000;
        proxy_set_header Host $host;
        proxy_set_header X-Forwarded-Host $http_host;      # never trust the client's value
        proxy_set_header X-Forwarded-Proto $scheme;
        proxy_set_header Upgrade $http_upgrade;
        proxy_set_header Connection $connection_upgrade;
    }
}
```

### Two security rules for proxy headers

1. **Always set `X-Forwarded-Host` from `$http_host`.** Passing a client-supplied
   `X-Forwarded-Host` through lets an attacker forge the origin the backend reconstructs in
   `requestOrigin()`, defeating the same-origin check.
2. **Unknown hostnames must not fall through to the dashboard.** The default server rejects
   hostnames that match no vhost instead of serving the panel, so the admin UI is never reachable
   through an unconfigured domain.

## Troubleshooting

- **502 Bad Gateway**: container not running, app not listening on `internal_port`, proxy not
  attached to the project network (`connectProxyToProjectNetwork`), or wrong `container_name`.
- **404 / connection refused**: no vhost matches that `server_name`, or DNS does not point at the server.
- **App only reachable on `:5500`**: host ports are opt-in — enable **Publish host ports** on the
  project and redeploy, or use a domain.
- **`DNS_PROBE_FINISHED_NXDOMAIN`**: DNS record missing — or, if you just created it, a stale local
  resolver cache (`ipconfig /flushdns`). Not a Docklift issue.
- **Certificate order fails on a multi-domain request**: one SAN's DNS is missing; ACME fails the
  whole order. Remove that hostname or create its record.
- **Config errors**: `docker exec docklift-nginx-proxy nginx -t`
- **Vhost for a deleted project still answering**: run a reconcile, or check `syncNginxConfigs()`.
