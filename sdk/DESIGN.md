# Anfra SDK — design

Headless TypeScript SDK for declaring and running data apps against a semantic layer, through a
Backend the provisioner supplies. The author writes HTML/CSS/JS; the SDK supplies data and state,
never UI. Forked from the Holistics Data App SDK (ADR 0009).

Vocabulary is defined in [CONTEXT.md](./CONTEXT.md). Decisions with lasting consequences are in
[docs/adr](./docs/adr).

Note the two nouns. An **App** is what `createApp` returns: a runtime entity graph, owned here. A
**Data App** is the stored HTML document a user authors and shares, owned by the Data App authoring
context — its code may construct zero, one or several Apps. They are not the same thing at two
lifecycle stages.

## Surface

```ts
// Provisioner — the page hosting the Data App. Never seen by the author.
import { createSdk, installSandbox } from 'anfra-sdk'      // or the IIFE bundle's `AnfraSdk`
const sdk = createSdk({
  datasets,                                  // Record<uname, DatasetDescriptor>
  user,                                      // who the app runs as; see Model below
  features: { dateDrill: true },             // what the tenant allows; see ADR 0004
  backend: {                                 // where queries go; see Backend below
    submitQuery: (request, signal) => myServer.query(request, signal),
    fieldSuggestions: (request, signal) => myServer.suggest(request, signal),
  },
})
installSandbox(sdk)                          // assigns the `Anfra` global

// Author / agent — `Anfra` is a provisioned global, declared in a shipped ambient .d.ts
const app = Anfra.createApp({ title: 'Sales Overview' })   // timezone optional

const revenue = app.createQuery('revenue', {
  dataset: 'sales',
  aql: `
    explore {
      dimensions { month: date_trunc(orders.created_at, "month") }
      measures { total: orders | sum(orders.amount) }
      filters { orders.status is "completed" }
    }
  `,
})

const byRegion = app.createQuery('byRegion', {
  dataset: 'sales',
  aql: `
    explore {
      dimensions { region: users.region }
      measures { total: orders | sum(orders.amount) }
      sorts { total desc }
    }
  `,
})

const region = app.createFilter('region', { field: 'users.region' })
const grain  = app.createDateDrill('grain', { default: 'month' })

// Mappings are always explicit: the field is never inferred from the control's own field.
app.mapControl(region, revenue,  { field: 'users.region' })
app.mapControl(grain,  revenue,  { field: 'orders.created_at' })

// Cross-filtering names no field — which ones a click conditions is decided by the rows picked.
app.mapCrossFilter(byRegion, revenue)

// The SDK never sees the click; the author's own chart handler hands over the rows it drew.
chart.on('click', (bar) => { byRegion.select([bar.row]); app.execute() })

app.subscribe(render)
await app.execute()
```

The host installs the provisioned instance with `installSandbox(sdk)`, which assigns the
`Anfra` global and returns an uninstall. `ambient.d.ts` declares it for the sandbox's
TypeScript config, and is the single artifact an agent reads to learn the surface.

## Model

Three layers, kept separate on purpose — `VizSetting` conflates all three and is the reason it is
never on the wire (ADR 0001).

| Layer | Owns | Where it lives |
| --- | --- | --- |
| Environment | datasets, user, features, backend | the SDK instance (ADR 0003, ADR 0010) |
| Declaration | queries, controls, mappings, timezone | the app; serialises via `toJSON()` |
| Runtime | conditions, results, state | the entity handles |

**Query** — a dataset and an AQL body, plus an optional `pageSize`. Nothing else. The SDK does not
parse the AQL, so grouping, aggregation, query-local expressions and the query's own filters are all
the author's to write and opaque here (ADR 0001). Sort is a runtime override rather than part of the
declaration, matching how a column-header sort behaves on a dashboard: it changes the rows returned
without re-querying the warehouse.

**User** — who the app runs as: `id`, `name`, `email`, `role`, `timezone`, and
`permissions { canViewGeneratedSql, canExportData }`. Required at construction and provisioned like
`datasets`, so author code needs no await and no null check. The permissions advise rather than
authorise — every query is re-authorised by the backend as this person, so an app that ignores them
renders a control that returns blanks, not one that leaks. An explicit allowlist, not a projection
of the host's user record. `timezone` is available but defaults nothing: an app declaring no
timezone still lets the backend apply its own default.

**Features** — backend capabilities the host knows about, so a declaration that depends on a
disabled one throws at the declaring line instead of running and quietly doing nothing. One key
today, `dateDrill`. Absent means the host did not say and nothing is blocked (ADR 0004).

**Interactive control** — filter, date drill. One condition model, a constructor per kind for the
sake of a typed API. A filter's source is either field-backed (options fetched lazily through
`backend.fieldSuggestions`, permission-filtered by the backend) or manual (a static option list, or none,
meaning free input).

**Mapping** — a directed edge, of two kinds, in one tagged `toJSON()` list (ADR 0005).
- `app.mapControl(from, to, { field, aggregation? })` — a control conditioning one field of one
  query. Always explicit; never inferred from the control's own field. Ids are `from.to.field`.
  With an `aggregation` the condition applies to the aggregate, not the column.
