#!/bin/bash
set -e

START_TIME=$(date +%s); INSTALL_DIR="${DOCKLIFT_INSTALL_DIR:-/opt/docklift}"
# Containers of the 2.1.x layout that the Go backend replaces (it now owns :8080).
OBSOLETE_CONTAINERS="docklift-frontend docklift-nginx"
HEALTH_TIMEOUT=180

# >>> docklift-ui >>>
# Shared progress helpers. Each script is fetched on its own (curl | sudo bash)
# and cannot source a file, so this block is copied verbatim into install.sh,
# install-dev.sh, upgrade.sh and uninstall.sh; scripts/test-ui-helpers.sh (CI)
# fails when the copies differ. Edit one copy, then paste it into the others.
#
#   ui_steps N           number of fixed steps, for the overall progress bar
#   ui_step "Title"      start the next step (prints the bar)
#   ui_run "Label" cmd…  run cmd in the background, output appended to $UI_LOG,
#                        spinner + elapsed time; returns cmd's exit status. On
#                        failure prints the last lines of its output.
#   ui_note "text"       called by a ui_run command: shown next to its spinner
#   ui_ok / ui_info / ui_warn / ui_fail "text"   one-line messages
#   ui_swap_ensure       temporary swap for the image build on small hosts
#   ui_cleanup           stop a running step, restore the cursor, remove swap
#
# Animation only when stdout is a terminal and neither CI=true, TERM=dumb nor
# NO_COLOR is set; otherwise plain lines without colour or carriage returns.
UI_FANCY=0
if [ -t 1 ] && [ "${CI:-}" != "true" ] && [ "${TERM:-dumb}" != "dumb" ] && [ -z "${NO_COLOR:-}" ]; then
    UI_FANCY=1
fi
if [ "$UI_FANCY" = 1 ]; then
    RED=$'\033[0;31m'; GREEN=$'\033[0;32m'; CYAN=$'\033[0;36m'; YELLOW=$'\033[0;33m'
    BOLD=$'\033[1m'; DIM=$'\033[2m'; NC=$'\033[0m'
else
    RED=''; GREEN=''; CYAN=''; YELLOW=''; BOLD=''; DIM=''; NC=''
fi
case "${LC_ALL:-${LC_CTYPE:-${LANG:-}}}" in
    *[Uu][Tt][Ff]-8*|*[Uu][Tt][Ff]8*)
        UI_SPIN=(⠋ ⠙ ⠹ ⠸ ⠼ ⠴ ⠦ ⠧ ⠇ ⠏); UI_MARK_OK="✓"; UI_MARK_FAIL="✗"; UI_MARK_WARN="!"
        UI_BAR_ON="█"; UI_BAR_OFF="░"; UI_ELL="…" ;;
    *)
        UI_SPIN=('-' "\\" '|' '/'); UI_MARK_OK="+"; UI_MARK_FAIL="x"; UI_MARK_WARN="!"
        UI_BAR_ON="#"; UI_BAR_OFF="-"; UI_ELL="..." ;;
esac
UI_LOG="${UI_LOG:-${TMPDIR:-/tmp}/docklift.log}"
UI_NOTE=""
UI_PID=""
UI_LABEL=""
UI_TOTAL=0
UI_STEP=0
UI_TAIL_SHOWN=0
UI_TAIL_LINES=40
UI_MEMINFO="${UI_MEMINFO:-/proc/meminfo}"
UI_SWAP_FILE=/var/tmp/docklift-build.swap
UI_SWAP_MADE=0
UI_SWAP_BELOW_MB=1900   # RAM + swap under this: add a temporary swap file …
UI_SWAP_TARGET_MB=2560  # … that brings the total to this
UI_RAM_HINT_MB=1900     # below this: recommend 2 GB RAM after the build

# ui_log_init PATH: start a fresh log at PATH (a temp file when not writable)
ui_log_init() {
    UI_LOG=$1
    if ! { : >"$UI_LOG"; } 2>/dev/null; then
        UI_LOG=$(mktemp "${TMPDIR:-/tmp}/docklift.XXXXXX")
    fi
    return 0
}

