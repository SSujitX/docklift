---
name: Database Management
description: Guide for managing the Docklift SQLite database using Prisma.
---

# Database Management Guide

Docklift uses SQLite as its database, managed by Prisma ORM.

## Schema Location

`backend/prisma/schema.prisma`

Database file: `data/docklift.db` on the host → `/app/data/docklift.db` in the backend container
(`DATABASE_URL=file:/app/data/docklift.db`).

Prisma client is a Proxy singleton (`lib/prisma.ts`) with `reconnectPrisma()` after restore replaces
the SQLite file. Backups use `VACUUM INTO` snapshots — see `system_administration` skill.

## Migration Model: checked-in Prisma migrations

Production boot runs `node dist/scripts/ensureDb.js` (see `backend/Dockerfile` CMD):

1. Dedupe `env_variables` duplicates (so unique `(project_id, key)` can apply)
2. `prisma migrate deploy` against `backend/prisma/migrations/`
3. Legacy installs that used `db push` (no `_prisma_migrations` history) are **baselined** then
   repaired idempotently (`publish_host_port`, `is_secret`, unique index)

**Never** ship `prisma db push --accept-data-loss` on container startup. Local `db:push` is for
dev experiments only.

### Schema change workflow
1. Edit `backend/prisma/schema.prisma`.
2. `bunx prisma migrate dev --name <desc>` (creates SQL under `prisma/migrations/`).
3. `bun run db:generate` — typed client.
4. Type-check: `cd backend; bunx tsc --noEmit`.
5. Verify on a **copy** of a real DB with `bun run db:ensure` / container boot — not only `db push`.

## Core Commands

Run from the `backend/` directory:

```bash
bun run db:studio      # web GUI to browse/edit data
bun run db:generate    # regenerate the typed client after editing the schema
bun run db:migrate     # prisma migrate deploy
bun run db:ensure      # production-equivalent bootstrap (dedupe + migrate + repair)
bun run db:push        # local-only; do not use as the container boot path
```

## Models

| Model | Table | Purpose |
|-------|-------|---------|
| `User` | `users` | Admin accounts. `passwordChangedAt` → JWT `pwdv` claim |
| `Project` | `projects` | An application: source, build settings, status |
| `Service` | `services` | One deployable unit within a project (Dockerfile, domain, ports) |
| `Deployment` | `deployments` | Build/deploy history, trigger, captured logs |
| `EnvVariable` | `env_variables` | Per-project vars: `is_build_arg` / `is_runtime` / **`is_secret`**; **`@@unique([project_id, key])`** |
| `PersistentVolume` | `persistent_volumes` | Configured named-volume mounts per service |
| `Port` | `ports` | Host port pool (`is_locked`); used only when project `publish_host_port` is true |
| `Settings` | `settings` | Key/value system settings (GitHub App creds, ACME email, panel domain) |

### Build & storage fields on `Project`

| Field | Default | Meaning |
|-------|---------|---------|
| `build_type` | `"auto"` | `auto` \| `dockerfile` \| `railpack` |
| `base_directory` | `"."` | Subdirectory to build from (monorepos) |
| `dockerfile_path` | `null` | Explicit Dockerfile when not auto-detecting |
| `internal_port` | `3000` | Port the app listens on inside the container |
| `publish_host_port` | `false` | When true, publish host ports from the pool |

`EnvVariable`: dedupe via `lib/envVariables.dedupeEnvVariables()` inside `scripts/ensureDb.ts`
**before** migrate deploy. Invalid keys → 400; duplicates → 409.

`PersistentVolume` has unique constraints on `(project_id, name)` and
`(project_id, service_name, mount_path)`, so one service cannot mount two volumes at the same path.

All child models cascade on project delete (`onDelete: Cascade`) — deleting a project removes its
services, deployments, env vars, volume records and frees its ports.

## Troubleshooting

- Client out of sync with schema → `bun run db:generate`.
- Startup fails on migrate → check `docker logs docklift-backend` for `[ensureDb]`; never “fix” with
  `--accept-data-loss` in the Dockerfile.
- Fresh install has no tables → `ensureDb` / migrate deploy failed.
- Restored backup looks stale → restore also reconciles projects and reloads nginx; see the
  `system_administration` skill.
