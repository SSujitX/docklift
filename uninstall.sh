#!/bin/bash
# DockLift uninstaller - removes every DockLift resource and nothing else.
set -uo pipefail

RED='\033[0;31m'
GREEN='\033[0;32m'
CYAN='\033[0;36m'
YELLOW='\033[1;33m'
BOLD='\033[1m'
DIM='\033[2m'
NC='\033[0m'

INSTALL_DIR="/opt/docklift"

# DockLift names every resource `docklift*`, `dl_*` (containers) or `dl-*` (compose
# projects, and the images/volumes compose derives from them). Anything that matches
# neither pattern belongs to another workload and is left untouched.
NAME_RE='^(docklift|dl[-_])'
PROJECT_RE='^dl-'

# Base images DockLift pulls. Removed without -f so Docker refuses while another
# workload still uses them.
BASE_IMAGES=(nginx:stable-alpine certbot/certbot)

echo -e "${RED}${BOLD}⚠️  THIS WILL DELETE ALL DOCKLIFT DATA, PROJECTS, AND DATABASES! ⚠️${NC}"

if [[ "${1:-}" == "-y" || "${1:-}" == "--force" ]]; then
    echo -e "${YELLOW}Force mode detected. Skipping confirmation.${NC}"
else
    echo -e "${YELLOW}Are you sure you want to continue? (y/N)${NC}"
    read -r response
    if [[ ! "$response" =~ ^([yY][eE][sS]|[yY])$ ]]; then
        echo "Aborted."
        exit 1
    fi
fi

if ! command -v docker &>/dev/null; then
    echo -e "${YELLOW}Docker not found - removing installation directory only.${NC}"
    rm -rf "$INSTALL_DIR"
    echo -e "${GREEN}✅ Done.${NC}"
    exit 0
fi

# Containers match on their own name or on the compose project that created them, so
# projects that set a custom container_name are still caught.
docklift_containers() {
    docker ps -a --format '{{.ID}}|{{.Names}}|{{.Label "com.docker.compose.project"}}' 2>/dev/null |
        awk -F'|' -v n="$NAME_RE" -v p="$PROJECT_RE" '$2 ~ n || $3 ~ p {print $1}'
}

docklift_images() {
    docker images --format '{{.Repository}}:{{.Tag}}' 2>/dev/null |
        awk '$0 ~ /^(docklift-|dl-)/ && $0 !~ /<none>/'
}

docklift_volumes() {
    # Names dl-* / docklift*, compose project labels, or ownership labels from runtime compose
    docker volume ls --format '{{.Name}}|{{.Labels}}' 2>/dev/null |
        awk -F'|' -v n="$NAME_RE" \
          '$1 ~ n || $2 ~ /com\.docker\.compose\.project=dl-/ || $2 ~ /com\.docklift\.(project|managed)=/ {print $1}'
}

docklift_networks() {
    # Control-plane docklift_network + per-project dl-net-* (+ label ownership)
    docker network ls --format '{{.Name}}|{{.Labels}}' 2>/dev/null |
        awk -F'|' -v n="$NAME_RE" \
          '$1 ~ n || $1 ~ /^dl-net-/ || $2 ~ /com\.docklift\.(project|managed)=/ {print $1}'
}

count() {
    [ -z "${1:-}" ] && { echo 0; return; }
    grep -c . <<<"$1"
}

# Step 1: let compose tear down the core stack it owns
if [ -f "$INSTALL_DIR/docker-compose.yml" ]; then
    printf "${CYAN}[1/6]${NC} Stopping the DockLift stack... "
    (cd "$INSTALL_DIR" && docker compose down --volumes --remove-orphans --rmi all) >/dev/null 2>&1 || true
    echo -e "${GREEN}done${NC}"
else
    echo -e "${CYAN}[1/6]${NC} No compose file at $INSTALL_DIR ${DIM}(skipping)${NC}"
fi

