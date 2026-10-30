# Developing the Anfra skills

## Setup

Requires Node.js 22 and pnpm.

```
pnpm install      # also installs git hooks (commitlint, link + frontmatter validation)
pnpm commit       # guided Conventional Commit
```

Commits follow [Conventional Commits](https://www.conventionalcommits.org/) with the
plugin as scope, e.g. `feat(plugins.anfra): ...`.

## Releasing

`main` is protected, so releases go through a PR:

1. On a branch, run `pnpm bump plugins/<plugin> <version>`. This bumps the plugin's
   `plugin.json` (Claude, Cursor and Codex), the matching marketplace version, and the
   plugin's `CHANGELOG.md`, and writes the version at the end of each of its skills
   (`---`, then `anfra skills <version>`), which `pnpm validate-frontmatter` checks.
2. Commit as `release(plugins.<plugin>): <version>` and open a PR.
3. After the PR merges, CI tags the commit as `<plugin>-v<version>` and
   `marketplace-v<version>`.
