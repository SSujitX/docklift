# File Management

Browse and edit project source files directly in the browser after a project has been created and source material is on disk under `deployments/<project-id>/`.

## Features

- Tree view of project files
- Monaco-based code editor (VS Code engine)
- Syntax highlighting for many languages
- Edit Dockerfiles, config files, and application source
- Changes save to disk on the server

## Workflow tips

1. Prefer committing important changes to Git and redeploying from GitHub so the panel is not the only copy of the truth
2. After editing a Dockerfile or build config, run **Deploy** / **Redeploy** so the new image is built
3. Runtime-only edits (for example a config file mounted via Storage) may only need a **Restart** if the process reloads that file — most apps still need a redeploy
4. ZIP-sourced projects are especially convenient to tweak in-browser before you wire GitHub

## Related API

| Method | Endpoint | Description |
|--------|----------|-------------|
| `GET` | `/api/files/:projectId` | List tree |
| `GET` | `/api/files/:projectId/content` | Read file contents |
| `PUT` | `/api/files/:projectId/content` | Write file contents |

## Limits

File editing is for the project source tree Docklift manages. It is not a full remote IDE substitute, and it does not replace [persistent Storage](./environment.md) mounts for data that must survive container replacement.
