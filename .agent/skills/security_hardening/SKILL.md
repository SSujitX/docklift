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
-   **Middleware**: All protected routes use `authMiddleware` from `lib/authMiddleware.ts` — never manually decode JWTs in route handlers.
-   **Storage**: Frontend stores in `localStorage` key `docklift_token`.
-   **Download Safety**: Backup downloads use `fetch` + `Authorization: Bearer` header + blob pattern — **never** put JWTs in URL query parameters.

### SSE Tokens (Short-lived)
-   SSE connections use dedicated 5-minute tokens (`purpose: 'sse'`).
-   Generated via `POST /api/auth/sse-token`.
-   Passed as query param `?token=<sseToken>` (not the long-lived session JWT).
-   Backend validates `purpose === 'sse'` before allowing SSE connections.

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

Applied in: `deployments.ts` (container migration).

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
-   **Verification order**: Signature is verified FIRST — before any database queries or processing. This prevents unauthenticated requests from triggering DB lookups.
-   Raw body (`req.rawBody`) is captured via `express.json({ verify })` callback for accurate HMAC comparison.
-   Debounced via `recentDeploys` Map with 10-second cooldown per project.

## Git Token Security

### Just-in-Time Token Pattern
GitHub installation tokens are set just-in-time and immediately scrubbed after use:

```typescript
let gitTokenSet = false;
try {
  // Set token in git remote URL
  await gitInstance.remote(['set-url', 'origin', authenticatedUrl]);
  gitTokenSet = true;
  // Pull code
  await pullRepo(projectPath, ...);
} finally {
  // SECURITY: Always scrub token from remote URL
  if (gitTokenSet && gitInstance) {
    await gitInstance.remote(['set-url', 'origin', cleanUrl]);
  }
}
```

Applied in: `deployments.ts` (deploy handler).

## Terminal Security

### WebSocket Authentication
-   **JWT**: Required to establish WebSocket connection (query param `?token=`).
-   **Password Re-verification**: After WS connect, user must enter account password.
-   **Rate Limiting**: Max 5 logins/minute.
-   **Session Limits**: Max 3 concurrent connections per user.
-   **Idle Timeout**: Auto-disconnect after 15 minutes of inactivity.

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
-   Applied globally via `helmet()` middleware in `index.ts`.

### CORS
-   Configured from `CORS_ORIGIN` environment variable.

### Setup Token (Backup Restore)
-   One-time token stored in `.setup-token` file.
-   Consumed (deleted) after single use.
-   Only used for unauthenticated `/restore-upload` on fresh installs.
-   Frontend (`setup/page.tsx`) fetches token via `GET /api/auth/setup-token` and sends as `x-setup-token` header.

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
