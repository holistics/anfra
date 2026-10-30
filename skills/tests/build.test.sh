#!/usr/bin/env bash
# The facade's build (skills/scripts/build.mjs): its output is a function of
# skills/ and its parameters, so the publish job can rebuild what the facade
# holds and compare.

set -uo pipefail

skills="$(cd "$(dirname "$0")/.." && pwd)"
tmp=$(mktemp -d)
trap 'rm -rf "$tmp"' EXIT

failed=0
check() { # name, then the command that must succeed
  local name=$1; shift
  if "$@" >/dev/null 2>&1; then echo "ok   $name"; else echo "FAIL $name"; failed=1; fi
}

build() { node "$skills/scripts/build.mjs" --version "$1" --source "$2" --out "$3" >/dev/null; }

build 0.15.0 abc123 "$tmp/a"
build 0.15.0 abc123 "$tmp/b"
build 0.15.1 abc123 "$tmp/c"

check "the same parameters give the same bytes" diff -r "$tmp/a" "$tmp/b"
check "another version gives other bytes" bash -c "! diff -rq '$tmp/a' '$tmp/c'"

stamps_ok=1
for f in "$tmp"/a/plugins/*/skills/*/SKILL.md; do
  [ "$(tail -n 2 "$f")" = $'---\nanfra skills 0.15.0' ] || { echo "     no stamp: $f"; stamps_ok=0; }
done
check "every skill ends with the stamp" test "$stamps_ok" = 1

versions_ok=1
for f in "$tmp"/a/.claude-plugin/marketplace.json "$tmp"/a/.cursor-plugin/marketplace.json "$tmp"/a/.agents/plugins/marketplace.json; do
  [ "$(node -e "console.log(require(process.argv[1]).metadata.version)" "$f")" = 0.15.0 ] || { echo "     no version: $f"; versions_ok=0; }
done
for f in "$tmp"/a/plugins/*/.*-plugin/plugin.json; do
  [ "$(node -e "console.log(require(process.argv[1]).version)" "$f")" = 0.15.0 ] || { echo "     no version: $f"; versions_ok=0; }
done
check "every manifest has the version" test "$versions_ok" = 1

check "the parameters are recorded" bash -c "node -e 'const b=require(process.argv[1]); process.exit(b.version===\"0.15.0\" && b.source===\"abc123\" ? 0 : 1)' '$tmp/a/.build.json'"
check "the README says where it comes from" grep -q "holistics/anfra/tree/abc123/skills" "$tmp/a/README.md"
check "the changelog is published" test -s "$tmp/a/CHANGELOG.md"
check "the source is left as it was" bash -c "! grep -rq '^anfra skills ' '$skills/plugins' && ! grep -rq '\"version\"' '$skills/plugins' '$skills/.claude-plugin'"

exit "$failed"
