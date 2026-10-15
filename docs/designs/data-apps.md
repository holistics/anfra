# Data Apps

A **Data App** is an interactive analysis over a repo's datasets: something a reader opens and an author (often an agent) develops. Its **definition** is one HTML file under the repo's `apps/`, which declares its queries (in AQL, today), with controls and cross-filters, and renders their results with its own HTML, CSS and JavaScript. anfra supplies data and state; the author owns the page, and what anfra draws (an inspect overlay, a plugin's modal) stays in a layer above it. Every feature a Data App reaches beyond its page, running a query included, is a plugin its host installs ([plugins.md](plugins.md)).

This doc is the design: how a definition becomes a running page, where each concern lives, and the boundaries that keep author code from reaching more than it should. The trust model behind those boundaries is [security.md](security.md); what belongs in core at all is [philosophy.md](philosophy.md).

## Why one HTML file

- **Any UI an author or agent can write.** No component set, no framework to learn: HTML, CSS and JavaScript, with any charting library from a CDN.
- **Nothing to build or deploy.** A file in the repo is a Data App; `anfra serve` shows it, and versioning, review and sharing are the repo's.
- **The SDK is provisioned, not imported.** A definition never mentions the SDK: its host puts the `Anfra` global in place before the definition's first script runs. So a definition carries no version of the SDK, and the same file runs on any platform.

A definition can come from many authors, people and agents, and an agent busy building an analysis may not think of security. So anfra guards every definition for its author: it runs in a sandbox, and reaches data only through the SDK, which offers everything a Data App needs. The safe way is the only way, and also the easy one. Everything below follows from that.

## The pieces

| Piece | Where | What it is |
|---|---|---|
| Data App definition | the repo's `apps/**/*.html` | The author's part: one HTML file that declares its queries, controls and cross-filters through the `Anfra` global, and renders the results with its own HTML, CSS and JavaScript. It runs in the frame. |
| `@holistics/anfra-sdk/common` | `web/sdk/src/common` | The contract the others share: types (`Backend`, its requests and results, `DatasetDescriptor`, `User`), the error classes, the bridge's messages. No DOM, no network: all of it can end up in a frame. |
| `@holistics/anfra-sdk/app` | `web/sdk/src/app` | What runs in the frame: the **app runtime** (`createApp`, queries, controls, cross-filters, results, state), given a `Backend` and implementing none, and the **frame bootstrap** (the kernel), which provisions it from the host, runs the plugins' frame setups and installs `Anfra`. |
| `@holistics/anfra-sdk/host` | `web/sdk/src/host` | What hosts a Data App, in the page around it: the **host runtime**, which holds the page's plugins and middleware, provisions each frame's document, mounts it sandboxed, and answers its calls over the bridge ([plugins.md](plugins.md)). |
| `@holistics/anfra-sdk/plugins` | `web/sdk/src/plugins` | anfra's own plugins: `core` (queries and suggestions), `export`, `lineage`, `render`, `inspect`. |
| `@holistics/anfra-sdk/api` | `web/sdk/src/api` | The core API for the code around a Data App (the page hosting it, scripts, tests): a client generated from `api/openapi.yaml`, `coreApiBackend` (a `Backend` as core ops), `loadDatasets` (what a Data App is provisioned with). |
| appserve backend | `internal/appserve` | `anfra serve`'s Data App routes: the pages, the Data App tree, the files under `apps/`, the reader, live reload. Reaches no data. |
| appserve frontend | `web/appserve` | The page `anfra serve` shows: the tree of Data Apps, the running one in its frame, the repo's problems, the server's health, the inspect panel. Built on `@holistics/anfra-sdk/host` and `@holistics/anfra-sdk/api`. |

The SDK is one package with four entrypoints because they are four sides of one contract, released together; its root exports nothing. Its own guide is [`web/sdk/README.md`](../../web/sdk/README.md); the workspace is [`web/README.md`](../../web/README.md).

## The runtime

```mermaid
flowchart LR
  subgraph page["App page, at the API's origin"]
    fe["appserve frontend<br/>tree · problems · health · inspect"]
    host["@holistics/anfra-sdk/host"]
    api["@holistics/anfra-sdk/api<br/>coreApiBackend · loadDatasets"]
    subgraph frame["iframe sandbox=allow-scripts, srcdoc: opaque origin (sandboxed)"]
      app["@holistics/anfra-sdk/app<br/>bootstrap + app runtime: Anfra"]
      def["Data App definition"]
    end
  end
  subgraph serve["anfra serve"]
    appserve["appserve backend<br/>/, /apps/…, /appserve/…"]
    core["core API<br/>/api/core.*"]
  end
  sidecars["anfra-node · canal-query"]
  repo[("repo: apps/, *.aml")]

  def -->|"calls"| app
  app <-->|"bridge: postMessage"| host
  fe --> host
  host --> api
  fe --> api
  api -->|"core.show, core.query, core.lineage, core.validate, core.status"| core
  fe -->|"tree, context, files, events"| appserve
  core --> sidecars
  appserve --> repo
  sidecars --> repo
```

Opening a Data App:

1. The frontend reads `/appserve/context` (the repo's name, the reader, whether live reload is on) and `/appserve/apps` (the tree, labelled by each definition's `<title>`).
2. It loads the datasets once, with `core.show` of the repo (`loadDatasets`), and the definition from `/appserve/files/<path>`.
3. The page's host runtime (`createHost`, `host/runtime.ts`, with every plugin anfra has) mounts the frame and serves the bridge *before* the document loads, so no call is missed. The document (`host/provision.ts`) is the definition with three things added at the top of its `<head>`: a `<base href>` at the definition's folder, so its relative URLs (images, scripts next to it) resolve; the provision data (datasets, the reader, the installed plugins, the host's origin) as JSON; and the frame script: the kernel, then each plugin's frame setup.
4. In the frame, the bootstrap reads the provision data, builds a `Backend` that makes `core`'s calls over the bridge, runs the plugins' setups, installs `Anfra`, and the definition runs.
5. Each query, each filter's suggestions, and each export crosses the bridge as a plugin's call; the plugin's host half answers with `coreApiBackend`, which calls the core API.

The next three sections follow these steps: what a Data App starts with (2 and 3), where it runs and how it reaches out (3 to 5), and what each query carries (5). appserve, which carries out steps 1 to 3 on `anfra serve`, comes after them.

## What a Data App starts with

Steps 2 and 3: before the definition's first script runs, the frame is given everything it needs to describe and query the repo, so a definition never fetches its own setup.

- **Datasets: one `core.show` of the repo.** It answers every dataset in full: models, fields, metrics, as AQL can query them (`internal/command/show`, from anfra-node's `aml.show`). `toDescriptor` maps each to the SDK's `DatasetDescriptor`, named by its fqn. A dataset that can't be shown in full comes in outline with a diagnostic saying why; `loadDatasets` leaves it out rather than failing the rest, and reports the diagnostics, so a Data App that doesn't use it still runs. Diagnostics also report the repo's files that don't compile and a data source the repo doesn't configure.
- **The reader** (`User`): the platform's to say. On `anfra serve` it is the local user, with every permission and the machine's time zone (`internal/appserve/reader.go`). The permissions are rendering hints only; every query is authorised again by whoever answers it.
- **The plugins the host installed**, by name and version: what it can do beyond queries. A definition reads them as `Anfra.plugins`, and finds each one's API under its name (`query.export`, `Anfra.render`, `Anfra.lineage`) only where it is installed ([plugins.md](plugins.md)).
- **The base URL and the host's origin**, from step 3.

`core.show`'s shape is an object's interface, not its implementation: what a caller needs to write a query or read its result (names, labels, types, roles, AQL definitions), never SQL, tables or data sources. It is read from the compiler today. Moving it onto the semantic catalog, so one part of the system reads the repo's objects, changes where the answer comes from, not what it means.

## Where it runs, and how it reaches out: the frame and the bridge

Steps 3 to 5: the definition runs sandboxed, and the bridge is its one way out.

The frame is `sandbox="allow-scripts"` with its document as `srcdoc`, so it has an **opaque origin**. Its own requests to the API are cross-origin, which `anfra serve` refuses, and carry no credentials on a platform that has them. Storage, forms, popups and navigation are unavailable to it. A definition reaches data only through the bridge.

The bridge (`common/bridge.ts`, `common/endpoint.ts`; each side runs one endpoint) is a small postMessage protocol, the same both ways, carrying the plugins' calls and events ([plugins.md](plugins.md)):

- `anfra:call` with a plugin's call name (`core.submitQuery`) and an id, answered by `anfra:result`, ok or with an error by its SDK class name; `anfra:cancel` aborts one. Either side calls: the frame `core.submitQuery`, the host `render.draw`.
- `anfra:event`, a plugin's notice nobody answers: the host's `inspect.watch` and `inspect.highlight`, the frame's `inspect.snapshot` and `inspect.picked`.
- `anfra:ready`, from the frame each time its document loads.
- **The frame reaches only the calls a plugin lets it make** (`core.submitQuery`, `core.fieldSuggestions`, `export.query`, `lineage.trace`, `render.save`); every other call is the host's, refused before any middleware or handler runs. Nothing else of the API is reachable from a frame, whatever the host's credentials.
- Messages are accepted only from that frame's window (an opaque origin can't be checked by name), and the frame posts only to the host's origin, from the provision data.

The host never sees the frame's DOM. What it knows of the page is the **structure**: the semantic parts a definition marks on its own elements (`data-anfra-container`, `data-anfra-block`, `data-anfra-query`, `data-anfra-control`, with an optional `data-anfra-label`), read by the app runtime into a tree of nodes and posted in the inspect snapshot beside the apps. A container holds anything; a block holds no container or block; a query or control node is an element drawn by that entity, under whatever marked element encloses it. Problems in the markup (an unknown name, a duplicate id, a block inside a block) are reported on the node, never thrown: the markup is optional, partial, and for inspection only. It is read only while a host inspects, on each snapshot and on a debounced mutation observer limited to elements coming and going and the five attributes, so a frame nobody inspects does no work. Highlighting, locating and picking all run in the frame, which draws one overlay (a tinted box and a label) over the marked element, in the plugin layer; the host needs no geometry. Snapshots, the host's highlight and locate, and pick mode are the `inspect` plugin's; the structure, the overlay and `locate()` are the app runtime's. A definition can locate its own entities too (`query.locate()`), for a button of its own. The attributes are static, so a tool that never runs the definition can read the same structure from the file; an author copies a node's handle, `[River](data-anfra-block="river")`, to hand to an agent, which finds the element by that attribute.

**`@holistics/anfra-sdk/app` imports only `common`.** The frame script carries no API client, so a definition can't find one to misuse. `app/bundle.test.ts` builds the frame script as released and checks it. The frame script is built into `host` (a virtual module, `anfra-sdk:frame-script`, answered by an esbuild plugin in `web/sdk/tsup.config.ts`), and each plugin's frame setup into `plugins` the same way, so a host always carries the app runtime and the plugins it was built with.

## What each query carries: the query and the reader's state

Step 5, in detail: what crosses the bridge and reaches `core.query` for each query.

A query's AQL is fixed when it is declared. What the reader changes (filter values, a cross-filter selection, a sort, a date grain, the page) travels beside it, as the **Query Input**, and `core.query` applies it before compiling (`internal/command/query/input.go`; `app/execution/submitQuery.ts` builds it):

| The reader's state | Becomes |
|---|---|
| A control mapped to a query (`mapControl`) | an entry in `input.filters`, on the mapped field |
| A cross-filter selection | entries in `input.filters`, or one generated condition in `input.conditions` when the selection can't be expressed as ANDed filters |
| `query.setSort` | `input.sorts` |
| A date drill | `input.dateDrills`, on the field the drilled dimensions are built on |
| Paging, the app's time zone | `page`, `page_size`, `timezone` |

So reader input is never spliced into AQL text, and the executed AQL comes back with the result, which is what the inspect panel shows. anfra-node applies the Query Input to the query's `explore { }`, which is why a Data App's queries are explores: controls, cross-filters, sorts and date drills have nothing to apply to in another shape. The Query Input and result columns keep amql's own field names (camelCase), which the SDK speaks too, rather than being converted in core and back in the SDK. A column's types are the core's own, not amql's, so they are in the API's case: `semantic_type`, `arrow_type`, `db_type` ([result-values.md](result-values.md#the-shape)). A column mixes the two cases, and each key's case says whose it is.

`coreApiBackend` maps the core API's errors to the SDK's classes (`api/errors.ts`): a query anfra refuses or fails is a `QueryError`, a refusal of access a `PermissionError`, an unavailable server a `TransportError`. See [errors.md](errors.md) for the codes.

**Exports** (`query.export()`) carry the same query and Query Input, built by the same code, from the state the result on screen was last run with: a control the reader changed but has not applied is not in it. No page: an export is every row ([exports.md](exports.md)). The `export` plugin's host half calls the Backend's `exportQuery` (`core.query.export`), saves the file from the host's page by following its link with `<a download>`, since the frame has no `allow-downloads`, and answers the frame with the link and its row count. An export changes nothing on its query: a failed one is the author's to show, from `query.export()`'s rejection. A host without the plugin can't export: `query.export` isn't there.

**Field suggestions** (a filter control's values) are one `core.query` too: a distinct-values explore on the field, sorted and capped, narrowed for a text field by what the reader typed, as a Query Input filter, never spliced. The field must be one of the provisioned dataset's, since it is placed into AQL. It's a formal op used as intended, so a platform authorises it as a query.

## Renderings: a picture of what the reader sees

A **rendering** is a PNG or PDF of the whole app, or of one block or container of its structure, as the reader sees it: what is drawn, not the query's rows (that's an [export](exports.md)). It is made in the frame, in the reader's browser, because only there is the page in the state the reader sees it (control values, the selection, sorts, pages loaded, the author's own scripts). A browser on a server could draw vector PDFs and render with no reader present, but would have to rebuild that state, and anfra would have to ship one.

- **The author** calls `Anfra.render({ block | container?, format: 'png' | 'pdf', filename?, pixelRatio? })`; with no node, the whole app. It draws at once, so the author awaits `app.execute()` first for fresh results.
- **The host** calls `render.draw` on the mounted app (appserve: a download action on each container and block in the Structure tab, and one for the whole page in the toolbar). The frame half answers it: it first waits for running queries to settle (up to 10 s), then draws.

**Drawing.** DOM to canvas: the element is cloned with its computed styles into an SVG `foreignObject`, which the browser draws onto a canvas (html-to-image). The renderer (`src/render`) is the `render` plugin's frame body, sent to the frame on its first rendering, so frames that never render don't carry it. Inside the frame it works around the sandbox: a stylesheet on another origin (a font service) can't be read in place, so its `@font-face` rules are fetched and their files embedded; an image whose server answers no CORS headers can't be drawn at all, so it is left blank, its box kept so nothing moves, and its URL is reported in the result's `omitted`. Never drawn: the plugin layer (the overlay, any plugin's UI), and any element marked `data-anfra-render="skip"` (the author's own download button). The whole app is the body at the document's full scroll size, on the page's background.

**Formats.** PNG, at `pixelRatio` device pixels per CSS pixel: the screen's, at least 2, lowered when the canvas would pass a browser's limit (16384 px a side). A PDF is one page the size of the rendering holding the picture as a JPEG (`src/render/pdf.ts`), so it is the PNG in a document; a vector PDF can replace it behind the same `format`.

**Saving.** The frame can't download, so it posts the file's `Blob` to the host (`render.save`), which saves it with an object URL and `<a download>`, named `<app title> - <node label or id>`, or the app's title. Nothing reaches anfra's server. `render.save` gives a frame back only what a rendering needs, since its script can post it directly, with any blob and name: the host saves a PNG or a PDF, told by its first bytes, as that type, under a name cleaned of control and path characters and ending in that extension (`report.pdf.exe` is saved as `report.pdf.exe.pdf`), one at a time and at most 5 in 10 s per frame. Anything else is refused with a `ValidationError`.

**Errors** are `ValidationError`s naming the node: no element marked with that id, a picture over the canvas limit, a canvas the browser won't read (the author's own, drawn from another origin without CORS). A missing image is not an error: the picture is saved, and `omitted` says what is blank, which appserve shows. **Off unless the host installs the `render` plugin** (appserve does). Without it, nothing answers `render.save`, and `Anfra.render` isn't there. An author shows a render button only when `Anfra.plugins.has('render')` and `Anfra.user.permissions.canExportData` are both true.

