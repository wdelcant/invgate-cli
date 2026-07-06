#!/usr/bin/env bash
set -euo pipefail

cd "$(dirname "$0")/.."
CURRENT=$(cat .version)
IFS='.' read -r major minor patch <<< "$CURRENT"

# Determine bump type from commit message
MSG=$(git log -1 --pretty=%B)
if echo "$MSG" | grep -q "BREAKING CHANGE\|^feat!:"; then
  major=$((major + 1)); minor=0; patch=0
elif echo "$MSG" | grep -qE "^feat[(:]"; then
  minor=$((minor + 1)); patch=0
else
  patch=$((patch + 1))
fi

NEW="${major}.${minor}.${patch}"
echo "Bumping $CURRENT → $NEW"
echo "$NEW" > .version
git add .version
git commit -m "chore(release): bump to v$NEW [skip ci]"
git tag "v$NEW"
git push
git push --tags
echo "✓ Released v$NEW"
