#!/usr/bin/env bash
# Bump the anfra release version and regenerate CHANGELOG.md from conventional
# commits. Review + commit the result and merge to master; tag_and_release.yml then
# tags `anfra-v<version>` (read from manifest.yml) and the assembler publishes it.
#
# Usage: pnpm bump <version>     e.g. pnpm bump 0.2.0
set -euo pipefail

version="${1:?usage: pnpm bump <version>   e.g. pnpm bump 0.2.0}"

# manifest.yml is the single source of truth for the release version.
sed -i.bak -E "s/^version: .*/version: ${version}/" manifest.yml && rm -f manifest.yml.bak

# Prepend the changes since the last anfra-v* tag to the changelog.
git fetch --tags --quiet
pnpm run changelog

echo "Bumped manifest.yml -> ${version} and updated CHANGELOG.md."
echo "Review both, commit, and merge to master to cut anfra-v${version}."
