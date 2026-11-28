#!/bin/bash
set -euo pipefail

# ╔════════════════════════════════════════════════════════════════════════════╗
# ║                         DOCKLIFT UPGRADE SCRIPT                             ║
# ║  Safely upgrades Docklift while preserving all data and user containers     ║
# ╚════════════════════════════════════════════════════════════════════════════╝
#
# Also migrates a 2.1.x install (backend + frontend + nginx gateway containers)
# to the single Go backend container: the obsolete docklift-frontend and
# docklift-nginx containers and the docklift-frontend image are removed once
# the new backend is healthy.
#
#   curl -fsSL https://raw.githubusercontent.com/SSujitX/docklift/master/upgrade.sh | sudo bash
#   ... | sudo bash -s -- v=3.0.0          # pin a release instead of the latest
#
# Environment:
#   DOCKLIFT_VERSION      release to upgrade to (default: latest GitHub release)
#   DOCKLIFT_INSTALL_DIR  install location (default /opt/docklift)
#   DOCKLIFT_CI_LOCAL     "true": upgrade to the commit checked out in the current
#                         directory (a git clone) instead of GitHub (CI and testing)

# git checkout replaces this file while it runs; bash reads scripts
# incrementally, so run from a private copy when started from a file.
if [ -z "${DOCKLIFT_UPGRADE_COPY:-}" ] && [ -f "${BASH_SOURCE[0]:-}" ]; then
    DOCKLIFT_UPGRADE_COPY=$(mktemp /tmp/docklift-upgrade.XXXXXX)
    cp "${BASH_SOURCE[0]}" "$DOCKLIFT_UPGRADE_COPY"
    export DOCKLIFT_UPGRADE_COPY
    exec bash "$DOCKLIFT_UPGRADE_COPY" "$@"
fi
# Keep the terminal reachable from traps that fire inside log-redirected blocks
exec 3>&1 4>&2

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

INSTALL_DIR="${DOCKLIFT_INSTALL_DIR:-/opt/docklift}"
SOURCE_DIR="$PWD"
START_TIME=$(date +%s)
ROLLBACK_REF=""
PRE_BACKEND_TAG="docklift-backend:pre-upgrade"
PRE_FRONTEND_TAG="docklift-frontend:pre-upgrade"
# Containers of the 2.1.x layout that the Go backend replaces.
OBSOLETE_CONTAINERS=(docklift-frontend docklift-nginx)
ROLLBACK_ARMED=0
IN_ROLLBACK=0
BACKUP_FILE=""
CONF_SNAPSHOT=""
LOG=""
REQUESTED_VERSION="${DOCKLIFT_VERSION:-}"
HEALTH_TIMEOUT=180

format_time() {
    local s=$1 h=0 m=0
    # `((…))` returns exit 1 when the value is 0 — must not trip `set -e`
    ((h = s / 3600, m = (s % 3600) / 60, s = s % 60)) || true
    if [ "$h" -gt 0 ]; then printf "%dh %dm %ds" "$h" "$m" "$s"
    elif [ "$m" -gt 0 ]; then printf "%dm %ds" "$m" "$s"
    else printf "%ds" "$s"
    fi
}

read_version() {
    # Root package.json carries the release version in every layout (2.1.x and Go).
    local v
    v=$(grep -o '"version": *"[^"]*"' "$INSTALL_DIR/package.json" 2>/dev/null | head -1 | cut -d'"' -f4 || true)
    printf '%s' "${v:-unknown}"
}

health_ok() {
    # Probe the API inside the backend container: busybox wget exists in both the
    # Go image and the 2.1.x Node image; node fetch is the 2.1.x fallback.
    docker exec docklift-backend wget -q -O /dev/null http://127.0.0.1:8000/api/health >/dev/null 2>&1 && return 0
    docker exec docklift-backend node -e \
      "fetch('http://127.0.0.1:8000/api/health').then(r=>process.exit(r.ok?0:1)).catch(()=>process.exit(1))" \
      >/dev/null 2>&1
}

