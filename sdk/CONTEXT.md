# Anfra SDK

A framework-agnostic TypeScript SDK for building data apps: the author declares queries in AQL and
interactive controls against a semantic layer, wires them together, and renders the resulting raw
data with their own HTML/CSS/JS. Headless — the SDK returns data and state, never UI. Datasets and
query execution are owned by other contexts — reached through a provisioned **Backend** — and this
one owns declaration and runtime. Forked from the Holistics Data App SDK (ADR 0009).

## Language

**App**:
An entity graph — queries, interactive controls, and the mappings between them — plus the runtime
that executes it. Created by `createApp`, and client-declared: the SDK never persists one.
_Avoid_: "dashboard" — an app has no layout; "data app" — that is the stored artifact that may
construct one, and it belongs to another context.

**Declaration**:
The portable part of an app: its entities and their mappings, and nothing that varies by reader.
Constructing an app declares it; no reader-specific value and no I/O belongs to that moment. What a
reader has chosen is runtime state — a condition on the control it was set through, or the app's
selection when they picked it out of a query's rows.

**Data App**:
The product artifact a user creates, names and shares: an HTML document — a file in an anfra Data
Folder, or a row in Holistics — run inside a sandbox. It is *not* the persisted form of an App — it
is a document whose code may construct zero, one or several Apps at runtime. (Cross-reference —
owned by the Data App authoring context, not here.)

**Environment**:
Everything an app needs that its declaration cannot state: the dataset descriptors, who is reading,
what the tenant allows, and the backend queries go to. Held by the SDK instance, never by the app, so
that a declaration stays portable and a host can run the same one against different environments.
Timezone is not part of it — an app declares its own, and different apps in one environment may
report in different zones.

**Provisioner**:
Whoever builds the SDK instance and hands it to the author's code — the page hosting a Data App,
such as anfra's Shell. Author code never provisions; it receives an SDK that is already
provisioned.

**Backend**:
Where a provisioned SDK sends queries and field-suggestion lookups, supplied by the provisioner:
two calls, each resolving or rejecting. The SDK knows nothing about the server behind it — anfra,
Holistics, or a bridge to either — and does no polling of its own (ADR 0010).
_Avoid_: "transport", "API", "server" — those name one way of being a backend.

**Query Input**:
The structured additions a query carries on each run: the filters and conditions reaching it
through mappings and cross-filters, the reader's sort, and its date drills. Sent beside the AQL,
never spliced into it; the backend decides how to apply them. anfra rewrites the AQL with them
(the root `CONTEXT.md` defines it as anfra applies it).

**Execution options**:
Per-run settings that are not AQL: page, page size, timezone, and whether to skip a result cache.
anfra applies them when it compiles the query (the root `CONTEXT.md`).

**Dataset descriptor**:
The metadata for one dataset — its id, models, fields, and metrics. Part of the environment: it
resolves the unames a declaration refers to. Neither the SDK nor the app fetches it. The SDK
declares this type itself rather than borrowing one, so a provisioner needs no particular metadata
library to build it.

**Capability**:
Something the tenant's configuration allows or forbids, which no declaration can state and the
author cannot discover. Provisioned with the rest of the environment so that a declaration relying
on a disabled capability fails at the declaring line rather than running and quietly doing nothing.
Absent means the host did not say and nothing is blocked; only an explicit refusal throws.
_Avoid_: "feature flag" — this is the narrow subset of toggles whose absence would otherwise be
silent on this path, not a mirror of the toggle list.

**User**:
The reader's identity, as the app sees it: who they are, their reporting timezone, and what they are
allowed to see. Part of the environment, provisioned alongside the dataset descriptors and never
fetched, so author code greets someone without awaiting anything.
Its permissions **advise, they do not authorise**. Every query is re-authorised by the backend as
this person, so an app that ignores them renders a control that returns blank values rather than one that
leaks. Its timezone is likewise available but is not a default: an app that declares no timezone
still lets the backend apply its own default.
_Avoid_: reading it as the set of things the app may do; treating it as the reader's full record —
it is a deliberate subset of what the provisioner holds.

### Declaration

**Query**:
A named declaration of a dataset and an AQL body. That is the whole of it. The SDK does not parse
the AQL, so everything the language can express — grouping, aggregation, query-local expressions,
the query's own filters — is the author's to write and opaque to this package.
Paging is the one thing declared beside the AQL, because turning a page is a viewer interaction
rather than part of what the query means.
_Avoid_: "viz", "block", "widget" — all imply a rendering this context does not own.

**Field reference**:
How a control mapping, a filter or a selection names a field: `model.field` for a model field, a
bare `name` for a dataset-level metric. This is the grammar for naming a field *to* the SDK. It is
not how a query names one — a query says nothing to the SDK about its fields.

**Shape**:
Not a concept here. What a result looks like is whatever the AQL produced.

**Interactive control**:
Umbrella term for the inputs that steer an app's queries: today a filter or a date drill control.
They are one mechanism — a named condition that maps onto queries — and differ only in the operators
they produce. A cross-filter is *not* one of them: its source is a query rather than an input, and it
carries many conditions rather than one.
_Avoid_: "control" unqualified; "filter" as the umbrella (a filter is one kind of control).

**Filter**:
An interactive control holding a value condition. Its **source** is either *field-backed* (options
come from a model field, fetched on demand from the backend, which filters them by the reader's
permissions) or *manual*
(the author supplies a value type, and either a static option list or none, meaning free input).

