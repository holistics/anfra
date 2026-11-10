# docs

How anfra is designed, and how to use it beyond what the user docs cover. The user docs, for people building analytics with anfra, are at [docs.anfra.ai](https://docs.anfra.ai).

## Designs

Why anfra is the way it is: read these before changing the parts they cover.

| Doc | Covers |
|---|---|
| [philosophy](designs/philosophy.md) | What anfra core is and where it stops, and the guidelines that follow from that. Start here. |
| [architecture](designs/architecture.md) | anfra and its sidecars, how a command runs, the CLI and `anfra serve`, where state lives. |
| [CLI and warm server](designs/cli-vs-warm-server.md) | Why the CLI forwards commands to a running `anfra serve`, and which commands it keeps: CLI-only and CLI-local ones. |
| [engine](designs/engine.md) | The `engine` package: what a platform embedding anfra gets, and what core must keep promising it. |
| [commands and API](designs/commands-and-api.md) | One command definition, served as the CLI, the HTTP API and MCP; the OpenAPI contract. |
| [CLI pretty output](designs/cli-pretty.md) | How a command's result is shown to a person at a terminal: the prettifier's conventions, hints and views, and the bar a view must clear. |
| [errors](designs/errors.md) | The error framework: codes, scopes, violations, and how an error reaches each surface. |
| [errors in a Data App](designs/errors/data-apps.md) | A Data App's errors: their shape on any host, what crosses the bridge, and where each runtime error goes (the panel, `anfra.log`, a host's listeners). |
| [logs](designs/logs.md) | What a log record carries for people and for programs, and the events: a soft contract for alerts and dashboards. |
| [JSON](designs/json.md) | How anfra reads and writes JSON: one package, `shared/jsonkit`, and the rule that every package uses it. |
| [exports](designs/exports.md) | A query's whole result as a file, through one op that answers a link, on every door and server. |
| [export stores](designs/export-store.md) | Where an export's file goes and how its link is served: anfra's own store, its cleanup, and how a platform's differs. |
| [result values](designs/result-values.md) | What a query's answer holds: values encoded (every number a string), and each column's semantic, Arrow and database types. |
| [numbers in the SDK](designs/sdk-numbers.md) | What a Data App gets for a number: the API's string and its column's type, which say what it is; helpers come later. |
| [Data Apps](designs/data-apps.md) | Data Apps end to end: the SDK, the sandboxed frame, provisioning, Query Input, appserve. |
| [SDK plugins](designs/sdk-plugins.md) | Everything a Data App reaches is a plugin the host installs: how host, frame and plugins talk, middleware, the plugin layer. |
| [security](designs/security.md) | The trust model: what `anfra serve` protects, what it leaves to the operator. |
| [background routines](designs/background-routines.md) | Work a server runs beside its requests: its kinds, the rules, panics, and the `shared/routine` helper. |
| [sidecars](designs/sidecars.md) | The sidecars' lifecycle: embedding, spawning, shutdown, caches, what passes between processes. |
| [MotherDuck copy](designs/motherduck-copy.md) | `anfra publish` offering to copy a DuckDB data source to MotherDuck, through MotherDuck's CLI, with a token of the org's own. |
| [release](designs/release.md) | From `manifest.yml` to a release: binaries, the Docker image, versions, updates. |
| [init sample](designs/init-sample.md) | `anfra init --example`: the e-commerce sample from its own public repo, asked in a terminal and offered to agents. |
| [anfra console](designs/console.md) | A session at a terminal: AQL typed directly, commands behind a `\`, the output coloured for the person typing, a warm server. |
| [agent setup](designs/agent-setup.md) | Installing the anfra skills into coding agents, keeping them current, and telling agents when they aren't. |
| [readable prose](designs/readable-prose.md) | How to write error messages, notices, help and docs that a person or an agent can read and act on. |

## Research

Findings and options for work not yet designed.

| Doc | Covers |
|---|---|
| [skills install](research/skills-install.md) | Installing the anfra skills into every coding agent, keeping them current, and what each agent supports. |

## Implementations

Plans for work being built: the steps, their order, and where each stands.

| Doc | Covers |
|---|---|
| [`--json` on every CLI command](implementations/cli-json.md) | Every command's answer, warnings and error as JSON instead of YAML: the shared rendering first, then the CLI-only commands one at a time. |
| [`anfra console`](implementations/console.md) | The console's steps: one process running many commands, the skeleton, output for a person, modes and AQL, a warm server. |
| [revamp query result data types](implementations/revamp-query-result-data-types.md) | Encoded values (every number a string) and each column's semantic, Arrow and database types, across canal-query, amql, the core and the SDK. |

## Todos

Work known or agreed, not built yet.

| Doc | Covers |
|---|---|
| [backlog](todos/backlog.md) | Work known to be needed, not yet scheduled: what, why, and where to start. |
| [check before push](todos/check-pre-push.md) | Running `make check` from a pre-push hook, to catch what would fail CI. |
| [publish status and check](todos/publish-status-and-check.md) | `anfra publish status`, and the rest of `anfra publish check`: asking about a publish without publishing. |
| [improve prose](todos/improve-proses.md) | Rewriting anfra's user-facing text by the readable prose guide: errors first, then descriptions, notices and help. |

## Usage

How to run and build on anfra: embedding the engine, running the Docker image, configuring a setup. See [usage/](usage/README.md).

## Dev

Conventions for writing anfra's code.

| Doc | Covers |
|---|---|
| [method naming](dev/method-naming.md) | Methods as messages: whose act a name is, hooks versus methods, one name for one action across layers. |
| [error mapping](dev/error-mapping.md) | One function per operation boundary deciding which error its caller sees: nil and the unmapped passing through, called at every exit. |

For working on anfra itself, see [DEVELOPMENT.md](../DEVELOPMENT.md).
