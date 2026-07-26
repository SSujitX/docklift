# Dockerfile & Railpack Builds

Every project has a build mode. You do **not** always need a Dockerfile — **Auto** and **Railpack** can build from language manifests (for example `package.json`, `requirements.txt`, `pyproject.toml`).

## Build modes

| Mode | Behaviour |
|------|-----------|
| **Auto** (default) | Use a repository `Dockerfile` if present; otherwise Railpack |
| **Dockerfile** | Always your Dockerfile — fails loudly if missing |
| **Railpack** | Always Railpack, even if a Dockerfile exists |

**Railpack** (the builder behind Railway) detects Node, Python, Go, PHP, Ruby, Java, Rust, and more, then builds with BuildKit.

Extra settings:

- **Base directory** — monorepo subdirectory to build from
- **Dockerfile path** — non-root Dockerfile location

## Dockerfile tips

When you do ship a Dockerfile, include an `EXPOSE` directive so Docklift can detect the internal listen port for proxying and optional host-port mapping.

### Node.js example

```dockerfile
FROM node:20-alpine
WORKDIR /app
COPY package*.json ./
RUN npm install
COPY . .
EXPOSE 3000
CMD ["npm", "start"]
```

### Python Flask example

```dockerfile
FROM python:3.11-slim
WORKDIR /app
COPY requirements.txt .
RUN pip install -r requirements.txt
COPY . .
EXPOSE 5000
CMD ["python", "app.py"]
```

## Ports and networking

- The app must listen on `0.0.0.0` inside the container (not only `127.0.0.1`)
- Internal port comes from `EXPOSE`, project settings, or Railpack detection
- Public access is preferably via a [custom domain](./domains.md) on nginx-proxy
- Host ports (`5500`–`5600`) are opt-in — see [Port Management](./ports.md)

## Build-time secrets

Mark env vars as **build** when the image needs them at build time.

- Plain build vars map to Docker `ARG` / `--build-arg` when declared
- **BuildKit secret** (with build): passed as a Docker secret, not `--build-arg`. In the Dockerfile:

```dockerfile
RUN --mount=type=secret,id=MY_SECRET \
  MY_SECRET="$(cat /run/secrets/MY_SECRET)" && ./build.sh
```

Avoid baking runtime credentials into layers. Prefer runtime env injection — see [Environment Variables](./environment.md).

## Multi-Dockerfile layouts

```text
my-project/
├── api/Dockerfile
├── frontend/Dockerfile
└── worker/Dockerfile
```

Each Dockerfile becomes a service. Deploy still runs at the project level. Details: [Deployment](./deployment.md).
