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
data loader.

**Docs** workflow (`.github/workflows/docs.yml`) deploys Pages when:

- `website/**` or `CHANGELOG.md` is pushed, or
- **Release** finishes and calls Docs via `workflow_call` (preferred after a version bump —
  release commits use `[skip ci]`, so a push alone would not rebuild the site)

Output: `.vitepress/dist`.

## Custom domain

`public/CNAME` is set to `docklift.dev`. In the GitHub repo:

1. **Settings → Pages → Build and deployment** → Source: **GitHub Actions**
2. DNS for `docklift.dev`: A/AAAA records for GitHub Pages, or CNAME to `ssujitx.github.io` as GitHub documents

After the first successful **Docs** workflow run, the site is live.
