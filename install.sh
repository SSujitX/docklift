#!/bin/bash
set -e

# Colors
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
CYAN='\033[0;36m'
NC='\033[0m' # No Color

echo -e "${CYAN}"
echo "╔═══════════════════════════════════════════════════════════╗"
echo "║                                                           ║"
echo "║   🐳 DOCKLIFT - Self-Hosted Docker Deployment Platform    ║"
echo "║                                                           ║"
echo "╚═══════════════════════════════════════════════════════════╝"
echo -e "${NC}"

# Check if running as root
if [ "$EUID" -ne 0 ]; then
    echo -e "${RED}❌ Please run as root (use sudo)${NC}"
    exit 1
fi

echo -e "${YELLOW}[1/6]${NC} Checking system requirements..."

# Check for Docker
if ! command -v docker &> /dev/null; then
    echo -e "${YELLOW}[2/6]${NC} Installing Docker..."
    curl -fsSL https://get.docker.com | sh -s -- --quiet
    systemctl enable docker >/dev/null 2>&1
    systemctl start docker >/dev/null 2>&1
    echo -e "${GREEN}  ✓ Docker installed${NC}"
else
    echo -e "${GREEN}  ✓ Docker found${NC}"
fi

# Check for Docker Compose
if ! docker compose version &> /dev/null; then
    echo -e "${YELLOW}[3/6]${NC} Installing Docker Compose..."
    apt-get update -qq && apt-get install -y -qq docker-compose-plugin >/dev/null 2>&1
    echo -e "${GREEN}  ✓ Docker Compose installed${NC}"
else
    echo -e "${GREEN}  ✓ Docker Compose found${NC}"
fi

# Check for Git
if ! command -v git &> /dev/null; then
    echo -e "${YELLOW}[4/6]${NC} Installing Git..."
    apt-get update -qq && apt-get install -y -qq git >/dev/null 2>&1
    echo -e "${GREEN}  ✓ Git installed${NC}"
else
    echo -e "${GREEN}  ✓ Git found${NC}"
fi

# Set install directory
INSTALL_DIR="/opt/docklift"

# Clone or update repo
if [ -d "$INSTALL_DIR" ]; then
    echo -e "${YELLOW}[5/6]${NC} Updating existing installation..."
    cd "$INSTALL_DIR"
    git pull origin master -q
else
    echo -e "${YELLOW}[5/6]${NC} Cloning Docklift..."
    git clone -q https://github.com/SSujitX/docklift.git "$INSTALL_DIR"
    cd "$INSTALL_DIR"
fi

# Create data directories
mkdir -p "$INSTALL_DIR/data"
mkdir -p "$INSTALL_DIR/deployments"
mkdir -p "$INSTALL_DIR/nginx-proxy/conf.d"

# Clean up any conflicting network from previous installs
docker network rm docklift_network 2>/dev/null || true

# Build and start
echo -e "${YELLOW}[6/6]${NC} Building and starting Docklift (this may take a few minutes)..."
docker compose up -d --build 2>&1 | tail -20

# Wait for containers to start
echo -e "${YELLOW}  Waiting for containers to start...${NC}"
sleep 5

# Check if containers are running
RUNNING=$(docker compose ps --format "table {{.Name}}\t{{.Status}}" 2>/dev/null | grep -c "Up" || echo "0")

if [ "$RUNNING" -gt 0 ]; then
    # Get server IP
    SERVER_IP=$(hostname -I | awk '{print $1}')

    echo ""
    echo -e "${GREEN}╔═══════════════════════════════════════════════════════════╗${NC}"
    echo -e "${GREEN}║                                                           ║${NC}"
    echo -e "${GREEN}║   ✅ Docklift installed successfully!                     ║${NC}"
    echo -e "${GREEN}║                                                           ║${NC}"
    echo -e "${GREEN}╚═══════════════════════════════════════════════════════════╝${NC}"
    echo ""
    echo -e "   ${CYAN}Dashboard:${NC}  http://${SERVER_IP}:8080"
    echo -e "   ${CYAN}API:${NC}        http://${SERVER_IP}:8000"
    echo ""
    echo -e "   ${YELLOW}Manage:${NC}"
    echo -e "   cd $INSTALL_DIR && docker compose logs -f  # View logs"
    echo -e "   cd $INSTALL_DIR && docker compose down     # Stop"
    echo -e "   cd $INSTALL_DIR && docker compose up -d    # Start"
    echo ""
else
    echo ""
    echo -e "${YELLOW}⚠️  Containers built but may still be starting.${NC}"
    echo -e "${YELLOW}   Check status with: cd $INSTALL_DIR && docker compose ps${NC}"
    echo ""
    
    SERVER_IP=$(hostname -I | awk '{print $1}')
    echo -e "   ${CYAN}Dashboard:${NC}  http://${SERVER_IP}:8080"
    echo -e "   ${CYAN}API:${NC}        http://${SERVER_IP}:8000"
    echo ""
fi
