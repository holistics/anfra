# A query is a dataset and a raw AQL body

> **Amended by ADR 0010** in this fork: queries no longer go to Holistics endpoints; the SDK hands them to a provisioned Backend.

`app.createQuery(name, { dataset, aql })`. The author writes AQL; the SDK never parses it. Execution
goes through `POST /api/v2/data_sets/submit_execute_aql`, which compiles the AQL to a `VizSetting`,
appends the conditions the app's controls and selection contribute, and runs the result through the
same resolver pipeline a dashboard uses.

Three things follow from that one sentence, and they are the whole of this decision: the query is
opaque to the SDK, the filter overlay is applied after compilation rather than by editing text, and
`VizSetting` — the thing being compiled to — never appears on the wire or in this package's types.

## Why AQL, when structured declaration was tried first

The first cut declared queries structurally: `dimensions`, `measures`, `adhocFields`, `filters`,
`sort`, `shape`. It was chosen over AQL text for two specific reasons, and it is worth recording
that both were real at the time and both were answered rather than waved away.

**"Injecting filter conditions into author-written `explore { }` needs AST surgery."** True, if the
text is what you edit. It is not: the AQL is compiled to a `VizSetting` first — reusing
`Mcp::Services::AqlToVizSetting`, the same compiler the AI agent's `execute_aql.rb` path uses — and
a condition is appended to the structured `filters` array that compilation produces. That array's
resolution logic already resolves a field name against `adhoc_fields` first and then against real
dataset fields, and already supports filtering on a field the query does not select and on an
aggregate rather than a column. Appending to it needed no new compiler capability. A hash append,
not surgery.

**"The only shipping AQL-over-HTTP path skips row-level permissions."** Also true of
`POST /adhoc_queries` under `AQL_SQL_EDITOR`, which calls `AqlToModel` directly and bypasses
`Viz::Data::FieldResolver` entirely. This path never does that. Execution routes through
`Viz::Data::ExploreResolver`, which for a DB-backed dataset — the case every published data app
hits — calls the same `FieldResolver` plus `BuildPermissionRulesFromDataSet` pipeline that
`submit_query` does. Structural parity with the endpoint being replaced, not an argument that it is
probably fine.

With both objections answered, structured declaration had nothing left to recommend it. It was
strictly more surface for strictly less expressive power, and every gap in it — no inline ratios, no
conditional aggregates, one shape — was a gap AQL does not have.

## Considered and rejected

**Compiling to `VizSetting` client-side, and putting it on the wire.** Rejected then, and still
rejected — this is the one part of the original decision that survives untouched. `buildVizSetting`
does run in the browser today (the AI chat path uses it, and Rails shells out to a Node bundle of
that same file), so it was technically available. But `VizSetting` is a chart binding, not a query.
Its `fields` are keyed by viz-type-specific role, so copying it re-imports the exact coupling this
SDK exists to escape, and it would make `query.result` change shape when someone swapped a chart
type. `VizSetting` remains absent from this package's public types by rule, not by accident.

**Keeping structured queries alongside raw AQL.** Rejected. No author had built against the
structured form, and an agent trained to write AQL has no reason to emit a structured declaration
instead. Two authoring paths for one that is actually used is two to maintain and two to document.

**A declared `adhocFields` bag.** This shipped, briefly, and is subsumed rather than merely
rejected — so its reasoning is worth keeping, because it explains a rule the glossary still
enforces. A query declared named AQL expressions separately from selection, under the same
reference grammar every other field used: `orders.sizeBucket` an adhoc dimension attached to
`orders`, `marginPct` an adhoc metric. Separating definition from selection mattered because a
field can then be filtered on without being selected — for a metric that merely avoids an unwanted
column, but for a dimension it changes the answer, since selecting one changes the grouping grain.

Raw AQL absorbs all of it: an AQL body defines and selects in one text, and the separation the bag
provided is something the language already expresses. What survives the absorption is the
resolution order — an adhoc name is tried *first*, so a name that also resolves to a real field
would silently shadow it. That collision is rejected rather than shadowed, unchanged from when the
bag declared it; enforcement now sits in the AQL parser rather than in this package.

