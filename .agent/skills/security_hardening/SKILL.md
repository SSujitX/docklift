---
name: Security Hardening
description: Security patterns, guards, and best practices enforced across the Docklift codebase.
---

# Security Hardening Guide

This skill documents all security patterns implemented in Docklift. Follow these conventions when adding new features to maintain the security posture.

## Authentication & Authorization

### JWT Tokens
-   **Signing**: `jsonwebtoken` with `JWT_SECRET` from environment (auto-generated on first run if empty).
-   **Expiry**: 7 days for session tokens.
-   **`pwdv` claim**: must equal `User.passwordChangedAt.getTime()`; middleware rejects mismatch or missing claim.
-   **Middleware**: All protected routes use `authMiddleware` from `lib/authMiddleware.ts` — never manually decode JWTs in route handlers.
-   **Storage**: Frontend stores in `localStorage` key `docklift_token`.
-   **Frontend**: use `authFetch()` so 401 clears session app-wide.
-   **Download Safety**: Backup downloads use `authFetch` + blob pattern — **never** put JWTs in URL query parameters.
-   **Bootstrap registration**: atomic rename claim of `.bootstrap-secret` (see `authentication` skill).

### SSE Tokens (Short-lived)
-   SSE connections use dedicated 5-minute tokens (`purpose: 'sse'`).
-   Generated via `POST /api/auth/sse-token` (Bearer session JWT required).
-   Passed as query param `?token=<sseToken>` (not the long-lived session JWT).
-   **`authMiddleware`**: Bearer session JWT only — query tokens are ignored/rejected.
-   **`sseAuthMiddleware`**: query SSE token only — mounted exclusively on log stream routes (`/api/system/logs/:service`, `/api/logs/.../stream/...`).
-   An SSE token in `?token=` cannot authorize DELETE/reboot or other protected APIs.

### Rate Limiting
-   All `/api/auth` routes are rate-limited via `express-rate-limit`.
-   Applied at the mount level in `index.ts`: `app.use('/api/auth', authLimiter, authRouter)`.

### Password Hashing
-   Uses `bcrypt` with **12 salt rounds**.
-   Never store or log plaintext passwords.

## Route Protection

### Public vs Protected Routes (in `index.ts`)
| Route | Access |
|-------|--------|
| `/api/auth/register`, `/login`, `/status` | Public (rate limited) |
| `/api/github/webhook`, `/callback`, `/manifest/callback`, `/setup` | Public (GitHub flow) |
| `/api/backup/restore-upload` with valid setup token | Public (one-time restore) |
| All other `/api/*` routes | Requires JWT via `authMiddleware` |

### Internal API Secret
-   `X-Internal-Secret` header used for backend-to-backend calls (e.g., webhook → deploy).
-   Stored in `INTERNAL_API_SECRET` env var.

## Origin Validation (CORS & WebSocket)

`lib/originCheck.ts` is the single implementation, shared by the CORS middleware in `index.ts` and
the terminal WebSocket upgrade in `services/terminal.ts`.

**RULE**: An origin is trusted only when it matches **scheme + host + port** of the request the
browser actually made, or when it appears *exactly* in the operator allowlist (`CORS_ORIGIN`,
`DOCKLIFT_FRONTEND_URL`).

```typescript
const allowed = isTrustedOrigin(origin, req.headers, {
  fallbackProto: req.protocol,
  allow: [config.frontendUrl],
});
```

-   `requestOrigin()` reconstructs the browser-facing origin from the forwarded `Host`/proto headers.
-   Scheme mismatches are tolerated **only on default ports**, to survive TLS termination at the proxy.
-   Requests with no `Origin` header (non-browser clients) are allowed — the JWT is still the gate.

### Why hostname-only matching is a real vulnerability

A previous refactor compared **hostnames only**. On a Docklift server that is exactly wrong: user apps
are deployed on the *same host* under other ports and subdomains. Any co-located app could then make
credentialed cross-origin requests to the dashboard API. Port and scheme are part of the origin —
keep them in the comparison.

### Nginx must not pass through a client `X-Forwarded-Host`

Because `requestOrigin()` trusts the forwarded host, every proxy config sets it from the connection
itself:

```nginx
proxy_set_header X-Forwarded-Host $http_host;
```

