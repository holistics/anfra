# @holistics/anfra-sdk

The TypeScript SDK for **Data Apps**: interactive analyses over a repo's datasets, each defined by a
single HTML file (a **Data App definition**) that declares queries in AQL, with controls and
cross-filters, and renders the results with its own HTML, CSS and JS. The SDK supplies data and
state, never UI.

It started as a fork of the Holistics Data App SDK, and diverges from it.

```sh
npm install @holistics/anfra-sdk
```

Each anfra release publishes it at its own version: the SDK's API client matches that release's
core API.

## Entrypoints

The package root exports nothing; each entrypoint is for a different side of a Data App.

| Entrypoint | For | Holds |
|---|---|---|
| `@holistics/anfra-sdk/app` | a Data App definition, in its frame | the **app runtime** (`createApp`, queries, controls, cross-filters, results and state), and the **frame bootstrap** that provisions it from its host |
| `@holistics/anfra-sdk/host` | the page hosting a Data App | the **host runtime**: the plugins and middleware a page installs, provisioning each frame, mounting it sandboxed, and answering its calls over the **bridge**; `definePlugin` for a plugin of your own |
| `@holistics/anfra-sdk/plugins` | the page hosting a Data App | anfra's own **plugins**: `core` (queries), `export`, `lineage`, `render`, `inspect` |
| `@holistics/anfra-sdk/build` | a plugin's build, in Node | `buildFrameHalf`: a plugin's frame half as the script a host provisions |
| `@holistics/anfra-sdk/testing` | a plugin's tests | `testFrame`: a frame half run on the real kernel against a stand-in host |
| `@holistics/anfra-sdk/api` | the host's page, scripts, tests | a client for the core API, generated from anfra's spec, and a **Backend** over it |
| `@holistics/anfra-sdk/common` | all of the above | the contract they share: types, error classes, the bridge's messages |

## Writing a Data App

A definition needs no setup and imports nothing: its host provisions the SDK as the `Anfra` global
before the definition's first script runs.

```js
const app = Anfra.createApp({ title: 'Sales' });

const status = app.createFilter('status', { dataset: 'ecommerce', field: 'orders.status' });
const byRegion = app.createQuery('byRegion', {
  dataset: 'ecommerce',
  aql: 'explore { dimensions { region: users.region } measures { revenue: sum(orders.amount) } }',
});
app.mapControl(status, byRegion, { field: 'orders.status' });

app.subscribe(() => render(byRegion.result));
await app.execute();
```

Everything beyond queries comes from a **plugin** the host installed, which adds its API under its
own name (`query.export`, `Anfra.render`, `Anfra.lineage`). Where the host didn't install it, the
member isn't there: check `Anfra.plugins.has(name)` before using an optional one.

Data Apps written before plugins read `Anfra.features.export` (`.render`, `.lineage`). It still
answers, from the installed plugins, but it is deprecated and warns in the console: replace it with
`Anfra.plugins.has('export')`. It goes in the next breaking release.

A reader can download a query's whole result, with the inputs its result on screen was run with
(the `export` plugin):

```js
if (Anfra.plugins.has('export') && Anfra.user.permissions.canExportData) {
  button.onclick = () => byRegion.export().catch((err) => showError(err.message));
}
```

`export()` resolves once the file is made and the host has started saving it, with its link and
`rowCount`. The file is a CSV named after the app and the query unless you pass `filename`.

A reader can download a picture of the app, or of one block or container, as they see it (the
`render` plugin):

```js
await Anfra.render({ block: 'trend', format: 'png' });   // or 'pdf'; no node: the whole app
```

It works only where the host installed `render` (`anfra serve` does). Check
`Anfra.plugins.has('render')` and `Anfra.user.permissions.canExportData` before offering it, and
mark what shouldn't be drawn (the download button itself) `data-anfra-render="skip"`. Serve images
and web fonts with CORS: an image without it is left blank (listed in the result's `omitted`), a
font without it falls back.

`Anfra.datasets` describes what there is to query. For TypeScript, `@holistics/anfra-sdk/ambient` declares the
global. A plugin's members are typed as optional: check the member itself (`if (Anfra.render)`,
`query.export?.()`), which narrows it.

`Anfra.lineage({ parent: 'orders', child: 'amount' })` (the `lineage` plugin) traces fields through the repo's catalog, each named
by its parent and its name in it: what each is built from, as a diagram to draw (`results[i].diagram`), or
why it could not be (`results[i].error`). It reads the catalog the host last ingested (`anfra ingest`).

A definition can mark its parts, so a host's inspect panel can show them as a tree and find them on
the page:

```html
<section data-anfra-container="page" data-anfra-label="Overview">
  <div data-anfra-block="revenue" data-anfra-query="byRegion">…</div>   <!-- a block drawn by a query -->
  <div data-anfra-block="trend">
    <div data-anfra-query="trend"></div>                                  <!-- the query's chart -->
    <select data-anfra-control="grain"></select>                          <!-- and its control -->
  </div>
</section>
```

