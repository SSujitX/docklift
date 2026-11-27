---
name: Networking & Proxy
description: Guide to Docklift's networking, custom domains, the nginx-proxy reverse proxy, and Let's Encrypt SSL (Go backend).
---

# Networking & Proxy Guide

Docklift has **one** nginx container. Keep the roles of the three containers straight:

| Container | Listens | Role |
|-----------|---------|------|
| `docklift-backend` | `:8080` (host, `DASHBOARD_BIND`) -> `:8000` (container `PORT`) | Go binary serving the API, WebSocket terminal and the embedded dashboard, same-origin. No gateway nginx in front of it. |
| `docklift-nginx-proxy` | `:80`, `:443` (host) | **Public edge**: user app domains, the optional panel domain, TLS termination |
| `docklift-certbot` | none | ACME HTTP-01 renewal loop (every 12 h); issuance runs through `docker exec` from the backend |

The 2.x gateway container `docklift-nginx` (with its own `nginx.conf` and `docklift-nginx:80` upstream) and `docklift-frontend` no longer exist. Panel-domain confs that still point at them are rewritten at boot (see "Panel domains").

## Dashboard bind (`DASHBOARD_BIND`)

`docker-compose.yml` publishes `"${DASHBOARD_BIND:-0.0.0.0}:8080:8000"` on `docklift-backend`, so a fresh install is reachable at `http://SERVER_IP:8080`. The installer prints that URL plus the one-time **setup code** (`data/.bootstrap-secret`, also logged by the backend on a fresh install). First account creation still requires that secret; finding the IP alone must not claim the panel.

Optional hardening (operator choice):
- Add an HTTPS panel domain under **Settings → Domain**
- Firewall / VPN in front of `:8080`
- Set `DASHBOARD_BIND=127.0.0.1` for localhost-only access

Do **not** market raw HTTP as private or encrypted. Do **not** force localhost by default; that breaks the intended server install flow.

## Control-plane network

`docklift_network` (`docker-compose.yml`: `name: docklift_network`, bridge, IPv6 on) holds `docklift-backend`, `docklift-nginx-proxy` and `docklift-certbot`. Subnets `172.28.0.0/16` and `fd12:3456:789a::/64`.

`docklift-nginx-proxy` has **fixed addresses** at the top of each subnet: `172.28.255.254` and `fd12:3456:789a::ffff:fffe`. Docker hands out dynamic addresses from the bottom, so they cannot collide. The backend's `TRUSTED_PROXIES` default is loopback plus exactly these two addresses (`config.DefaultTrustedProxies`): `X-Forwarded-For`, `-Proto` and `-Host` are honoured only when the socket peer is a trusted proxy. Because the dashboard is also reachable directly on `:8080`, a direct client cannot forge its address, scheme or host, and app containers on the network are not trusted. Changing the subnet or the addresses means changing `TRUSTED_PROXIES`.

The backend creates the network on startup if it is missing (`EnsureNetwork`, name from `DOCKER_NETWORK`, default `docklift_network`).

## Network isolation (user projects)

Each deployed project gets its **own bridge network** named `dl-net-<id8>` (`ProjectNetworkName` in `server/internal/deploy/naming.go`; `id8` = first 8 hex characters of the project UUID). The generated runtime compose attaches app containers only to that network, with labels:

- `com.docklift.managed=true`
- `com.docklift.project=<projectId>`
- `com.docklift.service=<serviceName>`

After `compose up`, the backend calls `Runtime.ConnectProxyToProjectNetwork` (`dockerx.ConnectProxy`) so `docklift-nginx-proxy` can resolve `container_name` through Docker DNS. It inspects the network first, treats "already connected" as success and returns every other failure: for an app, the deploy is then marked **failed** and domains are **not** activated. Control-plane containers are **not** joined to project networks. Linked managed database containers are joined (see `managed_databases`).

Before stop / cancel / delete `compose down`, `DisconnectProxyFromProjectNetwork` detaches the proxy so Docker can remove the network. On delete, `TeardownProjectNetwork` also removes it. If teardown fails, the proxy is reconnected. At backend startup `ReattachProxyOnBoot` re-attaches the proxy to every existing project network, because the attachment belongs to the proxy container: recreating `docklift-nginx-proxy` (the first upgrade from 2.x does, because of the fixed addresses) would otherwise leave every app domain at 502 until each project was redeployed.

### Reserved service names

