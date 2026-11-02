# The `anfra init` sample project

> Status: built in [holistics/anfra#99](https://github.com/holistics/anfra/pull/99) (`cmd/anfra/init.go`), not released. The sample lives in [holistics/anfra-sample-ecommerce](https://github.com/holistics/anfra-sample-ecommerce).

`anfra init` can add one sample project, so a new user or their coding agent can try anfra without connecting a warehouse.

## Goals

The sample should:

- work with no warehouse, credentials or database server
- work the same for a person at a terminal and an agent without one
- be opt-in: no choice means no sample
- get fixes without an anfra release
- be a public repo people can clone or fork

## What the sample adds

The files go in the repo's usual folders, under `ecommerce/`:

- `models/ecommerce/`: 6 AML models
- `datasets/ecommerce/ecommerce.dataset.aml`: the `ecommerce` dataset, with joins and 8 metrics
- `data/ecommerce.duckdb`: the data, 1.8 MB, opened read-only with no external access
- `apps/ecommerce/overview.html`: a Data App, opened with `anfra serve`; it defaults to all time, since the data ends in 2026
- `wiki/ecommerce/`: business rules, one file per topic, plus an `AGENTS.md` for maintaining them

In a fresh repo, `.anfra/data_sources.yml` gets an `ecommerce_duckdb` source in place of the Postgres placeholder:

```yaml
ecommerce_duckdb:
  type: duckdb
  connection:
    path: data/ecommerce.duckdb?access_mode=read_only&enable_external_access=false
```

## How `anfra init` behaves

| Call | Asks? | Adds the sample? |
|---|---|---|
| `anfra init` in a terminal | yes: `Include the e-commerce sample project? [y/N]` | only on `y` |
| `anfra init --with-sample` | no | yes |
| `anfra init --yes` (`-y`) | no | no |
| `anfra init` without a terminal (agents, CI) | never | no, but its output offers it |

Rules:

- the default is no, because a sample in a real repo is intrusive
- without a terminal, init never reads stdin; it prints a note telling an agent to ask its user, then run `anfra init <dir> --with-sample`
- `--help` gives agents the same instruction
- the base repo is written first, so a failed download leaves a working repo and prints the retry command
- existing files are never changed; an existing `data_sources.yml` is kept, and init prints the block to paste
- after adding the sample, init suggests `anfra validate`, `anfra show ecommerce`, a query and `anfra serve`
- there is no `anfra seed` or `seed --remove` command

## Decisions

### Download the latest `main`, not a pinned commit

How it works:

- init fetches `https://github.com/holistics/anfra-sample-ecommerce/archive/refs/heads/main.tar.gz`, with no GitHub API call or token
- it writes only allowlisted paths and ignores links, paths out of the repo and everything else
- the archive is capped at 32 MB
- it prints the commit it got, for example `holistics/anfra-sample-ecommerce@874df43`

Trade-off:

- gain: sample fixes reach every anfra version, with no release step
- cost: an old anfra can get a sample using newer AML, and runs are not reproducible

What limits the risk:

- branch protection on the sample's `main`: 1 approval, admins included, no force pushes
- the allowlist and the read-only DuckDB file
- sample repo CI (not merged yet): `anfra validate` and a query on the oldest supported anfra and the latest, on pull requests, `main` and weekly

### Agents learn about the sample from the skill first

Agents learn about the sample from:

- the `anfra-setup-repo` skill, which asks the user before running init, then runs init once with `--with-sample` or `--yes`
- `anfra init --help`
- init's output, after a plain `anfra init`

None of these is guaranteed: an agent may lack the skills and skip `--help`.

A quick check, 2 runs per setup, agents asked to "set up an anfra project" with no terminal:

| Agent had | What it did |
|---|---|
| no skill | read `--help`, asked before init |
| the skill without the sample step | ran plain `anfra init`, then asked: two runs |
| the skill with the sample step | asked before init |

This is a signal, not a measurement: one model, few runs.

### The sample does not touch the root `AGENTS.md`

- `anfra init` writes the anfra block into the root `AGENTS.md`
- the sample's only useful agent guidance is how to maintain its wiki, so it lives in `wiki/ecommerce/AGENTS.md`
- the root `AGENTS.md` is the same with or without the sample, and after one run or two

### The wiki holds rules, not numbers

- numbers from the data go stale, so agents profile the data themselves
- each metric lists its grain, time basis, aggregation and constraints
- "revenue" with no qualifier excludes cancelled orders
- accepted limit: `anfra search` cannot find the wiki, because ingestion reads only `aml`, `dbt` and `lookml`; agents read the folder directly

### `context_sources.yml` is unchanged

- the template does not hint `dbt` or `lookml` until anfra can convert them to AML

## Open

- **Separate repo or inside anfra.** A separate repo has its own page and ships fixes at once, but needs 2 PRs for coupled changes, its own CI, and must stay readable by every old anfra. Inside anfra (`samples/ecommerce/`, embedded in the binary) means 1 PR, existing CI, offline init and a sample tested with each release, at the cost of about 1.8 MB in the binary. anfra is public and releases often, so either is findable and fixes wait hours at most.
- **The two-run agent path.** The second run keeps the `data_sources.yml` the first run wrote, still the Postgres template, and prints the block to paste. Replacing a file still identical to the template would fix it.
- **User docs:** whether docs.anfra.ai should mention the sample.
- **More samples:** a second one would need a way to name it, such as `--with-sample=<name>`.
