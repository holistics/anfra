#!/usr/bin/env bash
# Open, replace or close the pull requests that make an anfra release (docs/designs/release.md):
#
# - The rc PR cuts a release candidate (rc): `pnpm bump <rc>` on a branch fresh from main,
#   committed as `release: <rc>`. release_candidate_pr.yml runs this daily, and by hand.
# - The release PR (--release <rc>) releases an rc: `pnpm bump <release> <rc>`, committed as
#   `release: <release>`. build_release.yml runs this once the rc is out.
#
# The branch: the rc PR's is release/rc, or a new one --branch names, which must not exist yet
# (it is never overwritten). The release PR's is release/next. The bot owns release/rc and
# release/next, and rebuilds and force-pushes them: the open pull request on the branch is closed,
# and a new one opened, so its readers are notified, unless it holds this version of the current
# main already, when nothing changes.
#
# While an rc waits for its release (scripts/lib/versions.mjs waiting), a daily run (--daily)
# cuts no rc. If main has changes the changelog lists since the rc, it comments on the release PR
# instead, hiding its comment of the day before, and fails if no release PR is open. A run by hand
# cuts the next rc then: of the same release, or of a later one if main's commits call for it.
#
# With nothing releasable since the last release, a daily run closes the rc PR, and a run by hand
# fails unless it names a version.
#
# The commit's author is the bot that $GIT_BOT names (a login, `<name>[bot]`), and gh acts as
# whoever $GH_TOKEN is.
#
# --dry-run does everything but push and touch pull requests, on a local branch, and prints the
# pull request it would open, or the comment it would post; check out your branch again after.
#
# Usage: scripts/release-pr.sh [--daily] [--version <rc>] [--branch <new branch>] [--dry-run]
#        scripts/release-pr.sh --release <rc> [--dry-run]
set -euo pipefail

RC_BRANCH=release/rc
RELEASE_BRANCH=release/next
BASE=main
usage="usage: $0 [--daily] [--version <rc>] [--branch <new branch>] [--dry-run] | --release <rc> [--dry-run]"

version="" branch="" release_of="" daily=false dry_run=false
while [ $# -gt 0 ]; do
  case "$1" in
    --version) version="${2#v}"; shift 2 ;;
    --branch) branch="$2"; shift 2 ;;
    --release) release_of="${2#v}"; shift 2 ;;
    --daily) daily=true; shift ;;
    --dry-run) dry_run=true; shift ;;
    *) echo "$usage" >&2; exit 2 ;;
  esac
done

cd "$(dirname "$0")/.."
fail() { echo "::error::$*" >&2; exit 1; }
run() { if $dry_run; then echo "(dry run) $*"; else "$@"; fi; }
versions() { node scripts/lib/versions.mjs "$@"; }
repo_url() { echo "${GITHUB_SERVER_URL:-https://github.com}/${GITHUB_REPOSITORY:-$(gh repo view --json nameWithOwner -q .nameWithOwner)}"; }

git diff --quiet HEAD || fail "the working tree has changes: commit or stash them first"
git fetch --quiet --tags origin "$BASE"
base="origin/$BASE"
open_pr() { gh pr list --head "$1" --base "$BASE" --state open --json number -q '.[0].number'; }

# An rc or a release merged to main whose tag isn't pushed yet: tag_and_release.yml is about to
# tag it, and a run after that sees it.
pending="$(git show "$base:manifest.yml" | sed -n 's/^version: *//p')"
if ! git rev-parse -q --verify "refs/tags/v$pending" >/dev/null; then
  echo "$BASE has $pending, which isn't tagged yet: run again once tag_and_release.yml has tagged it"
  exit 0
fi

