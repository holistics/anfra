#!/usr/bin/env bash
# Tag HEAD with the current version of each plugin and of the marketplace,
# skipping tags that already exist on origin. Safe to run on every push.
#
# Tags: <plugin>-v<version> (from plugins/<plugin>/.claude-plugin/plugin.json)
#       marketplace-v<version> (from .claude-plugin/marketplace.json)
#
# Usage: tag-release.sh [--dry-run]

set -euo pipefail

cd "$(dirname "$0")/.."

DRY_RUN=false
[ "${1:-}" = "--dry-run" ] && DRY_RUN=true

tags=()
for manifest in plugins/*/.claude-plugin/plugin.json; do
  plugin=$(basename "$(dirname "$(dirname "$manifest")")")
  tags+=("${plugin}-v$(jq -r '.version' "$manifest")")
done
tags+=("marketplace-v$(jq -r '.metadata.version' .claude-plugin/marketplace.json)")

existing=$(git ls-remote --tags origin | awk '{print $2}')

new_tags=()
for tag in "${tags[@]}"; do
  if grep -qx "refs/tags/${tag}" <<<"$existing"; then
    echo "Skip: $tag already exists"
  else
    echo "New:  $tag"
    new_tags+=("$tag")
  fi
done

if [ "${#new_tags[@]}" -eq 0 ] || [ "$DRY_RUN" = true ]; then
  exit 0
fi

for tag in "${new_tags[@]}"; do
  git tag "$tag" HEAD
done
git push origin "${new_tags[@]/#/refs/tags/}"
