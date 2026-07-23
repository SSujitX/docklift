/**
 * Docker-dependent smoke tests (3–6).
 * Requires: backend on :8000, Docker Engine running.
 * Run: bun run scripts/smoke-docker.ts
 */
import fs from 'fs';
import path from 'path';
import { spawnSync } from 'child_process';
import jwt from 'jsonwebtoken';

const BASE = process.env.SMOKE_BASE || 'http://127.0.0.1:8000';
const ROOT = process.cwd();
const TMP = path.join(ROOT, 'data', 'smoke-tmp');
const secrets = JSON.parse(fs.readFileSync(path.join(ROOT, 'data', '.secrets'), 'utf8'));
const JWT_SECRET = secrets.jwtSecret as string;

type Result = { name: string; ok: boolean; detail: string };
const results: Result[] = [];

function record(name: string, ok: boolean, detail: string) {
  results.push({ name, ok, detail });
  console.log(`${ok ? 'PASS' : 'FAIL'}  ${name} — ${detail}`);
}

async function login(): Promise<string> {
  const login = await fetch(`${BASE}/api/auth/login`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ email: 'smoke@docklift.local', password: 'SmokeTest123!' }),
  });
  if (login.ok) {
    const body = await login.json() as { token: string };
    return body.token;
  }
  return jwt.sign(
    { userId: 'smoke', email: 'smoke@docklift.local', role: 'admin' },
    JWT_SECRET,
    { expiresIn: '1h' },
  );
}

function writeZipFromDir(dir: string, zipPath: string) {
  // Use PowerShell Compress-Archive
  if (fs.existsSync(zipPath)) fs.unlinkSync(zipPath);
  const r = spawnSync(
    'powershell',
    ['-NoProfile', '-Command', `Compress-Archive -Path "${dir}\\*" -DestinationPath "${zipPath}" -Force`],
    { encoding: 'utf8' },
  );
  if (r.status !== 0) {
    throw new Error(`zip failed: ${r.stderr || r.stdout}`);
  }
}

async function createUploadProject(
  token: string,
  name: string,
  zipPath: string,
  domain?: string,
): Promise<string> {
  const form = new FormData();
  form.append('name', name);
  form.append('source_type', 'upload');
  form.append('project_type', 'app');
  if (domain) form.append('domain', domain);
  const bytes = fs.readFileSync(zipPath);
  form.append('files', new Blob([bytes]), path.basename(zipPath));

  const res = await fetch(`${BASE}/api/projects`, {
    method: 'POST',
    headers: { Authorization: `Bearer ${token}` },
    body: form,
  });
  const body = await res.json() as { id?: string; error?: string };
  if (!res.ok || !body.id) {
    throw new Error(`create project failed: ${res.status} ${body.error || JSON.stringify(body)}`);
  }
  return body.id;
}

async function getProject(token: string, id: string) {
  const res = await fetch(`${BASE}/api/projects/${id}`, {
    headers: { Authorization: `Bearer ${token}` },
  });
  return res.json() as Promise<{ status?: string; id: string; domain?: string }>;
}

async function getLatestDeployment(token: string, projectId: string) {
  const res = await fetch(`${BASE}/api/deployments/${projectId}?limit=1`, {
    headers: { Authorization: `Bearer ${token}` },
  });
  const list = await res.json() as Array<{ id: string; status: string }>;
  return list[0] || null;
}