A host should weigh it more than exports, for security. The frame is sandboxed without `allow-downloads` because the app's code is trusted less than the host's page, and rendering gives part of that back: the host saves a file the frame made, from the host's own origin, and the frame's script can ask for it directly, without `Anfra.render()`. The checks under **Saving** narrow that to a PNG or a PDF, but what such a file holds, and when it is saved, is still the app's to choose. An export carries none of this: the frame asks only for a query, and the file, its name and its link are the server's. So a host decides whether it trusts the apps it serves with downloads, rather than getting them by upgrading the SDK.

## appserve: what `anfra serve` adds

appserve is how `anfra serve` presents Data Apps, in two halves: a backend (`internal/appserve`) that serves the page and what it reads (the tree, the context, the files; steps 1 and 2), and a frontend (`web/appserve`), the page itself, which lists the Data Apps and runs the one a reader opens (steps 1 to 3).

appserve is also the layer to replace. A team building its own platform, or bringing Data Apps into its own app, keeps the SDK and the core API, and swaps appserve for its own pages: its own navigation, its own place to keep Data Apps, its own idea of who the reader is, around `@holistics/anfra-sdk/host` and `@holistics/anfra-sdk/api`. anfra-cloud does this: its web app serves Data Apps beside sign-in, organisations and user management, in one richer layer ([what is core's and what is a platform's](#what-is-cores-and-what-is-a-platforms)).

`anfra serve` serves the Data Apps by default (`--no-apps` turns them off), beside the core API on the same listener and origin, so the page needs no CORS (`cmd/anfra/serve.go`). appserve's routes are not ops: they are not in the spec or in discovery, since they are `anfra serve`'s alone.

| Route | Serves |
|---|---|
| `/`, `/apps/<path>` | The frontend's page, the same for every path: the frontend reads the path. `/apps/sales/overview` names `apps/sales/overview.html`. Refuses to be framed (`frame-ancestors 'none'`). |
| `/appserve/assets/…` | The frontend's built files. |
| `/appserve/apps` | The tree of Data App definitions, with their titles. |
| `/appserve/context` | The repo's name, the reader, whether live reload is on. |
| `/appserve/files/<path>` | Any file under `apps/`, as written: confined to `apps/` (through symlinks too), no dot-files. Served with `Content-Security-Policy: sandbox allow-scripts` and `nosniff`, so opening a definition's URL directly never runs it at the API's origin. |
| `/appserve/events` | Live reload's event stream (SSE), when it is on. |

The frontend is built into `internal/appserve/dist` and embedded in the binary; a binary built without it serves a page saying how to build it ([release.md](release.md)). Under `make dev`, `ANFRA_APPSERVE_DEV_URL` makes the pages redirect to Vite's dev server instead, which serves the frontend from source and proxies `/api` and `/appserve` back.

The frontend shows the repo's problems from two sources: `core.validate` (files that don't compile, the validators' errors) and the datasets' diagnostics (a data source not configured, a dataset not shown in full). It polls `core.status` for the server's health, and catches up when the server comes back.

### Live reload

The backend watches `apps/` and the semantic layer (its `.aml` files; skipping dot-directories and `node_modules`), debounces changes, and publishes events: Data Apps changed, with their paths; or the semantic layer changed. The frontend reloads the open Data App when its file changes; on a semantic layer change it reloads the problems and the datasets, then remounts the frame. Nothing is rebuilt on the server: `core.show` and `core.query` read the compile cache, which follows the files.

The watcher runs only while an event stream is open: the first starts it, the last to close stops it, and closing the server ends them all (`internal/appserve/watch.go`). A server only agents and the CLI use watches nothing. `--no-watch` turns it off entirely, for hosting Data Apps for others, where nothing is edited.

## What is core's and what is a platform's

Core (the engine and its ops) knows nothing of Data Apps, pages or readers. It gained only what is right for every client: `core.query`'s Query Input and `core.show`. Everything that maps between the SDK and the core API is `@holistics/anfra-sdk/api`'s; provisioning and answering the frame is `@holistics/anfra-sdk/host`'s; where Data Apps live and who reads them is the server's (appserve, on `anfra serve`).

That keeps the browser half portable. Another platform, anfra-cloud included, serves Data Apps with its own page around `@holistics/anfra-sdk/host` and `@holistics/anfra-sdk/api`, pointed at its own core API (for example `coreApiBackend('/api/o/{org}')`), and its own server in appserve's place: where the definitions come from, who the reader is, and whether that reader may run each query, see each dataset's schema, or get suggestions. Those are the platform's decisions, made before a core op runs ([engine.md](engine.md)).

Anything a Data App needs that core doesn't formally offer starts in the platform, not in core: a stopgap there is visibly a stopgap, while one in core becomes an implicit contract. A core op is added when no existing op expresses the need completely, with the right permission, as a typed answer. The bar is high because an op, once added, is part of the core API's contract, which every platform serves and a breaking change must declare ([the contract](commands-and-api.md#the-contract-apiopenapiyaml)).
