---
name: Managed Databases
description: Coolify/Dokploy-style managed databases with Dokku-style app linking.
---

# Managed Databases

DockLift databases are first-class managed projects (`project_type=database`,
`source_type=managed`, `db_engine` set). They are **not** Git/ZIP apps.

## Engines

Defined in `backend/src/lib/databaseEngines.ts`:

| id | New-create default | Bare `[managed:id]` pin | Env key |
|----|--------------------|-------------------------|---------|
| postgres | postgres:18-alpine | postgres:16-alpine | DATABASE_URL |
| mysql | mysql:8.4 | mysql:8.4 | DATABASE_URL |
| mariadb | mariadb:11 | mariadb:11 | DATABASE_URL |
| redis | redis:8-alpine | redis:7-alpine | REDIS_URL |
| mongodb | mongo:8 | mongo:7 | MONGODB_URI |

- **`GET /api/databases/engines`** loads **live tags from Docker Hub** (`dockerHubTags.ts`, 6h cache only when pagination completes); static `versions` are fallback
- Filtered to operator-friendly majors (e.g. `18`, `18-alpine`) — not every distro/RC tag
- Create accepts `version` (tag); stores full image on service as `[managed:engine|repo:tag]`
- Deploy pulls the **stored** image; bare `[managed:engine]` → `LEGACY_MANAGED_IMAGES` (never silent major bump)
- **Postgres volume mount is version-aware**: ≤17 → `/var/lib/postgresql/data`; 18+/`latest` → `/var/lib/postgresql` (official image contract). Create + deploy sync `persistent_volumes.mount_path`. Redeploy cannot migrate data that already landed on an anonymous volume under the old `/data` mount — recreate + restore if needed.
- Credentials stored as project env (passwords marked `is_secret`)
- `publish_host_port` default **false** — prefer linking

## API

| Method | Path | Purpose |
|--------|------|---------|
| GET | `/api/databases/engines` | Catalog (+ live Hub `versions`, `imageRepo`) |
| GET/POST | `/api/databases` | List / create (`version` / `tag` on create) |
| GET | `/api/databases/:id/connection` | URL + credentials |
| GET/POST/DELETE | `/api/databases/:id/links` | Link management |
| GET | `/api/databases/links/by-app/:appProjectId` | Links on an app |

Create does **not** require Git. Deploy **async-pulls** the image (never `spawnSync` —
blocking pull freezes the whole API and blanks the project UI). Pull is registered with
the deploy cancel tracker (kill + timeout), and the deploy lock is owned by
`deploymentId` so a cancelled pull cannot unlock a newer deploy. Pull output streams into
deployment logs (polled from Deployments tab).

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
- Project detail (database) — tabs: **Overview · Deployments · Logs** only
  (no Environment / Build / Storage / Source / Domains — credentials on Overview Connection;
  locked at create; recreate to rotate; prefer linking over public DNS)
- Overview — Connection + Linked apps (`ManagedDatabasePanel`)
- Project detail (app) — Attach database (`AttachDatabasePanel`): scope picker only on **All services**; locked to current service in service workspace (rounded custom selects)
- Logs — same LogViewer as apps (service runtime logs)

## Rules

- One owner per `(app_project_id, service_name, env_key)` — unique index + overwrite gate.
- App stop / cancel / delete must `disconnectLinkedDatabasesFromApp` before compose down;
  on failed teardown, `reapplyDatabaseLinksForApp` (same as proxy restore).
- DB delete: Docker teardown first, then `cleanupLinksForDatabaseProject`, then prisma delete.
- Credential env keys are locked; recreate DB to rotate.
- Prefer linking over advertising raw `IP:port`.
- Link requires DB `status` running/degraded; env inject before/with soft network attach.