# Every service of the compose file currently checked out must be running.
stack_running() {
    local want have svc
    want=$(docker compose config --services 2>>"$LOG") || return 1
    have=$(docker compose ps --status running --services 2>>"$LOG") || return 1
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

sqlite_backup() {
    local src=$1 dest=$2
    # Prefer SQLite online backup API when sqlite3 CLI is present
    if command -v sqlite3 >/dev/null 2>&1; then
        sqlite3 "$src" ".backup '$dest'"
        return $?
    fi
    # Fallback: file copy only when backend is stopped (no open writers). The Go
    # backend uses WAL; a stopped server has checkpointed, but keep any -wal too.
    cp "$src" "$dest" || return 1
    if [ -s "$src-wal" ]; then
        cp "$src-wal" "$dest-wal" || return 1
    fi
}

restore_database() {
    local db="$INSTALL_DIR/data/docklift.db"
    cp "$BACKUP_FILE" "$db"
    # -wal/-shm left by the new backend belong to the newer database state.
    rm -f "$db-wal" "$db-shm"
    if [ -f "$BACKUP_FILE-wal" ]; then
        cp "$BACKUP_FILE-wal" "$db-wal"
    fi
}

tag_running_images() {
    # Preserve current images so rollback does not depend on rebuilding an old git ref
    if docker image inspect docklift-backend >/dev/null 2>&1; then
        docker tag docklift-backend "$PRE_BACKEND_TAG" 2>/dev/null || true
    fi
    if docker image inspect docklift-frontend >/dev/null 2>&1; then
        docker tag docklift-frontend "$PRE_FRONTEND_TAG" 2>/dev/null || true
    else
        # A stale tag from an older upgrade must not be restored on rollback
        docker rmi "$PRE_FRONTEND_TAG" 2>/dev/null || true
    fi
}

remove_obsolete_containers() {
    local c
    for c in "${OBSOLETE_CONTAINERS[@]}"; do
        if docker container inspect "$c" >/dev/null 2>&1; then
            docker rm -f "$c" || return 1
        fi
    done
}

print_manual_rollback() {
    echo -e "  ${BOLD}Manual rollback:${NC}"
    echo -e "    cd $INSTALL_DIR"
    echo -e "    docker compose stop"
    [ -n "$ROLLBACK_REF" ] && echo -e "    git checkout -f $ROLLBACK_REF"
    if [ -n "${BACKUP_FILE:-}" ]; then
        echo -e "    cp $BACKUP_FILE data/docklift.db && rm -f data/docklift.db-wal data/docklift.db-shm"
    fi
    [ -n "${CONF_SNAPSHOT:-}" ] && echo -e "    tar -xzf $CONF_SNAPSHOT -C $INSTALL_DIR/nginx-proxy"
    echo -e "    docker tag $PRE_BACKEND_TAG docklift-backend"
    echo -e "    docker image inspect $PRE_FRONTEND_TAG >/dev/null 2>&1 && docker tag $PRE_FRONTEND_TAG docklift-frontend"
    echo -e "    docker compose up -d --remove-orphans"
}

rollback_upgrade() {
    # Prevent re-entrancy from ERR trap while we are already rolling back
    if [ "$IN_ROLLBACK" = "1" ]; then
        return 0
    fi
    IN_ROLLBACK=1
    ROLLBACK_ARMED=0
    trap - ERR
    # Ctrl-C during the rollback still exits through on_exit (cleanup)
    trap 'exit 130' INT
    trap 'exit 143' TERM
    set +e
    # The failure may have happened inside a block redirected to the log:
    # report on the terminal the script started with.
    exec 1>&3 2>&4
    # Interrupted while a step was running: stop it before touching the stack
    ui_stop

    if [ "$UI_TAIL_SHOWN" = 1 ]; then
        echo -e "\n  ${RED}${BOLD}Upgrade failed.${NC}"
    else
        echo -e "\n  ${RED}${BOLD}Upgrade failed — last lines of $LOG:${NC}"
        tail -n 12 "$LOG" 2>/dev/null | sed 's/^/    /'
    fi
    echo -e "\n  ${RED}${BOLD}Rolling back upgrade…${NC}"
    cd "$INSTALL_DIR"
    {
        echo "--- rollback started at $(date) ---"
        docker compose stop
        docker stop docklift-backend
    } >>"$LOG" 2>&1

    if [ -n "${ROLLBACK_REF}" ]; then
        { git checkout "${ROLLBACK_REF}" -q || git checkout -f "${ROLLBACK_REF}" -q; } >>"$LOG" 2>&1
    fi

    if [ -n "${BACKUP_FILE:-}" ] && [ -f "$BACKUP_FILE" ]; then
        # Backend must be stopped before overwriting SQLite
        restore_database && echo -e "  ${YELLOW}Database restored from backup${NC}"
    fi

    # The new backend rewrites panel-domain confs at boot; put the old ones back
    if [ -n "${CONF_SNAPSHOT:-}" ] && [ -f "$CONF_SNAPSHOT" ]; then
        tar -xzf "$CONF_SNAPSHOT" -C "$INSTALL_DIR/nginx-proxy" >>"$LOG" 2>&1 \
          && echo -e "  ${YELLOW}nginx-proxy configs restored${NC}"
    fi

    # Prefer tagged pre-upgrade images (reliable) over rebuild-from-git
    if docker image inspect "$PRE_BACKEND_TAG" >/dev/null 2>&1; then
        docker tag "$PRE_BACKEND_TAG" docklift-backend >>"$LOG" 2>&1
    fi
    if docker image inspect "$PRE_FRONTEND_TAG" >/dev/null 2>&1; then
        docker tag "$PRE_FRONTEND_TAG" docklift-frontend >>"$LOG" 2>&1
    fi

    ui_run "Starting the previous version" restart_previous
    if ui_run "Waiting for /api/health" wait_healthy; then
        echo -e "  ${YELLOW}Rolled back to the previous version — it is running and healthy.${NC}"
        echo -e "  ${DIM}Details: $LOG${NC}"
    else
        echo -e "  ${RED}Rollback did not bring the previous version back to health.${NC}"
        echo -e "  ${DIM}Details: $LOG${NC}"
        print_manual_rollback
    fi
}

# Any exit while armed rolls back: an ERR in the main body, an errexit inside a
# function (where the ERR trap does not reach), or a signal.
on_exit() {
    local ec=$?
    if [ "${ROLLBACK_ARMED:-0}" = "1" ] && [ "$ec" -ne 0 ]; then
        rollback_upgrade
    fi
    # After the rollback: it may rebuild, so the temporary swap goes last
    ui_cleanup
    if [ -n "${DOCKLIFT_UPGRADE_COPY:-}" ]; then
        rm -f "$DOCKLIFT_UPGRADE_COPY"
    fi
}
trap on_exit EXIT
# Signals outside the armed window still leave through on_exit (cleanup)
trap 'exit 130' INT
trap 'exit 143' TERM

arm_rollback() {
    ROLLBACK_ARMED=1
    trap 'if [ "${ROLLBACK_ARMED:-0}" = "1" ]; then rollback_upgrade; fi; exit 1' ERR INT TERM
}

disarm_rollback() {
    ROLLBACK_ARMED=0
    trap - ERR
    trap 'exit 130' INT
    trap 'exit 143' TERM
}

# Rollback: the pre-upgrade images were tagged back; rebuild only if that fails
restart_previous() {
    docker compose up -d --remove-orphans || docker compose up -d --build --remove-orphans
}

fetch_code() {
    if [ "$TARGET_TAG" = "local" ]; then
        # The commit checked out in the current directory (a git clone of the
        # new code), checked out exactly as a release tag would be
        if [ "$SOURCE_DIR" != "$INSTALL_DIR" ]; then
            git -c safe.directory='*' fetch -q "$SOURCE_DIR" HEAD && git checkout -q FETCH_HEAD || return 1
        fi
    else
        git fetch origin --tags -q || return 1
        if [ -n "$TARGET_TAG" ]; then
            git checkout "$TARGET_TAG" -q 2>/dev/null || git checkout "tags/$TARGET_TAG" -q || return 1
        else
            git fetch origin master -q && git reset --hard origin/master -q || return 1
        fi
    fi
}

# 2.1.x gateway and frontend: replaced by the backend, which now owns :8080
swap_containers() {
    remove_obsolete_containers && docker compose up -d --remove-orphans
}

# Normalize to GitHub tag form (vX.Y.Z). Empty means resolve latest release.
normalize_release_tag() {
    local raw="$1"
    raw=$(printf '%s' "$raw" | tr -d '[:space:]')
    case "$raw" in
        ""|latest|LATEST) printf '' ;;
        v*) printf '%s' "$raw" ;;
        *) printf 'v%s' "$raw" ;;
    esac
}

