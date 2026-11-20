#!/usr/bin/env bash
# The areas of the repo, each the files one of CI's jobs reads (docs/designs/ci.md).
#
#   scripts/ci-areas.sh hashes [commit]          each area's hash at commit (default HEAD):
#                                                `<area>_hash=<hash>`, for the jobs that check files
#   scripts/ci-areas.sh changed <base> [head]    which areas changed from base to head; without
#                                                head, to the working tree, uncommitted and untracked
#                                                files included: `<area>=true|false`
#
# Two kinds of area:
#
# - A file area is what a job's result depends on alone: lint, go, api, web, skills, manifest.
#   CI skips the job when the area's hash already passed (an artifact, passed-<area>-<hash>).
# - A base area is what a check judges against the branch the change lands on: api-breaking,
#   release, dependencies. CI runs it on pull requests, when the area changed from the PR's base.
#
# An area is the files its job reads, not only the ones it is named for: a Go test reads the
# testdata beside it, the SDK's types come from api/openapi.yaml. When a job starts reading a file
# outside its area, add the file here. Three rules make a mistake cost time rather than a check:
#
# - CI's own files (the workflow, its actions, the Makefile, this script) are in every area.
# - A file in no area and not one no job reads (no_job_reads) is in every area.
# - A package.json whose only change is its `version` (a release's bump) counts for manifest
#   alone: elsewhere its hash is taken without the version.
set -euo pipefail
cd "$(dirname "${BASH_SOURCE[0]}")/.."

file_areas=(lint go api web skills manifest)
base_areas=(api-breaking release dependencies)

# CI's own files: how every job runs.
is_ci() {
  case "$1" in
  .github/workflows/continuous_integration.yml | .github/actions/* | Makefile | scripts/ci-areas.sh) return 0 ;;
  esac
  return 1
}

# Files no CI job reads: docs, dev tooling, release-only scripts. A file in no area that isn't
# listed here runs every job.
no_job_reads() {
  case "$1" in
  *.md | docs/* | LICENSE | NOTICE | .gitignore | .husky/* | .github/CODEOWNERS) return 0 ;;
  Procfile.dev | .air.toml | .env.local.example | docker-compose.dev.yml | dev/*) return 0 ;;
  Dockerfile | install.sh | commitlint.config.mjs) return 0 ;; # the release build; commitlint always runs
  .github/workflows/* | .github/dependency-review-config.yml) return 0 ;; # other workflows; dependencies reads these
  scripts/release-pr.sh | scripts/release-notes.sh | scripts/dev-serve.sh | scripts/cloudapi.sh | scripts/sidecars.sh) return 0 ;;
  esac
  return 1
}

# Whether a file is in an area, by its own patterns: `*` matches across folders.
in_area() {
  local area="$1" file="$2"
  case "$area" in
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
  api) # the contract is fresh: what the commands publish
    case "$file" in
    *.go | go.mod | go.sum | api/* | scripts/openapi.sh) return 0 ;;
    esac
    ;;
  web) # the SDK, the design package and the frontend: typecheck, build, tests
    case "$file" in
    # The SDK's types come from the contract.
    web/* | package.json | pnpm-lock.yaml | pnpm-workspace.yaml | .nvmrc | api/openapi.yaml) return 0 ;;
    esac
    ;;
  skills) # pnpm skills:check
    case "$file" in
    skills/CHANGELOG.md) ;;
    skills/* | package.json | pnpm-lock.yaml | pnpm-workspace.yaml | .nvmrc) return 0 ;;
    esac
    ;;
  manifest) # the versions agree, the cloud servers parse
    case "$file" in
    manifest.yml | web/*/package.json | scripts/cloud-url/*) return 0 ;;
    esac
    ;;
  api-breaking) # a breaking change to the contract, against the base's, is declared by a commit
    case "$file" in
    api/* | scripts/openapi.sh) return 0 ;;
    esac
    ;;
  release) # a release PR is the bot's (scripts/check-release-pr.sh, which runs scripts/bump.sh)
    case "$file" in
    manifest.yml | web/*/package.json | scripts/check-release-pr.sh | scripts/bump.sh) return 0 ;;
    scripts/changelog-merge.mjs | scripts/next-version.mjs | scripts/lib/* | conventional-changelog.config.mjs) return 0 ;;
    esac
    ;;
  dependencies) # GitHub's dependency review: Go, pnpm and the Actions the workflows use
    case "$file" in
    go.mod | go.sum | tools/go.mod | tools/go.sum | *package.json | pnpm-lock.yaml) return 0 ;;
    .github/workflows/* | .github/actions/* | .github/dependency-review-config.yml) return 0 ;;
    esac
    ;;
  esac
  return 1
}

# Whether a file counts for an area, the rules above included.
counts_for() {
  local area="$1" file="$2" other
  is_ci "$file" && return 0
  in_area "$area" "$file" && return 0
  no_job_reads "$file" && return 1
  for other in "${file_areas[@]}" "${base_areas[@]}"; do
    in_area "$other" "$file" && return 1
  done
  return 0 # in no area: every area
}

# Each file area's hash at a commit: the paths and content hashes of the files that count for it.
hashes() {
  local commit="$1" area mode type blob path
  declare -A lines=()
  while read -r mode type blob path; do
    for area in "${file_areas[@]}"; do
      counts_for "$area" "$path" || continue
      if [[ "$path" == *package.json && "$area" != manifest ]]; then
        # Without its version: a release's bump changes no area but manifest.
        blob="$(git show "$commit:$path" | jq -S 'del(.version)' | git hash-object --stdin)"
      fi
      lines[$area]+="$blob $path"$'\n'
    done
  done < <(git ls-tree -r "$commit")
  for area in "${file_areas[@]}"; do
    echo "${area}_hash=$(printf '%s' "${lines[$area]:-}" | sort | sha256sum | cut -c1-32)"
  done
}

# Whether a package.json changed only in its `version`, from base to head (or the working tree).
version_only() {
  local base="$1" head="$2" file="$3" before after
  before="$(git show "$base:$file" 2>/dev/null | jq -S 'del(.version)')" || return 1
  if [ -n "$head" ]; then
    after="$(git show "$head:$file" 2>/dev/null | jq -S 'del(.version)')" || return 1
  else
    after="$(jq -S 'del(.version)' "$file" 2>/dev/null)" || return 1
  fi
  [ -n "$before" ] && [ "$before" = "$after" ]
}

# Which areas changed from base to head (or the working tree).
changed() {
  local base="$1" head="$2" file area
  declare -A touched=()
  while IFS= read -r file; do
    for area in "${file_areas[@]}" "${base_areas[@]}"; do
      [ -n "${touched[$area]:-}" ] && continue
      counts_for "$area" "$file" || continue
      if [[ "$file" == *package.json && "$area" != manifest && "$area" != release ]] && version_only "$base" "$head" "$file"; then
        continue
      fi
      touched[$area]=true
    done
  done < <(
    if [ -n "$head" ]; then
      git diff --name-only --no-renames "$base" "$head"
    else
      git diff --name-only --no-renames "$base"
      git ls-files --others --exclude-standard
    fi
  )
  for area in "${file_areas[@]}" "${base_areas[@]}"; do
    echo "$area=${touched[$area]:-false}"
  done
}

case "${1:-}" in
hashes) hashes "${2:-HEAD}" ;;
changed) changed "${2:?usage: scripts/ci-areas.sh changed <base> [head]}" "${3:-}" ;;
*)
  echo "usage: scripts/ci-areas.sh hashes [commit] | changed <base> [head]" >&2
  exit 2
  ;;
esac
