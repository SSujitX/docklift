#!/bin/bash
set -euo pipefail

# ╔════════════════════════════════════════════════════════════════════════════╗
# ║                         DOCKLIFT UPGRADE SCRIPT                             ║
# ║  Safely upgrades Docklift while preserving all data and user containers     ║
# ╚════════════════════════════════════════════════════════════════════════════╝
#
# Also migrates a 2.1.x install (backend + frontend + nginx gateway containers)
# to the single Go backend container: the obsolete docklift-frontend and
# docklift-nginx containers and the docklift-frontend image are removed once
# the new backend is healthy.
#
#   curl -fsSL https://raw.githubusercontent.com/SSujitX/docklift/master/upgrade.sh | sudo bash
#   ... | sudo bash -s -- v=3.0.0          # pin a release instead of the latest
#
# Environment:
#   DOCKLIFT_VERSION      release to upgrade to (default: latest GitHub release)
#   DOCKLIFT_INSTALL_DIR  install location (default /opt/docklift)
#   DOCKLIFT_CI_LOCAL     "true": upgrade to the commit checked out in the current
#                         directory (a git clone) instead of GitHub (CI and testing)

# git checkout replaces this file while it runs; bash reads scripts
# incrementally, so run from a private copy when started from a file.
if [ -z "${DOCKLIFT_UPGRADE_COPY:-}" ] && [ -f "${BASH_SOURCE[0]:-}" ]; then
    DOCKLIFT_UPGRADE_COPY=$(mktemp /tmp/docklift-upgrade.XXXXXX)
    cp "${BASH_SOURCE[0]}" "$DOCKLIFT_UPGRADE_COPY"
    export DOCKLIFT_UPGRADE_COPY
    exec bash "$DOCKLIFT_UPGRADE_COPY" "$@"
fi
# Keep the terminal reachable from traps that fire inside log-redirected blocks
exec 3>&1 4>&2

# Colors & Vars
RED='\033[0;31m'; GREEN='\033[0;32m'; CYAN='\033[0;36m'; YELLOW='\033[0;33m'
BOLD='\033[1m'; DIM='\033[2m'; NC='\033[0m'
INSTALL_DIR="${DOCKLIFT_INSTALL_DIR:-/opt/docklift}"
SOURCE_DIR="$PWD"
START_TIME=$(date +%s)
ROLLBACK_REF=""
PRE_BACKEND_TAG="docklift-backend:pre-upgrade"
PRE_FRONTEND_TAG="docklift-frontend:pre-upgrade"
# Containers of the 2.1.x layout that the Go backend replaces.
OBSOLETE_CONTAINERS=(docklift-frontend docklift-nginx)
ROLLBACK_ARMED=0
IN_ROLLBACK=0
BACKUP_FILE=""
CONF_SNAPSHOT=""
LOG=""
REQUESTED_VERSION="${DOCKLIFT_VERSION:-}"
HEALTH_TIMEOUT=180

format_time() {
    local s=$1 h=0 m=0
    # `((…))` returns exit 1 when the value is 0 — must not trip `set -e`
    ((h = s / 3600, m = (s % 3600) / 60, s = s % 60)) || true
    if [ "$h" -gt 0 ]; then printf "%dh %dm %ds" "$h" "$m" "$s"
    elif [ "$m" -gt 0 ]; then printf "%dm %ds" "$m" "$s"
    else printf "%ds" "$s"
    fi
}

read_version() {
    # Root package.json carries the release version in every layout (2.1.x and Go).
    local v
    v=$(grep -o '"version": *"[^"]*"' "$INSTALL_DIR/package.json" 2>/dev/null | head -1 | cut -d'"' -f4 || true)
    printf '%s' "${v:-unknown}"
}

