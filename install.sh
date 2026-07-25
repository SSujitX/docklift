#!/bin/bash
set -e

# Colors & Vars
RED='\033[0;31m'; GREEN='\033[0;32m'; CYAN='\033[0;36m'; YELLOW='\033[0;33m'; BOLD='\033[1m'; DIM='\033[2m'; NC='\033[0m'
START_TIME=$(date +%s); INSTALL_DIR="/opt/docklift"

format_time() {
    local s=$1 h=0 m=0
    # `((…))` returns exit 1 when the value is 0 — must not trip `set -e`
    ((h = s / 3600, m = (s % 3600) / 60, s = s % 60)) || true
    [ $h -gt 0 ] && printf "%dh %dm %ds" $h $m $s || ([ $m -gt 0 ] && printf "%dm %ds" $m $s || printf "%ds" $s)
}

print_access_info() {
    PUB4=$(curl -4 -s --connect-timeout 2 https://api.ipify.org 2>/dev/null || echo "")
    PUB6=$(curl -6 -s --connect-timeout 2 https://api64.ipify.org 2>/dev/null || echo "")
    PRV=$(hostname -I 2>/dev/null | tr ' ' '\n' | grep -v "${PUB4:-NOT_SET}" | grep -E '^(10\.|172\.(1[6-9]|2[0-9]|3[0-1])\.|192\.168\.)' | head -1 || echo "")
    # Wait briefly for backend to write the bootstrap secret on first boot
    for _ in 1 2 3 4 5 6 7 8 9 10; do
        [ -s "$INSTALL_DIR/data/.bootstrap-secret" ] && break
        sleep 1
    done
    SETUP_CODE=$(tr -d '\n\r' < "$INSTALL_DIR/data/.bootstrap-secret" 2>/dev/null || echo "")

    echo -e "  ${GREEN}${BOLD}Docklift is ready${NC}\n"
    if [ -n "$PUB4" ]; then
        echo -e "  ${BOLD}Dashboard:${NC} http://${PUB4}:8080"
    elif [ -n "$PRV" ]; then
        echo -e "  ${BOLD}Dashboard:${NC} http://${PRV}:8080"
    else
        echo -e "  ${BOLD}Dashboard:${NC} http://SERVER_IP:8080"
    fi
    [ -n "$PUB6" ] && echo -e "  ${DIM}IPv6:${NC}      http://[${PUB6}]:8080"
    [ -n "$PUB4" ] && [ -n "$PRV" ] && echo -e "  ${DIM}Private:${NC}   http://${PRV}:8080"

    if [ -n "$SETUP_CODE" ]; then
        echo -e "\n  ${BOLD}Setup code:${NC} ${CYAN}${SETUP_CODE}${NC}"
        echo -e "  ${DIM}Open the dashboard, paste this code, and create your admin account.${NC}"
    else
        echo -e "\n  ${YELLOW}Setup code:${NC} not ready yet — run:"
        echo -e "  ${DIM}  docker logs docklift-backend | grep -A8 \"Fresh install\"${NC}"
        echo -e "  ${DIM}  # or: cat $INSTALL_DIR/data/.bootstrap-secret${NC}"
    fi
    echo -e "\n  ${DIM}HTTP on a public IP is convenient for first setup — not encrypted.${NC}"
    echo -e "  ${DIM}After login: Settings → Domain for HTTPS, or firewall / DASHBOARD_BIND=127.0.0.1.${NC}\n"
}

# Header
clear 2>/dev/null || true
echo -e "\n  ${CYAN}____             _    _ _  __ _   ${NC}"
echo -e "  ${CYAN}|  _ \\  ___   ___| | _| (_)/ _| |_ ${NC}\n  ${CYAN}| | | |/ _ \\ / __| |/ / | | |_| __|${NC}"
echo -e "  ${CYAN}| |_| | (_) | (__|   <| | |  _| |_ ${NC}\n  ${CYAN}|____/ \\___/ \\___|_|\\_\\_|_|_|  \\__|${NC}\n"
echo -e "  ${DIM}Self-Hosted Docker Deployment Platform${NC}\n"

[ "$EUID" -ne 0 ] && echo -e "  ${RED}Error: Run with sudo${NC}" && exit 1

# Pin a release: DOCKLIFT_VERSION=2.0.2 or bash -s -- v=2.0.2 (default: latest)
# Examples:
#   curl -fsSL .../install.sh | sudo bash
#   curl -fsSL .../install.sh | sudo bash -s -- v=2.0.2
#   curl -fsSL .../install.sh | sudo DOCKLIFT_VERSION=2.0.2 bash
REPO_URL="https://github.com/SSujitX/docklift.git"
REQUESTED_VERSION="${DOCKLIFT_VERSION:-}"
while [ $# -gt 0 ]; do
    case "$1" in
        v=*|version=*|--version=*)
            REQUESTED_VERSION="${1#*=}"
            ;;
        --version|-v)
            shift
            REQUESTED_VERSION="${1:-}"
            ;;
        latest|LATEST)
            REQUESTED_VERSION=""
            ;;
        v[0-9]*|[0-9]*.[0-9]*)
            REQUESTED_VERSION="$1"
            ;;
        *)
            echo -e "  ${RED}Error: unknown install arg: $1${NC}"
            echo -e "  ${DIM}Use: bash -s -- v=2.0.2   or   DOCKLIFT_VERSION=2.0.2${NC}"
            exit 1
            ;;
    esac
    shift || true
