// Regenerates testdata/seed.db.gz: a SQLite database written by the TypeScript
// backend's own Prisma client and migrations, holding state the HTTP API cannot
// create without GitHub or Docker (webhook secret, GitHub App settings, managed
// database projects, successful deployments with image tags).
//
// Usage (from tests/contract):
//   node testdata/seed/generate.mjs ../../backend testdata/seed.db.gz
//
// IDs and values here are mirrored in seed_test.go — change both together.
import { execFileSync } from 'node:child_process';
import { createRequire } from 'node:module';
import fs from 'node:fs';
import os from 'node:os';
import path from 'node:path';
import zlib from 'node:zlib';

const backendDir = path.resolve(process.argv[2] || '../../backend');
const outFile = path.resolve(process.argv[3] || 'testdata/seed.db.gz');
const require = createRequire(path.join(backendDir, 'package.json'));

const work = fs.mkdtempSync(path.join(os.tmpdir(), 'docklift-seed-'));
const dbPath = path.join(work, 'seed.db');
const dbUrl = `file:${dbPath.replace(/\\/g, '/')}`;

const prismaCli = path.join(backendDir, 'node_modules', 'prisma', 'build', 'index.js');
execFileSync(process.execPath, [prismaCli, 'migrate', 'deploy'], {
  cwd: backendDir,
  env: { ...process.env, DATABASE_URL: dbUrl },
  stdio: 'inherit',
});

process.env.DATABASE_URL = dbUrl;
const { PrismaClient } = require('@prisma/client');
const bcrypt = require('bcrypt');
const prisma = new PrismaClient();

const id = (n) => `5eed0000-0000-4000-8000-${String(n).padStart(12, '0')}`;
const at = (iso) => new Date(iso);

await prisma.user.create({
  data: {
    id: id(1),
    name: 'Seed Admin',
    email: 'seed-admin@contract.test',
    password: bcrypt.hashSync('SeedPassw0rd!', 12),
    role: 'admin',
    passwordChangedAt: at('2026-01-01T00:00:00.000Z'),
    created_at: at('2026-01-01T00:00:00.000Z'),
  },
});

const settings = {
  github_webhook_secret: 'contract-webhook-secret',
  github_app_id: '123456',
  github_app_slug: 'docklift-contract',
  github_app_name: 'docklift-contract',
  github_installation_id: '987654',
  github_username: 'contract-user',
  github_avatar_url: 'https://avatars.example.test/u/1',
};
for (const [key, value] of Object.entries(settings)) {
  await prisma.settings.create({ data: { key, value, updated_at: at('2026-01-01T00:00:00.000Z') } });
}

const repo = 'https://github.com/contract-org/contract-repo';
await prisma.project.create({
  data: {
    id: id(101), name: 'seed-gh-main', source_type: 'github', project_type: 'app',
    github_url: repo, github_branch: 'main', status: 'stopped', auto_deploy: true,
    webhook_secret: 'a'.repeat(64), created_at: at('2026-01-01T00:01:00.000Z'),
  },
});
await prisma.project.create({
  data: {
    id: id(102), name: 'seed-gh-develop', source_type: 'github', project_type: 'app',
    github_url: `${repo}.git`, github_branch: 'develop', status: 'stopped', auto_deploy: true,
    webhook_secret: 'b'.repeat(64), created_at: at('2026-01-01T00:02:00.000Z'),
  },
});
await prisma.project.create({
  data: {
    id: id(103), name: 'seed-gh-manual', source_type: 'github', project_type: 'app',
    github_url: repo, github_branch: 'main', status: 'stopped', auto_deploy: false,
    webhook_secret: null, created_at: at('2026-01-01T00:03:00.000Z'),
  },
});

await prisma.project.create({
  data: {
    id: id(201), name: 'seed-app', source_type: 'upload', project_type: 'app',
    status: 'stopped', build_type: 'auto', created_at: at('2026-01-01T00:04:00.000Z'),
  },
});
await prisma.service.create({
  data: {
    id: id(211), project_id: id(201), name: 'app', dockerfile_path: 'Dockerfile',
    container_name: 'dl_seed-app_5eed0000_app', internal_port: 3000, status: 'stopped',
    created_at: at('2026-01-01T00:04:00.000Z'),
  },
});
const deployments = [
  [301, 'success', { app: 'docklift-5eed0000-app:5eed0000' }, '2026-01-02T00:00:00.000Z'],
  [302, 'success', null, '2026-01-03T00:00:00.000Z'],
  [303, 'failed', null, '2026-01-04T00:00:00.000Z'],
  [304, 'success', { app: 'docklift-5eed0000-app:5eed0004' }, '2026-01-05T00:00:00.000Z'],
];
for (const [n, status, tags, created] of deployments) {
  await prisma.deployment.create({
    data: {
      id: id(n), project_id: id(201), status, trigger: 'manual',
      commit_message: `seed deployment ${n}`, logs: `seed log ${n}\n`,
      ...(tags ? { image_tags: tags } : {}),
      created_at: at(created), finished_at: at(created),
    },
  });
}

const dbProjects = [
  [401, 'seed-pg', 'postgres', 'running', { POSTGRES_USER: 'docklift', POSTGRES_PASSWORD: 'seedpgpass', POSTGRES_DB: 'seed_pg' }],
  [402, 'seed-redis', 'redis', 'stopped', { REDIS_PASSWORD: 'seedredispass' }],
  [403, 'seed-mysql', 'mysql', 'running', {}],
];
for (const [n, name, engine, status, env] of dbProjects) {
  await prisma.project.create({
    data: {
      id: id(n), name, description: `${engine} seed`, source_type: 'managed', project_type: 'database',
      db_engine: engine, status, build_type: 'dockerfile', internal_port: engine === 'redis' ? 6379 : engine === 'mysql' ? 3306 : 5432,
      created_at: at(`2026-01-01T00:0${n - 395}:00.000Z`),
    },
  });
  await prisma.service.create({
    data: {
      id: id(n + 10), project_id: id(n), name: 'db', dockerfile_path: `[managed:${engine}|${engine}:latest]`,
      container_name: `dl_${name}_5eed0000_db`, internal_port: engine === 'redis' ? 6379 : engine === 'mysql' ? 3306 : 5432,
      status, created_at: at('2026-01-01T00:10:00.000Z'),
    },
  });
  for (const [key, value] of Object.entries(env)) {
    await prisma.envVariable.create({
      data: { project_id: id(n), service_name: '', key, value, is_runtime: true, is_build_arg: false, is_secret: key.includes('PASSWORD') },
    });
  }
}

await prisma.$disconnect();
fs.mkdirSync(path.dirname(outFile), { recursive: true });
fs.writeFileSync(outFile, zlib.gzipSync(fs.readFileSync(dbPath), { level: 9 }));
fs.rmSync(work, { recursive: true, force: true });
console.log(`wrote ${outFile}`);
