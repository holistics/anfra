---
name: build-data-app
description: Build or change an Anfra Data App — one self-contained HTML file in the repo's `apps/` that queries datasets, and can trace fields' lineage, through the `Anfra` SDK global and renders them with its own HTML, CSS and JS — then check it in `anfra serve`. Use for a dashboard, report, page or interactive analysis over the repo's datasets.
---

# Building a Data App

A **Data App** is one self-contained HTML file under the repo's `apps/`. `anfra serve` lists it by
its `<title>` and runs it in a sandboxed frame where the SDK is already on the global `Anfra`. The
SDK supplies data and state; every pixel is yours.

References, read when a step needs them:

- [references/api.md](references/api.md): every SDK call, the result shape, the plugins, errors.
  **Read it before writing any code; do not invent APIs.**
- [references/plugins/](references/plugins/): one file per built-in plugin beyond `core`
  ([export](references/plugins/export.md), [render](references/plugins/render.md),
  [lineage](references/plugins/lineage.md)). Read one only when the agreed design uses that plugin,
  and check `Anfra.plugins.has(name)` before using it.
- [references/downloads.md](references/downloads.md): the download button for exports and pictures.
  Read it only when the agreed design lists a download.
- [references/example.html](references/example.html): a complete app in the default look. Start a
  new file from its skeleton, or from the closest app already in `apps/`.
- [references/DESIGN.md](references/DESIGN.md): the anfra app design guide (colors, type,
  spacing, chart palette) behind the default look. Read it when you build in the default look.

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

Also ask which look the user wants, unless they already said: the **default anfra style**
(recommended), **something creative** (you choose the palette and layout), or **their own design**
(a brand, colors or a reference to follow). See [Look and feel](#look-and-feel).

Then write the declaration list: every `createQuery` (dataset, dimensions, measures),
`createFilter`, `createDateDrill`, `mapControl` and `mapCrossFilter`, with real field names, how
each query renders (chart or table; default ECharts 6 from jsDelivr, as in the example), and which
blocks, if any, the reader can download from, one line each with its formats (`trend: CSV`). List
a download only when the user asked for one, or propose one where it plainly helps (a detail
table's data) and let them decide; none is the default. Leave pictures (PNG, PDF) to the host,
which offers them for every block and the whole page, unless the user asks for them in the app.
Show it to the user and get a yes before writing the file. When the data cannot answer the
request, say so and offer the nearest thing it can.

### 2.1 Lineage

Lineage explains and governs a metric rather than showing its value: it answers what the metric is
built from, the dimensions, measures and other metrics it depends on, and the models and source
tables behind them. Add it when the user asks for it, or when the request is about tracing, understanding
or governing a metric: "how is revenue calculated", "which tables feed this KPI", a field catalog,
a definitions panel beside a chart. An app that only shows numbers needs none.

When it is in, add to the declaration list which fields the app traces, when (on load, or when the
reader picks a field or clicks a chart), and how the diagram is drawn (see
[references/plugins/lineage.md](references/plugins/lineage.md)).

## Look and feel

An app must look finished without the user asking: native controls, default buttons and unstyled
text are not acceptable. What counts as finished depends on the look the user chose.

- **Their own design:** follow it, and skip the defaults below.
- **Creative:** pick your own palette, type and layout, and keep the basics: readable contrast,
  tabular numerals, light and dark (`prefers-color-scheme`), styled loading, empty and error
  states.
- **Default anfra style:** start from [references/example.html](references/example.html), which
  applies the tokens in [references/DESIGN.md](references/DESIGN.md) to
  [Basecoat UI](https://basecoatui.com), a plain HTML/CSS component library. Use its tokens as
  CSS variables, its three text sizes (20, 14, 12px), 32px controls, one violet primary action and
  the chart palette (violet first). Ignore what only fits the anfra web app itself: Tailwind class
  names, the sidebar, the sign-in sheet, the logo. Load Inter from the CDN with a `system-ui`
  fallback.

In every look:

- **A single HTML file uses Basecoat** (pinned CDN CSS and JS, as in the example). Use shadcn
  (`shadcn-vue` for Vue) only when the app is written in React or Vue.
- **Filters are Basecoat comboboxes**, with `aria-multiselectable="true"` for several values. Never
  a native `<select multiple>`. Basecoat has no date picker: use two styled `<input type="date">`
  and preset buttons, as in the example.
- **Basecoat `.card` is a flex column that clips overflow.** A filter bar built as `class="card
  filters"` with only `display: flex; flex-wrap: wrap` keeps the column direction, so
  `align-items: end` pushes every control to the right edge, and the clipped overflow cuts off a
  combobox's dropdown at the card's bottom edge. Override both on the bar: `flex-direction: row;
  align-items: flex-end; overflow: visible` (as `.card.filters` does in the example). Check
  the filter bar in a browser with a dropdown open before handing over.
- Charts: if a `dataviz` skill is available, follow it. Give ECharts hex colors: it cannot parse
  `oklch`.

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
summing to the total). Trace each lineage target the same way, and check its `refers` path is the
one you expect:

```sh
anfra lineage --targets '[{"parent":"<dataset fqn>","child":"<metric>"}]'
```

## 4. Write the file

One self-contained `.html` file in `apps/` (subfolders group apps in the list), with a `<title>`
the list shows.
`apps/sales/overview.html` opens at `/apps/sales/overview`. Follow the example's skeleton: declare
everything, `subscribe` a render function, `execute()`, then wire controls and clicks to set state
and `execute()` again. Keep its two layers apart: the look (styles, markup, chart theme) and the
SDK wiring.

The frame is a **sandbox** (`allow-scripts` only, opaque origin) under a **content policy**. Write
for it:

- **Data comes only from the SDK.** The frame has no network of its own: `fetch`, XHR, WebSocket
  and `sendBeacon` are blocked but to the CDNs below. Never call anfra's API or any other service
  from the app.
- **Libraries, styles and fonts only from cdnjs.cloudflare.com, cdn.jsdelivr.net, esm.sh,
  unpkg.com and Google Fonts** (fonts.googleapis.com, fonts.gstatic.com), each by its `https://`
  URL with a pinned version. A script, style or font from anywhere else is blocked. Images may come
  from any `https://` server. When the user needs another origin (their own CDN), add it to the
  repo's `.anfra/config.yml` under `apps.allow_origins` (`[https://cdn.acme.com]`, an origin with
  nothing after it), and tell them anfra Cloud keeps its own list, which an Admin edits in the
  Organization's settings.
- **Every CDN `<script src>` and stylesheet `<link>` carries its hash:** `integrity="sha384-…"
  crossorigin="anonymous"`, as in the example, so a library changed at its CDN fails to load
  rather than runs beside the reader's data. jsDelivr shows a file's hash on its page; otherwise
  `curl -s <url> | openssl dgst -sha384 -binary | openssl base64 -A` gives what follows `sha384-`.
  Load fonts from `@fontsource` on jsDelivr, as the example does, rather than Google Fonts' CSS,
  which differs per browser and so cannot be hashed.
- `localStorage`, `sessionStorage` and cookies throw: keep state in variables. This includes
  Basecoat's theme switcher, which stores the mode: follow `prefers-color-scheme` instead.
- Forms, popups, navigation, `<iframe>` and `<base>` are blocked: use buttons and `change`
  listeners, never `<form>`.
- **The app is its one HTML file: nothing it loads may be a file of the repo.** No relative `src`
  or `href` (`./chart.js`, `logo.png`, `../shared/app.css`). Put your own scripts and styles
  inline (`<script>`, `<style>`), images inline as SVG or a `data:` URI, and libraries and fonts
  from a CDN as above. Never split an app into several files or make one app load another's, and
  never add anything but `.html` apps under `apps/`.
- Every query is an `explore { }`: controls, cross-filters, sorting and date drills only apply to
  one.
- Mark the page's structure in the HTML, as [references/api.md](references/api.md#structure)
  shows: `data-anfra-container` and `data-anfra-block` on the parts of the page, with a
  `data-anfra-label`, and `data-anfra-query` / `data-anfra-control` on the elements each query
  and control draws into. Every query and control sits in a block: a standalone control gets a
  block of its own, a chart's own control goes in the chart's block. The Inspect panel shows the
  tree, finds each part on the page, and gives the user a handle for it to hand back to you.
  Mark every part; it is one attribute each.

Render a selection without rebuilding the chart. A query that drives a cross-filter is not re-run
by its own selection, so its result is unchanged when the reader clicks: restyle the marks it
already draws (dim the unselected ones) rather than recreating the series, or the chart replays
its entry animation on every click. With ECharts, merge with a plain `setOption(option)` for a
selection change and keep `replaceMerge` for a new result.

Add a download only where the agreed design lists one, never on your own initiative, and read
[references/downloads.md](references/downloads.md) first: a block gets at most one download
control, in its header, with every agreed format an item of it, never a button per format; a
picture of the whole app is one control in the page's header. Don't build a CSV from
`query.result.rows`: a paged result holds only the pages loaded. Any app may be pictured from the
host's toolbar, so keep every app render-safe: chart entry animations off (ECharts
`animation: false`), images and web fonts from servers that send CORS headers (jsDelivr, Google
Fonts), and `preserveDrawingBuffer: true` on a WebGL chart.

