#!/usr/bin/env bash
# Check a pull request that changes manifest.yml's version (docs/designs/release.md). CI runs it
# on each pull request, on the commit GitHub merges it as, against its base.
#
# - An rc PR, to a version such as 0.23.0-rc.1: fails if its release is out already, since a
#   release that's out takes no more rcs. Anything else in it is up to its review.
# - A release PR, to a version such as 0.23.0: fails unless it's exactly what the bot generates,
#   `pnpm bump <version> <promoted_from>` on the base, so main's changelogs say what the release
#   does: its build merges them from the rc's tag, not from main. Edit an rc's notes in its rc PR.
#   It also fails once a newer rc of the release is out, since that rc's release PR replaces it.
#
# Any other pull request passes.
#
# Usage: scripts/check-release-pr.sh [base] [head]   (default HEAD^1 and HEAD)
set -euo pipefail
cd "$(dirname "$0")/.."

base="$(git rev-parse "${1:-HEAD^1}")"
head="$(git rev-parse "${2:-HEAD}")"
fail() { echo "::error::$*" >&2; exit 1; }
field() { git show "$1:manifest.yml" | sed -n "s/^$2: *//p"; }

from="$(field "$base" version)"
to="$(field "$head" version)"
if [ "$from" = "$to" ]; then
  echo "manifest.yml's version stays $to: not an rc PR or a release PR"
  exit 0
fi
git fetch --quiet --tags origin
release="${to%%-*}"

if [ "$to" != "$release" ]; then
  ! git rev-parse -q --verify "refs/tags/v$release" >/dev/null \
    || fail "v$release is out already, so it takes no more rcs: cut an rc of a later version"
  echo "rc PR for $to: its release isn't out yet"
  exit 0
fi

rc="$(field "$head" promoted_from)"
[ -n "$rc" ] || fail "this pull request releases $to without promoted_from: a release is made from an rc, whose build opens its release PR"

# The bot's release PR, generated on the base in a worktree of its own.
wt="$(mktemp -d)"
trap 'git worktree remove --force "$wt" >/dev/null 2>&1 || true; rm -rf "$wt"' EXIT
git worktree add --quiet --detach "$wt" "$base"
linked=false
if [ ! -e "$wt/node_modules" ]; then ln -s "$PWD/node_modules" "$wt/node_modules"; linked=true; fi
(cd "$wt" && bash scripts/bump.sh "$to" "$rc" >/dev/null) \
  || fail "the release PR for $rc can't be generated (above): if a newer rc is out, merge its release PR instead"
if $linked; then rm "$wt/node_modules"; fi

if ! git -C "$wt" diff --quiet "$head"; then
  git -C "$wt" diff --stat "$head" >&2
  git -C "$wt" diff "$head" >&2
  fail "this release PR isn't what the bot generates for $rc (the diff above turns this PR into it). Its notes are merged from the rcs' sections at $rc's tag, as the release's build merges them: edit an rc's notes in its rc PR, or cut another rc, and let the bot regenerate this PR."
fi
echo "release PR for $to, from $rc: exactly what the bot generates"
