#!/usr/bin/env bash
# The progress helpers shared by install.sh, install-dev.sh, upgrade.sh and
# uninstall.sh (the block between "# >>> docklift-ui >>>" and
# "# <<< docklift-ui <<<"):
#   1. the four copies must be identical (each script is fetched on its own);
#   2. behaviour: exit status kept, set -e kept, output tail on failure,
#      out-of-memory hint, notes, plain output without escape codes, and,
#      where util-linux is available, terminal animation and Ctrl-C handling.
#
#   scripts/test-ui-helpers.sh          check
#   scripts/test-ui-helpers.sh --sync   copy install.sh's block into the other scripts
#
# `cond && pass || fail` is intended (pass/fail always succeed); harness bodies
# are single-quoted on purpose (they expand in the generated script).
# shellcheck disable=SC2015,SC2016
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
SCRIPTS=(install.sh install-dev.sh upgrade.sh uninstall.sh)
BEGIN='# >>> docklift-ui >>>'
END='# <<< docklift-ui <<<'

extract() { sed -n "/^$BEGIN\$/,/^$END\$/p" "$1"; }

if [ "${1:-}" = "--sync" ]; then
    block=$(extract "$ROOT/install.sh")
    for f in "${SCRIPTS[@]:1}"; do
        # ENVIRON, not awk -v: -v would interpret the backslashes in the block
        UI_BLOCK="$block" awk -v b="$BEGIN" -v e="$END" '
            $0 == b { print ENVIRON["UI_BLOCK"]; skip = 1; next }
            $0 == e { skip = 0; next }
            !skip' "$ROOT/$f" >"$ROOT/$f.tmp" && cat "$ROOT/$f.tmp" >"$ROOT/$f" && rm -f "$ROOT/$f.tmp"
        echo "synced $f"
    done
    exit 0
fi

FAILS=0
PASSES=0
pass() { PASSES=$((PASSES + 1)); echo "  pass: $1"; }
fail() { FAILS=$((FAILS + 1)); echo "  FAIL: $1"; }

echo "docklift-ui block"
REF=$(extract "$ROOT/install.sh")
if [ -z "$REF" ] || ! grep -qF "$END" <<<"$REF"; then
    fail "install.sh has no complete '$BEGIN' … '$END' block"
fi
for f in "${SCRIPTS[@]:1}"; do
    if [ "$(extract "$ROOT/$f")" = "$REF" ]; then
        pass "$f block identical to install.sh"
    else
        fail "$f block differs from install.sh (fix one copy, then: scripts/test-ui-helpers.sh --sync)"
        diff <(printf '%s\n' "$REF") <(extract "$ROOT/$f") | head -20 || true
    fi
done

TMP=$(mktemp -d)
trap 'rm -rf "$TMP"' EXIT
printf '%s\n' "$REF" >"$TMP/block.sh"

# harness NAME BODY: a script that loads the block under set -euo pipefail
harness() {
    {
        echo '#!/usr/bin/env bash'
        echo 'set -euo pipefail'
        echo "source '$TMP/block.sh'"
        echo "UI_LOG='$TMP/$1.log'"
        echo 'trap "ui_cleanup; echo cleanup-ran >>\"$UI_LOG.cleanup\"" EXIT'
        echo "trap 'exit 130' INT"
        echo "trap 'exit 143' TERM"
        printf '%s\n' "$2"
    } >"$TMP/$1.sh"
}

echo "plain output (not a terminal)"
harness ok '
many() { local i; for i in $(seq 1 60); do echo "line $i"; done; exit 3; }
noted() { ui_note "retrying (2/4)"; sleep 2; }
oom() { echo "#20 78.37 Killed"; return 1; }
ui_steps 2
ui_step "First"
ui_run "Slow step" sleep 2
ui_run "Noted step" noted
if ui_run "Failing step" many; then echo "status=0"; else echo "status=$?"; fi
ui_step "Second"
if ui_run "Oom step" oom; then :; fi
ui_run "Exit 137" sh -c "exit 137" || echo "status137=$?"
ui_run "Fatal step" sh -c "exit 7"
echo "not reached"'
set +e
CI=true bash "$TMP/ok.sh" >"$TMP/ok.out" 2>&1
rc=$?
set -e
out=$(cat "$TMP/ok.out")
[ "$rc" -eq 7 ] && pass "set -e: failing ui_run ends the script with its exit status (7)" \
    || fail "script exit status $rc, want 7"
grep -q 'not reached' <<<"$out" && fail "script continued after a failing ui_run" || pass "nothing runs after a failing ui_run"
grep -q '^  \[1/2\] First$' <<<"$out" && pass "plain step header" || fail "plain step header missing"
grep -qE '^    Slow step\.\.\. done \([1-3]s\)$' <<<"$out" && pass "plain step line with duration" || fail "plain step line: $(grep 'Slow step' <<<"$out")"
grep -q 'Noted step\.\.\. retrying (2/4)\.\.\. done' <<<"$out" && pass "note shown in plain mode" || fail "note missing: $(grep 'Noted' <<<"$out")"
grep -q 'status=3' <<<"$out" && pass "exit status kept (3)" || fail "exit status not kept"
grep -q 'line 60' <<<"$out" && ! grep -q 'line 20$' <<<"$out" && grep -q 'line 21$' <<<"$out" \
    && pass "failure shows only the last 40 lines" || fail "failure tail wrong"