**Editing the AQL text to inject filters (AST surgery).** Rejected — see above. It is the problem
compiling first makes disappear.

**Extending `submit_query` / `SubmitGenerate` to accept raw AQL.** Rejected. That endpoint's
`QueryPayload` is shaped entirely around structured params, and the SDK-driven additions made to it
during the structured era — `Filter#aggregation`, `adhoc_fields`, `aql_conditions`, `debug` — have
been reverted, since nothing else adopted that surface. A dedicated endpoint owns raw-AQL-plus-
filter-overlay execution instead.

**One override mechanism for filters, sort and limit.** Rejected after checking how sort and
pagination actually behave. `Viz::Caching::Keys` excludes UI-level `sort`, `page` and `page_size`
from the cache key entirely — a column-header sort re-queries the cache store, not the warehouse.
Folding them into the same compile-then-override mechanism as filters would invent a second
cache-affecting path where the product already has a proven cache-excluded one. Sort, `pageSize` and
page navigation stay runtime parameters passed straight through, matching dashboard behaviour
exactly. `shape` is dropped as a declared concern altogether: whatever the compiled result produces
is what is used.

**Duplicating the compile-and-inject logic in a Data-App-specific service.** Rejected. If the AI MCP
tool and data apps both need "compile AQL, optionally overlay a filter, execute", that is one domain
operation with two callers, not two operations that resemble each other. The shared service is
extracted from the existing pipeline rather than reimplemented.

## Consequences

- **There is no semantic governance property, and nothing replaces it.** The structured form could
  claim every number in an app traced to a modeller-approved definition. That claim is gone: an
  author can put any AQL the dataset's permissions allow into a query. This is deliberate. An AQL
  body compiles to fields the server re-authorises as the reader, so it exposes nothing that reader
  could not already query — what is given up is provenance, not access.

- **No authoring-time validation at all.** The SDK checks that `aql` is non-empty and that the
  dataset uname resolves, and nothing else. Syntax errors, unknown fields and ambiguous references
  all surface at execute as a `QueryError`, carrying the diagnostics the server returns. This is a
  real regression against the error contract in ADR 0003, which leans on failures arriving
  synchronously at the declaring line. A Monaco-based linting layer is the intended follow-up. A
  field renamed after a data app is published breaks it silently until a viewer opens it; accepted
  as an MVP risk.

- **Result columns are classified server-side.** Since the SDK never parses the AQL, it cannot know
  what a column is. `ColumnMeta` carries `fieldName`, `modelId`, `adhoc` and `isMeasure`, resolved in
  the same order a filter's target resolves — adhoc first, then a real dataset field. This is what
  lets a control mapping or a cross-filter know which columns it can target, and it is why
  `ColumnMeta.name` (the row key, possibly an alias the AQL declared) is tracked separately from
  `fieldName` (always the real one).

- **Cross-filtering reaches only real dataset fields.** An adhoc column is query-local: a condition
  naming it would arrive at a target query that has never heard of it. Such columns are excluded
  from selection, and the exclusion is reported — see ADR 0005.

- **Aggregated filters compile through the default `filters { }` path**, as a result-field filter
  producing an outer `WHERE` over the grouped CTE. Never through the `viz_setting:amql_having_conditions`
  toggle path. This is worth stating explicitly because the naming suggests the opposite: the
  toggle-on `having { }` path has AQL-string snapshots only, while the default path has SQL-level
  and end-to-end coverage. The default is not a fallback here, it is the verified branch.

- **Window-function aggregations are refused rather than passed through.** A mapping onto a measure
  using `running sum` and its three siblings is rejected at declaration, because the compiler
  discards such a filter without signalling it. The SDK is deliberately narrower than the wire here.

- **RLS parity is verified for the DB-backed execution branch only.** `ExploreResolver`'s
  no-`db_dataset` branch — reachable only from the AML Studio working environment — does not visibly
  merge AML-native `permission { }` rules the way the DB-backed branch does. No published data app
  reaches it, but the gap is unconfirmed rather than known-safe, and is worth a direct check with the
  AI/MCP or AML Studio owners before that branch is ever exposed to a published app.
