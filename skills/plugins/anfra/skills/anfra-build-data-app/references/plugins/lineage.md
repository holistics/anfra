# Plugin: lineage

`Anfra.lineage()`, from the `lineage` plugin. It is there only where the host installed the plugin:
check `Anfra.plugins.has('lineage')` first (see [api.md](../api.md#plugins)).


`Anfra.lineage` traces fields upstream through the repo's catalog: which other metrics and which
model fields a metric uses, and the models those fields belong to (a model's table is its
`props.table_name`). It can also trace a whole model or dataset, with every field in it. Use it
for an app that explains where a number comes from, or a field catalog. It reads the catalog `anfra ingest` builds, not the AML: after an AML change, run
`anfra ingest` or lineage shows the old definitions. It is a plain promise, outside `createApp`,
`execute()` and `subscribe`, and nothing re-runs it for you. Offer it only when
`Anfra.plugins.has('lineage')`: on a host without the plugin, `Anfra.lineage` is `undefined`.

```js
const controller = new AbortController();
const { results: [revenue, country, ecommerce] } = await Anfra.lineage([
  { parent: 'ecommerce', child: 'revenue' },  // a dataset metric: the dataset's fqn, the metric's name
  { parent: 'customers', child: 'country' },  // a model field: the model's fqn, the field's name
  { parent: 'ecommerce' },                    // a whole dataset (or model), with every field in it
], { limit: 50, signal: controller.signal }); // options are all optional
```

- **Targets** name a field by what holds it, not as a [field reference](../api.md#field-references): a
  dataset metric `revenue` is `{ parent: <dataset fqn>, child: 'revenue' }`, a model field
  `customers.country` is `{ parent: 'customers', child: 'country' }`. Leave out `child` to trace
  the whole model or dataset: `{ parent: 'customers' }`. Names match exactly, case included, so
  take them from `Anfra.datasets`. One target may be passed without the array. The answer is
  still `results`.
- **Results**: `results` has one entry per target, in the same order:
  `{ target, query, diagram }`, or `{ target, query, error }` when that target could not be traced.
  `error` is a **string**, not an `Error`, so render it as is. The others still have their diagrams.
- **An unknown name is an error**, not an empty diagram. A parent the catalog does not know gives
  `parent "sale" not found`. A child its parent does not have gives
  `parent "sales" has no child "reven"`. Both come with an empty `query`. If the name is right,
  run `anfra ingest`: the catalog may predate the AML.
- **An empty diagram** (`total` of 0, no error) means nothing is upstream of the target. A model
  dimension that is a plain column comes back empty: it has no lineage to draw, so show it as a
  source column (its model's `table_name` and the column) rather than as "not found".
- **The promise rejects** when the call itself is wrong or fails: an empty list, a target without
  a non-empty `parent`, or a `child` that is given but empty (`invalid_request`), an unreachable server
  (`connection_failed`), a reader refused (`forbidden`). It rejects, rather
  than throws, so the `try`/`catch` around the `await` sees it.
- **Options**: `limit` / `offset` page the matched entities; `diagram.total` counts them over every
  page, so `diagram.matched.length < diagram.total` means there are more. `signal` cancels: the
  promise then rejects with an error whose `name` is `'AbortError'`. Abort the previous call when a
  newer pick replaces it, and ignore an `AbortError`.

## Diagram shape

```js
diagram.total    // matched entities, over every page
diagram.matched  // this page's matched entity ids: what the field is built from, for paging
                 // only. It may leave out the traced field and holds ids no card draws.
diagram.layers   // [{ index: 0, types: ['aml.model'], card_ids: ['aml.model:…'] }, ...]: the columns, upstream first
diagram.cards    // every card, in layer order:
                 // [{ id, type: 'aml.model', label, layer: 0, props, links, sections }, ...]
diagram.schema   // { edges: { refers: 'A refers B: A uses B. …' },
                 //   types: { 'aml.model': { edges: { refers: ['aml.table'] }, top_level: true } } }
```

- A **card** is a model, dataset, table, … Its `layer` is its column: data flows from lower layers
  to higher ones. Draw one column per `diagram.layers` entry, its cards looked up by `card_ids`.
  A card's `sections` group its nodes by type: `[{ type: 'aml.dimension', nodes: [...] }]`. A node
  may have `sections` of its own, so walk them recursively. `detached: true` marks a node whose
  container is not on the diagram.
- **A card holds every field of its model, or every metric of its dataset**, not only the ones on
  the lineage. The lineage is the `refers` links. Find the traced field as the node whose
  `props.fqn` is `` `${parent}.${child}` ``, follow `refers` links from it until none are left, and
  draw (or highlight) only the nodes reached. Drawing the cards as they come shows every metric
  of the dataset for any metric. For a whole parent, start from the card whose `props.fqn` is
  `parent`, and from every node in its `sections`.
- Every card and node has an `id` unique in the diagram (`aml.dimension:ab8ed0924c5a`), a `type`,
  a `label`, optional `props` (catalog properties: `name`, `fqn`, `file_path`, `data_type`,
  `description`, …), optional `tags`, optional `links`, and `ref` when it is a copy of another
  entity.
- **`links`** is an array on the edge's *from* node: `[{ to: id, type: 'refers', props? }]`.
  `{ to: B, type: 'refers' }` on A means A uses B.
- `schema.edges` explains each edge type in words: read it, rather than guessing what an unfamiliar
  edge means.
