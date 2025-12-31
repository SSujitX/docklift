#!/bin/bash
set -e

# ╔════════════════════════════════════════════════════════════════════════════╗
# ║                         DOCKLIFT UPGRADE SCRIPT                             ║
# ║  Safely upgrades Docklift while preserving all data and user containers     ║
# ╚════════════════════════════════════════════════════════════════════════════╝

# Colors & Vars
RED='\033[0;31m'; GREEN='\033[0;32m'; CYAN='\033[0;36m'; YELLOW='\033[0;33m'
BOLD='\033[1m'; DIM='\033[2m'; NC='\033[0m'
INSTALL_DIR="/opt/docklift"
START_TIME=$(date +%s)

format_time() {
    local s=$1; ((h=s/3600, m=(s%3600)/60, s=s%60))
    [ $h -gt 0 ] && printf "%dh %dm %ds" $h $m $s || ([ $m -gt 0 ] && printf "%dm %ds" $m $s || printf "%ds" $s)
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

# Get current version
OLD_VERSION=$(grep -o '"version": *"[^"]*"' backend/package.json 2>/dev/null | head -1 | cut -d'"' -f4 || echo "unknown")
echo -e "  ${BOLD}Current Version:${NC} ${CYAN}$OLD_VERSION${NC}"

# Step 1: Backup database
echo -e "\n  ${CYAN}[1/5]${NC} Backing up database..."
BACKUP_DIR="$INSTALL_DIR/backups"
mkdir -p "$BACKUP_DIR"
BACKUP_FILE="$BACKUP_DIR/docklift_$(date +%Y%m%d_%H%M%S).db.bak"
if [ -f "$INSTALL_DIR/data/docklift.db" ]; then
    cp "$INSTALL_DIR/data/docklift.db" "$BACKUP_FILE"
    echo -e "        ${DIM}➞ Backed up to: $BACKUP_FILE${NC}"
    echo -e "        ${GREEN}done${NC}"
else
    echo -e "        ${YELLOW}No existing database found (fresh install?)${NC}"
fi

# Step 2: Fetch latest code
FETCH_ST=$(date +%s)
printf "  ${CYAN}[2/5]${NC} Fetching latest code..."
{
    git fetch origin master -q
    git reset --hard origin/master -q
} >/dev/null 2>&1
echo -e " ${GREEN}done${NC} ${DIM}($(format_time $(($(date +%s) - FETCH_ST))))$NC"

# Get new version
NEW_VERSION=$(grep -o '"version": *"[^"]*"' backend/package.json 2>/dev/null | head -1 | cut -d'"' -f4 || echo "unknown")
if [ "$OLD_VERSION" = "$NEW_VERSION" ]; then
    echo -e "        ${DIM}➞ Already on latest version: $NEW_VERSION${NC}"
else
    echo -e "        ${DIM}➞ Upgrading: $OLD_VERSION → ${GREEN}$NEW_VERSION${NC}"
fi

# Step 3: Stop only Docklift containers (preserve user containers)
printf "  ${CYAN}[3/5]${NC} Stopping Docklift containers..."
{
    docker compose stop docklift-backend docklift-frontend docklift-nginx docklift-nginx-proxy 2>/dev/null || true
} >/dev/null 2>&1
echo -e " ${GREEN}done${NC}"
echo -e "        ${DIM}➞ User project containers are untouched${NC}"

# Step 4: Rebuild and restart
BUILD_ST=$(date +%s)
echo -e "\n  ${CYAN}[4/5]${NC} Rebuilding Docklift..."
echo -e "        ${DIM}This may take a few minutes...${NC}"
LOG="/opt/docklift/upgrade.log"
echo "--- Upgrade started at $(date) ---" > "$LOG"
if ! docker compose up -d --build docklift-backend docklift-frontend docklift-nginx docklift-nginx-proxy >> "$LOG" 2>&1; then
    echo -e "\n  ${RED}Build failed! Rolling back...${NC}"
    echo -e "  ${RED}Check $LOG for details.${NC}"
    # Restore backup
    if [ -f "$BACKUP_FILE" ]; then
        cp "$BACKUP_FILE" "$INSTALL_DIR/data/docklift.db"
        echo -e "  ${YELLOW}Database restored from backup${NC}"
    fi
    exit 1
fi
echo -e "        ${GREEN}done${NC} ${DIM}($(format_time $(($(date +%s) - BUILD_ST))))$NC"

# Step 5: Health check
printf "  ${CYAN}[5/5]${NC} Verifying..."
sleep 5
RUNNING=$(docker compose ps --format "{{.Name}}" 2>/dev/null | grep -c "docklift" || echo "0")
if [ "$RUNNING" -ge 4 ]; then
    echo -e " ${GREEN}all systems operational${NC}"
else
    echo -e " ${YELLOW}warning: only $RUNNING/4 containers running${NC}"
fi

# Summary
echo -e "\n  ╔══════════════════════════════════════════════════════════════╗"
echo -e "  ║  ${GREEN}${BOLD}✓ UPGRADE COMPLETE${NC}                                        ║"
echo -e "  ╚══════════════════════════════════════════════════════════════╝"
TOTAL_TIME=$(($(date +%s) - START_TIME))
echo -e "  ${DIM}Time: $(format_time $TOTAL_TIME) | Version: $NEW_VERSION${NC}\n"

# Show what was preserved
echo -e "  ${BOLD}✓ Preserved:${NC}"
echo -e "    ${GREEN}•${NC} Database (projects, settings, deployments)"
echo -e "    ${GREEN}•${NC} All user containers (dl_* containers)"
echo -e "    ${GREEN}•${NC} Nginx configurations"
echo -e "    ${GREEN}•${NC} Project files in /deployments"
echo -e ""

# Show backup info
BACKUP_COUNT=$(ls -1 "$BACKUP_DIR"/*.db.bak 2>/dev/null | wc -l || echo "0")
echo -e "  ${DIM}Backups stored: $BACKUP_COUNT files in $BACKUP_DIR${NC}"
echo -e "  ${DIM}To restore: cp $BACKUP_FILE $INSTALL_DIR/data/docklift.db${NC}\n"

# Access info
if [ "$CI" != "true" ]; then
    PUB4=$(curl -4 -s --connect-timeout 2 https://api.ipify.org 2>/dev/null || echo "")
    if [ -n "$PUB4" ]; then
        echo -e "  ${BOLD}Access Docklift:${NC} http://${PUB4}:8080\n"
    fi
fi
