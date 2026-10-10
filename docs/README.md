# docs

How anfra is designed, and how to use it beyond what the user docs cover. The user docs, for people building analytics with anfra, are at [docs.anfra.ai](https://docs.anfra.ai).

## Designs

Why anfra is the way it is: read these before changing the parts they cover.

| Doc | Covers |
|---|---|
| [philosophy](designs/philosophy.md) | What anfra core is and where it stops, and the guidelines that follow from that. Start here. |
| [architecture](designs/architecture.md) | anfra and its sidecars, how a command runs, the CLI and `anfra serve`, where state lives. |
| [engine](designs/engine.md) | The `engine` package: what a platform embedding anfra gets, and what core must keep promising it. |
| [commands and API](designs/commands-and-api.md) | One command definition, served as the CLI, the HTTP API and MCP; the OpenAPI contract. |
| [errors](designs/errors.md) | The error framework: codes, scopes, violations, and how an error reaches each surface. |
| [JSON](designs/json.md) | How anfra reads and writes JSON: one package, `shared/jsonkit`, and the rule that every package uses it. |
| [exports](designs/exports.md) | A query's whole result as a file, through one op that answers a link, on every door and server. |
| [export stores](designs/export-store.md) | Where an export's file goes and how its link is served: anfra's own store, its cleanup, and how a platform's differs. |
| [Data Apps](designs/data-apps.md) | Data Apps end to end: the SDK, the sandboxed frame, provisioning, Query Input, appserve. |
| [security](designs/security.md) | The trust model: what `anfra serve` protects, what it leaves to the operator. |
| [sidecars](designs/sidecars.md) | The sidecars' lifecycle: embedding, spawning, shutdown, caches, what passes between processes. |
| [release](designs/release.md) | From `manifest.yml` to a release: binaries, the Docker image, versions, updates. |

## Usage

How to run and build on anfra: embedding the engine, running the Docker image, configuring a setup. See [usage/](usage/README.md).

For working on anfra itself, see [DEVELOPMENT.md](../DEVELOPMENT.md).