Render each query's four states: `executing` (loading), `success`, `error` (show
`error.message`), and success with no rows ("No sales this week", never a bare zero). Log
`result.columns`, `result.rows[0]` and `result.debug?.executedAql` on each query's first success:
the real row keys, value types and the AQL that ran. Put on the page the definitions, dates and
any rounding a reader needs to trust a number.

## 5. Run and check it

First run `anfra validate` and fix every `data_app` warning it reports: a file referenced beside
the app (every `src`, `href` and CSS `url()` must be `https://`, `data:` or a `#` fragment), or a CDN
script or stylesheet without `integrity`.

```sh
anfra status               # is a server already running for this repo, and where?
anfra serve                # if not: http://127.0.0.1:7878/apps/ (or the free port it prints)
```

Leave it running: it reloads the open app whenever its file, or the AML, changes. Lineage is the
exception: run `anfra ingest` after an AML change, or it shows the old definitions. Its header shows
the repo's problems (AML that does not compile, a data source not configured). Its Inspect panel
has two tabs: **Structure**, the tree of marked parts, with a problem count for markup it could
not read (an unknown query name, a block inside a block), and **Data**, each query's state,
executed AQL and error, in how many places it is drawn, and a Locate button. A query that says
"Not drawn yet" has no element marked with it.

If you can drive a browser, open the app, exercise every control and click, and read the console
of the app's frame, not just the page. Otherwise ask the user to, and to report what they see
against what they expected, with that console. Read errors against
[references/api.md](references/api.md#errors): a `ValidationError` names a declaring line (or an
`Anfra.lineage` call with an empty or malformed target list; an unknown field comes back
as that target's `error` or an empty diagram); a
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
