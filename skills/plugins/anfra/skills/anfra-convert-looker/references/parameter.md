# LookML `parameter` → AML `param`

How to convert a LookML `parameter` field and the Liquid that reads it. The sections follow Looker's
[`parameter`](https://docs.cloud.google.com/looker/docs/reference/param-field-parameter) page.

A parameter becomes a [param field](https://docs.holistics.io/docs/modeling/param-fields) of the model. The Liquid
that reads it becomes an AQL expression that reads the param:

```lookml
view: orders {
  parameter: metric_picker {
    type: unquoted
    default_value: "revenue"
    allowed_value: { label: "Total Revenue" value: "revenue" }
    allowed_value: { label: "Order Count"   value: "orders" }
  }

  measure: selected_metric {
    type: number
    sql:
      {% if metric_picker._parameter_value == 'revenue' %} ${total_revenue}
      {% else %} ${order_count}
      {% endif %} ;;
  }
}
```
```aml
Model orders {
  type: 'table'
  label: 'Orders'
  data_source_name: 'demodb'
  table_name: 'orders'

  dimension id {
    label: 'Id'
    type: 'number'
    primary_key: true
    definition: @sql {{ #SOURCE.id }};;
  }
  dimension amount {
    label: 'Amount'
    type: 'number'
    definition: @sql {{ #SOURCE.amount }};;
  }
  measure total_revenue {
    label: 'Total Revenue'
    type: 'number'
    definition: @aql sum(orders.amount);;
  }
  measure order_count {
    label: 'Order Count'
    type: 'number'
    definition: @aql count(orders.id);;
  }

  param metric_picker {
    label: 'Metric Picker'
    type: 'text'
    description: 'Values: revenue (Total Revenue), orders (Order Count). Looker default: revenue'
    allowed_values: ['revenue', 'orders']
  }
  measure selected_metric {
    label: 'Selected Metric'
    type: 'number'
    definition: @aql case(
      when: 'revenue' in orders.metric_picker, then: orders.total_revenue,
      else: orders.order_count
    );;
  }
}
```

Three things are different from Looker in every conversion:

- **A param has no default.** `default_value` moves to the dashboard filter (see [`default_value`](#default_value)).
- **A param read in AQL must have a value.** Looker falls back to the default or to the `{% else %}` branch. In
  Holistics a report that reads an unset param in AQL fails.
- **A param is a list of values.** Read it with `'x' in model.p` or `first(model.p)`, never with `==`.

## Quick reference

| LookML | AML | Section |
|---|---|---|
| `parameter` | `param` | [parameter](#parameter) |
| `type` | `type` | [type](#type) |
| `allowed_value` | `allowed_values` (labels are lost) | [allowed_value](#allowed_value) |
| `default_value` | default of the dashboard filter | [default_value](#default_value) |
| `{% if p._parameter_value == 'x' %}`, `CASE WHEN {% parameter p %} = 'x'` | AQL `case(when: 'x' in m.p, …)` | [Choosing a field or expression](#choosing-a-field-or-expression) |
| `{% parameter p %}` as a value | `first(m.p)` | [Inserting a value](#inserting-a-value) |
| `{% parameter p %}` as SQL text (column, function, keyword) | one `case()` branch per allowed value | [Inserting SQL text](#inserting-sql-text) |
| `{% parameter p %}` in a derived table | `{{ p }}` in the query model | [In a derived table](#in-a-derived-table) |
| `{% parameter p %}` in a table name, free-input SQL text | unsupported | [Unsupported uses](#unsupported-uses) |
| `label_from_parameter` | unsupported | [field.md](field.md#label_from_parameter) |
| other subparameters (`label`, `description`, `hidden`, `suggest_*`, …) | see field.md | [field.md](field.md) |

---

## `parameter`

Looker: [`parameter`](https://docs.cloud.google.com/looker/docs/reference/param-field-parameter) ·
Holistics: [Parameter Fields](https://docs.holistics.io/docs/modeling/param-fields)

`parameter: p { … }` → `param p { … }` in the model of the view.

1. Convert only parameters that are read by a field or query you convert. Drop a parameter that nothing reads, and
   list it in the conversion report.
2. Keep the name. The Liquid `p._parameter_value` and `{% parameter p %}` become references to `<model>.p`.
3. `label`, `description`, `hidden` → the same properties (see [field.md](field.md)). Without a label, use the name
   in title case. `view_label`, `group_label`, `group_item_label` → drop.
4. A parameter that is read from another view, or by a field that moved to the Dataset, goes into a small param
   model that is added to the Dataset. Reference it as `<param model>.p`.

```lookml
parameter: currency {
  type: string
  description: "Currency of the amounts"
}
```
```aml
param currency {
  label: 'Currency'
  type: 'text'
  description: 'Currency of the amounts'
}
```

A param model for parameters shared by several models (see
[Dynamic Metrics Selection](https://docs.holistics.io/docs/modeling/dynamic-measures)):

```aml
Model report_params {
  type: 'query'
  label: 'Report Params'
  data_source_name: 'demodb'
  models: []
  query: @sql select 1;;

  param metric_picker {
    label: 'Metric Picker'
    type: 'text'
    allowed_values: ['revenue', 'orders']
  }
}
```
```aml
Dataset sales_dataset {
  label: 'Sales'
  data_source_name: 'demodb'
  models: [orders, report_params]
  relationships: []

  metric selected_metric {
    label: 'Selected Metric'
    type: 'number'
    definition: @aql case(
      when: 'revenue' in report_params.metric_picker, then: orders.total_revenue,
      else: orders.order_count
    );;
  }
}
```

Mark for review: a param model driving Dataset metrics is documented. Driving Dataset *dimensions* this way
compiles, but the Holistics docs describe cross-model dimension selection as not yet available.

Differences:

- Looker lets the user pick exactly one value. A Holistics filter on a param can pass several; set the dashboard
  filter to single-select.
- The param is listed with the model's other fields, not under "Filter-Only Fields".

## `type`

Looker: [Dimension, filter, and parameter types](https://docs.cloud.google.com/looker/docs/reference/param-dimension-filter-parameter-types),
[`parameter` types](https://docs.cloud.google.com/looker/docs/reference/param-field-parameter#parameter_types) ·
Holistics: [Parameter Fields](https://docs.holistics.io/docs/modeling/param-fields)

| LookML `type` | AML `type` | Note |
|---|---|---|
| `string`, or no `type` | `'text'` | The Liquid compares with `"'Day'"`; write `'Day'` in AQL. |
| `unquoted` | `'text'` | "Unquoted" only says how Looker pastes the value into SQL. See [Inserting SQL text](#inserting-sql-text). |
| `number` | `'number'` | |
| `yesno` | `'truefalse'` | Read it as `first(m.p) == true`. |
| `date` | `'date'` | |
| `date_time` | `'date'`; `'datetime'` when the SQL that reads it needs a timestamp | Looker lets the user pick one day. |

Other field types are not valid AML param types. Use the closest of the five above and report it.

```lookml
parameter: min_amount { type: number }
parameter: as_of_date { type: date_time }
parameter: include_tax { type: yesno }
```
```aml
param min_amount {
  label: 'Min Amount'
  type: 'number'
}
param as_of_date {
  label: 'As Of Date'
  type: 'date'
}
param include_tax {
  label: 'Include Tax'
  type: 'truefalse'
}
```

## `allowed_value`

Looker: [`allowed_value`](https://docs.cloud.google.com/looker/docs/reference/param-field-parameter#specifying_allowed_values) ·
Holistics: [Parameter Fields](https://docs.holistics.io/docs/modeling/param-fields)

Collect the `value` of every `allowed_value`, in order, into `allowed_values`.

1. Use the `value`, not the `label`. The value is what the Liquid compares with, so it is what the converted
   `case()` compares with.
2. A number parameter gets numbers: `allowed_values: [7, 30, 90]`.
3. The labels are lost: Holistics shows the values. When a label differs from its value, write the pairs into the
   param's `description`.

```lookml
parameter: date_granularity {
  type: unquoted
  allowed_value: { label: "By Day"   value: "day" }
  allowed_value: { label: "By Month" value: "month" }
}
```
```aml
param date_granularity {
  label: 'Date Granularity'
  type: 'text'
  description: 'Values: day (By Day), month (By Month)'
  allowed_values: ['day', 'month']
}
```

Alternative, when the labels matter more than the values: use the labels as the allowed values, and compare with the
labels in every `case()` that reads the param.

## `default_value`

Looker: [`default_value`](https://docs.cloud.google.com/looker/docs/reference/param-field-default-value) ·
Holistics: [AML FilterBlock](https://docs.holistics.io/reference/aml/filter-block)

Unsupported on the param: AML has no default for a param field.

1. Emit nothing on the param. Append `Looker default: <value>` to its `description`.
2. Report the value as the **required** default of the dashboard filter that drives the param. A dashboard that
   shows a field reading the param needs a filter on it, with a default.
3. If the explore sets the parameter in `always_filter`, that value wins over `default_value`.

```lookml
parameter: date_granularity {
  type: unquoted
  default_value: "day"
  allowed_value: { value: "day" }
  allowed_value: { value: "month" }
}
```
```aml
param date_granularity {
  label: 'Date Granularity'
  type: 'text'
  description: 'Looker default: day'
  allowed_values: ['day', 'month']
}
```

The filter that carries the default, on a dashboard (dashboards themselves are out of scope):

```aml
block f_granularity: FilterBlock {
  label: 'Date Granularity'
  type: 'field'
  source: FieldFilterSource {
    dataset: sales_dataset
    field: r(orders.date_granularity)
  }
  default {
    operator: 'is'
    value: 'day'
  }
}
```

---

## Reading the parameter

Looker: [Using `parameter` with Liquid](https://docs.cloud.google.com/looker/docs/reference/param-field-parameter#using_parameter_with_liquid),
[Templated filters and Liquid parameters](https://docs.cloud.google.com/looker/docs/templated-filters)

Liquid rewrites the SQL text before it runs. Holistics has no text templating in field definitions, so convert the
*intent* of each use. Find the matching pattern below. Other Liquid (user attributes, `_in_query`, Liquid in `html`
or `link`) is in [liquid.md](liquid.md).

### Choosing a field or expression

Holistics: [Dynamic Dimensions Selection](https://docs.holistics.io/docs/modeling/dynamic-dimensions),
[Dynamic Metrics Selection](https://docs.holistics.io/docs/modeling/dynamic-measures)

Both of these shapes, in the `sql` of a dimension, dimension group or measure:

```lookml
{% if p._parameter_value == 'v1' %} E1 {% elsif p._parameter_value == 'v2' %} E2 {% else %} E3 {% endif %}

CASE WHEN {% parameter p %} = 'v1' THEN E1 WHEN {% parameter p %} = 'v2' THEN E2 ELSE E3 END
```

become an AQL `case()`:

```
case(when: 'v1' in m.p, then: <E1>, when: 'v2' in m.p, then: <E2>, else: <E3>)
```

1. **Compared value.** `unquoted` and `number` parameters: the bare value (`when: 7 in m.p`). `string` parameters
   are compared with `"'Day'"` in Liquid: strip the inner quotes. `!=` → `not ('v' in m.p)`.
2. **Branches.** Convert each branch like any field SQL (see [sql-references.md](sql-references.md)): `${dim}` →
   `m.dim`; a timeframe such as `${created_week}` → `m.created_at | week()` (see
   [dimension-group.md](dimension-group.md)). SQL that AQL cannot express → put it in a hidden helper dimension and
   reference that.
3. **One type.** Every branch of `case()` is compiled and must return the same type. Liquid emits only the branch
   taken, so Looker branches may differ. Cast all branches to text, or split the field.
4. **Measures.** Branches that are measures stay measure references or AQL aggregates.
5. Keep `{% else %}` as `else:`. It is not a default: the param still needs a value.
6. `{% assign v = p._parameter_value %}` followed by `{% if v == … %}`, and `{% case p._parameter_value %}`, are the
   same pattern.

```lookml
parameter: date_granularity {
  type: unquoted
  allowed_value: { value: "day" }
  allowed_value: { value: "month" }
}

dimension: created_dynamic {
  sql:
    {% if date_granularity._parameter_value == 'day' %} ${created_date}
    {% else %} ${created_month}
    {% endif %} ;;
}
```
```aml
param date_granularity {
  label: 'Date Granularity'
  type: 'text'
  allowed_values: ['day', 'month']
}
dimension created_dynamic {
  label: 'Created Dynamic'
  type: 'date'
  definition: @aql case(
    when: 'day' in orders.date_granularity, then: orders.created_at | day(),
    else: orders.created_at | month()
  );;
}
```

Differences:

- All branches are part of the generated SQL, as a `CASE` over a constant.
- When several values are passed, the first matching `when:` wins.
- A selector that only switches day / week / month of one date can often be replaced by a dashboard date-drill
  control. That is a dashboard decision; the `case()` is the faithful conversion.

### Inserting a value

Holistics: [Parameter Fields](https://docs.holistics.io/docs/modeling/param-fields)

`{% parameter p %}` where SQL expects a value (a number, a string, a date) → `first(m.p)` in AQL. Convert the whole
expression to AQL.

| LookML `sql` | AQL |
|---|---|
| `${amount} > {% parameter min_amount %}` | `orders.amount > first(orders.min_amount)` |
| `${status} = {% parameter status_picker %}` | `orders.status in orders.status_picker` |
| `SUM(CASE WHEN ${status} = {% parameter status_picker %} THEN 1 ELSE 0 END)` | `count(orders.id) \| where(orders.status in orders.status_picker)` |
| `{% parameter include_tax %}` (a `yesno` parameter) | `first(orders.include_tax) == true` |

An `unquoted` parameter used as a value has its quotes in the LookML (`'{% parameter p %}'`). Drop them.

```lookml
parameter: min_amount { type: number }

dimension: is_large_order {
  type: yesno
  sql: ${amount} > {% parameter min_amount %} ;;
}
```
```aml
param min_amount {
  label: 'Min Amount'
  type: 'number'
}
dimension is_large_order {
  label: 'Is Large Order'
  type: 'truefalse'
  definition: @aql orders.amount > first(orders.min_amount);;
}
```

A field's `@sql` definition cannot read a param. If part of the expression cannot be written in AQL, keep that part
(without the parameter) in a hidden helper dimension and combine it with the param in AQL.

### Inserting SQL text

Looker: [`type: unquoted`](https://docs.cloud.google.com/looker/docs/reference/param-field-parameter#parameters_of_type_unquoted)

An `unquoted` parameter pasted into the SQL as a column name, a function name or a keyword. Holistics never pastes
user input into SQL as text. If the parameter has `allowed_value`s, write the expression once per value and choose
with `case()`, as in [Choosing a field or expression](#choosing-a-field-or-expression).

| Pasted as | LookML | Conversion |
|---|---|---|
| column name | `${TABLE}.{% parameter breakdown_by %}` | one branch per value: the dimension of that column |
| function name | `{% parameter agg %}(${amount})` | one aggregate per value: `sum(…)`, `average(…)`, `max(…)` |
| date part | `DATE_TRUNC({% parameter period %}, ${created_raw})` | one branch per value: `m.created_at \| month()`, … |
| inside a string | `'This {% parameter period %}'` | one literal per branch, or `concat('This ', first(m.p))` |
| a number | `INTERVAL {% parameter n %} DAY` | a value: see [Inserting a value](#inserting-a-value) |

```lookml
parameter: agg {
  type: unquoted
  allowed_value: { label: "Total"   value: "SUM" }
  allowed_value: { label: "Average" value: "AVG" }
  allowed_value: { label: "Largest" value: "MAX" }
}

measure: amount_metric {
  type: number
  sql: {% parameter agg %}(${amount}) ;;
}
```
```aml
param agg {
  label: 'Agg'
  type: 'text'
  description: 'Values: SUM (Total), AVG (Average), MAX (Largest)'
  allowed_values: ['SUM', 'AVG', 'MAX']
}
measure amount_metric {
  label: 'Amount Metric'
  type: 'number'
  definition: @aql case(
    when: 'SUM' in orders.agg, then: sum(orders.amount),
    when: 'AVG' in orders.agg, then: average(orders.amount),
    when: 'MAX' in orders.agg, then: max(orders.amount)
  );;
}
```

Difference: the converted field accepts exactly the listed values.

### In a derived table

Looker: [Templated filters and Liquid parameters](https://docs.cloud.google.com/looker/docs/templated-filters) ·
Holistics: [Dynamic Query Model](https://docs.holistics.io/docs/query-parameters)

A derived table becomes a query model (see [derived-tables.md](derived-tables.md)). Declare the `param` on that
query model and write `{{ p }}` where the SQL had `{% parameter p %}`.

1. `{{ p }}` renders a **value**, as a quoted literal for text and dates. Drop quotes that the LookML wrote around
   the tag. It cannot stand for a column, table or keyword.
2. Only the first value is used.
3. `{% if p._parameter_value == 'v' %} … {% endif %}` in derived-table SQL → `CASE {{ p }} WHEN 'v' THEN … END`,
   when every branch is valid SQL at the same time. A branch that changes the structure of the query (another
   `FROM`, another `GROUP BY`) is unsupported; report it.
4. Persistence settings are dropped like on any derived table
   (see [derived-tables.md](derived-tables.md#persistence-parameters)).

```lookml
view: customer_spend {
  derived_table: {
    sql:
      SELECT customer_id, SUM(amount) AS spend
      FROM public.orders
      WHERE created_at >= {% parameter since %}
      GROUP BY 1 ;;
  }

  parameter: since { type: date_time }

  dimension: customer_id { primary_key: yes  type: number }
  dimension: spend { type: number }
}
```
```aml
Model customer_spend {
  type: 'query'
  label: 'Customer Spend'
  data_source_name: 'demodb'
  models: []

  param since {
    label: 'Since'
    type: 'date'
  }

  query: @sql
    SELECT customer_id, SUM(amount) AS spend
    FROM public.orders
    WHERE created_at >= {{ since }}
    GROUP BY 1
  ;;

  dimension customer_id {
    label: 'Customer Id'
    type: 'number'
    primary_key: true
    definition: @sql {{ #SOURCE.customer_id }};;
  }
  dimension spend {
    label: 'Spend'
    type: 'number'
    definition: @sql {{ #SOURCE.spend }};;
  }
}
```

The validator does not read the SQL text, so it does not check `{{ since }}`. Without a value for the param the SQL
is invalid: the dashboard filter needs a default here too. Mark for review: how `{{ p }}` renders a number param is
not documented.

`{% parameter p %}` in `sql_always_where` or `sql_on`: see [explore.md](explore.md#sql_always_where) and
[explore.md](explore.md#sql_on). A relationship cannot depend on a param.

### Unsupported uses

| Use | What to do |
|---|---|
| SQL text from a parameter without `allowed_value`s (free input) | Unsupported. Emit the field with `default_value` substituted, so that the model compiles, and report it. |
| A parameter in a table name (`` `events_{% parameter suffix %}` ``, `sql_table_name`) | Unsupported. Emit the default table and report it. Manual options: one model per table, or a query model that `UNION ALL`s the tables with a column that a filter selects on. See [view.md](view.md#sql_table_name). |
| `label_from_parameter` | Unsupported. See [field.md](field.md#label_from_parameter). |
| A parameter read in `html`, `link`, `label` | Unsupported. See [liquid.md](liquid.md). |
| A parameter that builds a period-over-period comparison together with `{% date_start %}` | Replace the whole block. See [filter.md](filter.md#period-over-period-blocks). |
