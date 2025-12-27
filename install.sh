#!/bin/bash
set -e

# Colors and Formatting
RED='\033[0;31m'
GREEN='\033[0;32m'
BLUE='\033[0;34m'
CYAN='\033[0;36m'
YELLOW='\033[1;33m'
BOLD='\033[1m'
NC='\033[0m' # No Color
DIM='\033[2m'

# Spinner function
spinner() {
    local pid=$1
    local delay=0.1
    local spinstr='⠋⠙⠹⠸⠼⠴⠦⠧⠇⠏'
    while kill -0 "$pid" 2> /dev/null; do
        local temp=${spinstr#?}
        printf "   ${BLUE}%c${NC}  " "$spinstr"
        local spinstr=$temp${spinstr%"$temp"}
        sleep $delay
        printf "\b\b\b\b\b\b"
    done
    printf "    \b\b\b\b"
}

clear

cat << "EOF"
  ____             _    _ _  __ _   
 |  _ \  ___   ___| | _| (_)/ _| |_ 
 | | | |/ _ \ / __| |/ / | | |_| __|
 | |_| | (_) | (__|   <| | |  _| |_ 
 |____/ \___/ \___|_|\_\_|_|_|  \__|
                                    
  Self-Hosted PaaS for Docker
EOF
echo ""
echo -e "${DIM}  Version 1.0.0${NC}"
echo ""

# Check if running as root
if [ "$EUID" -ne 0 ]; then
    echo -e "${RED}❌ Error: Please run as root (use sudo)${NC}"
    exit 1
fi

echo -e "${BOLD}🚀 Starting Installation...${NC}\n"

# Step 1: System Requirements
printf "${CYAN}[1/5]${NC} Checking system requirements..."
{
    command -v docker &> /dev/null && command -v git &> /dev/null
} &
pid=$!
spinner $pid

if ! command -v docker &> /dev/null; then
    echo -e "\n${YELLOW}Docker not found. Installing...${NC}"
    curl -fsSL https://get.docker.com | sh -s -- --quiet > /dev/null 2>&1
    systemctl enable docker >/dev/null 2>&1
    systemctl start docker >/dev/null 2>&1
fi

if ! command -v git &> /dev/null; then
    echo -e "\n${YELLOW}Git not found. Installing...${NC}"
    if [ -f /etc/debian_version ]; then
        apt-get update -qq && apt-get install -y -qq git >/dev/null 2>&1
    elif [ -f /etc/alpine-release ]; then
        apk add --no-cache git >/dev/null 2>&1
    fi
fi
echo -e "${GREEN}✓ Ready${NC}"

# Step 2: Directories
INSTALL_DIR="/opt/docklift"
printf "${CYAN}[2/5]${NC} Preparing directories ($INSTALL_DIR)..."
mkdir -p "$INSTALL_DIR/data" "$INSTALL_DIR/deployments" "$INSTALL_DIR/nginx-proxy/conf.d"
echo -e "${GREEN}✓ Done${NC}"

# Step 3: Fetch Code
printf "${CYAN}[3/5]${NC} Fetching latest version..."
if [ -d "$INSTALL_DIR/.git" ]; then
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
echo -e "${GREEN}✓ Updated${NC}"

# Step 4: Cleanup
printf "${CYAN}[4/5]${NC} Cleaning networking..."
docker network rm docklift_network 2>/dev/null || true
echo -e "${GREEN}✓ Cleaned${NC}"

# Step 5: Launch
echo -e "${CYAN}[5/5]${NC} Building and launching Docklift..."
echo -e "${DIM}      (This takes a moment for the first build)${NC}"

# Run compose up
LOG_FILE=$(mktemp)
(
    docker compose up -d --build --remove-orphans > "$LOG_FILE" 2>&1
) &
pid=$!
spinner $pid
wait $pid
EXIT_CODE=$?

if [ $EXIT_CODE -ne 0 ]; then
    echo -e "\n${RED}❌ Build Failed!${NC}"
    cat "$LOG_FILE"
    rm "$LOG_FILE"
    exit $EXIT_CODE
fi
rm "$LOG_FILE"

# Wait for health check (simple wait)
sleep 5

# Final Status Check
RUNNING=$(docker compose ps --format "table {{.Name}}\t{{.Status}}" 2>/dev/null | grep -c "Up" || echo "0")
IP=$(hostname -I 2>/dev/null | awk '{print $1}' || echo "127.0.0.1")
PUBLIC_IP=$(curl -s --connect-timeout 2 https://api.ipify.org || echo "Unavailable")

echo ""
if [ "$RUNNING" -gt 0 ]; then
    echo -e "${GREEN}╔═══════════════════════════════════════════════════════════════════╗${NC}"
    echo -e "${GREEN}║                                                                   ║${NC}"
    echo -e "${GREEN}║   ✅ Docklift is ready to use!                                    ║${NC}"
    echo -e "${GREEN}║                                                                   ║${NC}"
    echo -e "${GREEN}╚═══════════════════════════════════════════════════════════════════╝${NC}"
    echo ""
    echo -e "    You can access your dashboard at:"
    echo -e "    ${BOLD}Local:${NC}   http://${IP}:8080"
    echo -e "    ${BOLD}Public:${NC}  http://${PUBLIC_IP}:8080"
    echo ""
    echo -e "${DIM}    Default Port Range: 3001-3100${NC}"
    echo -e "${DIM}    Data Directory:     $INSTALL_DIR/data${NC}"
    echo ""
    echo -e "${CYAN}Make sure firewall port 8080 is open!${NC}"
else
    echo -e "${RED}⚠️  Something went wrong. Containers are not up.${NC}"
    echo -e "Run 'cd $INSTALL_DIR && docker compose logs' to debug."
fi
echo ""