# Step 2: containers (core stack leftovers + every deployed project)
printf "${CYAN}[2/6]${NC} Removing containers... "
CONTAINERS=$(docklift_containers)
if [ -n "$CONTAINERS" ]; then
    docker stop $CONTAINERS >/dev/null 2>&1 || true
    docker rm -f -v $CONTAINERS >/dev/null 2>&1 || true
    echo -e "${GREEN}removed $(count "$CONTAINERS")${NC}"
else
    echo -e "${DIM}none found${NC}"
fi

# Step 3: images - DockLift's own builds, then the base images if nothing else needs them
printf "${CYAN}[3/6]${NC} Removing images... "
IMAGES=$(docklift_images)
[ -n "$IMAGES" ] && docker rmi -f $IMAGES >/dev/null 2>&1 || true
KEPT=""
for img in "${BASE_IMAGES[@]}"; do
    if docker image inspect "$img" >/dev/null 2>&1; then
        docker rmi "$img" >/dev/null 2>&1 || KEPT="$KEPT $img"
    fi
done
echo -e "${GREEN}removed $(count "$IMAGES") DockLift image(s)${NC}"
[ -n "$KEPT" ] && echo -e "        ${DIM}kept in use by other containers:$KEPT${NC}"

# Step 4: volumes and networks
printf "${CYAN}[4/6]${NC} Removing volumes and networks... "
VOLUMES=$(docklift_volumes)
[ -n "$VOLUMES" ] && docker volume rm -f $VOLUMES >/dev/null 2>&1 || true
NETWORKS=$(docklift_networks)
[ -n "$NETWORKS" ] && docker network rm $NETWORKS >/dev/null 2>&1 || true
echo -e "${GREEN}$(count "$VOLUMES") volume(s), $(count "$NETWORKS") network(s)${NC}"

# Step 5: skip host image cleanup — step 3 already removed DockLift-owned images.
# Never run host-wide prune (shared Docker hosts).
printf "${CYAN}[5/6]${NC} Skipping host image cleanup... "
echo -e "${GREEN}ok${NC}"

# Step 6: installation directory (database, deployments, backups, certificates)
printf "${CYAN}[6/6]${NC} Removing $INSTALL_DIR... "
rm -rf "$INSTALL_DIR"
echo -e "${GREEN}done${NC}"

# Verify rather than assume
LEFT_C=$(docklift_containers)
LEFT_I=$(docklift_images)
LEFT_V=$(docklift_volumes)
LEFT_N=$(docklift_networks)
if [ -n "$LEFT_C$LEFT_I$LEFT_V$LEFT_N" ] || [ -e "$INSTALL_DIR" ]; then
    echo -e "\n${RED}${BOLD}⚠ Some resources could not be removed:${NC}"
    [ -n "$LEFT_C" ] && echo -e "  containers: $(echo $LEFT_C | tr '\n' ' ')"
    [ -n "$LEFT_I" ] && echo -e "  images:     $(echo $LEFT_I | tr '\n' ' ')"
    [ -n "$LEFT_V" ] && echo -e "  volumes:    $(echo $LEFT_V | tr '\n' ' ')"
    [ -n "$LEFT_N" ] && echo -e "  networks:   $(echo $LEFT_N | tr '\n' ' ')"
    [ -e "$INSTALL_DIR" ] && echo -e "  directory:  $INSTALL_DIR"
    exit 1
fi

# Anything still holding an app port after the containers are gone is not ours to kill.
BUSY=""
if command -v ss &>/dev/null; then
    BUSY=$(ss -tln 2>/dev/null | awk '{print $4}' | grep -oE ':(55[0-9][0-9]|5600)$' | tr -d ':' | sort -un | tr '\n' ' ')
fi

echo -e "\n${GREEN}${BOLD}✅ Uninstallation complete. No DockLift resources remain.${NC}"
if [ -n "$BUSY" ]; then
    echo -e "${YELLOW}Note:${NC} ports still in use by non-DockLift processes: ${BUSY}"
fi
echo -e "${DIM}Docker Engine and git were left installed. You can now run the installer again.${NC}"
