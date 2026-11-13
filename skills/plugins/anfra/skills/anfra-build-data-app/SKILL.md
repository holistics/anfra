---
name: anfra-build-data-app
description: Build or change an Anfra Data App — one self-contained HTML file in the repo's `apps/` that queries datasets, and can trace fields' lineage, through the `Anfra` SDK global and renders them with its own HTML, CSS and JS — then check it in `anfra serve`. Use for a dashboard, report, page or interactive analysis over the repo's datasets.
---

# Building a Data App

A **Data App** is one self-contained HTML file under the repo's `apps/`. `anfra serve` lists it by
its `<title>` and runs it in a sandboxed frame where the SDK is already on the global `Anfra`. The
SDK supplies data and state; every pixel is yours.

References, read when a step needs them:

- [references/app-design.md](references/app-design.md): layout for each kind of page, the chart
  chooser, controls, words and numbers, states, and the design self-check. **Read it at step 2 in
  the default look.** Creative and own-design builds skip it.
- [references/api.md](references/api.md): every SDK call, the result shape, the plugins, errors.
  **Read it before writing any code; do not invent APIs.**
- [references/plugins/](references/plugins/): one file per built-in plugin beyond `core`
  ([export](references/plugins/export.md), [render](references/plugins/render.md),
  [lineage](references/plugins/lineage.md)). Read one only when the agreed design uses that plugin,
  and check `Anfra.plugins.has(name)` before using it.
- [references/downloads.md](references/downloads.md): the download button for exports and pictures.
  Read it only when the agreed design lists a download.
- [references/example.html](references/example.html): a complete app in the default look: header
  with page controls, KPI row with delta pills, a trend beside its breakdown, a footer of
  definitions. Start a new file from it, or from the closest app already in `apps/`.
- The SDK docs, for what these references don't cover: look it up with [](../anfra-docs/).

## 1. Learn the datasets

```sh
anfra show                 # every dataset: models, fields (dimensions, measures), metrics
anfra show <dataset fqn>   # one dataset
```

The fqn is what a query's `dataset:` takes. A field is `model.field` (`orders.created_at`), a
dataset metric its bare name. A measure is already aggregated: select it as it is, never wrap it
in `sum()`. Read labels and descriptions: they carry the modeler's caveats on grain, fan-out and
which date to use. For a metric's definition, read its `*.dataset.aml`.

If a number the app needs has no field or metric, add it to the AML with the [](../anfra-aml/) skill
rather than computing it in JavaScript.

## 2. Agree on the design

