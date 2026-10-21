#!/usr/bin/env bash
# The sidecars an anfra built from source runs with, checked against the releases manifest.yml
# pins. Unless you bring your own, they are those releases, downloaded for this machine into
# .sidecars/ (git-ignored): one copy each, replaced when a pin moves. anfra-node's is its unminified
# build, whose spans carry the source's function names; a release from before it had one gives
# the minified build.
#
#   scripts/sidecars.sh fetch   download the pinned releases into .sidecars/, replacing others
#   scripts/sidecars.sh check   download what .sidecars/ lacks; fail if a sidecar in use is not
#                               the pinned version (make dev and make test run it)
#
# Your own sidecar is one ANFRA_<NAME>_BIN names, other than .sidecars/<name>: check holds it to
# the pin too, and fetch leaves it alone. One ANFRA_<NAME>_URL names is already running, so
# neither touches it. ANFRA_SIDECAR_SKIP_VERSION_CHECK=1 skips the version check, to run a build
# whose version is not the pin on purpose.
#
# Downloads with gh, from holistics-core's and canal's releases, which are private: `gh auth
# login` as an account that can read both.
set -euo pipefail
cd "$(dirname "${BASH_SOURCE[0]}")/.."

dir="$PWD/.sidecars"

# name, manifest key, GitHub repo, environment variables' infix, the suffix of the release asset
# to prefer (- for none)
sidecars=(
  "anfra-node anfra_node holistics/holistics-core NODE -unminified"
  "canal-query canal_query holistics/canal CANAL_QUERY -"
)

usage() { sed -n '2,/^set /{/^set /d;s/^# \{0,1\}//;p;}' "$0" >&2; exit 2; }

# The release tag manifest.yml pins for key, e.g. anfra-node-v0.5.0.
pin() {
  local tag
  tag=$(sed -n "s/^ *$1: *\([^ #]*\).*/\1/p" manifest.yml)
  [ -n "$tag" ] || { echo "manifest.yml pins no sidecars.$1" >&2; exit 1; }
  echo "$tag"
}

# What a binary says its version is, or nothing. A release from before --version ignores the
# flag, and canal-query then starts a server, so it gets a few seconds and is killed.
version_of() {
  local out pid i
  out=$(mktemp)
  "$1" --version >"$out" 2>/dev/null </dev/null &
  pid=$!
  for i in $(seq 50); do
    kill -0 "$pid" 2>/dev/null || break
    sleep 0.1
  done
  kill "$pid" 2>/dev/null || true
  wait "$pid" 2>/dev/null || true
  head -n 1 "$out" | grep -E '^[0-9]+\.[0-9]+\.[0-9]+' || true
  rm -f "$out"
}

target() {
  local os arch
  case "$(uname -s)" in
  Linux) os=linux ;;
  Darwin) os=darwin ;;
  *) echo "no sidecar releases for $(uname -s)" >&2; exit 1 ;;
  esac
  case "$(uname -m)" in
  x86_64 | amd64) arch=x64 ;;
  arm64 | aarch64) arch=arm64 ;;
  *) echo "no sidecar releases for $(uname -m)" >&2; exit 1 ;;
  esac
  echo "$os-$arch"
}

download() {
  local name=$1 repo=$2 tag=$3 variant=$4 asset tmp
  command -v gh >/dev/null || { echo "downloading $name needs gh (https://cli.github.com), logged in as an account that can read $repo" >&2; exit 1; }
  asset="$name-$(target)"
  [ "$variant" = - ] || asset+=$variant
  echo "Downloading $asset from $repo's release $tag ..."
  mkdir -p "$dir"
  tmp=$(mktemp -d "$dir/.download.XXXXXX")
  if [ "$variant" != - ] && ! gh release download -R "$repo" "$tag" -p "$asset" -D "$tmp" 2>/dev/null; then
    asset="$name-$(target)"
    echo "$tag has none: downloading $asset instead ..."
  fi
  if [ ! -e "$tmp/$asset" ] && ! gh release download -R "$repo" "$tag" -p "$asset" -D "$tmp"; then
    rm -rf "$tmp"
    echo "could not download $asset from $repo's release $tag: can the account gh is logged in as read $repo?" >&2
    exit 1
  fi
  chmod +x "$tmp/$asset"
  if [ "$(uname -s)" = Darwin ]; then xattr -d com.apple.quarantine "$tmp/$asset" 2>/dev/null || true; fi
  mv -f "$tmp/$asset" "$dir/$name"
  rm -rf "$tmp"
}

mode=${1:-}
case "$mode" in fetch | check) ;; *) usage ;; esac

failed=()
for s in "${sidecars[@]}"; do
  read -r name key repo env variant <<<"$s"
  bin_var="ANFRA_${env}_BIN" url_var="ANFRA_${env}_URL"
  bin=${!bin_var:-} url=${!url_var:-}
  [ -z "$url" ] || { echo "$name: $url_var is set, so anfra dials it: left alone"; continue; }
  own=
  [ -z "$bin" ] || [ "$bin" = "$dir/$name" ] || own=1
  bin=${bin:-$dir/$name}
  tag=$(pin "$key")
  want=${tag##*-v}

  if [ "$mode" = fetch ]; then
    if [ -n "$own" ]; then echo "$name: $bin_var names your own, $bin: left alone"; continue; fi
    if [ -x "$bin" ] && [ "$(version_of "$bin")" = "$want" ]; then echo "$name: $want, as pinned"; continue; fi
    download "$name" "$repo" "$tag" "$variant"
    continue
  fi

  if [ ! -e "$bin" ]; then
    [ -z "$own" ] || { failed+=("$name: $bin_var names $bin, which does not exist"); continue; }
    download "$name" "$repo" "$tag" "$variant"
  fi
  [ "${ANFRA_SIDECAR_SKIP_VERSION_CHECK:-}" != 1 ] || continue
  have=$(version_of "$bin")
  [ "$have" != "$want" ] || continue
  if [ -n "$own" ]; then
    failed+=("$name: $bin is ${have:-a version from before --version}, not $want. Rebuild it, or unset $bin_var for the pinned release")
  else
    failed+=("$name: .sidecars/$name is ${have:-a version from before --version}, not $want. Run \`make sidecars\` to replace it")
  fi
done

if [ ${#failed[@]} -gt 0 ]; then
  echo "The sidecars are not the ones manifest.yml pins:" >&2
  printf '  %s\n' "${failed[@]}" >&2
  echo "ANFRA_SIDECAR_SKIP_VERSION_CHECK=1 runs them anyway." >&2
  exit 1
fi
