#!/usr/bin/env bash
# Publishing to the facade (skills/scripts/publish.sh), against a local git repo
# standing in for holistics/anfra-skills: the first publish, a release that
# changes nothing, one that changes the skills, and a hand-edited facade.

set -uo pipefail

skills="$(cd "$(dirname "$0")/.." && pwd)"
publish="$skills/scripts/publish.sh"
tmp=$(mktemp -d)
trap 'rm -rf "$tmp"' EXIT

failed=0
check() { # name, want (a substring), got
  if [[ "$3" == *"$2"* ]]; then echo "ok   $1"; else echo "FAIL $1: got [$3]"; failed=1; fi
}

facade="$tmp/facade"
git init -q "$facade"
git -C "$facade" config user.email test@example.com
git -C "$facade" config user.name test
echo "old" > "$facade/README.md"
git -C "$facade" add -A && git -C "$facade" commit -q -m "before the move"

out=$(bash "$publish" "$facade" 0.15.0 aaa --commit)
check "the first publish" "The facade has no build yet: publishing 0.15.0." "$out"
check "it's committed" "release: 0.15.0" "$(git -C "$facade" log -1 --format=%s)"
check "and tagged" "anfra-v0.15.0" "$(git -C "$facade" tag)"
check "the old content is gone" "This repo is published from" "$(head -1 "$facade/README.md")"

# A relative facade path, as the release passes.
out=$(cd "$tmp" && bash "$publish" facade 0.16.0 bbb --commit)
check "an unchanged release publishes nothing" "unchanged since 0.15.0: nothing to publish" "$out"
check "and commits nothing" "release: 0.15.0" "$(git -C "$facade" log -1 --format=%s)"

echo "edited by hand" >> "$facade/plugins/anfra/skills/anfra-aml/SKILL.md"
git -C "$facade" commit -q -am "a hand edit"
out=$(bash "$publish" "$facade" 0.17.0 ccc --commit)
check "a hand-edited facade counts as changed" "changed since 0.15.0: publishing 0.17.0" "$out"
check "and is overwritten" "anfra skills 0.17.0" "$(tail -1 "$facade/plugins/anfra/skills/anfra-aml/SKILL.md")"
check "with the new parameters" '"source": "ccc"' "$(cat "$facade/.build.json")"

out=$(bash "$publish" "$facade" 0.18.0 ddd)
check "without --commit, nothing is committed" "release: 0.17.0" "$(git -C "$facade" log -1 --format=%s)"

exit "$failed"
