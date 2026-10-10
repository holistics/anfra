---
name: build-data-app
description: Build or change an Anfra Data App — one HTML file in the repo's `apps/` that queries datasets through the `Anfra` SDK global and renders them with its own HTML, CSS and JS — then check it in `anfra serve`. Use for a dashboard, report, page or interactive analysis over the repo's datasets.
---

# Building a Data App

A **Data App** is one HTML file under the repo's `apps/`. `anfra serve` lists it by its
`<title>` and runs it in a sandboxed frame where the SDK is already on the global `Anfra`. The SDK
supplies data and state; every pixel is yours.

References, read when a step needs them:

- [references/api.md](references/api.md): every SDK call, the result shape, errors. **Read it
  before writing any code; do not invent APIs.**
- [references/example.html](references/example.html): a complete app. Start a new file from its
  skeleton, or from the closest app already in `apps/`.

## 1. Learn the datasets

```sh
anfra show                 # every dataset: models, fields (dimensions, measures), metrics
anfra show <dataset fqn>   # one dataset
```

The fqn is what a query's `dataset:` takes. A field is `model.field` (`orders.created_at`), a
dataset metric its bare name. A measure is already aggregated: select it as it is, never wrap it
in `sum()`. Read labels and descriptions: they carry the modeler's caveats on grain, fan-out and
which date to use. For a metric's definition, read its `*.dataset.aml`.

If a number the app needs has no field or metric, add it to the AML with the [](../aml/) skill
rather than computing it in JavaScript.

## 2. Agree on the design

Restate the request as one result the user can check ("revenue for the last full week against
the week before, with a country picker"). Ask only what changes the numbers: what a business word
means, which date counts, what to exclude, each with your recommended answer.

Then write the declaration list: every `createQuery` (dataset, dimensions, measures),
`createFilter`, `createDateDrill`, `mapControl` and `mapCrossFilter`, with real field names, and
how each query renders (chart or table; default ECharts 6 from jsDelivr, as in the example). Show
it to the user and get a yes before writing the file. When the data cannot answer the request, say
so and offer the nearest thing it can.

## 3. Write and check the queries

A query's `aql` is AQL: write it with the [](../aql/) and [](../write-aql/) skills. A Data App adds
four rules, because the SDK does not run your AQL as written: it sends it with the reader's state
(filter values, the cross-filter selection, sorts, date drills, the page), which anfra applies to
the query's `explore { }` before it runs.

- **Write an `explore { }`**, optionally after query-local `dimension` and `metric` declarations.
  Any AQL expression works inside it. A pipe-style query (`orders | select(...)`) runs, but fails
  as soon as a reader touches a control, sort or date drill mapped to it.
- **Alias every dimension and measure** (`revenue: sum(orders.amount)`). The alias is the row key
  in `query.result.rows`, and what `sorts` and `setSort` name.
- **No limit.** Paging is the SDK's (`pageSize`, `fetchMore`).
- **Nothing a reader changes goes in the AQL text.** Static conditions go in `filters` or
  `having`; a reader's choice is a control (`createFilter` + `mapControl`), never a string built
  into the query.

Run each query before it goes in the app, so a mistake surfaces here rather than in a frame:

```sh
anfra query --dataset <fqn> '<aql>'
```

Check its headline number against a direct SQL query on the source, or a simple invariant (parts
summing to the total).

## 4. Write the file

One `.html` file in `apps/` (subfolders group apps in the list), with a `<title>` the list shows.
`apps/sales/overview.html` opens at `/apps/sales/overview`. Follow the example's skeleton: declare
everything, `subscribe` a render function, `execute()`, then wire controls and clicks to set state
and `execute()` again.

The frame is a **sandbox** (`allow-scripts` only, opaque origin). Write for it:

- Data comes only from the SDK. Don't call anfra's API yourself: the frame cannot reach it.
- `localStorage`, `sessionStorage` and cookies throw: keep state in variables.
- Forms, popups and navigation are blocked: use buttons and `change` listeners, never `<form>`.
- Load libraries from a CDN, with pinned versions. Files next to the app in `apps/` (images,
  scripts) load by relative path.
- Every query is an `explore { }`: controls, cross-filters, sorting and date drills only apply to
  one.
- Mark the page's structure in the HTML, as [references/api.md](references/api.md#structure)
  shows: `data-anfra-container` and `data-anfra-block` on the parts of the page, with a
  `data-anfra-label`, and `data-anfra-query` / `data-anfra-control` on the elements each query
  and control draws into. The Inspect panel shows the tree, finds each part on the page, and
  gives the user a handle for it to hand back to you. Mark every part; it is one attribute each.

Render a selection without rebuilding the chart. A query that drives a cross-filter is not re-run
by its own selection, so its result is unchanged when the reader clicks: restyle the marks it
already draws (dim the unselected ones) rather than recreating the series, or the chart replays
its entry animation on every click. With ECharts, merge with a plain `setOption(option)` for a
selection change and keep `replaceMerge` for a new result.

Render each query's four states: `executing` (loading), `success`, `error` (show
`error.message`), and success with no rows ("No sales this week", never a bare zero). Log
`result.columns`, `result.rows[0]` and `result.debug?.executedAql` on each query's first success:
the real row keys, value types and the AQL that ran. Put on the page the definitions, dates and
any rounding a reader needs to trust a number.

## 5. Run and check it

```sh
anfra status               # is a server already running for this repo, and where?
anfra serve                # if not: http://127.0.0.1:7878/apps/ (or the free port it prints)
```

Leave it running: it reloads the open app whenever its file, or the AML, changes. Its header shows
the repo's problems (AML that does not compile, a data source not configured). Its Inspect panel
has two tabs: **Structure**, the tree of marked parts, with a problem count for markup it could
not read (an unknown query name, a block inside a block), and **Data**, each query's state,
executed AQL and error, with how many places draw it and a Locate button. A query "not on the
page" is one no element is marked with yet.

If you can drive a browser, open the app, exercise every control and click, and read the console
of the app's frame, not just the page. Otherwise ask the user to, and to report what they see
against what they expected, with that console. Read errors against
[references/api.md](references/api.md#errors): a `ValidationError` names a declaring line; a
`QueryError` carries anfra's message about the AQL, or names the bad entry (`filters[0].field`).
Compare a misbehaving query's `executedAql` with what you meant, and fix the smallest thing that
explains it. If you could not check the interface, say so rather than claiming it works.

## 6. Hand over

Tell the user, in their terms: what the page shows and how to use each control, its URL, the
assumptions you made, and what you checked. When a later request names a part by its handle,
`[Revenue over time](data-anfra-block="trend")` or a bare `data-anfra-block="trend"`, search the
file for that attribute: it is the element the user means. A `127.0.0.1` URL opens only on this machine; `anfra
serve` has no authentication, so ask before serving it beyond loopback (`--addr`). Make each
requested change the same way: restate it if it changes what a number means, change the smallest
thing, and check that the agreed result still holds.

## Related skills
* [](../aml/) — add the fields and metrics an app needs.
* [](../write-aql/) — write and validate the queries.
* [](../setup-repo/) — set up the repo and its data source first.
