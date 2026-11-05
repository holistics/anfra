#!/usr/bin/env bash
# Bump the anfra release version and regenerate CHANGELOG.md from conventional
# commits, and skills/CHANGELOG.md from those that changed the skills. Review + commit the result and merge to main; tag_and_release.yml then
# tags `v<version>` (read from manifest.yml) and the assembler publishes it.
#
# release_pr.yml runs it daily and opens the pull request; run it yourself for a release by hand.
#
# Usage: pnpm bump <version>     e.g. pnpm bump 0.2.0
#        pnpm bump next          the version the commits since the last release call for
#                                (scripts/next-version.mjs; `pnpm next-version` only prints it)
set -euo pipefail

version="${1:?usage: pnpm bump <version|next>   e.g. pnpm bump 0.2.0}"
if [ "$version" = next ]; then
  git fetch --tags --quiet
  version="$(node "$(dirname "$0")/next-version.mjs")"
  if [ -z "$version" ]; then
    echo "Nothing to release: no feat, fix, perf, build, security or breaking commit since the last release." >&2
    echo "Name a version to release anyway: pnpm bump <version>" >&2
    exit 1
  fi
  echo "Next version: $version"
fi

# manifest.yml is the single source of truth for the release version.
sed -i.bak -E "s/^version: .*/version: ${version}/" manifest.yml && rm -f manifest.yml.bak
# The SDK is published with each release, at its version (build_release.yml checks they agree).
(cd web/sdk && npm pkg set version="$version")
# So is the design package, which hosts take the tokens and rules from.
(cd web/design && npm pkg set version="$version")

# Prepend the changes since the last v* tag to the changelog. The pending
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
  -t v

# The skills' own changelog, published with them to holistics/anfra-skills: an
# entry only when a commit since the last release changed them.
skills_note=""
last="$(git describe --tags --abbrev=0 --match 'v*' 2>/dev/null || true)"
if [ -z "$last" ] || [ -n "$(git log --oneline "$last"..HEAD -- skills)" ]; then
  pnpm exec conventional-changelog \
    -n conventional-changelog.config.mjs \
    -c "$ctx" \
    -i skills/CHANGELOG.md -s \
    -t v \
    --commit-path skills
  skills_note=" and skills/CHANGELOG.md"
fi

echo "Bumped manifest.yml, web/sdk/package.json and web/design/package.json -> ${version} and updated CHANGELOG.md${skills_note}."
echo "Review them, commit, and merge to main to cut v${version}."
