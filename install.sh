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
    local spinstr='|/-\'
    
    while kill -0 "$pid" 2> /dev/null; do
        # Only animate if in a TTY and NOT in CI
        if [ -t 1 ] && [ -z "$CI" ]; then
            local temp=${spinstr#?}
            printf "   ${BLUE}%c${NC}  " "$spinstr"
            local spinstr=$temp${spinstr%"$temp"}
            printf "\b\b\b\b\b\b"
        fi
        sleep $delay
    done
    
    # Clear spinner artifact if we acted
    if [ -t 1 ] && [ -z "$CI" ]; then
        printf "    \b\b\b\b"
    fi
}


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
    echo -e "${YELLOW}[2/6]${NC} Installing Docker..."
    curl -fsSL https://get.docker.com | sh -s -- --quiet > /dev/null 2>&1
    if command -v systemctl &> /dev/null; then
        systemctl enable docker >/dev/null 2>&1 || true
        systemctl start docker >/dev/null 2>&1 || true
    fi

if ! command -v git &> /dev/null; then
    echo -e "\n${YELLOW}Git not found. Installing...${NC}"
    if [ -f /etc/debian_version ]; then
        apt-get update -qq && apt-get install -y -qq git >/dev/null 2>&1
    elif [ -f /etc/alpine-release ]; then
        apk add --no-cache git >/dev/null 2>&1
    elif [ -f /etc/redhat-release ]; then
        # CentOS / AlmaLinux / RHEL / Fedora
        if command -v dnf &> /dev/null; then
            dnf install -y git >/dev/null 2>&1
        else
            yum install -y git >/dev/null 2>&1
        fi
    fi
fi
echo -e "${GREEN}✓ Ready${NC}"

# Step 2: Fetch Code
INSTALL_DIR="/opt/docklift"


# Step 2: Fetch Code
printf "${CYAN}[2/5]${NC} Fetching latest version..."
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

# Step 3: Directories
printf "${CYAN}[3/5]${NC} Preparing directories ($INSTALL_DIR)..."
mkdir -p "$INSTALL_DIR/data" "$INSTALL_DIR/deployments" "$INSTALL_DIR/nginx-proxy/conf.d"
echo -e "${GREEN}✓ Done${NC}"

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
EXIT_CODE=0
wait $pid || EXIT_CODE=$?

if [ $EXIT_CODE -ne 0 ]; then
    echo -e "\n${RED}❌ Build Failed!${NC}"
    if [ -f "$LOG_FILE" ]; then
        cat "$LOG_FILE"
        rm "$LOG_FILE"
    fi
    exit $EXIT_CODE
fi
rm "$LOG_FILE"

# Wait for health check (simple wait)
sleep 5

# Final Status Check
RUNNING=$(docker compose ps --format "table {{.Name}}\t{{.Status}}" 2>/dev/null | grep -c "Up" || echo "0")

if [ "$RUNNING" -gt 0 ]; then
    echo ""
    echo -e "${GREEN}Your instance is ready to use!${NC}"
    echo ""
    
    # Public IPs
    PUBLIC_IPV4=$(curl -4 -s --connect-timeout 2 https://api.ipify.org || echo "")
    PUBLIC_IPV6=$(curl -6 -s --connect-timeout 2 https://api64.ipify.org || echo "")
    
    if [ -n "$PUBLIC_IPV4" ]; then
        echo -e "You can access Docklift through your Public IPv4: ${BOLD}http://${PUBLIC_IPV4}:8080${NC}"
    fi
    
    if [ -n "$PUBLIC_IPV6" ]; then
        echo -e "You can access Docklift through your Public IPv6: ${BOLD}http://[${PUBLIC_IPV6}]:8080${NC}"
    fi
    
    echo ""
    echo -e "If your Public IP is not accessible, you can use the following Private IPs:"
    echo ""
    
    # Get all local IPs
    hostname -I 2>/dev/null | tr ' ' '\n' | while read ip; do
        if [ -n "$ip" ]; then
            echo -e "${BOLD}http://${ip}:8080${NC}"
        fi
    done
    
    echo ""
    echo -e "${DIM}Default credentials: Create your account on first login.${NC}"
    echo ""
else
    echo -e "${RED}⚠️  Something went wrong. Containers are not up.${NC}"
    echo -e "Run 'cd $INSTALL_DIR && docker compose logs' to debug."
fi
echo ""