# "1m 05s" style durations for step results; m:ss for the live counter
ui_dur() {
    local s=$1
    if [ "$s" -ge 3600 ]; then printf '%dh %02dm' $((s / 3600)) $((s % 3600 / 60))
    elif [ "$s" -ge 60 ]; then printf '%dm %02ds' $((s / 60)) $((s % 60))
    else printf '%ds' "$s"
    fi
}

ui_cols() {
    local c
    c=$( { stty size </dev/tty; } 2>/dev/null | awk '{print $2}') || true
    case "$c" in ''|0|*[!0-9]*) c=80 ;; esac
    [ "$c" -ge 30 ] || c=30
    printf '%s' "$c"
}

ui_ok()   { printf '    %s%s%s %s\n' "$GREEN" "$UI_MARK_OK" "$NC" "$*"; }
ui_info() { printf '    %s%s%s\n' "$DIM" "$*" "$NC"; }
ui_warn() { printf '    %s%s %s%s\n' "$YELLOW" "$UI_MARK_WARN" "$*" "$NC"; }
ui_fail() { printf '    %s%s %s%s\n' "$RED" "$UI_MARK_FAIL" "$*" "$NC"; }

ui_steps() { UI_TOTAL=$1; UI_STEP=0; }

ui_bar() {
    local filled=$1 width=20 i s=""
    [ "$UI_TOTAL" -gt 0 ] || { printf ''; return 0; }
    filled=$((filled * width / UI_TOTAL))
    for ((i = 0; i < width; i++)); do
        if [ "$i" -lt "$filled" ]; then s+="$UI_BAR_ON"; else s+="$UI_BAR_OFF"; fi
    done
    printf '%s' "$s"
}

ui_step() {
    UI_STEP=$((UI_STEP + 1))
    if [ "$UI_FANCY" = 1 ]; then
        printf '\n  %s[%s]%s  %s%d/%d%s  %s%s%s\n' "$CYAN" "$(ui_bar $((UI_STEP - 1)))" "$NC" \
            "$DIM" "$UI_STEP" "$UI_TOTAL" "$NC" "$BOLD" "$*" "$NC"
    else
        printf '\n  [%d/%d] %s\n' "$UI_STEP" "$UI_TOTAL" "$*"
    fi
}

# ui_finish "text": full progress bar (terminal only) once every step is done
ui_finish() {
    if [ "$UI_FANCY" = 1 ]; then
        printf '\n  %s[%s]%s  %s%d/%d%s  %s%s%s\n' "$CYAN" "$(ui_bar "$UI_TOTAL")" "$NC" \
            "$DIM" "$UI_TOTAL" "$UI_TOTAL" "$NC" "$BOLD" "$*" "$NC"
    fi
    return 0
}

ui_note() {
    if [ -n "${UI_NOTE:-}" ]; then printf '%s\n' "$*" >"$UI_NOTE" 2>/dev/null || true; fi
    printf '%s\n' "$*"
    return 0
}

ui_size() { wc -c <"$1" 2>/dev/null | tr -d ' ' || echo 0; }

ui_mem_mb() {
    local kb
    kb=$(awk -v k="$1:" '$1 == k { print $2; exit }' "$UI_MEMINFO" 2>/dev/null) || true
    case "$kb" in ''|*[!0-9]*) return 1 ;; esac
    printf '%s' $((kb / 1024))
}

