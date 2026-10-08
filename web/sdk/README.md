# anfra-sdk

The TypeScript SDK for **Data Apps**: interactive analyses over a repo's datasets, each defined by a
single HTML file (a **Data App definition**) that declares queries in AQL, with controls and
cross-filters, and renders the results with its own HTML, CSS and JS. The SDK supplies data and
state, never UI.

It started as a fork of the Holistics Data App SDK, and diverges from it.

## Entrypoints

The package root exports nothing; each entrypoint is for a different side of a Data App.

| Entrypoint | For | Holds |
|---|---|---|
| `anfra-sdk/app` | a Data App definition, in its frame | the **app runtime** (`createApp`, queries, controls, cross-filters, results and state), and the **frame bootstrap** that provisions it from its host |
| `anfra-sdk/host` | the page hosting a Data App | provisioning its frame, mounting it sandboxed, and answering its calls over the **bridge** |
| `anfra-sdk/api` | the host's page, scripts, tests | a client for the core API, generated from anfra's spec, and a **Backend** over it |
| `anfra-sdk/common` | all of the above | the contract they share: types, error classes, the bridge's messages |

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

`Anfra.datasets` describes what there is to query. For TypeScript, `anfra-sdk/ambient` declares the
global.

## Hosting a Data App

```ts
import { coreApiBackend, coreClient, loadDatasets } from 'anfra-sdk/api';
import { mountDataApp } from 'anfra-sdk/host';

const client = coreClient('/api');                       // the core API, on whichever host
const { datasets } = await loadDatasets(client);         // every dataset, through core.show

const mounted = mountDataApp({
  container: document.querySelector('#data-app')!,
  definition: await (await fetch('/appserve/files/sales/overview.html')).text(),
  baseHref: '/appserve/files/sales/',                    // where its relative URLs resolve
  datasets,
  user,
  backend: coreApiBackend(client, { datasets }),         // queries and field suggestions, as core.query
});
// mounted.setInspecting(true) to receive inspection snapshots; mounted.unmount() when done.
```

`mountDataApp` puts the definition in an iframe with `sandbox="allow-scripts"`, as its `srcdoc`,
with the frame bootstrap and the provisioned data ahead of it. The bootstrap's Backend forwards each
call to the host over postMessage; the host answers it with the Backend it was given.

## The boundary

A Data App definition is author code, often written by an agent, so it is never trusted:

- Its frame has an opaque origin. Its own requests to the API are cross-origin, which `anfra serve`
  refuses, and carry no credentials on a host that has them.
- The bridge answers only the Backend's two methods, `submitQuery` and `fieldSuggestions`. Nothing
  else of the API is reachable from a frame.
- `anfra-sdk/app` imports only `common`: the script injected into a frame carries no API client.
  A test on the built frame script holds this.

## Development

From the repository root, after `pnpm install`:

```sh
pnpm build:sdk     # dist/{common,app,host,api}.js, with the frame script built into host
pnpm dev:sdk       # the same, rebuilt on change
pnpm test:sdk      # vitest
```

In `web/sdk/`, `pnpm typecheck`, and `pnpm generate` to regenerate `src/api/schema.d.ts` after the
core API's spec (`api/openapi.yaml`) changes.
