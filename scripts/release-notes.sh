#!/usr/bin/env bash
# Print a release's notes. Its section of CHANGELOG.md, as `pnpm bump` wrote it, under
# "## Changelog"; then, given GitHub's generated notes for the release (a file), their pull
# requests and new contributors, and their compare link as the footer.
#
# Fails when the version has no section, so a release never goes out with notes that differ
# from the changelog.
#
# Usage: scripts/release-notes.sh <version> [generated-notes]   e.g. scripts/release-notes.sh 0.4.0
set -euo pipefail

version="${1:?usage: scripts/release-notes.sh <version> [generated-notes]   e.g. 0.4.0}"
generated="${2:-}"
changelog="$(dirname "$0")/../CHANGELOG.md"

# The version's section, without its own heading (the release's title says the version), and
# without the blank lines around it.
section="$(awk -v heading="## [$version]" '
  index($0, "## ") == 1 { if (found) exit; found = (index($0, heading) == 1); next }
  found
' "$changelog" | sed -e '/./,$!d' | sed -e ':a' -e '/^\n*$/{$d;N;ba' -e '}')"

if [ -z "$section" ]; then
  echo "CHANGELOG.md has no section for $version: run pnpm bump $version" >&2
  exit 1
fi

printf '## Changelog\n\n%s\n' "$section"
if [ -n "$generated" ]; then
  # GitHub's sections are ## already: rename its list of pull requests, and set its compare
  # link apart as the footer.
  printf '\n'
  awk '
    $0 == "## What'"'"'s Changed" { print "## Pull requests"; next }
    index($0, "**Full Changelog**") == 1 { print "---"; print "" }
    { print }
  ' "$generated"
fi