health_ok() {
    # Probe the API inside the backend container: busybox wget exists in both the
    # Go image and the 2.1.x Node image; node fetch is the 2.1.x fallback.
    docker exec docklift-backend wget -q -O /dev/null http://127.0.0.1:8000/api/health >/dev/null 2>&1 && return 0
    docker exec docklift-backend node -e \
      "fetch('http://127.0.0.1:8000/api/health').then(r=>process.exit(r.ok?0:1)).catch(()=>process.exit(1))" \
      >/dev/null 2>&1
}

# Every service of the compose file currently checked out must be running.
stack_running() {
    local want have svc
    want=$(docker compose config --services 2>>"$LOG") || return 1
    have=$(docker compose ps --status running --services 2>>"$LOG") || return 1
    for svc in $want; do
        printf '%s\n' "$have" | grep -qx "$svc" || return 1
    done
}

wait_healthy() {
    local deadline=$(( $(date +%s) + HEALTH_TIMEOUT ))
    while [ "$(date +%s)" -lt "$deadline" ]; do
        if health_ok && stack_running; then
            return 0
        fi
        sleep 3
    done
    return 1
}

sqlite_backup() {
    local src=$1 dest=$2
    # Prefer SQLite online backup API when sqlite3 CLI is present
    if command -v sqlite3 >/dev/null 2>&1; then
        sqlite3 "$src" ".backup '$dest'"
        return $?
    fi
    # Fallback: file copy only when backend is stopped (no open writers). The Go
    # backend uses WAL; a stopped server has checkpointed, but keep any -wal too.
    cp "$src" "$dest" || return 1
    if [ -s "$src-wal" ]; then
        cp "$src-wal" "$dest-wal" || return 1
    fi
}

restore_database() {
    local db="$INSTALL_DIR/data/docklift.db"
    cp "$BACKUP_FILE" "$db"
    # -wal/-shm left by the new backend belong to the newer database state.
    rm -f "$db-wal" "$db-shm"
    if [ -f "$BACKUP_FILE-wal" ]; then
        cp "$BACKUP_FILE-wal" "$db-wal"
    fi
}

tag_running_images() {
    # Preserve current images so rollback does not depend on rebuilding an old git ref
    if docker image inspect docklift-backend >/dev/null 2>&1; then
        docker tag docklift-backend "$PRE_BACKEND_TAG" 2>/dev/null || true
    fi
    if docker image inspect docklift-frontend >/dev/null 2>&1; then
        docker tag docklift-frontend "$PRE_FRONTEND_TAG" 2>/dev/null || true
    else
        # A stale tag from an older upgrade must not be restored on rollback
        docker rmi "$PRE_FRONTEND_TAG" 2>/dev/null || true
    fi
}

remove_obsolete_containers() {
    local c
    for c in "${OBSOLETE_CONTAINERS[@]}"; do
        if docker container inspect "$c" >/dev/null 2>&1; then
            docker rm -f "$c" || return 1
        fi
    done
}

print_manual_rollback() {
    echo -e "  ${BOLD}Manual rollback:${NC}"
    echo -e "    cd $INSTALL_DIR"
    echo -e "    docker compose stop"
    [ -n "$ROLLBACK_REF" ] && echo -e "    git checkout -f $ROLLBACK_REF"
    if [ -n "${BACKUP_FILE:-}" ]; then
        echo -e "    cp $BACKUP_FILE data/docklift.db && rm -f data/docklift.db-wal data/docklift.db-shm"
    fi
    [ -n "${CONF_SNAPSHOT:-}" ] && echo -e "    tar -xzf $CONF_SNAPSHOT -C $INSTALL_DIR/nginx-proxy"
    echo -e "    docker tag $PRE_BACKEND_TAG docklift-backend"
    echo -e "    docker image inspect $PRE_FRONTEND_TAG >/dev/null 2>&1 && docker tag $PRE_FRONTEND_TAG docklift-frontend"
    echo -e "    docker compose up -d --remove-orphans"
}

