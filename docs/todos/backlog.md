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