while [ $# -gt 0 ]; do
    case "$1" in
        v=*|version=*|--version=*) REQUESTED_VERSION="${1#*=}" ;;
        --version|-v) shift; REQUESTED_VERSION="${1:-}" ;;
        latest|LATEST) REQUESTED_VERSION="" ;;
        v[0-9]*|[0-9]*.[0-9]*) REQUESTED_VERSION="$1" ;;
        *)
            echo -e "  ${RED}Error: unknown upgrade arg: $1${NC}"
            echo -e "  ${DIM}Use: bash -s -- v=3.0.0   or   DOCKLIFT_VERSION=3.0.0${NC}"
            exit 1
            ;;
    esac
    shift || true
done
TARGET_TAG=$(normalize_release_tag "$REQUESTED_VERSION")

# Header
[ "$UI_FANCY" = 1 ] && { clear 2>/dev/null || true; }
echo -e "\n  ${CYAN}____             _    _ _  __ _   ${NC}"
echo -e "  ${CYAN}|  _ \\  ___   ___| | _| (_)/ _| |_ ${NC}\n  ${CYAN}| | | |/ _ \\ / __| |/ / | | |_| __|${NC}"
echo -e "  ${CYAN}| |_| | (_) | (__|   <| | |  _| |_ ${NC}\n  ${CYAN}|____/ \\___/ \\___|_|\\_\\_|_|_|  \\__|${NC}\n"
echo -e "  ${DIM}Self-Hosted Docker Deployment Platform${NC}"
echo -e "  ${YELLOW}${BOLD}⬆ UPGRADE MODE${NC}\n"

