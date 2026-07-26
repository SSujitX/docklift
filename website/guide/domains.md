# Custom Domains & HTTPS

Connect custom domains to the Docklift panel and to deployed services. Certificates come from Let's Encrypt via HTTP-01 (ports **80** and **443** must reach the server).

Domains are the preferred way to expose apps. You do **not** need a published host port for domain traffic — nginx-proxy routes on the project network to `container_name:internal_port`.

## Panel domain

Access Docklift itself via hostname instead of `http://SERVER_IP:8080`.

1. Point a DNS **A record** at your server IP
2. Go to **Settings → Domain**
3. Optionally set the ACME email, then add your domain (e.g. `panel.example.com`)
4. Wait until SSL shows **HTTPS Active** (use Retry if DNS was not ready)

Unknown hostnames on ports 80/443 are rejected — they never fall through to the dashboard.

## Service domains

Each Dockerfile/service can have multiple domains.

1. Go to **Project → Domains**
2. Add a domain for the service (e.g. `api.example.com`)
3. Save — nginx-proxy routes to the container and certbot issues HTTPS

Host ports remain optional under **Build Settings**.

## DNS examples

| Type | Name | Value | Usage |
|------|------|-------|-------|
| `A` | `@` | `YOUR_SERVER_IP` | Apex (`example.com`) |
| `A` | `panel` | `YOUR_SERVER_IP` | Docklift panel |
| `A` | `api` | `YOUR_SERVER_IP` | API service |
| `A` | `*` | `YOUR_SERVER_IP` | Wildcard subdomains |

## Certificate flow

1. Docklift writes an HTTP vhost
2. Certbot solves the ACME HTTP-01 challenge
3. The vhost is rewritten with HTTPS plus an HTTP→HTTPS redirect
4. Renewals run automatically (about every 12 hours)

**`www` is not added for you.** A certificate order fails entirely if one of its hostnames has no DNS record. Create the `www` DNS record first, then add `www.example.com` as an additional domain.

If issuance fails, check:

```bash
docker logs docklift-certbot --tail 200
```

## Cloudflare

- Prefer DNS-only (gray cloud) while the certificate is first issuing
- After SSL is **Active** in Docklift, set SSL/TLS mode to **Full (strict)**
- Do **not** use Flexible — that sends plain HTTP to your origin
- Ensure Cloudflare can reach `/.well-known/acme-challenge/` over HTTP during issuance

## Troubleshooting

| Symptom | Check |
|---------|--------|
| `502 Bad Gateway` | Container running? Listening on the configured internal port? |
| Nothing / `NXDOMAIN` | DNS missing or not propagated |
| Cert order fails | One hostname in the request lacks DNS — see certbot logs |
| Panel not on hostname | Use **Settings → Domain**; IP:8080 still works until then |
