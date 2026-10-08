# TODO: run `make check` before a push

`make check` runs CI's lint, type and API-contract checks locally. Running it from a git hook catches what would otherwise fail CI minutes later.

## Where: pre-push, not pre-commit

- **pre-commit** would run it on every commit, `--amend` and fixup. It checks the whole working tree, not what is committed, so unrelated uncommitted changes can fail a commit or pass a broken one. A hook that slow and that noisy teaches people and agents to skip it with `--no-verify`.
- **pre-push** runs it once per push, as the branch is about to reach CI and others, judging exactly what the push adds. It can still be skipped deliberately (`git push --no-verify`).

## Skip it when only docs change

So it isn't friction for doc-only maintainers: git passes the hook each ref being pushed with its local and remote commits, so the hook can list the files the push changes, and skip when every one is documentation (any `*.md`, and anything under `docs/`). A push that touches anything else, even alongside docs, runs the full check.

```sh
#!/usr/bin/env sh
# .husky/pre-push: run make check before a push, unless it changes nothing but docs.
zero=0000000000000000000000000000000000000000
changed=$(
  while read -r local_ref local_sha remote_ref remote_sha; do
    [ "$local_sha" = "$zero" ] && continue                      # deleting a branch: nothing to check
    if [ "$remote_sha" = "$zero" ]; then                         # a new branch: what it adds to main
      base=$(git merge-base origin/main "$local_sha")
    else
      base=$remote_sha
    fi
    git diff --name-only "$base" "$local_sha"
  done
)
if [ -n "$changed" ] && ! printf '%s\n' "$changed" | grep -qvE '(\.md$|^docs/)'; then
  echo "pre-push: only docs changed, skipping make check"
  exit 0
fi
exec make check
```

The hook is installed like the existing commit-msg hook: husky sets up `.husky/` on `pnpm install`, which `make setup` runs.

## To do

1. Add `.husky/pre-push` (above); mention it in DEVELOPMENT.md's Checks and in AGENTS.md.
2. Test it against a scratch remote, not GitHub: a docs-only push (skips), a mixed push (runs), a new branch (diffs from its merge base with main), deleting a branch (skips), and pushing several refs at once.
3. Decide whether CI should skip its jobs on docs-only changes too. Not with `paths-ignore`: `ci_passed` is the required check, and a workflow that doesn't run leaves it pending forever, blocking the merge. Instead, a first step that detects docs-only changes and lets the other jobs skip themselves, so `ci_passed` still reports. Only worth it if CI time on docs PRs bothers anyone.
