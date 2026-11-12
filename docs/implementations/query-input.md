# Implementation: a query's input

Implements [a query's input](../designs/query-input.md): everything a caller chooses about a query, other than its text, in one object, `input`, which an answer returns as the query ran; and the seeds, the first of which is the time a query runs as of. Built first for the [query cache](../designs/query-cache.md), which finds a result again by a query and its seeds.

The change spans anfra-node, anfra core (the API, the CLI, the engine's types) and the SDK. This doc says why, what changed shape on each surface, who it affects, and the order it ships in.

## Why the page, the time zone and the seeds move into `input`

> **To run a query again, a caller sends its text and the `input` its answer returned, and nothing else.**

Callers run a query again all the time: the next page, an export of what's on screen, an agent reading what a user saw, a requery on a cached result. Each needs the query exactly as it ran.

- **Spread across the request, every input is one more thing to remember, and forgetting one fails silently.** A next page sent without the first page's time zone, or without its seeds, isn't refused: it runs another query that looks the same. A `last 7 days` page fetched after midnight shows a different week.
- **The time is an input now, and it must travel with the rest.** A query's rows depend on when it runs (`last 7 days`), and that time is generated on each request unless the caller sets it. The answer returns it in `input`, so whoever replays the query replays the time too, without knowing it exists.
- **One object makes replaying mechanical.** A client, an agent or the SDK copies `input`, changing only what its purpose needs (the next page sets `pagination.page`; an export of every row drops `pagination`). An input added later is carried by every client without any of them changing.
- **It's what the cache relies on.** A cached result is found again by its query and the time it ran as of, recompiled as the reader. That only works if the reader's request carries that time, which `input` guarantees.

Access is never in `input`: user attributes and data restrictions come from who is asking, so replaying an `input` can't widen what a caller sees. The design is in [a query's input](../designs/query-input.md#why-it-is-kept-together).

## Where it stands

| Piece | Where | State |
|---|---|---|
| anfra-node takes and returns `seeds`, and returns `cacheKey` | holistics-core, `apps/anfra-node` | Done: anfra-node 0.14.0 |
| `input` holds the page, the time zone and the seeds; the inputs beside it deprecated; answers return `input` | anfra, `internal/command/query` | Done |
| The CLI's flags for nested fields, and its name matching | anfra, `internal/command/args.go`, `cmd/anfra` | Done |
| The renamed types: `QueryDefinition`, `QueryInput`, `QueryRequestParams`, `QueryExportRequestParams` | anfra, `internal/command/query`, `engine` | Done |
| The SDK carries `input` in the API's shape, and replays the input a run came back with | anfra, `web/sdk` | Done |
| anfra-cloud on the renamed engine types | anfra-cloud, `internal/bi/enginehost` | Done on its branch; builds against anfra's checkout, waits for the anfra release |
| A refused name suggests the one meant | anfra | Not started: an enhancement, after |

## The shapes, by surface

### The core API (HTTP, MCP)

Not breaking. The old inputs keep working, deprecated.

**The request.** `page`, `page_size` and `timezone` move into `input`, as `pagination` and `timezone`; `input.seeds` is new. All of `input` is camelCase ([its case](../designs/query-input.md#its-case)).

```yaml
# Before
query: explore { … }
dataset: ecommerce
input: { filters: […], conditions: […], sorts: […], dateDrills: […] }
page: 2
page_size: 100
timezone: Asia/Ho_Chi_Minh

# After
query: explore { … }
dataset: ecommerce
input:
  filters: […]
  conditions: […]
  sorts: […]
  dateDrills: […]
  pagination: { page: 2, pageSize: 100 }
  timezone: Asia/Ho_Chi_Minh
  seeds: { referenceTime: 2026-11-12T09:41:27Z }
```

- **The old inputs** (`page`, `page_size`, `timezone`, and `timezone` on `query.export`) are `deprecated: true` in the schema, and moved into `input` before the command runs. A request that sets one both ways takes `input`'s.
- **`query.export`** takes the same `input`. With `input.pagination`, the file holds that page; without, every row, as before.
- **Refusals** name what the caller sent: `input.pagination`, or `page_size` when the page came the old way. anfra-node's refusal of a pivot's page is now on `input.pagination` (was `page_size`); of a seed, on `input.seeds.referenceTime`.

**The answer.** `input` comes back as the query ran, every default filled in: the first page made explicit, `Etc/UTC` when no time zone was sent, and the `referenceTime` anfra-node compiled with. Absent for a SQL query.

| Command | Where |
|---|---|
| `query`, as a page | `input`, beside `aql` |
| `query`, as a stream | `input` on the `compiled` event, before any row |
| `query compile` | `input`, beside `aql` |
| `query export` | Not returned: its answer is the link |

**The OpenAPI document.** The components are renamed with the Go types (below): `QueryRunInput` is `QueryRequestParams`, `RunInput` is `QueryInput`, `ExportInput` is `QueryExportRequestParams`, the query's text and target are `QueryDefinition`; `Pagination` and `Seeds` are new. What goes over the wire doesn't change with the names; a client generated from the document gets new type names.

### The CLI

Not breaking.

| Before | After |
|---|---|
| `--page 2`, `--page-size 100`, `--timezone …` | `--input.pagination.page 2`, `--input.pagination.page-size 100`, `--input.timezone …`. The old flags work, shown as deprecated, and go with the API's old inputs |
| (none) | `--input.seeds.reference-time …` |

- **An object's nested fields are flags of their own**, at any depth, named by their path in hyphens: a camelCase field's name too (`pageSize` is `page-size`).
- **A flag matches ignoring case, `_` and `-`:** `--input.pagination.pageSize` reaches `--input.pagination.page-size`. Only the hyphenated name is shown. Two fields in one place that differ only so are refused when a command's flags are built.
- **A refusal on a nested field** names it as the CLI does: `--input.pagination`.

The rule behind both is [one name per field](../designs/commands-and-api.md#one-name-per-field).

### The engine's types (platforms embedding the engine)

Breaking, at compile time, for a platform using `engine.QueryInput`.

| Concept | Before | After |
|---|---|---|
| The query: its text, language, and dataset or data source | `engine.QueryInput` | `engine.QueryDefinition` |
| The request's `input` | (not exported; `RunInput` inside) | `engine.QueryInput` |
| `core.query`'s and `core.query.compile`'s input | `engine.QueryRunInput` | `engine.QueryRequestParams`; the old name kept as a deprecated alias |
| `core.query.export`'s input | `engine.QueryExportInput` | `engine.QueryExportRequestParams`; the old name kept as a deprecated alias |

`engine.QueryInput` changes meaning, so it can't be an alias: a platform reading it as the query (`in.Dataset`, `in.Lang`) fails to compile, rather than reading the wrong thing. `QueryRequestParams` embeds `QueryDefinition`, so a field once reached as `in.QueryInput` is `in.QueryDefinition`. anfra-cloud's three uses (`internal/bi/enginehost/ops.go`) are changed on its branch.

### The SDK

Not breaking for a Data App: its API (`createQuery`, `fetchMore`, results and their `meta`) is the same. Breaking for the SDK's extension points: a `Backend` implementation, and a plugin that reads `kernel.queryRequest` or calls `core.submitQuery` itself. None outside the SDK exists in anfra, anfra-cloud or holistics-core.

| Type | Before | After |
|---|---|---|
| `QueryInput` | The transforms: `filters`, `conditions`, `sorts`, `dateDrills` | The whole input, the API's shape: the transforms, and `pagination`, `timezone`, `seeds` |
| `BackendQueryRequest` | `dataset`, `aql`, `input` (the transforms), `page`, `pageSize`, `timezone`, `bustCache` | `dataset`, `aql`, `input`, `bustCache` |
| `BackendExportRequest` | `dataset`, `aql`, `input` (the transforms), `timezone`, file options | `dataset`, `aql`, `input`, file options |
| `BackendQueryResult` | (no input) | `input?`: the input as the query ran |
| `QueryRequest` (`kernel.queryRequest`) | `dataset`, `aql`, `input` (the transforms), `timezone` | `dataset`, `aql`, `input`, without a page or seeds |
| `QueryPagination`, `QuerySeeds` | (none) | New |
| `@holistics/anfra-sdk/api`'s `toQueryInput`, `toExportInput` | Exported | Replaced by `toQueryRequestParams`, `toExportRequestParams` |

How a query runs:

- **A fresh run** (`execute`, a changed control, `refresh`) builds its input from the app's state, with no seeds: as of now.
- **A query keeps the input its run came back with.** Its next page sends that input with the page bumped, so a page fetched after midnight continues the same last 7 days; its export sends it without `pagination`, so the file is every row of the result on screen, as of when it ran.
- **The core API backend's "input as it ran"** is the input it sent, with the seeds the server generated. The rest of the server's echo is what was sent, or a default a replay needn't send; taking it whole would turn the SDK's exact types (an operator, a grain) back into plain strings.

### anfra-node's RPC (anfra's sidecar)

`aql.compile_to_sql`:

| | Before | After |
|---|---|---|
| Request | `options.referenceTime` | `seeds.referenceTime`; `options.referenceTime` is no longer read. anfra never sent it |
| Response | `sql`, `aql`, `columns`, `dataSource` | Also `cacheKey` (`v1:<sha256>`) and `seeds` (the seeds used, every one filled in) |

anfra's client sends `seeds`, reads them back, and reads `cacheKey`, which nothing uses until the cache exists. From an anfra-node that returns no seeds, anfra answers with the seeds the caller sent, if any.

## Release order

anfra must not take `input.seeds` before its anfra-node honours them: against an anfra-node that ignores `seeds`, a caller's `referenceTime` would be dropped, and the answer would still claim the query ran as of it.

1. **anfra-node:** release the version that takes and returns `seeds`.
2. **anfra:** pin that anfra-node (`manifest.yml`) in the PR with these changes, or just before it. Check once against the real binary that an answer's `input` carries a `referenceTime`, and that the query sent back with it compiles to the same SQL. Release anfra.
3. **anfra-cloud:** bump to that anfra, with its `ops.go` change, and regenerate its `web/src/api-schema.d.ts` for the renamed components.

In the SDK's release notes: the `Backend` and kernel changes above, and the two exports replaced.
