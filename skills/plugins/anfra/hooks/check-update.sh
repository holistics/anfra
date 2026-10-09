#!/usr/bin/env sh
# SessionStart hook: tell the agent when a newer anfra release is out, so it
# can mention it to the user. Reads what `anfra version` reports from anfra's
# own cached daily check (no network here; anfra refreshes the cache in the
# background). Silent when anfra is current, not installed, too old to report
# updates, opted out (ANFRA_NO_UPDATE_NOTIFIER), or when the session is not in
# an anfra repo: `anfra version` sets up the directory it runs in as a repo.
# Always exits 0.

set -u

input=$(cat)
cwd=$(printf '%s' "$input" | sed -n 's/.*"cwd"[[:space:]]*:[[:space:]]*"\([^"]*\)".*/\1/p')
[ -n "$cwd" ] && cd "$cwd" 2>/dev/null

[ -d .anfra ] || exit 0
[ "${ANFRA_NO_UPDATE_NOTIFIER:-}" = 1 ] && exit 0
command -v anfra >/dev/null 2>&1 || exit 0

# YAML: version: 0.4.2, and when known, update: { available: true, latest: 0.5.0 }.
out=$(anfra version 2>/dev/null) || exit 0
printf '%s\n' "$out" | grep -Eq '^[[:space:]]+available:[[:space:]]*true' || exit 0
latest=$(printf '%s\n' "$out" | sed -n 's/^[[:space:]]*latest:[[:space:]]*//p' | head -n 1)
current=$(printf '%s\n' "$out" | sed -n 's/^version:[[:space:]]*//p' | head -n 1)
[ -n "$latest" ] || exit 0

msg="anfra $latest is available (installed: ${current:-unknown}). Mention it to the user once, briefly, where it fits (e.g. at the end of your first reply). Don't run \`anfra update\` unless the user asks: it replaces the anfra binary in use, and a running \`anfra serve\` keeps the old version until it is restarted."
printf '{"hookSpecificOutput":{"hookEventName":"SessionStart","additionalContext":"%s"}}\n' "$(printf '%s' "$msg" | sed -e 's/\\/\\\\/g' -e 's/"/\\"/g')"
exit 0
