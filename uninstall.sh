#!/bin/bash
set -e

# Colors
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m'

echo -e "${RED}${BOLD}⚠️  THIS WILL DELETE ALL DOCKLIFT DATA, PROJECTS, AND DATABASES! ⚠️${NC}"
echo -e "${YELLOW}Are you sure you want to continue? (y/N)${NC}"
read -r response

if [[ ! "$response" =~ ^([yY][eE][sS]|[yY])$ ]]; then
    echo "Aborted."
    exit 1
fi

echo -e "${YELLOW}Stopping Docklift containers...${NC}"
cd /opt/docklift 2>/dev/null || true
if command -v docker >/dev/null; then
    docker compose down --volumes --remove-orphans 2>/dev/null || true
fi

echo -e "${YELLOW}Cleaning up Docker resources...${NC}"
# Remove only docklift network/resources if possible, but prune is safer for clean slate
docker network rm docklift_network 2>/dev/null || true

echo -e "${YELLOW}Removing installation directory (/opt/docklift)...${NC}"
rm -rf /opt/docklift

echo -e "${GREEN}✅ Uninstallation Complete. Your system is clean.${NC}"
echo -e "You can now run the installer again for a fresh start."
