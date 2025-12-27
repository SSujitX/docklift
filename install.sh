#!/bin/bash
set -e

# Colors
RED='\033[0;31m'
GREEN='\033[0;32m'
CYAN='\033[0;36m'
YELLOW='\033[1;33m'
BOLD='\033[1m'
DIM='\033[2m'
NC='\033[0m'

# Track start time
START_TIME=$(date +%s)

# Format seconds
format_time() {
    local s=$1
    if [ $s -ge 3600 ]; then
        printf "%dh %dm %ds" $((s/3600)) $((s%3600/60)) $((s%60))
    elif [ $s -ge 60 ]; then
        printf "%dm %ds" $((s/60)) $((s%60))
    else
        printf "%ds" $s
    fi
}

# Clickable link (OSC 8)
link() {
    printf '\033]8;;%s\033\\%s\033]8;;\033\\' "$1" "$2"
}

# Header
clear 2>/dev/null || true
echo ""
echo -e "${CYAN}"
echo "  ____             _    _ _  __ _   "
echo " |  _ \\  ___   ___| | _| (_)/ _| |_ "
echo " | | | |/ _ \\ / __| |/ / | | |_| __|"
echo " | |_| | (_) | (__|   <| | |  _| |_ "
echo " |____/ \\___/ \\___|_|\\_\\_|_|_|  \\__|"
echo -e "${NC}"
echo -e "  ${DIM}Self-Hosted Docker Deployment Platform${NC}"
echo ""

# Check root
if [ "$EUID" -ne 0 ]; then
    echo -e "  ${RED}Error: Please run with sudo${NC}"
    exit 1
fi

echo -e "  ${BOLD}Starting Installation${NC}"
echo ""

# Step 1: Requirements
printf "  ${CYAN}[1/5]${NC} Checking requirements..."
NEED_DOCKER=false
NEED_GIT=false
command -v docker &>/dev/null || NEED_DOCKER=true
command -v git &>/dev/null || NEED_GIT=true
echo -e " ${GREEN}done${NC}"

if [ "$NEED_DOCKER" = true ]; then
    printf "        Installing Docker..."
    curl -fsSL https://get.docker.com | sh -s -- --quiet >/dev/null 2>&1 || true
    systemctl enable docker >/dev/null 2>&1 || true
    systemctl start docker >/dev/null 2>&1 || true
    echo -e " ${GREEN}done${NC}"
fi

if [ "$NEED_GIT" = true ]; then
    printf "        Installing Git..."
    apt-get update -qq && apt-get install -y -qq git >/dev/null 2>&1 || \
    yum install -y git >/dev/null 2>&1 || \
    apk add --no-cache git >/dev/null 2>&1
    echo -e " ${GREEN}done${NC}"
fi

# Step 2: Fetch
INSTALL_DIR="/opt/docklift"
FETCH_START=$(date +%s)

printf "  ${CYAN}[2/5]${NC} Fetching code..."
if [ "$DOCKLIFT_CI_LOCAL" = "true" ]; then
    mkdir -p "$INSTALL_DIR"
    cp -r . "$INSTALL_DIR/"
    cd "$INSTALL_DIR"
elif [ -d "$INSTALL_DIR/.git" ]; then
    cd "$INSTALL_DIR"
    docker compose down 2>/dev/null || true
    git fetch origin master -q 2>/dev/null
    git reset --hard origin/master -q 2>/dev/null
else
    git clone -q https://github.com/SSujitX/docklift.git "$INSTALL_DIR" 2>/dev/null
    cd "$INSTALL_DIR"
fi

FETCH_END=$(date +%s)
FETCH_TIME=$((FETCH_END - FETCH_START))
echo -e " ${GREEN}done${NC} ${DIM}($(format_time $FETCH_TIME))${NC}"

# Show version
VERSION=$(grep -o '"version": *"[^"]*"' "$INSTALL_DIR/backend/package.json" 2>/dev/null | head -1 | cut -d'"' -f4 || echo "1.0.0")
echo -e "        ${DIM}Version: ${VERSION}${NC}"

# Step 3: Directories
printf "  ${CYAN}[3/5]${NC} Creating directories..."
mkdir -p "$INSTALL_DIR/data" "$INSTALL_DIR/deployments" "$INSTALL_DIR/nginx-proxy/conf.d"
echo -e " ${GREEN}done${NC}"

# Step 4: Network
printf "  ${CYAN}[4/5]${NC} Cleaning network..."
docker network rm docklift_network 2>/dev/null || true
echo -e " ${GREEN}done${NC}"

# Step 5: Build
BUILD_START=$(date +%s)
echo ""
echo -e "  ${CYAN}[5/5]${NC} Building containers..."
echo -e "        ${DIM}This may take a few minutes...${NC}"

# Nginx config
rm -f "$INSTALL_DIR/nginx-proxy/conf.d/"*.conf 2>/dev/null || true
cat > "$INSTALL_DIR/nginx-proxy/conf.d/default.conf" <<EOF
server {
    listen 80 default_server;
    server_name _;
    return 404;
}
EOF

# Build
LOG_FILE=$(mktemp)
docker compose up -d --build --remove-orphans > "$LOG_FILE" 2>&1
EXIT_CODE=$?
BUILD_END=$(date +%s)
BUILD_TIME=$((BUILD_END - BUILD_START))

if [ $EXIT_CODE -ne 0 ]; then
    echo ""
    echo -e "  ${RED}Build failed!${NC}"
    cat "$LOG_FILE"
    rm "$LOG_FILE"
    exit $EXIT_CODE
fi
rm "$LOG_FILE"

sleep 5

# Results
END_TIME=$(date +%s)
TOTAL_TIME=$((END_TIME - START_TIME))
RUNNING=$(docker compose ps --format "table {{.Name}}\t{{.Status}}" 2>/dev/null | grep -c "Up" || echo "0")

echo ""
if [ "$RUNNING" -gt 0 ]; then
    echo -e "  ${GREEN}${BOLD}Installation Complete!${NC}"
    echo ""
    echo -e "  ${DIM}Build: $(format_time $BUILD_TIME) | Total: $(format_time $TOTAL_TIME)${NC}"
    echo ""
    
    if [ "$CI" != "true" ]; then
        PUBLIC_IPV4=$(curl -4 -s --connect-timeout 2 https://api.ipify.org 2>/dev/null || echo "")
        PUBLIC_IPV6=$(curl -6 -s --connect-timeout 2 https://api64.ipify.org 2>/dev/null || echo "")
        PRIVATE_IP=$(hostname -I 2>/dev/null | awk '{print $1}')
        
        echo -e "  ${BOLD}Access Docklift:${NC}"
        echo ""
        
        if [ -n "$PUBLIC_IPV4" ]; then
            URL="http://${PUBLIC_IPV4}:8080"
            echo -e "  ${CYAN}IPv4:${NC}    $(link "$URL" "$URL")"
        fi
        
        if [ -n "$PUBLIC_IPV6" ]; then
            URL="http://[${PUBLIC_IPV6}]:8080"
            echo -e "  ${CYAN}IPv6:${NC}    $(link "$URL" "$URL")"
        fi
        
        if [ -n "$PRIVATE_IP" ]; then
            URL="http://${PRIVATE_IP}:8080"
            echo -e "  ${DIM}Private:${NC} $(link "$URL" "$URL")"
        fi
        echo ""
    else
        echo -e "  ${DIM}CI environment - containers ready${NC}"
        echo ""
    fi
else
    echo -e "  ${RED}Error: Containers not running${NC}"
    echo -e "  ${DIM}Run: cd $INSTALL_DIR && docker compose logs${NC}"
fi
