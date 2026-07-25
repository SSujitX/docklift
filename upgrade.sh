#!/bin/bash
set -euo pipefail

# ╔════════════════════════════════════════════════════════════════════════════╗
# ║                         DOCKLIFT UPGRADE SCRIPT                             ║
# ║  Safely upgrades Docklift while preserving all data and user containers     ║
# ╚════════════════════════════════════════════════════════════════════════════╝

# Colors & Vars
RED='\033[0;31m'; GREEN='\033[0;32m'; CYAN='\033[0;36m'; YELLOW='\033[0;33m'
BOLD='\033[1m'; DIM='\033[2m'; NC='\033[0m'
INSTALL_DIR="/opt/docklift"
START_TIME=$(date +%s)
ROLLBACK_REF=""
PRE_BACKEND_TAG="docklift-backend:pre-upgrade"
PRE_FRONTEND_TAG="docklift-frontend:pre-upgrade"
ROLLBACK_ARMED=0
IN_ROLLBACK=0
BACKUP_FILE=""

format_time() {
    local s=$1 h=0 m=0
    # `((…))` returns exit 1 when the value is 0 — must not trip `set -e`
    ((h = s / 3600, m = (s % 3600) / 60, s = s % 60)) || true
    [ $h -gt 0 ] && printf "%dh %dm %ds" $h $m $s || ([ $m -gt 0 ] && printf "%dm %ds" $m $s || printf "%ds" $s)
}

health_ok() {
    # Probe API inside the backend container (Node has fetch; no curl/wget dependency)
    docker compose exec -T backend node -e \
      "fetch('http://127.0.0.1:8000/api/health').then(r=>process.exit(r.ok?0:1)).catch(()=>process.exit(1))" \
      >/dev/null 2>&1
}

sqlite_backup() {
    local src=$1 dest=$2
    # Prefer SQLite online backup API when sqlite3 CLI is present
    if command -v sqlite3 >/dev/null 2>&1; then
        sqlite3 "$src" ".backup '$dest'"
        return $?
    fi
    # Fallback: file copy only when backend is stopped (no open writers)
    cp "$src" "$dest"
}

tag_running_images() {
    # Preserve current images so rollback does not depend on rebuilding an old git ref
    if docker image inspect docklift-backend >/dev/null 2>&1; then
        docker tag docklift-backend "$PRE_BACKEND_TAG" 2>/dev/null || true
    fi
    if docker image inspect docklift-frontend >/dev/null 2>&1; then
        docker tag docklift-frontend "$PRE_FRONTEND_TAG" 2>/dev/null || true
    fi
}

rollback_upgrade() {
    # Prevent re-entrancy from ERR trap while we are already rolling back
    if [ "$IN_ROLLBACK" = "1" ]; then
        return 0
    fi
    IN_ROLLBACK=1
    ROLLBACK_ARMED=0
    trap - ERR INT TERM

    echo -e "\n  ${RED}${BOLD}Rolling back upgrade…${NC}"
    cd "$INSTALL_DIR"
    docker compose stop backend frontend nginx nginx-proxy certbot 2>/dev/null || true

    if [ -n "${ROLLBACK_REF}" ]; then
        git checkout "${ROLLBACK_REF}" -q 2>/dev/null || git checkout -f "${ROLLBACK_REF}" -q || true
    fi

    if [ -n "${BACKUP_FILE:-}" ] && [ -f "$BACKUP_FILE" ]; then
        # Backend must be stopped before overwriting SQLite
        cp "$BACKUP_FILE" "$INSTALL_DIR/data/docklift.db"
        echo -e "  ${YELLOW}Database restored from backup${NC}"
    fi

    # Prefer tagged pre-upgrade images (reliable) over rebuild-from-git
    if docker image inspect "$PRE_BACKEND_TAG" >/dev/null 2>&1; then
        docker tag "$PRE_BACKEND_TAG" docklift-backend 2>/dev/null || true
    fi
    if docker image inspect "$PRE_FRONTEND_TAG" >/dev/null 2>&1; then
        docker tag "$PRE_FRONTEND_TAG" docklift-frontend 2>/dev/null || true
    fi

    docker compose up -d --remove-orphans backend frontend nginx nginx-proxy certbot >> "$LOG" 2>&1 || \
      docker compose up -d --build --remove-orphans backend frontend nginx nginx-proxy certbot >> "$LOG" 2>&1 || true
    echo -e "  ${RED}Rollback attempted. Check $LOG${NC}"
}

arm_rollback() {
    ROLLBACK_ARMED=1
    trap 'if [ "${ROLLBACK_ARMED:-0}" = "1" ]; then rollback_upgrade; fi; exit 1' ERR INT TERM
}

disarm_rollback() {
    ROLLBACK_ARMED=0
    trap - ERR INT TERM
}

# Header
clear 2>/dev/null || true
echo -e "\n  ${CYAN}____             _    _ _  __ _   ${NC}"
echo -e "  ${CYAN}|  _ \\  ___   ___| | _| (_)/ _| |_ ${NC}\n  ${CYAN}| | | |/ _ \\ / __| |/ / | | |_| __|${NC}"
echo -e "  ${CYAN}| |_| | (_) | (__|   <| | |  _| |_ ${NC}\n  ${CYAN}|____/ \\___/ \\___|_|\\_\\_|_|_|  \\__|${NC}\n"
echo -e "  ${DIM}Self-Hosted Docker Deployment Platform${NC}"
echo -e "  ${YELLOW}${BOLD}⬆ UPGRADE MODE${NC}\n"

# Pre-flight checks
[ "$EUID" -ne 0 ] && echo -e "  ${RED}Error: Run with sudo${NC}" && exit 1

