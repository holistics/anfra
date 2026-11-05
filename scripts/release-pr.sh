#!/usr/bin/env bash
# Open, update or close the pull request that cuts the next release: `pnpm bump` on a branch fresh
# from main, committed as `release: <version>`. release_pr.yml runs it; see its header for when,
# and why release/next is overwritten.
#
# - The version: --version, else scripts/next-version.mjs's, from the commits since the last tag.
# - The branch: --branch, which must not exist yet (it is never overwritten), else release/next,
#   rebuilt and force-pushed.
# - Nothing releasable on release/next: its open pull request is closed. On a named branch,
#   nothing releasable without --version fails.
#
# The commit's author is the bot that $GIT_BOT names (a login, `<name>[bot]`), and gh acts as
# whoever $GH_TOKEN is.
#
# --dry-run does everything but push and touch pull requests, on a local branch, and prints the
# pull request it would open; check out your branch again after.
#
# Usage: scripts/release-pr.sh [--version <version>] [--branch <new branch>] [--dry-run]
set -euo pipefail

DEFAULT_BRANCH=release/next
BASE=main

version="" branch="" dry_run=false
while [ $# -gt 0 ]; do
  case "$1" in
    --version) version="${2#v}"; shift 2 ;;
    --branch) branch="$2"; shift 2 ;;
    --dry-run) dry_run=true; shift ;;
    *) echo "usage: $0 [--version <version>] [--branch <new branch>] [--dry-run]" >&2; exit 2 ;;
  esac
done
branch="${branch:-$DEFAULT_BRANCH}"

cd "$(dirname "$0")/.."
fail() { echo "::error::$*" >&2; exit 1; }
run() { if $dry_run; then echo "(dry run) $*"; else "$@"; fi; }

git diff --quiet HEAD || fail "the working tree has changes: commit or stash them first"
git fetch --quiet --tags origin "$BASE"
base="origin/$BASE"

# The branch: release/next is ours to overwrite; any other must be new.
if [ "$branch" != "$DEFAULT_BRANCH" ]; then
  git check-ref-format --branch "$branch" >/dev/null || fail "'$branch' is not a valid branch name"
  if git ls-remote --exit-code --heads origin "refs/heads/$branch" >/dev/null; then
    fail "branch $branch exists already: name a new one, so its commits are not overwritten"
  fi
fi

# The version.
last="$(git describe --tags --abbrev=0 --match 'v*' "$base")"
if [ -n "$version" ]; then
  semver='^(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)(-[0-9A-Za-z.-]+)?$'
  [[ "$version" =~ $semver ]] || fail "'$version' is not a version, e.g. 0.19.0"
  ! git rev-parse -q --verify "refs/tags/v$version" >/dev/null || fail "v$version is released already"
  newest="$(printf '%s\n%s\n' "${last#v}" "${version%%-*}" | sort -V | tail -1)"
  [ "$newest" = "${version%%-*}" ] || fail "$version is older than the last release, $last"
else
  version="$(node scripts/next-version.mjs "$base")"
fi

open_pr() { gh pr list --head "$branch" --base "$BASE" --state open --json number -q '.[0].number'; }

# Nothing to release (one was merged since, say): retire release/next's pull request rather than
# leave one that would release nothing.
if [ -z "$version" ]; then
  [ "$branch" = "$DEFAULT_BRANCH" ] || fail "nothing releasable since $last: give a version to release anyway"
  echo "Nothing releasable since $last"
  pr="$(open_pr)"
  if [ -n "$pr" ]; then
    run gh pr close "$pr" --delete-branch \
      --comment "Nothing to release: $BASE has no releasable commits since $last. The next run that finds some opens a new pull request."
  fi
  exit 0
fi
echo "Releasing $version (after $last) from $branch"

# The bump, committed as the bot.
bot="${GIT_BOT:-github-actions[bot]}"
bot_id="$(gh api "users/$bot" --jq .id)"
git checkout --quiet -B "$branch" "$base"
pnpm bump "$version"
git add -A
git -c user.name="$bot" -c user.email="$bot_id+$bot@users.noreply.github.com" \
  commit --quiet -m "release: $version"
git show --stat --format='%an <%ae>%n%s' HEAD

if [ "$branch" = "$DEFAULT_BRANCH" ]; then
  run git push --force origin "HEAD:refs/heads/$branch"
else
  run git push origin "HEAD:refs/heads/$branch" # never forced: a branch that appeared since fails it
fi

# The pull request. The changelog first, so a notification's preview shows what the release brings.
body="$(mktemp)"
trap 'rm -f "$body"' EXIT
{
  scripts/release-notes.sh "$version"
  echo
  echo "---"
  echo
  echo "Cuts anfra $version: merging this tags \`v$version\`, and the release builds and publishes it."
  echo
  if [ "$branch" = "$DEFAULT_BRANCH" ]; then
    echo "> [!NOTE]"
    echo "> Opened by [Release PR](${GITHUB_SERVER_URL:-https://github.com}/${GITHUB_REPOSITORY:-$(gh repo view --json nameWithOwner -q .nameWithOwner)}/actions/workflows/release_pr.yml): it rebuilds \`$branch\` from $BASE every day except Sunday and force-pushes it, so commits pushed here are lost. A release that needs commits of its own (sidecar pins, say): run the workflow with a new \`branch\`, and push them there."
    echo
  fi
  # TODO: drop once release_pr.yml uses a GitHub App's token.
  echo "> [!IMPORTANT]"
  echo "> CI does not start on its own on this pull request: approve its workflows (the button on the pull request) to run it, again after each update."
} > "$body"

title="release: $version"
if $dry_run; then
  printf '\n(dry run) pull request from %s: %s\n\n' "$branch" "$title"
  cat "$body"
  exit 0
fi
pr="$(open_pr)"
if [ -n "$pr" ]; then
  gh pr edit "$pr" --title "$title" --body-file "$body"
  echo "Updated #$pr"
else
  gh pr create --base "$BASE" --head "$branch" --title "$title" --body-file "$body"
fi