# Last lines of the output since byte OFFSET of $UI_LOG, the log path, and a
# plain explanation when the command was killed for lack of memory.
ui_show_tail() {
    local off=$1 rc=$2 tail_txt mem swap
    tail_txt=$(tail -c +"$((off + 1))" "$UI_LOG" 2>/dev/null | tr '\r' '\n' | grep -v '^[[:space:]]*$' | tail -n "$UI_TAIL_LINES") || true
    if [ -n "$tail_txt" ]; then
        printf '      %sLast lines of the output:%s\n' "$DIM" "$NC"
        printf '%s\n' "$tail_txt" | sed 's/^/        /'
    fi
    printf '      %sFull log: %s%s\n' "$DIM" "$UI_LOG" "$NC"
    # shellcheck disable=SC2034  # read by upgrade.sh's rollback
    UI_TAIL_SHOWN=1
    if [ "$rc" -eq 137 ] || printf '%s\n' "$tail_txt" \
        | grep -qE '(^|[[:space:]])Killed[[:space:]]*$|exit code: 137|code 137([^0-9]|$)|signal: killed|[Oo]ut of memory'; then
        mem=$(ui_mem_mb MemTotal || echo '?'); swap=$(ui_mem_mb SwapTotal || echo '?')
        printf '      %sThis ran out of memory: the kernel killed it (exit 137). The server has %s MB RAM and %s MB swap.%s\n' \
            "$RED" "$mem" "$swap" "$NC"
        printf '      %sAdd swap (e.g. 2 GB), free memory or use a server with 2 GB RAM or more, then run this script again.%s\n' \
            "$RED" "$NC"
    fi
    return 0
}

# Process tree of PID (PID first), so a stopped step leaves nothing behind.
ui_tree() {
    local pid=$1 kid
    printf '%s ' "$pid"
    for kid in $( { ps -eo pid=,ppid= 2>/dev/null || ps -o pid,ppid 2>/dev/null; } \
        | awk -v p="$pid" '$2 == p { print $1 }'); do
        ui_tree "$kid"
    done
}

# Stop the running step (Ctrl-C, a signal or an exit while it runs) and give the cursor back.
ui_stop() {
    local pids i
    if [ -n "${UI_PID:-}" ]; then
        pids=$(ui_tree "$UI_PID")
        # shellcheck disable=SC2086
        kill -TERM $pids 2>/dev/null || true
        for i in 1 2 3 4 5 6 7 8 9 10; do
            kill -0 "$UI_PID" 2>/dev/null || break
            sleep 0.3 2>/dev/null || sleep 1
        done
        # shellcheck disable=SC2086
        kill -KILL $pids 2>/dev/null || true
        wait "$UI_PID" 2>/dev/null || true
        UI_PID=""
        if [ "$UI_FANCY" = 1 ]; then printf '\r\033[K'; else printf '\n'; fi
        ui_fail "$UI_LABEL: interrupted"
    fi
    [ "$UI_FANCY" = 1 ] && printf '\033[?25h'
    [ -n "${UI_NOTE:-}" ] && rm -f "$UI_NOTE"
    return 0
}

