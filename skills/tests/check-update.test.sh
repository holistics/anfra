#!/usr/bin/env bash
# The SessionStart update notice (plugins/anfra/hooks/check-update.sh), against a fake
# `anfra` that prints a given `anfra version` answer, and another for
# `anfra version --skills`, which an older fake refuses as an unknown flag. The
# source's skills carry no stamp, so its hook takes the plain path; the facade's
# build, stamped, takes --skills.

set -uo pipefail

skills="$(cd "$(dirname "$0")/.." && pwd)"
hook="$skills/plugins/anfra/hooks/check-update.sh"
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

fake() { # the fake anfra's `version` answer and exit status; then its `version --skills` answer, if it knows --skills
  printf '%s' "$1" > "$tmp/plain"
  rm -f "$tmp/skills" "$tmp/ran" "$tmp/args"
  [ $# -ge 3 ] && printf '%s' "$3" > "$tmp/skills"
  cat > "$tmp/bin/anfra" <<EOS
#!/bin/sh
touch "$tmp/ran"
echo "\$*" >> "$tmp/args"
if [ "\$2" = --skills ]; then
  [ -f "$tmp/skills" ] || { echo "unknown flag: --skills" >&2; exit 1; }
  cat "$tmp/skills"; exit 0
fi
cat "$tmp/plain"
exit ${2:-0}
EOS
  chmod +x "$tmp/bin/anfra"
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

# The facade's build, whose skills are stamped: --skills, and its hints.
node "$skills/scripts/build.mjs" --version 0.15.0 --source test --out "$tmp/facade" >/dev/null
hook="$tmp/facade/plugins/anfra/hooks/check-update.sh"

behind=$'hints:\n    - \'anfra skills 0.15.0 are behind the latest, 0.16.0: run `anfra agent setup --agent <your id>`.\'\nskills:\n    latest: 0.16.0\n    version: 0.15.0\nversion: 0.15.0\n'
fake "$newer" 0 "$behind"
out=$(run "$tmp/repo")
check "the plugin's stamp is passed" "version --skills 0.15.0" "$(cat "$tmp/args")"
check "the skills behind: their hint" 'anfra says: anfra skills 0.15.0 are behind the latest, 0.16.0: run `anfra agent setup --agent <your id>`.' "$out"
check "and what to do with it" "Don't run these commands unless the user asks" "$out"
check "valid JSON" '' "$(printf '%s' "$out" | python3 -c 'import json,sys; json.load(sys.stdin)' 2>&1)"
check "in Claude Code, the hint names it" '--agent claude-code' "$(run "$tmp/repo" CLAUDE_PLUGIN_ROOT=/x)"

both=$'hints:\n    - \'anfra 0.16.0 is out: ask the user to run `anfra update`.\'\n    - \'anfra skills 0.15.0 are behind the latest, 0.16.0: run `anfra agent setup --agent <your id>`.\'\nskills:\n    latest: 0.16.0\n    version: 0.15.0\nupdate:\n    available: true\n    latest: 0.16.0\nversion: 0.15.0\n'
fake "$newer" 0 "$both"
out=$(run "$tmp/repo")
check "both behind: anfra's hint" 'anfra 0.16.0 is out: ask the user to run `anfra update`.' "$out"
check "both behind: the skills' hint" 'anfra skills 0.15.0 are behind' "$out"

fake "$newer" 0 $'skills:\n    latest: 0.15.0\n    version: 0.15.0\nversion: 0.15.0\n'
check "nothing to act on: silent" '' "$(run "$tmp/repo")"

fake "$newer" 0 $'hints:\n    - it\'\'s quoted\nversion: 0.15.0\n'
check "a plain hint is passed as it is" "anfra says: it''s quoted" "$(run "$tmp/repo")"

fake "$newer"
check "an anfra that refuses --skills: its update notice" 'anfra 0.5.0 is available (installed: 0.4.2)' "$(run "$tmp/repo")"

exit "$failed"