Without this line a client can send its own `X-Forwarded-Host` and forge the origin the backend
believes it is serving, defeating the same-origin check. Applies to `nginx.conf` and every template
in `services/nginxSsl.ts`.

## Command Execution Security

### spawnSync over execSync
**RULE**: Never use `execSync()` with string concatenation. Always use `spawnSync()` with argument arrays to prevent command injection.

```typescript
// ✅ CORRECT — argument array, no shell injection possible
import { spawnSync } from 'child_process';
spawnSync('docker', ['rm', '-f', containerName], { stdio: 'ignore' });

// ❌ WRONG — string interpolation allows injection
execSync(`docker rm -f ${containerName}`);
```

Applied in: `deployments.ts` (container teardown/migration) and `projects.ts` (volume lifecycle).

### Build Argument Allowlisting
`validateDockerBuildArgs()` (`services/compose.ts`) passes only the build args a Dockerfile actually
declares with `ARG`. Forwarding every project env var into `docker build` would bake runtime secrets
into image layers, where they stay readable via `docker history`.

`buildServiceImage()` additionally **rejects** protected host variables (e.g. `PATH`, `HOME`,
`DOCKER_HOST`) as build variables, so a project setting cannot repoint the builder's toolchain or
Docker endpoint. Covered by `buildResolver.test.ts`.

### Path Resolution for User-Supplied Directories
`resolveProjectPath()` (`services/buildResolver.ts`) resolves `base_directory` / `dockerfile_path`
and rejects anything that escapes the deployment root, so a crafted project setting cannot make the
builder read from elsewhere on the host.

## Error Handling

### Error Message Sanitization
**RULE**: Never expose `error.message` in API responses. Always return generic messages.

```typescript
// ✅ CORRECT
catch (error: any) {
  console.error('Login error:', error);
  res.status(500).json({ error: 'Login failed' });
}

// ❌ WRONG — leaks internal details
catch (error: any) {
  res.status(500).json({ error: error.message });
}
```

Currently enforced in: `auth.ts`. Remaining: `system.ts` `/version` endpoint (low risk).

## Streaming Safety

### Disconnection Guard Pattern
All streaming endpoints must use the `writeLog` guard to prevent crashes on client disconnect:

```typescript
const writeLog = (text: string) => {
  try { if (!res.writableEnded) res.write(text); } catch {}
  logs.push(text);
};
```

Applied in: all 4 streaming handlers in `deployments.ts` (deploy, stop, restart, redeploy).

Also in `docker.ts` `streamContainerLogs`: uses `safeWrite()` + `closed` flag + `res.on('close')` cleanup.

## Webhook Security

### GitHub Webhook Signature Verification
-   Uses `crypto.timingSafeEqual` (prevents timing attacks).
-   Signature verified against `github_webhook_secret` stored in DB.
-   **Fail closed**: If the webhook secret is not configured, the endpoint returns 401 (unsigned webhooks are never accepted).
-   **Verification order**: Signature is verified FIRST — before any database queries or processing. This prevents unauthenticated requests from triggering DB lookups.
-   Raw body (`req.rawBody`) is captured via `express.json({ verify })` callback for accurate HMAC comparison.
-   Debounced via `recentDeploys` Map with 10-second cooldown per project.

## Git Token Security

### Just-in-Time Token Pattern
GitHub installation tokens are set just-in-time and scrubbed after use (deploy pull **and** initial clone via `scrubOriginRemote`). After scrub, origin is verified to contain no credentials. **If scrub/verify fails, fail the deploy or roll back project create** — never continue with a token left in `.git/config`.

Applied in: `deployments.ts` (deploy handler), `projects.ts` (clone).

## Terminal Security

### WebSocket Authentication
-   **JWT**: Required to establish WebSocket connection (query param `?token=`).
-   **Password Re-verification**: After WS connect, user must enter account password.
-   **Rate Limiting**: Max 5 logins/minute.
-   **Session Limits**: Max 3 concurrent connections per user.
-   **Idle Timeout**: Auto-disconnect after 30 minutes of inactivity.

### Resize Input Validation
Terminal resize messages are validated to prevent injection:

```typescript
if (!Number.isInteger(cols) || !Number.isInteger(rows) ||
    cols < 1 || cols > 500 || rows < 1 || rows > 200) {
  return; // silently ignore invalid resize
}
```

Applied in: `terminal.ts`.

