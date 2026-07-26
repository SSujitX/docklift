# Managed Databases

Docklift can run **Postgres**, **MySQL**, **MariaDB**, **Redis**, and **MongoDB** as first-class database projects — official images, named volumes, generated credentials. **Host ports stay off by default.** Prefer linking over publishing `IP:port`.

## Create

1. Open **Databases → New Database**
2. Pick an engine, **version** (tags load from Docker Hub — majors / Alpine where available), and name
3. Docklift creates the volume, credentials, and starts a deploy (image pull)
4. Copy the connection URL from the database **Overview → Connection** panel

Database projects only show **Overview**, **Deployments**, and **Logs**. Credentials live on Overview → Connection (locked at create — recreate to rotate). No Environment / Build / Source / Domains.

## Link to an app or service

From the database:

1. Open **Linked apps**
2. Choose a project and optional service (empty service = shared env for all services)

From an app:

1. Overview → **Attach database**

What linking does:

- Joins the DB container to the app's Docker network
- Injects `DATABASE_URL`, `REDIS_URL`, or `MONGODB_URI` as a runtime secret
- Requires **Redeploy the app** so running containers see the new variable
- Only one database may own a given env key on the same project/service scope
- Attaching over an existing env key requires an explicit **Overwrite** confirmation

## Security notes

| Topic | Guidance |
|-------|----------|
| Exposure | Prefer linking; keep host ports off |
| Credentials | Set at create time and locked in the UI/API |
| Rotation | **Recreate** the database to rotate passwords (official images often apply init env only on first volume init) |
| Delete | Removes injected env from linked apps after teardown succeeds |
| Env edits | Blocked for managed credential fields so Connection URLs cannot lie about the server |

## Persistence

Managed databases use named volumes. Control-plane [backups](./backup.md) cover Docklift's SQLite and project metadata — **not** the full contents of Docker named volumes. Back up database data separately if you need restore guarantees for app data.

## Related

- [Environment Variables](./environment.md) — shared vs service scopes
- [Port Management](./ports.md) — why private-by-default matters
- [Deployment](./deployment.md) — redeploy after attach
