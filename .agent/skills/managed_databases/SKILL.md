---
name: Managed Databases
description: Coolify/Dokploy-style managed databases with Dokku-style app linking.
---

# Managed Databases

DockLift databases are first-class managed projects (`project_type=database`,
`source_type=managed`, `db_engine` set). They are **not** Git/ZIP apps.

## Engines

Defined in `backend/src/lib/databaseEngines.ts`:

| id | Image | Env key for apps |
|----|-------|------------------|
| postgres | postgres:16-alpine | DATABASE_URL |
| mysql | mysql:8.4 | DATABASE_URL |
| mariadb | mariadb:11 | DATABASE_URL |
| redis | redis:7-alpine | REDIS_URL |
| mongodb | mongo:7 | MONGODB_URI |

- Official images + named volume at the engine mount path
- Credentials stored as project env (passwords marked `is_secret`)
- `publish_host_port` default **false** — prefer linking

## API

| Method | Path | Purpose |
|--------|------|---------|
| GET | `/api/databases/engines` | Catalog |
| GET/POST | `/api/databases` | List / create |
| GET | `/api/databases/:id/connection` | URL + credentials |
| GET/POST/DELETE | `/api/databases/:id/links` | Link management |
| GET | `/api/databases/links/by-app/:appProjectId` | Links on an app |

Create does **not** require Git. Deploy pulls the image (no Dockerfile build).

## Linking

`DatabaseLink` rows: database → app project, optional `service_name` (`""` = shared env).

On link:

1. Attach DB container to app `dl-net-*` (`connectContainerToProjectNetwork`)
2. Upsert scoped/shared runtime secret env (`DATABASE_URL` / `REDIS_URL` / …)
3. Operator **redeploys the app** so containers pick up env

After DB or app deploy, links are re-applied (network join).

## UI

- `/databases` — list
- `/databases/new` — engine picker + create & deploy
- Project detail (database) — Connection + Linked apps (`ManagedDatabasePanel`)
- Project detail (app) — Attach database (`AttachDatabasePanel`)
- Logs — same LogViewer as apps (service runtime logs)

## Rules

- One owner per `(app_project_id, service_name, env_key)` — unique index + overwrite gate.
- App stop / cancel / delete must `disconnectLinkedDatabasesFromApp` before compose down;
  on failed teardown, `reapplyDatabaseLinksForApp` (same as proxy restore).
- DB delete: Docker teardown first, then `cleanupLinksForDatabaseProject`, then prisma delete.
- Credential env keys are locked; recreate DB to rotate.
- Prefer linking over advertising raw `IP:port`.
- Link requires DB `status` running/degraded; env inject before/with soft network attach.
