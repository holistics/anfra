#!/usr/bin/env bash
# anfra-cloud's contract, vendored for the CLI's client of an anfra remote server
# (internal/cloudapi). anfra-cloud owns it; this copies its committed
# api/openapi.yaml here, and internal/cloudapi's tests hold the client's types to
# it. Synced by hand for now: run `sync` after anfra-cloud changes an op the CLI
# calls, then run the tests.
#
#   scripts/cloudapi.sh sync [<anfra-cloud checkout>]   default: ../anfra-cloud
set -euo pipefail
cd "$(dirname "${BASH_SOURCE[0]}")/.."

case "${1:-}" in
sync)
  src=${2:-../anfra-cloud}/api/openapi.yaml
  [ -f "$src" ] || { echo "no $src: give the path to an anfra-cloud checkout" >&2; exit 1; }
  cp "$src" internal/cloudapi/openapi.yaml
  go test ./internal/cloudapi/
  ;;
*)
  echo "usage: scripts/cloudapi.sh sync [<anfra-cloud checkout>]" >&2
  exit 2
  ;;
esac
