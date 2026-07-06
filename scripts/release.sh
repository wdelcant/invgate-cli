#!/usr/bin/env bash
set -euo pipefail
cd "$(dirname "$0")/.."

BUMP="${1:-patch}"
CURRENT=$(cat .version)
IFS='.' read -r major minor patch <<< "$CURRENT"

case "$BUMP" in
  major) major=$((major + 1)); minor=0; patch=0 ;;
  minor) minor=$((minor + 1)); patch=0 ;;
  patch) patch=$((patch + 1)) ;;
  *) echo "Usage: $0 {patch|minor|major}" >&2; exit 1 ;;
esac

NEW="${major}.${minor}.${patch}"
echo "$CURRENT → $NEW"
echo "$NEW" > .version
git add .version
git commit -m "chore(release): v$NEW"
git tag "v$NEW"
git push
git push --tags
echo "✓ Released v$NEW"