rollback_upgrade() {
    # Prevent re-entrancy from ERR trap while we are already rolling back
    if [ "$IN_ROLLBACK" = "1" ]; then
        return 0
    fi
    IN_ROLLBACK=1
    ROLLBACK_ARMED=0
    trap - ERR INT TERM
    set +e
    # The failure may have happened inside a block redirected to the log:
    # report on the terminal the script started with.
    exec 1>&3 2>&4

    echo -e "\n  ${RED}${BOLD}Upgrade failed — last lines of $LOG:${NC}"
    tail -n 12 "$LOG" 2>/dev/null | sed 's/^/    /'
    echo -e "\n  ${RED}${BOLD}Rolling back upgrade…${NC}"
    cd "$INSTALL_DIR"
    {
        echo "--- rollback started at $(date) ---"
        docker compose stop
        docker stop docklift-backend
    } >>"$LOG" 2>&1

    if [ -n "${ROLLBACK_REF}" ]; then
        { git checkout "${ROLLBACK_REF}" -q || git checkout -f "${ROLLBACK_REF}" -q; } >>"$LOG" 2>&1
    fi

    if [ -n "${BACKUP_FILE:-}" ] && [ -f "$BACKUP_FILE" ]; then
        # Backend must be stopped before overwriting SQLite
        restore_database && echo -e "  ${YELLOW}Database restored from backup${NC}"
    fi

    # The new backend rewrites panel-domain confs at boot; put the old ones back
    if [ -n "${CONF_SNAPSHOT:-}" ] && [ -f "$CONF_SNAPSHOT" ]; then
        tar -xzf "$CONF_SNAPSHOT" -C "$INSTALL_DIR/nginx-proxy" >>"$LOG" 2>&1 \
          && echo -e "  ${YELLOW}nginx-proxy configs restored${NC}"
    fi

    # Prefer tagged pre-upgrade images (reliable) over rebuild-from-git
    if docker image inspect "$PRE_BACKEND_TAG" >/dev/null 2>&1; then
        docker tag "$PRE_BACKEND_TAG" docklift-backend >>"$LOG" 2>&1
    fi
    if docker image inspect "$PRE_FRONTEND_TAG" >/dev/null 2>&1; then
        docker tag "$PRE_FRONTEND_TAG" docklift-frontend >>"$LOG" 2>&1
    fi

    { docker compose up -d --remove-orphans || docker compose up -d --build --remove-orphans; } >>"$LOG" 2>&1

    if wait_healthy; then
        echo -e "  ${YELLOW}Rolled back to the previous version — it is running and healthy.${NC}"
        echo -e "  ${DIM}Details: $LOG${NC}"
    else
        echo -e "  ${RED}Rollback did not bring the previous version back to health.${NC}"
        echo -e "  ${DIM}Details: $LOG${NC}"
        print_manual_rollback
    fi
}

# Any exit while armed rolls back: an ERR in the main body, an errexit inside a
# function (where the ERR trap does not reach), or a signal.
on_exit() {
    local ec=$?
    if [ "${ROLLBACK_ARMED:-0}" = "1" ] && [ "$ec" -ne 0 ]; then
        rollback_upgrade
    fi
    if [ -n "${DOCKLIFT_UPGRADE_COPY:-}" ]; then
        rm -f "$DOCKLIFT_UPGRADE_COPY"
    fi
}
trap on_exit EXIT

arm_rollback() {
    ROLLBACK_ARMED=1
    trap 'if [ "${ROLLBACK_ARMED:-0}" = "1" ]; then rollback_upgrade; fi; exit 1' ERR INT TERM
}

disarm_rollback() {
    ROLLBACK_ARMED=0
    trap - ERR INT TERM
}

# Normalize to GitHub tag form (vX.Y.Z). Empty means resolve latest release.
normalize_release_tag() {
    local raw="$1"
    raw=$(printf '%s' "$raw" | tr -d '[:space:]')
    case "$raw" in
        ""|latest|LATEST) printf '' ;;
        v*) printf '%s' "$raw" ;;
        *) printf 'v%s' "$raw" ;;
    esac
}