A runtime compose service key is a DNS alias on the project network, and the proxy joins every project network. A scanned service whose name equals or starts with `docklift` (case-insensitive) is therefore **refused** (`deploy.IsReservedServiceName`; `GET /api/projects/:id/build/detect` returns 400, a deploy fails with the same message). Without that, a project could answer for `docklift-backend`.

### Upstream routing (apps)

`BuildServiceProxyLocation` (`server/internal/nginx/templates.go`) renders:

```nginx
location / {
    set $target_<serviceId with _> <container_name>;
    proxy_pass http://$target_<serviceId with _>:<internal_port>;
    proxy_http_version 1.1;
    proxy_set_header Upgrade $http_upgrade;
    proxy_set_header Connection $connection_upgrade;
    proxy_set_header Host $http_host;
    proxy_set_header X-Real-IP $remote_addr;
    proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
    proxy_set_header X-Forwarded-Proto $scheme;
}
```

with `resolver 127.0.0.11 valid=30s ipv6=off;` (Docker embedded DNS) in the server block. The upstream is a variable, so nginx starts even when the container is down. **Do not** route user apps through `host.docker.internal:<published-port>` for normal domain traffic. Service id, container name and port are validated before they reach the file (`InvalidInputError`).

### Host ports (opt-in)

`projects.publish_host_port` defaults to **false**. When false, runtime compose publishes **no** host ports: traffic reaches apps via domain -> nginx-proxy -> project network.

When true, a host port is allocated from `PORT_RANGE_START`-`PORT_RANGE_END` (default 5500-5600, `deploy.AllocatePort`, transactional) and published as `host:internal`. The toggle lives in project Build Settings; `GET /api/ports` shows the pool and `DELETE /api/ports/:port` releases a port that no running container or service uses. Never bind user apps to `127.0.0.1` as a substitute for isolation; that breaks proxy routing unless the model is redesigned.

## Key Files

| Path | Purpose |
|------|---------|
| `nginx-proxy/nginx.conf` | Top-level config: log format (URI without query string), `client_max_body_size 500M`, `$connection_upgrade` map, TLS defaults (TLS 1.2/1.3), `include conf.d/*.conf` |
| `nginx-proxy/snippets/acme.conf` | Shared ACME challenge `location ^~ /.well-known/acme-challenge/`, included by every `listen 80` block |
| `nginx-proxy/conf.d/default.conf` | Catch-all default server (see below). Tracked; never generated |
| `nginx-proxy/conf.d/service-<serviceId>.conf` | **Generated** per-service vhost; never hand-edit |
| `nginx-proxy/conf.d/<domain>.conf` | **Generated** panel-domain vhost |
| `nginx-proxy/certbot/www` | ACME webroot (certbot RW, nginx-proxy RO) |
| `nginx-proxy/certbot/conf` | `/etc/letsencrypt` (RO in proxy, RW in backend and certbot) |
| `server/internal/nginx/templates.go` | Server-block templates: `BuildHTTPHTTPSServers`, `BuildServiceProxyLocation`, `BuildPanelProxyLocation`, `BuildPanelConf` |
| `server/internal/nginx/service.go` | `UpdateServiceDomain`, `CleanupServiceDomain`, `SyncConfigs` |
| `server/internal/nginx/panel.go` | Panel-domain confs, hostname ownership (`AssertHostnamesAvailable`) |
| `server/internal/nginx/reload.go` | `docker exec ... nginx -t` / `-s reload`, atomic apply with rollback, self-heal |
| `server/internal/nginx/migrate.go` | Boot migration of old panel confs |
| `server/internal/nginx/stack.go` | `Stack` wiring (DNS checker, certs, nginx), `Boot`, `StartWatchers` |
| `server/internal/certs/` | Issuance, lineage selection, status, renew watcher, activity log |
| `server/internal/dnscheck/` | DNS preflight and public-IP discovery |
| `server/internal/api/deployments/domains.go` | Service domain endpoints |
| `server/internal/api/domains/domains.go` | Panel domain endpoints (`/api/domains`) |
| `server/internal/deploy/compose.go` | Runtime compose: project network, labels, opt-in ports |
| `server/internal/dockerx/network.go` | `ConnectProxy` / `DisconnectProxy` / `TeardownNetwork` / `EnsureNetwork` |

Image pins: `nginx:1.30.8-alpine` and `certbot/certbot:v5.10.0` in `docker-compose.yml`.

## Catch-all and unknown hosts

