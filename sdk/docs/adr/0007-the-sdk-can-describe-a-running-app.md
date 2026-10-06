# The SDK can describe a running app, for the host to render

> **Amended by ADR 0010** in this fork: there is no `client` or `poller` any more; `cause` holds whatever the backend threw.

`Sdk` keeps the apps `createApp` returns, and `App.toInspectJSON()` projects one into plain data:
the declaration `toJSON()` already produces, plus the runtime state it deliberately omits — control
conditions, the selection, each query's state, error and provenance.

This is the introspection API ADR 0003 said would not exist. That decision was about what an
*author* needs, and it still holds for authors. The consumer here is the host, which renders an
Inspect panel outside the sandbox where author code cannot reach it.

## Why the SDK owns the projection

The host cannot build it. Structured clone is what carries a message over the port, and it ignores
`toJSON()`, drops getters, and throws outright on functions and `AbortController`. Every useful
value on an app is behind a getter — `query.state`, `control.condition`, `app.queries` — and
`Query.controller` holds an `AbortController` for the duration of every run, which is exactly when
someone is looking. Posting live objects is not a thing that can be made to work.

So something must read each getter and write a plain object. Putting that in the bootstrap would
mean a projector reaching into SDK internals from outside, untested by the package's own suite, and
quietly wrong the day `Query` grows a field. A devtool that lies because its projector missed
something is worse than no devtool.

## What it excludes, and why

- **Rows.** An unpaged query returns every row and `fetchMore` appends, so rows are the one unbounded
  thing in the payload — and the one thing already on screen in the preview beside it. The panel
  answers *why* the data is what it is, not *what* it is.
- **`QueryError.cause`.** It holds whatever the transport threw, which can be a `Response` or an
  `AbortError`. Errors are projected field by field instead: a `DataAppError` loses its prototype
  and its own properties through structured clone, so `entity` and `status` — the useful half —
  have to be written out explicitly or they simply vanish.
- **Everything reachable but irrelevant**: `client`, `poller`, the internal `Map`s. Each is either
  uncloneable or drags the whole object graph across.

## Consequences

- **Apps outlive the author's reference.** A registry holds every app for the life of the `Sdk`.
  Acceptable here because the frame is discarded and rebuilt on every Run, which is what bounds it
  — an SDK instance that lived longer would need a `WeakRef`.
- **It is a list, not a thing.** One data app's code may construct several apps, or none. The
  payload is an array and the panel renders what it finds, including nothing.
- **`selection.lossy` gets somewhere to be seen.** It has only ever reached a `console.warn`, which
  is the wrong medium for a fact about the correctness of what is on screen.
- **The projection has to track the getters.** Adding a field to `Query` or `Control` and not to
  the projector produces a panel that is silently out of date. Its tests are the guard, and they
  live in this package precisely so they fail here rather than in the host.
- **ADR 0003 is amended, not overturned.** `Holistics.datasets` is still the whole of what an
  author gets, and there is still no `describe()` on the author-facing surface.
