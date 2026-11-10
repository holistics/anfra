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
# anfra-canal runs canal's services (anfra-canal query is canal-query), which anfra supervises as
# sidecars of their own: check also holds the releases its --version says it runs to
# sidecars.anfra_canal_services. A service ANFRA_<SERVICE>_URL names is already running, and one
# ANFRA_<SERVICE>_BIN names is your own standalone build, which anfra runs in place of
# anfra-canal's, held to the release sidecars.anfra_canal_services pins for it. anfra-canal is
# left alone when anfra runs none of its services from it.
#
# Downloads with gh, from holistics-core's and canal's releases, which are private: `gh auth
# login` as an account that can read both.
set -euo pipefail
cd "$(dirname "${BASH_SOURCE[0]}")/.."

dir="$PWD/.sidecars"

# name, manifest key, its environment variables' infix, the services it runs by theirs
# (comma-separated; its own for a sidecar that is its one service), GitHub repo, the suffix of the
# release asset to prefer (- for none)
sidecars=(
  "anfra-node anfra_node NODE NODE holistics/holistics-core -unminified"
  "anfra-canal anfra_canal CANAL CANAL_QUERY holistics/canal -"
)

usage() { sed -n '2,/^set /{/^set /d;s/^# \{0,1\}//;p;}' "$0" >&2; exit 2; }

# The release tag manifest.yml pins for key, e.g. anfra-node-v0.5.0, or for a service of
# anfra_canal's under anfra_canal_services, e.g. canal_query.
pin() {
  local tag
  tag=$(sed -n "s/^ *$1: *\([^ #]*\).*/\1/p" manifest.yml)
  [ -n "$tag" ] || { echo "manifest.yml pins no ${2:-sidecars}.$1" >&2; exit 1; }
  echo "$tag"
}

# What a binary prints for --version, or nothing. A release from before --version ignores the
# flag, and canal-query then starts a server, so it gets a few seconds and is killed.
version_output() {
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
  cat "$out"
  rm -f "$out"
}

# The version in version_output's first line, or nothing.
version_in() { head -n 1 <<<"$1" | grep -E '^[0-9]+\.[0-9]+\.[0-9]+' || true; }

# A service's name from its variables' infix: CANAL_QUERY is canal-query.
service_name() { tr '[:upper:]_' '[:lower:]-' <<<"$1"; }

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
  read -r name key env services repo variant <<<"$s"
  bin_var="ANFRA_${env}_BIN"
  bin=${!bin_var:-}

  # Whether anfra runs any of its services from it here, and not from one already running or
  # from your own standalone build.
  used=
  for svc in ${services//,/ }; do
    svc_name=$name
    [ "$svc" = "$env" ] || svc_name=$(service_name "$svc")
    url_var="ANFRA_${svc}_URL" svc_bin_var="ANFRA_${svc}_BIN"
    if [ -n "${!url_var:-}" ]; then
      echo "$svc_name: $url_var is set, so anfra dials it: left alone"
    elif [ "$svc" != "$env" ] && [ -n "${!svc_bin_var:-}" ]; then
      # Your own standalone build of the service, held to the release sidecars.<key>_services pins.
      svc_bin=${!svc_bin_var}
      if [ "$mode" = fetch ]; then
        echo "$svc_name: $svc_bin_var names your own, $svc_bin, which anfra runs instead of $name: left alone"
      elif [ ! -e "$svc_bin" ]; then
        failed+=("$svc_name: $svc_bin_var names $svc_bin, which does not exist")
      elif [ "${ANFRA_SIDECAR_SKIP_VERSION_CHECK:-}" != 1 ]; then
        svc_tag=$(pin "$(tr '[:upper:]' '[:lower:]' <<<"$svc")" "sidecars.${key}_services")
        have=$(version_in "$(version_output "$svc_bin")")
        [ "$have" = "${svc_tag##*-v}" ] ||
          failed+=("$svc_name: $svc_bin is ${have:-a version from before --version}, not ${svc_tag##*-v}, which manifest.yml's sidecars.${key}_services pins. Rebuild it, or unset $svc_bin_var for $name's")
      fi
    else
      used=1
    fi
  done
  if [ -z "$used" ]; then
    [ "$services" = "$env" ] || echo "$name: anfra runs none of its services from it: left alone"
    continue
  fi

  # A relative path is the repo root's (where make runs this), made absolute so that one naming
  # .sidecars/<name> is not taken for your own. Its folder may not exist yet: left as it is.
  case $bin in
  '' | /*) ;;
  *) if bin_dir=$(cd "$(dirname "$bin")" 2>/dev/null && pwd); then bin=$bin_dir/$(basename "$bin"); fi ;;
  esac
  own=
  [ -z "$bin" ] || [ "$bin" = "$dir/$name" ] || own=1
  bin=${bin:-$dir/$name}
  tag=$(pin "$key")
  want=${tag##*-v}

  if [ "$mode" = fetch ]; then
    if [ -n "$own" ]; then echo "$name: $bin_var names your own, $bin: left alone"; continue; fi
    if [ -x "$bin" ] && [ "$(version_in "$(version_output "$bin")")" = "$want" ]; then echo "$name: $want, as pinned"; continue; fi
    download "$name" "$repo" "$tag" "$variant"
    continue
  fi

  if [ ! -e "$bin" ]; then
    [ -z "$own" ] || { failed+=("$name: $bin_var names $bin, which does not exist"); continue; }
    download "$name" "$repo" "$tag" "$variant"
  fi
  [ "${ANFRA_SIDECAR_SKIP_VERSION_CHECK:-}" != 1 ] || continue
  shown=.sidecars/$name
  [ -z "$own" ] || shown=$bin
  out=$(version_output "$bin")
  have=$(version_in "$out")
  if [ "$have" != "$want" ]; then
    if [ -n "$own" ]; then
      failed+=("$name: $bin is ${have:-a version from before --version}, not $want. Rebuild it, or unset $bin_var for the pinned release")
    else
      failed+=("$name: .sidecars/$name is ${have:-a version from before --version}, not $want. Run \`make sidecars\` to replace it")
    fi
    continue
  fi

  # The releases of its services it says it runs (e.g. "canal-query 2.25.0"), against
  # sidecars.<key>_services.
  [ "$services" != "$env" ] || continue
  for svc in ${services//,/ }; do
    svc_name=$(service_name "$svc")
    svc_key=$(tr '[:upper:]' '[:lower:]' <<<"$svc")
    svc_tag=$(pin "$svc_key" "sidecars.${key}_services")
    runs=$(sed -n "s/^$svc_name \([^ ]*\)$/\1/p" <<<"$out")
    [ "$runs" != "${svc_tag##*-v}" ] || continue
    if [ -n "$runs" ]; then
      failed+=("$name: $shown $want runs $svc_name $runs, but manifest.yml's sidecars.${key}_services pins $svc_tag. Set it to ${svc_tag%-v*}-v$runs")
    else
      failed+=("$name: $shown $want doesn't say which $svc_name it runs, which manifest.yml's sidecars.${key}_services pins to $svc_tag. Use a build of $name that says it in --version")
    fi
  done
done

if [ ${#failed[@]} -gt 0 ]; then
  echo "The sidecars are not the ones manifest.yml pins:" >&2
  printf '  %s\n' "${failed[@]}" >&2
  echo "ANFRA_SIDECAR_SKIP_VERSION_CHECK=1 runs them anyway." >&2
  exit 1
fi