`nginx-proxy/conf.d/default.conf` has a `default_server` on :80 that serves only the ACME challenge path and answers every other request with **444** (connection closed), and a `default_server ssl` on :443 with `ssl_reject_handshake on`. Unknown hostnames therefore never reach the dashboard and never get a shared certificate. Public dashboard access requires an explicit panel domain (Settings → Domain).

## How Routing Works (service domains)

1. **Domain assigned** to a service (a project can have several services, each with its own domain) through `PUT /api/deployments/:projectId/services/:serviceId` (`updateService`). The `domain` field accepts a comma-separated list; each entry must match the hostname grammar (`certs.IsValidHostname`: dot-separated labels, at least two labels). The **first** entry is the UI/meta primary. The Let's Encrypt `live/<name>/` directory is resolved by `FindCoveringCertName` (SAN coverage), not assumed to equal the first hostname, so reordering must not drop HTTPS.
2. **Config generated** at `nginx-proxy/conf.d/service-<serviceId>.conf` by `UpdateServiceDomain`. A config exists only while the service has a domain, a container name, and status `running` or `starting`; otherwise an existing file is removed (and nginx reloaded).
3. **HTTP/ACME first, then HTTPS.** The first write is an HTTP vhost with the ACME snippet, applied and reloaded so the challenge path is live. Then `IssueCertificate` runs (unless `SkipIssue`) and the vhost is rewritten. Marker comments record the state: `# docklift-ssl: pending|http-only`, `partial` (a certificate covers only some hostnames; `:80` keeps proxying so HSTS hosts are not downgraded, `:443` serves only the hostnames on the certificate) and `active` (`:80` serves ACME and redirects with 301, `:443` serves every hostname).
4. **Apply and reload**: `Reloader.ApplyConf` writes the file atomically, runs `docker exec docklift-nginx-proxy nginx -t`, and on a rejected config restores the previous content (or deletes the new file) and returns `*ReloadError{RolledBack: true}`. `Reload` runs `nginx -s reload`; on failure it removes a service conf nginx rejects with "host not found in upstream" (self-heal) and tries once more, then returns the error (message prefix `Nginx reload failed:`). Callers must not treat a failed reload as success; on an nginx write or reload error the domain `PUT` puts the previous `domain` value back in the database and returns the error.
5. **Duplicate hostnames**: `AssertHostnamesAvailable` rejects (409) a hostname already assigned to another service or used by a panel conf.
6. **Reconciliation at boot** (`SyncConfigs`, startup hook `nginx-sync`): restores missing confs of running services with domains (reusing existing certificates, issuing only when none covers the hosts) and deletes `service-*.conf` files whose service no longer exists or has no domain, then reloads, so stale vhosts do not keep hijacking a hostname.
7. **During a deploy**, `activateServiceDomains` writes the vhosts after the status is saved; the write is cooperative (`ShouldContinue`) so a cancel aborts it.

## Panel domains (dashboard behind a domain)

`POST /api/domains {domain, port}` adds a panel domain: `ProvisionPanelSSL` writes the HTTP vhost, reloads, issues a certificate for exactly that hostname (a `www` mapping is a separate entry), then switches to HTTPS once a certificate covers it. Other routes: `GET /api/domains`, `POST /api/domains/dns-check` (up to 10 hosts), `GET /api/domains/:domain/ssl`, `POST /api/domains/:domain/ssl/retry`, `DELETE /api/domains/:domain`, `GET|PUT /api/domains/ssl/email`. Files are `nginx-proxy/conf.d/<domain>.conf`, starting with `# docklift-panel-domain` and `# docklift-panel-port: <port>`.

-   **Port 8080 means the dashboard.** The conf proxies to the Go backend, resolved through Docker DNS by its network-qualified name:

    ```nginx
    location / {
        set $dashboard docklift-backend.docklift_network;
        proxy_pass http://$dashboard:8000;      # 8000 = the backend's PORT
        proxy_http_version 1.1;
        proxy_set_header Host $http_host;
        proxy_set_header X-Forwarded-Host $http_host;   # always from $http_host
        proxy_set_header X-Forwarded-Proto $scheme;
        proxy_read_timeout 86400s;  proxy_send_timeout 86400s;  proxy_buffering off;   # WebSocket terminal and streams
        # plus Upgrade / Connection / X-Real-IP / X-Forwarded-For as in service vhosts
    }
    ```

    The name is `PanelUpstreamHost = docklift-backend.docklift_network`. It is qualified on purpose: the proxy also sits on every project network, where service keys are DNS aliases, and Docker's embedded DNS resolves `<name>.<network>` by looking up `<name>` only among that network's endpoints. Combined with the `docklift*` service-name ban, a project cannot hijack the panel upstream. The `docklift-nginx:80` upstream is gone.
