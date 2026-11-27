# Docklift end-to-end suite

`TestEndToEnd` drives a **real install** (`install.sh` or `upgrade.sh` on a Docker host) through
its HTTP API the way a user does, then checks the result on the host with the `docker` CLI:

first admin → GitHub project (`docklift-test`, `server-test/`) deployed → reachable inside its
network and through nginx-proxy with a domain → awkward env values seen byte for byte by the
container → stop / restart → failing build (status failed, lock released, next deploy works) →
second deploy refused while one runs, cancel mid-build → rollback to the first image (step-up)
→ managed databases linked to the app, injected URL authenticates from the app network →
storage volume survives redeploys → ZIP project → file editor (`.git` and traversal refused) →
container log SSE, stats → terminal WebSocket (auth, command, resize, exit) → system endpoints →
backup create / download / restore on the same instance → purge keeps in-use and rollback images
→ project delete removes containers, networks, images, volumes, nginx confs and files.

It is destructive for the instance it runs against (restore, purge, project deletes): use a
throwaway install.

## Running

Run on the Docker host of the stack (the host-side checks call `docker` and read the install
directory), from this directory:

```sh
DOCKLIFT_E2E_URL=http://127.0.0.1:8080 \
DOCKLIFT_E2E_BOOTSTRAP_SECRET="$(sudo cat /opt/docklift/data/.bootstrap-secret)" \
DOCKLIFT_E2E_ADMIN_PASSWORD='a-long-test-password' \
go test -count=1 -timeout 90m -v ./...
```

Without `DOCKLIFT_E2E_URL` the test skips.

| Variable | Default | Meaning |
|---|---|---|
| `DOCKLIFT_E2E_URL` | (required) | dashboard / API base URL |
| `DOCKLIFT_E2E_ADMIN_PASSWORD` | (required) | admin password to register (fresh install) or log in with |
| `DOCKLIFT_E2E_ADMIN_EMAIL` | `e2e-admin@docklift.test` | admin email |
| `DOCKLIFT_E2E_BOOTSTRAP_SECRET` | | needed when the install has no admin yet |
| `DOCKLIFT_E2E_PROXY_URL` | `http://127.0.0.1:80` | nginx-proxy, for the domain check (sent with a `Host` header) |
| `DOCKLIFT_E2E_DOMAIN` | random `*.docklift.test` | domain assigned to the app |
| `DOCKLIFT_E2E_INSTALL_DIR` | `/opt/docklift` | install directory (nginx confs, deployments) |
| `DOCKLIFT_E2E_REPO` / `_REPO_BRANCH` / `_REPO_DIR` / `_REPO_PORT` | `SSujitX/docklift-test`, `master`, `server-test`, `4000` | app to deploy |
| `DOCKLIFT_E2E_ENGINES` | `postgres,redis` | managed database engines to create (postgres and redis are kept until the end; others are removed right after their check) |
| `DOCKLIFT_E2E_RESTORE` | `1` | `0` skips backup restore |
| `DOCKLIFT_E2E_DOCKER` | `1` | `0` skips the host-side `docker` checks (API-only run from another machine) |