Restate the request as one result the user can check ("revenue for the last full week against
the week before, with a country picker"). Ask only what changes the numbers: what a business word
means, which date counts, what to exclude, each with your recommended answer.

Also ask which look the user wants, unless they already said: the **default anfra style**
(recommended), **something creative** (you choose the palette and type), or **their own design**
(a brand, colors or a reference to follow). See [Look and feel](#look-and-feel).

In the default look, read [references/app-design.md](references/app-design.md) first. Then write
the **page plan**, one line each. Where you must guess, write the guess and mark it "(assumed)",
because the user corrects a written guess in one reply.

1. **Kind of page:** a KPI or monitoring page (the reader checks numbers against a comparison), or
   another kind: an explorer, a catalog, a lineage view, a detail lookup, an ops list.
2. **Reader and decision:** who opens the page, when, and what they do after reading it ("GMs, at
   the Monday meeting: which hotel needs a push this week"). The moment sets the default period.
3. **The lead**, on a KPI or monitoring page: the one number the decision turns on, and what it is
   compared with. Other kinds have no lead, so don't invent one. Lead with the control or view the
   reader came for.
4. **Blocks and controls in reading order:** for each block, the question it answers, its form and
   its query. For each control, the question it lets the reader ask. A control no question needs is
   left out.
5. **Defaults:** the period, comparison and selection the page opens on. A KPI page opens where
   the story is (the last full month vs the same month last year) rather than on all time with no
   comparison, which leaves the analysis to the reader. An explorer or catalog may open on all time.

The plan comes before the queries because it decides them. A comparison in the lead needs its
own measure, and a breakdown its own query.

Then write the declaration list: every `createQuery` (dataset, dimensions, measures),
`createFilter`, `createDateDrill`, `mapControl` and `mapCrossFilter`, with real field names, how
each query renders (chart or table; default ECharts 6 from jsDelivr, as in the example), and which
blocks, if any, the reader can download from, one line each with its formats (`trend: CSV`). List
a download only when the user asked for one, or propose one where it plainly helps (a detail
table's data) and let them decide; none is the default. Leave pictures (PNG, PDF) to the host,
which offers them for every block and the whole page, unless the user asks for them in the app.
Show the plan and the list to the user together and get a yes before writing the file. When the
data cannot answer the request, say so and offer the nearest thing it can.

### 2.1 Lineage

Lineage explains and governs a metric rather than showing its value: it answers what the metric is
built from, the dimensions, measures and other metrics it depends on, and the models and source
tables behind them. Add it when the user asks for it, or when the request is about tracing,
understanding or governing a metric: "how is revenue calculated", "which tables feed this KPI", a
field catalog, a definitions panel beside a chart. An app that only shows numbers needs none.

When it is in, add to the declaration list which fields the app traces, when (on load, or when the
reader picks a field or clicks a chart), and how the diagram is drawn (see
[references/plugins/lineage.md](references/plugins/lineage.md)).

## Look and feel

An app must look finished without the user asking: native controls, default buttons and unstyled
text are not acceptable.

- **Default anfra style:** start from [references/example.html](references/example.html) and follow
  app-design.md. The example applies anfra's tokens to [Basecoat UI](https://basecoatui.com), a
  plain HTML/CSS component library, and already has the type scale, 32px controls, cards, chart
  theme and tooltips. Keep them, and spend your effort on the plan.
- **Their own design:** take its colors, type and layout from what they gave.
- **Creative:** you choose the palette, type, density and chart style. Write the direction into
  the plan first (a tradition to borrow from, a palette drawn from the data's domain, one move
  repeated on every block).

Creative and own-design builds skip app-design.md. They follow the user's design or the plan's
direction, and the floors below.

**Floors, in every look.** Breaking one makes the page wrong or unusable rather than different:

- Honest charts: bars start at 0, no dual y-axis, no pie past 3 parts.
- Every number carries its unit, and every change its comparison.
- Every block has its loading, updating, empty and error states, each in its loaded size.
- Text meets 4.5:1 and chart marks 3:1 on their surface, in light and dark. Nothing the reader must
  read is under 12px.
- Real buttons and inputs, visible labels, a visible focus ring, and controls side by side at one
  height.
- Nothing only on hover, and nothing that animates on its own, because the host pictures the page at
  any moment.

In every look:

- **Write plain HTML, CSS and JS, with Basecoat for the components** (pinned CDN CSS and JS, as in
  the example). Don't bring in a framework or its component kit (React, Vue, shadcn), because the
  app is one HTML file with no build step, and the example and these references assume none.
- **Controls are Basecoat's, as in the example:** never a native `<select>`. Several picks stay one
  32px line of removable chips (the example's multi-select) rather than the combobox's chips, which
  wrap and grow the field. For a long list, add a search input to its menu that hides the options
  that don't match, because Basecoat's select has no search of its own.
- **Never put a dropdown in a Basecoat `.card`.** It is a flex column that clips its overflow, so
  a dropdown is cut off at the card's edge, in its header too. Page controls sit in the page's
  header row. A control that changes one chart goes in that card's header: a segmented control,
  or a dropdown only with `overflow: visible` on that card.
- **Basecoat's script wires components by class name**, on `DOMContentLoaded` and then for every
  node added later: `.accordion`, `.combobox`, `.command`, `.drawer`, `.dropdown-menu`,
  `.popover`, `div.select`, `.sidebar`, `.tabs`, `.toast`, `#toaster` and `input[type="range"]`.
  Use those names only on that component's own markup. An app's own `.drawer` makes Basecoat
  throw "Failed to initialize drawer", so name it `app-drawer`. The script loads with `defer`, so
  `basecoat` is undefined while the page's inline scripts run. Never call it, because a call at
  the top level throws before `execute()`, and the page runs no query.
- Charts: in the default look, follow app-design.md's chooser. If a
  `dataviz` skill is available, use it for what app-design.md does not cover. Give ECharts hex
  colors: it cannot parse `oklch`.

## 3. Write and check the queries

A query's `aql` is AQL: write it with the [](../anfra-aql/) and [](../anfra-write-aql/) skills. A
Data App adds four rules, because the SDK does not run your AQL as written: it sends it with the
reader's state (filter values, the cross-filter selection, sorts, date drills, the page), which
anfra applies to the query's `explore { }` before it runs.

- **Write an `explore { }`**, optionally after query-local `dimension` and `metric` declarations.
  Any AQL expression works inside it. A pipe-style query (`orders | select(...)`) runs, but fails
  as soon as a reader touches a control, sort or date drill mapped to it.
- **Alias every dimension and measure** (`revenue: sum(orders.amount)`). The alias is the row key
  in `query.result.rows`, and what `sorts` and `setSort` name. Never name an alias after a model
  (`orders: orders | count(orders.id)`), because the alias then hides the model from the query's
  other measures.
- **No limit.** Paging is the SDK's (`pageSize`, `fetchMore`). A query without `pageSize` gets
  every row, up to the server's maximum, past which it fails rather than truncating. The 10,000
  rows `anfra query` stops at are that command's page, not a cap on apps.
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

One self-contained `.html` file in `apps/` (subfolders group apps in the list), with a `<title>` the
list shows. `apps/sales/overview.html` opens at `/apps/sales/overview`. Follow the example's
skeleton: declare everything, `subscribe` a render function, `execute()`, then wire controls and
clicks to set state and `execute()` again. Keep its two layers apart: the look (styles, markup,
chart theme) and the SDK wiring. Lay the page out in the plan's order.

Delete what the plan doesn't use from the example, markup and script together. Script left without
its markup throws on `null` before `execute()`, so the page shows skeletons forever. Markup left
without its script is a control that does nothing. Replace the example's header comment with one
line on the page's purpose and the plan's decisions (the lead, the defaults, what was left out), so
the next change starts there.

An app whose reader builds what it shows (a chart builder, a dashboard builder) edits and removes
its queries, controls and edges as the reader works: name each with a stable id, never a title,
and see [Changing the app while it runs](references/api.md#changing-the-app-while-it-runs).

The frame is a **sandbox** (`allow-scripts` only, opaque origin) under a **content policy**. Write
for it:

- **Data comes only from the SDK.** The frame has no network of its own: `fetch`, XHR, WebSocket
  and `sendBeacon` are blocked but to the CDNs below. Never call anfra's API or any other service
  from the app.
- **Libraries, styles and fonts only from cdn.jsdelivr.net, cdnjs.cloudflare.com and Google
  Fonts** (fonts.googleapis.com, fonts.gstatic.com), each by its `https://` URL with a pinned
  version. A script, style or font from anywhere else is blocked, esm.sh and unpkg.com included:
  load an npm package from jsDelivr (`/npm/<pkg>@<version>/<file>`), and an ES module from its
  `+esm` build (`https://cdn.jsdelivr.net/npm/<pkg>@<version>/+esm`). Images may come
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
- **A library may load after the SDK's first notification.** Load any library a render function
  calls with a plain `<script>` (no `defer`, `async` or `type="module"`), and still guard the
  call: when the library is missing (`typeof echarts === 'undefined'`), the render function
  returns early and the next notification draws.
- `localStorage`, `sessionStorage` and cookies throw: keep state in variables. This includes
  Basecoat's theme switcher, which stores the mode. Follow `prefers-color-scheme`, or the host's
  own mode with `Anfra.theme` ([references/api.md](references/api.md#environment)), instead. Set
  the `dark` class in `<head>`, as the example does, or a dark-mode reader sees a white flash on
  every reload. If the repo's instructions set dark mode another way (a `data-theme` attribute),
  follow them, and keep the `dark` class in step with it, because Basecoat and the tokens key off
  `.dark`. When you follow `Anfra.theme`, toggle the class in its listener too
  (`classList.toggle('dark', theme.scheme === 'dark')`).
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
  block of its own, a chart's own control goes in the chart's block. The host reads these marks to
  find each part and give the user a handle to it. Mark every part, since each takes one attribute.

Render a selection without rebuilding the chart. A query that drives a cross-filter is not re-run
by its own selection, so its result is unchanged when the reader clicks: restyle the marks it
already draws (dim the unselected ones) rather than recreating the series, which redraws every
mark and drops the hover state on each click. With ECharts, merge with a plain
`setOption(option)` for a selection change and keep `replaceMerge` for a new result.

Add a download only where the agreed design lists one, never on your own initiative, and read
[references/downloads.md](references/downloads.md) first: a block gets at most one download
control, in its header, with every agreed format an item of it, never a button per format; a
picture of the whole app is one control in the page's header. Don't build a CSV from
`query.result.rows`: a paged result holds only the pages loaded. Any app may be pictured from the
host's toolbar, so keep every app render-safe: chart entry animations off (ECharts
`animation: false`), images and web fonts from servers that send CORS headers (jsDelivr, Google
Fonts), and `preserveDrawingBuffer: true` on a WebGL chart.

Render each query's states, each in the block's loaded size, as the example's `renderStatus()` does:
loading (a skeleton), updating (the old result, dimmed), empty (the empty scope in words, never a
bare zero), and error. Render an error by who must act, as
[references/api.md](references/api.md#errors) shows: no access or sign in again for `forbidden` and
`unauthenticated`, the message for any other `user` or `client` error, and for a `server` one
"couldn't load" with a Retry that calls `app.execute()`, which reruns only the failed queries. Log
`result.columns`, `result.rows[0]` and `result.debug?.executedAql` on each query's first success:
the real row keys, value types and the AQL that ran. Keep that log, and a `console.error` for each
failure you catch, since that is what reaches `anfra serve`'s log. Delete other debug logging and
commented-out code before handing over. A `console.log` stays in the browser's console (only
warnings and errors reach `anfra serve`'s log): working from the log alone, run the query with
`anfra query` for its columns. Put on the page the definitions, dates and any rounding a reader
needs to trust a number.

## 5. Run and check it

First run `anfra validate` and fix every `data_app` warning it reports: a file referenced beside the
app (every `src`, `href` and CSS `url()` must be `https://`, `data:` or a `#` fragment), or a CDN
script or stylesheet without `integrity`.

```sh
anfra status               # is a server already running for this repo, and where?
anfra serve                # if not: http://127.0.0.1:7878/apps/ (or the free port it prints)
```

Leave it running: it reloads the open app whenever its file, or the AML, changes. Lineage is the
exception: run `anfra ingest` after an AML change, or it shows the old definitions. Its header shows
the repo's problems (AML that does not compile, a data source not configured).

Open the app in a browser and click every control once. Each click should change the page and log no
error. Open every dropdown too. Start a browser yourself only after asking the user once, because it
opens processes and windows on their machine. Otherwise ask them to open the app. Then read what
went wrong without the browser: while the app is open, `anfra serve` writes each of its runtime
errors (a failed query, an uncaught error or rejection, a listener's exception, a call the host
failed) and its `console.error` / `console.warn` lines to its log, the file `anfra status` prints as
`repo.logs`, one JSON line each with `"service":"app"`, the app's path as `app`, and
`"event":"app.error"` (the error's message as `msg`, with its `kind`, `code`, `scope`, `request_id`,
`stack`) or `"event":"app.console"` (the line as `msg`). The `kind` says where the error came from:
`call.query` (a failed query), `call.fetch_more`, `call.export` and the like (a call you left
unhandled), `app.uncaught`, `app.listener` (your code), `plugin.uncaught` (a plugin's), or
`host.internal` (the host's):

```sh
grep '"service":"app"' <repo.logs> | grep '"app":"sales/overview.html"' | tail -20
```

No `"service":"app"` lines at all, with the app open, means `anfra serve` runs with
`ANFRA_NO_APP_LOGS=1`: read the panel instead, or ask the user to.

Each load of the app starts with an `"event":"app.mounted"` line, with the file's `modified` time:
after an edit, read only the lines after the last one, or you are reading the old file's errors.
A stack names the file's own lines as `definition:<line>:<column>`, and the SDK's as `anfra-sdk.js`.

The same errors are in the panel's Overview, under Problems. Read errors against
[references/api.md](references/api.md#errors), by `code`: a `declaration_invalid` error
(`ValidationError`) names a declaring line; `invalid_request` names a call made wrong (an
`Anfra.lineage` call with an empty or malformed target list; an unknown field comes back as that
target's `error` or an empty diagram); `query_invalid` carries anfra's diagnostics for the AQL, with
their lines, and `validation_failed` names the bad entry a control or cross-filter added
(`input.filters[0].field`: a `mapControl` typo, an unsupported operator, a drill on a field no
dimension is built on). Compare a misbehaving query's `executedAql` with what you meant, and fix the
smallest thing that explains it. When the server fails a query, its full cause is in the same log:
find the server's line by the app line's `request_id`. A bare "The call failed." is a host-side
error: its app line (`"kind":"host.internal"`) has the original message and the call. If you could
not check the interface, say so rather than claiming it works.

Then read the file against the floors (Look and feel) and, in the default look, app-design.md's
self-check (section 7). Fix every "no" before you hand over. With a
browser, look at the page in light and dark, and in a narrow frame.

## 6. Hand over

Tell the user, in their terms: what the page leads with and shows, how to use each control, its URL,
the assumptions you made, and what you checked. When a later request names a part by its handle,
`[Revenue over time](data-anfra-block="trend")` or a bare `data-anfra-block="trend"`, search the
file for that attribute: it is the element the user means. A `127.0.0.1` URL opens only on this
machine; `anfra serve` has no authentication, so ask before serving it beyond loopback (`--addr`).
Make each requested change the same way: restate it if it changes what a number means, change the
smallest thing in place (no `renderTrendV2` beside the old function, no commented-out old code), and
check that the agreed result still holds.

When the user says something is wrong ("the chart is empty"), read the app's log lines since its
last `app.mounted` before asking them anything (see "Run and check it"): the error, its `kind` and
the server's line by `request_id` usually say what. If there is no mount since your last edit, ask
them to reload the app first.

## Related skills
* [](../anfra-aml/) — add the fields and metrics an app needs.
* [](../anfra-write-aql/) — write and validate the queries.
* [](../anfra-setup-repo/) — set up the repo and its data source first.
