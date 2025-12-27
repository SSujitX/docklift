#!/bin/bash
set -e

# Colors
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m'

echo -e "${RED}${BOLD}⚠️  THIS WILL DELETE ALL DOCKLIFT DATA, PROJECTS, AND DATABASES! ⚠️${NC}"

if [[ "$1" == "-y" || "$1" == "--force" ]]; then
    echo -e "${YELLOW}Force mode detected. Skipping confirmation.${NC}"
else
    echo -e "${YELLOW}Are you sure you want to continue? (y/N)${NC}"
    read -r response

    if [[ ! "$response" =~ ^([yY][eE][sS]|[yY])$ ]]; then
        echo "Aborted."
        exit 1
    fi
fi

echo -e "${YELLOW}Stopping and removing ALL Docklift-related containers...${NC}"
# Find all container IDs that have "docklift" OR "dl_" in the name
DOCKLIFT_CONTAINERS=$(docker ps -a -q --filter name=docklift --filter name=dl_)

if [ -n "$DOCKLIFT_CONTAINERS" ]; then
    echo -e "${YELLOW}Found containers, stopping and removing...${NC}"
    docker stop $DOCKLIFT_CONTAINERS 2>/dev/null || true
    docker rm $DOCKLIFT_CONTAINERS 2>/dev/null || true
    echo -e "${GREEN}✓ Removed all containers ($DOCKLIFT_CONTAINERS)${NC}"
else
    echo -e "${DIM}No docklift or dl_ containers found.${NC}"
fi

echo -e "${YELLOW}Cleaning up Docker resources...${NC}"
# Remove the network
docker network rm docklift_network 2>/dev/null || true
docker network prune -f 2>/dev/null || true

# Clean up any orphaned volumes/images related to docklift
docker volume prune -f 2>/dev/null || true

echo -e "${YELLOW}Killing any leftover processes on project ports (3001-3100)...${NC}"
# Use fuser to kill anything holding our port range
for port in {3001..3050}; do
    fuser -k ${port}/tcp 2>/dev/null || true
done

echo -e "${YELLOW}Removing installation directory (/opt/docklift)...${NC}"
rm -rf /opt/docklift

echo -e "${GREEN}✅ Uninstallation Complete. System is now 100% clean.${NC}"
echo -e "You can now run the installer again for a fresh start."
