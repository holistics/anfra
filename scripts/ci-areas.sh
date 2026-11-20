#!/usr/bin/env bash
# The areas of the repo a change touches, each the files one of CI's jobs checks
# (.github/workflows/continuous_integration.yml): CI runs a job only when its area
# changed. Prints one `<area>=true|false` line per area, for $GITHUB_OUTPUT.
#
#   scripts/ci-areas.sh <base> [head]   the change from base to head; without head, to the
#                                       working tree, uncommitted and untracked files included
#
# An area is the files its job reads, not only the ones it is named for: a Go test
# reads the testdata beside it, the SDK's types come from api/openapi.yaml. A file
# no area lists (docs/, the changelogs, the READMEs at the root) runs no job but
# commitlint. When a job starts reading a file outside its area, add the file here.
#
# A package.json whose only change is its `version` (a release's bump) counts for
# the manifest area alone, which checks the versions agree; nothing else reads it.
#
# A change to CI itself (the workflow, or this script) touches every area.
set -euo pipefail
cd "$(dirname "${BASH_SOURCE[0]}")/.."

base="${1:?usage: scripts/ci-areas.sh <base> [head]}"
head="${2:-}" # empty: the working tree

# Whether a package.json changed only in its `version`.
version_only() {
  local before after
  before="$(git show "$base:$1" 2>/dev/null | jq -S 'del(.version)')" || return 1
  if [ -n "$head" ]; then
    after="$(git show "$head:$1" 2>/dev/null | jq -S 'del(.version)')" || return 1
  else
    after="$(jq -S 'del(.version)' "$1" 2>/dev/null)" || return 1
  fi
  [ -n "$before" ] && [ "$before" = "$after" ]
}

# Each area's files, as case patterns: `*` matches across folders.
in_area() {
  local area="$1" file="$2"
  case "$area" in
  ci)
    case "$file" in
    .github/workflows/continuous_integration.yml | scripts/ci-areas.sh) return 0 ;;
    esac
    ;;
  lint) # golangci-lint, at the version tools/go.mod pins
    case "$file" in
    *.go | go.mod | go.sum | tools/go.mod | tools/go.sum | .golangci.yml) return 0 ;;
    esac
    ;;
  go) # tidy, the generated SQL code, vet, build, the Go tests
    case "$file" in
    # The Go folders whole: testdata, embedded files, queries.sql and migrations,
    # internal/cloudapi's openapi.yaml.
    cmd/* | internal/* | engine/* | shared/*) return 0 ;;
    scripts/*.go | go.mod | go.sum | tools/go.mod | tools/go.sum | scripts/sqlc.sh) return 0 ;;
    esac
    ;;
  api) # the contract is fresh, and a breaking change is declared
    case "$file" in
    *.go | go.mod | go.sum | api/* | scripts/openapi.sh) return 0 ;;
    esac
    ;;
  web) # the SDK, the design package and the frontend: typecheck, build, tests
    case "$file" in
    # The SDK's types come from the contract; a Go change to the API changes it too.
    web/* | package.json | pnpm-lock.yaml | pnpm-workspace.yaml | .nvmrc | api/openapi.yaml) return 0 ;;
    esac
    ;;
  skills) # pnpm skills:check
    case "$file" in
    skills/CHANGELOG.md) ;;
    skills/* | package.json | pnpm-lock.yaml | .nvmrc) return 0 ;;
    esac
    ;;
  manifest) # the versions agree, a release PR is the bot's, the cloud servers parse
    case "$file" in
    manifest.yml | web/*/package.json | scripts/check-release-pr.sh | scripts/bump.sh | scripts/cloud-url/*) return 0 ;;
    esac
    ;;
  dependencies) # GitHub's dependency review: Go, pnpm and the Actions the workflows use
    case "$file" in
    go.mod | go.sum | tools/go.mod | tools/go.sum | *package.json | pnpm-lock.yaml) return 0 ;;
    .github/workflows/* | .github/dependency-review-config.yml) return 0 ;;
    esac
    ;;
  esac
  return 1
}

areas=(lint go api web skills manifest dependencies)
declare -A touched=()

while IFS= read -r file; do
  if in_area ci "$file"; then
    for area in "${areas[@]}"; do touched[$area]=true; done
    break
  fi
  if [[ "$file" == *package.json ]] && version_only "$file"; then
    in_area manifest "$file" && touched[manifest]=true
    continue
  fi
  for area in "${areas[@]}"; do
    in_area "$area" "$file" && touched[$area]=true
  done
done < <(
  if [ -n "$head" ]; then
    git diff --name-only --no-renames "$base" "$head"
  else
    git diff --name-only --no-renames "$base"
    git ls-files --others --exclude-standard
  fi
)

for area in "${areas[@]}"; do
  echo "$area=${touched[$area]:-false}"
done
