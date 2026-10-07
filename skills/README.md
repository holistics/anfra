# Anfra skills

A marketplace of agent skills for working with [Anfra](https://anfra.ai) —
building and exploring analytics via **AML** (modeling) and **AQL** (querying)
against a local AMQL repo, using the `anfra` CLI (`anfra query`, `anfra validate`).

The skills are self-contained: they assume only the `anfra` command, not any
other tools or platform.

## Install

### Claude Code
Within Claude Code, run this command to add the Anfra skills marketplace:
```bash
/plugin marketplace add holistics/anfra-skills
```

Then browse and install the relevant plugins via `/plugin` > `Marketplaces` > `anfra-skills` > `Browse plugins` > Install.

(Optional) enable auto-update via `/plugin` > `Marketplaces` > `anfra-skills` > `Enable auto-update`.

### Cursor
Cursor's plugin marketplaces are managed at the team/org level (Teams or Enterprise plans). An admin imports the marketplace once, then teammates install individual plugins.

1. As an admin, go to **Dashboard** → **Settings** → **Plugins**.
2. Under **Team Marketplaces**, click **Import** and paste `https://github.com/holistics/anfra-skills`.
3. Review the parsed plugins, set Team Access groups if needed, name the marketplace, and save.
4. Teammates open the marketplace panel in Cursor and install the plugins they want (required plugins install automatically).

### Codex
Add the marketplace with the Codex CLI:
```bash
codex plugin marketplace add holistics/anfra-skills
```

Then install the plugins you want from the Plugins Directory. Upgrade later with `codex plugin marketplace upgrade`.

The Codex plugin includes the skills and the AML validation hook. The `aql-writer` agent is only available in Claude Code, because the Codex plugin format doesn't document support for subagents.

## Plugins

### `anfra-development`
Develop analytics with Anfra.

| Skill | What it does |
|---|---|
| `aql` | Core AQL knowledge base (lessons, per-function reference docs, worked examples). |
| `write-aql` | Author and validate an AQL query to answer a data question. |
| `validate-aql` | Type-check an AQL query or expression without running it. |
| `run-aql` | Run a validated AQL query and return rows (or compile it to SQL). |
| `lookup-values` | Verify a field's exact stored values before filtering. |
| `aml` | Write and edit AML models, fields, relationships, and datasets. |
| `setup-repo` | Set up a new Anfra repo: `anfra init`, connect a data source, a first model and dataset. |
| `build-data-app` | Build a Data App (one HTML file in `apps/`) on the repo's datasets, and check it in `anfra serve --apps`. |

| Agent | What it does |
|---|---|
| `aql-writer` | Writes and validates AQL in a subagent, keeping the reference-heavy authoring out of the main context. |

| Hook | What it does |
|---|---|
| Validate AML | After each `Write`/`Edit` (or Codex `apply_patch`) of an AML file, runs `anfra validate` on it and reports diagnostics. |

More plugins (e.g. for consumers/explorers) may be added to this marketplace over time.

## Development

Requires Node.js 22 and pnpm.

```
pnpm install      # also installs git hooks (commitlint, link + frontmatter validation)
pnpm commit       # guided Conventional Commit
```

Commits follow [Conventional Commits](https://www.conventionalcommits.org/) with the
plugin as scope, e.g. `feat(plugins.anfra-development): ...`.

## Releasing

`main` is protected, so releases go through a PR:

1. On a branch, run `pnpm bump plugins/<plugin> <version>`. This bumps the plugin's
   `plugin.json` (Claude, Cursor and Codex), the matching marketplace version, and the
   plugin's `CHANGELOG.md`.
2. Commit as `release(plugins.<plugin>): <version>` and open a PR.
3. After the PR merges, CI tags the commit as `<plugin>-v<version>` and
   `marketplace-v<version>`.
