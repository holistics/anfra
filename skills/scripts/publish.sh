#!/usr/bin/env bash
# Publishes the anfra skills to the facade, holistics/anfra-skills, when they
# changed (docs/designs/agent-setup.md#releasing-the-skills).
#
# Usage: publish.sh <facade checkout> <version> <anfra commit> [--commit]
#
# It rebuilds what the facade holds, with the parameters the facade recorded in
# .build.json, and compares: the same tree means nothing that's published
# changed, the build script included, so it publishes nothing. Otherwise it
# writes this version's build into the checkout, and with --commit commits it
# and tags it anfra-v<version>; the caller pushes. A facade with no .build.json,
# as before the first publish, always gets this build.

set -euo pipefail

facade="${1:?usage: publish.sh <facade checkout> <version> <anfra commit> [--commit]}"
version="${2:?version}"
source="${3:?anfra commit}"
commit=false
[ "${4:-}" = --commit ] && commit=true

build="$(cd "$(dirname "$0")" && pwd)/build.mjs"
tmp=$(mktemp -d)
trap 'rm -rf "$tmp"' EXIT

# The facade's tree, without git's own folder.
tree() { (cd "$1" && find . -path ./.git -prune -o -type f -print | LC_ALL=C sort | xargs -d '\n' sha256sum); }

if [ -f "$facade/.build.json" ]; then
  was_version=$(node -e 'console.log(require(process.argv[1]).version)' "$facade/.build.json")
  was_source=$(node -e 'console.log(require(process.argv[1]).source)' "$facade/.build.json")
  node "$build" --version "$was_version" --source "$was_source" --out "$tmp/was" >/dev/null
  if [ "$(tree "$tmp/was")" = "$(tree "$facade")" ]; then
    echo "The anfra skills are unchanged since $was_version: nothing to publish."
    exit 0
  fi
  echo "The anfra skills changed since $was_version: publishing $version."
else
  echo "The facade has no build yet: publishing $version."
fi

node "$build" --version "$version" --source "$source" --out "$tmp/now" >/dev/null
# Everything in the facade comes from the build: replace all but .git.
find "$facade" -mindepth 1 -maxdepth 1 ! -name .git -exec rm -rf {} +
cp -R "$tmp/now/." "$facade/"

if [ "$commit" = true ]; then
  git -C "$facade" add -A
  git -C "$facade" commit -q -m "release: $version" -m "Published from https://github.com/holistics/anfra/tree/$source/skills."
  git -C "$facade" tag "anfra-v$version"
  echo "Committed and tagged anfra-v$version; push it to publish."
fi