while [ $# -gt 0 ]; do
    case "$1" in
        v=*|version=*|--version=*) REQUESTED_VERSION="${1#*=}" ;;
        --version|-v) shift; REQUESTED_VERSION="${1:-}" ;;
        latest|LATEST) REQUESTED_VERSION="" ;;
        v[0-9]*|[0-9]*.[0-9]*) REQUESTED_VERSION="$1" ;;
        *)
            echo -e "  ${RED}Error: unknown upgrade arg: $1${NC}"
            echo -e "  ${DIM}Use: bash -s -- v=3.0.0   or   DOCKLIFT_VERSION=3.0.0${NC}"
            exit 1
            ;;
    esac
    shift || true
done
TARGET_TAG=$(normalize_release_tag "$REQUESTED_VERSION")

# Header
clear 2>/dev/null || true
echo -e "\n  ${CYAN}____             _    _ _  __ _   ${NC}"
echo -e "  ${CYAN}|  _ \\  ___   ___| | _| (_)/ _| |_ ${NC}\n  ${CYAN}| | | |/ _ \\ / __| |/ / | | |_| __|${NC}"
echo -e "  ${CYAN}| |_| | (_) | (__|   <| | |  _| |_ ${NC}\n  ${CYAN}|____/ \\___/ \\___|_|\\_\\_|_|_|  \\__|${NC}\n"
echo -e "  ${DIM}Self-Hosted Docker Deployment Platform${NC}"
echo -e "  ${YELLOW}${BOLD}⬆ UPGRADE MODE${NC}\n"

# Pre-flight checks
[ "$EUID" -ne 0 ] && echo -e "  ${RED}Error: Run with sudo${NC}" && exit 1

if [ ! -d "$INSTALL_DIR" ] || [ ! -f "$INSTALL_DIR/docker-compose.yml" ]; then
    echo -e "  ${RED}Error: Docklift not found at $INSTALL_DIR${NC}"
    echo -e "  ${DIM}Run the install script first: curl -fsSL https://raw.githubusercontent.com/SSujitX/docklift/master/install.sh | sudo bash${NC}"
    exit 1
fi
for cmd in docker git tar; do
    if ! command -v "$cmd" >/dev/null 2>&1; then
        echo -e "  ${RED}Error: $cmd is required${NC}"
        exit 1
    fi
done
if ! docker compose version >/dev/null 2>&1; then
    echo -e "  ${RED}Error: the Docker Compose plugin (docker compose) is required${NC}"
    exit 1
fi

cd "$INSTALL_DIR"
LOG="$INSTALL_DIR/upgrade.log"
echo "--- Upgrade started at $(date) ---" > "$LOG"

# Pin current git ref for source rollback
ROLLBACK_REF=$(git rev-parse HEAD 2>/dev/null || echo "")

# Get current version
OLD_VERSION=$(read_version)
echo -e "  ${BOLD}Current Version:${NC} ${CYAN}$OLD_VERSION${NC}"
echo -e "        ${DIM}➞ Git ref: ${ROLLBACK_REF:-unknown}${NC}"

# Resolve the target before taking anything offline
if [ "${DOCKLIFT_CI_LOCAL:-}" = "true" ]; then
    TARGET_TAG="local"
elif [ -z "$TARGET_TAG" ]; then
    TARGET_TAG=$(curl -fsS https://api.github.com/repos/SSujitX/docklift/releases/latest 2>>"$LOG" \
        | grep '"tag_name"' | head -1 | cut -d'"' -f4 || true)
    TARGET_TAG=$(printf '%s' "$TARGET_TAG" | tr -d '[:space:]')