ui_run() {
    local label=$1 start off rc=0 now el frame=0 note="" shown="" stage="" line cols max
    shift
    UI_LABEL=$label
    UI_NOTE="$UI_LOG.note"
    : >"$UI_NOTE" 2>/dev/null || UI_NOTE=""
    printf -- '--- %s (%s)\n' "$label" "$(date '+%F %T')" >>"$UI_LOG"
    off=$(ui_size "$UI_LOG")
    printf -v start '%(%s)T' -1
    "$@" </dev/null >>"$UI_LOG" 2>&1 &
    UI_PID=$!
    if [ "$UI_FANCY" = 1 ]; then
        cols=$(ui_cols); max=$((cols - 7))
        printf '\033[?25l'
    else
        printf '    %s...' "$label"
    fi
    while kill -0 "$UI_PID" 2>/dev/null; do
        if [ -n "$UI_NOTE" ] && { [ "$UI_FANCY" != 1 ] || [ $((frame % 5)) -eq 0 ]; }; then
            note=""; read -r note <"$UI_NOTE" 2>/dev/null || true
        fi
        if [ "$UI_FANCY" = 1 ]; then
            if [ $((frame % 10)) -eq 0 ]; then
                stage=$(tail -c +"$((off + 1))" "$UI_LOG" 2>/dev/null | tail -n 60 \
                    | grep -E '^#[0-9]+ \[[^]]+\] ' | tail -n 1 | tr -d '\r') || true
                stage=${stage#\#* }
            fi
            printf -v now '%(%s)T' -1; el=$((now - start))
            line="$label$UI_ELL $(printf '%d:%02d' $((el / 60)) $((el % 60)))"
            [ -n "$note" ] && line+="  $note"
            [ -n "$stage" ] && line+="  $stage"
            printf '\r\033[K    %s%s%s %s' "$CYAN" "${UI_SPIN[frame % ${#UI_SPIN[@]}]}" "$NC" "${line:0:max}"
            sleep 0.1 2>/dev/null || sleep 1
        else
            if [ -n "$note" ] && [ "$note" != "$shown" ]; then
                printf ' %s...' "$note"; shown=$note
            fi
            # quick steps finish without a full second's wait
            if [ "$frame" -lt 10 ]; then sleep 0.2 2>/dev/null || sleep 1; else sleep 1; fi
        fi
        frame=$((frame + 1))
    done
    wait "$UI_PID" || rc=$?
    UI_PID=""
    printf -v now '%(%s)T' -1; el=$((now - start))
    [ -n "$UI_NOTE" ] && rm -f "$UI_NOTE"
    UI_NOTE=""
    if [ "$UI_FANCY" = 1 ]; then
        printf '\r\033[K\033[?25h'
        if [ "$rc" -eq 0 ]; then
            printf '    %s%s%s %s %s(%s)%s\n' "$GREEN" "$UI_MARK_OK" "$NC" "$label" "$DIM" "$(ui_dur "$el")" "$NC"
        else
            printf '    %s%s %s%s %s(failed after %s, exit %d)%s\n' "$RED" "$UI_MARK_FAIL" "$label" "$NC" \
                "$DIM" "$(ui_dur "$el")" "$rc" "$NC"
        fi
    elif [ "$rc" -eq 0 ]; then
        printf ' done (%s)\n' "$(ui_dur "$el")"
    else
        printf ' failed (%s, exit %d)\n' "$(ui_dur "$el")" "$rc"
    fi
    if [ "$rc" -eq 130 ] || [ "$rc" -eq 143 ]; then
        # Ctrl-C reached the command itself: no failure output to show
        printf '      %sInterrupted. Full log: %s%s\n' "$DIM" "$UI_LOG" "$NC"
        # shellcheck disable=SC2034  # read by upgrade.sh's rollback
        UI_TAIL_SHOWN=1
    elif [ "$rc" -ne 0 ] && [ "${UI_QUIET:-0}" != 1 ]; then
        ui_show_tail "$off" "$rc"
    fi
    printf -- '--- %s: exit %d after %ss\n' "$label" "$rc" "$el" >>"$UI_LOG"
    return "$rc"
}

# Runs in the background (ui_run): create and enable a swap file of $1 MB.
ui_swap_make() {
    local f=$UI_SWAP_FILE mb=$1 try
    for try in fallocate dd; do
        swapoff "$f" 2>/dev/null || true
        rm -f "$f"
        (umask 077 && : >"$f") || return 1
        chmod 600 "$f" || return 1
        chattr +C "$f" 2>/dev/null || true   # btrfs: swap files must be NOCOW
        if [ "$try" = fallocate ]; then
            fallocate -l "${mb}M" "$f" || continue
        else
            dd if=/dev/zero of="$f" bs=1M count="$mb" || return 1
        fi
        mkswap "$f" >/dev/null && swapon "$f" && return 0
        echo "swapon refused the $try-created file"
    done
    return 1
}

# A swap file left by an earlier run that was killed before its cleanup ran.
ui_swap_stale() {
    if [ -e "$UI_SWAP_FILE" ]; then
        swapoff "$UI_SWAP_FILE" 2>/dev/null || true
        rm -f "$UI_SWAP_FILE" 2>/dev/null || true
    fi
    return 0
}

# Image builds need about 2 GB of RAM + swap. Below that, add a temporary swap
# file (never touches /etc/fstab or existing swap); ui_cleanup removes it.
ui_swap_ensure() {
    local mem swap total need free dir=${UI_SWAP_FILE%/*}
    ui_swap_stale
    mem=$(ui_mem_mb MemTotal) || return 0
    swap=$(ui_mem_mb SwapTotal) || swap=0
    total=$((mem + swap))
    [ "$total" -lt "$UI_SWAP_BELOW_MB" ] || return 0
    if [ "${DOCKLIFT_NO_SWAP:-}" = "1" ]; then
        ui_warn "Low memory ($mem MB RAM + $swap MB swap); DOCKLIFT_NO_SWAP=1, so no temporary swap. The build may run out of memory."
        return 0
    fi
    need=$((UI_SWAP_TARGET_MB - total))
    if ! command -v swapon >/dev/null 2>&1 || ! command -v mkswap >/dev/null 2>&1; then
        ui_warn "Low memory ($mem MB RAM + $swap MB swap) and no swapon/mkswap here: no temporary swap. The build may run out of memory."
        return 0
    fi
    free=$(df -Pk "$dir" 2>/dev/null | awk 'NR == 2 { print int($4 / 1024) }')
    case "$free" in ''|*[!0-9]*) free=0 ;; esac
    if [ "$free" -lt $((need + 1024)) ]; then
        ui_warn "Low memory ($mem MB RAM + $swap MB swap) but only $free MB free in $dir (need $((need + 1024)) MB): no temporary swap. The build may run out of memory."
        return 0
    fi
    ui_info "Low memory ($mem MB RAM + $swap MB swap): adding a temporary $need MB swap file for the build; it is removed when this script ends."
    UI_SWAP_MADE=1
    if UI_QUIET=1 ui_run "Adding temporary swap ($UI_SWAP_FILE)" ui_swap_make "$need" \
        && grep -q "^${UI_SWAP_FILE}[[:space:]]" /proc/swaps 2>/dev/null; then
        return 0
    fi
    swapoff "$UI_SWAP_FILE" 2>/dev/null || true
    rm -f "$UI_SWAP_FILE" 2>/dev/null || true
    UI_SWAP_MADE=0
    ui_warn "This system refused a swap file (container, or a filesystem without swap file support); continuing without. The build may run out of memory."
    return 0
}

ui_swap_release() {
    [ "$UI_SWAP_MADE" = 1 ] || return 0
    UI_SWAP_MADE=0
    swapoff "$UI_SWAP_FILE" 2>/dev/null || true
    rm -f "$UI_SWAP_FILE" 2>/dev/null || true
    if [ -e "$UI_SWAP_FILE" ]; then
        ui_warn "Could not remove the temporary swap file; run: swapoff $UI_SWAP_FILE && rm -f $UI_SWAP_FILE"
    else
        ui_info "Removed the temporary swap file."
    fi
    return 0
}

# One dim line after a build on a host under ~2 GB of RAM.
ui_ram_hint() {
    local mem
    mem=$(ui_mem_mb MemTotal) || return 0
    if [ "$mem" -lt "$UI_RAM_HINT_MB" ]; then
        ui_info "This server has $mem MB RAM; 2 GB or more is recommended to build apps comfortably."
    fi
    return 0
}

# For the EXIT trap: also runs on failure and Ctrl-C.
ui_cleanup() {
    ui_stop
    ui_swap_release
    return 0
}
# <<< docklift-ui <<<

format_time() {
    local s=$1 h=0 m=0
    # `((…))` returns exit 1 when the value is 0 — must not trip `set -e`
    ((h = s / 3600, m = (s % 3600) / 60, s = s % 60)) || true
    if [ "$h" -gt 0 ]; then printf "%dh %dm %ds" "$h" "$m" "$s"
    elif [ "$m" -gt 0 ]; then printf "%dm %ds" "$m" "$s"
    else printf "%ds" "$s"
    fi
}

# Probe the API inside the backend container (busybox wget; no host curl/port assumptions)
health_ok() {
    docker exec docklift-backend wget -q -O /dev/null http://127.0.0.1:8000/api/health >/dev/null 2>&1
}

# Every service of docker-compose.yml must be running.
stack_running() {
    local want have svc
    want=$(docker compose config --services 2>/dev/null) || return 1
    have=$(docker compose ps --status running --services 2>/dev/null) || return 1
    for svc in $want; do
        printf '%s\n' "$have" | grep -qx "$svc" || return 1
    done
}

wait_healthy() {
    local deadline=$(( $(date +%s) + HEALTH_TIMEOUT ))
    while [ "$(date +%s)" -lt "$deadline" ]; do
        if health_ok && stack_running; then
            return 0
        fi
        sleep 3
    done
    return 1
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
        echo -e "  ${DIM}  docker logs docklift-backend 2>&1 | grep -A8 \"Fresh install\"${NC}"
        echo -e "  ${DIM}  # or: cat $INSTALL_DIR/data/.bootstrap-secret${NC}"
    fi
    echo -e "\n  ${DIM}HTTP on a public IP is convenient for first setup — not encrypted.${NC}"
    echo -e "  ${DIM}After login: Settings → Domain for HTTPS, or firewall / DASHBOARD_BIND=127.0.0.1.${NC}\n"
}

# Installs Docker with Docker's own script (runs under ui_run). A freshly booted
# server often holds the apt lock for a minute (cloud-init, unattended-upgrades),
# so retry; ui_run shows the installer's output if every attempt fails.
install_docker() {
    local script=/tmp/docklift-get-docker.sh try
    for try in 1 2 3 4; do
        if curl -fsSL https://get.docker.com -o "$script" && sh "$script"; then
            rm -f "$script"
            return 0
        fi
        rm -f "$script"
        if [ "$try" -lt 4 ]; then
            ui_note "retrying ($((try + 1))/4)"
            sleep 30
        fi
    done
    return 1
}

install_git() {
    { apt-get update -qq && DEBIAN_FRONTEND=noninteractive apt-get install -y -qq git; } \
        || yum install -y git || apk add --no-cache git
}

# Installed compose file: also stops a 2.1.x frontend + nginx gateway
stop_stack() {
    docker compose down || true
}

fetch_master() {
    git fetch origin master -q && git reset --hard origin/master -q
}

copy_local() {
    mkdir -p "$INSTALL_DIR" && cp -r . "$INSTALL_DIR/"
}

trap ui_cleanup EXIT
trap 'exit 130' INT
trap 'exit 143' TERM

# Header
[ "$UI_FANCY" = 1 ] && { clear 2>/dev/null || true; }
echo -e "\n  ${CYAN}____             _    _ _  __ _   ${NC}"
echo -e "  ${CYAN}|  _ \\  ___   ___| | _| (_)/ _| |_ ${NC}\n  ${CYAN}| | | |/ _ \\ / __| |/ / | | |_| __|${NC}"
echo -e "  ${CYAN}| |_| | (_) | (__|   <| | |  _| |_ ${NC}\n  ${CYAN}|____/ \\___/ \\___|_|\\_\\_|_|_|  \\__|${NC}\n"
echo -e "  ${DIM}Self-Hosted Docker Deployment Platform${NC}"
echo -e "  ${YELLOW}${BOLD}⚠ DEVELOPMENT BUILD${NC}\n"

[ "$EUID" -ne 0 ] && echo -e "  ${RED}Error: Run with sudo${NC}" && exit 1
echo -e "  ${BOLD}Starting Development Installation${NC}\n"

# Warning
echo -e "  ${YELLOW}Warning: This installs the latest development code from master branch."
echo -e "  For production use, run: curl -sSL https://raw.githubusercontent.com/SSujitX/docklift/master/install.sh | sudo bash${NC}\n"

ui_log_init /var/log/docklift-install.log
echo -e "  ${DIM}Log: $UI_LOG${NC}"
ui_steps 5

# Step 1: Requirements
ui_step "Checking requirements"
if ! command -v docker >/dev/null 2>&1; then
    ui_run "Installing Docker" install_docker || {
        ui_info "Install Docker manually (https://docs.docker.com/engine/install/), then retry."
        exit 1
    }
fi
if ! command -v git >/dev/null 2>&1; then
    ui_run "Installing git" install_git || { ui_info "Install git with your package manager, then retry."; exit 1; }
fi
if ! docker compose version >/dev/null 2>&1; then
    ui_fail "The Docker Compose plugin (docker compose) is required."
    exit 1
fi
ui_ok "Docker, Docker Compose and git are available"

# Step 2: Fetch latest from master
ui_step "Fetching development code"
FETCH_ST=$(date +%s)
if [ "${DOCKLIFT_CI_LOCAL:-}" = "true" ]; then
    ui_run "Copying the local checkout to $INSTALL_DIR" copy_local
elif [ -d "$INSTALL_DIR/.git" ]; then
    cd "$INSTALL_DIR"
    ui_run "Stopping the running stack" stop_stack
    ui_run "Fetching master" fetch_master
else
    ui_run "Cloning https://github.com/SSujitX/docklift.git" git clone -q https://github.com/SSujitX/docklift.git "$INSTALL_DIR"
fi
cd "$INSTALL_DIR"
# Root package.json is the single source of the release version
VERSION=$(grep -o '"version": *"[^"]*"' "$INSTALL_DIR/package.json" 2>/dev/null | head -1 | cut -d'"' -f4 || true)
VERSION=${VERSION:-dev}
COMMIT=$(cd "$INSTALL_DIR" && git rev-parse --short HEAD 2>/dev/null || echo "unknown")
ui_ok "Version $VERSION (master@$COMMIT) $(format_time $(($(date +%s) - FETCH_ST)))"

# Step 3: directories (keep repo default.conf — do not overwrite secure catch-all)
ui_step "Creating directories"
mkdir -p \
  "$INSTALL_DIR/data" \
  "$INSTALL_DIR/deployments" \
  "$INSTALL_DIR/backups" \
  "$INSTALL_DIR/nginx-proxy/conf.d" \
  "$INSTALL_DIR/nginx-proxy/snippets" \
  "$INSTALL_DIR/nginx-proxy/certbot/www" \
  "$INSTALL_DIR/nginx-proxy/certbot/conf"
ui_ok "Data directories in $INSTALL_DIR"

ui_step "Cleaning network"
docker network rm docklift_network >/dev/null 2>&1 || true
ui_ok "docklift_network will be recreated"

# Step 5: build and start
ui_step "Building and starting Docklift"
BUILD_ST=$(date +%s)
cd "$INSTALL_DIR"
# Leftovers of a 2.1.x layout would hold :8080 and the container names
for c in $OBSOLETE_CONTAINERS; do
    docker rm -f "$c" >/dev/null 2>&1 || true
done
ui_swap_ensure
ui_run "Building the Docklift image (a few minutes)" docker compose build --no-cache
ui_ram_hint
ui_run "Starting containers" docker compose up -d --remove-orphans

if ! ui_run "Waiting for /api/health" wait_healthy; then
    ui_fail "Docklift did not become healthy within ${HEALTH_TIMEOUT}s"
    docker compose ps 2>&1 || true
    docker logs --tail 50 docklift-backend 2>&1 || true
    echo -e "\n  ${DIM}Run: cd $INSTALL_DIR && docker compose logs${NC}\n"
    exit 1
fi
# The 2.1.x dashboard image is not used by the new layout
docker rmi docklift-frontend docklift-frontend:pre-upgrade >/dev/null 2>&1 || true
ui_swap_release
ui_finish "Development installation complete"

TOTAL_TIME=$(($(date +%s) - START_TIME)); BUILD_TIME=$(($(date +%s) - BUILD_ST))
echo -e "\n  ${GREEN}${BOLD}Development Installation Complete!${NC}\n  ${DIM}Build: $(format_time $BUILD_TIME) | Total: $(format_time $TOTAL_TIME)${NC}\n"
if [ "${CI:-}" != "true" ]; then
    print_access_info
    echo -e "  ${YELLOW}${BOLD}Note:${NC} ${DIM}This is a development build. For updates, re-run this script.${NC}"
    echo -e "  ${DIM}For stable releases, use: curl -sSL https://raw.githubusercontent.com/SSujitX/docklift/master/install.sh | sudo bash${NC}\n"
else
    echo -e "  ${DIM}Version: $VERSION@$COMMIT | Build: $(format_time $BUILD_TIME)${NC}\n"
fi
