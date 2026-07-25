---
name: Release Process
description: Guide to Docklift's automated release pipeline using semantic-release.
---

# Release Process

Docklift uses **semantic-release** to fully automate versioning, changelogs, and GitHub Releases.

## How It Works

```
Push to master → Run "Release & Test" workflow → semantic-release handles everything
```

### Pipeline Steps (automatic)
1. Runs tests via `test-ubuntu.yml`
2. Analyzes commit messages to determine version bump
3. Bumps `package.json` in root, frontend, and backend
4. Updates `CHANGELOG.md`
5. Commits bumped files back to master: `chore(release): X.Y.Z [skip ci]`
6. Creates git tag `vX.Y.Z`
7. Creates GitHub Release with generated notes

## Key Files

| File | Purpose |
|------|---------|
| `release.config.cjs` | semantic-release config (plugins, release rules, assets) |
| `.github/workflows/release.yml` | GitHub Actions workflow (manual trigger via `workflow_dispatch`) |
| `CHANGELOG.md` | Auto-updated changelog |
| `package.json` (root) | Root version + semantic-release devDependencies |

## Commit Convention

Commits **must** follow [Conventional Commits](https://www.conventionalcommits.org/) format:

```
type(scope): description
```

### Release Rules (from `release.config.cjs`)

| Commit Type | Release Type |
|-------------|-------------|
| `feat:` | patch |
| `fix:` | patch |
| `perf:`, `style:`, `refactor:` | patch |
| `docs:`, `test:`, `ci:`, `chore:`, `build:` | patch |
| `wip:` | patch |
| `BREAKING CHANGE` (type, scope, or subject) | **major** |
| `*force minor*` in subject | minor |
| `*force major*` in subject | major |
| `*force patch*` / `*force release*` in subject | patch |
| `*skip release*` in subject | no release |

> **Note:** ALL commit types trigger a **patch** release. This is intentional — Docklift treats every commit type as release-worthy.

### Examples

```bash
# Standard commits
git commit -m "fix(deploy): use fetch+reset instead of git pull"
git commit -m "feat(logs): add search functionality to log viewer"
git commit -m "docs: update README with release instructions"

# Force a minor release
git commit -m "feat(api): add new endpoint *force minor*"

# Skip release entirely
git commit -m "chore: update comments *skip release*"
```

## Server upgrade script (`upgrade.sh`)

When documenting or changing upgrades:
- Pin `ROLLBACK_REF=$(git rev-parse HEAD)` before fetching new code.
- On compose build failure **or** failed `/api/health` after start: checkout previous ref, restore
  `.db.bak`, recreate stack.
- Stop backend before DB snapshot; prefer `sqlite3 .backup`, else copy while stopped.
- Tag `docklift-backend:pre-upgrade` / `docklift-frontend:pre-upgrade` before rebuild; rollback
  retags those images (do not rely only on rebuilding an old git ref).
- Create/validate backup directory **before** stopping backend.
- `docker compose stop backend` must succeed; probe run-state as running | stopped | probe-error
  (no `|| true` on stop). Abort before DB copy on running **or** probe-error; restart backend on abort.
- Capture `backend_run_state` with `if backend_run_state; then … else BACKEND_STATE=$?; fi` —
  never as a standalone call under `set -e` (return 1/2 would abort before the snapshot).
  Behavioral coverage: `scripts/test-upgrade-backend-run-state.sh` (stubbed Docker).
- `arm_rollback` immediately after verified stop; `disarm_rollback` only after final health OK.

- Health probe: `docker compose exec backend node -e "fetch('http://127.0.0.1:8000/api/health')…"`.
- Print `http://SERVER_IP:8080` after success.
- Preserve `data/`, `deployments/`, nginx confs, certs, user `dl_*` containers.
- `format_time` must tolerate `((0))` under `set -e` (`|| true`).

Install scripts (`install.sh` / `install-dev.sh`): print Dashboard URL + Setup code; `cd /opt/docklift`
before `docker compose`; same `format_time` rule. `install.sh` accepts optional release pin
(`bash -s -- v=2.0.2` / `DOCKLIFT_VERSION`); default is GitHub `releases/latest`. Validates the
tag (ls-remote) before `compose down`; fails closed if latest cannot be resolved (no master fallback).

Uninstall: DockLift-named/labelled resources only (incl. `dl-net-*`); **no** host-wide
`docker system prune` / `builder prune`.

## How to Release

```bash
# 1. Commit your changes with conventional messages
git add -A
git commit -m "fix(deploy): description of change"

# 2. Push to master
git push origin master

# 3. Go to GitHub → Actions → "Release & Test" → Run workflow
# semantic-release does everything else automatically
```

## Version Bump Strategy

The `@semantic-release/exec` plugin bumps versions in sub-packages:

```bash
npm version X.Y.Z --no-git-tag-version --allow-same-version --prefix frontend
npm version X.Y.Z --no-git-tag-version --allow-same-version --prefix backend
```

The `@semantic-release/npm` plugin bumps the root `package.json`.

The `@semantic-release/git` plugin commits these files back:
- `CHANGELOG.md`
- `package.json`, `package-lock.json`
- `frontend/package.json`, `frontend/package-lock.json`
- `backend/package.json`, `backend/package-lock.json`

## GitHub Token

The workflow uses `secrets.GH_TOKEN` (not the default `GITHUB_TOKEN`) to allow semantic-release to push commits back to master. This must be a Personal Access Token with `repo` scope.

## Troubleshooting

| Issue | Fix |
|-------|-----|
| "No workspaces found" | Use `--prefix` instead of `--workspaces` in prepareCmd |
| "Version not changed" | Add `--allow-same-version` flag |
| No release created | Ensure commits use conventional format (`type: msg`) |
| "Not allowed to push" | Check `GH_TOKEN` secret has `repo` scope |
| Tests fail | Fix tests before release — `test` job must pass first |

## DO NOT Use `bumpp`

The project previously used `bumpp` for manual version bumping. **Do not use `bumpp`** — it conflicts with semantic-release by creating tags that semantic-release doesn't expect. Let semantic-release handle all versioning.