# Pre-flight checks
[ "$EUID" -ne 0 ] && echo -e "  ${RED}Error: Run with sudo${NC}" && exit 1

if [ ! -d "$INSTALL_DIR" ] || [ ! -f "$INSTALL_DIR/docker-compose.yml" ]; then
    echo -e "  ${RED}Error: Docklift not found at $INSTALL_DIR${NC}"
    echo -e "  ${DIM}Run the install script first: curl -fsSL https://raw.githubusercontent.com/SSujitX/docklift/master/install.sh | sudo bash${NC}"
    exit 1
fi
for cmd in docker git tar; do
    if ! command -v "$cmd" >/dev/null 2>&1; then
        echo -e "  ${RED}Error: $cmd is required${NC}"
        exit 1
    fi
done
if ! docker compose version >/dev/null 2>&1; then
    echo -e "  ${RED}Error: the Docker Compose plugin (docker compose) is required${NC}"
    exit 1
fi

cd "$INSTALL_DIR"
LOG="$INSTALL_DIR/upgrade.log"
ui_log_init "$LOG"
LOG=$UI_LOG
echo "--- Upgrade started at $(date) ---" > "$LOG"

# Pin current git ref for source rollback
ROLLBACK_REF=$(git rev-parse HEAD 2>/dev/null || echo "")

