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

## Third-party license notices in releases

anfra's releases ship other projects' code inside their binaries, on every OS, with none of their license notices. Checked on 2026-11-24 ([windows-canal-build.md, Licenses](../research/windows-canal-build.md#licenses)):

- anfra has `LICENSE` (Apache-2.0) and a `NOTICE` with only its own copyright. The release (`.github/workflows/build_release.yml`) ships the gzipped binary alone; the Docker image only labels `licenses=Apache-2.0`.
- canal has no `LICENSE` or `NOTICE`. anfra-node (holistics-core `apps/anfra-node`) has no `LICENSE` and no `license` in `package.json`.
- Inside the binaries: DuckDB (MIT), statically linked into canal on every OS; Apache Arrow (Apache-2.0, whose `NOTICE` must be passed on); godror and ODPI-C (Apache-2.0 / UPL); Bun's runtime with WebKit's JavaScriptCore (its own terms), compiled into anfra-node; and hundreds of Go and npm modules. None of their notices are shipped, and none are in the binaries.

MIT, for one, asks that "the above copyright notice and this permission notice shall be included in all copies or substantial portions of the Software" ([DuckDB's LICENSE](https://github.com/duckdb/duckdb/blob/main/LICENSE)). DuckDB's own Windows download, `libduckdb-windows-amd64.zip`, holds no license file, so passing it along doesn't carry the notice either.

Native Windows support depends on it: anfra will ship DuckDB's official `duckdb.dll` beside canal, for MotherDuck, only if these notices are handled; otherwise the Windows build stays static and MotherDuck isn't supported on native Windows ([WINDOWS-PLAN.md, Decisions needed](../../WINDOWS-PLAN.md#decisions-needed)). The static build ships DuckDB too, so the notices are needed either way.

To do:

- Confirm with whoever owns licensing at Holistics what to ship and where, Bun's WebKit part in particular.
- Generate a `THIRD_PARTY_NOTICES` file at build time, from all three repos: `go-licenses` for anfra's and canal's Go modules, a license tool for anfra-node's npm bundle, and hand-written entries for what those tools can't see (DuckDB and its bundled libraries, Bun's runtime).
- Ship it in every release archive, and make it readable from the binary (a command or a docs page), since anfra unpacks its sidecars where users don't look.
- Fail CI when a new dependency's license isn't on an allowed list.

### DuckDB, for the Windows `duckdb.dll`

The part native Windows depends on. Linux and macOS already ship DuckDB linked inside `anfra-canal`, so it's needed there too; Windows only makes it visible, as its own file.

- **DuckDB's notice:** the copyright line ("Copyright 2018-2026 Stichting DuckDB Foundation") and the MIT text, from DuckDB's `LICENSE` at the shipped version (v1.5.6 today). DuckDB's `libduckdb-windows-amd64.zip` holds no license file, so anfra adds it.
- **What's built into `duckdb.dll`:** DuckDB compiles other libraries in, each with its own license and notice: `fmt`, `re2`, `utf8proc`, `yyjson`, `zstd`, `miniz`, `mbedtls`, `fsst`, `fastpforlib`, `hyperloglog`, `skiplistlib`, `pg_query`, and the built-in `icu`, `json` and `parquet` extensions (MIT, BSD, Apache-2.0, the Unicode license for ICU, the PostgreSQL license for `pg_query`). Their texts are in DuckDB's source, under `third_party/` and the extension folders, at the same version. The list above is from the static build's link line; check it against the shipped version.
- **Where users see it:** the DLL is unpacked into `%USERPROFILE%\.anfra\sidecars\…`, where nobody looks, so the notices go in a `THIRD_PARTY_NOTICES.txt` inside the release zip, next to `anfra.exe`, and in a docs page or an `anfra licenses` command.
- **Kept in step:** generate the DuckDB part from the same DuckDB version as the shipped DLL, at release time; CI fails if the two differ.
- **Ship the DLL unmodified:** `duckdb.dll` is signed by "Stichting DuckDB Foundation" (checked 2026-11-25: signature valid). Changing it breaks that signature, and re-signing it as Holistics would misstate who made it.
- **Not ours to license:** extensions DuckDB downloads at run time (MotherDuck's, and `json`/`icu` for a static build) come from DuckDB's and MotherDuck's servers to the user's machine; anfra doesn't ship them. MotherDuck's service and extension have their own terms between the user and MotherDuck.
- **Trademark:** "DuckDB" is the DuckDB Foundation's trademark. Saying anfra uses DuckDB is fine; implying DuckDB endorses anfra isn't.

Effort for the DuckDB part alone: ~0.5–1 engineer-day with Claude, plus the licensing review. It's a subset of the whole item.

Rough effort for the whole item: 1–2 engineer-days with Claude, plus the licensing review.

## Ship anfra-node's Windows stop

anfra stops anfra-node on Windows with Ctrl-Break, which reaches it as `SIGBREAK` (`internal/sidecar/stop_windows.go`, commit "stop anfra-node with ctrl-break on windows"). The anfra-node that handles it is only on a local holistics-core branch, `feat/anfra-node-windows-stop` (commit `4b3059a4f`, "fix(apps.anfra-node): shut down on sigbreak", with a `patch` changeset), not pushed ([SPEC-win-anfra-node.md](../../SPEC-win-anfra-node.md)).

Without the handler, Windows ends anfra-node at once on Ctrl-Break: its shutdown doesn't run and its compile cache isn't written. So, before anfra ships on Windows (`win-release`):

1. Push the holistics-core branch and open its PR.
2. Release an anfra-node with it.
3. Move anfra's pin (`manifest.yml`, `anfra_node`) to that release.

Until then, only anfra built from source on Windows meets an anfra-node without the handler.

## File names Windows can't keep apart, or hold

Found in `win-files-auth` ([SPEC-win-files-auth.md](../../SPEC-win-files-auth.md)).

- **Names that differ only in case** (`Orders.aml`, `orders.aml`): Windows' file names ignore case, so pull on Windows writes one over the other. Pull should refuse them on Windows, naming both.
- **Names Windows can't hold:** on Windows, pull now refuses device names (`CON`, `aux.aml`…), `:` `<>"|?*`, control characters and a trailing dot or space (`internal/publish/workdir`). A repo published from Linux or macOS can still hold them, and then can't be pulled on Windows. Publish could warn about them, on every OS, so a repo stays usable by a teammate on Windows.