A **container** holds anything; a **block** holds no container or block. `data-anfra-query` and
`data-anfra-control` name what an element is drawn by (`name`, or `app/name` for a frame with
several apps). Ids are unique per kind. The markup is optional and partial, read only while a host
inspects, and never thrown over: a wrong name or a block inside a block is reported in the tree.
`byRegion.locate()` scrolls to the elements marked with that query and shows them for a moment.

## Hosting a Data App

```ts
import { coreApiBackend, coreClient, loadDatasets } from '@holistics/anfra-sdk/api';
import { createHost } from '@holistics/anfra-sdk/host';
import {
  corePlugin, exportPlugin, inspectPlugin, lineagePlugin, renderPlugin,
} from '@holistics/anfra-sdk/plugins';

const client = coreClient('/api');                       // the core API, on whichever server serves it
const { datasets } = await loadDatasets(client);         // every dataset, through core.show
const backend = coreApiBackend(client, { datasets });    // queries, suggestions, exports and lineage, as core ops

const host = createHost({                                // one per page
  plugins: [
    corePlugin(backend),                                 // required: runs the queries
    exportPlugin(backend),                               // query.export()
    lineagePlugin(backend),                              // Anfra.lineage()
    renderPlugin(),                                      // Anfra.render(): the host saves PNGs and PDFs the frame makes
    inspectPlugin(),                                     // what an inspect panel needs
  ],
});

const mounted = host.mount({
  container: document.querySelector('#data-app')!,
  definition: await (await fetch('/appserve/files/sales/overview.html')).text(),
  baseHref: '/appserve/files/sales/',                    // where its relative URLs resolve
  datasets,
  user,
});
await mounted.call('render.draw', { format: 'pdf' });    // save a picture of the app
mounted.on('inspect.snapshot', ({ apps, structure }) => …);
await mounted.call('inspect.watch', true);               // start the snapshots
mounted.unmount();                                       // when done
```

`host.mount` puts the definition in an iframe with `sandbox="allow-scripts"`, as its `srcdoc`, with
the frame's kernel, the provisioned data and each plugin's frame setup ahead of it. Every call the
frame makes, and every call a plugin makes of another, goes through the host's one pipeline to the
plugin that answers it. An export's file is saved from the host's page, by following its link: the
frame can't download.

### Middleware

Middleware wraps calls by name (`*`, `core.*`, `core.submitQuery`), in order of `stage`, then name:

```ts
createHost({
  plugins,
  middleware: [{
    name: 'log',
    match: '*',
    handle: async (request, call, next) => {
      const started = Date.now();
      try { return await next(request); } finally { console.log(call.name, call.caller, Date.now() - started); }
    },
  }],
});
```

It may change the request, answer in the plugin's place (not calling `next`), refuse (throwing), or
change the answer. `call.caller` says who made the call: the frame, the host, or another plugin;
`call.origin` who started it, through any plugins between (`call.chain`). Count what the frame
started by `origin`: a query a plugin runs for the frame's request has the plugin as its `caller`.
A handler forwarding a call or request the frame chose makes it with `{ asOrigin: true }`, so it
is refused if the frame couldn't make it itself.

### A plugin of your own

A plugin adds a feature to the Data Apps on your host, such as a call from the frame to your
server. Make one with `definePlugin`, build its frame half with `@holistics/anfra-sdk/build`, and
test it with `@holistics/anfra-sdk/testing`. The guide, with a worked example, is
[docs/usage/sdk-plugins.md](../../docs/usage/sdk-plugins.md); the design is
[docs/designs/sdk-plugins.md](../../docs/designs/sdk-plugins.md).

## The boundary

A Data App definition can come from any author, often an agent, which may not think of security while building an analysis. So the SDK guards it for its author, and makes the safe way the easy one:

- Its frame has an opaque origin. Its own requests to the API are cross-origin, which `anfra serve` refuses, and carry no credentials on a platform that has them.
- The frame reaches only the calls its host's plugins let it make (`frame: true`); every other call is the host's. Nothing else of the API is reachable from a frame.
- `@holistics/anfra-sdk/app` imports only `common`: the script injected into a frame carries no API client.  A test on the built frame script holds this.

## Development

From the repository root, after `pnpm install`:

```sh
pnpm build:sdk     # dist/{common,app,host,plugins,api,build,testing}.js, with the frame script built into host and the plugins' into plugins
pnpm dev:sdk       # the same, rebuilt on change
pnpm test:sdk      # vitest
```

In `web/sdk/`, `pnpm typecheck`, and `pnpm generate` to regenerate `src/api/schema.d.ts` after the
core API's spec (`api/openapi.yaml`) changes.
