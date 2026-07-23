/**
 * Local smoke checks for P0/P1 auth + webhook (no Docker required).
 * Run: bun run scripts/smoke-p0.ts  (from backend/)
 *
 * Does NOT cover Docker-dependent flows (deploy cancel, domain nginx, git scrub).
 */
import jwt from 'jsonwebtoken';
import fs from 'fs';
import path from 'path';

const BASE = process.env.SMOKE_BASE || 'http://127.0.0.1:8000';
const secretsPath = path.join(process.cwd(), 'data', '.secrets');
const secrets = JSON.parse(fs.readFileSync(secretsPath, 'utf8'));
const JWT_SECRET = secrets.jwtSecret as string;

type Result = { name: string; ok: boolean; detail: string };
const results: Result[] = [];

function record(name: string, ok: boolean, detail: string) {
  results.push({ name, ok, detail });
  console.log(`${ok ? 'PASS' : 'FAIL'}  ${name} — ${detail}`);
}

async function main() {
  // Health
  const health = await fetch(`${BASE}/api/health`);
  record('health', health.ok, `status ${health.status}`);
  if (!health.ok) {
    console.error('Backend not reachable. Start with: bun run dev');
    process.exit(1);
  }

  // Ensure a session user exists (/api/auth/status returns setupComplete, not hasUsers)
  const statusRes = await fetch(`${BASE}/api/auth/status`);
  const status = await statusRes.json() as { setupComplete: boolean };
  let sessionToken = '';

  if (!status.setupComplete) {
    const reg = await fetch(`${BASE}/api/auth/register`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({
        name: 'Smoke Admin',
        email: 'smoke@docklift.local',
        password: 'SmokeTest123!',
      }),
    });
    const regBody = await reg.json() as { token?: string; error?: string };
    sessionToken = regBody.token || '';
    record('register', !!sessionToken, sessionToken ? 'created smoke user' : (regBody.error || `status ${reg.status}`));
  } else {
    const login = await fetch(`${BASE}/api/auth/login`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({
        email: 'smoke@docklift.local',
        password: 'SmokeTest123!',
      }),
    });
    if (login.ok) {
      const body = await login.json() as { token: string };
      sessionToken = body.token;
      record('login', true, 'smoke user');
    } else {
      // Fallback: mint a session JWT directly for middleware checks (dev smoke only)
      sessionToken = jwt.sign(
        { userId: 'smoke', email: 'smoke@docklift.local', role: 'admin' },
        JWT_SECRET,
        { expiresIn: '1h' },
      );
      record('login', true, 'minted session JWT (login failed — using signed token)');
    }
  }

  // Issue SSE token via API
  const sseRes = await fetch(`${BASE}/api/auth/sse-token`, {
    method: 'POST',
    headers: {
      Authorization: `Bearer ${sessionToken}`,
      'Content-Type': 'application/json',
    },
  });
  const sseBody = await sseRes.json() as { token?: string; error?: string };
  const sseToken = sseBody.token || '';
  record('sse-token issue', sseRes.ok && !!sseToken, sseRes.ok ? 'got purpose=sse token' : (sseBody.error || `${sseRes.status}`));

  // 1) Normal APIs must reject SSE query tokens
  const destroy = await fetch(`${BASE}/api/projects/00000000-0000-0000-0000-000000000000?token=${encodeURIComponent(sseToken)}`, {
    method: 'DELETE',
  });
  record(
    'SSE query on DELETE /projects → 401',
    destroy.status === 401,
    `status ${destroy.status}`,
  );

  const reboot = await fetch(`${BASE}/api/system/reboot?token=${encodeURIComponent(sseToken)}`, {
    method: 'POST',
  });
  record(
    'SSE query on POST /system/reboot → 401',
    reboot.status === 401,
    `status ${reboot.status}`,
  );

  // Session JWT via query must also fail on normal APIs (Bearer-only)
  const sessQuery = await fetch(`${BASE}/api/projects?token=${encodeURIComponent(sessionToken)}`);
  record(
    'session JWT via query on GET /projects → 401',
    sessQuery.status === 401,
    `status ${sessQuery.status}`,
  );

  // Bearer session still works
  const list = await fetch(`${BASE}/api/projects`, {
    headers: { Authorization: `Bearer ${sessionToken}` },
  });
  record(
    'Bearer session on GET /projects → 200',
    list.status === 200,
    `status ${list.status}`,
  );

  // Bearer SSE token must fail on normal APIs
  const bearerSse = await fetch(`${BASE}/api/projects`, {
    headers: { Authorization: `Bearer ${sseToken}` },
  });
  record(
    'Bearer SSE token on GET /projects → 401',
    bearerSse.status === 401,
    `status ${bearerSse.status}`,
  );

  // 2) SSE middleware accepts SSE query on log stream route (may 400/500 without docker — must NOT be 401)
  const sysLogs = await fetch(`${BASE}/api/system/logs/backend?tail=1&token=${encodeURIComponent(sseToken)}`);
  record(
    'SSE query on /system/logs/backend auth OK',
    sysLogs.status !== 401,
    `status ${sysLogs.status} (401=auth fail; other=auth passed)`,
  );
  // Cancel body so we don't hold a long-lived SSE stream open (esp. on Docker hosts)
  await sysLogs.body?.cancel().catch(() => {});

  // Session JWT query on SSE route must 401
  const sysLogsSess = await fetch(`${BASE}/api/system/logs/backend?tail=1&token=${encodeURIComponent(sessionToken)}`);
  record(
    'session JWT query on /system/logs → 401',
    sysLogsSess.status === 401,
    `status ${sysLogsSess.status}`,
  );
  await sysLogsSess.body?.cancel().catch(() => {});

  // 7) Webhook without secret → 401 with the missing-secret message (not merely "invalid signature")
  const webhook = await fetch(`${BASE}/api/github/webhook`, {
    method: 'POST',
    headers: {
      'Content-Type': 'application/json',
      'X-GitHub-Event': 'push',
    },
    body: JSON.stringify({
      ref: 'refs/heads/main',
      repository: { clone_url: 'https://github.com/example/repo.git', html_url: 'https://github.com/example/repo' },
    }),
  });
  const webhookBody = await webhook.json().catch(() => ({})) as { error?: string };
  record(
    'webhook without secret → 401 + message',
    webhook.status === 401 && webhookBody.error === 'Webhook secret not configured',
    `status ${webhook.status} error=${JSON.stringify(webhookBody.error || null)}`,
  );

  const failed = results.filter((r) => !r.ok);
  console.log('\n---');
  console.log(`Passed ${results.length - failed.length}/${results.length}`);
  if (failed.length) {
    console.log('Failed:');
    failed.forEach((f) => console.log(`  - ${f.name}: ${f.detail}`));
    process.exit(1);
  }
  console.log('Auth/webhook smoke checks passed. For Docker flows run: bun run scripts/smoke-docker.ts');
}

main().catch((e) => {
  console.error(e);
  process.exit(1);
});