done

# Normalize to GitHub tag form (vX.Y.Z). Empty means resolve latest release.
normalize_release_tag() {
    local raw="$1"
    raw=$(printf '%s' "$raw" | tr -d '[:space:]')
    [ -z "$raw" ] && { printf ''; return 0; }
    case "$raw" in
        latest|LATEST) printf ''; return 0 ;;
        v*) printf '%s' "$raw" ;;
        *) printf 'v%s' "$raw" ;;
    esac
}

fail_fetch() {
    echo -e " ${RED}failed${NC}"
    echo -e "  ${RED}$1${NC}"
    [ -n "${2:-}" ] && echo -e "  ${DIM}$2${NC}"
    exit 1
}

TARGET_TAG=$(normalize_release_tag "$REQUESTED_VERSION")
WANT_LATEST=0
[ -z "$TARGET_TAG" ] && WANT_LATEST=1

echo -e "  ${BOLD}Starting Installation${NC}\n"
if [ "$WANT_LATEST" -eq 1 ]; then
    echo -e "  ${DIM}Requested release: ${CYAN}latest${NC}\n"
else
    echo -e "  ${DIM}Requested release: ${CYAN}${TARGET_TAG}${NC}\n"
fi

# Step 1: Requirements
printf "  ${CYAN}[1/5]${NC} Checking requirements..."
for cmd in docker git; do
    if ! command -v $cmd &>/dev/null; then
        printf " Installing $cmd..."
        if [ "$cmd" = "docker" ]; then curl -fsSL https://get.docker.com | sh -s -- --quiet >/dev/null 2>&1
        else apt-get update -qq && apt-get install -y -qq git >/dev/null 2>&1 || yum install -y git >/dev/null 2>&1 || apk add --no-cache git >/dev/null 2>&1; fi
    fi
done
echo -e " ${GREEN}done${NC}"

# Step 2: Resolve tag BEFORE stopping any running stack, then fetch/checkout
FETCH_ST=$(date +%s)
printf "  ${CYAN}[2/5]${NC} Fetching code..."

CREATED_INSTALL_DIR=0
# CI copies the local workspace — skip GitHub resolve/checkout (offline-safe).
if [ "$DOCKLIFT_CI_LOCAL" = "true" ]; then
    mkdir -p "$INSTALL_DIR" && cp -r . "$INSTALL_DIR/" && cd "$INSTALL_DIR"
    TARGET_TAG="local"