-   **Other ports** proxy to `host.docker.internal:<port>` (`extra_hosts: host.docker.internal:host-gateway` on nginx-proxy), for a service published on the host.
-   **Boot migration** (`Stack.Boot` -> `MigratePanelUpstreams`, before `SyncConfigs`): panel confs written for the removed gateway (`set $dashboard docklift-nginx; proxy_pass http://$dashboard:80;`, or the unqualified `docklift-backend`) are rewritten to `docklift-backend.docklift_network:<PORT>`, `nginx -t` runs, and nginx reloads. It is idempotent (an up-to-date conf is left alone and nothing reloads), follows a changed `PORT`, restores every original if nginx rejects the result, and keeps the rewritten files if nginx cannot be run at all (they are what nginx-proxy loads when it next starts).
-   Golden tests: `server/internal/nginx/testdata/golden/*.conf` are renderings from the 2.x backend. `TestGoldenFiles` requires the Go templates to produce them byte for byte, except dashboard panel confs, which must equal `rewritePanelUpstream(golden)`. Do not change the templates without a reason existing installs can live with; if a golden must change, say why in the commit.

## SSL / Let's Encrypt

-   Issuance runs in `docklift-certbot` through the backend: `docker exec docklift-certbot certbot certonly --webroot -w /var/www/certbot --non-interactive --agree-tos --email <email> --cert-name <name> [--staging] [--expand] [--force-renewal] -d <host>...` (`certs.CertbotRequest.Args`, arguments as a list, 10 minute cap). The certbot container also runs `certbot renew --webroot -w /var/www/certbot` every 12 hours.
-   The ACME account email is the `ssl_acme_email` setting (`PUT /api/domains/ssl/email`), falling back to `CERTBOT_EMAIL`. `CERTBOT_STAGING=true` (or `1`) switches to the staging CA for testing without burning rate limits.
-   Public IP discovery for DNS checks uses **HTTPS only** (`https://api.ipify.org?format=json`, 4 s timeout, cached 5 min, response must parse as an IP address; redirects off HTTPS are refused).
-   `StartCertRenewWatcher` (startup hook `cert-renew-watcher`) reloads nginx-proxy after certbot renews something. It scans the newest `live/*/fullchain.pem` mtime every 5 minutes (and once at startup), persists the last-seen mtime in `DATA_PATH/ssl-cert-mtime`, retries a failed reload after 15 s, and so also reloads for a renewal that happened while the backend was down.
-   Certificates live at `/etc/letsencrypt/live/<certName>/{fullchain,privkey}.pem` (`certs.NginxCertPaths`). `ResolveCertName`, `FindCoveringCertName` and `SelectCertLineage` pick the healthiest covering lineage (covering the primary hostname and not expired beat a same-named orphan folder). Vhosts use that path so HTTPS does not drop when the first hostname changes.
-   Certbot flags (`planOrder`): `--expand` only for strict SAN supersets; shrink or replace uses `--force-renewal` without `--expand` (avoids `live/*-0001` forks). Success requires the PEMs to exist **and** cover every requested hostname.
-   Status values: `missing`, `pending`, `active`, `expiring` (under 21 days left), `expired`, `failed`; the persisted record is the `ssl_meta_<primary>` setting, the PEMs remain the source of truth. A certificate with under 20 days left is reported as overdue for renewal.

### `www` is never added implicitly

Only hostnames the user explicitly entered are put in the vhost and in the certificate SANs. Auto-adding `www.` was a real bug: if the `www` DNS record does not exist, the ACME order for the whole certificate fails, so the apex domain gets **no** certificate either. Users who want `www` add it as another comma-separated domain, and must create the DNS record first.

### DNS preflight

`IssueCertificate` checks every hostname on the order first (`dnscheck.CheckDomain`: A and AAAA lookups, 5 s each). Statuses: `ok` (an A record equals this server's public IP), `mismatch` (resolves elsewhere; expected behind Cloudflare's proxy), `missing` (the resolver answered NXDOMAIN / no A and no AAAA), `unknown` (SERVFAIL, timeout, or the server IP is unknown). When a **requested** hostname is `missing`, issuance is **skipped** with an actionable error instead of calling certbot: ACME would fail the whole order anyway, and failed orders count against the account rate limit. Lookup failures that are our fault (`unknown`) never block. Extra names still on an existing lineage are dropped from the order only when their DNS is definitively `missing`.