# Get current version
OLD_VERSION=$(read_version)
echo -e "  ${BOLD}Current Version:${NC} ${CYAN}$OLD_VERSION${NC}"
echo -e "        ${DIM}➞ Git ref: ${ROLLBACK_REF:-unknown}${NC}"

# Resolve the target before taking anything offline
if [ "${DOCKLIFT_CI_LOCAL:-}" = "true" ]; then
    TARGET_TAG="local"
elif [ -z "$TARGET_TAG" ]; then
    TARGET_TAG=$(curl -fsS https://api.github.com/repos/SSujitX/docklift/releases/latest 2>>"$LOG" \
        | grep '"tag_name"' | head -1 | cut -d'"' -f4 || true)
    TARGET_TAG=$(printf '%s' "$TARGET_TAG" | tr -d '[:space:]')
fi
echo -e "        ${DIM}➞ Target: ${TARGET_TAG:-origin/master (no release found)}${NC}"
if [ -n "$TARGET_TAG" ] && [ "$TARGET_TAG" != "local" ] \
    && ! git ls-remote --exit-code --tags origin "refs/tags/${TARGET_TAG}" >>"$LOG" 2>&1; then
    echo -e "  ${RED}Error: release tag not found on origin: ${TARGET_TAG}${NC}"
    echo -e "  ${DIM}Nothing was changed. See https://github.com/SSujitX/docklift/releases${NC}"
    exit 1
fi
echo -e "        ${DIM}➞ Log: $LOG${NC}"
ui_steps 6

# Step 1: Prepare backup dir BEFORE stopping backend (avoid pre-trap outage gap)
ui_step "Preparing rollback snapshot"
BACKUP_DIR="$INSTALL_DIR/backups"
mkdir -p "$BACKUP_DIR"
# Prove the directory is writable before we take the API offline
STAMP=$(date +%Y%m%d_%H%M%S)
BACKUP_FILE="$BACKUP_DIR/docklift_${STAMP}.db.bak"
touch "$BACKUP_DIR/.write-test" && rm -f "$BACKUP_DIR/.write-test"

tag_running_images >>"$LOG" 2>&1 || true
ui_ok "Tagged pre-upgrade images (if present)"

# Stop backend and prove it is not writing before any DB copy
if ! ui_run "Stopping the backend" docker compose stop backend; then
    ui_fail "Failed to stop backend — aborting (no DB snapshot taken)"
    docker compose start backend >>"$LOG" 2>&1 || true
    exit 1
fi

# Backend run-state probe: 0=running, 1=stopped, 2=probe-error (indeterminate)
# Probe-error must NOT be treated as stopped — fail closed before any DB copy.
backend_run_state() {
    local out ec
    out=$(docker compose ps --status running --services 2>&1)
    ec=$?
    if [ "$ec" -ne 0 ]; then
        echo "$out" >>"$LOG"
        return 2
    fi
    if printf '%s\n' "$out" | grep -qx 'backend'; then
        return 0
    fi

    out=$(docker ps -q --filter "name=docklift-backend" --filter "status=running" 2>&1)
    ec=$?
    if [ "$ec" -ne 0 ]; then
        echo "$out" >>"$LOG"
        return 2
    fi
    if [ -n "$(printf '%s' "$out" | tr -d '[:space:]')" ]; then
        return 0
    fi

    out=$(docker ps --format '{{.Names}}' 2>&1)
    ec=$?
    if [ "$ec" -ne 0 ]; then
        echo "$out" >>"$LOG"
        return 2
    fi
    if printf '%s\n' "$out" | grep -qx 'docklift-backend'; then
        return 0
    fi
    return 1
}

