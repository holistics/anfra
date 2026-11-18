# LookML `explore` and `join` → AML `Dataset` and relationships

How to convert every parameter of a LookML explore and of its joins. The sections follow the order of Looker's
[Explore parameters](https://docs.cloud.google.com/looker/docs/reference/param-explore) and
[Join parameters](https://docs.cloud.google.com/looker/docs/reference/param-join) pages.

An explore becomes one [AML Dataset](https://docs.holistics.io/reference/aml/dataset). Each join becomes one
[relationship](https://docs.holistics.io/reference/aml/relationship) of that dataset.

```lookml
explore: order_items {
  label: "Orders & Items"
  description: "One row per item sold."

  join: orders {
    sql_on: ${order_items.order_id} = ${orders.id} ;;
    relationship: many_to_one
  }
  join: users {
    sql_on: ${orders.user_id} = ${users.id} ;;
    relationship: many_to_one
  }
}
```
```aml
Dataset order_items_dataset {
  label: 'Orders & Items'
  description: 'One row per item sold.'
  data_source_name: 'demodb'
  models: [order_items, orders, users]
  relationships: [
    relationship(order_items.order_id > orders.id, true),
    relationship(orders.user_id > users.id, true)
  ]
}
```

## The main difference: a dataset has no base view

Read this before converting; it applies to every explore.

- A Looker explore is a tree. Every query starts `FROM <base view>` and joins outward, so every result is limited to
  rows that can be reached from the base view.
- A Holistics dataset is a set of models and relationships with no root. Holistics picks the tables and the joins
  per query. See [Relationships vs joins](https://docs.holistics.io/docs/joins/relationships-vs-joins).

So a query that uses only fields of a joined model can return more rows than in Looker. In the example above, a count
of `users` alone is "users with at least one order item" in Looker and "all users" in Holistics. Nothing in a
dataset can pin the base view. Tell the user about this difference in the conversion report.

## Quick reference

Explore parameters:

| LookML parameter | AML | Section |
|---|---|---|
| `explore` | `Dataset` | [explore](#explore) |
| `extension`, `extends` | flattened into the dataset | [extends-and-refinements.md](extends-and-refinements.md) |
| `fields` | Dataset `view` | [fields](#fields) |
| `tags` | unsupported | [tags](#tags) |
| `description` | `description` | [description](#description) |
| `group_label` | unsupported | [group_label](#group_label) |
| `hidden` | unsupported | [hidden](#hidden) |
| `label` | `label` | [label](#label) |
| `query` | unsupported | [query](#query) |
| `view_label` | `group` in the Dataset `view` | [view_label](#view_label) |
| `access_filter` | `permission` | [access_filter](#access_filter) |
| `always_filter`, `conditionally_filter` | unsupported | [always_filter](#always_filter-conditionally_filter) |
| `case_sensitive` | unsupported | [case_sensitive](#case_sensitive) |
| `sql_always_having` | unsupported | [sql_always_having](#sql_always_having) |
| `sql_always_where` | unsupported, with workarounds | [sql_always_where](#sql_always_where) |
| `always_join` | unsupported | [always_join](#always_join) |
| `join` | relationship | [Join parameters](#join-parameters) |
| `autogenerate_primary_keys` | not needed | [autogenerate_primary_keys](#autogenerate_primary_keys) |
| `cancel_grouping_fields` | unsupported | [cancel_grouping_fields](#cancel_grouping_fields) |
| `from`, `view_name` | base model, or an alias model | [from, view_name](#from-view_name) |
| `persist_for`, `persist_with` | unsupported | [persist_for, persist_with](#persist_for-persist_with) |
| `required_access_grants` | unsupported | [required_access_grants](#required_access_grants) |
| `sql_table_name` | alias model with `table_name` | [sql_table_name](#sql_table_name) |
| `sql_analytic_model_name` | unsupported | [sql_analytic_model_name](#sql_analytic_model_name) |
| `sql_preamble` | unsupported | [sql_preamble](#sql_preamble) |
| `symmetric_aggregates` | not needed | [symmetric_aggregates](#symmetric_aggregates) |
| `aggregate_table` | unsupported | [aggregate-awareness.md](aggregate-awareness.md) |
| `explore: +name`, `final` | merged into the dataset | [extends-and-refinements.md](extends-and-refinements.md) |

Join parameters:

| LookML parameter | AML | Section |
|---|---|---|
| `join` | one model + one relationship | [join](#join) |
| `view_label` | alias model `label`, or a `view` group | [view_label (join)](#view_label-join) |
| `fields` | Dataset `view` | [fields (join)](#fields-join) |
| `foreign_key` | relationship to the primary key | [foreign_key](#foreign_key) |
| `from` | alias model | [from](#from) |
| `outer_only` | drop | [outer_only](#outer_only) |
| `relationship` | relationship type and direction | [relationship](#relationship) |
| `required_joins` | not needed | [required_joins](#required_joins) |
| `sql_on` | relationship keys | [sql_on](#sql_on) |
| `sql_table_name` | alias model with `table_name` | [sql_table_name (join)](#sql_table_name-join) |
| `type` | `nullable`; `full_outer` and `cross` unsupported | [type](#type) |
| `required_access_grants` | unsupported | [required_access_grants (join)](#required_access_grants-join) |
| `sql_where` | relationship `where` | [sql_where](#sql_where) |
| `sql`, `sql_foreign_key` | see section | [Join parameters to avoid](#join-parameters-to-avoid) |

---

# Explore parameters

## Structural parameters

### `explore`

Looker: [`explore`](https://docs.cloud.google.com/looker/docs/reference/param-explore-explore) ·
Holistics: [AML Dataset](https://docs.holistics.io/reference/aml/dataset)

Resolve `extends` and refinements first ([extends-and-refinements.md](extends-and-refinements.md)). An explore with
`extension: required` produces no dataset. Then, for each explore:

1. **Name.** `Dataset <explore>_dataset`. The suffix is required: a Dataset cannot have the name of a Model, and an
   explore is usually named after its base view. If two model files define an explore of the same name, use
   `<model file name>__<explore>_dataset`.
2. **`label`, `description`.** See [`label`](#label), [`description`](#description).
3. **`data_source_name`.** From the `connection` of the model file ([model.md](model.md)).
4. **`models`.** The base view's model first ([`from`, `view_name`](#from-view_name)), then one entry per join in
   LookML order. The order only affects how the models are listed in the UI.
5. **`relationships`.** One entry per join ([Join parameters](#join-parameters)).
6. **File.** `datasets/<model file name>/<explore>_dataset.dataset.aml`.

The validator does not check that the fields named in `relationships`, `view` and `permission` exist. Check every
one of them yourself against the converted models.

### `extension`, `extends`

See [extends-and-refinements.md](extends-and-refinements.md).

### `fields`

Looker: [`fields` (for Explores)](https://docs.cloud.google.com/looker/docs/reference/param-explore-fields) ·
Holistics: [Custom dataset views](https://docs.holistics.io/docs/datasets/custom-views)

`fields` (on the explore and on its joins) → a `view` block in the dataset.

1. Compute the exposed fields as Looker does. First each join: no `fields` → all fields of the view;
   `fields: [a, b, set*]` → those fields; `fields: []` → none. Then the explore: `ALL_FIELDS*` is the result of the
   first step, `-view.field` removes a field, `view*` is all fields of a view. Expand `set*` names with the view's
   `set` definitions.
2. Emit `view { … }` only when some model is not fully exposed:
   - all fields exposed → `model m { }`;
   - some fields exposed → `model m { field a  field b }` (`field` covers dimensions and measures);
   - no field exposed → leave the model out of `view`, but keep it in `models` (other joins may go through it).

```lookml
explore: order_items {
  fields: [ALL_FIELDS*, -users.email]

  join: orders { … }
  join: users { … }
  join: user_facts { fields: [lifetime_orders]  … }
  join: products { fields: []  … }
}
```
```aml
view {
  model order_items { }
  model orders { }
  model users {
    field id
    field name
    field region
    field count
  }
  model user_facts {
    field lifetime_orders
  }
}
```

Difference: a custom view only organises the field list, and users can switch back to the original view. Looker's
`fields` removes the field from the explore. If the exclusion is there to restrict access, report it; use
[column-level permission](https://docs.holistics.io/docs/access-control/column-level-permission) for real access control.

### `tags`

Looker: [`tags`](https://docs.cloud.google.com/looker/docs/reference/param-explore-tags)

Unsupported. Drop.

## Display parameters

### `description`

Looker: [`description` (for Explores)](https://docs.cloud.google.com/looker/docs/reference/param-explore-description)

`description: "…"` → `description: '…'` on the dataset, unchanged.

### `group_label`

Looker: [`group_label` (for Explores)](https://docs.cloud.google.com/looker/docs/reference/param-explore-group-label)

Unsupported. A dataset has no grouping property. Put the dataset file in a folder named after the group label and
keep the value as a comment.

### `hidden`

Looker: [`hidden` (for Explores)](https://docs.cloud.google.com/looker/docs/reference/param-explore-hidden)

Unsupported. Still convert the explore (hidden explores usually feed dashboards or derived tables) and add the
comment `// hidden in Looker`. Manual equivalent: do not [share the dataset](https://docs.holistics.io/docs/datasets)
with explorers and viewers.

### `label`

Looker: [`label` (for Explores)](https://docs.cloud.google.com/looker/docs/reference/param-explore-label)

`label: "X"` → `label: 'X'` on the dataset. Without a label, use the explore name with underscores replaced by
spaces and each word capitalised (`order_items` → `'Order Items'`).

### `query`

Looker: [`query`](https://docs.cloud.google.com/looker/docs/reference/param-explore-query)

Unsupported. A "Quick Start" query is saved content, not part of the semantic layer. Emit nothing; list each query
(label, fields, filters, sorts, limit, pivots) in the conversion report so it can be rebuilt as a report or dashboard.

### `view_label`

Looker: [`view_label` (for Explores)](https://docs.cloud.google.com/looker/docs/reference/param-explore-view-label),
[`view_label` (for joins)](https://docs.cloud.google.com/looker/docs/reference/param-explore-join-view-label) ·
Holistics: [Custom dataset views](https://docs.holistics.io/docs/datasets/custom-views)

The same rule covers the explore-level and the join-level parameter:

| Case | AML |
|---|---|
| The join has `from` (it is an alias) | `label` of the alias model ([`from`](#from)) |
| Two or more views of the explore share the same `view_label` | a `group` in the dataset `view` that holds those models |
| One view, and the value equals the model's label | nothing |
| One view, another value | a one-model `group` in the dataset `view` |
| `view_label: ""` | drop |

The group name is the label in snake case. A group has a name but no `label` property.

```lookml
explore: order_items {
  join: users      { view_label: "Customers"  … }
  join: user_facts { view_label: "Customers"  … }
}
```
```aml
view {
  model order_items { }
  group customers {
    model users { }
    model user_facts { }
  }
}
```

Difference: Looker merges the fields of both views into one list under one heading. Holistics shows a group that
contains two models. `fields` and `view_label` write the same `view` block; combine them.

## Filter parameters

### `access_filter`

Looker: [`access_filter`](https://docs.cloud.google.com/looker/docs/reference/param-explore-access-filter) ·
Holistics: [Row-level permission as code](https://docs.holistics.io/docs/access-control/row-level-permission-as-code)

Each `access_filter` → one `permission` block in the dataset, named `<view>_<field>__<attribute>`.

```lookml
access_filter: {
  field: users.region
  user_attribute: sales_region
}
```
```aml
permission users_region__sales_region {
  field: r(users.region)
  operator: 'matches_user_attribute'
  value: 'sales_region'
}
```

The user attribute must be created in Holistics under the same name, and the users' values moved over. That is an
admin task outside AML; put it in the conversion report.

Differences to report:

- Looker applies the filter to everyone, admins included. Holistics does not enforce row-level permissions for
  admins, or for analysts who have access to the data source.
- A Looker attribute value can be a filter expression (`%`, `>=0`). A Holistics attribute holds a list of values or `All`.
- Row-level permission as code is not available on every Holistics plan.

### `always_filter`, `conditionally_filter`

Looker: [`always_filter`](https://docs.cloud.google.com/looker/docs/reference/param-explore-always-filter),
[`conditionally_filter`](https://docs.cloud.google.com/looker/docs/reference/param-explore-conditionally-filter)

Unsupported. A dataset has no required or default filter; in Holistics, filters live on dashboards.

Emit no AML. Write each `field: "expression"` pair (and the `unless` list) as a comment above the dataset and in the
conversion report, so that the dashboards built on this dataset get a default filter. Convert the expression with
[filter-expressions.md](filter-expressions.md). If the filtered field is a LookML `filter` or `parameter` field, see
[liquid.md](liquid.md).

### `case_sensitive`

Looker: [`case_sensitive` (for Explores)](https://docs.cloud.google.com/looker/docs/reference/param-explore-case-sensitive)

Unsupported as a dataset setting. Drop it and report it. For the filters you convert for this explore, see
`case_sensitive` in [model.md](model.md#case_sensitive).

### `sql_always_having`

Looker: [`sql_always_having`](https://docs.cloud.google.com/looker/docs/reference/param-explore-sql-always-having)

Unsupported. Add a `// TODO sql_always_having: <original>` comment and report it.

### `sql_always_where`

Looker: [`sql_always_where`](https://docs.cloud.google.com/looker/docs/reference/param-explore-sql-always-where)

Unsupported. A dataset has no fixed, invisible filter. Pick a workaround by the shape of the condition:

| Condition | Conversion |
|---|---|
| `${view.field} = '{{ _user_attributes['x'] }}'` | It is row-level security: a `permission` block, as for [`access_filter`](#access_filter) |
| A constant condition on base-view fields (`NOT ${events.is_test}`) | A wrapping query model (below); or, if every explore on that view applies the same condition, put the condition into the model itself |
| A condition on a joined view | Only if the condition can be put into that model; otherwise unsupported |
| Other Liquid | [liquid.md](liquid.md) |

If no workaround fits, emit the dataset without the filter, add `// TODO sql_always_where: <original>`, and report
it. A dashboard filter is not equivalent: users can change it.

The wrapping query model selects from the base model with the condition, re-declares the base model's fields, and
replaces the base model in this dataset only:

```lookml
explore: events {
  sql_always_where: NOT ${events.is_test} ;;
}
```
```aml
Model events__always_where {
  type: 'query'
  label: 'Events'
  data_source_name: 'demodb'
  models: [events]
  query: @sql
    select
      {{ #e.id }} as id,
      {{ #e.user_id }} as user_id,
      {{ #e.occurred_at }} as occurred_at
    from {{ #events as e }}
    where {{ #e.is_test }} = FALSE
  ;;

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
  dimension occurred_at {
    label: 'Occurred At'
    type: 'datetime'
    definition: @sql {{ #SOURCE.occurred_at }};;
  }
  measure count {
    label: 'Count'
    type: 'number'
    definition: @aql count(events__always_where.id);;
  }
}
```

Difference: the wrapper restricts the base model only. A query that uses only a joined model is not restricted,
while in Looker everything is seen through the filtered base view.

## Join parameters

### `always_join`

Looker: [`always_join`](https://docs.cloud.google.com/looker/docs/reference/param-explore-always-join)

Unsupported. Holistics joins only the models a query needs. Add a `// TODO always_join` comment and report it.
When it is used with an inner join to restrict rows, treat the intent like [`sql_always_where`](#sql_always_where).

### `join`

See [Join parameters](#join-parameters).

## Query parameters

### `autogenerate_primary_keys`

Looker: [`autogenerate_primary_keys`](https://docs.cloud.google.com/looker/docs/reference/param-explore-autogenerate-primary-keys)

Not needed. Drop it. A model without a primary key still needs one chosen by a person when it has a `count` measure
or is part of a range or many-to-many relationship.

### `cancel_grouping_fields`

Looker: [`cancel_grouping_fields`](https://docs.cloud.google.com/looker/docs/reference/param-explore-cancel-grouping-fields)

Unsupported. Drop.

### `from`, `view_name`

Looker: [`from` (for Explores)](https://docs.cloud.google.com/looker/docs/reference/param-explore-from),
[`view_name`](https://docs.cloud.google.com/looker/docs/reference/param-explore-view-name)

They decide which view is the base view, and under which name its fields are referenced:

| LookML | Base view | Fields referenced as |
|---|---|---|
| neither | the explore name | the explore name |
| `view_name: v` | `v` | `v` |
| `from: v` | `v` | the explore name |
| `from: v` + `view_name: w` | `v` | `w` |

- Same name for both → use the view's model directly.
- Different names → create an alias model, exactly as for a join with [`from`](#from), and use it in the dataset.

The base view is listed first in `models`. It gets no other marker.

### `persist_for`, `persist_with`

Unsupported. See [model.md](model.md#persist_for-persist_with).

### `required_access_grants`

Looker: [`required_access_grants` (for Explores)](https://docs.cloud.google.com/looker/docs/reference/param-explore-required-access-grants)

Unsupported. This is a **security-relevant loss**: add a comment with the grant definitions and report it. Manual
equivalent: [share the dataset](https://docs.holistics.io/docs/datasets) only with the user group that matches the grants.

### `sql_table_name`

Looker: [`sql_table_name` (for Explores)](https://docs.cloud.google.com/looker/docs/reference/param-explore-sql-table-name)

Same as [`sql_table_name` on a join](#sql_table_name-join): an alias model of the base view with a `table_name` override.

### `sql_analytic_model_name`

Unsupported.

### `sql_preamble`

Looker: [`sql_preamble`](https://docs.cloud.google.com/looker/docs/reference/param-explore-sql-preamble)

Unsupported. Holistics runs no statement before a query. Temporary functions defined there must be created in the
warehouse.

### `symmetric_aggregates`

Looker: [`symmetric_aggregates`](https://docs.cloud.google.com/looker/docs/reference/param-explore-symmetric-aggregates)

Not needed. Drop it. Holistics always prevents fan-out for AQL measures, from the declared relationships
(see [How joins work](https://docs.holistics.io/docs/joins/how-joins-work)).

If the explore sets `symmetric_aggregates: no` and has a `one_to_many` or `many_to_many` join, its Looker numbers
were inflated on purpose; the converted dataset returns de-duplicated numbers. Report the difference.

## Aggregate table parameters

`aggregate_table`: unsupported. See [aggregate-awareness.md](aggregate-awareness.md).

## Refinement parameters

`explore: +explore_name` and `final`: see [extends-and-refinements.md](extends-and-refinements.md).

## Explore parameters to avoid

| Parameter | Conversion |
|---|---|
| `access_filter_fields` | Removed from Looker. If it is still present, report it for manual review; it was a row-level restriction (see [`access_filter`](#access_filter)). |

---

# Join parameters

### `join`

Looker: [`join`](https://docs.cloud.google.com/looker/docs/reference/param-explore-join) ·
Holistics: [AML Relationship](https://docs.holistics.io/reference/aml/relationship)

Each join adds one model to `models` and one relationship to `relationships`. For each join, find:

- **`J`** — the joined view: the join name, or its alias model when the join has [`from`](#from).
- **`O`** — the other view named in the join condition. It is the base view or an earlier join. Do not assume it
  is the base view: joins often attach to another joined view.

Then build the relationship in three steps:

1. keys from [`sql_on`](#sql_on) or [`foreign_key`](#foreign_key);
2. type and direction from [`relationship`](#relationship);
3. options from [`type`](#type).

Set every relationship to active (`true` as the second argument). A converted explore is a tree, so there is one
path between any two models.

## Display parameters

### `view_label` (join)

See [`view_label`](#view_label).

## Join parameters

### `fields` (join)

Looker: [`fields` (for joins)](https://docs.cloud.google.com/looker/docs/reference/param-explore-join-fields)

See [`fields`](#fields).

### `foreign_key`

Looker: [`foreign_key`](https://docs.cloud.google.com/looker/docs/reference/param-explore-join-foreign-key)

The key pair is the named dimension and the primary key of the joined view.

- `foreign_key: dim` → `O` is the base view; `foreign_key: other.dim` → `O` is `other`.
- The other side is the dimension of `J` marked `primary_key: yes`. If there is none, the join needs manual review.

```lookml
explore: order_items {
  join: products { foreign_key: product_id }
}
```
```aml
relationship(order_items.product_id > products.id, true)
```

### `from`

Looker: [`from` (for joins)](https://docs.cloud.google.com/looker/docs/reference/param-explore-join-from) ·
Holistics: [AML Extend](https://docs.holistics.io/reference/aml/extend),
[Role-playing dimensions](https://docs.holistics.io/docs/modeling/modeling-patterns/role-playing-dimensions)

`join: A { from: V }` joins view `V` under the name `A`. Create an alias model `A` that extends `V`, and use `A` in
the dataset wherever the LookML says `${A.…}`.

1. `label` ← the join's `view_label`; otherwise the alias name in title case.
2. **Re-declare every field of `V` whose definition is `@aql`**, with `V.` replaced by `A.`. `extend` does not
   rewrite the model name inside AQL, so an inherited `count(airports.code)` fails in a dataset that only contains
   the alias. `@sql` definitions need no change.
3. Emit the alias model once and reuse it in other explores that use the same alias for the same view. If the name
   is taken by a view, or by an alias of a different view, name it `<explore>__<alias>`.
4. Do not add `V` itself to the dataset unless it is also joined without an alias.

```lookml
explore: flights {
  join: origin {
    from: airports
    view_label: "Origin Airport"
    sql_on: ${flights.origin_code} = ${origin.code} ;;
    relationship: many_to_one
  }
  join: destination {
    from: airports
    sql_on: ${flights.destination_code} = ${destination.code} ;;
    relationship: many_to_one
  }
}
```
```aml
Model origin = airports.extend({
  label: 'Origin Airport'
  measure count { definition: @aql count(origin.code);; }
})

Model destination = airports.extend({
  label: 'Destination'
  measure count { definition: @aql count(destination.code);; }
})

Dataset flights_dataset {
  label: 'Flights'
  data_source_name: 'demodb'
  models: [flights, origin, destination]
  relationships: [
    relationship(flights.origin_code > origin.code, true),
    relationship(flights.destination_code > destination.code, true)
  ]
}
```

### `outer_only`

Looker: [`outer_only`](https://docs.cloud.google.com/looker/docs/reference/param-explore-join-outer-only)

Drop.

### `relationship`

Looker: [`relationship`](https://docs.cloud.google.com/looker/docs/reference/param-explore-join-relationship) ·
Holistics: [AML Relationship](https://docs.holistics.io/reference/aml/relationship)

LookML states the cardinality from `O` to `J`. AML has no `one_to_many`: the "many" side is always written first.

| LookML `relationship` | AML |
|---|---|
| `many_to_one` (also the default when omitted) | `relationship(O.x > J.y, true)` |
| `one_to_many` | flipped: `relationship(J.y > O.x, true)` |
| `one_to_one` | `relationship(O.x - J.y, true)` |
| `many_to_many` | a `ManyToManyRelationship` block — see [non-equality conditions](#non-equality-conditions) |

```lookml
explore: orders {
  join: shipments {
    sql_on: ${orders.id} = ${shipments.order_id} ;;
    relationship: one_to_many
  }
  join: users {
    sql_on: ${orders.user_id} = ${users.id} ;;
    relationship: many_to_one
  }
  join: user_facts {
    sql_on: ${users.id} = ${user_facts.user_id} ;;
    relationship: one_to_one
  }
}
```
```aml
relationship(shipments.order_id > orders.id, true),
relationship(orders.user_id > users.id, true),
relationship(users.id - user_facts.user_id, true)
```

Both tools trust the declaration. A wrong `relationship` in LookML stays wrong after conversion.

### `required_joins`

Looker: [`required_joins`](https://docs.cloud.google.com/looker/docs/reference/param-explore-join-required-joins)

Not needed. Drop it. Once each relationship is attached to the right model, Holistics joins the models in between
on its own. Use the listed views as a hint for finding `O` when the join condition is raw SQL.

### `sql_on`

Looker: [`sql_on`](https://docs.cloud.google.com/looker/docs/reference/param-explore-join-sql-on) ·
Holistics: [AML Relationship](https://docs.holistics.io/reference/aml/relationship)

Remove SQL comments and outer parentheses, split the condition on top-level `AND`, and classify each part:

| Part | Example | Conversion |
|---|---|---|
| key pair | `${a.x} = ${b.y}` | [equality](#equality-on-one-or-more-key-pairs) |
| constant condition on one view | `${shipments.kind} = 'outbound'` | [`where` filter](#equality-with-a-constant-condition) |
| equality with a SQL expression on a side | `lower(${a.email}) = ${b.email}` | [helper dimension](#equality-on-a-sql-expression) |
| `<`, `>`, `BETWEEN`, `OR` between two views | `${e.at} >= ${s.started_at}` | [range / many-to-many](#non-equality-conditions) |
| anything else | Liquid, three views, `TRUE` | [unsupported shapes](#unsupported-shapes) |

Each `${view.field}` must be a dimension of the converted model. A raw `view.column` reference counts as a field
reference when a matching dimension exists.

#### Equality on one or more key pairs

One key pair → the short form:

```lookml
join: orders {
  sql_on: ${order_items.order_id} = ${orders.id} ;;
  relationship: many_to_one
}
```
```aml
relationship(order_items.order_id > orders.id, true)
```

Several key pairs → the long form: the first pair in `from` / `to`, each further pair in an `on` block. List every
pair; a missing pair silently changes the cardinality.

#### Equality with a constant condition

A condition on one view goes into the `where` block, on the side (`from` or `to`) of the model it refers to.
Translate simple comparisons to AQL (`=` → `==`, `<>` → `!=`). For anything else, put the SQL unchanged into a
hidden `truefalse` dimension of that model and reference the dimension. Drop a condition that names no view (`1 = 1`).

```lookml
join: shipments {
  sql_on: ${orders.id} = ${shipments.order_id}
      AND ${orders.warehouse_id} = ${shipments.warehouse_id}
      AND ${shipments.kind} = 'outbound' ;;
  relationship: one_to_many
}
```
```aml
RelationshipConfig {
  rel: EqualityRelationship {
    type: 'many_to_one'
    from: FieldRef { model: 'shipments', field: 'order_id' }
    to: FieldRef { model: 'orders', field: 'id' }
    on {
      from: FieldRef { model: 'shipments', field: 'warehouse_id' }
      to: FieldRef { model: 'orders', field: 'warehouse_id' }
    }
    where {
      from: @aql shipments.kind == 'outbound' ;;
    }
  }
  active: true
}
```

#### Equality on a SQL expression

When a side of the equality is an expression over one view, move the expression unchanged into a hidden dimension
of that model, named `join_key__<other view>`, and use it as the key.

```lookml
join: accounts {
  sql_on: lower(split_part(${events.email}, '@', 2)) = ${accounts.domain} ;;
  relationship: many_to_one
}
```
```aml
dimension join_key__accounts {
  label: 'Join Key (accounts)'
  type: 'text'
  hidden: true
  definition: @sql lower(split_part({{ email }}, '@', 2));;
}
```
```aml
relationship(events.join_key__accounts > accounts.domain, true)
```

The dimension goes into `Model events`; the relationship into the dataset. An expression that mixes two views
cannot become a model dimension: see [unsupported shapes](#unsupported-shapes).

#### Non-equality conditions

When the condition compares two views with something other than `=`, or contains `OR`, and it names exactly two
views, use a range or a many-to-many relationship:

| LookML `relationship` | AML block | `from` → `to` |
|---|---|---|
| `many_to_one`, `one_to_one` | `RangeRelationship` (`type: 'range'`) | `O` → `J` |
| `one_to_many` | `RangeRelationship` (`type: 'range'`) | `J` → `O` |
| `many_to_many` | `ManyToManyRelationship` (`type: 'many_to_many'`) | `O` → `J` |

- `from` and `to` are the **primary keys** of the two models, not the compared columns. Without a primary key on
  both sides, the join needs manual review.
- `match` holds the **whole** condition, equalities included, translated to AQL: `=` → `==`, `<>` → `!=`,
  `AND` / `OR` → `and` / `or`, `x BETWEEN a AND b` → `x >= a and x <= b`, `${v.f}` → `v.f`.
- A `many_to_many` join always uses `ManyToManyRelationship`, even when its condition is a plain equality.
- SQL that AQL cannot express (casts, `INTERVAL`, `CURRENT_DATE`) moves into hidden helper dimensions, so that
  `match` only compares dimensions.

```lookml
join: sessions {
  sql_on: ${events.user_id} = ${sessions.user_id}
      AND ${events.occurred_at} >= ${sessions.started_at}
      AND ${events.occurred_at} <= ${sessions.ended_at} ;;
  relationship: many_to_one
}
join: campaigns {
  sql_on: ${events.occurred_at} >= ${campaigns.starts_at}
      AND ${events.occurred_at} < ${campaigns.ends_at} ;;
  relationship: many_to_many
}
```
```aml
RelationshipConfig {
  rel: RangeRelationship {
    type: 'range'
    from: FieldRef { model: 'events', field: 'id' }
    to: FieldRef { model: 'sessions', field: 'id' }
    match: @aql
      events.user_id == sessions.user_id
        and events.occurred_at >= sessions.started_at
        and events.occurred_at <= sessions.ended_at
    ;;
  }
  active: true
},
RelationshipConfig {
  rel: ManyToManyRelationship {
    type: 'many_to_many'
    from: FieldRef { model: 'events', field: 'id' }
    to: FieldRef { model: 'campaigns', field: 'id' }
    match: @aql
      events.occurred_at >= campaigns.starts_at
        and events.occurred_at < campaigns.ends_at
    ;;
  }
  active: true
}
```

Mark these relationships for review. A range relationship assumes at most one match per source row; overlapping
ranges give different results than in Looker. If range relationships are not available in the target workspace, the
fallback is a query model that does the join in SQL.

#### Unsupported shapes

Do not emit a relationship. Add the model to `models`, add `// TODO` with the original `sql_on`, and report it.

| Shape | Closest workaround |
|---|---|
| Liquid in the condition | [liquid.md](liquid.md). An equality with a user attribute is a row filter: a `permission` block ([`access_filter`](#access_filter)) |
| The condition names three or more views | If it splits into key pairs per pair of views, one relationship per pair; this can create two paths between models, so check for [path ambiguity](https://docs.holistics.io/docs/joins/path-ambiguity). Otherwise a query model that joins the views in SQL |
| A side mixes two views (`COALESCE(${a.x}, ${b.x}) = ${c.k}`) | A query model |
| `TRUE`, `1 = 1`, or no condition | A cross join: see [`type`](#type) |
| `FALSE` | Keep the model in `models` with no relationship |
| Only a constant condition on one view | A query model filtered by the constant, then as a cross join |

### `sql_table_name` (join)

Looker: [`sql_table_name` (for joins)](https://docs.cloud.google.com/looker/docs/reference/param-explore-join-sql-table-name)

It points the view at another table of the same shape, for this explore only. Emit an alias model (as for
[`from`](#from), even when the join has no `from`) with a `table_name` override, and use it in this dataset. If the
join name equals the view name, name the alias `<explore>__<join>`.

```lookml
join: archived_flights {
  from: flights
  sql_table_name: archive.flights_2019 ;;
  sql_on: ${flights.id} = ${archived_flights.id} ;;
  relationship: one_to_one
}
```
```aml
Model archived_flights = flights.extend({
  label: 'Archived Flights'
  table_name: 'archive.flights_2019'
  measure count { definition: @aql count(archived_flights.id);; }
})
```

This works for table models only. If the view is a derived table, the override is unsupported.

### `type`

Looker: [`type` (for joins)](https://docs.cloud.google.com/looker/docs/reference/param-explore-join-type) ·
Holistics: [How joins work](https://docs.holistics.io/docs/joins/how-joins-work)

| LookML `type` | AML |
|---|---|
| `left_outer`, or omitted | nothing: the default relationship |
| `inner` | `nullable: false` (short form: `nullable=false`), with a comment that it came from `type: inner` |
| `full_outer` | unsupported: emit the default relationship and `// TODO full_outer` |
| `cross` | unsupported: emit no relationship and `// TODO cross join` |

```lookml
join: accounts {
  type: inner
  sql_on: ${events.account_id} = ${accounts.id} ;;
  relationship: many_to_one
}
```
```aml
relationship(events.account_id > accounts.id, true, nullable=false)
```

Differences to report:

- **`left_outer`.** Looker always keeps the rows of `O`. Holistics keeps the rows of the model the query
  aggregates. For a `one_to_many` join, Looker keeps parents without children, while a Holistics query with only a
  child metric starts from the child model.
- **`inner`.** `nullable: false` states that every row has a match; it is not a join instruction. Looker's `inner`
  is often used to remove rows without a match. Mark these relationships for review.
- **`full_outer`.** Holistics gives a similar result only when the query has a metric from each model.
- **`cross`.** There is no cross-join relationship. A person has to redesign it, usually as a query model that
  contains the `CROSS JOIN` in SQL.

## Query parameters

### `required_access_grants` (join)

Looker: [`required_access_grants` (for joins)](https://docs.cloud.google.com/looker/docs/reference/param-explore-join-required-access-grants)

Unsupported. This is a **security-relevant loss**: report it. Manual workarounds:
[column-level permission](https://docs.holistics.io/docs/access-control/column-level-permission) on the dimensions
of that model (it masks values, it does not hide the fields), or a second dataset without the model, shared with
other users.

### `sql_where`

Looker: [`sql_where`](https://docs.cloud.google.com/looker/docs/reference/param-explore-join-sql-where)

- The condition refers only to the joined view → the relationship's `where` filter on that side, as in
  [equality with a constant condition](#equality-with-a-constant-condition).
- The condition refers to another view → unsupported; add `// TODO sql_where: <original>`.

Difference: `sql_where` removes base rows that have no matching joined row. A relationship filter only limits which
rows can match. Mark it for review.

## Join parameters to avoid

### `sql`

Looker: [`sql` (for joins)](https://docs.cloud.google.com/looker/docs/reference/param-explore-join-sql) ·
Holistics: [Query Model](https://docs.holistics.io/reference/aml/query-model)

`sql` holds a complete raw join clause.

- **`LEFT JOIN UNNEST(${parent.array}) AS child`** (nested records). Relationships cannot unnest. Convert the child
  view to a query model that selects the parent's primary key and the unnested columns, then relate it to the
  parent. The parent needs a primary key.
- **Empty `sql: ;;`.** Keep the model in `models` with no relationship, or move its fields into the base model.
- **Other raw joins.** Read the `ON` clause and apply the [`sql_on`](#sql_on) rules. `RIGHT JOIN` is unsupported.

```lookml
explore: sessions {
  join: hits {
    sql: LEFT JOIN UNNEST(${sessions.hits}) AS hits ;;
    relationship: one_to_many
  }
}
```
```aml
Model hits {
  type: 'query'
  label: 'Hits'
  data_source_name: 'demodb'
  models: [sessions]
  query: @sql
    select
      {{ #s.id }} as session_id,
      hit.*
    from {{ #sessions as s }}
    left join unnest(s.hits) as hit
  ;;

  dimension session_id {
    label: 'Session Id'
    type: 'number'
    definition: @sql {{ #SOURCE.session_id }};;
  }
  dimension page_path {
    label: 'Page Path'
    type: 'text'
    definition: @sql {{ #SOURCE.page_path }};;
  }
}
```
```aml
relationship(hits.session_id > sessions.id, true)
```

The SQL of a query model is not checked by the validator. Mark the model for review.

### `sql_foreign_key`

Looker: [`sql_foreign_key`](https://docs.cloud.google.com/looker/docs/reference/param-explore-join-sql-foreign-key)

Treat the SQL as an expression key on `O` ([equality on a SQL expression](#equality-on-a-sql-expression)) and the
primary key of the joined view as the other side.
