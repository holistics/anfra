#!/usr/bin/env bash
# The core API's contract, generated from the command registry. Run `generate`
# after changing anything a command publishes: a command added, removed or
# renamed, its input or output types, its declared errors, or a code in the
# apperr catalog. Commit what it writes with the change; CI runs `diff` and fails
# on a stale file, and `breaking` against the base branch.
#
#   scripts/openapi.sh generate          write api/openapi.yaml (`anfra openapi`: no repo or sidecars)
#   scripts/openapi.sh diff              fail if the committed api/openapi.yaml is stale
#   scripts/openapi.sh breaking <base>   fail if api/openapi.yaml breaks the base document
#
# A breaking change reaches every client of the core API — Data Apps on either
# host, the core SDK — so it is refused unless it is declared: CI accepts one in
# a pull request whose commits mark it (`feat!:` or a `BREAKING CHANGE:` footer).
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
  tmp=$(mktemp)
  trap 'rm -f "$tmp"' EXIT
  go run ./cmd/anfra openapi > "$tmp"
  if ! diff -u "$spec" "$tmp"; then
    echo "the core API's contract is stale: run scripts/openapi.sh generate, and commit it with the change" >&2
    exit 1
  fi
  ;;
breaking)
  base=${2:?usage: scripts/openapi.sh breaking <base spec>}
  go run "$oasdiff" breaking --fail-on ERR "$base" "$spec"
  ;;
*)
  echo "usage: scripts/openapi.sh generate|diff|breaking <base spec>" >&2
  exit 2
  ;;
esac
