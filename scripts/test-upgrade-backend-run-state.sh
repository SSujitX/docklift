#!/usr/bin/env bash
# Behavioral tests for upgrade.sh backend_run_state under set -euo pipefail.
# Stubbed Docker — no real daemon required.
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
UPGRADE="$ROOT/upgrade.sh"
FAILS=0
PASSES=0

pass() { PASSES=$((PASSES + 1)); echo "  pass: $1"; }
fail() { FAILS=$((FAILS + 1)); echo "  FAIL: $1"; }

# --- Guard: upgrade.sh must capture via if/else (standalone call dies under set -e) ---
if grep -qE '^[[:space:]]*if backend_run_state; then' "$UPGRADE"; then
  pass "upgrade.sh captures backend_run_state inside if/else"
else
  fail "upgrade.sh must use: if backend_run_state; then … else BACKEND_STATE=\$?; fi"
fi
if grep -qE '^[[:space:]]*backend_run_state[[:space:]]*$' "$UPGRADE"; then
  fail "upgrade.sh still has a standalone backend_run_state call (set -e killer)"
else
  pass "no standalone backend_run_state call"
fi

# Extract the function body from upgrade.sh (keeps test in sync with production)
FN_FILE="$(mktemp)"
STUB_BIN="$(mktemp -d)"
LOG="$(mktemp)"
trap 'rm -f "$FN_FILE" "$LOG"; rm -rf "$STUB_BIN"' EXIT
sed -n '/^backend_run_state() {/,/^}/p' "$UPGRADE" >"$FN_FILE"
if ! grep -q 'backend_run_state()' "$FN_FILE"; then
  echo "Could not extract backend_run_state from upgrade.sh"
  exit 1
fi
export LOG

write_stub() {
  local mode="$1"
  cat >"$STUB_BIN/docker" <<EOF
#!/usr/bin/env bash
set -euo pipefail
mode="$mode"
# docker compose ps --status running --services
if [ "\${1-}" = "compose" ] && [ "\${2-}" = "ps" ]; then
  case "\$mode" in
    running) echo backend; exit 0 ;;
    stopped) exit 0 ;;
    compose-fail) echo "compose broken" >&2; exit 1 ;;
    *) exit 0 ;;
  esac
fi
# docker ps -q --filter name=… --filter status=running
if [ "\${1-}" = "ps" ] && [ "\${2-}" = "-q" ]; then
  case "\$mode" in
    docker-ps-q-running) echo "abc123"; exit 0 ;;
    docker-ps-fail) echo "daemon down" >&2; exit 1 ;;
    *) exit 0 ;;
  esac
fi
# docker ps --format '{{.Names}}'
if [ "\${1-}" = "ps" ] && [ "\${2-}" = "--format" ]; then
  case "\$mode" in
    names-running) echo "docklift-backend"; exit 0 ;;
    names-fail) echo "format fail" >&2; exit 1 ;;
    *) exit 0 ;;
  esac
fi
exit 0
EOF
  chmod +x "$STUB_BIN/docker"
}

# Capture pattern used by upgrade.sh — must not abort under set -e on return 1/2
capture_state() {
  local BACKEND_STATE
  if backend_run_state; then
    BACKEND_STATE=0
  else
    BACKEND_STATE=$?
  fi
  printf '%s' "$BACKEND_STATE"
}

run_case() {
  local mode="$1" expect="$2" label="$3"
  write_stub "$mode"
  PATH="$STUB_BIN:$PATH"
  # shellcheck disable=SC1090
  source "$FN_FILE"
  local got
  got="$(capture_state)"
  if [ "$got" = "$expect" ]; then
    pass "$label (got $got)"
  else
    fail "$label (expected $expect, got $got)"
  fi
}

echo "upgrade backend_run_state (set -e + stubs)"

run_case stopped 1 "stopped → state 1"
run_case running 0 "compose lists backend → state 0"
run_case compose-fail 2 "compose probe error → state 2"
run_case docker-ps-q-running 0 "docker ps -q running → state 0"
run_case docker-ps-fail 2 "docker ps -q error → state 2"
run_case names-running 0 "docker ps names running → state 0"
run_case names-fail 2 "docker ps names error → state 2"

# Safe capture must survive return 1 and continue (the release-blocker under set -e)
write_stub stopped
PATH="$STUB_BIN:$PATH"
# shellcheck disable=SC1090
source "$FN_FILE"
set -euo pipefail
STATE="$(capture_state)"
# If capture used a standalone call, set -e would have aborted before AFTER=1
AFTER=1
if [ "$STATE" -eq 1 ] && [ "$AFTER" -eq 1 ]; then
  pass "if/else capture survives return 1 under set -e"
else
  fail "if/else capture did not survive return 1 (state=$STATE)"
fi

# Probe-error (2) must also be capturable without aborting the shell
write_stub compose-fail
PATH="$STUB_BIN:$PATH"
source "$FN_FILE"
STATE="$(capture_state)"
AFTER=1
if [ "$STATE" -eq 2 ] && [ "$AFTER" -eq 1 ]; then
  pass "if/else capture survives return 2 under set -e"
else
  fail "if/else capture did not survive return 2 (state=$STATE)"
fi

echo ""
echo "$PASSES passed, $FAILS failed"
[ "$FAILS" -eq 0 ]