# Must capture under if/else — standalone `backend_run_state; BACKEND_STATE=$?`
# aborts under `set -e` on the normal stopped (1) and probe-error (2) returns.
if backend_run_state; then
    BACKEND_STATE=0
else
    BACKEND_STATE=$?
fi
if [ "$BACKEND_STATE" -eq 0 ]; then
    ui_fail "Backend still running after stop — aborting (no DB snapshot taken)"
    docker compose start backend >>"$LOG" 2>&1 || true
    exit 1
fi
if [ "$BACKEND_STATE" -eq 2 ]; then
    ui_fail "Could not verify backend stopped (Docker probe failed) — aborting"
    ui_info "➞ No DB snapshot taken; attempting to restart backend"
    docker compose start backend >>"$LOG" 2>&1 || true
    exit 1
fi
if [ "$BACKEND_STATE" -ne 1 ]; then
    ui_fail "Unexpected backend run-state ($BACKEND_STATE) — aborting"
    docker compose start backend >>"$LOG" 2>&1 || true
    exit 1
fi
ui_ok "Backend stopped (verified)"

# Arm immediately after verified stop so any later failure restarts the stack
arm_rollback

if [ -f "$INSTALL_DIR/data/docklift.db" ]; then
    if ui_run "Snapshotting the database" sqlite_backup "$INSTALL_DIR/data/docklift.db" "$BACKUP_FILE"; then
        ui_ok "DB snapshot: $BACKUP_FILE"
    else
        ui_fail "Database backup failed"
        rollback_upgrade
        exit 1
    fi
else
    ui_warn "No existing database found (fresh install?)"
    BACKUP_FILE=""
fi

# The new backend rewrites panel-domain confs at boot
# (docklift-nginx:80 → docklift-backend.docklift_network:8000)
if [ -d "$INSTALL_DIR/nginx-proxy/conf.d" ]; then
    CONF_SNAPSHOT="$BACKUP_DIR/nginx-conf_${STAMP}.tar.gz"
    tar -czf "$CONF_SNAPSHOT" -C "$INSTALL_DIR/nginx-proxy" conf.d
    ui_ok "nginx-proxy conf snapshot: $CONF_SNAPSHOT"
fi

# Step 2: Fetch the new code
ui_step "Fetching ${TARGET_TAG:-latest master}"
ui_run "Fetching ${TARGET_TAG:-latest master}" fetch_code

NEW_VERSION=$(read_version)
if [ "$OLD_VERSION" = "$NEW_VERSION" ]; then
    ui_info "➞ Already on version: $NEW_VERSION (rebuilding)"
else
    ui_info "➞ Upgrading: $OLD_VERSION → $NEW_VERSION"
fi

mkdir -p \
  "$INSTALL_DIR/data" \
  "$INSTALL_DIR/deployments" \
  "$INSTALL_DIR/backups" \
  "$INSTALL_DIR/nginx-proxy/conf.d" \
  "$INSTALL_DIR/nginx-proxy/snippets" \
  "$INSTALL_DIR/nginx-proxy/certbot/www" \
  "$INSTALL_DIR/nginx-proxy/certbot/conf"

# Step 3: Build while nginx-proxy keeps serving user app domains
ui_step "Building Docklift"
ui_swap_ensure
if ! ui_run "Building the Docklift image (a few minutes)" docker compose build; then
    rollback_upgrade
    exit 1
fi
ui_ram_hint
# The proxy and certbot images are pinned in docker-compose.yml; fetch the
# pinned tags now so the container swap below never waits on (or fails at) a pull.
if ! ui_run "Pulling nginx-proxy and certbot images" docker compose pull nginx-proxy certbot; then
    rollback_upgrade
    exit 1
fi

