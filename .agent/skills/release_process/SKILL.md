---
name: Release Process
description: Guide to Docklift's manual release flow (root VERSION file, sync-version.sh, Release workflow).
---

# Release Process

The root `VERSION` file (`X.Y.Z`, one line) is the single source of the product version. A release
is a manual run of the **Release** workflow: it bumps `VERSION` (or releases it as it is), syncs the
copies, writes the `CHANGELOG.md` section, commits, tags `vX.Y.Z`, publishes the GitHub Release
"Docklift vX.Y.Z" and deploys the docs site.

## How It Works

```
Push to master → CI + Install green → Actions → "Release" (bump, dry_run) → tag + GitHub Release + Docs (Pages)
```

### Pipeline Steps
1. **CI** (`ci.yml`) and **Install** (`install.yml`) run on push; Release does not re-test. Wait for green.
2. Run **Release** from master with `bump` = `patch` | `minor` | `major` | `current` and optionally `dry_run`.
3. **Bump VERSION**: `patch` 3.0.0 → 3.0.1, `minor` → 3.1.0, `major` → 4.0.0. `current` keeps `VERSION`
   as it is (for a `VERSION` you already changed in a commit).
4. **Reject duplicate tag**: fails if `vX.Y.Z` exists locally or on origin. With `current` this means
   "VERSION is already released": bump it, or commit a new `VERSION` first.
5. **Sync version copies**: `scripts/sync-version.sh` (see below).
6. **Build release notes and CHANGELOG**: `scripts/release-notes.sh vX.Y.Z NOTES_FILE` lists every
   non-merge commit since the previous `v*` tag (oldest first, earlier `chore(release):` commits left
   out) as `- subject ([sha](commit link))`. It writes the GitHub Release body (`## What's Changed`
   + **Full Changelog** compare link) and adds `## [X.Y.Z](compare link) (YYYY-MM-DD)` with the same
   list at the top of `CHANGELOG.md`. It fails when there is no commit since the previous tag.
7. **Show release diff**: `git status` + `git diff` (the whole run so far is the dry-run preview).
8. **Commit, tag, and push** (skipped on `dry_run`): `chore(release): vX.Y.Z` with `[skip ci]` in the
   body, annotated tag `vX.Y.Z`, one `git push --atomic` of master and the tag (if master moved since
   the run started, nothing lands).
9. **Create GitHub Release** (skipped on `dry_run`): `gh release create vX.Y.Z --title "Docklift vX.Y.Z"
   --verify-tag --latest`.
10. **Docs** (skipped on `dry_run`): `docs.yml` via `workflow_call` with `ref: vX.Y.Z`, so docklift.dev
    builds from the release commit (new changelog). Docs also runs alone on `website/**` /
    `CHANGELOG.md` pushes.

The homepage preview and `/changelog` read the root `CHANGELOG.md` at VitePress build time
(`website/scripts/sync-changelog.mjs`, `website/.vitepress/changelog.data.ts`, which splits releases on
`## [x.y.z]` and the `# [x.y.0]` headings semantic-release wrote up to 3.0.0). Do not edit released
sections by hand; fix wording in a later commit if needed.

## Key Files

| File | Purpose |
|------|---------|
| `VERSION` | **Version source of truth** |
| `scripts/sync-version.sh` | `sync` (default) writes `VERSION` into every copy; `check` fails on drift (CI runs it) |
| `scripts/release-notes.sh` | Release notes + `CHANGELOG.md` section from the git log since the previous tag |
| `.github/workflows/release.yml` | Manual **Release** (`bump`, `dry_run`) + Docs |
| `.github/workflows/install.yml` | Install smoke matrix (`fresh`, `from-release`; the `fresh` cell also runs `tests/e2e`) |
| `.github/workflows/ci.yml` | Go, contract suite, frontend, shell checks (incl. `sync-version.sh check`), image build + smoke test |
| `.github/workflows/docs.yml` | VitePress → GitHub Pages (`workflow_call` input `ref`) |
| `CHANGELOG.md` | One section per release, newest first |
| `package.json` (root) | Not a Node package: only a synced `version` copy, kept for existing installs (below) |

