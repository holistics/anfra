#!/usr/bin/env bash
# The SessionStart update notice (plugins/anfra-development/hooks/check-update.sh), against a fake
# `anfra` that prints a given `anfra version` answer.

set -uo pipefail

hook="$(cd "$(dirname "$0")/.." && pwd)/plugins/anfra-development/hooks/check-update.sh"
tmp=$(mktemp -d)
trap 'rm -rf "$tmp"' EXIT
mkdir -p "$tmp/bin" "$tmp/repo/.anfra" "$tmp/elsewhere"

failed=0
check() { # name, want ("" for silence, else a substring), got
  if { [ -z "$2" ] && [ -n "$3" ]; } || { [ -n "$2" ] && [[ "$3" != *"$2"* ]]; }; then
    echo "FAIL $1: got [$3]"
    failed=1
  else
    echo "ok   $1"
  fi
}

fake() { # the fake anfra's `version` answer, and its exit status
  printf '#!/bin/sh\ntouch "%s/ran"\nprintf "%%s" "%s"\nexit %s\n' "$tmp" "$1" "${2:-0}" > "$tmp/bin/anfra"
  chmod +x "$tmp/bin/anfra"
  rm -f "$tmp/ran"
}

run() { # the session's cwd, then any env assignments
  local cwd=$1; shift
  printf '{"session_id":"s","hook_event_name":"SessionStart","source":"startup","cwd":"%s"}' "$cwd" \
    | env PATH="$tmp/bin:/usr/bin:/bin" "$@" sh "$hook"
}

newer=$'update:\n    available: true\n    latest: 0.5.0\nversion: 0.4.2\n'

fake "$newer"
out=$(run "$tmp/repo")
check "a newer release is announced" 'anfra 0.5.0 is available (installed: 0.4.2)' "$out"
check "as SessionStart context" '"hookEventName":"SessionStart","additionalContext":"' "$out"
check "valid JSON" '' "$(printf '%s' "$out" | python3 -c 'import json,sys; json.load(sys.stdin)' 2>&1)"

fake $'update:\n    available: false\n    latest: 0.4.2\nversion: 0.4.2\n'
check "current: silent" '' "$(run "$tmp/repo")"

fake $'version: 0.4.1\n'
check "an anfra that doesn't report updates: silent" '' "$(run "$tmp/repo")"

fake "$newer" 1
check "anfra fails: silent" '' "$(run "$tmp/repo")"

fake "$newer"
check "opted out: silent" '' "$(run "$tmp/repo" ANFRA_NO_UPDATE_NOTIFIER=1)"

fake "$newer"
check "not in an anfra repo: silent" '' "$(run "$tmp/elsewhere")"
check "not in an anfra repo: anfra not run" '' "$( [ -e "$tmp/ran" ] && echo ran)"

rm -f "$tmp/bin/anfra"
check "anfra not installed: silent" '' "$(run "$tmp/repo")"

exit "$failed"
