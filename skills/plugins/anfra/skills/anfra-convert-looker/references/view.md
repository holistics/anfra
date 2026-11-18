# LookML `view` → AML `Model`

How to convert every parameter of a LookML view. The sections follow the order of Looker's
[View parameters](https://docs.cloud.google.com/looker/docs/reference/param-view) page.

A view becomes one [AML Model](https://docs.holistics.io/reference/aml/model):

```lookml
view: orders {
  sql_table_name: public.orders ;;

  dimension: id     { primary_key: yes  type: number  sql: ${TABLE}.id ;; }
  dimension: status { type: string  sql: ${TABLE}.status ;; }
  measure: order_count { type: count }
}
```
```aml
Model orders {
  type: 'table'
  label: 'Orders'
  data_source_name: 'demodb'
  table_name: 'public.orders'

  dimension id {
    label: 'Id'
    type: 'number'
    primary_key: true
    definition: @sql {{ #SOURCE.id }};;
  }
  dimension status {
    label: 'Status'
    type: 'text'
    definition: @sql {{ #SOURCE.status }};;
  }
  measure order_count {
    label: 'Order Count'
    type: 'number'
    definition: @aql count(orders.id);;
  }
}
```

## Quick reference

| LookML parameter | AML | Section |
|---|---|---|
| `view` | `Model` | [view](#view) |
| `include` | not needed | [include](#include) |
| `test` | unsupported | [test](#test) |
| `extension`, `extends` | flattened into the model | [extends-and-refinements.md](extends-and-refinements.md) |
| `dimension`, `dimension_group`, `measure`, `filter`, `parameter` | model fields | [fields](#dimension--dimension_group--measure--filter--parameter) |
| `set`, `drill_fields` | Dataset-level drill settings | [field.md](field.md#drill_fields) |
| `calendar_definition` | unsupported | [calendar_definition](#calendar_definition) |
| `label` | `label` | [label](#label) |
| `fields_hidden_by_default` | `hidden: true` on each field | [fields_hidden_by_default](#fields_hidden_by_default) |
| `suggestions` | unsupported | [suggestions](#suggestions) |
| `autogenerate_primary_keys` | not needed | [autogenerate_primary_keys](#autogenerate_primary_keys) |
| `required_access_grants` | unsupported | [required_access_grants](#required_access_grants) |
| `sql_table_name` | `table_name` | [sql_table_name](#sql_table_name) |
| `derived_analytic_model`, `sql_analytic_model_name` | unsupported | [analytic models](#derived_analytic_model-sql_analytic_model_name) |
| `derived_table` and its subparameters | query model | [derived-tables.md](derived-tables.md) |
| `view: +name`, `final` | merged into the model | [extends-and-refinements.md](extends-and-refinements.md) |
| `distkey`, `view_label` | see section | [Parameters to avoid](#parameters-to-avoid) |

---

## Structural parameters

### `view`

Looker: [`view`](https://docs.cloud.google.com/looker/docs/reference/param-view-view) ·
Holistics: [AML Model](https://docs.holistics.io/reference/aml/model),
[Table Model](https://docs.holistics.io/reference/aml/table-model)

Each `view: <name>` that is not a refinement (`view: +name`) becomes one model.

1. **Name.** Use the view name unchanged, including its case. Every field reference and relationship is written
   against this name.
2. **Type.** `type: 'table'` with a `table_name` (see [`sql_table_name`](#sql_table_name)). A view with a
   `derived_table` becomes `type: 'query'` (see [derived-tables.md](derived-tables.md)).
3. **`data_source_name`.** A view has no connection of its own; take the `connection` of the model file that
   includes the view, and translate it to the name of the matching Holistics data source (see [model.md](model.md)).
4. **`label`.** See [`label`](#label).
5. **File.** One model per file, named `<view_name>.model.aml`, in a folder under `models/` that mirrors the view's
   LookML folder. A view declared inside a model or explore file still gets its own file.

Things to check first:

- **Two views with the same name.** In Looker this works when different model files include different ones.
  All AML objects of a project share one namespace, so a second `Model orders` fails with `Duplicated name`.
  Keep the view the converted model files use. If several are used, rename all but one (for example
  `orders__<folder>`) and rewrite the references to them. This needs a human decision.
- **A Dataset cannot have the name of a Model.** An explore named after its base view (`explore: orders`) therefore
  becomes `Dataset orders_dataset` (see [explore.md](explore.md)).
- **`extends`, `extension: required` and refinements** are resolved before this step
  (see [extends-and-refinements.md](extends-and-refinements.md)).

### `include`

Looker: [`include`](https://docs.cloud.google.com/looker/docs/reference/param-model-include)

Not needed. AML has no import statement: every model, dataset and constant of a project is visible everywhere.
Emit nothing.

Still read the `include` lines, because they tell you which views belong to which model file, and therefore which
`connection` each view gets and which of two same-named views is meant.

### `test`

Looker: [`test`](https://docs.cloud.google.com/looker/docs/reference/param-model-test)

Unsupported. AML has no data tests. Drop it and list it in the conversion report so the assertion can be re-created
elsewhere (for example as a dbt test).

### `extension`, `extends`

See [extends-and-refinements.md](extends-and-refinements.md).

### `dimension` | `dimension_group` | `measure` | `filter` | `parameter`

Each field is converted by its own rules and placed in the model body:

| Field | Doc |
|---|---|
| `dimension` | [dimension.md](dimension.md) |
| `dimension_group` | [dimension-group.md](dimension-group.md) |
| `measure` | [measure.md](measure.md) |
| `filter` | [filter.md](filter.md) |
| `parameter` | [parameter.md](parameter.md) |

Two placement rules belong to the view:

- A field whose `sql` references another view (`${other_view.field}`) cannot live in the model. It becomes a
  Dataset-level `dimension` or `metric` (see [sql-references.md](sql-references.md)).
- `filter` and `parameter` fields are not dimensions or measures. Do not emit them as model fields
  (see [filter.md](filter.md), [parameter.md](parameter.md)).

### `set`, `drill_fields`

See [`drill_fields`](field.md#drill_fields). A `set` is expanded during conversion and has no AML counterpart.

## Custom calendar parameters

### `calendar_definition`

Looker: [`calendar_definition`](https://docs.cloud.google.com/looker/docs/reference/param-view-calendar-definition)

Unsupported.

## Display parameters

### `label`

Looker: [`label` (for view)](https://docs.cloud.google.com/looker/docs/reference/param-view-label) ·
Holistics: [AML Model](https://docs.holistics.io/reference/aml/model)

`label: "X"` → `label: 'X'` on the model. Without a label, use the view name in title case
(`order_items` → `'Order Items'`), which is what Looker shows by default.

```lookml
view: users {
  label: "Customers"
}
```
```aml
Model users {
  type: 'table'
  label: 'Customers'
  data_source_name: 'demodb'
  table_name: 'users'
}
```

Difference: a model has one label everywhere. Looker can relabel a view per explore or join (`view_label`); that is
handled in [explore.md](explore.md).

### `fields_hidden_by_default`

Looker: [`fields_hidden_by_default`](https://docs.cloud.google.com/looker/docs/reference/param-view-fields-hidden-by-default) ·
Holistics: [AML Field](https://docs.holistics.io/reference/aml/field)

AML has no model-level switch. With `fields_hidden_by_default: yes`, add `hidden: true` to every field of the view
that does not say `hidden: no`.

```lookml
view: users {
  fields_hidden_by_default: yes

  dimension: id    { type: number }
  dimension: email { type: string  hidden: no }
}
```
```aml
Model users {
  type: 'table'
  label: 'Users'
  data_source_name: 'demodb'
  table_name: 'users'

  dimension id {
    label: 'Id'
    type: 'number'
    hidden: true
  }
  dimension email {
    label: 'Email'
    type: 'text'
  }
}
```

## Filter parameters

### `suggestions`

Looker: [`suggestions` (for view)](https://docs.cloud.google.com/looker/docs/reference/param-view-suggestions)

Unsupported. Drop it.

## Query parameters

### `autogenerate_primary_keys`

Looker: [`autogenerate_primary_keys`](https://docs.cloud.google.com/looker/docs/reference/param-explore-autogenerate-primary-keys)

Not needed. Drop it. If the view declares no primary key, a person still has to pick one: a `count` measure and some
relationships need a `primary_key: true` dimension (see [measure.md](measure.md)).

### `required_access_grants`

Looker: [`required_access_grants` (for view)](https://docs.cloud.google.com/looker/docs/reference/param-view-required-access-grants)

Unsupported. AML cannot hide a whole model from some users.

This is a **security-relevant loss**: after conversion the view is visible to everyone who can open a dataset that
contains it. List every such view in the conversion report. Manual workarounds: restrict who the datasets are shared
with, or apply [column-level permission](https://docs.holistics.io/docs/access-control/column-level-permission) to the
sensitive dimensions.

### `sql_table_name`

Looker: [`sql_table_name` (for view)](https://docs.cloud.google.com/looker/docs/reference/param-view-sql-table-name) ·
Holistics: [Table Model](https://docs.holistics.io/reference/aml/table-model)

`sql_table_name` → `table_name`, a required property of a table model. Trim the value, then pick the matching form:

| `sql_table_name` | AML |
|---|---|
| absent | `table_name: '<view_name>'` (Looker's default) |
| plain name: `orders`, `public.orders`, `db.schema.orders` | the same text: `table_name: 'public.orders'` |
| quoted identifiers: `"PUBLIC"."USERS"`, `` `proj.ds.tbl` ``, `[dbo].[Orders]` | the same text, quotes included: `table_name: '"PUBLIC"."USERS"'` |
| contains `@{constant}` | `${constant}` interpolation — [below](#with-a-lookml-constant) |
| `-- if dev --` / `-- if prod --` | AML `if` on `H.git.is_production` — [below](#development-and-production-tables) |
| `{{ _user_attributes['x'] }}` | `H.current_user.x` — [below](#with-user-attributes) |
| other Liquid (`{% parameter %}`, `{% if view.field._in_query %}`) | unsupported — [below](#other-liquid) |
| a subquery `( SELECT … )` | a query model — [below](#subquery) |
| `UNNEST(…)`, a table function | unsupported as a stand-alone model; nested-record views exist only inside a join (see [explore.md](explore.md)) |

Always write the value in a single-quoted AML string, and escape `'` as `\'`, `\` as `\\`, and a literal `${` as
`\${`. Do not use double quotes for the string: the value itself often contains them.

An unqualified name (`orders`) resolves against the default schema of the Holistics data source, as it did against
the default schema of the Looker connection. Report such names so the two defaults can be compared.

#### With a LookML constant

`@{NAME}` → `${NAME}`; the constant itself becomes an [AML constant](https://docs.holistics.io/reference/aml/constant)
(see [manifest.md](manifest.md)). See [string interpolation](https://docs.holistics.io/reference/aml/string-interpolation).

```lookml
constant: SCHEMA { value: "analytics" }

view: payments {
  sql_table_name: @{SCHEMA}.payments ;;
}
```
```aml
const SCHEMA = 'analytics'

Model payments {
  type: 'table'
  label: 'Payments'
  data_source_name: 'demodb'
  table_name: '${SCHEMA}.payments'
}
```

#### Development and production tables

Looker uses the `-- if dev --` line in Development Mode and the `-- if prod --` line in production. Use an
[AML `if`](https://docs.holistics.io/reference/aml/if-else) on
[`H.git.is_production`](https://docs.holistics.io/reference/aml/user-attributes-and-variables). If both lines name
the same table, emit the plain name instead.

```lookml
view: run_results {
  sql_table_name:
  -- if dev -- dev_artifacts.run_results
  -- if prod -- prod_artifacts.run_results
  ;;
}
```
```aml
const run_results__table_name =
  if (H.git.is_production) { 'prod_artifacts.run_results' }
  else { 'dev_artifacts.run_results' }

Model run_results {
  type: 'table'
  label: 'Run Results'
  data_source_name: 'demodb'
  table_name: run_results__table_name
}
```

Difference: Looker switches per user session; `H.git.is_production` switches per workspace mode. Review these models.

#### With user attributes

`{{ _user_attributes['x'] }}` → `H.current_user.x`. The user attribute must exist in Holistics under the same name.
See [Dynamic Schemas](https://docs.holistics.io/docs/development/dynamic-schema).

```lookml
view: activity {
  sql_table_name:
  -- if dev -- {{ _user_attributes['dbt_dev_schema'] }}.activity
  -- if prod -- {{ _user_attributes['dbt_prod_schema'] }}.activity
  ;;
}
```
```aml
const dbt_schema =
  if (H.git.is_production) { H.current_user.dbt_prod_schema }
  else { H.current_user.dbt_dev_schema }

Model activity {
  type: 'table'
  label: 'Activity'
  data_source_name: 'demodb'
  table_name: '${dbt_schema}.activity'
}
```

When many views use the same schema expression, declare the constant once and reuse it.

#### Other Liquid

Unsupported. AML cannot choose a table per query. Emit the last `{% else %}` branch (or the parameter's
`default_value`) as a plain name, and list the view in the conversion report for a person to review.

#### Subquery

A parenthesised `SELECT` is not a table. Emit a [query model](https://docs.holistics.io/reference/aml/query-model)
with the outer parentheses removed (see [derived-tables.md](derived-tables.md)).

```lookml
view: paid_orders {
  sql_table_name: (SELECT * FROM public.orders WHERE status = 'paid') ;;
}
```
```aml
Model paid_orders {
  type: 'query'
  label: 'Paid Orders'
  data_source_name: 'demodb'
  models: []
  query: @sql SELECT * FROM public.orders WHERE status = 'paid' ;;
}
```

### `derived_analytic_model`, `sql_analytic_model_name`

Looker: [`derived_analytic_model`](https://docs.cloud.google.com/looker/docs/reference/param-view-derived-analytic-model),
[`sql_analytic_model_name`](https://docs.cloud.google.com/looker/docs/reference/param-view-sql-analytic-model-name)

Unsupported.

## Derived table parameters

`derived_table` and all of its subparameters (`sql`, `explore_source`, `datagroup_trigger`, `sql_trigger_value`,
`interval_trigger`, `persist_for`, `increment_key`, `increment_offset`, `create_process`, `sql_create`,
`materialized_view`, `publish_as_db_view`, `cluster_keys`, `partition_keys`, `indexes`, `sortkeys`, `distribution`,
`distribution_style`, `table_compression`, `table_format`): see [derived-tables.md](derived-tables.md).

## Refinement parameters

`view: +view_name` and `final`: see [extends-and-refinements.md](extends-and-refinements.md).

## Parameters to avoid

| Parameter | Conversion |
|---|---|
| [`view_label`](https://docs.cloud.google.com/looker/docs/reference/param-view-view-label) (view level) | Same as [`label`](#label). Use it as the model `label` only when the view has no `label`. |
| [`distkey`](https://docs.cloud.google.com/looker/docs/reference/param-view-distkey) | Treat as `distribution` (see [derived-tables.md](derived-tables.md)). |