## Path Security

### Path Traversal Prevention (`files.ts`)
```typescript
const resolved = path.resolve(projectDir, relativePath);
if (!resolved.startsWith(projectDir)) {
  return res.status(403).json({ error: 'Access denied: path traversal detected' });
}
```

### Symlink Protection
```typescript
const realPath = fs.realpathSync(resolved);
if (!realPath.startsWith(projectDir)) {
  return res.status(403).json({ error: 'Access denied: symlink escape' });
}
```

### Project ID Validation
```typescript
const projectIdRegex = /^[a-f0-9-]{36}$/;
```

## File Upload Safety

### Safe ZIP Extraction
Project uploads and backup archives are extracted through `lib/safeUnzip.ts`, which rejects absolute
paths and `..` entries. A raw `unzipper` extract would happily write outside the destination
directory (zip-slip).

### Multer Configuration
-   Upload destination uses **absolute path**: `path.join(config.dataPath, 'uploads')`.
-   Temp files are **always cleaned up** via `try/finally`:
```typescript
try {
  // extract zip...
} finally {
  try { fs.unlinkSync(req.file.path); } catch {}
}
```

## Infrastructure Security

### Security Headers
-   Applied in `index.ts` via custom middleware: `X-Content-Type-Options`, `X-Frame-Options`,
    `X-XSS-Protection`, `Referrer-Policy`, `Permissions-Policy`, and `Strict-Transport-Security`
    (only when the request itself arrived over HTTPS). Helmet is not used.

### Trusted Proxy Depth
-   `app.set('trust proxy', 1)` — exactly one proxy (nginx). A larger value would let clients spoof
    `X-Forwarded-For` and evade rate limiting.

### Public Proxy Hostname Isolation
-   Hostnames with no generated vhost are rejected at the edge rather than falling through to the
    dashboard, so the admin UI is never served on an unconfigured domain.

### Access Log Hygiene
-   `nginx-proxy/nginx.conf` logs `$uri` rather than the full request line, keeping SSE `?token=`
    query parameters out of access logs.

### CORS
-   Strict same-origin via `isTrustedOrigin()`, plus exact `CORS_ORIGIN` entries — see
    [Origin Validation](#origin-validation-cors--websocket) above.

### Setup Token (Backup Restore)
-   One-time token stored under `config.dataPath` (`.setup-token`) — never a hardcoded `./data` path.
-   Consumed (deleted) after single use.
-   Only used for unauthenticated `/restore-upload` on fresh installs.
-   Frontend Setup page fetches token via `GET /api/auth/setup-token` (bootstrap secret required) and sends `x-setup-token`.

### Graceful Shutdown
Backend handles SIGTERM/SIGINT for clean exit:
```typescript
const shutdown = async (signal: string) => {
  server.close();           // Stop accepting new connections
  cleanupAllSessions();     // Kill all terminal PTY sessions
  await prisma.$disconnect(); // Close database connection
  process.exit(0);
};
process.on('SIGTERM', () => shutdown('SIGTERM'));
process.on('SIGINT', () => shutdown('SIGINT'));
```

Applied in: `index.ts`.

## Checklist for New Features

When adding new endpoints or features, verify:
- [ ] Route is protected by `authMiddleware` (or has explicit reason to be public)
- [ ] Error responses use generic messages, not `error.message`
- [ ] Streaming endpoints use disconnection guards
- [ ] File paths are validated against traversal and symlink escapes
- [ ] Temp files are cleaned up in `finally` blocks
- [ ] Destructive operations include audit logging (`console.log(\`[AUDIT]...\`)`)
- [ ] Shell commands use `spawnSync()` with argument arrays, never `execSync()` with strings
- [ ] Sensitive tokens (JWT, Git) are never placed in URLs — use Authorization headers
- [ ] Terminal/WebSocket inputs are validated (type, bounds) before processing
- [ ] Cross-origin checks go through `isTrustedOrigin()` — never compare hostnames alone
- [ ] User-supplied paths go through `resolveProjectPath()` / `pathSecurity.ts`
- [ ] Archives are extracted via `safeUnzip.ts`, never raw `unzipper`
- [ ] New build inputs are allowlisted, not forwarded wholesale into `docker build`
- [ ] New nginx templates set `X-Forwarded-Host` from `$http_host`
