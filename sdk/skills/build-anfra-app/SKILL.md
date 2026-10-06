---
name: build-anfra-app
description: Build an anfra Data App — one self-contained HTML file in a Data Folder's `apps/` that queries datasets through the `Anfra` SDK global — from a short description, then iterate on the user's test feedback.
disable-model-invocation: true
---

# Building a Data App

A **Data App** is one HTML file saved under a Data Folder's `apps/` directory. The anfra demo lists
it by its `<title>` and runs it in a sandboxed frame where a provisioned SDK already sits on the
global `Anfra`. The SDK supplies data and state; every pixel is yours. You don't run the app
yourself: the user opens it in the demo and reports back, so their console output is your only
debugger.

Reference, read when a step needs it:

- [API.md](API.md): every SDK call, the result shape, errors. Read before writing any code.
- [AQL.md](AQL.md): writing the query bodies.
- [example.html](example.html): a complete working app. Start the file from its skeleton.

## Steps

### 1. Collect the environment

The Data Folder is plain files, so read it directly:

- `datasets/*.dataset.aml`: each `Dataset <name> { … }`. The name is what `dataset:` takes. Its
  `models:` list says which models it joins, `relationships:` how, and each `metric <name> { … }` is
  a dataset metric, named bare in AQL.
- `models/*.model.aml`: each `Model <name> { … }` and its `dimension <field> { type: … }` and
  `measure` blocks. A field is `model.field` (`orders.ordered_at`). A measure is already
  aggregated: select it as-is, never wrap it in `sum()`.
- Descriptions and labels carry the modeler's caveats on grain, fan-out and which date to use:
  follow them.

If you can't read the folder, ask the user to save this as `apps/env.html`, open it in the demo, and
copy back the JSON it prints:

```html
<title>Environment</title>
<pre id="out" style="white-space:pre-wrap"></pre>
<script>
  const summary = {
    user: Anfra.user,
    datasets: Object.fromEntries(Object.entries(Anfra.datasets).map(([uname, d]) => [uname, {
      label: d.label,
      models: Object.fromEntries(d.data_models.map((m) => [m.name,
        m.fields.map((f) => `${f.name}: ${f.type}${f.is_custom_measure ? ' (measure)' : ''}`)])),
      metrics: d.metrics.map((m) => `${m.name}: ${m.type}`),
    }])),
  };
  document.getElementById('out').textContent = JSON.stringify(summary, null, 2);
</script>
```

Done when you hold the dataset name and every model and field name you intend to use appears
verbatim in what you collected, or is declared as a query-local dimension or metric in the AQL.

### 2. Grill the design

Ask one question at a time, each with your recommended answer, until you can name every entity in
the app. Cover:

- The **queries**: what each one shows, its dataset, its dimensions and measures, which fields exist.
- The **controls**: which filters (field-backed or a fixed list), whether a date grain switch, and
  exactly which queries each one conditions and on which field.
- **Cross-filtering**: which chart clicks filter which other queries (same dataset only), and whether
  clicks replace the selection or toggle rows for multi-select (see [API.md](API.md#cross-filtering)).
- **Rendering**: chart or table per query, which library (default: ECharts 6 from jsDelivr, as in the
  example), and layout.

Done when you can write the declaration list: every `createQuery`, `createFilter`,
`createDateDrill`, `mapControl` and `mapCrossFilter` call, with real field names from step 1.
Show that list to the user and get a yes before writing code.

### 3. Write the file

One `.html` file in the Data Folder's `apps/` (subfolders group apps in the demo's list), with a
`<title>` the list will show. Everything inline except libraries loaded from a CDN, with pinned
versions. Follow the skeleton in [example.html](example.html): declare everything, subscribe a render
function, `execute()`, then wire controls and clicks to set state and call `execute()` again.

The frame is a **sandbox** (`allow-scripts` only). Write for it:

- Data comes only from the SDK. Don't call the demo server or anfra yourself.
- The origin is opaque: keep state in variables, since `localStorage`, `sessionStorage` and cookies
  throw.
- Forms, popups and navigation are blocked. Use buttons and `change` listeners, never `<form>`.
- Every query is an `explore { }` (see [AQL.md](AQL.md)): controls, cross-filters, sorting and date
  drills only apply to one.
- Log `result.columns`, `result.rows[0]` and `result.debug?.executedAql` of each query on first
  success, so the user's console paste tells you the real row keys, value formats and the AQL that
  actually ran.

Render every query's four states: `executing` (loading), `success`, `error` (show `error.message`),
and empty rows.

Record **generation metadata** in `<head>`, as the first child after `<meta charset>`:

```html
<script type="application/json" id="data-app-metadata">
{
  "prompt": "a sales overview with revenue by region and a monthly trend",
  "model": null,
  "effort": null,
  "createdAt": null,
  "updatedAt": null
}
</script>
```

- `prompt`: the text the user invoked this skill with; if they invoked it with none, their first
  message describing the app. Copy it verbatim, never summarised or corrected. It is fixed for the
  life of the file: later answers and feedback never change it.
- `model`: the model name or identifier explicitly supplied by the runtime or session context for
  the latest revision. Use `null` when unavailable; do not infer it from the agent or product name.
- `effort`: the configured reasoning effort for the latest revision, when explicitly available
  (for example, `"high"`). Otherwise use `null`; do not estimate it from task complexity.
- `createdAt`: the first generation's UTC timestamp in ISO 8601 format, read from an available
  clock (for example, `"2026-09-24T14:30:00Z"`). Preserve it on later revisions.
- `updatedAt`: the latest revision's UTC timestamp, read when producing the file. On first
  generation, set it equal to `createdAt`. Use `null` for unavailable timestamps rather than
  inventing a time or using the time the reader opens the app.

Serialize the metadata as JSON, escaping every `<` as the literal JSON escape `<` so prompt
text cannot close the script tag. Keep the block static, with no runtime script to populate it.
When editing an older file, add missing fields using these rules; an unknown original creation
time stays `null`.

Done when the file contains every entity from the approved list, each query is rendered in all
four states, and the metadata block contains all five fields with known values or explicit `null`s.

### 4. Hand over and iterate

Save the file (or give it to the user to save) and ask them to open it in the demo and report:

- what they see, against what they expected;
- the browser console for the Data App's frame.

The demo reloads a Data App when its file changes, so each revision is one save away.

Read their report against [API.md](API.md#errors): a `ValidationError` points at a declaring line; a
`QueryError` carries anfra's message about the AQL or names the bad entry (`filters[0].field`).
Compare a misbehaving query's `executedAql` with what you meant. Change the smallest thing that
explains it, write the whole updated file, preserving `prompt` and `createdAt` while refreshing
`model`, `effort`, and `updatedAt` for this revision, and repeat until the user says it's done.
