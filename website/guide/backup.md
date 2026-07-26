# Backup & Restore

Back up Docklift's control database, source projects, configurations, certificates, and secrets. **Docker named-volume contents** (including managed database data files) and external databases require their own backups.

## What's included

| Included | Contents |
|----------|----------|
| Database | Projects, users, settings, environment variables, storage definitions |
| Project files | Sources under `deployments/` |
| Nginx configs | Custom domain vhosts |
| Certificates | Let's Encrypt material managed with nginx-proxy |
| GitHub App key | `github-app.pem` when configured |
| Secrets | Control-plane secrets needed to restore the panel |

Not a full substitute for `pg_dump` / volume snapshots of app databases.

## Create a backup

1. Go to **Settings → Backup**
2. Click **Create Backup**
3. Wait for the progress stream to finish
4. Download the `.zip` to your local machine (or off-server storage)

Backups also land on the server under `/opt/docklift/data/backups/` (path may appear as `/data/backups/` inside the backend container).

## Restore (existing install)

::: warning
Restoring replaces **all** current control-plane data. Running app containers may be stopped. Take a fresh backup of the current state first if you might need it.
:::

1. Go to **Settings → Restore**
2. Upload a backup `.zip` or pick a previously uploaded file
3. Confirm with your **account password** (step-up auth)
4. Wait for the restore stream (API is locked while maintenance runs)
5. Backend restarts; projects are reconciled automatically when possible

## Restore on a fresh installation

Useful for migrating to a new VPS before creating users:

1. Install Docklift on the new server (note the **Setup code**)
2. Open `http://SERVER_IP:8080`
3. Enter the Setup code, then choose **Restore from Backup**
4. Upload your backup file (setup token authorizes this once — no admin password yet)
5. Setup code is consumed only after a full successful restore (admin present + reconcile OK)
6. If restore fails or is incomplete, the DB rolls back and the setup code stays valid for retry
7. If DB rollback itself fails, Docklift seals recovery with `data/.restore-critical` — do not keep retrying until repaired and cleared by an admin

## Best practices

- Create backups before major upgrades or risky changes
- Download and store backups off-server
- Test restore on a staging server before production migration
- Delete old on-server backups to free disk
- Separately back up managed database volumes / external DBs

## Related commands

```bash
curl -fsSL https://raw.githubusercontent.com/SSujitX/docklift/master/upgrade.sh | sudo bash
```

Upgrade takes its own snapshot path; UI backups remain the portable archive for server migration.
