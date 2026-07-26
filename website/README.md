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

Output: `.vitepress/dist` (deployed by `.github/workflows/docs.yml` to GitHub Pages).

## Custom domain

`public/CNAME` is set to `docklift.dev`. In the GitHub repo:

1. **Settings → Pages → Build and deployment** → Source: **GitHub Actions**
2. DNS for `docklift.dev`: A/AAAA records for GitHub Pages, or CNAME to `ssujitx.github.io` as GitHub documents

After the first successful **Docs** workflow run, the site is live.
