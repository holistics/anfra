---
name: anfra-setup-repo
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

Before running `anfra init`, ask the user whether to add the e-commerce sample: 6 models, a
dataset, a small DuckDB file and a Data App, so they can try anfra without a warehouse of their own.
Ask first, because init writes `.anfra/data_sources.yml` and never changes it afterwards. Then run
init once, from the repo's folder (an existing Git repo is fine):

```sh
anfra init --example   # the user wants the sample; or: anfra init <dir> --example
anfra init --yes       # they don't; or: anfra init <dir> --yes
```

With the sample, the repo already queries its DuckDB file: skip step 3 and run `anfra validate`.

`anfra init` creates what is missing and never changes an existing file beyond adding to it, so it is safe
to run again:

| Path | What it is |
|---|---|
| `.anfra/data_sources.yml` | The data sources to query, **with credentials**. Git-ignored. |
| `.anfra/data_sources.yml.example` | The same without credentials, to commit, so a collaborator knows what to fill in. |
| `.anfra/context_sources.yml` | What `anfra ingest` reads for `anfra search`: the repo's own AML. |
| `models/`, `datasets/`, `apps/` | AML models, AML datasets, Data Apps. |
| `.gitignore` | Gains `.anfra/data_sources.yml`. |
| `AGENTS.md` | Gains the anfra block, telling coding agents to use the anfra skills (and `@AGENTS.md` in an existing `CLAUDE.md`). |

Ask the user whether they're starting a new repo, or joining one their org already publishes. To
join one:

- **If the team shares it through git,** clone it, and run `anfra init` in the clone, which adds
  what isn't committed. The org's address is committed in `.anfra/config.yml`, so there's nothing
  to pull, and the committed `.anfra/data_sources.yml.example` names the data sources.
- **Otherwise,** bring its files from the org, as below.

Then connect the data source (step 3) and query it (step 5). The models are already there, so skip
step 4.

### Joining from the org, without git

`anfra pull` needs the org's address, `https://<server>/o/<org>`. Find it with the user:

1. Run `anfra auth status`. It lists the orgs the user is signed in to on this machine, with their
   addresses. If one is the org, confirm it with the user.
2. Otherwise, ask the user to open the org in their browser, and paste the address of any of its
   pages, such as `https://bi.example.com/o/acme/dashboards/12`. anfra keeps only the org's part.
3. If the user knows only the server's address, run `anfra auth login https://<server>`. The user
   chooses the org in the browser, and the login's result gives its address.

Then pull, which saves the address as the repo's org (`org.url` in `.anfra/config.yml`):

```sh
anfra pull --set-org https://<server>/o/<org>
```

If it answers that the user isn't signed in, run `anfra auth login https://<server>/o/<org>`. It
opens the user's browser, where they approve the sign-in, so tell them to look for it. Then pull
again.

Or ask the user to run `anfra pull` in their own terminal. There, it lists the orgs they're signed
in to, lets them choose, and offers to sign in to another. Then carry on once they say it's done.

The pull's hints name the data sources the repo uses that `.anfra/data_sources.yml` doesn't have,
with their types. Add those in step 3.

See [](../anfra-publish/) for pulling and publishing with others.

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

Use the [](../anfra-aml/) skill: study the tables with `anfra query --lang sql --data-source <name>`
(schema and a data profile, never data questions), write a model per table in `models/`, their
relationships, and a dataset in `datasets/` that bundles them, each with `data_source_name: '<name>'`.

```sh
anfra validate        # the whole repo; fix every diagnostic
anfra show            # the datasets, as AQL sees them
```

## 5. Query it

Run one representative AQL query against the dataset with [](../anfra-run-aql/), and check a number
against a direct SQL query on the source (a row count, a total). Validation proves the AML is
well-formed, not that the numbers are right.

## Then

- `anfra serve` serves the repo's Data Apps at `http://127.0.0.1:7878/apps/`: build one with
  [](../anfra-build-data-app/).
- `anfra ingest`, then `anfra search <words>`, finds models, fields and metrics by meaning.
- `anfra agent setup` installs these skills into the user's coding agents.
- `anfra publish` publishes the repo to the user's org, once they ask. A team that shares work
  through git publishes what it merged to its shared branch: see [](../anfra-publish/).

## Related skills
* [](../anfra-aml/) — write the models, relationships and datasets.
* [](../anfra-run-aql/) — query a dataset.
* [](../anfra-build-data-app/) — build a Data App on the datasets.
* [](../anfra-docs/) — look up the anfra and AMQL docs, for what these skills don't cover.
* [](../anfra-publish/) — publish to an org, and pull what others published.
