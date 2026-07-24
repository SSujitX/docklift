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

## Migration Model: `db push`, not migrations

There is **no** `prisma/migrations` directory. The backend container runs
`npx prisma db push --skip-generate` on every startup, so schema changes apply automatically on
upgrade. Consequences to keep in mind:

- Always give new columns a default or make them nullable — `db push` cannot backfill.
- Renaming a column is a drop + add, i.e. **data loss**. Add the new column and migrate values in
  code instead.
- Destructive changes make `db push` refuse (or prompt) — verify against a copy of a real DB before shipping.

## Core Commands

Run from the `backend/` directory:

```bash
bun run db:studio      # web GUI to browse/edit data
bun run db:generate    # regenerate the typed client after editing the schema
bun run db:push        # apply schema to the SQLite file
```

### Schema change workflow
1. Edit `backend/prisma/schema.prisma`.
2. `bun run db:generate` — updates the client so TypeScript sees the new fields.
3. `bun run db:push` — applies to the local DB.
4. Type-check: `cd backend; .\node_modules\.bin\tsc --noEmit`.

### Reset (destroys all data)
```bash
bun run db:push --force-reset
```

## Models

| Model | Table | Purpose |
|-------|-------|---------|
| `User` | `users` | Admin accounts. `passwordChangedAt` invalidates older JWTs |
| `Project` | `projects` | An application: source, build settings, status |
| `Service` | `services` | One deployable unit within a project (Dockerfile, domain, ports) |
| `Deployment` | `deployments` | Build/deploy history, trigger, captured logs |
| `EnvVariable` | `env_variables` | Per-project vars, flagged `is_build_arg` / `is_runtime` |
| `PersistentVolume` | `persistent_volumes` | Configured named-volume mounts per service |
| `Port` | `ports` | Host port pool allocation (`is_locked` reserves a port) |
| `Settings` | `settings` | Key/value system settings (GitHub App creds, ACME email, panel domain) |

### Build & storage fields on `Project`

| Field | Default | Meaning |
|-------|---------|---------|
| `build_type` | `"auto"` | `auto` \| `dockerfile` \| `railpack` |
| `base_directory` | `"."` | Subdirectory to build from (monorepos) |
| `dockerfile_path` | `null` | Explicit Dockerfile when not auto-detecting |
| `internal_port` | `3000` | Port the app listens on inside the container |

`PersistentVolume` has unique constraints on `(project_id, name)` and
`(project_id, service_name, mount_path)`, so one service cannot mount two volumes at the same path.

All child models cascade on project delete (`onDelete: Cascade`) — deleting a project removes its
services, deployments, env vars, volume records and frees its ports.

## Troubleshooting

- Client out of sync with schema → `bun run db:generate`.
- `db push` refuses a change → it is destructive; add-then-migrate instead of renaming.
- Fresh install has no tables → the backend's startup `db push` failed; check `docker logs docklift-backend`.
- Restored backup looks stale → restore also reconciles projects and reloads nginx; see the
  `system_administration` skill.
