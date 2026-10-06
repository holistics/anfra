# Cross-filtering is its own edge kind, not a mapping to a control

A cross-filter is a directed edge from one query to another, declared with `app.mapCrossFilter` and
carrying no field. Its runtime state is the app's single **selection**: the rows a reader picked out
of one query, and the conditions those rows put on that query's targets.

## Why a generic edge did not work

An early version had one generic `{ from, to }` mapping shape, on the reasoning that cross-filtering
would later be a `query -> control` edge through it. That was written before anyone looked at how
cross-filtering behaves.

Dashboard as Code settles it. `CrossFilterInteraction` is the only one of its five interaction kinds
without a `field_path`, and one click produces several conditions at once — a single table row turns
into three. A control mapping is one control, one query, one field, one condition. A cross-filter has
no field, has many conditions, and its source is a query. Every part of the invariant breaks, and
`query -> control` was never going to work regardless, because a control holds exactly one condition
and has nowhere to put the rest.

Making the field optional was the tempting repair and is the worse one: an optional field asks every
consumer to infer the kind from which properties happen to be set, where a tag makes them branch on
it. The list stays single — matching `definition.interactions`, which carries all five Dashboard as
Code kinds discriminated by `type` — and `MappingJson` is a tagged union.

## Considered and rejected

**A separate `crossFilters` list in `AppJson`.** Diverges from the shape Dashboard as Code
persists, for no gain over a tag.

**Overloading `app.map` on the source type.** This is how AML spells it — a cross-filter is a
`FilterInteraction` whose `from` names a viz block, recognised by the *absence* of `field:`.
Rejected for authoring by agents: an agent that has seen `map(control, query, { field })` will
supply a field for the query-source call, and the resulting "no overload matches this call" is a
known flailing loop. `mapControl` / `mapCrossFilter` keeps the shared `map` prefix, so completion
still clusters them, while naming the concept where it can be searched for.

**Automatic edges for every same-dataset pair,** as Dashboard as Code generates. DaC can afford it
because a GUI shows every edge and a checkbox tree prunes them. An author who adds a sixth query
should not silently acquire five edges they cannot see.

**Landing the selection in an existing declared filter.** One filter holds one condition, and it
would make a click indistinguishable from a choice the reader made in a dropdown.

## What a selection sends, and when the structured form gives up

Conditions are collected field by field and ANDed. That is the proven path — smaller payloads,
stable cache keys — and it says exactly what the reader picked for every single-dimension selection
and every contiguous date range.

It cannot say everything. A flat AND of independent clauses has no spelling for `(A and B) or
(C and D)`, so picking `(APAC, Jan)` and `(EMEA, Feb)` collapses to `region IN (APAC, EMEA) AND
month IN (Jan, Feb)`, which also admits `(APAC, Feb)`. Nor for "these two months but not the one
between", since a discontiguous pick would have to widen into one span covering the gap.

When the collapse would over-select, the SDK sends one AQL condition instead — an OR over the rows,
saying exactly what was picked — and the selection is not lossy. AQL conditions are an existing
product feature, written in the Visualization → Condition tab and emitted by `viz_to_aql.rb` as a
fragment into `filters { }`. Nothing in the compiler needed to change; the expression simply had no
route from a public query API. How that expression is built is ADR 0006.

The two forms never both carry a selection. The structured one is tried first and stays in charge
whenever it can say what the selection means.

**`match_ranges` was the obvious fix for the date case alone, and was dropped.** It is absent from
the operator discriminator in `Operator.yml`, its `end_exclusive` travels in an `options` key no
schema in the tree has, and `VizConditionToAqlCondition` raises on it outright — AQL has no `Or` in
`InfixOperator`, so emitting it would have meant writing the very fragment escape hatch that AQL
conditions already are. It would also have fixed only dates, leaving the OR-of-tuples case exactly
as it was.

## Consequences

- **No cycle check on these edges.** `assertAcyclic` still guards control mappings, but
  bidirectional cross-filtering is the normal case — DaC generates both directions by default — and
  is not a loop, because one selection is live at a time and a target never re-propagates.
- **ADR 0002 is untouched.** A selection is set between runs, never during one, so `execute()` keeps
  its single flat pass. Cross-filtering needed no reactive propagation after all.
- **The selection lives on the app.** Not centralisation for its own sake: it belongs to no query,
  since picking rows in a second query replaces it. Controls still own their conditions.
- **Two sources of lossiness, and only one is fixable.** An expression fixes the collapse. It cannot
  fix a dropped adhoc column, which is query-local: no condition of any shape can name it in the
  target, because the target's AQL has never heard of it. `lossy` survives for that case alone.
  Conflating the two would report a genuinely lossy selection as exact, which is the one outcome
  worse than the original limitation.
- **A generated condition falls back rather than guesses.** If any value has no AQL spelling, the
  lossy structured condition stands. A condition that shows too much is recoverable; one that shows
  the wrong thing is not.
- **Selection values go over the wire as they came back.** The SDK stringifies nothing and coerces
  nothing — a value that arrived as a number leaves as one.
- **Measures never condition anything,** matching DaC. A measure's value describes a bucket rather
  than identifying it.
- **A source is never filtered by its own selection**, so it keeps every row and the app can show
  which are selected. It does re-run when it stops being another query's target, and comes back with
  new rows for the same data — which is why reading a selection back for highlighting matches on the
  values it was derived from rather than on the row objects handed over.
- **Not persisted or shareable.** Same as DaC, where cross-filter state is absent from `_fstate`.
  `Filter state` in CONTEXT.md stays reserved.
