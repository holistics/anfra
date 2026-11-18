# LookML `extends` and refinements → flattened AML

How to convert LookML's reuse mechanisms: `extends`, `extension: required`, refinements (`view: +name`,
`explore: +name`), `final`, and files included from an imported project.

Looker: [Reusing code with extends](https://docs.cloud.google.com/looker/docs/reusing-code-with-extends),
[LookML refinements](https://docs.cloud.google.com/looker/docs/lookml-refinements),
[Importing files from other projects](https://docs.cloud.google.com/looker/docs/importing-projects)

**Flatten first, then convert.** Resolve every `extends` and every refinement on the LookML side. This gives one
final, self-contained view or explore per name. Convert each of them with the ordinary rules
([view.md](view.md), [explore.md](explore.md) and the field docs), as if it had been written that way.

```lookml
view: event_base {
  extension: required

  dimension: id         { primary_key: yes  type: number  sql: ${TABLE}.id ;; }
  dimension: user_id    { type: number  sql: ${TABLE}.user_id ;; }
  dimension: event_name { type: string  description: "Raw event name"  sql: ${TABLE}.name ;; }
  measure: count        { type: count }
}

view: web_events {
  extends: [event_base]
  sql_table_name: analytics.web_events ;;

  dimension: user_id    { hidden: yes }
  dimension: event_name { label: "Event"  sql: LOWER(${EXTENDED}) ;; }
  measure: page_views   { type: count  filters: [event_name: "page_view"] }
}
```
```aml
// view: web_events = event_base ← web_events
Model web_events {
  type: 'table'
  label: 'Web Events'
  data_source_name: 'demodb'
  table_name: 'analytics.web_events'

  dimension id {
    label: 'Id'
    type: 'number'
    primary_key: true
    definition: @sql {{ #SOURCE.id }};;
  }
  // event_base; `hidden` from web_events
  dimension user_id {
    label: 'User Id'
    type: 'number'
    hidden: true
    definition: @sql {{ #SOURCE.user_id }};;
  }
  // event_base (type, description); web_events (label, sql)
  dimension event_name {
    label: 'Event'
    type: 'text'
    description: 'Raw event name'
    definition: @sql LOWER({{ #SOURCE.name }});;
  }
  // event_base, qualified with the final model name
  measure count {
    label: 'Count'
    type: 'number'
    definition: @aql count(web_events.id);;
  }
  measure page_views {
    label: 'Page Views'
    type: 'number'
    definition: @aql count(web_events.id) | where(web_events.event_name == 'page_view');;
  }
}
```

No model is emitted for `event_base`.

## Quick reference

| LookML | AML | Section |
|---|---|---|
| `extends` (view) | inherited fields are written into the child's `Model` | [extends (view)](#extends-view) |
| `extends` (explore) | inherited joins are written into the child's `Dataset` | [extends (explore)](#extends-explore) |
| `extension: required` | no Model or Dataset is emitted | [extension](#extension) |
| `${EXTENDED}`, `EXTENDED*` | replaced by the parent's text | [Merging two layers](#merging-two-layers) |
| `view: +name` | merged into the `Model` of `name` | [view refinement](#view-name) |
| `explore: +name` | merged into the `Dataset` of `name` | [explore refinement](#explore-name) |
| `final` | unsupported; drop | [final](#final) |
| `include: "//project/…"` | the imported files are converted together | [Imported projects](#imported-projects) |

---

## Do not use AML `extend` for this

AML has [`extend`](https://docs.holistics.io/reference/aml/extend), but it does not behave like LookML `extends`
or like a refinement. Do not emit `Model child = parent.extend({ … })` for these constructs:

- **AQL is inherited as text.** A parent measure `count(users.id)` still says `users` in the extended model. It
  then fails to validate, or it aggregates the parent model instead of the child. LookML's `${TABLE}` and
  `${field}` are relative to the view, so LookML has no such problem.
- **A refinement cannot be expressed.** A refinement changes an object in place under its own name. AML cannot
  define the same name twice (`Duplicated name`).
- **Dataset lists are replaced, not appended.** LookML `join` is additive. An extended Dataset that adds one model
  must restate all `models` and `relationships`.
- **The model type cannot change.** A child that replaces `sql_table_name` with `derived_table` cannot be an
  `extend` of a table model.
- **Nothing can be removed**, and an abstract view cannot be a `Model` (a table model requires `table_name`).

The cost of flattening is duplication: inherited fields are written once per child. A later change to a shared
parent must be made in every child, or made in LookML and converted again. Say this in the conversion report.

(`extend` is still used for one thing: an alias model for a join with `from` — see [explore.md](explore.md#from).)

## How to flatten

Work per LookML model file. A refinement only applies in the model files that include it.

1. **Collect the files.** Resolve the model file's `include` lines, in order, and the includes of the included
   files. `include: "//project/path"` points into an imported project (see [Imported projects](#imported-projects)).
   The order of the files is the include order; inside a file, top to bottom.
2. **Build each view and explore `N`** that is not a refinement. Merge these layers in this order; a later layer
   overrides an earlier one:
   1. the objects in `N`'s own `extends` list, in list order. Each parent is first built by this same procedure,
      with its own parents and its own refinements.
   2. the objects in the `extends` lists of `N`'s refinements, in refinement order;
   3. `N`'s own body;
   4. `N`'s refinements (`+N`), in the order from step 1.
3. **Apply defaults after merging**, not before. A view with no `sql_table_name` and no `derived_table` in any
   layer uses a table named after itself. An explore with no `from` and no `view_name` in any layer is based on the
   view named after itself.
4. **Drop** the refinement blocks and the objects with `extension: required`. Keep the final objects.
5. **Convert** each final object with the ordinary rules. Inherited fields are converted in the final view's
   context: `${TABLE}` is its table, `${field}` is its own (possibly overridden) field, `type: count` counts its
   primary key, and every AQL reference is qualified with its name (`count(web_events.id)`).
6. **Add a comment** on the AML object that names its layers (`// view: web_events = event_base ← web_events`),
   and on each field whose parameters come from more than one layer. A reviewer can then trace every value.

### Merging two layers

Merge the LookML text as written. Parameters that a layer does not write take no part; do not fill in defaults
before merging.

| LookML | Merge |
|---|---|
| single-value parameter (`label`, `type`, `sql`, `hidden`, `sql_table_name`, …) | the later layer replaces the earlier one |
| named block (`dimension`, `measure`, `dimension_group`, `filter`, `parameter`, `set`, `join`, `derived_table`, …) | same name → merge its parameters by these same rules; different name → keep both |
| additive parameters: field `link`, `action`, `filters`; `allowed_value`; derived table `bind_filters`, `filters`, `sorts`, `dev_filters`; explore `access_filter`, `aggregate_table`, `join`, `query`; `extends` | keep the values of both layers |
| any other list (`drill_fields`, `timeframes`, `tiers`, `tags`, `fields`, set `fields`) | the later layer replaces the earlier one |
| `${EXTENDED}` in `sql` | replace it with the earlier layer's `sql` |
| `EXTENDED*` in a set's `fields` | replace it with the earlier layer's list |
| `extension: required`, `final` | not inherited |

Notes:

- A field that only changes one parameter (`dimension: user_id { hidden: yes }`) merges into the inherited field
  *before* conversion. This can change other results: a `primary_key: yes` added by a layer changes what
  `type: count` converts to ([measure.md](measure.md)).
- Additive measure `filters`: all conditions of all layers go into one `where(…)` ([measure.md](measure.md#filters)).
- A `join` of the same name in two layers: merge it parameter by parameter, like a dimension. Looker's docs only
  say that `join` is additive, so list these joins in the conversion report.
- `${EXTENDED}` and `EXTENDED*` are documented for `extends`. If one appears in a refinement or in another list,
  apply the same substitution and report it.

---

## `extends` (view)

Looker: [`extends` (for views)](https://docs.cloud.google.com/looker/docs/reference/param-view-extends) ·
Holistics: [AML Model](https://docs.holistics.io/reference/aml/model)

One self-contained `Model` per extending view, named after the *extending* view. It holds the inherited fields and
its own fields. See the example at the top of this page.

- **Parent and child are independent models.** A parent without `extension: required` is also a normal view: emit
  its own Model as well.
- **Several parents** (`extends: [a, b]`): merge in list order. On a conflict the later parent wins, then the view's
  own body.
- **Chains** (`c` extends `b` extends `a`): build `b` first, then merge `c` on top.
- **`sql_table_name` / `derived_table`** come from the last layer that writes one. A child may replace a table with
  a derived table or the reverse; the Model is then a query model or a table model accordingly
  ([derived-tables.md](derived-tables.md)).
- **No table anywhere in the chain:** the table name defaults to the child's view name. This is usually a mistake
  in the LookML. Convert it as written and list it in the conversion report.
- **A child of a derived table** gets its own copy of the query, as in Looker. Persistence is dropped
  ([derived-tables.md](derived-tables.md#persistence-parameters)).

Two parents and a chain:

```lookml
view: device_fields {
  extension: required

  dimension: os         { type: string  sql: ${TABLE}.os ;; }
  dimension: event_name { description: "Event name reported by the device" }
}

view: app_events {
  extends: [event_base, device_fields]
  sql_table_name: analytics.app_events ;;
}

view: paid_app_events {
  extends: [app_events]
  sql_table_name: analytics.paid_app_events ;;
}
```
```aml
// view: app_events = event_base ← device_fields ← app_events
Model app_events {
  type: 'table'
  label: 'App Events'
  data_source_name: 'demodb'
  table_name: 'analytics.app_events'

  dimension id {
    label: 'Id'
    type: 'number'
    primary_key: true
    definition: @sql {{ #SOURCE.id }};;
  }
  dimension user_id {
    label: 'User Id'
    type: 'number'
    definition: @sql {{ #SOURCE.user_id }};;
  }
  // event_base (type, sql); device_fields (description)
  dimension event_name {
    label: 'Event Name'
    type: 'text'
    description: 'Event name reported by the device'
    definition: @sql {{ #SOURCE.name }};;
  }
  // device_fields
  dimension os {
    label: 'Os'
    type: 'text'
    definition: @sql {{ #SOURCE.os }};;
  }
  measure count {
    label: 'Count'
    type: 'number'
    definition: @aql count(app_events.id);;
  }
}
```

`app_events` is concrete, so it gets its own Model. `paid_app_events` becomes a second Model with the same five
fields, `table_name: 'analytics.paid_app_events'`, and `count(paid_app_events.id)`.

## `extends` (explore)

Looker: [`extends` (for Explores)](https://docs.cloud.google.com/looker/docs/reference/param-explore-extends) ·
Holistics: [AML Dataset](https://docs.holistics.io/reference/aml/dataset)

One self-contained `Dataset` per extending explore.

- **`join` is additive.** `models` and `relationships` hold the joins of every layer, parents first.
  `access_filter`, `aggregate_table` and `query` are additive too.
- **Every other parameter** (`label`, `description`, `hidden`, `fields`, `sql_always_where`, `always_filter`,
  `persist_with`, …): the last layer that writes it wins. A parent's `hidden: yes` is inherited like any other
  parameter.
- **The base view is decided after merging:** `from`, else `view_name`, else the explore's own name
  ([explore.md](explore.md#from-view_name)). A parent that is meant to be extended normally writes `view_name`.
- **Dataset-level fields are repeated.** A field that references another view becomes a Dataset `dimension` or
  `metric` ([measure.md](measure.md)). Emit it in every flattened Dataset that contains all the models it uses.

```lookml
explore: orders_base {
  extension: required
  view_name: orders

  join: users {
    relationship: many_to_one
    sql_on: ${orders.user_id} = ${users.id} ;;
  }
}

explore: orders {
  extends: [orders_base]
  description: "One row per order."
}

explore: orders_with_items {
  extends: [orders_base]

  join: order_items {
    relationship: one_to_many
    sql_on: ${order_items.order_id} = ${orders.id} ;;
  }
}
```
```aml
// explore: orders = orders_base ← orders
Dataset orders_dataset {
  label: 'Orders'
  description: 'One row per order.'
  data_source_name: 'demodb'

  models: [orders, users]
  relationships: [
    // join: users, from orders_base
    relationship(orders.user_id > users.id, true)
  ]
}
```
```aml
// explore: orders_with_items = orders_base ← orders_with_items
Dataset orders_with_items_dataset {
  label: 'Orders With Items'
  data_source_name: 'demodb'

  models: [orders, users, order_items]
  relationships: [
    // join: users, from orders_base
    relationship(orders.user_id > users.id, true),
    // join: order_items, from orders_with_items
    relationship(order_items.order_id > orders.id, true)
  ]
}
```

No Dataset is emitted for `orders_base`.

## `extension`

Looker: [`extension` (for views)](https://docs.cloud.google.com/looker/docs/reference/param-view-extension),
[`extension` (for Explores)](https://docs.cloud.google.com/looker/docs/reference/param-explore-extension)

A view or explore with `extension: required` only exists to be extended. Emit no Model and no Dataset for it. Its
content appears in each object that extends it.

- `extension: required` is not inherited: the children are normal objects.
- **Never extended in the project** (for example a template for other projects): emit nothing, and list it in the
  conversion report.
- **Named directly by a `join` or an `explore_source`:** this is invalid LookML. Report it; do not convert that
  join or derived table.

## `view: +name`

Looker: [LookML refinements](https://docs.cloud.google.com/looker/docs/lookml-refinements) ·
Holistics: [AML Model](https://docs.holistics.io/reference/aml/model)

A refinement is one more layer of view `name`. Merge all refinements of `name` into the view, in include order and
then top to bottom within a file. Emit one Model `name`; emit nothing for the refinement blocks themselves.

- A refinement may add fields, change single parameters of existing fields, and replace `sql_table_name` or
  `derived_table`.
- **`extends` inside a refinement** adds parents. They are applied *before* the view's own body (layer 2 in
  [How to flatten](#how-to-flatten)): with `view: orders { label: "Orders" }` and
  `view: +orders { extends: [users_base] }`, the label stays "Orders" even if `users_base` writes a label.
- **Different results per model file.** If two model files include different refinements of one view, the view has
  two final forms. Emit the first as `Model <view>` and the others as `Model <view>__<model file name>`, use the
  matching one in each model file's datasets, and list them in the conversion report.
- **Wildcard includes** (`include: "/refinements/*.lkml"`) do not define an order between the matched files. If two
  refinements of one view write the same parameter and only a wildcard decides their order, report the conflict.
- **The refined view is not available** (its project was not supplied): see [Imported projects](#imported-projects).

```lookml
# imported: //shop_core/views/orders.view.lkml
view: orders {
  sql_table_name: staging.orders ;;

  dimension: id     { primary_key: yes  type: number  sql: ${TABLE}.id ;; }
  dimension: status { type: string  sql: ${TABLE}.status ;; }
  measure: count    { type: count }
}

# refinements/basic.lkml
view: +orders {
  dimension: status       { label: "Order Status" }
  measure: completed_count { type: count  filters: [status: "complete"] }
}

# refinements/finishing_touches.lkml, included after basic.lkml
view: +orders {
  final: yes
  sql_table_name: prod.orders ;;

  dimension: status { label: "Status"  description: "Fulfilment status" }
}
```
```aml
// view: orders = shop_core ← basic.lkml ← finishing_touches.lkml
Model orders {
  type: 'table'
  label: 'Orders'
  data_source_name: 'demodb'
  // finishing_touches.lkml (shop_core: staging.orders)
  table_name: 'prod.orders'

  dimension id {
    label: 'Id'
    type: 'number'
    primary_key: true
    definition: @sql {{ #SOURCE.id }};;
  }
  // shop_core (type, sql); label from finishing_touches.lkml, which overrides basic.lkml
  dimension status {
    label: 'Status'
    type: 'text'
    description: 'Fulfilment status'
    definition: @sql {{ #SOURCE.status }};;
  }
  measure count {
    label: 'Count'
    type: 'number'
    definition: @aql count(orders.id);;
  }
  // basic.lkml
  measure completed_count {
    label: 'Completed Count'
    type: 'number'
    definition: @aql count(orders.id) | where(orders.status == 'complete');;
  }
}
```

## `explore: +name`

Looker: [LookML refinements](https://docs.cloud.google.com/looker/docs/lookml-refinements) ·
Holistics: [AML Dataset](https://docs.holistics.io/reference/aml/dataset)

Same as [`view: +name`](#view-name). Merge all refinements into the explore and emit one Dataset.

- Joins of a refinement are appended. `access_filter`, `aggregate_table` and `query` are appended too.
- `label`, `description`, `hidden`, `fields`, `always_filter`, `sql_always_where`, `persist_with`, `from` and
  `view_name` are replaced.
- Convert the final explore with [explore.md](explore.md).

```lookml
# imported: //shop_core/explores/orders.explore.lkml
explore: orders {
  label: "Orders"
  description: "Core orders explore."
}

# refinements/basic.lkml
explore: +orders {
  label: "Orders (Shop)"

  join: users {
    relationship: many_to_one
    sql_on: ${orders.user_id} = ${users.id} ;;
  }
}
```
```aml
// explore: orders = shop_core ← basic.lkml
Dataset orders_dataset {
  label: 'Orders (Shop)'
  description: 'Core orders explore.'
  data_source_name: 'demodb'

  models: [orders, users]
  relationships: [
    // join: users, added by basic.lkml
    relationship(orders.user_id > users.id, true)
  ]
}
```

## `final`

Looker: [LookML refinements](https://docs.cloud.google.com/looker/docs/lookml-refinements) (section "Using `final: yes` to prevent further refinements")

Unsupported. Drop it. `final: yes` does not change the object; it only makes Looker reject any refinement applied
after this one. AML has no such marker, and no AML object can be changed in place anyway.

If a refinement of the same object *is* applied after a `final: yes` refinement, the LookML is invalid. Report it
and do not convert that object.

## Imported projects

Looker: [Importing files from other projects](https://docs.cloud.google.com/looker/docs/importing-projects)

Refinements are mostly used on views and explores of an imported project (a Looker Block, or a hub project refined
by spoke projects). AML cannot import a project, so the imported files are converted together with the importing
project ([manifest.md](manifest.md#local_dependency-remote_dependency)).

1. Get a copy of every project named by `local_dependency` or `remote_dependency`. For `remote_dependency`, use the
   commit in `ref`.
2. `include: "//project/path"` then works like a local include. An include inside an imported file resolves inside
   that file's own project.
3. Imported views and explores are ordinary layers. Flatten them with the importing project's `extends` and
   refinements on top.
4. Convert only the objects that the importing project's model files include. The result is one AML project.
5. Constants: a file of project `p` uses `p`'s constants, with the importing manifest's `override_constant` values
   applied. Substitute them before flattening.

**The imported project is not available.** A refinement or an `extends` whose base is missing cannot be flattened.
It cannot be converted field by field either: the merged field decides the type, the primary key and the SQL.
Emit nothing for that object, and list it in the conversion report with the missing path. Do not guess the base.

Difference: Looker keeps the imported project read-only and upgradable. After conversion there is no upstream: to
take a new version of the imported project, convert again.

## Conversion report

List these, so that a person can review them:

- every flattened object, with its layers (parents and refinements);
- objects with `extension: required` that nothing extends;
- views with no `sql_table_name` or `derived_table` in any layer;
- joins of the same name in two layers;
- `${EXTENDED}` or `EXTENDED*` outside `extends`;
- views with different final forms in different model files;
- refinement conflicts whose order depends on a wildcard include;
- refinements applied after `final: yes`;
- refinements and `extends` whose base is missing.