fi
echo -e "        ${DIM}➞ Target: ${TARGET_TAG:-origin/master (no release found)}${NC}"
if [ -n "$TARGET_TAG" ] && [ "$TARGET_TAG" != "local" ] \
    && ! git ls-remote --exit-code --tags origin "refs/tags/${TARGET_TAG}" >>"$LOG" 2>&1; then
    echo -e "  ${RED}Error: release tag not found on origin: ${TARGET_TAG}${NC}"
    echo -e "  ${DIM}Nothing was changed. See https://github.com/SSujitX/docklift/releases${NC}"
    exit 1
fi

# Step 1: Prepare backup dir BEFORE stopping backend (avoid pre-trap outage gap)
echo -e "\n  ${CYAN}[1/6]${NC} Preparing rollback snapshot..."
BACKUP_DIR="$INSTALL_DIR/backups"
mkdir -p "$BACKUP_DIR"
# Prove the directory is writable before we take the API offline
STAMP=$(date +%Y%m%d_%H%M%S)
BACKUP_FILE="$BACKUP_DIR/docklift_${STAMP}.db.bak"
touch "$BACKUP_DIR/.write-test" && rm -f "$BACKUP_DIR/.write-test"

tag_running_images >>"$LOG" 2>&1 || true
echo -e "        ${DIM}➞ Tagged pre-upgrade images (if present)${NC}"

# Stop backend and prove it is not writing before any DB copy
if ! docker compose stop backend >>"$LOG" 2>&1; then
    echo -e "        ${RED}Failed to stop backend — aborting (no DB snapshot taken)${NC}"
    docker compose start backend >>"$LOG" 2>&1 || true
    exit 1
fi

# Backend run-state probe: 0=running, 1=stopped, 2=probe-error (indeterminate)
# Probe-error must NOT be treated as stopped — fail closed before any DB copy.
backend_run_state() {
    local out ec
    out=$(docker compose ps --status running --services 2>&1)
    ec=$?
    if [ "$ec" -ne 0 ]; then
        echo "$out" >>"$LOG"
        return 2
    fi
    if printf '%s\n' "$out" | grep -qx 'backend'; then
        return 0
    fi

    out=$(docker ps -q --filter "name=docklift-backend" --filter "status=running" 2>&1)
    ec=$?
    if [ "$ec" -ne 0 ]; then
        echo "$out" >>"$LOG"
        return 2
    fi
    if [ -n "$(printf '%s' "$out" | tr -d '[:space:]')" ]; then
        return 0
    fi

    out=$(docker ps --format '{{.Names}}' 2>&1)
    ec=$?
    if [ "$ec" -ne 0 ]; then
        echo "$out" >>"$LOG"
        return 2
    fi
    if printf '%s\n' "$out" | grep -qx 'docklift-backend'; then
        return 0
    fi
    return 1
}

# Must capture under if/else — standalone `backend_run_state; BACKEND_STATE=$?`
# aborts under `set -e` on the normal stopped (1) and probe-error (2) returns.
if backend_run_state; then
    BACKEND_STATE=0
else
    BACKEND_STATE=$?
fi
if [ "$BACKEND_STATE" -eq 0 ]; then
    echo -e "        ${RED}Backend still running after stop — aborting (no DB snapshot taken)${NC}"
    docker compose start backend >>"$LOG" 2>&1 || true
    exit 1
fi
if [ "$BACKEND_STATE" -eq 2 ]; then
    echo -e "        ${RED}Could not verify backend stopped (Docker probe failed) — aborting${NC}"
    echo -e "        ${DIM}➞ No DB snapshot taken; attempting to restart backend${NC}"
    docker compose start backend >>"$LOG" 2>&1 || true
    exit 1
fi
if [ "$BACKEND_STATE" -ne 1 ]; then
    echo -e "        ${RED}Unexpected backend run-state ($BACKEND_STATE) — aborting${NC}"
    docker compose start backend >>"$LOG" 2>&1 || true
    exit 1
