# Docklift developer targets; they mirror .github/workflows/ci.yml.
#
#   make build      dashboard + Go binary with the dashboard embedded -> bin/docklift
#   make test       go vet + go test (server), go vet (contract suite)
#   make contract   contract suite against the Go binary
#   make frontend   bun install, typecheck, vite build, audit
#   make lint       gofmt, go vet, bash -n + shellcheck, actionlint, compose config
#   make vuln       govulncheck
#   make docker     docker build -t docklift-backend .
#   make dev        Go API on :8000 + Vite dev server on :3600 (proxying /api and /ws)
#
# Docker-gated tests: DOCKLIFT_DOCKER=1 make test contract (needs a Docker daemon).
# Needs bash, Go, Bun; lint also uses shellcheck and actionlint when installed.

SHELL := bash
.SHELLFLAGS := -eu -o pipefail -c
.DEFAULT_GOAL := build

# Root package.json is the single source of the release version.
VERSION ?= $(shell sed -n 's/^[[:space:]]*"version":[[:space:]]*"\([^"]*\)".*/\1/p' package.json | head -n 1)
GOVULNCHECK_VERSION ?= v1.9.0
IMAGE ?= docklift-backend

ifeq ($(OS),Windows_NT)
EXE := .exe
endif
BIN := $(CURDIR)/bin/docklift$(EXE)
GO_LDFLAGS := -s -w -X docklift/internal/version.Version=$(VERSION)
SHELL_SCRIPTS := install.sh install-dev.sh upgrade.sh uninstall.sh $(wildcard scripts/*.sh)

.PHONY: build server frontend test contract lint vuln docker dev clean

## build: dashboard + server; the dashboard is embedded only for this build
## (server/web/dist gets its tracked placeholder back afterwards).
build: frontend
	mkdir -p bin && cp server/web/dist/index.html bin/.web-placeholder.html
	cp -R frontend/dist/. server/web/dist/
	status=0; (cd server && CGO_ENABLED=0 go build -trimpath -ldflags "$(GO_LDFLAGS)" -o "$(BIN)" ./cmd/docklift) || status=$$?; 	  find server/web/dist -mindepth 1 -delete; 	  mv bin/.web-placeholder.html server/web/dist/index.html; 	  exit $$status
	@echo "built $(BIN) ($(VERSION))"

## server: Go binary with the placeholder dashboard (enough for API work and the contract suite)
server:
	cd server && CGO_ENABLED=0 go build -trimpath -ldflags "$(GO_LDFLAGS)" -o "$(BIN)" ./cmd/docklift

frontend:
	cd frontend && bun install --frozen-lockfile
	cd frontend && bunx tsc -p tsconfig.app.json --noEmit
	cd frontend && bunx vite build
	cd frontend && bun audit --prod

test:
	cd server && go vet ./...
	cd server && go test -count=1 ./...
	cd tests/contract && go vet ./...

contract: server
	cd tests/contract && DOCKLIFT_GO_BIN="$(BIN)" go test -count=1 ./...

lint:
	@unformatted=$$(gofmt -l server tests/contract); \
	  if [ -n "$$unformatted" ]; then echo "gofmt needed:"; echo "$$unformatted"; exit 1; fi
	cd server && go vet ./...
	cd tests/contract && go vet ./...
	for f in $(SHELL_SCRIPTS); do bash -n "$$f"; done
	@if command -v shellcheck >/dev/null; then shellcheck $(SHELL_SCRIPTS); else echo "shellcheck not installed; skipped"; fi
	@if command -v actionlint >/dev/null; then actionlint; else echo "actionlint not installed; skipped"; fi
	bash scripts/test-upgrade-backend-run-state.sh
	@if docker compose version >/dev/null 2>&1; then docker compose -f docker-compose.yml config --quiet; else echo "docker compose not installed; compose check skipped"; fi

vuln:
	cd server && go run "golang.org/x/vuln/cmd/govulncheck@$(GOVULNCHECK_VERSION)" ./...

docker:
	docker build --build-arg VERSION=$(VERSION) -t $(IMAGE) .

## dev: local state lives in ./data, ./deployments, ./backups and ./nginx-proxy
## (all gitignored); Docker features need a reachable daemon (DOCKER_HOST).
dev:
	mkdir -p data deployments backups nginx-proxy/conf.d nginx-proxy/certbot/conf
	trap 'kill 0' EXIT INT TERM; \
	  (cd frontend && bun install --frozen-lockfile && bun run dev) & \
	  cd server && \
	  DATA_PATH="$(CURDIR)/data" DATABASE_URL="file:$(CURDIR)/data/docklift.db" \
	  DEPLOYMENTS_PATH="$(CURDIR)/deployments" BACKUP_PATH="$(CURDIR)/backups" \
	  NGINX_CONF_PATH="$(CURDIR)/nginx-proxy/conf.d" LETSENCRYPT_PATH="$(CURDIR)/nginx-proxy/certbot/conf" \
	  DOCKLIFT_FRONTEND_URL=http://127.0.0.1:3600 \
	  go run -ldflags "-X docklift/internal/version.Version=$(VERSION)" ./cmd/docklift serve

clean:
	if [ -f bin/.web-placeholder.html ]; then 	  find server/web/dist -mindepth 1 -delete && mv bin/.web-placeholder.html server/web/dist/index.html; fi
	rm -rf bin frontend/dist