## Where the version is read

| Consumer | How it gets the version |
|----------|-------------------------|
| Go binary (`docklift/internal/version.Version`, `/api/health`, `/api/system/version`) | `-ldflags -X`: the `Dockerfile` reads `/src/VERSION` unless `--build-arg VERSION` is given (`DOCKLIFT_BUILD_VERSION` in compose); `make build` / `make server` / `make dev` read `VERSION` |
| Dashboard label (`__APP_VERSION__`) | `frontend/vite.config.ts` reads `../VERSION`, or `DOCKLIFT_APP_VERSION` (the Dockerfile sets it from the `VERSION` build arg) |
| `install.sh`, `install-dev.sh`, `upgrade.sh` | `$INSTALL_DIR/VERSION`; `install.sh` / `upgrade.sh` fall back to the root `package.json` for checkouts of 3.0.0 and older |
| `frontend/package.json` `version` | Copy, synced |
| Root `package.json` `version` | Copy, synced (see below) |
| Release-pin examples (`v=X.Y.Z`, `DOCKLIFT_VERSION=X.Y.Z`, `--build-arg VERSION=X.Y.Z`) in `README.md`, `commands.md`, `website/guide/commands.md`, `website/guide/installation.md`, `install.sh`, `upgrade.sh`, `Dockerfile` | Copies, synced. Every such example in those files is rewritten; name an older release another way ("tag v2.1.6") |

The Go module has no version of its own. `uninstall.sh` reads no version.

### Why the root `package.json` stays

The `upgrade.sh` shipped with 3.0.0 is on disk at `/opt/docklift/upgrade.sh` on existing installs, and
the dashboard's **Upgrade** button runs that copy (`server/internal/api/system/host_linux.go`). After it
checks out the new tag it reports the new version from the root `package.json`. Keeping a minimal
`package.json` with the synced `version` lets those installs print `3.0.0 → X.Y.Z` instead of
`unknown`. (2.1.x installs read `backend/package.json`, which 3.0.0 already removed.) The file can go
once no supported install runs a pre-VERSION `upgrade.sh`.

## How to Release

```bash
# 1. Commit (conventional messages: they become the changelog lines) and push to master
git commit -m "fix(deploy): description of change"
git push origin master

# 2. Wait for CI + Install, then GitHub → Actions → "Release" → Run workflow
#    bump: patch | minor | major | current, dry_run: true first to preview the diff and notes
# 3. Run again with dry_run off
```

Local checks:

```bash
./scripts/sync-version.sh check    # what CI runs
./scripts/sync-version.sh          # after changing VERSION by hand (then commit; release with bump=current)
```

| `bump` | `VERSION` 3.0.0 → | Tag |
|--------|-------------------|-----|
| `patch` | 3.0.1 | `v3.0.1` |
| `minor` | 3.1.0 | `v3.1.0` |
| `major` | 4.0.0 | `v4.0.0` |
| `current` | 3.0.0 (unchanged) | `v3.0.0`: fails, it exists |

## What install and upgrade expect

`install.sh` and `upgrade.sh` resolve the target through `GET /repos/SSujitX/docklift/releases/latest`
(`tag_name`), check the tag with `git ls-remote`, then `git checkout` it; the server's version check
(`/api/system/version`) strips the `v` from the same `tag_name`. Release tags must stay `vX.Y.Z`
on master, and the GitHub Release must be the latest (non-draft, non-prerelease; `--latest`).
The release title ("Docklift vX.Y.Z") is not read by anything.

## Server upgrade script (`upgrade.sh`)

When documenting or changing upgrades:
- Pin `ROLLBACK_REF=$(git rev-parse HEAD)` before fetching new code.
- On compose build failure **or** failed `/api/health` after start: checkout previous ref, restore
  `.db.bak`, recreate stack.
