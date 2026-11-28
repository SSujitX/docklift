#!/usr/bin/env bash
# Sync the product version from the root VERSION file.
# Usage:
#   ./scripts/sync-version.sh          # write VERSION into every copy below
#   ./scripts/sync-version.sh check    # exit 1 if any copy drifts from VERSION
#
# These read VERSION themselves (no copy to keep in sync): Dockerfile (Go binary
# stamp and dashboard build), Makefile, frontend/vite.config.ts (__APP_VERSION__),
# install.sh, install-dev.sh and upgrade.sh.
#
# Copies kept in sync here:
#   package.json            root "version". The upgrade.sh shipped with 3.0.0 (on
#                           disk on existing installs, run by the dashboard's
#                           Upgrade button) reads it from the new checkout.
#   frontend/package.json   "version" of the dashboard package.
#   EXAMPLE_FILES           release-pin examples: v=X.Y.Z, DOCKLIFT_VERSION=X.Y.Z
#                           and --build-arg VERSION=X.Y.Z. Every such example in
#                           these files is rewritten; name an older release in
#                           another form (e.g. "tag v2.1.6").
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT"

VERSION="$(tr -d '[:space:]' < VERSION)"
if [[ ! "$VERSION" =~ ^[0-9]+\.[0-9]+\.[0-9]+$ ]]; then
  echo "Invalid VERSION (expected X.Y.Z): '$VERSION'" >&2
  exit 1
fi

JSON_FILES=(package.json frontend/package.json)
EXAMPLE_FILES=(
  README.md
  commands.md
  website/guide/commands.md
  website/guide/installation.md
  install.sh
  upgrade.sh
  Dockerfile
)

# First "version": "..." of a package.json (the top-level field), set to VERSION.
render_json() {
  awk -v v="$VERSION" '
    !done && /"version"[[:space:]]*:[[:space:]]*"[^"]*"/ {
      sub(/"version"[[:space:]]*:[[:space:]]*"[^"]*"/, "\"version\": \"" v "\"")
      done = 1
    }
    { print }
    END { if (!done) exit 3 }
  ' "$1"
}

# Release-pin examples, set to VERSION.
render_examples() {
  sed -E \
    -e "s/(^|[^[:alnum:]_])v=[0-9]+\.[0-9]+\.[0-9]+/\1v=${VERSION}/g" \
    -e "s/DOCKLIFT_VERSION=[0-9]+\.[0-9]+\.[0-9]+/DOCKLIFT_VERSION=${VERSION}/g" \
    -e "s/--build-arg VERSION=[0-9]+\.[0-9]+\.[0-9]+/--build-arg VERSION=${VERSION}/g" \
    "$1"
}

# Line endings are ignored when comparing: a Windows checkout (core.autocrlf)
# has CRLF in the working tree; git stores LF either way.
lf() { tr -d '\r' < "$1"; }

render() {
  local kind=$1 file=$2
  if [[ ! -f "$file" ]]; then
    echo "missing $file" >&2
    return 1
  fi
  if [[ "$kind" == json ]]; then
    render_json <(lf "$file") || { echo "no \"version\" field in $file" >&2; return 1; }
  else
    render_examples <(lf "$file")
  fi
}

# Exit 0 when FILE (line endings aside) already equals the rendered $tmp.
same() { lf "$1" | cmp -s "$tmp" -; }

each_file() {
  local f
  for f in "${JSON_FILES[@]}"; do printf 'json %s\n' "$f"; done
  for f in "${EXAMPLE_FILES[@]}"; do printf 'examples %s\n' "$f"; done
}

tmp="$(mktemp)"
trap 'rm -f "$tmp"' EXIT

mode="${1:-sync}"

if [[ "$mode" == "check" ]]; then
  ok=1
  while read -r kind file; do
    if ! render "$kind" "$file" > "$tmp"; then
      ok=0
      continue
    fi
    if ! same "$file"; then
      echo "$file is out of sync with VERSION $VERSION:" >&2
      lf "$file" | diff - "$tmp" >&2 || true
      ok=0
    fi
  done < <(each_file)
  if [[ "$ok" -ne 1 ]]; then
    echo "Run ./scripts/sync-version.sh to fix." >&2
    exit 1
  fi
  echo "OK: every copy matches VERSION $VERSION"
  exit 0
fi

if [[ "$mode" != "sync" ]]; then
  echo "Usage: $0 [sync|check]" >&2
  exit 1
fi

echo "Syncing product version $VERSION"

while read -r kind file; do
  render "$kind" "$file" > "$tmp"
  if ! same "$file"; then
    # Overwrite in place: keeps the file's mode (install.sh, upgrade.sh) and,
    # on a CRLF working tree, its line endings.
    if grep -q $'\r' "$file"; then
      sed 's/$/\r/' "$tmp" > "$file"
    else
      cat "$tmp" > "$file"
    fi
    echo "  updated $file"
  fi
done < <(each_file)

echo "Done: every copy now at $VERSION"
bash "$ROOT/scripts/sync-version.sh" check
