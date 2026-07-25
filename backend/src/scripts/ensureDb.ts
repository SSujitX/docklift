/**
 * Production DB bootstrap:
 * 1) Dedupe env_variables (so unique(project_id, service_name, key) can apply)
 * 2) prisma migrate deploy (checked-in migrations — never db push --accept-data-loss)
 * 3) Baseline legacy db-push installs, then repair any missing columns/indexes
 */
import { execFileSync } from 'child_process';
import path from 'path';
import { fileURLToPath } from 'url';
import { dedupeEnvVariables } from '../lib/envVariables.js';
import prisma from '../lib/prisma.js';

const __dirname = path.dirname(fileURLToPath(import.meta.url));
const backendRoot = path.resolve(__dirname, '../..');

function runPrisma(args: string[]): void {
  execFileSync('npx', ['prisma', ...args], {
    cwd: backendRoot,
    stdio: 'inherit',
    env: process.env,
    shell: process.platform === 'win32',
  });
}

function assertSafeIdent(name: string): string {
  if (!/^[A-Za-z_][A-Za-z0-9_]*$/.test(name)) {
    throw new Error(`Unsafe SQL identifier: ${name}`);
  }
  return name;
}

async function tableExists(name: string): Promise<boolean> {
  const safe = assertSafeIdent(name);
  const rows = await prisma.$queryRawUnsafe<Array<{ name: string }>>(
    `SELECT name FROM sqlite_master WHERE type='table' AND name='${safe}'`
  );
  return rows.length > 0;
}

async function columnExists(table: string, column: string): Promise<boolean> {
  const safeTable = assertSafeIdent(table);
  const cols = await prisma.$queryRawUnsafe<Array<{ name: string }>>(
    `PRAGMA table_info(${safeTable})`
  );
  return cols.some((c) => c.name === column);
}

/** Idempotent repairs for installs that used `db push` before migrate history existed. */
export async function repairLegacySchema(): Promise<void> {
  if (!(await tableExists('projects'))) return;

  if (!(await columnExists('projects', 'publish_host_port'))) {
    await prisma.$executeRawUnsafe(
      `ALTER TABLE "projects" ADD COLUMN "publish_host_port" BOOLEAN NOT NULL DEFAULT false`
    );
    console.log('[ensureDb] Added projects.publish_host_port');
  }

  if (await tableExists('env_variables')) {
    if (!(await columnExists('env_variables', 'is_secret'))) {
      await prisma.$executeRawUnsafe(
        `ALTER TABLE "env_variables" ADD COLUMN "is_secret" BOOLEAN DEFAULT false`
      );
      console.log('[ensureDb] Added env_variables.is_secret');
    }

    const hasServiceScope = await columnExists('env_variables', 'service_name');
    if (hasServiceScope) {
      // Scoped env: unique is (project_id, service_name, key). Never recreate the
      // pre-scope (project_id, key) index — it blocks shared+service overrides.
      await prisma.$executeRawUnsafe(
        `DROP INDEX IF EXISTS "env_variables_project_id_key_key"`
      );
      await prisma.$executeRawUnsafe(
        `CREATE UNIQUE INDEX IF NOT EXISTS "env_variables_project_id_service_name_key_key" ON "env_variables"("project_id", "service_name", "key")`
      );
    } else {
      await prisma.$executeRawUnsafe(
        `CREATE UNIQUE INDEX IF NOT EXISTS "env_variables_project_id_key_key" ON "env_variables"("project_id", "key")`
      );
    }
  }
}

async function migrationHistoryEmpty(): Promise<boolean> {
  if (!(await tableExists('_prisma_migrations'))) return true;
  const rows = await prisma.$queryRawUnsafe<Array<{ c: number }>>(
    `SELECT COUNT(*) as c FROM "_prisma_migrations"`
  );
  return Number(rows[0]?.c ?? 0) === 0;
}

async function baselineLegacyIfNeeded(): Promise<void> {
  const hasProjects = await tableExists('projects');
  if (!hasProjects) return;
  if (!(await migrationHistoryEmpty())) return;

  console.log('[ensureDb] Legacy database detected (no migration history) — baselining init');
  runPrisma(['migrate', 'resolve', '--applied', '20260725120000_init']);
}

async function main() {
  try {
    if (await tableExists('env_variables')) {
      const removed = await dedupeEnvVariables();
      if (removed > 0) {
        console.log(`[ensureDb] Removed ${removed} duplicate env_variables row(s)`);
      }
    }
  } catch (err) {
    console.warn('[ensureDb] Dedupe skipped:', err);
  }

  try {
    runPrisma(['migrate', 'deploy']);
  } catch (firstErr) {
    console.warn('[ensureDb] migrate deploy failed — attempting legacy baseline:', firstErr);
    try {
      await baselineLegacyIfNeeded();
      await repairLegacySchema();
      runPrisma(['migrate', 'deploy']);
    } catch (secondErr) {
      console.error('[ensureDb] migrate deploy failed after baseline:', secondErr);
      process.exit(1);
    }
  }

  try {
    await repairLegacySchema();
  } catch (err) {
    console.error('[ensureDb] Schema repair failed:', err);
    process.exit(1);
  }

  await prisma.$disconnect();
  console.log('[ensureDb] Database ready');
}

main().catch(async (err) => {
  console.error('[ensureDb] Failed:', err);
  await prisma.$disconnect().catch(() => {});
  process.exit(1);
});
