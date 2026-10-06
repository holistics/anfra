# Anfra

**Anfra is the open source framework for vibe-coding your custom BI application.**

Use your own HTML, JavaScript, and charting library. Anfra connects the app to reusable metrics, live warehouse queries, and analytics interactions such as filtering, drill-down, and period comparisons.

## Why Anfra?

Traditional BI tools make it easier to trust the numbers, but often limit teams to fixed dashboard layouts and interactions. On the other hand, coding agents can build custom dashboard apps with HTML and JavaScript easily, but wiring each view to the warehouse — and getting filters, drill-downs, and comparisons right — is easy to get wrong and hard to reuse.

Anfra is an **open-source SDK and semantic backend for those apps**. Define datasets and metrics once, then let agents build custom interfaces using reusable queries and analytics interactions resolved by the engine.

Every result can also be traced to its query and model, so teams get flexible apps without redefining the numbers each time.

## Gallery demos

Sample custom BI apps built with Anfra.

| | |
|---|---|
| [![Funnel analysis](docs/images/gallery/funnel-analysis.png)](https://anfra-demo.pages.holistics.dev/fancy-demos/funnel-analysis) | **[Funnel analysis](https://anfra-demo.pages.holistics.dev/fancy-demos/funnel-analysis)**<br>A customer journey funnel where users add, remove, and reorder steps (signed up, placed an order, reached N orders, bought from a category, total spend). Filter by country, order status, and conversion window, then click a bar to see who converted or dropped off. Breakdown and signup-cohort charts cross-filter the funnel. |
| [![Cohort analysis](docs/images/gallery/cohort-analysis.png)](https://anfra-demo.pages.holistics.dev/fancy-demos/cohort-heatmap) | **[Cohort analysis](https://anfra-demo.pages.holistics.dev/fancy-demos/cohort-heatmap)**<br>A retention heatmap with signup-month cohorts as rows and months since signup as columns. Switch between revenue, orders, and units, or show values per customer. Click a cell or cohort to filter the panels below, and right-click a cell to see its underlying data. |
| [![Cashflow reports](docs/images/gallery/cashflow-reports.png)](https://anfra-demo.pages.holistics.dev/reports/cashflow) | **[Cashflow statement](https://anfra-demo.pages.holistics.dev/reports/cashflow)**<br>A financial statement grouped into operating, investing, and financing activities, with subtotals, net change in cash, and ending cash. Switch between monthly and quarterly periods. In-cell bars show positive and negative values at a glance. |
| [![Flex canvas](docs/images/gallery/flex-canvas.png)](https://anfra-demo.pages.holistics.dev/builders/flex) | **[Canvas builder](https://anfra-demo.pages.holistics.dev/builders/flex)**<br>A free-form canvas where users add chart blocks, move them around, and zoom. Click a mark on a chart to drill down by another dimension, such as country or category, and the new chart appears linked to the one it came from. |

[View more demos →](https://anfra-demo.pages.holistics.dev/)

## How Anfra works

<img src="docs/images/how-anfra-works.png" alt="How Anfra works" align="right" width="420">

Anfra comes with a few components:
- A semantic layer to define your models, datasets, and metrics as code
- A semantic UI SDK (JavaScript library) for wiring frontend code to underlying queries
- Anfra Server to run queries through the semantic layer and manage database connections

How it works in a few steps:
1. Connect Anfra to your warehouse.
2. Define models, datasets, and metrics in the semantic layer, or have your coding agent draft them for you to review.
3. Ask your coding agent to build an app. It reads the semantic layer and writes HTML and JavaScript that uses the Anfra SDK.
4. Run `anfra serve`. The server compiles each query through the semantic layer, runs it on the warehouse, and returns results to the page.
5. Check any number in the app to see the query and metric definitions behind it.

## Quickstart

### 1. Install Anfra

```sh
curl -fsSL https://raw.githubusercontent.com/holistics/anfra/main/install.sh | bash
```

The installer downloads the latest release for your platform, places the `anfra` binary in `~/.anfra/bin`, and prints the line to add it to your `PATH`.

Supported platforms: linux (x64/arm64) and macOS (x64/arm64).

You can configure the installer with environment variables:

- `ANFRA_INSTALL_DIR` — install somewhere else (default: `~/.anfra/bin`)
- `ANFRA_VERSION` — install a specific version, e.g. `0.1.0` (default: latest)

To update later:

```sh
anfra update          # replace the binary with the latest release
anfra update --check  # check for a newer release without installing
```

### 2. Create a project

<!-- TODO: `anfra setup` and `anfra init` are not in the CLI yet -->
```sh
anfra setup
anfra init custom-bi
cd custom-bi/
```

Your folder structure should look like this:

<!-- TODO: replace with what `anfra init` actually generates -->
```
custom-bi/
├── .anfra/
│   └── data_sources.yml   # warehouse connections
├── models/                # model definitions (AML)
├── datasets/              # dataset definitions (AML)
├── apps/                  # one folder per app (HTML + JS)
└── AGENTS.md              # instructions and skills for coding agents
```

### 3. Connect a warehouse

Open `.anfra/data_sources.yml` and fill in your database credentials:

```yaml
# .anfra/data_sources.yml
data_sources:
  warehouse:
    type: postgresql            # TODO: list supported types
    connection:
      host: localhost
      port: 5432
      user: anfra
      password: anfra
      dbname: analytics
```

### 4. Start building

Open the folder in Claude Code or Cursor. `anfra setup` has already registered the skills. <!-- TODO: confirm what `anfra setup` does -->

Then ask the agent for an app:

```text
Look at the orders data and build me a revenue overview: monthly trend,
revenue by region with a region filter, and a table of top products.
Clicking a region should filter everything else.
```

With no models yet, the agent proposes datasets and metrics as code in the `models/` and `datasets/` folders.

Run `anfra serve`. The app opens at `http://localhost:4000/<name>`. <!-- TODO: confirm port and app path -->

## Examples

<!-- TODO: add examples -->

## Semantic frontend objects

The SDK gives your page a few JavaScript building blocks for talking to Anfra's semantic backend. Each one has a specific job: request data, expose an input, or connect one view's selection to another. The snippets below assume an `app` created with `anfra.createApp()`.

### Query objects

A query object describes the data a view needs. Your frontend reads its `result` and passes that data to a chart or other UI. Here, the page redraws a revenue chart whenever the query updates:

```js
const trend = app.createQuery('trend', {
  dataset: 'sales',
  aql: `explore {
    dimensions { month: date_trunc(orders.created_at, "month") }
    measures { revenue: revenue }
  }`,
})

app.subscribe(() => renderRevenueChart(document.querySelector('#chart'), trend.result))
await app.execute()
```

Queries can be written as AQL, as above, or as structured `dimensions` and `measures` fields, as in the interaction example below.

### Control objects

A control object holds an input value, such as the selected region in a dropdown. Map it to a query, then update the control when the user changes the dropdown:

```js
const region = app.createFilter('region', { dataset: 'sales', field: 'users.region' })
app.mapControl(region, trend, { field: 'users.region' })

document.querySelector('#region').addEventListener('change', (event) => {
  region.setCondition({ operator: 'is', values: [event.target.value] })
  app.execute()
})
```

The dropdown is ordinary HTML; the control carries its selected value into the mapped query. Your subscription redraws the chart with the updated result.

### Interaction mappings

An interaction mapping connects a selectable view to the queries that should respond. When a user clicks a region in the breakdown, the frontend selects that row and executes the app; the mapping applies the selection to `trend`:

```js
const byRegion = app.createQuery('byRegion', {
  dataset: 'sales',
  dimensions: { region: { field: 'users.region' } },
  measures: { revenue: { field: 'revenue' } },
})

app.mapCrossFilter(byRegion, trend)

regionChart.on('click', (row) => {
  byRegion.select([row])
  app.execute()
})
```

Your subscription can then redraw both views from `byRegion.result` and `trend.result`. Your page decides how the views look and handle clicks; Anfra runs the queries and applies the mapped filters through the semantic model.

## What you can do with Anfra

### 1. Vibe-code sophisticated data apps

For data and analytics teams who want more than a dashboard grid: funnels, cohort retention, a Mixpanel-style event explorer, a P&L with period-over-period and drill-down, a personalized "my accounts" view for every sales rep.

Describe the app to Claude or Cursor. The agent reads your semantic layer through the Anfra MCP server and writes the page with the SDK. You iterate in plain language.

```text
Build a signup → activation → paid funnel by week, split by acquisition channel.
Clicking a step should cross-filter a table of accounts that dropped off there.
```

→ [Funnel](https://anfra.dev/gallery/funnel) · [Cohort retention](https://anfra.dev/gallery/cohorts) · [P&L with PoP](https://anfra.dev/gallery/pnl) <!-- TODO: build these samples -->

### 2. Put AI artifacts on governed data

For data leaders whose business users already make reports in Claude or ChatGPT.

Connect everyone's agent to one Anfra server instead of straight to the warehouse. The artifacts they create:

- query shared metric definitions rather than improvised SQL;
- keep no data in the file and refresh when opened;
- can be inspected back to the query and metric behind every number.

Your users keep the Claude experience. You keep one semantic layer. The open-source server does not manage users or permissions. When artifacts need to be shared with permissions, audited and revoked across the company, that's [Anfra Cloud](#anfra-oss-vs-anfra-cloud).


## Core concepts

| Concept                    | What it is                                                                                                                                             |
| -------------------------- | ------------------------------------------------------------------------------------------------------------------------------------------------------ |
| **Model**                  | A mapping from a warehouse table or SQL query to dimensions and measures, defined in AML in `models/`.                                                 |
| **Dataset**                | A set of related models the agent is allowed to query, defined in AML in `datasets/`.                                                                  |
| **Metric**                 | A named, reusable measure (`revenue`, `active_users`, `conversion_rate`) defined once and composable with others. In AML it is written as a `measure`. |
| **Query**                  | A declared request against a dataset: dimensions, measures, filters. Written as AQL or structured fields; query-local metrics are allowed.             |
| **Control**                | A filter, date drill or parameter whose value flows into the queries it's mapped to.                                                                   |
| **Mapping / cross-filter** | Explicit edges between controls, selections and queries. Nothing cascades implicitly, so behaviour is predictable.                                     |
| **Inspect**                | Every result carries provenance: executed query, fields, metric definitions, timing, cache status and lineage.                                         |

AML is the language for defining models and datasets. AQL is the language for querying them.

Advanced interactions, including drill-down, period-over-period, "view underlying data" and cross-filtering, are resolved by the semantic engine rather than recomputed in the browser. This is where Anfra apps stay correct when hand-written SQL doesn't.

## CLI

Run `anfra --help` for commands, or `anfra <command> --help` for a specific one.

| Command | What it does |
| --- | --- |
| `anfra serve` | Start a warm server for the current repo. Other commands route to it when it is running. |
| `anfra query` | Compile and run an AQL query against a dataset (`--generate` prints the SQL, `--validate` type-checks). |
| `anfra validate` | Validate the AML repo, optionally scoped to file globs. |
| `anfra ingest` | Build the local search catalog from context sources. |
| `anfra search` | Search the local catalog. |
| `anfra status` | Report whether a warm server is running and its sidecars are healthy. |
| `anfra update` | Update the binary to the latest release. |
| `anfra version` | Print the anfra version. |

## Architecture

| Package | Description |
| --- | --- |
| `core` | The engine: semantic layer, modeling language, query compiler, connections, catalog and lineage. No server, no UI. |
| `server` | HTTP semantic API and MCP server, wrapping `core`. |
| `cli` | `anfra init`, `anfra serve`, `anfra validate` and friends. |
| `sdk` (`@anfra/sdk`) | Headless browser SDK: queries, controls, interactions, state, provenance. |
| `skills` | Agent skills and worked examples for the SDK and modeling language. |

<!-- TODO: confirm repo layout (single repo vs engine + SDK repos — open item with Hoàng). -->

The modeling layer uses AML and AQL, the languages behind [Holistics](https://www.holistics.io), which run in production at hundreds of companies. <!-- TODO: verify customer-count claim before publishing -->

## Anfra OSS vs Anfra Cloud

The open-source engine is complete for building, running and self-hosting apps for your team. It is an engine, not the whole car: it doesn't manage users or enforce who can see what.

|                                               | Anfra (open source) | Anfra Cloud |
| --------------------------------------------- | ------------------- | ----------- |
| Semantic layer, metrics, AQL engine           | ✅                   | ✅           |
| JS SDK, controls, interactions, inspect       | ✅                   | ✅           |
| MCP server + agent skills                     | ✅                   | ✅           |
| Self-host                                     | ✅                   | Managed     |
| Users, SSO, roles                             | —                   | ✅           |
| Row-level and viewer-level permissions        | —                   | ✅           |
| Hosted apps: sharing, public links, discovery | —                   | ✅           |
| Audit trail, usage monitoring                 | —                   | ✅           |
| Snapshots, versioning                         | —                   | ✅           |



## FAQ

<details>
<summary><b>How is this different from giving Claude a warehouse or dbt MCP?</b></summary>

For a one-off question, not much. The difference shows up when you *build*: reusable metrics instead of re-derived SQL, interactions (drill, cross-filter, PoP) resolved by the engine, apps that refresh instead of holding pasted data, and a trace from every number back to its definition.
</details>

<details>
<summary><b>How is this different from Streamlit, Evidence or Hex?</b></summary>

Those give you a framework to write the app in. Anfra gives you a semantic backend for any front end the agent writes: plain HTML/JS, no Python runtime, no fixed component set. Metrics and interactions are defined once and shared across every app.
</details>

<details>
<summary><b>Do I have to learn AML or AQL?</b></summary>

No. The agent reads and writes both for you, and the skill package teaches it how. You review model changes as code.
</details>

<details>
<summary><b>I already have a BI tool.</b></summary>

Keep it for certified reporting. Use Anfra for the AI-built, exploratory and highly custom apps that don't fit a dashboard grid.
</details>

<details>
<summary><b>Can I use my existing semantic layer (dbt, Cube, Snowflake semantic views)?</b></summary>

Not yet. It's on the roadmap. Direct SQL datasets are supported today. <!-- TODO: confirm what ships in v0.1 -->
</details>

<details>
<summary><b>Is the browser computing any numbers?</b></summary>

Source data is resolved by the server at view time. Page code *can* transform returned rows in JavaScript. Inspect shows the server query, and anything computed on top of it in the page is the page's responsibility.
</details>