else
    if [ "$WANT_LATEST" -eq 1 ]; then
        TARGET_TAG=$(curl -fsS https://api.github.com/repos/SSujitX/docklift/releases/latest \
            | grep '"tag_name"' | head -1 | cut -d'"' -f4 || true)
        TARGET_TAG=$(printf '%s' "$TARGET_TAG" | tr -d '[:space:]')
        if [ -z "$TARGET_TAG" ]; then
            fail_fetch \
                "Could not resolve latest GitHub release tag" \
                "Check network / GitHub API, then retry. (Will not fall back to master.)"
        fi
    fi

    # Confirm the tag exists on the remote before touching a live install
    if ! git ls-remote --exit-code --tags "$REPO_URL" "refs/tags/${TARGET_TAG}" >/dev/null 2>&1; then
        fail_fetch \
            "Release tag not found: ${TARGET_TAG}" \
            "See https://github.com/SSujitX/docklift/releases (e.g. bash -s -- v=2.0.2)."
    fi

    if [ -d "$INSTALL_DIR/.git" ]; then
        cd "$INSTALL_DIR"
        # Fetch tags first; only stop compose after the pin is known-good remotely
        if ! git fetch origin --tags -q 2>/tmp/docklift-install-git.err; then
            fail_fetch "git fetch failed" "$(head -c 400 /tmp/docklift-install-git.err 2>/dev/null || true)"
        fi
        if ! git rev-parse -q --verify "refs/tags/${TARGET_TAG}" >/dev/null 2>&1 \
            && ! git rev-parse -q --verify "refs/tags/${TARGET_TAG}^{}" >/dev/null 2>&1; then
            fail_fetch \
                "Release tag not available locally after fetch: ${TARGET_TAG}" \
                "Running stack left untouched."
        fi
        docker compose down 2>/dev/null || true
        if ! git checkout -q "$TARGET_TAG" 2>/tmp/docklift-install-git.err \
            && ! git checkout -q "tags/${TARGET_TAG}" 2>/tmp/docklift-install-git.err; then
            echo -e " ${RED}failed${NC}"
            echo -e "  ${RED}Checkout failed for ${TARGET_TAG}${NC}"
            echo -e "  ${DIM}Try: cd $INSTALL_DIR && docker compose up -d${NC}"
            exit 1
        fi
    else
        if [ -e "$INSTALL_DIR" ] && [ ! -d "$INSTALL_DIR/.git" ]; then
            fail_fetch \
                "$INSTALL_DIR exists but is not a DockLift git checkout" \
                "Move/remove it, or use a clean host."
        fi
        if ! git clone -q "$REPO_URL" "$INSTALL_DIR" 2>/tmp/docklift-install-git.err; then
            fail_fetch "git clone failed" "$(head -c 400 /tmp/docklift-install-git.err 2>/dev/null || true)"
        fi
        CREATED_INSTALL_DIR=1
        cd "$INSTALL_DIR"
        git fetch origin --tags -q 2>/dev/null || true
        if ! git checkout -q "$TARGET_TAG" 2>/tmp/docklift-install-git.err \
            && ! git checkout -q "tags/${TARGET_TAG}" 2>/tmp/docklift-install-git.err; then
            [ "$CREATED_INSTALL_DIR" -eq 1 ] && rm -rf "$INSTALL_DIR"
            fail_fetch \
                "Checkout failed for ${TARGET_TAG}" \
                "See https://github.com/SSujitX/docklift/releases."
        fi
    fi
fi

echo -e " ${GREEN}done${NC} ${DIM}($(format_time $(($(date +%s) - FETCH_ST))))$NC"
VERSION=$(grep -o '"version": *"[^"]*"' "$INSTALL_DIR/backend/package.json" 2>/dev/null | head -1 | cut -d'"' -f4 || echo "1.0.0")
echo -e "        ${DIM}➞ Version: $VERSION ${CYAN}($TARGET_TAG)${NC}"

# Step 3-5: Setup & Build (keep repo default.conf — do not overwrite secure catch-all)
printf "  ${CYAN}[3/5]${NC} Creating directories... "
mkdir -p \
  "$INSTALL_DIR/data" \
  "$INSTALL_DIR/deployments" \
  "$INSTALL_DIR/backups" \
  "$INSTALL_DIR/nginx-proxy/conf.d" \
  "$INSTALL_DIR/nginx-proxy/snippets" \
  "$INSTALL_DIR/nginx-proxy/certbot/www" \
  "$INSTALL_DIR/nginx-proxy/certbot/conf"
echo -e "${GREEN}done${NC}"

printf "  ${CYAN}[4/5]${NC} Cleaning network... " && (docker network rm docklift_network 2>/dev/null || true) && echo -e "${GREEN}done${NC}"

BUILD_ST=$(date +%s); echo -e "\n  ${CYAN}[5/5]${NC} Building containers...\n        ${DIM}This may take a few minutes...${NC}"

cd "$INSTALL_DIR"
LOG=$(mktemp)
if ! docker compose up -d --build --remove-orphans > "$LOG" 2>&1; then
    echo -e "\n  ${RED}Build failed!${NC}"; cat "$LOG"; rm "$LOG"; exit 1
fi
rm "$LOG"; sleep 5

# Results
TOTAL_TIME=$(($(date +%s) - START_TIME)); BUILD_TIME=$(($(date +%s) - BUILD_ST))
RUNNING=$(docker compose ps --format "{{.Name}}" 2>/dev/null | grep -c "docklift" || true)
RUNNING=${RUNNING:-0}

if [ "$RUNNING" -gt 0 ]; then
    echo -e "\n  ${GREEN}${BOLD}Installation Complete!${NC}\n  ${DIM}Build: $(format_time $BUILD_TIME) | Total: $(format_time $TOTAL_TIME)${NC}\n"
    if [ "$CI" != "true" ]; then
        print_access_info
    else echo -e "  ${DIM}Version: $VERSION | Build: $(format_time $BUILD_TIME)${NC}\n"
    fi
else echo -e "  ${RED}Error: Containers not running${NC}\n  ${DIM}Run: cd $INSTALL_DIR && docker compose logs${NC}\n"
fi