if [ ! -d "$INSTALL_DIR" ]; then
    echo -e "  ${RED}Error: Docklift not found at $INSTALL_DIR${NC}"
    echo -e "  ${DIM}Run the install script first: curl -fsSL https://raw.githubusercontent.com/SSujitX/docklift/master/install.sh | sudo bash${NC}"
    exit 1
fi

cd "$INSTALL_DIR"
LOG="/opt/docklift/upgrade.log"
echo "--- Upgrade started at $(date) ---" > "$LOG"

# Pin current git ref for source rollback
ROLLBACK_REF=$(git rev-parse HEAD 2>/dev/null || echo "")

# Get current version
OLD_VERSION=$(grep -o '"version": *"[^"]*"' backend/package.json 2>/dev/null | head -1 | cut -d'"' -f4 || echo "unknown")
echo -e "  ${BOLD}Current Version:${NC} ${CYAN}$OLD_VERSION${NC}"
echo -e "        ${DIM}➞ Git ref: ${ROLLBACK_REF:-unknown}${NC}"

# Step 1: Prepare backup dir BEFORE stopping backend (avoid pre-trap outage gap)
echo -e "\n  ${CYAN}[1/6]${NC} Preparing rollback snapshot..."
BACKUP_DIR="$INSTALL_DIR/backups"
mkdir -p "$BACKUP_DIR"
# Prove the directory is writable before we take the API offline
BACKUP_FILE="$BACKUP_DIR/docklift_$(date +%Y%m%d_%H%M%S).db.bak"
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
        echo -e "        ${GREEN}done${NC}"
    else
        echo -e "        ${RED}Database backup failed${NC}"
        rollback_upgrade
        exit 1
    fi
else
    echo -e "        ${YELLOW}No existing database found (fresh install?)${NC}"
    BACKUP_FILE=""
fi

# Step 2: Fetch latest release
FETCH_ST=$(date +%s)
printf "  ${CYAN}[2/6]${NC} Fetching latest release..."
{
    LATEST_TAG=$(curl -fsS https://api.github.com/repos/SSujitX/docklift/releases/latest | grep '"tag_name"' | cut -d'"' -f4 || echo "")
    git fetch origin --tags -q
    if [ -n "$LATEST_TAG" ]; then
        git checkout "$LATEST_TAG" -q 2>/dev/null || git checkout "tags/$LATEST_TAG" -q
    else
        git fetch origin master -q && git reset --hard origin/master -q
    fi
} >>"$LOG" 2>&1
echo -e " ${GREEN}done${NC} ${DIM}($(format_time $(($(date +%s) - FETCH_ST))))$NC"

NEW_VERSION=$(grep -o '"version": *"[^"]*"' backend/package.json 2>/dev/null | head -1 | cut -d'"' -f4 || echo "unknown")
if [ "$OLD_VERSION" = "$NEW_VERSION" ]; then
    echo -e "        ${DIM}➞ Already on latest version: $NEW_VERSION${NC}"
else
    echo -e "        ${DIM}➞ Upgrading: $OLD_VERSION → ${GREEN}$NEW_VERSION${NC}"
fi

mkdir -p \
  "$INSTALL_DIR/backups" \
  "$INSTALL_DIR/nginx-proxy/conf.d" \
  "$INSTALL_DIR/nginx-proxy/snippets" \
  "$INSTALL_DIR/nginx-proxy/certbot/www" \
  "$INSTALL_DIR/nginx-proxy/certbot/conf"

# Step 3: Stop remaining Docklift containers (preserve user containers)
printf "  ${CYAN}[3/6]${NC} Stopping Docklift containers..."
{
    docker compose stop frontend nginx nginx-proxy certbot 2>/dev/null || true
} >>"$LOG" 2>&1
echo -e " ${GREEN}done${NC}"
echo -e "        ${DIM}➞ User project containers are untouched${NC}"

# Step 4: Rebuild and restart
BUILD_ST=$(date +%s)
echo -e "\n  ${CYAN}[4/6]${NC} Rebuilding Docklift..."
echo -e "        ${DIM}This may take a few minutes...${NC}"
if ! docker compose up -d --build --remove-orphans backend frontend nginx nginx-proxy certbot >> "$LOG" 2>&1; then
    rollback_upgrade
    exit 1
fi
echo -e "        ${GREEN}done${NC} ${DIM}($(format_time $(($(date +%s) - BUILD_ST))))$NC"

# Step 5: Health check with retries
printf "  ${CYAN}[5/6]${NC} Verifying health..."
HEALTHY=0
for i in 1 2 3 4 5 6 7 8 9 10; do
    sleep 3
    if health_ok; then
        HEALTHY=1
        break
    fi
done

RUNNING=$(docker compose ps --format "{{.Name}}" 2>/dev/null | grep -c "docklift" || true)
RUNNING=${RUNNING:-0}
if [ "$HEALTHY" -eq 1 ] && [ "$RUNNING" -ge 5 ]; then
    echo -e " ${GREEN}all systems operational${NC}"
    disarm_rollback
else
    echo -e " ${RED}health check failed${NC}"
    echo -e "        ${DIM}➞ running containers: $RUNNING/5, api health: $HEALTHY${NC}"
    rollback_upgrade
    exit 1
fi

# Step 6: Summary
echo -e "\n  ${CYAN}[6/6]${NC} Finalizing..."
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
echo -e "    ${GREEN}•${NC} Pre-upgrade images tagged ${PRE_BACKEND_TAG} / ${PRE_FRONTEND_TAG}"
echo -e ""

BACKUP_COUNT=$(ls -1 "$BACKUP_DIR"/*.db.bak 2>/dev/null | wc -l || echo "0")
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
