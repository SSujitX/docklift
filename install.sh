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

# Format seconds to human readable
format_duration() {
    local seconds=$1
    local hours=$((seconds / 3600))
    local minutes=$(((seconds % 3600) / 60))
    local secs=$((seconds % 60))
    
    if [ $hours -gt 0 ]; then
        echo "${hours}h ${minutes}m ${secs}s"
    elif [ $minutes -gt 0 ]; then
        echo "${minutes}m ${secs}s"
    else
        echo "${secs}s"
    fi
}

# Create clickable link (OSC 8)
link() {
    printf '\033]8;;%s\033\\%s\033]8;;\033\\' "$1" "$2"
}

# Print step
step() {
    printf "  ${CYAN}[%s]${NC} %s" "$1" "$2"
}

# Print done
done_step() {
    echo -e " ${GREEN}done${NC}"
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
    echo -e "  ${RED}Error: Please run as root (use sudo)${NC}"
    exit 1
fi

echo -e "  ${BOLD}Starting Installation...${NC}"
echo ""

# Step 1: Check requirements
step "1/5" "Checking requirements..."
NEED_DOCKER=false
NEED_GIT=false

if ! command -v docker &> /dev/null; then NEED_DOCKER=true; fi
if ! command -v git &> /dev/null; then NEED_GIT=true; fi
done_step

# Install Docker if needed
if [ "$NEED_DOCKER" = true ]; then
    step "1/5" "Installing Docker..."
    if [ "$CI" = "true" ]; then
        curl -fsSL https://get.docker.com | sh -s -- --quiet > /dev/null 2>&1 || true
    else
        curl -fsSL https://get.docker.com | sh -s -- --quiet > /dev/null 2>&1
    fi
    systemctl enable docker >/dev/null 2>&1 || true
    systemctl start docker >/dev/null 2>&1 || true
    done_step
fi

# Install Git if needed
if [ "$NEED_GIT" = true ]; then
    step "1/5" "Installing Git..."
    apt-get update -qq && apt-get install -y -qq git >/dev/null 2>&1 || \
    yum install -y git >/dev/null 2>&1 || \
    apk add --no-cache git >/dev/null 2>&1
    done_step
fi

# Step 2: Fetch code
INSTALL_DIR="/opt/docklift"

step "2/5" "Fetching code..."
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
done_step

# Show version
VERSION=$(grep -o '"version": *"[^"]*"' "$INSTALL_DIR/backend/package.json" 2>/dev/null | head -1 | cut -d'"' -f4 || echo "1.0.0")
echo -e "  ${DIM}v${VERSION}${NC}"
echo ""

# Step 3: Directories
step "3/5" "Creating directories..."
mkdir -p "$INSTALL_DIR/data" "$INSTALL_DIR/deployments" "$INSTALL_DIR/nginx-proxy/conf.d"
done_step

# Step 4: Network cleanup
step "4/5" "Cleaning network..."
docker network rm docklift_network 2>/dev/null || true
done_step

# Step 5: Build and launch
BUILD_START=$(date +%s)
echo ""
step "5/5" "Building containers..."
echo ""
echo -e "  ${DIM}This may take a few minutes...${NC}"
echo ""

# Default nginx config
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
BUILD_DURATION=$((BUILD_END - BUILD_START))

if [ $EXIT_CODE -ne 0 ]; then
    echo -e "  ${RED}Build failed!${NC}"
    cat "$LOG_FILE"
    rm "$LOG_FILE"
    exit $EXIT_CODE
fi
rm "$LOG_FILE"

# Wait for containers
sleep 5

# Total time
END_TIME=$(date +%s)
TOTAL_DURATION=$((END_TIME - START_TIME))

# Check status
RUNNING=$(docker compose ps --format "table {{.Name}}\t{{.Status}}" 2>/dev/null | grep -c "Up" || echo "0")

if [ "$RUNNING" -gt 0 ]; then
    echo ""
    echo -e "  ${GREEN}${BOLD}Installation Complete!${NC}"
    echo ""
    echo -e "  ${DIM}Build: $(format_duration $BUILD_DURATION) | Total: $(format_duration $TOTAL_DURATION)${NC}"
    echo ""
    
    # Get IPs
    PUBLIC_IPV4=$(curl -4 -s --connect-timeout 2 https://api.ipify.org || echo "")
    
    echo -e "  ${BOLD}Access Docklift:${NC}"
    echo ""
    
    if [ -n "$PUBLIC_IPV4" ]; then
        URL="http://${PUBLIC_IPV4}:8080"
        echo -e "  ${CYAN}>${NC} $(link "$URL" "$URL")"
    fi
    
    # Local IPs
    hostname -I 2>/dev/null | tr ' ' '\n' | head -3 | while read ip; do
        if [ -n "$ip" ]; then
            URL="http://${ip}:8080"
            echo -e "  ${DIM}>${NC} $(link "$URL" "$URL")"
        fi
    done
    
    echo ""
else
    echo -e "  ${RED}Something went wrong. Containers not running.${NC}"
    echo -e "  Run: cd $INSTALL_DIR && docker compose logs"
fi
echo ""
