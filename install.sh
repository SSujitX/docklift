#!/bin/bash
set -e

# Colors and Formatting
RED='\033[0;31m'
GREEN='\033[0;32m'
BLUE='\033[0;34m'
CYAN='\033[0;36m'
YELLOW='\033[1;33m'
MAGENTA='\033[0;35m'
BOLD='\033[1m'
NC='\033[0m' # No Color
DIM='\033[2m'

# Track start time
START_TIME=$(date +%s)

# Format seconds to human readable
format_duration() {
    local seconds=$1
    local hours=$((seconds / 3600))
    local minutes=$(((seconds % 3600) / 60))
    local secs=$((seconds % 60))
    
    local result=""
    if [ $hours -gt 0 ]; then
        result="${hours}h ${minutes}m ${secs}s"
    elif [ $minutes -gt 0 ]; then
        result="${minutes}m ${secs}s"
    else
        result="${secs}s"
    fi
    echo "$result"
}

# Create clickable link (OSC 8 hyperlink - works in modern terminals)
hyperlink() {
    local url=$1
    local text=$2
    # OSC 8 hyperlink escape sequence
    printf '\033]8;;%s\033\\%s\033]8;;\033\\' "$url" "$text"
}

# Spinner function
spinner() {
    local pid=$1
    local delay=0.1
    local spinstr='⠋⠙⠹⠸⠼⠴⠦⠧⠇⠏'
    
    while kill -0 "$pid" 2> /dev/null; do
        if [ -t 1 ] && [ -z "$CI" ]; then
            local temp=${spinstr#?}
            printf " ${CYAN}%c${NC} " "$spinstr"
            spinstr=$temp${spinstr%"$temp"}
            printf "\b\b\b\b"
        fi
        sleep $delay
    done
    
    if [ -t 1 ] && [ -z "$CI" ]; then
        printf "    \b\b\b\b"
    fi
}

# Print header
clear 2>/dev/null || true
echo ""
echo -e "${CYAN}${BOLD}"
cat <<'EOF'
  ╔═══════════════════════════════════════════════╗
  ║                                               ║
  ║   ██████╗  ██████╗  ██████╗██╗  ██╗          ║
  ║   ██╔══██╗██╔═══██╗██╔════╝██║ ██╔╝          ║
  ║   ██║  ██║██║   ██║██║     █████╔╝           ║
  ║   ██║  ██║██║   ██║██║     ██╔═██╗           ║
  ║   ██████╔╝╚██████╔╝╚██████╗██║  ██╗          ║
  ║   ╚═════╝  ╚═════╝  ╚═════╝╚═╝  ╚═╝LIFT      ║
  ║                                               ║
  ║   Self-Hosted Docker Deployment Platform      ║
  ║                                               ║
  ╚═══════════════════════════════════════════════╝
EOF
echo -e "${NC}"
echo -e "  ${DIM}v1.0.0 • github.com/SSujitX/docklift${NC}"
echo ""

# Check if running as root
if [ "$EUID" -ne 0 ]; then
    echo -e "${RED}  ❌ Error: Please run as root (use sudo)${NC}"
    exit 1
fi

echo -e "${BOLD}  🚀 Starting Installation...${NC}"
echo -e "  ${DIM}━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━${NC}"
echo ""

# Step 1: System Requirements
printf "  ${CYAN}[1/5]${NC} Checking system requirements..."
{
    command -v docker &> /dev/null && command -v git &> /dev/null
} &
pid=$!
spinner $pid

if ! command -v docker &> /dev/null; then
    printf "${YELLOW}[2/6]${NC} Installing Docker..."
    if [ "$CI" = "true" ]; then
        curl -fsSL https://get.docker.com | sh -s -- --quiet > /dev/null 2>&1 || echo -e "${YELLOW}⚠️ Docker installation skipped/failed in CI.${NC}"
    else
        curl -fsSL https://get.docker.com | sh -s -- --quiet > /dev/null 2>&1
    fi
    
    if command -v systemctl &> /dev/null; then
        systemctl enable docker >/dev/null 2>&1 || true
        systemctl start docker >/dev/null 2>&1 || true
    fi
fi

if ! command -v git &> /dev/null; then
    echo -e "\n${YELLOW}  Git not found. Installing...${NC}"
    apt-get update -qq && apt-get install -y -qq git >/dev/null 2>&1 || yum install -y git >/dev/null 2>&1 || apk add --no-cache git >/dev/null 2>&1
fi
echo -e " ${GREEN}✓${NC}"

# Step 2: Fetch Code
INSTALL_DIR="/opt/docklift"

printf "  ${CYAN}[2/5]${NC} Fetching latest version..."
if [ "$DOCKLIFT_CI_LOCAL" = "true" ]; then
    echo -e "${YELLOW}CI Local mode: Copying current directory to $INSTALL_DIR...${NC}"
    mkdir -p "$INSTALL_DIR"
    cp -r . "$INSTALL_DIR/"
    cd "$INSTALL_DIR"
elif [ -d "$INSTALL_DIR/.git" ]; then
    cd "$INSTALL_DIR"
    (
        docker compose down 2>/dev/null || true
        git fetch origin master -q
        git reset --hard origin/master -q
    ) &
    pid=$!
    spinner $pid
    wait $pid
else
    (
        git clone -q https://github.com/SSujitX/docklift.git "$INSTALL_DIR"
    ) &
    pid=$!
    spinner $pid
    wait $pid
    cd "$INSTALL_DIR"
fi
echo -e " ${GREEN}✓${NC}"

# Step 3: Directories
printf "  ${CYAN}[3/5]${NC} Preparing directories..."
mkdir -p "$INSTALL_DIR/data" "$INSTALL_DIR/deployments" "$INSTALL_DIR/nginx-proxy/conf.d"
echo -e " ${GREEN}✓${NC}"

# Step 4: Cleanup
printf "  ${CYAN}[4/5]${NC} Cleaning network..."
docker network rm docklift_network 2>/dev/null || true
echo -e " ${GREEN}✓${NC}"

# Step 5: Launch (track build time separately)
BUILD_START=$(date +%s)
echo ""
echo -e "  ${CYAN}[5/5]${NC} Building and launching..."
echo -e "  ${DIM}      This may take a few minutes for the first build${NC}"
echo ""

# Run compose up
LOG_FILE=$(mktemp)
(
    rm -f "$INSTALL_DIR/nginx-proxy/conf.d/"*.conf 2>/dev/null || true
    
    cat > "$INSTALL_DIR/nginx-proxy/conf.d/default.conf" <<EOF
server {
    listen 80 default_server;
    server_name _;
    return 404;
}
EOF

    docker compose up -d --build --remove-orphans > "$LOG_FILE" 2>&1
) &
pid=$!
spinner $pid
EXIT_CODE=0
wait $pid || EXIT_CODE=$?

BUILD_END=$(date +%s)
BUILD_DURATION=$((BUILD_END - BUILD_START))

if [ $EXIT_CODE -ne 0 ]; then
    echo -e "\n  ${RED}❌ Build Failed!${NC}"
    if [ -f "$LOG_FILE" ]; then
        cat "$LOG_FILE"
        rm "$LOG_FILE"
    fi
    exit $EXIT_CODE
fi
rm "$LOG_FILE"

# Wait for health check
sleep 5

# Calculate total time
END_TIME=$(date +%s)
TOTAL_DURATION=$((END_TIME - START_TIME))

# Final Status Check
RUNNING=$(docker compose ps --format "table {{.Name}}\t{{.Status}}" 2>/dev/null | grep -c "Up" || echo "0")

if [ "$RUNNING" -gt 0 ]; then
    # Success banner
    echo ""
    echo -e "  ${GREEN}${BOLD}━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━${NC}"
    echo -e "  ${GREEN}${BOLD}  ✅ Installation Complete!${NC}"
    echo -e "  ${GREEN}${BOLD}━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━${NC}"
    echo ""
    
    # Timing info
    echo -e "  ${DIM}⏱  Build time: $(format_duration $BUILD_DURATION)${NC}"
    echo -e "  ${DIM}⏱  Total time: $(format_duration $TOTAL_DURATION)${NC}"
    echo ""
    
    # Get IPs
    PUBLIC_IPV4=$(curl -4 -s --connect-timeout 2 https://api.ipify.org || echo "")
    PUBLIC_IPV6=$(curl -6 -s --connect-timeout 2 https://api64.ipify.org || echo "")
    
    echo -e "  ${BOLD}🌐 Access Docklift:${NC}"
    echo ""
    
    if [ -n "$PUBLIC_IPV4" ]; then
        URL="http://${PUBLIC_IPV4}:8080"
        echo -e "  ${CYAN}▸${NC} Public IPv4:  $(hyperlink "$URL" "$URL")"
    fi
    
    if [ -n "$PUBLIC_IPV6" ]; then
        URL="http://[${PUBLIC_IPV6}]:8080"
        echo -e "  ${CYAN}▸${NC} Public IPv6:  $(hyperlink "$URL" "$URL")"
    fi
    
    # Local IPs
    echo ""
    echo -e "  ${DIM}Local addresses:${NC}"
    hostname -I 2>/dev/null | tr ' ' '\n' | while read ip; do
        if [ -n "$ip" ]; then
            URL="http://${ip}:8080"
            echo -e "  ${DIM}▸${NC} $(hyperlink "$URL" "$URL")"
        fi
    done
    
    echo ""
    echo -e "  ${DIM}━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━${NC}"
    echo -e "  ${DIM}📖 Docs: https://github.com/SSujitX/docklift${NC}"
    echo -e "  ${DIM}━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━${NC}"
    echo ""
else
    echo -e "  ${RED}⚠️  Something went wrong. Containers are not up.${NC}"
    echo -e "  Run 'cd $INSTALL_DIR && docker compose logs' to debug."
fi
echo ""
