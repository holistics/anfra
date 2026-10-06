# anfra-sdk

A headless TypeScript SDK for **Data Apps**: single HTML files whose authors declare queries in AQL
plus interactive controls and cross-filters, and render the results with their own HTML, CSS and JS.
The SDK supplies data and state, never UI. It sends every query to a **Backend** supplied by the page
hosting the app, so it runs against anfra, or anything else that implements two methods.

Forked from the Holistics Data App SDK; see [ADR 0009](docs/adr/0009-anfra-sdk-is-a-divergent-fork.md).

## Using it

A provisioner (the page hosting a Data App) creates the SDK and installs it as the `Anfra` global
before the author's code runs:

```js
import { createSdk, installSandbox } from 'anfra-sdk'; // or <script src=".../anfra-sdk.global.js"> → AnfraSdk

const sdk = createSdk({
  datasets,          // Record<name, DatasetDescriptor>
  user,              // { id, name, email, role, timezone, permissions }
  backend: {
    submitQuery: (request, signal) => myServer.query(request, signal),
    fieldSuggestions: (request, signal) => myServer.suggest(request, signal),
  },
});
installSandbox(sdk);
```

Author code then needs no setup:

```js
const app = Anfra.createApp({ title: 'Sales' });
const byRegion = app.createQuery('byRegion', { dataset: 'ecommerce', aql: `explore { … }` });
app.subscribe(render);
await app.execute();
```

## Docs

- [DESIGN.md](DESIGN.md): the surface, model, Backend contract and runtime.
- [CONTEXT.md](CONTEXT.md): the vocabulary.
- [docs/adr](docs/adr): the decisions.
- [skills/build-anfra-app](skills/build-anfra-app/SKILL.md): the authoring guide for writing a Data App (also an agent skill).

## Development

```sh
pnpm install
pnpm test        # vitest
pnpm typecheck
pnpm build       # dist/index.js (ESM) + dist/anfra-sdk.global.js (IIFE, global `AnfraSdk`)
```
