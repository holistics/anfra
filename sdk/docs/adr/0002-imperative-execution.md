# Imperative execution, not the reactive model dashboards use

> **Amended by ADR 0010** in this fork: there are no SDK-side jobs; superseding aborts the earlier Backend call's signal.

A data app executes when someone calls `execute()`. Nothing re-queries on its own. This is
deliberately the opposite of Dashboard as Code, which is reactive: changing an applied filter value
re-derives each block's viz setting and every `VizResult` independently decides to re-run, with
"refresh all" implemented as an event-bus broadcast rather than an orchestrated call.

## Why

The reactive model works because the dashboard owns both the state and the rendering, so a value
change can propagate to a component that knows how to re-fetch itself. An SDK owns neither. Its
consumer is arbitrary HTML the author wrote, and there is no component to notify — so the
propagation edge has to be a promise the author awaits.

Imperative also gives answers the reactive model leaves implicit, and an SDK cannot: what happens
when one of five queries fails, what a second execute does to the first one's in-flight jobs, and
when it is safe to read a result. Autorun becomes a policy an author opts into
(`app.subscribe(() => app.execute())`) rather than a mode the runtime is in.

## Consequences

- `execute()` resolves with `{ succeeded, failed }` and never rejects; a failed query sets its own
  `state` and `error`, so one broken query cannot blank the rest.
- Only dirty queries re-run. A second `execute()` supersedes the first, cancelling in-flight jobs
  for queries that are still dirty; the first promise resolves with what it had.
- **Superseding is per query, not per run.** Because a run only touches the queries it re-runs, a
  later `execute()` can leave an earlier run's request in flight — a forced run followed by an
  ordinary one is enough. That request still owns its query and still writes its result. Deciding
  it at run level instead strands such a query in `executing`, with nothing in flight and nothing
  left to move it out.
- State is observable through one `subscribe(listener) => unsubscribe` on the app and on each
  entity — the signature `useSyncExternalStore` wants — rather than named events or a shipped
  signal primitive.
- The two runtime models now coexist in the product. Where a concept is genuinely the same, the
  vocabulary is shared (pending vs applied conditions, interactive controls); where it is not, it is
  named differently on purpose (mapping, not interaction).
