---
name: setup-repo
description: Set up a new Anfra repo — install the anfra CLI if needed, run `anfra init`, connect a data source, and get a first model and dataset validated and queried. Use when starting an Anfra project, connecting a database, or when a repo has no `.anfra/` config yet.
---

# Setting up an Anfra repo

An Anfra repo is a folder of AML (models and datasets), Data Apps, and a little config in
`.anfra/`. Done means: the data source answers a query, `anfra validate` passes, and an AQL query
returns rows from a dataset.

## 1. Check the CLI

```sh
anfra version
```

If it is missing, install it, then call it by the path the installer prints if it is not on the
`PATH` yet:

```sh
curl -fsSL https://raw.githubusercontent.com/holistics/anfra/main/install.sh | bash
```

Ask before installing. Reuse a working CLI rather than upgrading it unless the user asks
(`anfra update`).

## 2. Create the repo's files

From the repo's folder (an existing Git repo is fine):

```sh
anfra init            # or: anfra init <dir>
```

It creates what is missing and never changes an existing file, so it is safe to run again:

| Path | What it is |
|---|---|
| `.anfra/data_sources.yml` | The data sources to query, **with credentials**. Git-ignored. |
| `.anfra/data_sources.yml.example` | The same without credentials, to commit, so a collaborator knows what to fill in. |
| `.anfra/context_sources.yml` | What `anfra ingest` reads for `anfra search`: the repo's own AML. |
| `models/`, `datasets/`, `apps/` | AML models, AML datasets, Data Apps. |
| `.gitignore` | Gains `.anfra/data_sources.yml`. |

## 3. Connect the data source

Ask the user which database, and for its connection details. Then replace the template's
`warehouse` entry in `.anfra/data_sources.yml`:

```yaml
data_sources:
  <name>:            # what models and datasets name in data_source_name
    type: <type>     # postgresql, snowflake, bigquery, ... see the reference
    connection:      # keys depend on the type
      ...
```

**Read [references/data-sources.md](references/data-sources.md) for the type's exact keys.** They
are easy to guess wrong (`dbname`, not `database`; `user`, not `username`; `port` a number;
certificates and keys pasted in as text, not file paths), and a misspelled key is silently ignored.

Credentials:

- Write them only to `.anfra/data_sources.yml`, and check it is git-ignored (`git check-ignore
  .anfra/data_sources.yml`). Never echo a password back, put one in a commit, a log or your reply.
- Mirror the entry in `.anfra/data_sources.yml.example` with placeholders (`password: <password>`),
  so the committed file names the sources without their secrets.
- Prefer a read-only database user. Anfra only reads, and a read-only login keeps an exploratory
  SQL query from changing anything.

Check the connection with a trivial query:

```sh
anfra query --lang sql --data-source <name> 'select 1'
```

It fails with the database's own message (wrong host, login refused, unknown database): fix the
entry and run it again. The file is read on every command, so there is nothing to restart.

## 4. Model the data

Use the [](../aml/) skill: study the tables with `anfra query --lang sql --data-source <name>`
(schema and a data profile, never data questions), write a model per table in `models/`, their
relationships, and a dataset in `datasets/` that bundles them, each with `data_source_name: '<name>'`.

```sh
anfra validate        # the whole repo; fix every diagnostic
anfra show            # the datasets, as AQL sees them
```

## 5. Query it

Run one representative AQL query against the dataset with [](../run-aql/), and check a number
against a direct SQL query on the source (a row count, a total). Validation proves the AML is
well-formed, not that the numbers are right.

## Then

- `anfra serve --apps` serves the repo's Data Apps at `http://127.0.0.1:7878/apps/`: build one with
  [](../build-data-app/).
- `anfra ingest`, then `anfra search <words>`, finds models, fields and metrics by meaning.
- `anfra skills install` installs these skills into the user's coding agents.

## Related skills
* [](../aml/) — write the models, relationships and datasets.
* [](../run-aql/) — query a dataset.
* [](../build-data-app/) — build a Data App on the datasets.
