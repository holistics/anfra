#!/usr/bin/env bash
# The core API's contract, generated from the command registry. Run `generate`
# after changing anything a command publishes: a command added, removed or
# renamed, its input or output types, its declared errors, or a code in the
# apperr catalog. Commit what it writes with the change; CI runs `diff` and fails
# on a stale file, and `breaking` against the base branch.
#
# It also writes api/remote.yaml, the publish protocol's contract (shared/remoteapi):
# what a server that `anfra publish` publishes to serves.
#
#   scripts/openapi.sh generate                 write both (`anfra openapi [--remote]`: no repo or sidecars)
#   scripts/openapi.sh diff                     fail if either committed document is stale
#   scripts/openapi.sh breaking <base>          fail if api/openapi.yaml breaks the base document
#   scripts/openapi.sh breaking-remote <base>   fail if api/remote.yaml breaks the base document
#
# A breaking change reaches every client of the core API — Data Apps on either
# host, the core SDK — so it is refused unless it is declared: CI accepts one in
# a pull request whose commits mark it (`feat!:` or a `BREAKING CHANGE:` footer).
# The publish protocol has the same rule: its clients (the CLI, scripts, agents)
# cannot be reloaded, so a break is avoided, and only ever declared.
set -euo pipefail
cd "$(dirname "${BASH_SOURCE[0]}")/.."

spec=api/openapi.yaml
remote=api/remote.yaml
oasdiff=github.com/oasdiff/oasdiff@v1.33.0

case "${1:-}" in
generate)
  mkdir -p api
  go run ./cmd/anfra openapi > "$spec"
  go run ./cmd/anfra openapi --remote > "$remote"
  ;;
diff)
  tmp=$(mktemp -d)
  trap 'rm -rf "$tmp"' EXIT
  go run ./cmd/anfra openapi > "$tmp/openapi.yaml"
  go run ./cmd/anfra openapi --remote > "$tmp/remote.yaml"
  stale=0
  diff -u "$spec" "$tmp/openapi.yaml" || stale=1
  diff -u "$remote" "$tmp/remote.yaml" || stale=1
  if [ "$stale" = 1 ]; then
    echo "a contract is stale: run scripts/openapi.sh generate, and commit it with the change" >&2
    exit 1
  fi
  ;;
breaking)
  base=${2:?usage: scripts/openapi.sh breaking <base spec>}
  go run "$oasdiff" breaking --fail-on ERR "$base" "$spec"
  ;;
breaking-remote)
  base=${2:?usage: scripts/openapi.sh breaking-remote <base spec>}
  go run "$oasdiff" breaking --fail-on ERR "$base" "$remote"
  ;;
*)
  echo "usage: scripts/openapi.sh generate|diff|breaking <base spec>|breaking-remote <base spec>" >&2
  exit 2
  ;;
esac
