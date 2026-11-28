#!/bin/bash
# DockLift uninstaller - removes every DockLift resource and nothing else.
set -uo pipefail

INSTALL_DIR="${DOCKLIFT_INSTALL_DIR:-/opt/docklift}"

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

# DockLift names every resource `docklift*`, `dl_*` (containers) or `dl-*` (compose
# projects, and the images/volumes compose derives from them). Anything that matches
# neither pattern belongs to another workload and is left untouched.
NAME_RE='^(docklift|dl[-_])'
PROJECT_RE='^dl-'

# Base images DockLift pulls. Removed without -f so Docker refuses while another
# workload still uses them.
# Pinned tags from docker-compose.yml, plus the unpinned tags of 2.x installs.
BASE_IMAGES=(nginx:1.30.8-alpine certbot/certbot:v5.10.0 nginx:stable-alpine certbot/certbot)

echo -e "${RED}${BOLD}⚠️  THIS WILL DELETE ALL DOCKLIFT DATA, PROJECTS, AND DATABASES! ⚠️${NC}"

if [[ "${1:-}" == "-y" || "${1:-}" == "--force" ]]; then
    echo -e "${YELLOW}Force mode detected. Skipping confirmation.${NC}"
else
    echo -e "${YELLOW}Are you sure you want to continue? (y/N)${NC}"
    # curl … | sudo bash: stdin is the script itself, so ask the terminal
    response=""
    if [ -t 0 ]; then
        read -r response
    elif { : </dev/tty; } 2>/dev/null; then
        read -r response </dev/tty
    fi
    if [[ ! "$response" =~ ^([yY][eE][sS]|[yY])$ ]]; then
        echo "Aborted."
        exit 1
    fi
fi

trap ui_cleanup EXIT
trap 'exit 130' INT
trap 'exit 143' TERM

if ! command -v docker &>/dev/null; then
    echo -e "${YELLOW}Docker not found - removing installation directory only.${NC}"
    rm -rf "$INSTALL_DIR"
    echo -e "${GREEN}✅ Done.${NC}"
    exit 0
fi

# A temp log, deleted when nothing is left behind (kept and named otherwise)
ui_log_init "$(mktemp "${TMPDIR:-/tmp}/docklift-uninstall.XXXXXX")"
ui_steps 6

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

# Each runs under ui_run; failures are reported by the final verification.
stop_stack() {
    (cd "$INSTALL_DIR" && docker compose down --volumes --remove-orphans --rmi all) || true
}

remove_containers() {
    docker stop "$@" || true
    docker rm -f -v "$@" || true
}

remove_images() {
    [ "$#" -gt 0 ] && { docker rmi -f "$@" || true; }
    return 0
}

remove_volumes_networks() {
    [ -n "$VOLUMES" ] && { mapfile -t IDS <<<"$VOLUMES"; docker volume rm -f "${IDS[@]}" || true; }
    [ -n "$NETWORKS" ] && { mapfile -t IDS <<<"$NETWORKS"; docker network rm "${IDS[@]}" || true; }
    return 0
}

# Step 1: let compose tear down the core stack it owns
ui_step "Stopping the DockLift stack"
if [ -f "$INSTALL_DIR/docker-compose.yml" ]; then
    ui_run "Stopping the DockLift stack" stop_stack
else
    ui_info "No compose file at $INSTALL_DIR (skipping)"
fi

# Step 2: containers (core stack leftovers + every deployed project)
ui_step "Removing containers"
CONTAINERS=$(docklift_containers)
if [ -n "$CONTAINERS" ]; then
    mapfile -t IDS <<<"$CONTAINERS"
    ui_run "Removing $(count "$CONTAINERS") container(s)" remove_containers "${IDS[@]}"
else
    ui_info "none found"
fi

# Step 3: images - DockLift's own builds, then the base images if nothing else needs them
ui_step "Removing images"
IMAGES=$(docklift_images)
IDS=()
[ -n "$IMAGES" ] && mapfile -t IDS <<<"$IMAGES"
ui_run "Removing $(count "$IMAGES") DockLift image(s)" remove_images ${IDS[@]+"${IDS[@]}"}
KEPT=""
for img in "${BASE_IMAGES[@]}"; do
    if docker image inspect "$img" >/dev/null 2>&1; then
        docker rmi "$img" >>"$UI_LOG" 2>&1 || KEPT="$KEPT $img"
    fi
done
[ -n "$KEPT" ] && ui_info "kept in use by other containers:$KEPT"

# Step 4: volumes and networks
ui_step "Removing volumes and networks"
VOLUMES=$(docklift_volumes)
NETWORKS=$(docklift_networks)
ui_run "Removing $(count "$VOLUMES") volume(s), $(count "$NETWORKS") network(s)" remove_volumes_networks

# Step 5: skip host image cleanup — step 3 already removed DockLift-owned images.
# Never run host-wide prune (shared Docker hosts).
ui_step "Skipping host image cleanup"
ui_ok "Host-wide prune is never run (shared Docker hosts)"

# Step 6: installation directory (database, deployments, backups, certificates)
ui_step "Removing $INSTALL_DIR"
ui_run "Removing $INSTALL_DIR" rm -rf "$INSTALL_DIR"
# The installer's log, and a temporary build swap file left by an installer that was killed
rm -f /var/log/docklift-install.log
ui_swap_stale

# Verify rather than assume
LEFT_C=$(docklift_containers)
LEFT_I=$(docklift_images)
LEFT_V=$(docklift_volumes)
LEFT_N=$(docklift_networks)
if [ -n "$LEFT_C$LEFT_I$LEFT_V$LEFT_N" ] || [ -e "$INSTALL_DIR" ]; then
    echo -e "\n${RED}${BOLD}⚠ Some resources could not be removed:${NC}"
    [ -n "$LEFT_C" ] && echo -e "  containers: $(tr '\n' ' ' <<<"$LEFT_C")"
    [ -n "$LEFT_I" ] && echo -e "  images:     $(tr '\n' ' ' <<<"$LEFT_I")"
    [ -n "$LEFT_V" ] && echo -e "  volumes:    $(tr '\n' ' ' <<<"$LEFT_V")"
    [ -n "$LEFT_N" ] && echo -e "  networks:   $(tr '\n' ' ' <<<"$LEFT_N")"
    [ -e "$INSTALL_DIR" ] && echo -e "  directory:  $INSTALL_DIR"
    echo -e "${DIM}Log: $UI_LOG${NC}"
    exit 1
fi
rm -f "$UI_LOG"
ui_finish "Uninstalled"

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