fi
echo -e "        ${DIM}➞ Backend stopped (verified)${NC}"

# Arm immediately after verified stop so any later failure restarts the stack
arm_rollback

if [ -f "$INSTALL_DIR/data/docklift.db" ]; then
    if sqlite_backup "$INSTALL_DIR/data/docklift.db" "$BACKUP_FILE"; then
        echo -e "        ${DIM}➞ DB snapshot: $BACKUP_FILE${NC}"
    else
        echo -e "        ${RED}Database backup failed${NC}"
        rollback_upgrade
        exit 1
    fi
else
    echo -e "        ${YELLOW}No existing database found (fresh install?)${NC}"
    BACKUP_FILE=""
fi

# The new backend rewrites panel-domain confs at boot
# (docklift-nginx:80 → docklift-backend.docklift_network:8000)
if [ -d "$INSTALL_DIR/nginx-proxy/conf.d" ]; then
    CONF_SNAPSHOT="$BACKUP_DIR/nginx-conf_${STAMP}.tar.gz"
    tar -czf "$CONF_SNAPSHOT" -C "$INSTALL_DIR/nginx-proxy" conf.d
    echo -e "        ${DIM}➞ nginx-proxy conf snapshot: $CONF_SNAPSHOT${NC}"
fi
echo -e "        ${GREEN}done${NC}"

# Step 2: Fetch the new code
FETCH_ST=$(date +%s)
echo -ne "  ${CYAN}[2/6]${NC} Fetching ${TARGET_TAG:-latest master}..."
{
    if [ "$TARGET_TAG" = "local" ]; then
        # The commit checked out in the current directory (a git clone of the
        # new code), checked out exactly as a release tag would be
        if [ "$SOURCE_DIR" != "$INSTALL_DIR" ]; then
            git -c safe.directory='*' fetch -q "$SOURCE_DIR" HEAD
            git checkout -q FETCH_HEAD
        fi
    else
        git fetch origin --tags -q
        if [ -n "$TARGET_TAG" ]; then
            git checkout "$TARGET_TAG" -q 2>/dev/null || git checkout "tags/$TARGET_TAG" -q
        else
            git fetch origin master -q && git reset --hard origin/master -q
        fi
    fi
} >>"$LOG" 2>&1
echo -e " ${GREEN}done${NC} ${DIM}($(format_time $(($(date +%s) - FETCH_ST))))${NC}"

NEW_VERSION=$(read_version)
if [ "$OLD_VERSION" = "$NEW_VERSION" ]; then
    echo -e "        ${DIM}➞ Already on version: $NEW_VERSION (rebuilding)${NC}"
else
    echo -e "        ${DIM}➞ Upgrading: $OLD_VERSION → ${GREEN}$NEW_VERSION${NC}"
fi

mkdir -p \
  "$INSTALL_DIR/data" \
  "$INSTALL_DIR/deployments" \
  "$INSTALL_DIR/backups" \
  "$INSTALL_DIR/nginx-proxy/conf.d" \
  "$INSTALL_DIR/nginx-proxy/snippets" \
  "$INSTALL_DIR/nginx-proxy/certbot/www" \
  "$INSTALL_DIR/nginx-proxy/certbot/conf"

# Step 3: Build while nginx-proxy keeps serving user app domains
BUILD_ST=$(date +%s)
echo -e "\n  ${CYAN}[3/6]${NC} Building Docklift..."
echo -e "        ${DIM}This may take a few minutes...${NC}"
if ! docker compose build >>"$LOG" 2>&1; then
    echo -e "        ${RED}Build failed${NC}"
    rollback_upgrade
    exit 1
fi
# The proxy and certbot images are pinned in docker-compose.yml; fetch the
# pinned tags now so the container swap below never waits on (or fails at) a pull.
if ! docker compose pull nginx-proxy certbot >>"$LOG" 2>&1; then
    echo -e "        ${RED}Pulling nginx-proxy/certbot images failed${NC}"
    rollback_upgrade
    exit 1