Endpoints: `POST /api/deployments/:projectId/services/:serviceId/dns-check`, `POST /api/domains/dns-check`.

### Activity log

`certs.EventLog` keeps the last 40 events per hostname in memory (DNS results, certbot start, success or failure, nginx reloads). `GET /api/deployments/:projectId/services/:serviceId/ssl` returns `{ ssl, events }` and the project Domains tab polls it every 2 s while work is in flight so users watch issuance happen. Events are narration only; PEMs remain the source of truth.

### Error reporting

`certs.SummarizeCertbotError` reduces certbot's very verbose output to one actionable line (preferring the CA's `Domain:` / `Detail:` block over generic footers, capped at 700 characters). `frontend/src/components/domains/ServiceDomainCard.tsx` shows that summary with numbered remediation steps, plus a command for the raw logs:

```bash
docker exec docklift-certbot tail -n 100 /var/log/letsencrypt/letsencrypt.log
docker logs docklift-certbot --tail 200
```

### Cloudflare

Cloudflare's SSL mode must be **Full (strict)** once a real certificate is issued. With the orange cloud enabled, Cloudflare must still be able to reach `/.well-known/acme-challenge/` over plain HTTP for issuance to work.

## Config Template

Generated service vhosts look like this (an `active` example; `BuildHTTPHTTPSServers`):

```nginx
# Main server block for <service name>
# docklift-ssl: active primary=app.example.com cert=app.example.com
server {
    listen 80;
    server_name app.example.com;

    include /etc/nginx/snippets/acme.conf;

    location / {
        return 301 https://$host$request_uri;
    }
}

server {
    listen 443 ssl;
    listen [::]:443 ssl;
    http2 on;
    server_name app.example.com;

    ssl_certificate /etc/letsencrypt/live/app.example.com/fullchain.pem;
    ssl_certificate_key /etc/letsencrypt/live/app.example.com/privkey.pem;

    resolver 127.0.0.11 valid=30s ipv6=off;

    location / {
        set $target_<serviceId> dl_myapp_53b01966_app;
        proxy_pass http://$target_<serviceId>:3000;
        # headers as in "Upstream routing"
    }
}
```

Server names, certificate name and comment text are validated or sanitized before they reach the file (`checkNames`, `IsValidCertName`, `sanitizeComment`), so a line break in a service name cannot inject directives.

### Two security rules for proxy headers

1. **Panel confs always set `X-Forwarded-Host` from `$http_host`.** A client-supplied `X-Forwarded-Host` must not reach the backend, which rebuilds the request origin from it for the same-origin check. The backend additionally honours forwarded headers only from trusted proxy peers (see "Control-plane network").
2. **Unknown hostnames must not fall through to the dashboard.** The default server in `conf.d/default.conf` rejects hostnames that match no vhost, so the admin UI is never reachable through an unconfigured domain.

## Troubleshooting

- **502 Bad Gateway**: container not running, app not listening on `internal_port`, proxy not attached to the project network (`docker network inspect dl-net-<id8>`), or wrong `container_name`. After recreating `docklift-nginx-proxy`, restart the backend or redeploy so the proxy is re-attached (the backend also does it at boot).
- **Panel domain 502 after upgrading from 2.x**: the conf still points at `docklift-nginx`; the boot migration rewrites it to `docklift-backend.docklift_network:8000` and reloads. Check `docker logs docklift-backend` for "Panel domains now proxy to".
- **Empty reply / connection closed on an unknown host**: expected (444 / rejected TLS handshake from `default.conf`); no vhost matches that `server_name`, or DNS does not point at the server.
- **App only reachable on `:5500`**: host ports are opt-in; enable **Publish host ports** on the project and redeploy, or use a domain.
- **`DNS_PROBE_FINISHED_NXDOMAIN`**: DNS record missing, or a stale local resolver cache (`ipconfig /flushdns`). Not a Docklift issue.
- **Certificate order fails on a multi-domain request**: one SAN's DNS is missing; ACME fails the whole order. Remove that hostname or create its record.
- **Config errors**: `docker exec docklift-nginx-proxy nginx -t`
- **Vhost for a deleted project still answering**: restart the backend (`SyncConfigs` removes orphaned `service-*.conf`), or check the conf directory for a leftover file.
- **Dashboard unreachable on `:8080`**: check `DASHBOARD_BIND`, the firewall, and `curl http://127.0.0.1:8080/api/health` on the server.
