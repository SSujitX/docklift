# Docklift: one image (docklift-backend) serving the API, the WebSocket
# terminal and the embedded dashboard on PORT (8000).
#
#   docker build -t docklift-backend .
#   docker build --build-arg VERSION=3.0.1 -t docklift-backend .
#
# VERSION defaults to the root VERSION file, the single source of the release
# version (the Release workflow bumps it). It stamps the Go binary and the
# dashboard's version label.
#
# The frontend and Go stages run on the build platform (the output is
# architecture-independent JS, and Go cross-compiles), so multi-arch builds
# (linux/amd64, linux/arm64) emulate nothing but the final apk/railpack step.

# --- 1. Dashboard (React + Vite) ---------------------------------------------
FROM --platform=$BUILDPLATFORM oven/bun:1.4.5-alpine AS web
ARG VERSION=
WORKDIR /src/frontend
COPY frontend/package.json frontend/bun.lock ./
RUN --mount=type=cache,target=/root/.bun/install/cache \
    bun install --frozen-lockfile
COPY frontend/ ./
# vite.config.ts reads ../VERSION unless DOCKLIFT_APP_VERSION is set.
COPY VERSION /src/VERSION
# Same-origin API and WebSocket: the Go server serves both.
ENV VITE_API_URL=
# Vite only: CI typechecks (tsc), so the image build skips it and stays light
# on 1 GB servers.
RUN DOCKLIFT_APP_VERSION="$VERSION" bunx vite build

# --- 2. Go server --------------------------------------------------------------
FROM --platform=$BUILDPLATFORM golang:1.27.4-alpine3.24 AS server
ARG TARGETOS
ARG TARGETARCH
ARG VERSION=
# Go GC target for the compile. Empty = 50 on hosts with under 2 GB of RAM
# (compiling modernc.org/sqlite needs ~800 MB at the default 100, ~600 MB at
# 50, for a few percent more CPU), Go's default otherwise.
ARG GO_GOGC=
ENV CGO_ENABLED=0 GOTOOLCHAIN=local GOFLAGS=-mod=readonly
WORKDIR /src/server
COPY server/go.mod server/go.sum ./
RUN --mount=type=cache,target=/go/pkg/mod \
    go mod download
COPY server/ ./
COPY VERSION /src/VERSION
# go:embed picks up the real build instead of the tracked placeholder.
RUN rm -rf web/dist
COPY --from=web /src/frontend/dist ./web/dist
RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    set -eu; \
    v="${VERSION:-$(tr -d '[:space:]' < /src/VERSION)}"; \
    [ -n "$v" ] || { echo "cannot determine version (root VERSION file)" >&2; exit 1; }; \
    gogc="$GO_GOGC"; \
    mem_kb=$(awk '/^MemTotal:/ { print $2 }' /proc/meminfo 2>/dev/null || echo 0); \
    if [ -z "$gogc" ] && [ "${mem_kb:-0}" -gt 0 ] && [ "$mem_kb" -lt 1945600 ]; then gogc=50; fi; \
    if [ -n "$gogc" ]; then export GOGC="$gogc"; echo "GOGC=$gogc (host RAM ${mem_kb} kB)"; fi; \
    GOOS="$TARGETOS" GOARCH="$TARGETARCH" go build -trimpath \
      -ldflags "-s -w -X docklift/internal/version.Version=${v}" \
      -o /out/docklift ./cmd/docklift; \
    echo "built docklift ${v} for ${TARGETOS}/${TARGETARCH}"

# --- 3. Runtime ----------------------------------------------------------------
FROM alpine:3.24.4
ARG TARGETARCH
ARG RAILPACK_VERSION=0.33.0
ARG RAILPACK_SHA256_AMD64=e62b8abaf1349aaa92ebe9d552ffd023ea06511b73b1197e6c32db26f483f511
ARG RAILPACK_SHA256_ARM64=7315e9a8d2f95f5faec1cd592a1b4d54be9ee2f855c600f58a90e41bd349488e

# Runtime tools the server shells out to: docker CLI with the buildx and
# compose plugins (builds, compose up/down), git (clone/pull), bash + procps
# (terminal, ps/top), util-linux (nsenter into host PID 1 for system update,
# reboot and upgrade; script), CA roots and time zones.
RUN apk add --no-cache ca-certificates tzdata docker-cli docker-cli-buildx docker-cli-compose \
      git bash procps util-linux

# Pinned automatic application builder (musl build for Alpine), checksum-verified.
RUN set -eu; \
    case "$TARGETARCH" in \
      amd64) arch=x86_64; sum="$RAILPACK_SHA256_AMD64" ;; \
      arm64) arch=arm64;  sum="$RAILPACK_SHA256_ARM64" ;; \
      *) echo "Unsupported Railpack architecture: $TARGETARCH" >&2; exit 1 ;; \
    esac; \
    tgz="railpack-v${RAILPACK_VERSION}-${arch}-unknown-linux-musl.tar.gz"; \
    wget -qO "/tmp/$tgz" "https://github.com/railwayapp/railpack/releases/download/v${RAILPACK_VERSION}/$tgz"; \
    echo "$sum  /tmp/$tgz" | sha256sum -c -; \
    tar -xzf "/tmp/$tgz" -C /usr/local/bin railpack; \
    rm -f "/tmp/$tgz"; \
    railpack --version

COPY --from=server /out/docklift /usr/local/bin/docklift

# The working directory is where .env/.env.local and the default
# ./github-app.pem are looked up.
WORKDIR /app
RUN mkdir -p /app/data /deployments /nginx-conf

ENV PORT=8000 \
    DATABASE_URL=file:/app/data/docklift.db

# Runs as root on purpose: the server drives the host Docker daemon through
# /var/run/docker.sock and uses nsenter into host PID 1 (privileged, pid: host
# in docker-compose.yml). A non-root user could do neither.
USER root

EXPOSE 8000

HEALTHCHECK --interval=30s --timeout=5s --start-period=60s --retries=3 \
    CMD wget -q -O /dev/null "http://127.0.0.1:${PORT:-8000}/api/health" || exit 1

# serve applies the database migrations before listening.
ENTRYPOINT ["/usr/local/bin/docklift"]
CMD ["serve"]
