#!/usr/bin/env bash
# The core API's contract, generated from the command registry. Run `generate`
# after changing anything a command publishes: a command added, removed or
# renamed, its input or output types, its declared errors, or a code in the
# apperr catalog. Commit what it writes with the change; CI runs `diff` and fails
# on a stale file, and `breaking` against the base branch.
#
#   scripts/openapi.sh generate          write it (`anfra openapi`: no repo or sidecars)
#   scripts/openapi.sh diff              fail if the committed document is stale
#   scripts/openapi.sh breaking <base>   fail if api/openapi.yaml breaks the base document
#   scripts/openapi.sh declared <commit> fail if it breaks <commit>'s, and no commit since declares it
#
# A breaking change reaches every client of the core API — Data Apps on either
# host, the core SDK — so it is refused unless it is declared: CI accepts one in
# a pull request whose commits mark it (`feat!:` or a `BREAKING CHANGE:` footer).
#
# api/oasdiff-severity-levels.txt lowers oasdiff's checks that don't break a
# client to warnings: printed, not failed. Adding an error code to an op is one
# (response-property-one-of-added), since a client already has to handle a code
# it doesn't know: anything unclassified is internal_server_error.
set -euo pipefail
cd "$(dirname "${BASH_SOURCE[0]}")/.."

spec=api/openapi.yaml
oasdiff=github.com/oasdiff/oasdiff@v1.33.0

case "${1:-}" in
generate)
  mkdir -p api
  go run ./cmd/anfra openapi > "$spec"
  ;;
diff)
  tmp=$(mktemp -d)
  trap 'rm -rf "$tmp"' EXIT
  go run ./cmd/anfra openapi > "$tmp/openapi.yaml"
  if ! diff -u "$spec" "$tmp/openapi.yaml"; then
    echo "the contract is stale: run scripts/openapi.sh generate, and commit it with the change" >&2
    exit 1
  fi
  ;;
breaking)
  base=${2:?usage: scripts/openapi.sh breaking <base spec>}
  go run "$oasdiff" breaking --fail-on ERR \
    --severity-levels api/oasdiff-severity-levels.txt "$base" "$spec"
  ;;
declared)
  # The branch the change lands on: on a PR, its base; in the merge queue, the queue's base
  # commit; locally (make ci-api), origin/main.
  base=${2:?usage: scripts/openapi.sh declared <base commit>}
  if ! git cat-file -e "$base:$spec" 2>/dev/null; then
    echo "$base has no $spec yet: nothing to compare"
    exit 0
  fi
  tmp=$(mktemp -d)
  trap 'rm -rf "$tmp"' EXIT
  git show "$base:$spec" > "$tmp/base.yaml"
  if "$0" breaking "$tmp/base.yaml"; then exit 0; fi
  if git log --format=%B "$base..HEAD" | grep -Eq '^[a-z]+(\([^)]*\))?!:|^BREAKING CHANGE:'; then
    echo "breaking, and declared by a commit: ok"
    exit 0
  fi
  echo "a breaking change to the core API, not declared: mark the commit (feat!: …) or a BREAKING CHANGE: footer" >&2
  exit 1
  ;;
*)
  echo "usage: scripts/openapi.sh generate|diff|breaking <base spec>|declared <base commit>" >&2
  exit 2
  ;;
esac
