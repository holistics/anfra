#!/usr/bin/env bash
# Bump the anfra release version and regenerate CHANGELOG.md from conventional
# commits. Review + commit the result and merge to main; tag_and_release.yml then
# tags `anfra-v<version>` (read from manifest.yml) and the assembler publishes it.
#
# Usage: pnpm bump <version>     e.g. pnpm bump 0.2.0
set -euo pipefail

version="${1:?usage: pnpm bump <version>   e.g. pnpm bump 0.2.0}"

# manifest.yml is the single source of truth for the release version.
sed -i.bak -E "s/^version: .*/version: ${version}/" manifest.yml && rm -f manifest.yml.bak

# Prepend the changes since the last anfra-v* tag to the changelog. The pending
# release version is passed via a context file (mirrors canal's bump) — otherwise
# conventional-changelog reads it from package.json (which has none) and emits an
# empty "## []" header.
git fetch --tags --quiet
# Pass the pending version via a context file (mirrors canal's bump). It must end
# in .json — conventional-changelog loads -c by file extension.
ctx="/tmp/anfra-bump-context.json"
printf '{ "version": "%s" }\n' "$version" > "$ctx"
pnpm exec conventional-changelog \
  -n conventional-changelog.config.mjs \
  -c "$ctx" \
  -i CHANGELOG.md -s \
  -t anfra-v

echo "Bumped manifest.yml -> ${version} and updated CHANGELOG.md."
echo "Review both, commit, and merge to main to cut anfra-v${version}."
