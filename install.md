# Install anfra

These instructions are for a coding agent (Claude Code, Codex, Cursor or another) setting up anfra for a user. Run the steps in order, running the commands yourself. Stop to ask the user only where a step says to.

anfra is an open-source framework for building governed BI apps with coding agents. You model the user's warehouse data as metrics, then build Data Apps (single HTML pages using the anfra SDK) that query those metrics. The `anfra` CLI runs everything locally. Docs: https://anfra.ai/docs

## 1. Install the CLI

macOS or Linux. The download is about 250 MB.

```sh
curl -fsSL https://anfra.ai/install.sh | bash
export PATH="$HOME/.anfra/bin:$PATH"
```

The installer adds `~/.anfra/bin` to the user's shell startup files, but your current shell hasn't loaded them. Run the `export` line in each new shell, or call `~/.anfra/bin/anfra` directly. Check that it works with `anfra --version`.

## 2. Install the agent skills

```sh
anfra agent setup
```

This installs the anfra skills (`anfra-setup-repo`, `anfra-build-data-app`, `anfra-write-aql` and others) into Claude Code and Codex, and prints how to install them in any other agent it finds. If the skills don't load in this session until you restart, read them from https://github.com/holistics/anfra-skills and carry on.

## 3. Create a project

In the folder the user opened, or in a new one:

```sh
anfra init
```

This creates `models/`, `datasets/` and `apps/`, an `AGENTS.md`, and `.anfra/data_sources.yml`, which is git-ignored because it holds credentials.

## 4. Connect a database

Ask the user which database to connect. Fill in `.anfra/data_sources.yml` with placeholders for passwords, keys and tokens, and let the user enter those themselves. Never print them or ask for them in the chat. Suggest a read-only database user. Every database type and its fields: https://anfra.ai/docs/local-dev/data-sources

If the user has no database to hand, or wants to try anfra first, use the sample ecommerce database (customers, orders, order items, products, sessions, countries):

```sh
mkdir -p db
curl -fsSL -o db/ecommerce.duckdb https://anfra.ai/samples/ecommerce.duckdb
```

```yaml
# .anfra/data_sources.yml
data_sources:
  ecommerce:
    type: duckdb
    connection:
      path: db/ecommerce.duckdb?access_mode=read_only
```

## 5. Pick the app

If the user already described the app they want, build that. If not, look at the tables, suggest two or three apps you could build, and wait for the user to pick one.

## 6. Model the data

Use the `anfra-setup-repo` and `anfra-aml` skills. Model only the tables and metrics the app needs, in `models/` and `datasets/`, then run:

```sh
anfra validate
```

Fix what it reports and run it again until it passes.

## 7. Build the app

Use the `anfra-build-data-app` skill. Write the app as one HTML file in `apps/`, for example `apps/revenue.html`.

## 8. Serve it

```sh
anfra serve
```

Give the user the link to the app: an app at `apps/revenue.html` opens at http://127.0.0.1:7878/apps/revenue and reloads as files change. Tell them they can click any number in the app to see the metric and the SQL behind it.

## More

- Installer options, Docker, other agents: https://anfra.ai/docs/local-dev/installation
- Quick start for people: https://anfra.ai/docs/overview/quick-start
- CLI reference: https://anfra.ai/docs/reference/cli
- Docs index for agents: https://anfra.ai/docs/llms.txt
- Source: https://github.com/holistics/anfra
