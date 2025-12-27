#!/bin/bash
set -e

# Colors & Vars
RED='\033[0;31m'; GREEN='\033[0;32m'; CYAN='\033[0;36m'; BOLD='\033[1m'; DIM='\033[2m'; NC='\033[0m'
START_TIME=$(date +%s); INSTALL_DIR="/opt/docklift"

format_time() {
    local s=$1; ((h=s/3600, m=(s%3600)/60, s=s%60))
    [ $h -gt 0 ] && printf "%dh %dm %ds" $h $m $s || ([ $m -gt 0 ] && printf "%dm %ds" $m $s || printf "%ds" $s)
}

# Header
clear 2>/dev/null || true
echo -e "\n  ${CYAN}____             _    _ _  __ _   ${NC}"
echo -e "  ${CYAN}|  _ \\  ___   ___| | _| (_)/ _| |_ ${NC}\n  ${CYAN}| | | |/ _ \\ / __| |/ / | | |_| __|${NC}"
echo -e "  ${CYAN}| |_| | (_) | (__|   <| | |  _| |_ ${NC}\n  ${CYAN}|____/ \\___/ \\___|_|\\_\\_|_|_|  \\__|${NC}\n"
echo -e "  ${DIM}Self-Hosted Docker Deployment Platform${NC}\n"

[ "$EUID" -ne 0 ] && echo -e "  ${RED}Error: Run with sudo${NC}" && exit 1
echo -e "  ${BOLD}Starting Installation${NC}\n"

# Step 1: Requirements
printf "  ${CYAN}[1/5]${NC} Checking requirements..."
for cmd in docker git; do
    if ! command -v $cmd &>/dev/null; then
        printf " Installing $cmd..."
        if [ "$cmd" = "docker" ]; then curl -fsSL https://get.docker.com | sh -s -- --quiet >/dev/null 2>&1
        else apt-get update -qq && apt-get install -y -qq git >/dev/null 2>&1 || yum install -y git >/dev/null 2>&1 || apk add --no-cache git >/dev/null 2>&1; fi
    fi
done
echo -e " ${GREEN}done${NC}"

# Step 2: Fetch
FETCH_ST=$(date +%s)
printf "  ${CYAN}[2/5]${NC} Fetching code..."
{
    if [ "$DOCKLIFT_CI_LOCAL" = "true" ]; then mkdir -p "$INSTALL_DIR" && cp -r . "$INSTALL_DIR/" && cd "$INSTALL_DIR"
    elif [ -d "$INSTALL_DIR/.git" ]; then cd "$INSTALL_DIR" && docker compose down 2>/dev/null || true && git fetch origin master -q && git reset --hard origin/master -q
    else git clone -q https://github.com/SSujitX/docklift.git "$INSTALL_DIR" && cd "$INSTALL_DIR"; fi
} >/dev/null 2>&1
echo -e " ${GREEN}done${NC} ${DIM}($(format_time $(($(date +%s) - FETCH_ST))))$NC"
VERSION=$(grep -o '"version": *"[^"]*"' "$INSTALL_DIR/backend/package.json" 2>/dev/null | head -1 | cut -d'"' -f4 || echo "1.1.1")
echo -e "        ${DIM}➞ Version: $VERSION${NC}"

# Step 3-5: Setup & Build
printf "  ${CYAN}[3/5]${NC} Creating directories... " && mkdir -p "$INSTALL_DIR/data" "$INSTALL_DIR/deployments" "$INSTALL_DIR/nginx-proxy/conf.d" && echo -e "${GREEN}done${NC}"
printf "  ${CYAN}[4/5]${NC} Cleaning network... " && (docker network rm docklift_network 2>/dev/null || true) && echo -e "${GREEN}done${NC}"

BUILD_ST=$(date +%s); echo -e "\n  ${CYAN}[5/5]${NC} Building containers...\n        ${DIM}This may take a few minutes...${NC}"
cat > "$INSTALL_DIR/nginx-proxy/conf.d/default.conf" <<EOF
server { listen 80 default_server; server_name _; return 404; }
EOF

LOG=$(mktemp)
if ! docker compose up -d --build --remove-orphans > "$LOG" 2>&1; then
    echo -e "\n  ${RED}Build failed!${NC}"; cat "$LOG"; rm "$LOG"; exit 1
fi
rm "$LOG"; sleep 5

# Results
TOTAL_TIME=$(($(date +%s) - START_TIME)); BUILD_TIME=$(($(date +%s) - BUILD_ST))
RUNNING=$(docker compose ps --format "{{.Name}}" | grep -c "docklift" || echo "0")

if [ "$RUNNING" -gt 0 ]; then
    echo -e "\n  ${GREEN}${BOLD}Installation Complete!${NC}\n  ${DIM}Build: $(format_time $BUILD_TIME) | Total: $(format_time $TOTAL_TIME)${NC}\n"
    if [ "$CI" != "true" ]; then
        PUB4=$(curl -4 -s --connect-timeout 2 https://api.ipify.org 2>/dev/null || echo "")
        PUB6=$(curl -6 -s --connect-timeout 2 https://api64.ipify.org 2>/dev/null || echo "")
        PRV=$(hostname -I 2>/dev/null | tr ' ' '\n' | grep -v "${PUB4:-NOT_SET}" | grep -E '^(10\.|172\.(1[6-9]|2[0-9]|3[0-1])\.|192\.168\.)' | head -1 || echo "")
        if [ -n "$PUB4" ] || [ -n "$PRV" ]; then
            echo -e "  ${BOLD}Access Docklift:${NC}\n"
            [ -n "$PUB4" ] && printf "  ${CYAN}Public:  ${NC} http://${PUB4}:8080\n"
            [ -n "$PUB6" ] && printf "  ${CYAN}IPv6:    ${NC} http://[${PUB6}]:8080\n"
            [ -n "$PRV" ] && printf "  ${DIM}Private: ${NC} http://${PRV}:8080\n"
        fi
    else echo -e "  ${DIM}Version: $VERSION | Build: $(format_time $BUILD_TIME)${NC}\n"
    fi
else echo -e "  ${RED}Error: Containers not running${NC}\n  ${DIM}Run: cd $INSTALL_DIR && docker compose logs${NC}\n"
fi