- `app.mapCrossFilter(from, to)` — one query may filter another. No field: the click decides.
  Same dataset, no self-edge, both directions allowed. Ids are `from.to`.

Neither is ever generated for you. An entity with no mappings is legal, and there is no `unmap()`.

**Result column** — described by metadata the backend supplies, since the SDK never read the AQL.
A column's row key and its field name are tracked separately: a query that aliased a field or
transformed a dimension gives the column its own key, while the field name stays the real one. The
key reads a value out of a row; the field name is what a condition must say to be understood
elsewhere.

**Selection** — `query.select(rows, { fields? })` replaces the app's one selection and
`app.clearSelection()` empties it; `app.selection` and `query.selectedRows` read it back for
highlighting. Dimensions only. Conditions are collected per field and ANDed; when that would admit
combinations nobody picked, one generated AQL expression goes instead, saying exactly what was
chosen (ADR 0005, ADR 0006). `selection.lossy` reports what no condition can express — a dropped
adhoc column.

## Runtime

- `execute()` is the only required I/O. It commits pending conditions to applied, runs every dirty
  query, and resolves `{ succeeded, failed }` without ever rejecting.
- Effective input = the query's declaration + the conditions reaching it through control mappings +
  the selection reaching it through a cross-filter + the app's timezone + its sort and page. Only
  queries whose effective input changed re-run; nothing is dirty before the first execution.
  Dirtiness is computed from pending values; the payload that goes out carries applied ones.
- A second `execute()` supersedes the first, aborting the signal of each still-dirty query's
  in-flight backend call; a stale result that arrives anyway is ignored. `app.abort()` /
  `query.abort()` are explicit. `refresh()` is `execute({ force, bustCache })`.
- All dirty queries submit in parallel, one `backend.submitQuery` each. Whether the backend answers
  synchronously or polls jobs is its business; the SDK only awaits. No SDK-level timeout — a slow
  warehouse query is not an SDK error.
- `subscribe(listener) => unsubscribe`, on the app and on each entity.
- `query.result` is `{ columns, rows, meta: { page, pageSize, numRows }, debug? }`. One page per
  execute; `query.fetchMore()` / `query.hasMore` for the rest. Formatting is the author's: the SDK
  returns values, not display strings.
- `query.result.debug` says where the result came from — the AQL and SQL that ran, whether it came
  from cache, and when. The backend leaves the query texts out for readers without permission, so
  absence means *not permitted* (ADR 0008).

## Backend

The provisioner's `backend` has two methods, each taking an `AbortSignal` and returning a promise
(ADR 0010):

- `submitQuery({ dataset, aql, input, page, pageSize, timezone?, bustCache? })` resolves with
  `{ columns, values, meta?, debug? }`. `input` is the **Query Input**: `filters` and `conditions`
  from control mappings and cross-filter selections, the reader's `sorts`, and `dateDrills`. A date
  drill is never sent as a filter. `values` is positional, lined up with `columns`; the SDK keys
  rows by column name and fills in paging metadata the backend leaves out.
- `fieldSuggestions({ dataset, model, field, q })` resolves with the values a filter can offer.

The SDK sends the Query Input as data and never splices it into the AQL. anfra applies it by
rewriting the query's `explore { }`; another backend may do it however it likes.

## Errors

Four classes, each naming the entity that caused it. Resist adding a fifth — an error taxonomy is a
public API.

- `ValidationError` — thrown synchronously at declaration. What it covers is narrow, because a query
  is unparsed AQL: an unknown dataset, an unknown field on a *filter* or a named selection column, a
  mapping the graph refuses, or a declaration needing a capability the tenant has disabled. Anything
  inside the AQL surfaces at execute instead.
- `QueryError` — stored on the query, never thrown. Carries the backend's diagnostics, which is where
  an AQL syntax error or unknown field arrives. Any backend rejection that is not already a
  `DataAppError` becomes one.
- `PermissionError` — separate from `QueryError` so apps can render "you can't see this"
  differently from "this broke".
- `TransportError` — network, auth expiry, for a backend that reports them as such.

## Out of scope

Built-in charts · `<h-block />` · drill-down · view underlying data · drillthrough · export ·
shareable filter state · persistence of an app's declaration · `unmap()` · UI binding helpers.

A PoP control, deliberately: the dashboard PoP block is being sunset, so the SDK does not grow a
surface for it. The `transform_pop_*` operators stay in the `Operator` union, which mirrors the
published discriminator rather than the subset this package constructs — an author who needs the
comparison before the sunset lands can write the condition out.

Within cross-filtering: more than one selection at a time, cross-dataset selections, measures as
selection conditions, and cascading past one hop.

Authoring-time AQL validation. `checkAql` from `@holistics/amql` already runs in the browser and is
the intended follow-up; until it lands, a malformed query is a `QueryError` at execute rather than a
`ValidationError` at the declaring line (ADR 0001).
