# Agent guidelines

## Before changing anything

- Read [docs/designs/philosophy.md](docs/designs/philosophy.md): what belongs in anfra core, and the guidelines a change should follow. The other [design docs](docs/README.md) cover each part; read the one for the part you change.
- [DEVELOPMENT.md](DEVELOPMENT.md) says how to set up, run, test and release.

## Checks

Run what CI runs before you finish:

```sh
make check      # lint, type-check, the API contract
make test
```

If you changed a command's input or answer, regenerate the API contract and the SDK's types, and commit both:

```sh
scripts/openapi.sh generate
pnpm --filter anfra-sdk generate
```

## Commit messages

commitlint checks every commit, in a git hook and in CI. One bad commit fails CI, and fixing it means rewriting history, so get each message right when you commit.

- The rules are `commitlint.config.mjs`, extending `@commitlint/config-conventional`. Read it for the allowed types and scopes; they change.
- Header: `type: subject` or `type(scope): subject`, subject lowercase, imperative, no period.
- A change that breaks the core API's contract must say so: `feat!:`, or a `BREAKING CHANGE:` footer. CI checks.
- Wrap the body by hand at around 72 characters.

Check a branch's commits before pushing:

```sh
pnpm exec commitlint --from origin/main --to HEAD --verbose
```
