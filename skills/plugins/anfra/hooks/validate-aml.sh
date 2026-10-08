#!/usr/bin/env sh
# PostToolUse hook: validate AML files with the Anfra CLI after Write/Edit.
# Claude Code / Cursor send the edited path as tool_input.file_path; Codex
# sends apply_patch with the patch text in tool_input.command, so paths are
# also read from its "*** Add/Update File:" and "*** Move to:" headers.
# Always non-blocking: emits findings as additionalContext (a warning the
# agent sees) and exits 0. Silent on non-AML files and on successful
# validation.

set -u

emit_warning() {
  esc=$(printf '%s' "$1" | sed -e 's/\\/\\\\/g' -e 's/"/\\"/g' | awk 'BEGIN{ORS=""} {if(NR>1) printf "\\n"; print}')
  printf '{"hookSpecificOutput":{"hookEventName":"PostToolUse","additionalContext":"%s"}}\n' "$esc"
}

input=$(cat)
files=$(
  {
    printf '%s' "$input" | sed -n 's/.*"file_path"[[:space:]]*:[[:space:]]*"\([^"]*\)".*/\1/p'
    printf '%s' "$input" | grep -oE '\*\*\* (Add File|Update File|Move to): [^"\\]*' \
      | sed 's/^\*\*\* [A-Za-z ]*: //'
  } | grep '\.aml$' | sort -u
)

[ -n "$files" ] || exit 0

if ! command -v anfra >/dev/null 2>&1; then
  emit_warning "AML file edited but the \`anfra\` CLI is not installed, so it was not validated: $(printf '%s' "$files" | tr '\n' ' ')"
  exit 0
fi

msg=""
IFS='
'
for file in $files; do
  [ -f "$file" ] || continue # e.g. the source of an apply_patch move
  out=$(anfra validate "$file" 2>&1)
  if [ "$?" -ne 0 ]; then
    msg="${msg:+$msg

}AML validation failed for $file:
$out"
  fi
done
[ -n "$msg" ] && emit_warning "$msg"
exit 0
