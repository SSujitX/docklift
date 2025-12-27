#!/bin/bash
set -e

# Colors and Formatting
RED='\033[0;31m'
GREEN='\033[0;32m'
BLUE='\033[0;34m'
CYAN='\033[0;36m'
YELLOW='\033[1;33m'
BOLD='\033[1m'
NC='\033[0m'

# Spinner function
spinner() {
    local pid=$1
    local delay=0.1
    local spinstr='|/-\'
    while kill -0 "$pid" 2> /dev/null; do
        if [ -t 1 ] && [ -z "$CI" ]; then
            local temp=${spinstr#?}
            printf "   ${BLUE}%c${NC}  " "$spinstr"
            local spinstr=$temp${spinstr%"$temp"}
            printf "\b\b\b\b\b\b"
        fi
        sleep $delay
    done
    if [ -t 1 ] && [ -z "$CI" ]; then printf "    \b\b\b\b"; fi
}

cat << "EOF"
  ____             _    _ _  __ _   
 |  _ \  ___   ___| | _| (_)/ _| |_ 
 | | | |/ _ \ / __| |/ / | | |_| __|
 | |_| | (_) | (__|   <| | |  _| |_ 
 |____/ \___/ \___|_|\_\_|_|_|  \__|
                                    
  AlmaLinux Special Edition
EOF

echo -e "${DIM}  Version 1.0.0${NC}\n"

if [ "$EUID" -ne 0 ]; then
    printf "${RED}❌ Error: Please run as root${NC}"
    exit 1
fi

echo -e "${BOLD}🚀 Starting AlmaLinux Installation...${NC}\n"

# Step 1: Dependencies
printf "${CYAN}[1/5]${NC} Installing AlmaLinux dependencies..."
{
    dnf install -y --allowerasing dnf-plugins-core procps-ng curl >/dev/null 2>&1
} &
spinner $!

# Step 2: Docker
if ! command -v docker &> /dev/null; then
    echo -e "${YELLOW}[2/6]${NC} Installing Docker..."
    if [ "$CI" = "true" ]; then
        curl -fsSL https://get.docker.com | sh -s -- --quiet > /dev/null 2>&1 || echo "⚠️ Docker skipped in CI"
    else
        curl -fsSL https://get.docker.com | sh -s -- --quiet > /dev/null 2>&1
    fi
    systemctl enable docker >/dev/null 2>&1 || true
    systemctl start docker >/dev/null 2>&1 || true
fi

if ! command -v git &> /dev/null; then
    dnf install -y --allowerasing git >/dev/null 2>&1
fi

# Step 2: Fetch Code
INSTALL_DIR="/opt/docklift"
printf "${CYAN}[2/5]${NC} Fetching latest version..."
if [ "$DOCKLIFT_CI_LOCAL" = "true" ]; then
    mkdir -p "$INSTALL_DIR"
    cp -r . "$INSTALL_DIR/"
    cd "$INSTALL_DIR"
else
    if [ -d "$INSTALL_DIR/.git" ]; then
        cd "$INSTALL_DIR"
        docker compose down 2>/dev/null || true
        git fetch origin master -q
        git reset --hard origin/master -q
    else
        git clone -q https://github.com/SSujitX/docklift.git "$INSTALL_DIR"
        cd "$INSTALL_DIR"
    fi
fi
echo -e "${GREEN}✓ Ready${NC}"

# Step 3: Launch
printf "${CYAN}[3/5]${NC} Preparing environment..."
mkdir -p "$INSTALL_DIR/data" "$INSTALL_DIR/deployments" "$INSTALL_DIR/nginx-proxy/conf.d"
rm -f "$INSTALL_DIR/nginx-proxy/conf.d/"*.conf 2>/dev/null || true
cat > "$INSTALL_DIR/nginx-proxy/conf.d/default.conf" <<EOF
server {
    listen 80 default_server;
    server_name _;
    return 404;
}
EOF
echo -e "${GREEN}✓ Done${NC}"

echo -e "${CYAN}[4/5]${NC} Launching Docklift..."
if [ "$CI" = "true" ]; then
    echo -e "${YELLOW}⚠️ CI Environment detected: Skipping 'docker compose up' (Docker daemon not available).${NC}"
else
    docker compose up -d --build --remove-orphans > /dev/null 2>&1 || { echo -e "${RED}❌ Build Failed${NC}"; exit 1; }
fi

echo -e "${GREEN}✅ AlmaLinux Installation Complete!${NC}"
if [ "$CI" != "true" ]; then
    echo -e "Access at: ${BOLD}http://$(curl -s https://api.ipify.org || echo "localhost"):8080${NC}"
fi