# Step 4: Swap containers (user project containers are never touched)
ui_step "Restarting Docklift containers"
ui_run "Restarting Docklift containers" swap_containers
ui_info "➞ User project containers are untouched"

# Step 5: Health check with retries
ui_step "Verifying health"
if ui_run "Waiting for /api/health" wait_healthy; then
    ui_ok "All systems operational"
    disarm_rollback
else
    ui_fail "Health check failed"
    ui_info "➞ backend did not answer /api/health with every service running within ${HEALTH_TIMEOUT}s"
    docker compose ps >>"$LOG" 2>&1 || true
    docker logs --tail 100 docklift-backend >>"$LOG" 2>&1 || true
    rollback_upgrade
    exit 1
fi

# Step 6: Remove what the new layout no longer uses
ui_step "Finalizing"
if ! docker compose config --services 2>>"$LOG" | grep -qx frontend; then
    for img in docklift-frontend "$PRE_FRONTEND_TAG"; do
        if docker image inspect "$img" >/dev/null 2>&1 && docker rmi "$img" >>"$LOG" 2>&1; then
            ui_ok "Removed obsolete image $img"
        fi
    done
fi
ui_swap_release
ui_finish "Upgrade complete"
echo -e "\n  ╔══════════════════════════════════════════════════════════════╗"
echo -e "  ║  ${GREEN}${BOLD}✓ UPGRADE COMPLETE${NC}                                        ║"
echo -e "  ╚══════════════════════════════════════════════════════════════╝"
TOTAL_TIME=$(($(date +%s) - START_TIME))
echo -e "  ${DIM}Time: $(format_time $TOTAL_TIME) | Version: $NEW_VERSION${NC}\n"

echo -e "  ${BOLD}✓ Preserved:${NC}"
echo -e "    ${GREEN}•${NC} Database (projects, settings, deployments)"
echo -e "    ${GREEN}•${NC} All user containers (dl_* containers)"
echo -e "    ${GREEN}•${NC} Nginx / panel domain configs"
echo -e "    ${GREEN}•${NC} Let's Encrypt certificates (certbot/)"
echo -e "    ${GREEN}•${NC} Project files in /deployments"
echo -e "    ${GREEN}•${NC} Pre-upgrade backend image tagged ${PRE_BACKEND_TAG}"
echo -e ""

BACKUP_COUNT=$(find "$BACKUP_DIR" -maxdepth 1 -name '*.db.bak' 2>/dev/null | wc -l | tr -d ' ')
echo -e "  ${DIM}Backups stored: $BACKUP_COUNT files in $BACKUP_DIR${NC}"
if [ -n "${BACKUP_FILE}" ]; then
    echo -e "  ${DIM}DB snapshot: $BACKUP_FILE${NC}\n"
fi

# "Only allow access through the domain" closes :8080: name the domain instead.
PANEL_ONLY=$(docker exec docklift-backend docklift panel-access status 2>/dev/null \
    | sed -n 's|.*only through \(https://[^ ]*\).*|\1|p' | head -n 1 || true)
PUB4=$(curl -4 -s --connect-timeout 2 https://api.ipify.org 2>/dev/null || echo "")
if [ -n "$PANEL_ONLY" ]; then
    echo -e "  ${BOLD}Access Docklift:${NC} ${PANEL_ONLY}"
    echo -e "  ${DIM}(:8080 only answers through the domain; to reopen it: docker exec docklift-backend docklift panel-access open)${NC}\n"
else
    if [ -n "$PUB4" ]; then
        echo -e "  ${BOLD}Access Docklift:${NC} http://${PUB4}:8080"
    else
        echo -e "  ${BOLD}Access Docklift:${NC} http://SERVER_IP:8080"
    fi
    echo -e "  ${DIM}(Optional: HTTPS panel domain in Settings → Domain, then \"Only allow access through the domain\"; or DASHBOARD_BIND=127.0.0.1)${NC}\n"
fi
