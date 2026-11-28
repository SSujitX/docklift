#!/usr/bin/env bash
# Release notes for the Release workflow, from the git log since the previous tag.
# Usage:
#   ./scripts/release-notes.sh vX.Y.Z NOTES_FILE
#
# Writes the GitHub Release body to NOTES_FILE and adds the matching section at
# the top of CHANGELOG.md. Run it before the release commit (the range ends at
# HEAD), so the CHANGELOG section goes into that commit. The section heading is
# "## [X.Y.Z](compare link) (YYYY-MM-DD)", the form website/.vitepress/changelog.data.ts
# splits releases on. Merge commits and earlier release commits are left out.
#
# GITHUB_REPOSITORY (owner/name) picks the repository for links; default
# SSujitX/docklift.
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT"

TAG="${1:-}"
NOTES_FILE="${2:-}"
if [[ ! "$TAG" =~ ^v([0-9]+\.[0-9]+\.[0-9]+)$ ]] || [[ -z "$NOTES_FILE" ]]; then
  echo "Usage: $0 vX.Y.Z NOTES_FILE" >&2
  exit 1
fi
VERSION="${BASH_REMATCH[1]}"
REPO_URL="https://github.com/${GITHUB_REPOSITORY:-SSujitX/docklift}"

# Newest v* tag already in this history, other than TAG itself.
prev="$(git tag -l 'v*' --merged HEAD --sort=-v:refname | grep -vxF "$TAG" | head -n1 || true)"
range="HEAD"
if [[ -n "$prev" ]]; then
  range="${prev}..HEAD"
fi

# Oldest -> newest so the release reads from the last tag to this one.
# Markdown links so SHAs are clickable in the GitHub Release UI and on docklift.dev.
entries="$(git log "$range" --no-merges --reverse --pretty=format:'%H%x09%h%x09%s' \
  | awk -F '\t' -v url="$REPO_URL" '
      $3 ~ /^chore\(release\):/ { next }
      { printf "- %s ([%s](%s/commit/%s))\n", $3, $2, url, $1 }
    ')"
if [[ -z "$entries" ]]; then
  echo "No commits since ${prev:-the first commit}: nothing to release." >&2
  exit 1
fi

if [[ -n "$prev" ]]; then
  compare="${REPO_URL}/compare/${prev}...${TAG}"
else
  compare="${REPO_URL}/commits/${TAG}"
fi

{
  echo "## What's Changed"
  echo
  printf '%s\n' "$entries"
  echo
  if [[ -n "$prev" ]]; then
    echo "**Full Changelog**: ${compare}"
  else
    echo "_First tagged release._"
  fi
} > "$NOTES_FILE"

changelog_tmp="$(mktemp)"
trap 'rm -f "$changelog_tmp"' EXIT
{
  echo "## [${VERSION}](${compare}) ($(date -u +%Y-%m-%d))"
  echo
  printf '%s\n' "$entries"
  echo
  if [[ -f CHANGELOG.md ]]; then
    tr -d '\r' < CHANGELOG.md
  fi
} > "$changelog_tmp"
cat "$changelog_tmp" > CHANGELOG.md

echo "----- release notes (${prev:-no previous tag} -> ${TAG}) -----"
cat "$NOTES_FILE"
