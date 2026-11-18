# Liquid in LookML → AML / AQL

How to convert Liquid (`{% … %}`, `{{ … }}`) in LookML. The quick reference follows the variable list of Looker's
[Liquid variable reference](https://docs.cloud.google.com/looker/docs/liquid-variable-reference).

Liquid rewrites text before the query runs, and it can read the query, the user and the filters. AML has no text
templating. So Liquid is not translated tag by tag: a Liquid usage converts only when its **intent** matches one of
these few cases.

| Intent of the Liquid | AML / AQL | Where |
|---|---|---|
| A parameter chooses a field, an expression or a value | `param` + AQL `case()` / `first()` | [parameter.md](parameter.md) |
| A templated filter in a derived table | `param` + `{% filter(p) %}` in a query model | [filter.md](filter.md) |
| A templated filter in a dimension or measure | `param` + AQL `in` / `matches` (operators are lost) | [filter.md](filter.md) |
| A period-over-period block | `relative_period()` measures | [filter.md](filter.md#period-over-period-blocks) |
| A user attribute chooses a schema, table or connection | AML `const` with `H.current_user.x` | [below](#in-a-table-name-or-connection) |
| A user attribute restricts rows | Dataset `permission` | [below](#compared-with-a-column) |
| A user attribute chooses or masks a value | AML `if` on `H.current_user.x` | [below](#choosing-or-masking-a-value) |
| Development and production tables (`-- if dev --`) | AML `if` on `H.git.is_production` | [view.md](view.md#development-and-production-tables) |
| A condition that reads only manifest constants | evaluate it during conversion | [below](#liquid-tags) |

Everything else is **unsupported**: Liquid that reads the query (`_in_query`, `_is_selected`, `_is_filtered`,
`_filters`), Liquid that builds SQL text, and Liquid in `html`, `link`, `action` and `label`.

For each Liquid usage:

1. Find the intent in the table above and follow that section.
2. If no intent matches, do not invent AML. Emit the default branch (the `{% else %}` branch, or the branch for the
   parameter's `default_value`), or drop the parameter if it is a presentation parameter.
3. List the usage, with its original LookML, in the conversion report.

## Quick reference

| Liquid | Conversion | Section |
|---|---|---|
| `{{ value }}`, `{{ rendered_value }}`, `{{ filterable_value }}`, `{{ link }}`, `{{ linked_value }}` | unsupported | [Presentation parameters](#liquid-in-presentation-parameters) |
| `view.field._value`, `._rendered_value`, `._link`, … | unsupported | [Presentation parameters](#liquid-in-presentation-parameters) |
| `_filters['view.field']` | unsupported | [Reading the query](#_in_query-_is_selected-_is_filtered) |
| `{% date_start f %}`, `{% date_end f %}` | `{% filter(f) %}`, or two date params | [filter.md](filter.md#date_start-date_end) |
| `{% condition f %} … {% endcondition %}` | `{% filter(f) %}` in a query model; AQL `in` in a field | [filter.md](filter.md#reading-the-filter) |
| `{% parameter p %}`, `p._parameter_value` | `param` read in AQL | [parameter.md](parameter.md#reading-the-parameter) |
| `_user_attributes['x']` | `H.current_user.x`, or a `permission` | [_user_attributes](#_user_attributes) |
| `_localization['key']` | unsupported | [manifest.md](manifest.md#localization_settings) |
| `_model._name`, `_view._name`, `_explore._name`, `_field._name` | the literal name | [Names](#_model_name-_view_name-_explore_name-_field_name) |
| `_explore._dashboard_url`, `_query._query_timezone` | unsupported | [Unsupported uses](#unsupported-uses) |
| `view._in_query`, `view.field._in_query`, `._is_selected`, `._is_filtered` | unsupported, with two exceptions | [Reading the query](#_in_query-_is_selected-_is_filtered) |
| `{% incrementcondition %}` | removed; persistence is unsupported | [derived-tables.md](derived-tables.md#persistence-parameters) |
| `{% if %}`, `{% assign %}`, `{% case %}`, `{% unless %}`, `{% for %}` | by what they read | [Liquid tags](#liquid-tags) |
| Liquid filters: `\| url_encode`, `\| round`, `\| date: …` | unsupported | [Presentation parameters](#liquid-in-presentation-parameters) |
| `-- if dev --`, `-- if prod --` (not Liquid, but often next to it) | AML `if` on `H.git.is_production` | [view.md](view.md#development-and-production-tables) |

---

## `_user_attributes`

Looker: [User attributes](https://docs.cloud.google.com/looker/docs/admin-panel-users-user-attributes),
[Liquid variable reference](https://docs.cloud.google.com/looker/docs/liquid-variable-reference) ·
Holistics: [User Attributes](https://docs.holistics.io/docs/admin/user-attributes),
[AML user attributes and variables](https://docs.holistics.io/reference/aml/user-attributes-and-variables)

`_user_attributes['x']` → `H.current_user.x`. How it is used depends on what the LookML does with the value.

Rules for every use:

1. The user attribute must exist in Holistics under the same name, with the users' values moved over. That is an
   admin task outside AML. List every attribute in the conversion report.
2. **Never write `H.…` inside `${…}`.** `'${H.current_user.x}.orders'` is wrong, and the validator does not catch
   it. Put the bare `H.current_user.x` into a `const` first, then interpolate the constant: `'${my_const}.orders'`.

### In a table name or connection

Holistics: [Dynamic Schemas](https://docs.holistics.io/docs/development/dynamic-schema),
[Dynamic Data Sources](https://docs.holistics.io/docs/development/dynamic-data-source)

| LookML | AML |
|---|---|
| in `sql_table_name` | a `const`, interpolated into `table_name` — see [view.md](view.md#with-user-attributes) |
| in the `FROM` of a derived table | the same `const`, interpolated into the `query` of the query model |
| chooses the connection | `data_source_name: H.current_user.x` |

```lookml
view: order_totals {
  derived_table: {
    sql:
      SELECT user_id, SUM(amount) AS total
      FROM {{ _user_attributes['reporting_schema'] }}.orders
      GROUP BY 1 ;;
  }

  dimension: user_id { primary_key: yes  type: number }
  dimension: total { type: number }
}
```
```aml
const reporting_schema = H.current_user.reporting_schema

Model order_totals {
  type: 'query'
  label: 'Order Totals'
  data_source_name: 'demodb'
  models: []

  query: @sql
    SELECT user_id, SUM(amount) AS total
    FROM ${reporting_schema}.orders
    GROUP BY 1
  ;;

  dimension user_id {
    label: 'User Id'
    type: 'number'
    primary_key: true
    definition: @sql {{ #SOURCE.user_id }};;
  }
  dimension total {
    label: 'Total'
    type: 'number'
    definition: @sql {{ #SOURCE.total }};;
  }
}
```

A user attribute that selects the connection:

```aml
Model orders {
  type: 'table'
  label: 'Orders'
  data_source_name: H.current_user.warehouse
  table_name: 'public.orders'
}
```

Differences:

- A Looker attribute can have a default value. A Holistics attribute without a value gives an invalid table name.
  The attribute must hold exactly one value.

### Compared with a column

Holistics: [Row-level permission as code](https://docs.holistics.io/docs/access-control/row-level-permission-as-code)

`${field} = '{{ _user_attributes['x'] }}'` restricts the rows a user sees. It becomes a `permission` block in the
Dataset, the same as [`access_filter`](explore.md#access_filter).

| Where the comparison is | Conversion |
|---|---|
| `sql_always_where` | one `permission` per comparison |
| a join's `sql_on` | a `permission` on that model's field; the relationship keeps only the key part |
| the `WHERE` of a derived table | a `const`, interpolated into the query (below) |
| no column at all (`'{{ _user_attributes['dept'] }}' = 'finance'`) | Unsupported. It is an access gate, not a row filter. Report it; access to the dataset is set in Holistics sharing. |

```lookml
explore: orders {
  sql_always_where: ${users.region} = '{{ _user_attributes['sales_region'] }}' ;;
  join: users {
    sql_on: ${orders.user_id} = ${users.id} ;;
    relationship: many_to_one
  }
}
```
```aml
Dataset orders {
  label: 'Orders'
  data_source_name: 'demodb'
  models: [orders, users]
  relationships: [relationship(orders.user_id > users.id, true)]

  permission users_region__sales_region {
    field: r(users.region)
    operator: 'matches_user_attribute'
    value: 'sales_region'
  }
}
```

In the `WHERE` of a derived table:

```lookml
derived_table: {
  sql: SELECT * FROM public.product_names WHERE locale = '{{ _user_attributes['locale'] }}' ;;
}
```
```aml
const user_locale = H.current_user.locale

Model product_names {
  type: 'query'
  label: 'Product Names'
  data_source_name: 'demodb'
  models: []
  query: @sql SELECT * FROM public.product_names WHERE locale = '${user_locale}' ;;
}
```

Mark for review. Differences:

- Looker applies the condition to everyone. Holistics does not enforce row-level permissions for admins, or for
  analysts who have access to the data source. It is also not available on every plan.
- A Holistics attribute can hold several values, or `All`.
- The `const` form in a query model compiles, but its use for a `WHERE` value is not documented. Review it.
- If the condition must also apply to admins, add a `truefalse` dimension that dashboards filter on:
  `definition: @aql orders.locale in H.current_user.locale;;`.

### Choosing or masking a value

Holistics: [Column-level permission](https://docs.holistics.io/docs/access-control/column-level-permission),
[Use user attributes in AQL](https://docs.holistics.io/docs/admin/user-attributes/use-in-aql)

**A `CASE` on the attribute decides which SQL the field runs** (usually to mask a column). Use an AML `if` that
chooses the text of the `@sql` definition. Both branches stay SQL: rewrite `${col}` as `{{ #SOURCE.col }}` and
escape `'` as `\'`.

```lookml
dimension: email {
  type: string
  sql:
    CASE WHEN '{{ _user_attributes["can_see_pii"] }}' = 'yes' THEN ${TABLE}.email
         ELSE MD5(${TABLE}.email) END ;;
}
```
```aml
const email_sql =
  if (H.current_user.can_see_pii == 'yes') { '{{ #SOURCE.email }}' }
  else { 'MD5({{ #SOURCE.email }})' }

Model users {
  type: 'table'
  label: 'Users'
  data_source_name: 'demodb'
  table_name: 'public.users'

  dimension email {
    label: 'Email'
    type: 'text'
    definition: @sql ${email_sql};;
  }
}
```

The `if` runs before the query is built, so a user without access never gets SQL that reads the column. If another
dimension exposes the same raw column, it needs the same treatment; report it.

**The attribute is a value in a calculation.** Write the expression in AQL and read `H.current_user.x` directly.

```lookml
dimension: amount_local {
  type: number
  sql: ${amount} * {{ _user_attributes['fx_rate'] }} ;;
}
```
```aml
dimension amount_local {
  label: 'Amount Local'
  type: 'number'
  definition: @aql orders.amount * H.current_user.fx_rate;;
}
```

To branch on an attribute in AQL, use `in`, because an attribute is a list of values:

```aml
dimension amount_by_currency {
  label: 'Amount By Currency'
  type: 'number'
  definition: @aql case(
    when: in('SGD', H.current_user.currency), then: orders.amount * 1.36,
    else: orders.amount
  );;
}
```

Do not use the AQL form for masking: it needs the unmasked column as a model field, and `hidden` is not a security
feature.

Mark for review: how `==` behaves when the attribute holds several values, or none, is not documented. These
conversions assume one value per user.

### In `html`, `label`, `description`, `link`

| LookML | Conversion |
|---|---|
| `html` that only hides the value from some users | Drop the `html`. Mask the field itself, as in [Choosing or masking a value](#choosing-or-masking-a-value). |
| `label` or `description` per user or per locale | Unsupported. An AML `if` is not accepted as a label. Keep the default branch and report it. |
| `link`, `action` | Unsupported. See [Liquid in presentation parameters](#liquid-in-presentation-parameters). |

---

## `_in_query`, `_is_selected`, `_is_filtered`

Looker: [Usage of `_in_query`, `_is_selected`, and `_is_filtered`](https://docs.cloud.google.com/looker/docs/liquid-variable-reference) ·
Holistics: [`is_at_level`](https://docs.holistics.io/reference/aql/miscellaneous-functions#is_at_level)

Unsupported. These variables change the SQL by which fields the query selects or filters. A Holistics model cannot
see the query. `_filters['view.field']` is unsupported for the same reason.

| Use | What to do |
|---|---|
| `{% if f._is_filtered %} … {% else %} NULL {% endif %}` as a guard around `{% condition %}` or `{% date_start %}` | Convert the guarded branch by [filter.md](filter.md) and drop the guard. |
| A join condition or join list that depends on `view._in_query` | Drop the Liquid. Holistics joins only the models that a report needs. A join *path* that depends on the query is unsupported; report it. |
| A measure that changes by which dimension is in the query | AQL `case(when: is_at_level(dim), …)` — the one use with a counterpart (below). |
| A table chosen by the query (`sql_table_name` with `{% if view.field._in_query %}`) | Unsupported. Emit the `{% else %}` table and report it — see [view.md](view.md#other-liquid). If the other table is a roll-up of the same data, model the detailed table: results are the same, queries can be slower ([aggregate-awareness.md](aggregate-awareness.md)). |
| A dimension that changes column by the query | Unsupported. Emit the `{% else %}` branch and report it. |
| The boolean itself as a field value | Unsupported. Drop the field and report it. |
| In `label`, `html`, `link` | Unsupported. See [Liquid in presentation parameters](#liquid-in-presentation-parameters). |

A measure that depends on a dimension being in the report:

```lookml
measure: storage_bytes {
  type: number
  sql:
    {% if storage.database_name._is_selected %} AVG(${database_bytes})
    {% else %} AVG(${account_bytes})
    {% endif %} ;;
}
```
```aml
measure storage_bytes {
  label: 'Storage Bytes'
  type: 'number'
  definition: @aql case(
    when: is_at_level(storage.database_name), then: average(storage.database_bytes),
    else: average(storage.account_bytes)
  );;
}
```

Mark for review. `is_at_level(dim)` is true when the dimension is a grouping of the report. That is closest to
`_is_selected`. `_in_query` is also true when the field is only filtered, and `view._in_query` has no counterpart.

---

## Liquid tags

Looker: [Liquid variable reference](https://docs.cloud.google.com/looker/docs/liquid-variable-reference) ·
Holistics: [AML if-else](https://docs.holistics.io/reference/aml/if-else),
[AML constant](https://docs.holistics.io/reference/aml/constant)

`{% if %}`, `{% assign %}`, `{% case %}`, `{% unless %}` and `{% for %}` have no general counterpart. An AML `if`
can read only the current user (`H.current_user.*`) and the Git context (`H.git.*`); it cannot read a param, a
filter or the query. Convert a tag by what its condition reads:

| The tag reads | Conversion |
|---|---|
| a parameter (`p._parameter_value`) | AQL `case()` — [parameter.md](parameter.md#choosing-a-field-or-expression) |
| a user attribute | AML `if` — [_user_attributes](#_user_attributes) |
| only manifest constants (`{% if "@{SCOPE}" == "EU" %}`) | Evaluate it during conversion and emit the branch that is taken (below). |
| `_in_query`, `_is_selected`, `_is_filtered`, `_filters` | Unsupported — [above](#_in_query-_is_selected-_is_filtered) |
| `value`, in `html` or `link` | Unsupported — [Presentation parameters](#liquid-in-presentation-parameters) |

By tag:

| Tag | Conversion |
|---|---|
| `{% assign v = p._parameter_value %}`, then `{% if v == … %}` | Replace `v` by what it stands for, then convert the `if`. |
| `{% case x %}{% when 'a' %} …` | Read it as an `if` / `elsif` chain. |
| `{% unless c %}` | Read it as `if not c`. |
| `{% for x in list %}` that generates SQL from a fixed list | Write the SQL out once per item. A loop over a list that is not fixed is unsupported. |
| `{% assign %}` that builds SQL text from parameters or filters | Unsupported. Report the field or derived table for a manual redesign. |

A condition on a manifest constant is static: it has one answer for the whole project.

```lookml
constant: SCOPE { value: "EU" }

dimension: revenue {
  type: number
  sql: {% if "@{SCOPE}" == "EU" %} ${TABLE}.revenue_eur {% else %} ${TABLE}.revenue_usd {% endif %} ;;
}
```
```aml
dimension revenue {
  label: 'Revenue'
  type: 'number'
  definition: @sql {{ #SOURCE.revenue_eur }};;
}
```

Report the constant and the branch that was taken, so that a later change of the constant is not missed.

## `_model._name`, `_view._name`, `_explore._name`, `_field._name`

Replace the variable with the literal name during conversion: `'{{ _view._name }}'` in the view `orders` →
`'orders'`. `field._sql` → the converted definition of that field, or a reference to it.

In a view that is extended or joined under several names, the name differs per use. Substitute it after the
extends are resolved (see [extends-and-refinements.md](extends-and-refinements.md)), and report a name that depends
on the explore.

---

## Liquid in presentation parameters

Looker: [`html`](https://docs.cloud.google.com/looker/docs/reference/param-field-html),
[`link`](https://docs.cloud.google.com/looker/docs/reference/param-field-link),
[`action`](https://docs.cloud.google.com/looker/docs/reference/param-field-action) ·
Holistics: [Data formats](https://docs.holistics.io/docs/data-format),
[Actions](https://docs.holistics.io/docs/actions)

Unsupported. An AML field has no template for its display, so Liquid in `html`, `link`, `action`, `label` and
`description` is not emitted into models or datasets. Drop it and report it. The parameters themselves are in
[field.md](field.md). Report what the Liquid was for, so that the dashboards can be rebuilt:

| Liquid | Meaning in Looker | What to report |
|---|---|---|
| `{{ value }}`, `{{ rendered_value }}` | the value of the cell | use the column itself in the action URL or HTML format |
| `{{ view.field._value }}` | another field of the same row; Looker adds that field to the query | add the other column to the visualization |
| `{{ link }}`, `{{ field._link }}` | the URL of the default drill | none |
| `_filters['view.field']` | the filter that the user applied | none in the model; a dashboard interaction |
| `{% if value > 100 %}` around coloured markup | conditional formatting | conditional formatting on the visualization |
| `p._parameter_value` in a `label` | a label that follows a parameter | static label — see [field.md](field.md#label_from_parameter) |
| `_user_attributes['x']` in `html` | content or masking per user | masking: [Choosing or masking a value](#choosing-or-masking-a-value); otherwise none |
| `\| url_encode`, `\| encode_uri` | escapes a value for a URL | part of the action URL |
| `\| round`, `\| plus`, `\| times`, `\| divided_by` | arithmetic on the shown value | a number `format`, or a real calculated field |
| `\| date: "%b %d"` | another date format | a date `format` on the field (see [field.md](field.md)) |

A link that is built only from values of the row can be kept as data: a hidden text dimension that concatenates
the URL.

```lookml
dimension: order_id {
  type: number
  sql: ${TABLE}.id ;;
  link: {
    label: "Open in admin"
    url: "https://admin.example.com/orders/{{ value }}"
  }
}
```
```aml
dimension order_id {
  label: 'Order Id'
  type: 'number'
  definition: @sql {{ #SOURCE.id }};;
}
dimension order_admin_url {
  label: 'Order Admin Url'
  type: 'text'
  hidden: true
  definition: @aql concat('https://admin.example.com/orders/', cast(orders.order_id, 'text'));;
}
```

Difference: in Looker, a `._value` reference adds the other field to the query and to its `GROUP BY`. That side
effect is lost with the Liquid.

---

## Unsupported uses

| Use | What to do |
|---|---|
| `_in_query`, `_is_selected`, `_is_filtered` choosing a table, a column or a join path | Emit the `{% else %}` branch and report it. See [above](#_in_query-_is_selected-_is_filtered). |
| `_filters['view.field']` | Drop and report it. |
| A parameter in a table name, or SQL text from free input | See [parameter.md](parameter.md#unsupported-uses). |
| `{% condition %}` in `sql_on` | See [filter.md](filter.md#conditional-joins). |
| `{% assign %}` / `{% for %}` that build SQL from parameters or filters | Report it for a manual redesign. |
| A user attribute as an access gate without a column | Report it; set access with dataset sharing. |
| `label` or `description` per user, per locale (`_localization`) or per parameter | Keep the default text and report it. |
| `_explore._dashboard_url`, `_query._query_timezone` | Drop and report it. |
| Liquid in `html`, `link`, `action` | Drop and report it. See [above](#liquid-in-presentation-parameters). |
