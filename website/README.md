# Docklift docs site

Public documentation for [docklift.dev](https://docklift.dev), built with [VitePress](https://vitepress.dev).

## Local

```bash
cd website
bun install
bun run dev
```

## Build

```bash
bun run build
```

`predev` / `prebuild` run `scripts/sync-changelog.mjs`, which copies the repo-root
`CHANGELOG.md` into `website/changelog.md` (gitignored — do not edit by hand).
The homepage `<ChangelogPreview />` also loads that same root file via a VitePress
data loader. The Docs workflow redeploys on `website/**` / `CHANGELOG.md` pushes and
after a successful **Release & Test** run (`workflow_run`, because release commits
include `[skip ci]`). On `workflow_run`, checkout uses the **branch tip** so the
post-release `CHANGELOG.md` is included (not `head_sha`, which is pre-bump).

Output: `.vitepress/dist` (deployed by `.github/workflows/docs.yml` to GitHub Pages).

## Custom domain

`public/CNAME` is set to `docklift.dev`. In the GitHub repo:

1. **Settings → Pages → Build and deployment** → Source: **GitHub Actions**
2. DNS for `docklift.dev`: A/AAAA records for GitHub Pages, or CNAME to `ssujitx.github.io` as GitHub documents

After the first successful **Docs** workflow run, the site is live.
