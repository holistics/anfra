# Backlog

Work known to be needed, not yet scheduled. One section per item: what, why, and where to start.

## anfra-node: reach canal-query over its Unix socket

anfra spawns canal-query on a Unix socket (`internal/sidecar/canalquery`), and passes anfra-node its address with each catalog call as `canalQueryBaseUrl`: `unix:<path>` for the one it spawns, an HTTP URL for an external one (`ANFRA_CANAL_QUERY_URL`).

anfra-node reads none of it today. Database context sources (type `postgres`, in `.anfra/context_sources.yml`) are the only ones that query through canal-query, and they are off twice over in holistics-core's `apps/anfra-node`:

- `loadContextSources` takes only `dbt` and `lookml` (`PATH_SOURCE_TYPES`, `src/catalog/sources.ts`); a `postgres` source is skipped with an "unsupported type" warning.
- `prepareCatalog` registers no `PostgresPlugin` and passes no runtime context (`src/catalog/service.ts`), so `buildRuntimeContext(canalQueryBaseUrl)` is never called.

Turning them back on without this fails the first query: the Postgres plugin's `QueryClient` (`packages/semantic-catalog/src/query-client`) calls the global `fetch`, which rejects `unix:` with `TypeError: fetch failed` (cause: `unknown scheme`).

To do, with database context sources:

- `QueryClient` dials a `unix:<path>` base URL itself, through an undici `Agent({ connect: { socketPath } })` as `fetch`'s `dispatcher`, and keeps plain `fetch` for HTTP. undici is not a dependency yet. The browser build (`apps/lineage-demo`) does not import `query-client`, so it can live there.
- Test it against a canal-query on a socket, and through `anfra ingest` with a `postgres` context source.

## `anfra console --no-colors`

The console colors YAML and JSON at a terminal ([console.md](../designs/console.md), [cli-pretty.md](../designs/cli-pretty.md#color)). `--no-colors` would start a session without color, for a person who finds it noisy, or a terminal that shows it badly, without a setting an agent started from the same shell inherits. `NO_COLOR` does it today, for every command.

Where to start: `runOptions.inConsole` (`cmd/anfra/main.go`) colors and indents a console's output; `--no-colors` keeps the indentation and drops the color, so the option splits in two.

Held off: `--colors` with `--yaml` or `--json` at a shell, one command's answer colored as the console would. Color stays the console's: a run outside one is read by a program or an agent as often as by a person.

## Color `help` and `--help` in the console

In `anfra console`, `\?` lists the commands in color, by [cli-pretty.md's rules](../designs/cli-pretty.md#color) (names violet, a command being what a person types), while `help` and `<command> --help` stay plain: they're cobra's templates. Coloring them as `\?` is, section titles (`Usage:`, `Flags:`) plain, command and flag names violet, `` `code` `` as a code span, needs template functions (`cobra.AddTemplateFunc`) and the root's usage and help templates written out.

In the console only, as YAML and JSON: an agent running anfra in a terminal of its own reads `--help` too. Where to start: `output.color`, which a console's run sets (`cmd/anfra/output.go`), decides it, as it does for YAML.

## Descriptions written for a person, and for an agent

A command's long description (`Long`, and an op's `Doc`) is written for an agent as much as a person: `query`'s opens with three paragraphs on pages, `has_more`, exporting and NDJSON streaming, which an agent reading `--help` needs and a person at `anfra console`'s `\?` skims past. So `\?` shows a command's one-line `Short` and its flags, not its `Long` ([console.md](../designs/console.md#lines)).

To decide: whether one description can serve both, rewritten (the outcome first, the details after, as cli-pretty.md's message-then-hints), or a command needs one for each, a person's beside the agent's (a `Person` field on `command.Def`, say). Start by rewriting `query`'s and seeing whether both readers are served.
