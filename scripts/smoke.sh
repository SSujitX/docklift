#!/usr/bin/env bash
# Black-box smoke test of a running Docklift stack (Install workflow, manual checks).
#
#   scripts/smoke.sh register   first admin via the bootstrap secret from the backend logs, then check
#   scripts/smoke.sh check      log in as that admin and check the stack
#
# Environment:
#   DOCKLIFT_URL             dashboard URL (default http://127.0.0.1:8080)
#   DOCKLIFT_ADMIN_EMAIL     default admin@docklift.test
#   DOCKLIFT_ADMIN_PASSWORD  default Smoke-Test-Password-1
#   DOCKLIFT_INSTALL_DIR     install location, for the bootstrap secret fallback (default /opt/docklift)
set -euo pipefail

URL="${DOCKLIFT_URL:-http://127.0.0.1:8080}"
EMAIL="${DOCKLIFT_ADMIN_EMAIL:-admin@docklift.test}"
PASSWORD="${DOCKLIFT_ADMIN_PASSWORD:-Smoke-Test-Password-1}"
INSTALL_DIR="${DOCKLIFT_INSTALL_DIR:-/opt/docklift}"
BODY=$(mktemp)
trap 'rm -f "$BODY"' EXIT

FAILS=0
pass() { echo "  pass: $1"; }
fail() { FAILS=$((FAILS + 1)); echo "  FAIL: $1"; }

# http METHOD PATH [curl args...] -> prints the status code, body in $BODY
http() {
    local method=$1 path=$2
    shift 2
    curl -sS -o "$BODY" -w '%{http_code}' -X "$method" "$@" "$URL$path" || echo 000
}

expect() {
    local label=$1 want=$2 got=$3
    if [ "$got" = "$want" ]; then pass "$label ($got)"; else fail "$label (want $want, got $got: $(head -c 200 "$BODY"))"; fi
}

json_field() {
    sed -n "s/.*\"$1\":\"\([^\"]*\)\".*/\1/p" "$BODY" | head -n 1
}

bootstrap_secret() {
    local s
    s=$(docker logs docklift-backend 2>&1 | grep -A8 'Fresh install' \
        | sed -n 's/^║[[:space:]]*\([A-Za-z0-9_-]\{20,\}\)[[:space:]]*$/\1/p' | tail -n 1 || true)
    if [ -z "$s" ] && [ -r "$INSTALL_DIR/data/.bootstrap-secret" ]; then
        s=$(tr -d '\n\r' < "$INSTALL_DIR/data/.bootstrap-secret")
    fi
    printf '%s' "$s"
}

register() {
    local secret code
    secret=$(bootstrap_secret)
    if [ -z "$secret" ]; then
        fail "bootstrap secret in docker logs docklift-backend"
        return
    fi
    pass "bootstrap secret found in backend logs"
    code=$(http POST /api/auth/register -H 'content-type: application/json' -H "x-bootstrap-secret: wrong-$secret" \
        -d "{\"email\":\"$EMAIL\",\"password\":\"$PASSWORD\",\"name\":\"Smoke Admin\"}")
    if [ "$code" = 201 ]; then fail "register with a wrong bootstrap secret is refused (got 201)"; else pass "register with a wrong bootstrap secret is refused ($code)"; fi
    code=$(http POST /api/auth/register -H 'content-type: application/json' -H "x-bootstrap-secret: $secret" \
        -d "{\"email\":\"$EMAIL\",\"password\":\"$PASSWORD\",\"name\":\"Smoke Admin\"}")
    expect "first admin registration" 201 "$code"
}

check() {
    local code token version
    code=$(http GET /api/health)
    expect "GET /api/health" 200 "$code"
    version=$(json_field version)
    if grep -q '"status":"ok"' "$BODY"; then pass "health status ok (version ${version:-?})"; else fail "health body: $(cat "$BODY")"; fi

    code=$(http GET /)
    expect "GET / (dashboard)" 200 "$code"
    if grep -q '<div id="root">' "$BODY"; then pass "dashboard HTML served"; else fail "dashboard HTML missing #root"; fi
    code=$(http GET /projects/some-client-route)
    expect "SPA fallback for client routes" 200 "$code"
    code=$(http GET /api/does-not-exist)
    expect "unknown /api path is a JSON 404" 404 "$code"

    code=$(http GET /api/projects)
    expect "GET /api/projects without a token" 401 "$code"

    code=$(http POST /api/auth/login -H 'content-type: application/json' \
        -d "{\"email\":\"$EMAIL\",\"password\":\"$PASSWORD\"}")
    expect "login" 200 "$code"
    token=$(json_field token)
    if [ -z "$token" ]; then
        fail "login returned no token"
        return
    fi
    code=$(http GET /api/auth/me -H "authorization: Bearer $token")
    expect "GET /api/auth/me" 200 "$code"
    code=$(http GET /api/projects -H "authorization: Bearer $token")
    expect "GET /api/projects" 200 "$code"
    code=$(http GET /api/system/version -H "authorization: Bearer $token")
    expect "GET /api/system/version" 200 "$code"
    code=$(http GET /api/domains -H "authorization: Bearer $token")
    expect "GET /api/domains" 200 "$code"

    # WebSocket upgrade without a terminal token is refused before any upgrade
    code=$(curl -sS -o /dev/null -w '%{http_code}' --http1.1 --max-time 10 \
        -H 'Connection: Upgrade' -H 'Upgrade: websocket' -H 'Sec-WebSocket-Version: 13' \
        -H 'Sec-WebSocket-Key: c21va2UtdGVzdC1rZXktMTIzNA==' -H "Origin: $URL" \
        "$URL/ws/terminal" 2>/dev/null || true)
    if [ "$code" = 401 ]; then pass "WebSocket /ws/terminal without a token refused (401)"; else fail "WebSocket /ws/terminal without a token: got ${code:-no response}, want 401"; fi

    if command -v docker >/dev/null 2>&1; then
        local c
        for c in docklift-backend docklift-nginx-proxy docklift-certbot; do
            if [ "$(docker inspect -f '{{.State.Running}}' "$c" 2>/dev/null)" = true ]; then pass "$c running"; else fail "$c not running"; fi
        done
        for c in docklift-frontend docklift-nginx; do
            if docker container inspect "$c" >/dev/null 2>&1; then fail "obsolete container $c still exists"; else pass "no $c container"; fi
        done
    fi
}

case "${1:-}" in
    register) register; check ;;
    check) check ;;
    *) echo "usage: $0 register|check" >&2; exit 2 ;;
esac

echo ""
if [ "$FAILS" -eq 0 ]; then echo "smoke: all checks passed"; else echo "smoke: $FAILS check(s) failed"; fi
[ "$FAILS" -eq 0 ]
