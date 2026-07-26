# Environment Variables

Manage secrets and configuration for applications. Values are stored in Docklift's control database and injected into containers at deploy/runtime.

## Example

```bash
DATABASE_URL=postgresql://user:pass@host:5432/db
API_KEY=sk_live_…
```

## Scopes

| Scope | Where | Notes |
|-------|-------|--------|
| **Shared** | All services workspace | Applied to every service in the project |
| **Service** | One service workspace | Overrides shared on the same key |
| **Single-service projects** | Flat env list | No Workspace rail |

Invalid key names are rejected.

## Runtime vs build

Mark each variable as **runtime**, **build**, or both:

- **Runtime** — available inside the running container (`process.env`, etc.)
- **Build** — available during image build
- **BuildKit secret** (with build) — passed as a Docker secret, not `--build-arg`. Dockerfile should use:

```dockerfile
RUN --mount=type=secret,id=KEY …
```

Update variables, then **Deploy** (or Redeploy) so containers pick up changes. Restart alone may not rebuild images that baked build-time values.

## Databases and persistence

- An external PostgreSQL/MySQL URL in `DATABASE_URL` is not replaced during image builds
- For Docklift-managed databases, prefer [linking](./databases.md) over hand-pasting URLs — linking injects `DATABASE_URL` / `REDIS_URL` / `MONGODB_URI` and joins networks
- SQLite databases and uploaded files **inside** the container need a named mount from the project's **Storage** tab
- Persistent volumes survive deploy, redeploy, stop, restart, and image replacement
- Volumes are removed when you delete the project
- Application migrations can still alter DB contents; persistence is not a substitute for backups

```bash
docker volume ls --filter label=com.docklift.project
```

## Security notes

- Do not commit secrets into the repo; put them in Docklift env
- Prefer runtime injection over baking secrets into image layers
- Step-up password may be required for some destructive account operations elsewhere; env edits still require an authenticated admin session