**Aggregated filter**:
A condition whose field is aggregated before it is compared: `sum(orders.amount) > 100` rather than
`orders.amount > 100`. Carried as an `aggregation` on a **control mapping**, never on the
**condition** — the edge decides whether a condition meets the column or an aggregate over it, so
one control can do either depending on the query it reaches. The aggregated field need not be one
the query selects.
_Avoid_: "HAVING filter" — that names a SQL clause, and whether a backend compiles to it is the
backend's business (anfra does; Holistics' viz path does not). "Filter on metric" means something
else: a metric carries its own aggregation, so a filter on one is not an aggregated filter.

**Date drill control**:
An interactive control holding a date granularity. Sugar over the same condition model, its own kind
for the sake of a usable typed API. Reaches its queries as a **date drill** in the Query Input — it
redraws the mapped field's dimensions at a grain rather than filtering rows. Requires a capability;
without it the grain would silently never change.

**Condition**:
A control's value: an operator, its values, and an optional modifier. The same shape whether it
expresses a filter or a date grain.

**Mapping**:
A directed edge in the app's entity graph. The word covers both kinds: a *control mapping* and a
*cross-filter*. Always explicit — never inferred, and never generated for every pair that happens to
share a dataset. An entity with no mappings is legal and affects nothing.
_Avoid_: "interaction" — that term belongs to Dashboard as Code, where it also covers drillthrough
and linked filters. This context has cross-filtering; it has neither of those.

**Control mapping**:
A mapping from one interactive control to one query, naming the field the control's condition applies
to and an optional aggregation. Never inferred from the control's own field: "the control happens to
name a field this query also has" is the coupling that breaks silently when a dimension is renamed.

**Cross-filter**:
A mapping from one query to another, saying the first may filter the second. One concept with two
representations — the declared edge, and the *selection* that is its runtime state.
Carries no field: which fields it conditions is decided when the reader picks rows, not when the edge
is declared. Both directions may be declared, and that is not a cycle — one selection is live at a
time and never cascades. A query never cross-filters itself, because a source keeps every row so the
app can show which are selected.
_Avoid_: "drillthrough" and "linked filter" — Dashboard as Code concepts this context does not have;
"cross-filter" for the runtime state, which is a selection.

### Runtime

**Result column**:
One column of a query's result, described by metadata the backend supplies — because the SDK never
read the AQL that produced it. Classified adhoc-first and then against real dataset fields, the same
order a filter's target resolves in, which is what lets a mapping or a cross-filter know what it can
target.
Its **row key** and its **field name** are tracked separately and are not always the same: a query
that aliased a field or transformed a dimension gives the column a key of its own, while the field
name stays the real one. The key is how the author reads a value out of a row; the field name is
what a condition must say to be understood by another query.

**Adhoc column**:
A result column that is a query-local AQL expression rather than a field the dataset defines. The
SDK neither declares nor names one — the term survives because an adhoc column *cannot leave its
query*: a condition naming it would arrive somewhere that has never heard of it, so it is excluded
from cross-filter selection.
_Avoid_: "adhoc field" as something declared here — that was an earlier design, now absorbed into
the AQL body; "custom field" — a dataset-level field a modeller defines once, which outlives any
query.

**Pending condition** / **applied condition**:
A control's edited-but-uncommitted value, versus the value its mapped queries last ran with.
Executing the app commits pending to applied. Before an app's first execution nothing is applied,
so nothing is dirty — a control set from a URL at startup is not an unapplied change.
The selection has the same duality and the same execution commits it.

**Selection**:
What the reader has picked out of one query's results, and the conditions it puts on that query's
cross-filter targets. Dimensions only — a measure's value describes a bucket rather than identifying
it, so naming one is refused outright, while a measure column nobody named is simply passed over.
App-scoped and singular: picking rows in a second query replaces it, so clearing it leaves nothing
selected anywhere. Held on the app rather than on a query because it belongs to none of them; it is
the one piece of reader state with no natural owner, and every interactive control still owns its own
condition.
The SDK never sees the click. The author's own chart handler passes the rows it rendered.
Reading it back for a highlight matches on the values the selection was derived from, not on the row
objects handed over: a source is never filtered by its own selection, but it does re-run when it
stops being another query's target, and comes back with new rows for the same data.

**Lossy selection**:
A selection whose conditions do not say exactly what the reader picked. The collapse case — several
dimensions ANDed field by field, admitting combinations nobody chose — is not lossy, because it is
sent as a generated AQL expression instead. What remains is a dropped adhoc column, which no
condition of any shape can name in the target. Reported rather than hidden, because otherwise it
reads as a data bug.

**Dirty**:
A query whose effective input differs from the input of its last successful execution. Only dirty
queries re-run.
Effective input is everything that changes the rows — the query's declaration, the conditions
reaching it through control mappings, the selection reaching it through a cross-filter, the app's
timezone — plus its sort and page, which change the rows returned without re-querying the warehouse.
Note the asymmetry between asking and sending: dirtiness is computed from **pending** values, so a
query goes dirty the moment a reader edits a control, while the payload that eventually goes out
carries the **applied** ones.

**Execution**:
The app's only source of I/O apart from loading a control's options. Executing runs every dirty
query, and resolves with which succeeded and which failed rather than rejecting — one failed query
must not blank the rest.

**Result**:
A query's data: rows, the metadata describing each column, and where the result came from. Never a
rendered chart, and never display strings — formatting is the author's.

**Provenance**:
Where a result came from: the AQL and SQL that ran, whether the rows came from cache, and when the
run that produced them happened. The two query texts are present only for a reader permitted to see
generated SQL, so their absence means *not permitted* rather than *there was none*.

**Filter state**:
Reserved for the shareable, persisted control state a Data App will one day carry — not the values
a running app holds. (Cross-reference — the concept is owned by Dashboard as Code, where
it is the `_fstate` URL parameter.)
