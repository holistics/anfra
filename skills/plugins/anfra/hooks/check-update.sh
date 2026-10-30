#!/usr/bin/env sh
# SessionStart hook: tell the agent when anfra or these skills are behind their
# latest releases, so it can mention it to the user. Passes this plugin's skills
# version, from the stamp ending its skills, to `anfra version --skills`, which
# compares both with anfra's own cached check (no network here; anfra refreshes
# the cache in the background) and answers what to act on as hints
# (docs/designs/agent-setup.md). An anfra too old to know --skills, or skills
# with no stamp, as in a checkout of anfra's skills/, get plain `anfra version`
# and its update notice alone.
#
# Silent when nothing needs acting on, anfra is not installed or too old to
# report updates, opted out (ANFRA_NO_UPDATE_NOTIFIER), or when the session is
# not in an anfra repo: `anfra version` sets up the directory it runs in as a
# repo. Always exits 0.

set -u

input=$(cat)
cwd=$(printf '%s' "$input" | sed -n 's/.*"cwd"[[:space:]]*:[[:space:]]*"\([^"]*\)".*/\1/p')
[ -n "$cwd" ] && cd "$cwd" 2>/dev/null

[ -d .anfra ] || exit 0
[ "${ANFRA_NO_UPDATE_NOTIFIER:-}" = 1 ] && exit 0
command -v anfra >/dev/null 2>&1 || exit 0

after="Mention this to the user once, briefly, where it fits (for example at the end of your first reply). Don't run these commands unless the user asks: \`anfra update\` replaces the anfra binary in use, and a running \`anfra serve\` keeps the old version until it's restarted; updated skills load only after a restart."

notify() {
  printf '{"hookSpecificOutput":{"hookEventName":"SessionStart","additionalContext":"%s"}}\n' "$(printf '%s' "$1" | sed -e 's/\\/\\\\/g' -e 's/"/\\"/g')"
  exit 0
}

# This plugin's skills version: the stamp ending each skill, "---" then
# "anfra skills <version>".
plugin_root=$(cd "$(dirname "$0")/.." && pwd)
skills=$(tail -n 1 "$plugin_root/skills/anfra-setup-repo/SKILL.md" 2>/dev/null | sed -n 's/^anfra skills \([^[:space:]]*\)[[:space:]]*$/\1/p')

# Claude Code sets CLAUDE_PLUGIN_ROOT; the hints then name the agent to set up.
agent='<your id>'
[ -n "${CLAUDE_PLUGIN_ROOT:-}" ] && agent=claude-code

# YAML: hints: then one "- <hint>" a line, plain or single-quoted ('' for ').
if [ -n "$skills" ] && out=$(anfra version --skills "$skills" 2>/dev/null); then
  hints=$(printf '%s\n' "$out" | awk -v q="'" '
    /^hints:/ { in_hints = 1; next }
    /^[^[:space:]]/ { in_hints = 0 }
    in_hints && /^[[:space:]]+- / {
      sub(/^[[:space:]]+- /, "")
      if (substr($0, 1, 1) == q && substr($0, length($0), 1) == q) {
        $0 = substr($0, 2, length($0) - 2)
        gsub(q q, q)
      }
      printf "%s%s", sep, $0
      sep = " "
    }')
  [ -n "$hints" ] || exit 0
  notify "anfra says: $(printf '%s' "$hints" | sed "s/<your id>/$agent/g") $after"
fi

# An anfra that doesn't know --skills, or skills with no stamp: anfra's update
# notice alone.
# YAML: version: 0.4.2, and when known, update: { available: true, latest: 0.5.0 }.
out=$(anfra version 2>/dev/null) || exit 0
printf '%s\n' "$out" | grep -Eq '^[[:space:]]+available:[[:space:]]*true' || exit 0
latest=$(printf '%s\n' "$out" | sed -n '/^update:/,/^[^[:space:]]/s/^[[:space:]]*latest:[[:space:]]*//p' | head -n 1)
current=$(printf '%s\n' "$out" | sed -n 's/^version:[[:space:]]*//p' | head -n 1)
[ -n "$latest" ] || exit 0

notify "anfra $latest is available (installed: ${current:-unknown}). Mention it to the user once, briefly, where it fits (e.g. at the end of your first reply). Don't run \`anfra update\` unless the user asks: it replaces the anfra binary in use, and a running \`anfra serve\` keeps the old version until it is restarted."
