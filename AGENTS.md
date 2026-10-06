# Agent guidelines

## Commit messages

CI runs commitlint on every commit in a pull request (see `commitlint.config.mjs`, which extends `@commitlint/config-conventional`). One bad commit fails the `commitlint` and `ci_passed` checks, and fixing it means rewriting history, so get each message right when you commit.

- Header format: `type: subject` or `type(scope): subject`, for example `docs: add quickstart to README` or `fix(serve): handle empty query results`.
- Type must be one of: `feat`, `fix`, `perf`, `docs`, `style`, `chore`, `refactor`, `test`, `build`, `ci`, `security`, `release`.
- Scope is optional. If used, it must be one of: `cli`, `serve`, `query`, `validate`, `sidecar`, `repo`, `datasource`, `meta`, `deps`.
- Subject: lowercase start, imperative mood, no trailing period.
- Header: at most 100 characters.
- Body and footer: every line at most 100 characters. Wrap the body by hand at around 72 characters, and leave a blank line between the header and the body.

Before pushing, check the commits on your branch (run `pnpm install` once first):

```sh
pnpm exec commitlint --from origin/main --to HEAD --verbose
```
