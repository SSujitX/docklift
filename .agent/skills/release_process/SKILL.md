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
| `*force patch*` in subject | patch |
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