grep -q "Full log: $TMP/ok.log" <<<"$out" && pass "failure names the full log" || fail "full log path missing"
[ "$(grep -c 'ran out of memory' <<<"$out")" -eq 2 ] && pass "out-of-memory hint for a Killed line and for exit 137" \
    || fail "out-of-memory hint count: $(grep -c 'ran out of memory' <<<"$out")"
grep -q 'status137=137' <<<"$out" && pass "exit status 137 kept" || fail "137 not kept"
if grep -q $'\033' <<<"$out" || grep -q $'\r' <<<"$out"; then fail "plain output has escape codes or CRs"; else pass "plain output has no escape codes or CRs"; fi
grep -q 'cleanup-ran' "$TMP/ok.log.cleanup" && pass "EXIT cleanup ran" || fail "EXIT cleanup did not run"

if [ "$(uname -s)" != Linux ] || ! command -v setsid >/dev/null 2>&1 || ! env --default-signal=INT true 2>/dev/null; then
    echo "  skip: signal and terminal checks need Linux with setsid, script (util-linux) and env --default-signal"
else
    echo "Ctrl-C (SIGINT to the process group)"
    harness int 'ui_run "Long step" sh -c "sleep 313; echo finished"; echo "not reached"'
    # A background job starts with SIGINT ignored (and bash cannot trap a signal
    # ignored at entry); a script started from a terminal has it at default.
    setsid env --default-signal=INT bash "$TMP/int.sh" >"$TMP/int.out" 2>&1 &
    pid=$!
    sleep 1.5
    kill -INT -- "-$pid" 2>/dev/null || kill -INT "$pid"
    set +e; wait "$pid"; rc=$?; set -e
    sleep 0.5
    [ "$rc" -eq 130 ] && pass "Ctrl-C exits with 130" || fail "Ctrl-C exit status $rc"
    pgrep -f 'sleep 313' >/dev/null && { fail "step child still running after Ctrl-C"; pkill -f 'sleep 313' || true; } \
        || pass "step child stopped"
    grep -q 'Long step: interrupted' "$TMP/int.out" && pass "interrupted step reported" || fail "no interrupted line: $(cat "$TMP/int.out")"
    grep -q 'cleanup-ran' "$TMP/int.log.cleanup" && pass "EXIT cleanup ran after Ctrl-C" || fail "cleanup did not run after Ctrl-C"

    if command -v script >/dev/null 2>&1; then
        echo "terminal output (script(1) pseudo-terminal)"
        harness tty 'ui_steps 2; ui_step "Build"; ui_run "Slow step" sleep 1.5; ui_finish "Done"'
        TERM=xterm LANG=C.UTF-8 LC_ALL=C.UTF-8 CI='' NO_COLOR='' script -qec "bash $TMP/tty.sh" /dev/null >"$TMP/tty.out" 2>&1 </dev/null || true
        grep -q '⠋' "$TMP/tty.out" && pass "spinner animates" || fail "no spinner"
        grep -q '✓' "$TMP/tty.out" && pass "✓ result line" || fail "no ✓ line"
        grep -q '░' "$TMP/tty.out" && pass "progress bar" || fail "no progress bar"
        grep -q $'\033\[?25h' "$TMP/tty.out" && pass "cursor restored" || fail "cursor not restored"
        TERM=xterm LANG=C LC_ALL=C CI='' NO_COLOR='' script -qec "bash $TMP/tty.sh" /dev/null >"$TMP/ascii.out" 2>&1 </dev/null || true
        if grep -q '⠋\|✓\|░' "$TMP/ascii.out"; then fail "non-UTF-8 locale still prints Unicode"; else pass "ASCII spinner/bar without a UTF-8 locale"; fi
        TERM=xterm LANG=C.UTF-8 NO_COLOR=1 script -qec "bash $TMP/tty.sh" /dev/null >"$TMP/nocolor.out" 2>&1 </dev/null || true
        if grep -q $'\033' "$TMP/nocolor.out"; then fail "NO_COLOR output has escape codes"; else pass "NO_COLOR: plain text on a terminal"; fi

        harness ttyint 'ui_run "Long step" sh -c "sleep 314"; echo "not reached"'
        # ^C typed into the pseudo-terminal: the line discipline signals the foreground group
        { sleep 1.5; printf '\003'; sleep 3; } | TERM=xterm LANG=C.UTF-8 CI='' NO_COLOR='' \
            script -qec "bash $TMP/ttyint.sh" /dev/null >"$TMP/ttyint.out" 2>&1 || true
        sleep 0.5
        pgrep -f 'sleep 314' >/dev/null && { fail "terminal Ctrl-C left the step running"; pkill -f 'sleep 314' || true; } \
            || pass "terminal Ctrl-C stopped the step"
        grep -q $'\033\[?25h' "$TMP/ttyint.out" && pass "cursor restored after terminal Ctrl-C" || fail "cursor not restored after Ctrl-C"
        grep -q 'cleanup-ran' "$TMP/ttyint.log.cleanup" && pass "cleanup ran after terminal Ctrl-C" || fail "cleanup missing after terminal Ctrl-C"
    else
        echo "  skip: terminal checks need script (util-linux)"
    fi
fi

echo ""
echo "$PASSES passed, $FAILS failed"
[ "$FAILS" -eq 0 ]