# comment_waiting comments on the release PR of the waiting rc, when main has changes since it.
comment_waiting() {
  local rc="$1" pr preview tagged days body marker="<!-- release-pr: waiting -->"
  pr="$(open_pr "$RELEASE_BRANCH")"
  [ -n "$pr" ] || fail "$rc waits for its release, but no release PR is open on $RELEASE_BRANCH. Cut the next rc by hand (run Release candidate PR with no inputs), or reopen the release PR."

  # What main holds back, as the changelog would list it: the changes since the newest tag.
  git checkout --quiet --detach "$base"
  printf '{ "version": "next" }\n' > "$tmp/context.json"
  preview="$(pnpm exec conventional-changelog -n conventional-changelog.config.mjs -c "$tmp/context.json" -t v | tail -n +2 | sed -e '/./,$!d')"
  if ! grep -q '^\* ' <<<"$preview"; then
    echo "$rc waits for its release, and $BASE has nothing releasable since: no comment"
    return
  fi
  tagged="$(git log -1 --format=%cs "v$rc")"
  days=$(( ( $(date +%s) - $(git log -1 --format=%ct "v$rc") ) / 86400 ))
  body="$marker
\`$rc\` has waited for its release since $tagged ($days days). These changes merged since then wait for the release after it:

$preview

Merge this PR once anfra Cloud runs \`$rc\`. To ship the changes above first, cut the next rc by hand: run [Release candidate PR]($(repo_url)/actions/workflows/release_candidate_pr.yml) with no inputs."
  if $dry_run; then
    printf '(dry run) comment on #%s:\n\n%s\n' "$pr" "$body"
    return
  fi
  # Hide the comments of the days before, so only the newest shows.
  gh api --paginate "repos/{owner}/{repo}/issues/$pr/comments" \
    --jq ".[] | select(.body | startswith(\"$marker\")) | .node_id" |
    while read -r id; do
      gh api graphql -f query='mutation($id: ID!) { minimizeComment(input: {subjectId: $id, classifier: OUTDATED}) { clientMutationId } }' -f id="$id" >/dev/null
    done
  gh pr comment "$pr" --body "$body"
}

tmp="$(mktemp -d)"
trap 'rm -rf "$tmp"' EXIT

if [ -n "$release_of" ]; then
  # The release PR, for an rc that's out.
  [ -z "$version$branch" ] && ! $daily || fail "--release takes no other option but --dry-run"
  [[ "$release_of" =~ ^[0-9]+\.[0-9]+\.[0-9]+-rc\.[0-9]+$ ]] || fail "'$release_of' is not an rc's version, e.g. 0.23.0-rc.1"
  release="${release_of%%-*}"
  git rev-parse -q --verify "refs/tags/v$release_of" >/dev/null || fail "$release_of has no tag v$release_of"
  ! git rev-parse -q --verify "refs/tags/v$release" >/dev/null || fail "v$release is released already"
  newest="$(versions newest-rc "$release")"
  [ "$newest" = "$release_of" ] || fail "$release_of is not the newest rc of $release, which is $newest"
  branch="$RELEASE_BRANCH" version="$release" bump=("$release" "$release_of")
else
  # The rc PR.
  waiting="$(versions waiting)"
  if $daily && [ -n "$waiting" ]; then
    comment_waiting "$waiting"
    exit 0
  fi
  if [ -n "$branch" ]; then
    git check-ref-format --branch "$branch" >/dev/null || fail "'$branch' is not a valid branch name"
    if git ls-remote --exit-code --heads origin "refs/heads/$branch" >/dev/null; then
      fail "branch $branch exists already: name a new one, so its commits are not overwritten"
    fi
  fi
  branch="${branch:-$RC_BRANCH}"
  last="v$(versions last-release "$base")"
  if [ -n "$version" ]; then
    [[ "$version" =~ ^[0-9]+\.[0-9]+\.[0-9]+-rc\.[1-9][0-9]*$ ]] \
      || fail "'$version' is not an rc's version, e.g. 0.23.0-rc.1: a release is made from an rc, whose build opens the release PR"
    ! git rev-parse -q --verify "refs/tags/v$version" >/dev/null || fail "v$version is tagged already"
    ! git rev-parse -q --verify "refs/tags/v${version%%-*}" >/dev/null || fail "v${version%%-*} is released already"
  else
    version="$(node scripts/next-version.mjs "$base")"
  fi
  # Nothing to release (one was merged since, say): retire the rc PR rather than leave one that
  # would release nothing.
  if [ -z "$version" ]; then
    [ "$branch" = "$RC_BRANCH" ] && $daily || fail "nothing releasable since $last: give a version to release anyway"
    echo "Nothing releasable since $last"
    pr="$(open_pr "$branch")"
    if [ -n "$pr" ]; then
      run gh pr close "$pr" --delete-branch \
        --comment "Nothing to release: $BASE has no releasable commits since $last. The next run that finds some opens a new pull request."
    fi
    exit 0
  fi
  bump=("$version")
fi
title="release: $version"

# The bot's branch holds this version of the current main already, and a release PR this rc's
# release: change nothing, so the pull request's readers get no notification for a run that brings
# nothing new.
if { [ "$branch" = "$RC_BRANCH" ] || [ "$branch" = "$RELEASE_BRANCH" ]; } \
  && git fetch --quiet origin "+refs/heads/$branch:refs/remotes/origin/$branch" 2>/dev/null \
  && [ "$(git rev-parse "origin/$branch^")" = "$(git rev-parse "$base")" ] \
  && [ "$(git log -1 --format=%s "origin/$branch")" = "$title" ] \
  && [ "$(git show "origin/$branch:manifest.yml" | sed -n 's/^promoted_from: *//p')" = "$release_of" ] \
  && [ -n "$(open_pr "$branch")" ]; then
  echo "$branch is up to date: $title, on $base's $(git rev-parse --short "$base")"
  exit 0
fi
echo "Opening $title from $branch"

# The bump, committed as the bot.
bot="${GIT_BOT:-github-actions[bot]}"
bot_id="$(gh api "users/$bot" --jq .id)"
git checkout --quiet -B "$branch" "$base"
pnpm bump "${bump[@]}"
git add -A
git -c user.name="$bot" -c user.email="$bot_id+$bot@users.noreply.github.com" \
  commit --quiet -m "$title"
git show --stat --format='%an <%ae>%n%s' HEAD

if [ "$branch" = "$RC_BRANCH" ] || [ "$branch" = "$RELEASE_BRANCH" ]; then
  run git push --force origin "HEAD:refs/heads/$branch"
else
  run git push origin "HEAD:refs/heads/$branch" # never forced: a branch that appeared since fails it
fi

# The pull request. The changelog first, so a notification's preview shows what it brings.
{
  scripts/release-notes.sh "$version"
  echo
  echo "---"
  echo
  if [ -n "$release_of" ]; then
    echo "Releases \`$release_of\`'s commit as anfra $version: merging this tags \`v$version\` on it, and the release builds and publishes it. **Merge it once anfra Cloud runs \`$release_of\`.**"
    echo
    echo "The notes are the rcs' sections of the changelogs, merged. CI fails if this pull request changes them, or anything else: edit an rc's notes in its rc PR, or cut another rc."
    echo
    echo "> [!NOTE]"
    echo "> Opened by [Build anfra release]($(repo_url)/actions/workflows/build_release.yml) once \`$release_of\` was out. Each rc's build replaces it on \`$branch\`, so commits pushed here are lost."
  else
    echo "Cuts the release candidate anfra $version: merging this tags \`v$version\`, and the release builds and publishes it as a prerelease, which \`anfra update\` doesn't install. Its build then opens the release PR, which releases it once anfra Cloud runs it."
    echo
    echo "Edit the notes here, if they need it: they're this rc's section of \`CHANGELOG.md\`, and of \`skills/CHANGELOG.md\` when the skills changed."
    if [ "$branch" = "$RC_BRANCH" ]; then
      echo
      echo "> [!NOTE]"
      echo "> Opened by [Release candidate PR]($(repo_url)/actions/workflows/release_candidate_pr.yml): it rebuilds \`$branch\` from $BASE every day except Sunday and force-pushes it, so commits pushed here are lost, edits to the notes included. Merge it, or edit it, the day it opens. A release that needs commits of its own (sidecar pins, say): run the workflow with a new \`branch\`, and push them there."
    fi
  fi
  echo
  # TODO: drop once the workflows use a GitHub App's token.
  echo "> [!IMPORTANT]"
  echo "> CI does not start on its own on this pull request: approve its workflows (the button on the pull request) to run it."
} > "$tmp/body"

if $dry_run; then
  printf '\n(dry run) pull request from %s: %s\n\n' "$branch" "$title"
  cat "$tmp/body"
  exit 0
fi
# A new pull request rather than an edit of the open one: GitHub notifies no one of an edit, and a
# new pull request tells its readers the release changed.
old="$(open_pr "$branch")"
if [ -n "$old" ]; then
  gh pr close "$old" # its branch is the new one's, so it stays
fi
url="$(gh pr create --base "$BASE" --head "$branch" --title "$title" --body-file "$tmp/body")"
echo "Opened $url"
if [ -n "$old" ]; then
  gh pr comment "$old" --body "Replaced by $url, for $BASE as it is now."
  echo "Closed #$old"
fi