fi
echo -e "        ${GREEN}done${NC} ${DIM}($(format_time $(($(date +%s) - BUILD_ST))))${NC}"

# Step 4: Swap containers (user project containers are never touched)
echo -ne "  ${CYAN}[4/6]${NC} Restarting Docklift containers..."
{
    # 2.1.x gateway and frontend: replaced by the backend, which now owns :8080
    remove_obsolete_containers
    docker compose up -d --remove-orphans
} >>"$LOG" 2>&1
echo -e " ${GREEN}done${NC}"
echo -e "        ${DIM}➞ User project containers are untouched${NC}"

# Step 5: Health check with retries
echo -ne "  ${CYAN}[5/6]${NC} Verifying health..."
if wait_healthy; then
    echo -e " ${GREEN}all systems operational${NC}"
    disarm_rollback
else
    echo -e " ${RED}health check failed${NC}"
    echo -e "        ${DIM}➞ backend did not answer /api/health with every service running within ${HEALTH_TIMEOUT}s${NC}"
    docker compose ps >>"$LOG" 2>&1 || true
    docker logs --tail 100 docklift-backend >>"$LOG" 2>&1 || true
    rollback_upgrade
    exit 1
fi

# Step 6: Remove what the new layout no longer uses
echo -e "\n  ${CYAN}[6/6]${NC} Finalizing..."
if ! docker compose config --services 2>>"$LOG" | grep -qx frontend; then
    for img in docklift-frontend "$PRE_FRONTEND_TAG"; do
        if docker image inspect "$img" >/dev/null 2>&1 && docker rmi "$img" >>"$LOG" 2>&1; then
            echo -e "        ${DIM}➞ Removed obsolete image $img${NC}"
        fi
    done
fi
echo -e "  ╔══════════════════════════════════════════════════════════════╗"
echo -e "  ║  ${GREEN}${BOLD}✓ UPGRADE COMPLETE${NC}                                        ║"
echo -e "  ╚══════════════════════════════════════════════════════════════╝"
TOTAL_TIME=$(($(date +%s) - START_TIME))
echo -e "  ${DIM}Time: $(format_time $TOTAL_TIME) | Version: $NEW_VERSION${NC}\n"

echo -e "  ${BOLD}✓ Preserved:${NC}"
echo -e "    ${GREEN}•${NC} Database (projects, settings, deployments)"
echo -e "    ${GREEN}•${NC} All user containers (dl_* containers)"
echo -e "    ${GREEN}•${NC} Nginx / panel domain configs"
echo -e "    ${GREEN}•${NC} Let's Encrypt certificates (certbot/)"
echo -e "    ${GREEN}•${NC} Project files in /deployments"
echo -e "    ${GREEN}•${NC} Pre-upgrade backend image tagged ${PRE_BACKEND_TAG}"
echo -e ""

BACKUP_COUNT=$(find "$BACKUP_DIR" -maxdepth 1 -name '*.db.bak' 2>/dev/null | wc -l | tr -d ' ')
echo -e "  ${DIM}Backups stored: $BACKUP_COUNT files in $BACKUP_DIR${NC}"
if [ -n "${BACKUP_FILE}" ]; then
    echo -e "  ${DIM}DB snapshot: $BACKUP_FILE${NC}\n"
fi

PUB4=$(curl -4 -s --connect-timeout 2 https://api.ipify.org 2>/dev/null || echo "")
if [ -n "$PUB4" ]; then
    echo -e "  ${BOLD}Access Docklift:${NC} http://${PUB4}:8080"
else
    echo -e "  ${BOLD}Access Docklift:${NC} http://SERVER_IP:8080"
fi
echo -e "  ${DIM}(Optional: HTTPS panel domain in Settings, or DASHBOARD_BIND=127.0.0.1)${NC}\n"
