#!/usr/bin/env bash
# Generates the Go code of anfra's SQL queries with sqlc, at the version
# tools/go.mod pins. Run `generate` after changing a store's queries.sql or
# schema.sql, and commit what it writes; CI runs `diff` and fails on stale code.
#
#   scripts/sqlc.sh generate   write each config's gen/
#   scripts/sqlc.sh diff       fail if any committed gen/ is stale
set -euo pipefail
cd "$(dirname "${BASH_SOURCE[0]}")/.."

configs=(
  internal/querycache/store/sqlite/sqlc.yaml
)
for config in "${configs[@]}"; do
  go tool -modfile=tools/go.mod sqlc -f "$config" "$@"
done