- Stop backend before DB snapshot; prefer `sqlite3 .backup`, else copy while stopped.
- Tag `docklift-backend:pre-upgrade` before rebuild (the script also tags a leftover
  `docklift-frontend` from a 2.x install); rollback retags those images (do not rely only on
  rebuilding an old git ref).
- Upgrading from 2.x removes the obsolete `docklift-frontend` and `docklift-nginx` containers
  (`OBSOLETE_CONTAINERS` / `remove_obsolete_containers`); the stack is then `backend`, `nginx-proxy`,
  `certbot`. The new backend applies the embedded SQL migrations on start, over the existing database.
- Create/validate backup directory **before** stopping backend.
- `docker compose stop backend` must succeed; probe run-state as running | stopped | probe-error
  (no `|| true` on stop). Abort before DB copy on running **or** probe-error; restart backend on abort.
- Capture `backend_run_state` with `if backend_run_state; then … else BACKEND_STATE=$?; fi` —
  never as a standalone call under `set -e` (return 1/2 would abort before the snapshot).
  Behavioral coverage: `scripts/test-upgrade-backend-run-state.sh` (stubbed Docker).
- `arm_rollback` immediately after verified stop; `disarm_rollback` only after final health OK.

- Health probe (`health_ok`): `docker exec docklift-backend wget -q -O /dev/null http://127.0.0.1:8000/api/health`
  (the Go image has busybox `wget`; the `node` fetch fallback only matters when rolling back to a 2.1.x image).
- Print `http://SERVER_IP:8080` after success.
- Preserve `data/`, `deployments/`, nginx confs, certs, user `dl_*` containers.
- `format_time` must tolerate `((0))` under `set -e` (`|| true`).

Install scripts (`install.sh` / `install-dev.sh`): print Dashboard URL + Setup code; `cd /opt/docklift`
before `docker compose`; same `format_time` rule. `install.sh` accepts optional release pin
(`bash -s -- v=X.Y.Z` / `DOCKLIFT_VERSION`); default is GitHub `releases/latest`. Validates the
tag (ls-remote) before `compose down`; fails closed if latest cannot be resolved (no master fallback).

Uninstall: DockLift-named/labelled resources only (incl. `dl-net-*`); **no** host-wide
`docker system prune` / `builder prune`.

## GitHub Token

Checkout (and so the release push) uses `secrets.GH_TOKEN || github.token`. Set `GH_TOKEN` to a
Personal Access Token with `repo` scope when master is branch-protected; otherwise the workflow's
`GITHUB_TOKEN` (`contents: write`) is enough. `gh release create` uses `github.token`. A push made with
the PAT would start CI / Install / Docs on the release commit; `[skip ci]` in the commit body stops that
(Docs runs from Release instead). Pushes made with `GITHUB_TOKEN` start no workflows anyway.

## Troubleshooting

| Issue | Fix |
|-------|-----|
| CI: "X is out of sync with VERSION" | Run `./scripts/sync-version.sh` and commit |
| Release: "Tag vX.Y.Z already exists" | `current` on a released VERSION, or a tag made by hand: pick a bump, or delete the stray tag |
| Release: "No commits since vX.Y.Z" | Nothing to release; commit something first |
| Release: push rejected | master moved during the run (`--atomic`, nothing landed): run Release again; or the token cannot push to a protected master (set `GH_TOKEN`) |
| "Release runs from master only" | Pick master in "Use workflow from" |
| Release shipped something broken | Release does not re-test; run CI and Install on master first and wait for green |
| Binary reports the wrong version | Check `VERSION` and that the image build got no stale `VERSION` build arg (`DOCKLIFT_BUILD_VERSION` in compose) |
| Site shows no new changelog section | Docs must build the tag (`ref` input); re-run Docs with `workflow_dispatch` on master |

semantic-release (`release.config.cjs`, the root devDependencies) is gone; do not add it back, and do
not bump versions with `npm version` / `bumpp`. Change `VERSION` (or let Release do it) and run
`sync-version.sh`.