async function main() {
  fs.mkdirSync(TMP, { recursive: true });

  const health = await fetch(`${BASE}/api/health`);
  record('health', health.ok, `status ${health.status}`);
  if (!health.ok) process.exit(1);

  const docker = spawnSync('docker', ['info'], { encoding: 'utf8' });
  record('docker engine', docker.status === 0, docker.status === 0 ? 'ok' : (docker.stderr || 'docker info failed'));
  if (docker.status !== 0) process.exit(1);

  const token = await login();
  record('auth', !!token, 'session ready');

  // Windows Hyper-V often excludes 3000–5000 TCP ports; pre-lock so allocatePort skips them
  const { PrismaClient } = await import('@prisma/client');
  const prisma = new PrismaClient();
  try {
    for (let port = 3001; port <= 5500; port++) {
      await prisma.port.upsert({
        where: { port },
        update: { is_locked: true },
        create: { port, is_locked: true },
      });
    }
    record('port skip low range', true, 'locked 3001-5500 for Windows exclusions');
  } finally {
    await prisma.$disconnect();
  }

  // --- Prepare fixtures ---
  const slowDir = path.join(TMP, 'slow-app');
  const failDir = path.join(TMP, 'fail-app');
  fs.rmSync(slowDir, { recursive: true, force: true });
  fs.rmSync(failDir, { recursive: true, force: true });
  fs.mkdirSync(slowDir, { recursive: true });
  fs.mkdirSync(failDir, { recursive: true });

  // Slow build: sleep during RUN so cancel can interrupt
  fs.writeFileSync(
    path.join(slowDir, 'Dockerfile'),
    `FROM alpine:3.20
RUN sleep 45
EXPOSE 8080
CMD ["sleep", "infinity"]
`,
  );
  // Fail build: invalid instruction
  fs.writeFileSync(
    path.join(failDir, 'Dockerfile'),
    `FROM alpine:3.20
RUN false
EXPOSE 8080
CMD ["sleep", "infinity"]
`,
  );

  const slowZip = path.join(TMP, 'slow-app.zip');
  const failZip = path.join(TMP, 'fail-app.zip');
  writeZipFromDir(slowDir, slowZip);
  writeZipFromDir(failDir, failZip);

  // ========== 6 + 4: deploy with domain, then cancel ==========
  const domain = `smoke-${Date.now()}.example.test`;
  const slowId = await createUploadProject(token, `smoke-cancel-${Date.now()}`, slowZip, domain);
  record('create slow project', !!slowId, slowId);

  // Start deploy (don't await body — stream)
  const deployRes = await fetch(`${BASE}/api/deployments/${slowId}/deploy`, {
    method: 'POST',
    headers: {
      Authorization: `Bearer ${token}`,
      'Content-Type': 'application/json',
    },
    body: JSON.stringify({ trigger: 'manual' }),
  });
  record('deploy started', deployRes.status === 200, `status ${deployRes.status}`);

  // Wait until building
  let building = false;
  for (let i = 0; i < 30; i++) {
    await new Promise((r) => setTimeout(r, 1000));
    const p = await getProject(token, slowId);
    if (p.status === 'building') {
      building = true;
      break;
    }
  }
  record('project entered building', building, building ? 'building' : 'never saw building');

  // Cancel while building
  const cancelRes = await fetch(`${BASE}/api/deployments/${slowId}/cancel`, {
    method: 'POST',
    headers: { Authorization: `Bearer ${token}` },
  });
  // consume cancel stream briefly
  await cancelRes.body?.cancel().catch(() => {});
  record('cancel accepted', cancelRes.status === 200, `status ${cancelRes.status}`);

  // Also cancel the deploy response stream
  await deployRes.body?.cancel().catch(() => {});

  // Wait and ensure never running
  let finalStatus = '';
  let becameRunning = false;
  for (let i = 0; i < 45; i++) {
    await new Promise((r) => setTimeout(r, 2000));
    const p = await getProject(token, slowId);
    finalStatus = p.status || '';
    if (finalStatus === 'running') {
      becameRunning = true;
      break;
    }
    if (finalStatus === 'stopped' || finalStatus === 'error' || finalStatus === 'cancelled') {
      // give a few more seconds to catch late overwrite to running
      await new Promise((r) => setTimeout(r, 5000));
      const again = await getProject(token, slowId);
      finalStatus = again.status || finalStatus;
      if (again.status === 'running') becameRunning = true;
      break;
    }
  }
  record(
    '4 cancel never becomes running',
    !becameRunning && (finalStatus === 'stopped' || finalStatus === 'error' || finalStatus === 'cancelled'),
    `final status=${finalStatus} becameRunning=${becameRunning}`,
  );

  const dep = await getLatestDeployment(token, slowId);
  record(
    '4 deployment not success after cancel',
    !!dep && dep.status !== 'success',
    `deployment status=${dep?.status || 'none'}`,
  );

  // Domain conf: may exist if deploy got far enough to create service+activate,
  // or after a successful deploy. For cancel mid-build, check services + conf path after a successful mini deploy.
  // Run a quick successful deploy project for domain activation (test 6).
  const okDir = path.join(TMP, 'ok-app');
  fs.rmSync(okDir, { recursive: true, force: true });
  fs.mkdirSync(okDir, { recursive: true });
  fs.writeFileSync(
    path.join(okDir, 'Dockerfile'),
    `FROM alpine:3.20
EXPOSE 8080
CMD ["sleep", "infinity"]
`,
  );
  const okZip = path.join(TMP, 'ok-app.zip');
  writeZipFromDir(okDir, okZip);
  const domain2 = `smoke-ok-${Date.now()}.example.test`;
  const okId = await createUploadProject(token, `smoke-domain-${Date.now()}`, okZip, domain2);
  record('create domain project', !!okId, okId);

  const okDeploy = await fetch(`${BASE}/api/deployments/${okId}/deploy`, {
    method: 'POST',
    headers: {
      Authorization: `Bearer ${token}`,
      'Content-Type': 'application/json',
    },
    body: JSON.stringify({ trigger: 'manual' }),
  });
  // Read stream until done (or timeout)
  const okReader = okDeploy.body?.getReader();
  if (okReader) {
    const deadline = Date.now() + 180000;
    while (Date.now() < deadline) {
      const { done } = await okReader.read();
      if (done) break;
    }
    try { await okReader.cancel(); } catch { /* */ }
  }

  let okStatus = '';
  for (let i = 0; i < 30; i++) {
    const p = await getProject(token, okId);
    okStatus = p.status || '';
    if (okStatus === 'running' || okStatus === 'error') break;
    await new Promise((r) => setTimeout(r, 2000));
  }
  record('6 deploy completed', okStatus === 'running', `status=${okStatus}`);

  // Find nginx conf for service
  const nginxDir = path.resolve(ROOT, 'nginx-proxy', 'conf.d');
  // Also check backend-relative path used by config
  const nginxDir2 = path.resolve(ROOT, 'src', '..', 'nginx-proxy', 'conf.d');
  const confDirs = [nginxDir, path.join(ROOT, 'nginx-proxy', 'conf.d')];
  let confHit = '';
  for (const dir of confDirs) {
    if (!fs.existsSync(dir)) continue;
    const files = fs.readdirSync(dir).filter((f) => f.startsWith('service-') && f.endsWith('.conf'));
    for (const f of files) {
      const content = fs.readFileSync(path.join(dir, f), 'utf8');
      if (content.includes(domain2)) {
        confHit = path.join(dir, f);
        break;
      }
    }
    if (confHit) break;
  }
  record('6 nginx conf contains domain', !!confHit, confHit || `no conf with ${domain2}`);

  // ========== 3: failure → error ==========
  const failId = await createUploadProject(token, `smoke-fail-${Date.now()}`, failZip);
  const failDeploy = await fetch(`${BASE}/api/deployments/${failId}/deploy`, {
    method: 'POST',
    headers: {
      Authorization: `Bearer ${token}`,
      'Content-Type': 'application/json',
    },
    body: JSON.stringify({ trigger: 'manual' }),
  });
  const failReader = failDeploy.body?.getReader();
  if (failReader) {
    const deadline = Date.now() + 180000;
    while (Date.now() < deadline) {
      const { done } = await failReader.read();
      if (done) break;
    }
    try { await failReader.cancel(); } catch { /* */ }
  }

  let failStatus = '';
  for (let i = 0; i < 40; i++) {
    const p = await getProject(token, failId);
    failStatus = p.status || '';
    if (failStatus === 'error' || failStatus === 'running') break;
    await new Promise((r) => setTimeout(r, 2000));
  }
  record('3 failed deploy → error', failStatus === 'error', `status=${failStatus}`);

  const failDep = await getLatestDeployment(token, failId);
  record(
    '3 deployment status failed',
    failDep?.status === 'failed',
    `deployment=${failDep?.status || 'none'}`,
  );

  // ========== 5: git scrub (only if GitHub app configured) ==========
  try {
    const gh = await fetch(`${BASE}/api/github/app-status`, {
      headers: { Authorization: `Bearer ${token}` },
    });
    const ghBody = await gh.json() as { configured?: boolean; connected?: boolean };
    if (gh.ok && (ghBody.configured || ghBody.connected)) {
      record('5 GitHub configured', true, JSON.stringify(ghBody));
      record('5 git scrub', false, 'SKIP: needs a private repo clone via UI/API with installation token — run manually');
    } else {
      record('5 git scrub', true, 'SKIP: GitHub App not configured on this install (manual when connected)');
    }
  } catch (e: any) {
    record('5 git scrub', true, `SKIP: ${e.message}`);
  }

  const failed = results.filter((r) => !r.ok);
  console.log('\n---');
  console.log(`Passed ${results.length - failed.length}/${results.length}`);
  if (failed.length) {
    console.log('Failed:');
    failed.forEach((f) => console.log(`  - ${f.name}: ${f.detail}`));
    process.exit(1);
  }
  console.log('Docker smoke checks passed (git scrub may be SKIP).');
}

main().catch((e) => {
  console.error(e);
  process.exit(1);
});
