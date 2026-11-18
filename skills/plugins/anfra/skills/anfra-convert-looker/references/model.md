# LookML model file → AML project layout

How to convert every parameter of a LookML model file (`<name>.model.lkml`). The sections follow the order of
Looker's [Model parameters](https://docs.cloud.google.com/looker/docs/reference/param-model) page.

AML has no object for a model file. Its content is spread over the project:

- `connection` → `data_source_name` on every Model and Dataset that comes from the file;
- each `explore` → one [Dataset](https://docs.holistics.io/reference/aml/dataset);
- each included `view` → one [Model](https://docs.holistics.io/reference/aml/model);
- the other settings → project constants, Holistics settings outside the code, or nothing.

```lookml
# ecommerce.model.lkml
connection: "analytics"
label: "E-Commerce"

include: "/views/*.view.lkml"

explore: orders {
  join: users {
    relationship: many_to_one
    sql_on: ${orders.user_id} = ${users.id} ;;
  }
}
```

Resulting files (see [Project structure](https://docs.holistics.io/docs/development/project)):

```
manifest.aml                                         constants (manifest.md)
models/<view folder>/<view>.model.aml                one per view (view.md)
datasets/ecommerce/orders_dataset.dataset.aml        one per explore (explore.md)
```

```aml
Dataset orders_dataset {
  label: 'Orders'
  data_source_name: 'analytics'
  models: [orders, users]
  relationships: [
    relationship(orders.user_id > users.id, true)
  ]
}
```

## Quick reference

| LookML parameter | AML | Section |
|---|---|---|
| `include` | not needed | [include](#include) |
| `explore` | `Dataset` | [explore.md](explore.md) |
| `access_grant` | unsupported | [access_grant](#access_grant) |
| `test` | unsupported | [test](#test) |
| `label` | unsupported | [label](#label) |
| `case_sensitive` | `ilike` in converted filters | [case_sensitive](#case_sensitive) |
| `connection` | `data_source_name` | [connection](#connection) |
| `datagroup` | unsupported as an object | [datagroup](#datagroup) |
| `fiscal_month_offset` | compiled into fiscal dimensions | [fiscal_month_offset](#fiscal_month_offset) |
| `persist_for`, `persist_with` | unsupported | [persist_for, persist_with](#persist_for-persist_with) |
| `week_start_day` | organization setting | [week_start_day](#week_start_day) |
| `map_layer` | unsupported | [map_layer](#map_layer) |
| `named_value_format` | `const` holding a format string | [named_value_format](#named_value_format) |
| `scoping`, `template` | drop | [Parameters to avoid](#parameters-to-avoid) |

---

## Structural parameters

### `include`

Looker: [`include`](https://docs.cloud.google.com/looker/docs/reference/param-model-include)

Not needed. AML has no import statement: every model, dataset and constant of a project is visible everywhere.
Emit nothing.

Still evaluate the `include` lines, because they decide:

- which views belong to this model file, and so which `connection` they get;
- which explores and refinements apply;
- which files come from another project (`include: "//project/…"`, see [manifest.md](manifest.md)).

Resolve the patterns the way Looker does: a leading `/` is the project root, no leading `/` is relative to the
including file, `*` and `**` are wildcards, the `.lkml` suffix is optional.

Difference: in Looker a view that is not included cannot be referenced. After conversion everything is visible to
everything.

### `explore`

Each explore becomes one Dataset, in `datasets/<model file name>/`. See [explore.md](explore.md).

### `access_grant`

Looker: [`access_grant`](https://docs.cloud.google.com/looker/docs/reference/param-model-access-grant)

Unsupported. AML has no rule that hides an explore, view or field from users by user attribute.

This is a **security-relevant loss**. List every grant, and every `required_access_grants` that uses it, in the
conversion report. Manual workarounds: share the dataset only with the matching user group, or apply
[column-level permission](https://docs.holistics.io/docs/access-control/column-level-permission) (it masks values; it
does not hide the field).

### `test`

Looker: [`test`](https://docs.cloud.google.com/looker/docs/reference/param-model-test)

Unsupported. AML has no data tests. Drop it and keep the test's LookML in the conversion report so the assertion can
be re-created elsewhere (for example as a dbt test).

## Display parameters

### `label`

Looker: [`label` (for model)](https://docs.cloud.google.com/looker/docs/reference/param-model-label)

Unsupported. Nothing in AML represents the model file, so nothing can carry its label. Use the label (or the file
name) as the name of the `datasets/<…>/` folder and keep it as a comment.

## Filter parameters

### `case_sensitive`

Looker: [`case_sensitive` (for model)](https://docs.cloud.google.com/looker/docs/reference/param-model-case-sensitive) ·
Holistics: [AQL operators](https://docs.holistics.io/reference/aql/operator)

Not a property in AML. `case_sensitive: no` changes how Looker compiles string filters. Carry it into the filters you
convert for this model file's explores: where a text filter would become `==` or `like`, emit `ilike`
(see [filter-expressions.md](filter-expressions.md)).

```lookml
case_sensitive: no

measure: paid_orders {
  type: count
  filters: [status: "paid"]
}
```
```aml
measure paid_orders {
  label: 'Paid Orders'
  type: 'number'
  definition: @aql count(orders.id) | where(orders.status ilike 'paid');;
}
```

Filters that users type in the Holistics UI are not affected by the conversion.

## Query parameters

### `connection`

Looker: [`connection`](https://docs.cloud.google.com/looker/docs/reference/param-model-connection) ·
Holistics: [AML Model](https://docs.holistics.io/reference/aml/model),
[AML Dataset](https://docs.holistics.io/reference/aml/dataset)

`connection` → `data_source_name`.

1. Ask for (or take as input) a table `Looker connection name → Holistics data source name`. The names are chosen
   independently in each tool. Without an entry, use the connection name as it is and report it.
2. Every Dataset made from an explore of this model file gets this data source.
3. Every Model gets the data source of the model file that includes its view. If two model files with different
   connections include the same view, use the first and report it; at query time the Dataset's data source overrides
   the Model's.
4. `connection: "@{NAME}"` → `data_source_name: NAME`, without quotes, with the constant declared per
   [manifest.md](manifest.md).

```lookml
connection: "analytics"
```
```aml
Model users {
  type: 'table'
  label: 'Users'
  data_source_name: 'analytics'
  table_name: 'users'
}
```

With a constant:

```lookml
connection: "@{CONNECTION_NAME}"
```
```aml
const CONNECTION_NAME = 'demodb'

Model orders {
  type: 'table'
  label: 'Orders'
  data_source_name: CONNECTION_NAME
  table_name: 'orders'
}
```

### `datagroup`

Looker: [`datagroup`](https://docs.cloud.google.com/looker/docs/reference/param-model-datagroup)

Unsupported as an object: AML has no named caching policy. Emit nothing for the block itself.

- A derived table or aggregate table that names the datagroup loses its persistence: anfra has no schedules. See
  [derived-tables.md](derived-tables.md#persistence-parameters).
- `max_cache_age` has no place in models or datasets. See [`persist_for`, `persist_with`](#persist_for-persist_with).

### `fiscal_month_offset`

Looker: [`fiscal_month_offset`](https://docs.cloud.google.com/looker/docs/reference/param-model-fiscal-month-offset)

No AML setting. Emit nothing for the parameter itself; its value is used when converting the fiscal timeframes
(`fiscal_year`, `fiscal_quarter`, …) of this model file's views. See [dimension-group.md](dimension-group.md).

### `persist_for`, `persist_with`

Looker: [`persist_for` (for model)](https://docs.cloud.google.com/looker/docs/reference/param-model-persist-for),
[`persist_with` (for model)](https://docs.cloud.google.com/looker/docs/reference/param-model-persist-with) ·
Holistics: [Data caching](https://docs.holistics.io/docs/performance/data-caching)

Unsupported. Models and datasets have no cache setting; in Holistics the cache duration is set per dashboard.
Drop both and write the intended cache age into the conversion report, for whoever rebuilds the dashboards:

- `persist_for: "N hours"` → N in minutes;
- `persist_with: g` → the `max_cache_age` of datagroup `g`, in minutes.

### `week_start_day`

Looker: [`week_start_day`](https://docs.cloud.google.com/looker/docs/reference/param-model-week-start-day) ·
Holistics: [Week Start Day](https://docs.holistics.io/docs/datetimes/week-start-day)

Nothing in AML. Emit nothing, and report the value so that an admin sets the organization's **Week Start Day** to
match. Monday is the default on both sides.

Difference: the Looker setting is per model file, the Holistics setting is per organization. If model files disagree,
only one value can be kept; report the conflict.

## Visualization and formatting parameters

### `map_layer`

Looker: [`map_layer`](https://docs.cloud.google.com/looker/docs/reference/param-model-map-layer) ·
Holistics: [Custom Map](https://docs.holistics.io/guides/map/create-custom-map)

Unsupported in AML. Custom maps in Holistics are uploaded by an admin in the UI. Report it.

### `named_value_format`

Looker: [`named_value_format`](https://docs.cloud.google.com/looker/docs/reference/param-model-named-value-format) ·
Holistics: [AML Constant](https://docs.holistics.io/reference/aml/constant),
[Number format](https://docs.holistics.io/reference/aml/number-format)

Each named format becomes a string constant named `fmt_<name>`; every field with `value_format_name: <name>` gets
`format: fmt_<name>`. How to translate the format string itself: [field.md](field.md#value_format).

```lookml
named_value_format: usd_in_millions {
  value_format: "$0.00,,\" M\""
}

measure: revenue {
  type: sum
  sql: ${TABLE}.amount ;;
  value_format_name: usd_in_millions
}
```
```aml
const fmt_usd_in_millions = '[$$]#,###0.00,,"M"'
```
```aml
measure revenue {
  label: 'Revenue'
  type: 'number'
  definition: @aql sum(orders.amount);;
  format: fmt_usd_in_millions
}
```

- If the format string cannot be translated, emit no constant, drop the format on the fields, and report it.
- `strict_value_format` → drop.
- Named formats belong to one model file; AML constants are project-wide. If two model files define the same name
  differently, give the constants different names.

## Parameters to avoid

| Parameter | Conversion |
|---|---|
| [`scoping`](https://docs.cloud.google.com/looker/docs/reference/param-model-scoping) | Removed from Looker. Drop. |
| [`template`](https://docs.cloud.google.com/looker/docs/reference/param-model-template) | Removed from Looker. Drop. |
