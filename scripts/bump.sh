#!/usr/bin/env bash
# Bump anfra to a release candidate (rc), or to the release an rc leads to, in manifest.yml,
# web/sdk/package.json and web/design/package.json, and write the changelogs to match
# (docs/designs/release.md). Review + commit the result and merge to main; tag_and_release.yml
# then tags it, and build_release.yml publishes it.
#
# - An rc, such as 0.23.0-rc.1: adds its section to CHANGELOG.md, from the conventional commits
#   since the newest tag, rc or release, and to skills/CHANGELOG.md, when one of those commits
#   changed the skills. Edit the sections before you commit, if they need it. It also removes the
#   `promoted_from` of the release before.
# - A release, such as 0.23.0: sets `promoted_from` to the rc it releases, the newest rc of that
#   release unless you name one, and replaces the rc sections of both changelogs with the release's
#   (scripts/changelog-merge.mjs). It merges them as they are at the rc's tag, as the release's
#   build does, so main's changelogs say what the release does.
#
# release_pr.yml cuts the rcs, and build_release.yml opens each rc's release PR; run it yourself
# only for a release by hand.
#
# Usage: pnpm bump <version> [rc]   e.g. pnpm bump 0.23.0-rc.1, pnpm bump 0.23.0
#        pnpm bump next             the next rc the commits since the last release call for
#                                   (scripts/next-version.mjs; `pnpm next-version` only prints it)
set -euo pipefail
cd "$(dirname "$0")/.."

usage="usage: pnpm bump <version> [rc]   e.g. pnpm bump 0.23.0-rc.1, pnpm bump 0.23.0, pnpm bump next"
version="${1:?$usage}"
rc="${2:-}"
git fetch --tags --quiet

if [ "$version" = next ]; then
  version="$(node scripts/next-version.mjs)"
  if [ -z "$version" ]; then
    echo "Nothing to release: no feat, fix, perf, build, security or breaking commit since the last release." >&2
    echo "Name a version to release anyway: pnpm bump <version>-rc.1" >&2
    exit 1
  fi
  echo "Next version: $version"
fi
version="${version#v}"
case "$version" in
  *-*) [[ "$version" =~ ^[0-9]+\.[0-9]+\.[0-9]+-rc\.[1-9][0-9]*$ ]] || { echo "$version is not an rc's version, such as 0.23.0-rc.1" >&2; exit 1; } ;;
  *) [[ "$version" =~ ^[0-9]+\.[0-9]+\.[0-9]+$ ]] || { echo "$version is not a version, such as 0.23.0 or 0.23.0-rc.1" >&2; exit 1; } ;;
esac
release="${version%%-*}"
if git rev-parse -q --verify "refs/tags/v$release" >/dev/null; then
  echo "v$release is released already: bump to an rc of a later version" >&2
  exit 1
fi

set_versions() {
  # manifest.yml is the single source of truth for the release version.
  sed -i.bak -E "s/^version: .*/version: ${version}/" manifest.yml && rm -f manifest.yml.bak
  # The SDK is published with each release, at its version (build_release.yml checks they agree).
  (cd web/sdk && npm pkg set version="$version")
  # So is the design package, which hosts take the tokens and rules from.
  (cd web/design && npm pkg set version="$version")
  # Only a release has `promoted_from`.
  sed -i.bak -E '/^promoted_from: /d' manifest.yml && rm -f manifest.yml.bak
}

tmp="$(mktemp -d)"
trap 'rm -rf "$tmp"' EXIT

if [ "$version" != "$release" ]; then
  set_versions

  # Prepend the changes since the newest v* tag to the changelog. The version is passed via a
  # context file (mirrors canal's bump), which must end in .json, since conventional-changelog
  # loads -c by its extension; otherwise it reads it from package.json, which has none.
  ctx="$tmp/context.json"
  printf '{ "version": "%s" }\n' "$version" > "$ctx"
  pnpm exec conventional-changelog -n conventional-changelog.config.mjs -c "$ctx" -i CHANGELOG.md -s -t v

  # The skills' own changelog, published with them to holistics/anfra-skills: a section only when a
  # commit since the newest tag changed them.
  skills_note=""
  last="$(git describe --tags --abbrev=0 --match 'v*' 2>/dev/null || true)"
  if [ -z "$last" ] || [ -n "$(git log --oneline "$last"..HEAD -- skills)" ]; then
    pnpm exec conventional-changelog -n conventional-changelog.config.mjs -c "$ctx" \
      -i skills/CHANGELOG.md -s -t v --commit-path skills
    skills_note=" and skills/CHANGELOG.md"
  fi
  echo "Bumped manifest.yml, web/sdk/package.json and web/design/package.json -> ${version}, and added its section to CHANGELOG.md${skills_note}."
  echo "Review them, edit the notes if they need it, commit, and merge to main to cut v${version}."
  exit 0
fi

# A release: of an rc, which must be the newest of its release, so the release is the code the rc
# was checked as.
newest="$(node scripts/lib/versions.mjs newest-rc "$release")"
[ -n "$newest" ] || { echo "$release has no rc: a release is made from an rc, so cut one first (pnpm bump ${release}-rc.1)" >&2; exit 1; }
rc="${rc:-$newest}"
rc="${rc#v}"
[ "$rc" = "$newest" ] || { echo "$rc is not the newest rc of $release, which is $newest: release that one" >&2; exit 1; }

set_versions
sed -i.bak -E "s/^version: .*/&\npromoted_from: ${rc}/" manifest.yml && rm -f manifest.yml.bak

# The release's sections, merged from the changelogs at the rc's tag, on top of main's sections for
# the releases before it.
git show "v$rc:CHANGELOG.md" > "$tmp/CHANGELOG.md"
git show "v$rc:skills/CHANGELOG.md" > "$tmp/skills-CHANGELOG.md"
node scripts/changelog-merge.mjs "$release" "$tmp/CHANGELOG.md" "$tmp/skills-CHANGELOG.md"
# merged prints a changelog's first section, if it's the release's.
merged() { awk -v h="## [$release]" 'NR == 1 && index($0, h) != 1 { exit } /^## \[/ { n++ } n == 1' "$1"; }
# releases prints a changelog from its first release's section on, without the rc sections above it.
releases() { awk '/^## \[/ && !/^## \[[^]]*-rc\.[0-9]+\]/ { on = 1 } on' "$1"; }
for f in CHANGELOG.md skills/CHANGELOG.md; do
  src="$tmp/CHANGELOG.md"
  [ "$f" = CHANGELOG.md ] || src="$tmp/skills-CHANGELOG.md"
  { merged "$src"; releases "$f"; } > "$tmp/out"
  cat "$tmp/out" > "$f"
done

echo "Bumped manifest.yml, web/sdk/package.json and web/design/package.json -> ${release}, from ${rc}, and merged its rc sections in CHANGELOG.md and skills/CHANGELOG.md."
echo "Commit, and merge to main to release ${rc}'s commit as v${release}."
