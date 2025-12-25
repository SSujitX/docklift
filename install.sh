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

echo -e "${YELLOW}➜ Checking system requirements...${NC}"

# Check for Docker
if ! command -v docker &> /dev/null; then
    echo -e "${YELLOW}➜ Installing Docker...${NC}"
    curl -fsSL https://get.docker.com | sh
    systemctl enable docker
    systemctl start docker
    echo -e "${GREEN}✓ Docker installed${NC}"
else
    echo -e "${GREEN}✓ Docker found${NC}"
fi

# Check for Docker Compose
if ! docker compose version &> /dev/null; then
    echo -e "${YELLOW}➜ Installing Docker Compose...${NC}"
    apt-get update && apt-get install -y docker-compose-plugin
    echo -e "${GREEN}✓ Docker Compose installed${NC}"
else
    echo -e "${GREEN}✓ Docker Compose found${NC}"
fi

# Check for Git
if ! command -v git &> /dev/null; then
    echo -e "${YELLOW}➜ Installing Git...${NC}"
    apt-get update && apt-get install -y git
    echo -e "${GREEN}✓ Git installed${NC}"
else
    echo -e "${GREEN}✓ Git found${NC}"
fi

# Set install directory
INSTALL_DIR="/opt/docklift"

# Clone or update repo
if [ -d "$INSTALL_DIR" ]; then
    echo -e "${YELLOW}➜ Updating existing installation...${NC}"
    cd "$INSTALL_DIR"
    git pull origin master
else
    echo -e "${YELLOW}➜ Cloning Docklift...${NC}"
    git clone https://github.com/SSujitX/docklift.git "$INSTALL_DIR"
    cd "$INSTALL_DIR"
fi

# Create Docker network if not exists
echo -e "${YELLOW}➜ Creating Docker network...${NC}"
docker network create docklift_network 2>/dev/null || true
echo -e "${GREEN}✓ Docker network ready${NC}"

# Create data directories
mkdir -p "$INSTALL_DIR/data"
mkdir -p "$INSTALL_DIR/deployments"
mkdir -p "$INSTALL_DIR/nginx-proxy/conf.d"

# Build and start
echo -e "${YELLOW}➜ Building and starting Docklift...${NC}"
docker compose up -d --build

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
