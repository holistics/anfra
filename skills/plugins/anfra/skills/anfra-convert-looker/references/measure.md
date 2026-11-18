# LookML `measure` → AML `measure`

How to convert a LookML measure and every measure type. The sections follow Looker's
[`measure`](https://docs.cloud.google.com/looker/docs/reference/param-field-measure) page, then the type list of the
[Measure types](https://docs.cloud.google.com/looker/docs/reference/param-measure-types) page.

A measure becomes a `measure` of the view's model, written in [AQL](https://docs.holistics.io/reference/aql/aggregator-functions):

```lookml
view: order_items {
  dimension: id         { primary_key: yes  type: number  sql: ${TABLE}.id ;; }
  dimension: sale_price { type: number  sql: ${TABLE}.sale_price ;; }
  dimension: status     { type: string  sql: ${TABLE}.status ;; }

  measure: count            { type: count }
  measure: total_sale_price { type: sum  sql: ${sale_price} ;; }
  measure: completed_count  { type: count  filters: [status: "complete"] }
  measure: average_item_value {
    type: number
    sql: ${total_sale_price} / NULLIF(${count}, 0) ;;
  }
}
```
```aml
Model order_items {
  type: 'table'
  label: 'Order Items'
  data_source_name: 'demodb'
  table_name: 'order_items'

  dimension id {
    label: 'Id'
    type: 'number'
    primary_key: true
    definition: @sql {{ #SOURCE.id }};;
  }
  dimension sale_price {
    label: 'Sale Price'
    type: 'number'
    definition: @sql {{ #SOURCE.sale_price }};;
  }
  dimension status {
    label: 'Status'
    type: 'text'
    definition: @sql {{ #SOURCE.status }};;
  }

  measure count {
    label: 'Count'
    type: 'number'
    definition: @aql count(order_items.id);;
  }
  measure total_sale_price {
    label: 'Total Sale Price'
    type: 'number'
    definition: @aql sum(order_items.sale_price);;
  }
  measure completed_count {
    label: 'Completed Count'
    type: 'number'
    definition: @aql count(order_items.id) | where(order_items.status == 'complete');;
  }
  measure average_item_value {
    label: 'Average Item Value'
    type: 'number'
    definition: @aql safe_divide(order_items.total_sale_price, order_items.count);;
  }
}
```

The fragments in the sections below are fields of this model.

## Quick reference

| LookML | AML / AQL | Section |
|---|---|---|
| `measure` | model `measure`, or Dataset `metric` | [measure](#measure) |
| `sql` | the argument of the AQL aggregate | [sql](#sql) |
| `filters` | `\| where(…)` | [filters](#filters) |
| `type: average` | `average()` | [average](#average) |
| `type: average_distinct`, `sum_distinct`, `median_distinct`, `percentile_distinct`; `sql_distinct_key` | plain aggregate, or `unique(key) \| agg(max(x))` | [distinct-key types](#average_distinct-median_distinct-percentile_distinct-sum_distinct) |
| `type: count` | `count(<primary key>)` | [count](#count) |
| `type: count_distinct`; `approximate` | `count_distinct()`, `approx_count_distinct()` | [count_distinct](#count_distinct) |
| `type: date` | AQL expression, `type: 'date'` | [date](#date) |
| `type: list`; `list_field` | `string_agg()` | [list](#list) |
| `type: max`, `min` | `max()`, `min()` | [max, min](#max-min) |
| `type: median` | `median()` | [median](#median) |
| `type: number` | AQL expression over measures | [number](#number) |
| `type: percent_of_previous` | `previous()` | [percent_of_previous](#percent_of_previous) |
| `type: percent_of_total`; `direction` | `percent_of_total()` | [percent_of_total](#percent_of_total) |
| `type: percentile`; `percentile` | `percentile_cont()` | [percentile](#percentile) |
| `type: period_over_period` | `relative_period()` | [period_over_period](#period_over_period) |
| `type: running_total` | `running_total()` | [running_total](#running_total) |
| `type: string`, no `type` | AQL expression, `type: 'text'` | [string](#string) |
| `type: sum` | `sum()` | [sum](#sum) |
| `type: yesno` | AQL expression, `type: 'truefalse'` | [yesno](#yesno) |
| `approximate_threshold`, `allow_approximate_optimization`, `precision` | drop | [Query parameters](#query-parameters) |
| `label`, `description`, `hidden`, `value_format`, `drill_fields`, … | see [field.md](field.md) | — |

---

## `measure`

Looker: [`measure`](https://docs.cloud.google.com/looker/docs/reference/param-field-measure) ·
Holistics: [AML Model Fields](https://docs.holistics.io/reference/aml/field),
[AML Metric](https://docs.holistics.io/reference/aml/metric)

Each `measure: <name>` becomes one `measure <name>` in the model of its view.

1. **Name.** Keep it unchanged.
2. **AML `type`.** `'number'` for every aggregate type except `list` (`'text'`). For non-aggregate types see
   [`date`](#date), [`number`](#number), [`string`](#string), [`yesno`](#yesno).
3. **`definition`.** Write it in AQL (`@aql … ;;`), by the measure's `type` (sections below).
4. **References must be qualified.** Inside AQL, write `order_items.sale_price`, not `sale_price`, also for fields
   of the same model.
5. **AQL sees dimensions, not columns.** Every column or SQL expression the measure uses must first exist as a
   dimension (see [`sql`](#sql)).
6. **Other parameters** (`label`, `description`, `hidden`, `group_label`, `value_format`, `value_format_name`,
   `drill_fields`, `link`, `html`, `tags`, `required_fields`, `required_access_grants`, …): see [field.md](field.md).

**Where the measure lives.** A measure that uses a field of another view — in `sql`, `filters` or
`sql_distinct_key` — cannot be a model field. It becomes a Dataset `metric` (see
[sql-references.md](sql-references.md)). Three rules apply there:

- an aggregate over another model's dimension names the measure's own model as the table to aggregate:
  `sum(order_items, products.cost)`;
- a measure that references a moved measure also moves, and refers to it by bare name (`total_cost`);
- a Dataset metric is AQL only; there is no `@sql` fallback.

```lookml
# view: order_items
measure: total_cost { type: sum  sql: ${products.cost} ;; }
```
```aml
Dataset order_items_dataset {
  label: 'Order Items'
  data_source_name: 'demodb'
  models: [order_items, products]
  relationships: [
    relationship(order_items.product_id > products.id, true)
  ]

  metric total_cost {
    label: 'Total Cost'
    type: 'number'
    definition: @aql sum(order_items, products.cost);;
  }
}
```

**Primary keys and symmetric aggregates.** Not needed. Looker needs a primary key in every joined view to keep
measures correct across a one-to-many join
([symmetric aggregates](https://docs.cloud.google.com/looker/docs/best-practices/understanding-symmetric-aggregates)).
Holistics aggregates an AQL measure on its own model before joining
([fan-out resolution](https://docs.holistics.io/docs/joins/fan-out-resolution)), so nothing has to be converted.
A primary key is still needed for [`count`](#count).

Difference: only measures written in raw `@sql` lose this protection. Prefer `@aql` for every measure.

## `sql`

Looker: [`sql`](https://docs.cloud.google.com/looker/docs/reference/param-field-sql) ·
Holistics: [AML Model Fields](https://docs.holistics.io/reference/aml/field)

For an aggregate type, `sql` is the value to aggregate. Find the AQL operand:

| `sql` | Operand |
|---|---|
| `${dim}`, a dimension of the same view | `<model>.<dim>` |
| `${TABLE}.col`, and a dimension of the view has exactly this `sql` | that dimension |
| any other SQL that uses only this view | a hidden helper dimension `<measure>__operand` holding the SQL |
| `${dimension_group_timeframe}` | the dimension that [dimension-group.md](dimension-group.md) gives for the timeframe |
| `${other_view.field}` | the measure becomes a Dataset `metric` (see [`measure`](#measure)) |
| `{% parameter %}`, `{% condition %}`, other Liquid | see [parameter.md](parameter.md), [filter.md](filter.md), [liquid.md](liquid.md) |

The helper dimension carries the SQL text unchanged (references rewritten per
[sql-references.md](sql-references.md)), with `hidden: true`. Two measures with the same SQL share one helper.

```lookml
measure: total_margin {
  type: sum
  sql: ${sale_price} - ${TABLE}.cost ;;
}
```
```aml
dimension total_margin__operand {
  label: 'Total Margin (operand)'
  type: 'number'
  hidden: true
  definition: @sql {{ sale_price }} - {{ #SOURCE.cost }};;
}
measure total_margin {
  label: 'Total Margin'
  type: 'number'
  definition: @aql sum(order_items.total_margin__operand);;
}
```

When the SQL is simple enough to translate with certainty, you may write it inline in AQL instead and skip the
helper:

```aml
measure total_margin {
  label: 'Total Margin'
  type: 'number'
  definition: @aql sum(order_items.sale_price - order_items.cost);;
}
```

For non-aggregate types, `sql` is an expression over other measures; see [`number`](#number).

## `filters`

Looker: [`filters`](https://docs.cloud.google.com/looker/docs/reference/param-field-filters) ·
Holistics: [`where`](https://docs.holistics.io/reference/aql/where)

`filters` → `| where(…)` after the aggregate.

1. Convert each `field: "expression"` pair to one AQL condition. The type of the dimension decides the grammar;
   the full list of expressions is in [filter-expressions.md](filter-expressions.md).
2. Put all conditions in one `where()`, separated by commas. They are combined with AND, as in Looker.
3. The old syntax `filters: { field: status  value: "complete" }` converts the same way.
4. A filter on a field of another view makes the measure a Dataset `metric` (see [`measure`](#measure)).

```lookml
measure: completed_revenue {
  type: sum
  sql: ${sale_price} ;;
  filters: [status: "complete", is_returned: "no", sale_price: ">100"]
}
```
```aml
measure completed_revenue {
  label: 'Completed Revenue'
  type: 'number'
  definition: @aql sum(order_items.sale_price)
    | where(
      order_items.status == 'complete',
      order_items.is_returned == false,
      order_items.sale_price > 100
    );;
}
```

Notes:

- A condition that compares two fields or computes on fields needs a primary key on the model. A plain
  `field <operator> constant` condition does not.
- A condition on a field of another model: mark for review. The result should match Looker's, but it has not been
  checked on data.

## `type`

Looker: [Measure types](https://docs.cloud.google.com/looker/docs/reference/param-measure-types) ·
Holistics: [Aggregate functions](https://docs.holistics.io/reference/aql/aggregator-functions)

In every section below, `<operand>` is the operand found in [`sql`](#sql).

### `average`

Looker: [`average`](https://docs.cloud.google.com/looker/docs/reference/param-measure-types#average)

`type: average` → `average(<operand>)`.

```lookml
measure: average_sale_price { type: average  sql: ${sale_price} ;; }
```
```aml
measure average_sale_price {
  label: 'Average Sale Price'
  type: 'number'
  definition: @aql average(order_items.sale_price);;
}
```

### `average_distinct`, `median_distinct`, `percentile_distinct`, `sum_distinct`

Looker: [`average_distinct`](https://docs.cloud.google.com/looker/docs/reference/param-measure-types#average_distinct),
[`median_distinct`](https://docs.cloud.google.com/looker/docs/reference/param-measure-types#median_distinct),
[`percentile_distinct`](https://docs.cloud.google.com/looker/docs/reference/param-measure-types#percentile_distinct),
[`sum_distinct`](https://docs.cloud.google.com/looker/docs/reference/param-measure-types#sum_distinct),
[`sql_distinct_key`](https://docs.cloud.google.com/looker/docs/reference/param-field-sql-distinct-key) ·
Holistics: [`unique`](https://docs.holistics.io/reference/aql/unique),
[Nested aggregation](https://docs.holistics.io/as-code/aql/learn/nested-aggregation)

These types aggregate one value per key, on a table that repeats the value on several rows. The key is the dimension
in `sql_distinct_key`, or the view's primary key when the parameter is absent. If `sql_distinct_key` is a SQL
expression, put it in a hidden helper dimension, as for [`sql`](#sql).

- **The key is the view's primary key** → the type is redundant. Emit the plain aggregate: `sum(<operand>)`,
  `average(<operand>)`, `median(<operand>)`, `percentile_cont(<operand>, N/100)`.
- **Any other key** → nested aggregation:

| LookML `type` | AQL |
|---|---|
| `sum_distinct` | `unique(<key>) \| sum(max(<operand>))` |
| `average_distinct` | `unique(<key>) \| average(max(<operand>))` |
| `median_distinct` | `unique(<key>) \| median(max(<operand>))` |
| `percentile_distinct` with `percentile: N` | `unique(<key>) \| percentile_cont(max(<operand>), N/100)` |

```lookml
measure: total_shipping {
  type: sum_distinct
  sql_distinct_key: ${order_id} ;;
  sql: ${order_shipping} ;;
}
```
```aml
measure total_shipping {
  label: 'Total Shipping'
  type: 'number'
  definition: @aql unique(order_items.order_id) | sum(max(order_items.order_shipping));;
}
```

If the key or the operand belongs to another view, the measure becomes a Dataset `metric` (see [`measure`](#measure)).

Difference: Looker requires one value per key. `max()` picks that value. If the data has several values for a key,
the AQL takes the largest; Looker's result is then undefined. Mark these measures for review.

### `count`

Looker: [`count`](https://docs.cloud.google.com/looker/docs/reference/param-measure-types#count)

`type: count` has no `sql`. AQL cannot count a model, only a field.

- The view has a `primary_key: yes` dimension → `count(<model>.<primary key>)`.
- No primary key → add a hidden helper dimension `row_marker` and count it. Report the view: a person should pick a
  real primary key.

```lookml
measure: count { type: count }
```
```aml
measure count {
  label: 'Count'
  type: 'number'
  definition: @aql count(order_items.id);;
}
```

Without a primary key:

```aml
dimension row_marker {
  label: 'Row Marker'
  type: 'number'
  hidden: true
  definition: @sql 1;;
}
measure count {
  label: 'Count'
  type: 'number'
  definition: @aql count(order_items.row_marker);;
}
```

Difference: `count(field)` skips NULL values, `COUNT(*)` does not. The results are the same when the key is never
NULL, which a primary key must be.

### `count_distinct`

Looker: [`count_distinct`](https://docs.cloud.google.com/looker/docs/reference/param-measure-types#count_distinct),
[`approximate`](https://docs.cloud.google.com/looker/docs/reference/param-field-approximate)

`type: count_distinct` → `count_distinct(<operand>)`.

With `approximate: yes` → `approx_count_distinct(<operand>)`. AQL supports it on Snowflake, BigQuery, Databricks,
MotherDuck and Presto/Athena. On another database, emit `count_distinct` and report it.

`approximate: yes` on a `type: count` measure → drop the flag and emit the exact `count(…)`.

```lookml
measure: buyers        { type: count_distinct  sql: ${user_id} ;; }
measure: buyers_approx { type: count_distinct  sql: ${user_id} ;;  approximate: yes }
```
```aml
measure buyers {
  label: 'Buyers'
  type: 'number'
  definition: @aql count_distinct(order_items.user_id);;
}
measure buyers_approx {
  label: 'Buyers Approx'
  type: 'number'
  definition: @aql approx_count_distinct(order_items.user_id);;
}
```

A `count_distinct` measure without `sql` cannot be converted. Report it.

### `date`

Looker: [`date`](https://docs.cloud.google.com/looker/docs/reference/param-measure-types#date)

A non-aggregate type: `sql` holds the aggregate itself, usually `MAX(…)` or `MIN(…)` of a date.

1. `MAX(<x>)` / `MIN(<x>)` → `max(<operand>)` / `min(<operand>)`, with the operand found as in [`sql`](#sql).
   A `${created_raw}` reference resolves to the dimension group's base dimension
   (see [dimension-group.md](dimension-group.md)).
2. AML `type: 'date'`. When the operand is a datetime, wrap the result: `cast(max(…), 'date')`.
3. `type: date_time` → AML `type: 'datetime'`, without the cast.
4. `convert_tz` and `datatype` on a measure → drop.

```lookml
measure: last_order_date {
  type: date
  sql: MAX(${created_raw}) ;;
  convert_tz: no
}
```
```aml
measure last_order_date {
  label: 'Last Order Date'
  type: 'date'
  definition: @aql cast(max(order_items.created), 'date');;
}
```

Other SQL: translate as described under [`number`](#number).

### `list`

Looker: [`list`](https://docs.cloud.google.com/looker/docs/reference/param-measure-types#list)

`type: list` with `list_field: f` → `string_agg(<model>.<f>, sep: ', ', distinct: true)`, AML `type: 'text'`.

```lookml
measure: status_list { type: list  list_field: status }
```
```aml
measure status_list {
  label: 'Status List'
  type: 'text'
  definition: @aql string_agg(order_items.status, sep: ', ', distinct: true);;
}
```

Difference: Looker's separator and ordering depend on the database. The AQL result is one text value with `', '`
between the values, in no fixed order; add `order: 'asc'` for a stable order. SQL Server does not support
`distinct` here.

### `max`, `min`

Looker: [`max`](https://docs.cloud.google.com/looker/docs/reference/param-measure-types#max),
[`min`](https://docs.cloud.google.com/looker/docs/reference/param-measure-types#min)

`type: max` → `max(<operand>)`; `type: min` → `min(<operand>)`.

```lookml
measure: largest_sale  { type: max  sql: ${sale_price} ;; }
measure: smallest_sale { type: min  sql: ${sale_price} ;; }
```
```aml
measure largest_sale {
  label: 'Largest Sale'
  type: 'number'
  definition: @aql max(order_items.sale_price);;
}
measure smallest_sale {
  label: 'Smallest Sale'
  type: 'number'
  definition: @aql min(order_items.sale_price);;
}
```

### `median`

Looker: [`median`](https://docs.cloud.google.com/looker/docs/reference/param-measure-types#median)

`type: median` → `median(<operand>)`.

```lookml
measure: median_sale_price { type: median  sql: ${sale_price} ;; }
```
```aml
measure median_sale_price {
  label: 'Median Sale Price'
  type: 'number'
  definition: @aql median(order_items.sale_price);;
}
```

Difference: for an even number of rows Looker returns the mean of the two middle values. The Holistics docs do not
say which value AQL `median` returns in that case, and database support differs on both sides. Mark for review.

### `number`

Looker: [`number`](https://docs.cloud.google.com/looker/docs/reference/param-measure-types#number) ·
Holistics: [Null and zero functions](https://docs.holistics.io/reference/aql/null-and-zero-functions),
[Logical functions](https://docs.holistics.io/reference/aql/logical-functions)

A non-aggregate type: `sql` is an expression over other measures (`${measure}`), sometimes with SQL aggregates.
Translate the expression to AQL, with qualified references. Try in this order:

1. **Measure arithmetic.** Only `${measure}` references, numbers, `+ - * /`, parentheses and these idioms:

   | SQL | AQL |
   |---|---|
   | `${a} / NULLIF(${b}, 0)` | `safe_divide(m.a, m.b)` |
   | `1.0 * ${a} / ${b}` | `m.a * 1.0 / m.b` |
   | `COALESCE(x, y)` | `coalesce(x, y)` |
   | `=`, `<>` | `==`, `!=` |
   | `CASE WHEN c THEN x ELSE y END` | `case(when: c, then: x, else: y)` |

2. **One SQL aggregate over this view.** `SUM(x)`, `AVG(x)`, `MIN(x)`, `MAX(x)`, `COUNT(DISTINCT x)` → the AQL
   aggregate over the operand of `x` (see [`sql`](#sql)).
3. **Other SQL.** Translate it to AQL by hand and validate. Mark for review.
4. **Fallback**, only when the SQL references no measure and no `@aql` dimension. Keep the SQL:
   `definition: @sql …;;` with `${TABLE}.col` and `${dim}` rewritten to `{{ #SOURCE.col }}` and `{{ dim }}`. Report
   the measure: a raw SQL measure is not protected against fan-out (see [`measure`](#measure)). Not available for
   a Dataset `metric`.
5. **Otherwise** the measure cannot be converted mechanically. `@sql` cannot reference an `@aql` field, and every
   measure is `@aql`: `@sql 1.0 * {{ returned_count }} / {{ count }}` fails with E210 as soon as AQL uses the
   measure (see [sql-references.md](sql-references.md#field)). Report it for manual work.

```lookml
measure: average_item_value {
  type: number
  sql: ${total_sale_price} / NULLIF(${count}, 0) ;;
}
measure: return_rate {
  type: number
  sql: 1.0 * ${returned_count} / ${count} ;;
}
```
```aml
measure average_item_value {
  label: 'Average Item Value'
  type: 'number'
  definition: @aql safe_divide(order_items.total_sale_price, order_items.count);;
}
measure return_rate {
  label: 'Return Rate'
  type: 'number'
  definition: @aql order_items.returned_count * 1.0 / order_items.count;;
}
```

The fallback form, for `measure: any_returned { type: yesno  sql: BOOL_OR(${is_returned}) ;; }` where
`is_returned` is an `@sql` dimension:

```aml
measure any_returned {
  label: 'Any Returned'
  type: 'truefalse'
  definition: @sql BOOL_OR({{ is_returned }});;
}
```

Differences:

- In Looker, a raw aggregate inside a `type: number` measure is not protected by symmetric aggregates. The AQL
  translation is protected, so across a one-to-many join the converted measure can return a different (correct)
  number.
- `${a} / ${b}` on two integers truncates on some databases. Whether AQL `/` does the same is not documented.
  Mark plain integer divisions for review.

### `percent_of_previous`

Looker: [`percent_of_previous`](https://docs.cloud.google.com/looker/docs/reference/param-measure-types#percent_of_previous) ·
Holistics: [`previous`](https://docs.holistics.io/reference/aql/previous)

`sql` must be one `${measure}` reference `m`. Emit the change against the previous row:

```lookml
measure: count_growth { type: percent_of_previous  sql: ${count} ;; }
```
```aml
measure count_growth {
  label: 'Count Growth'
  type: 'number'
  definition: @aql safe_divide(
    order_items.count - previous(order_items.count, order: 'rows'),
    previous(order_items.count, order: 'rows')
  );;
}
```

Difference: Looker computes this after the query, over the rows as displayed and sorted. AQL computes it inside the
query, along the row dimensions. Mark for review.

### `percent_of_total`

Looker: [`percent_of_total`](https://docs.cloud.google.com/looker/docs/reference/param-measure-types#percent_of_total),
[`direction`](https://docs.cloud.google.com/looker/docs/reference/param-field-direction) ·
Holistics: [`percent_of_total`](https://docs.holistics.io/reference/aql/percent_of_total)

`sql` must be one `${measure}` reference `m` → `percent_of_total(<model>.<m>, 'column_total')`.

`direction` only matters in a pivoted table, and a model measure cannot know whether it will be pivoted. Emit
`'column_total'` in every case and report measures that had `direction: "row"` (or no `direction`), for which
`'row_total'` is the pivoted equivalent.

```lookml
measure: percent_of_revenue {
  type: percent_of_total
  sql: ${total_sale_price} ;;
  direction: "column"
}
```
```aml
measure percent_of_revenue {
  label: 'Percent Of Revenue'
  type: 'number'
  definition: @aql percent_of_total(order_items.total_sale_price, 'column_total');;
}
```

Differences:

- Looker computes the total over the rows returned (it respects the row limit); AQL computes it inside the query.
- Check the scale (0–1 or 0–100) of the result before you attach a percentage `format`.

### `percentile`

Looker: [`percentile`](https://docs.cloud.google.com/looker/docs/reference/param-measure-types#percentile)

`type: percentile` with `percentile: N` → `percentile_cont(<operand>, N/100)`, with the fraction written as a
number (`75` → `0.75`). On BigQuery, MySQL and Presto/Athena, where AQL has no aggregate `percentile_cont`, use
`percentile_disc`. A measure without `percentile` or `sql` cannot be converted; report it.

```lookml
measure: p75_sale_price {
  type: percentile
  percentile: 75
  sql: ${sale_price} ;;
}
```
```aml
measure p75_sale_price {
  label: 'P75 Sale Price'
  type: 'number'
  definition: @aql percentile_cont(order_items.sale_price, 0.75);;
}
```

Difference: Looker documents its own rule for picking the value, which is neither `percentile_cont` nor
`percentile_disc` exactly. Results can differ slightly on small groups. Mark for review.

### `period_over_period`

Looker: [`period_over_period`](https://docs.cloud.google.com/looker/docs/reference/param-measure-types#period_over_period),
[Period-over-period measures](https://docs.cloud.google.com/looker/docs/period-over-period) ·
Holistics: [`relative_period`](https://docs.holistics.io/reference/aql/relative_period)

Let `<m>` be the measure in `based_on`, and `<t>` the datetime dimension behind the timeframe in `based_on_time`
(see [dimension-group.md](dimension-group.md)). The previous-period value is
`<m> | relative_period(<t>, interval(-1 <period>))`.

| `kind` | AQL |
|---|---|
| `previous` (default) | the previous-period value |
| `difference` | `<m>` − the previous-period value |
| `relative_change` | `safe_divide(<m> − previous, previous)` |

```lookml
measure: count_last_month {
  type: period_over_period
  based_on: order_items.count
  based_on_time: order_items.created_month
  period: month
  kind: previous
}
measure: count_change {
  type: period_over_period
  based_on: order_items.count
  based_on_time: order_items.created_month
  period: month
  kind: relative_change
}
```
```aml
measure count_last_month {
  label: 'Count Last Month'
  type: 'number'
  definition: @aql order_items.count | relative_period(order_items.created, interval(-1 month));;
}
measure count_change {
  label: 'Count Change'
  type: 'number'
  definition: @aql safe_divide(
    order_items.count - (order_items.count | relative_period(order_items.created, interval(-1 month))),
    order_items.count | relative_period(order_items.created, interval(-1 month))
  );;
}
```

`period: fiscal_year`, `fiscal_quarter` and `custom_calendar_period` are unsupported; report them.

Mark every converted `period_over_period` measure for review: the behaviour on partial periods has not been
compared.

### `running_total`

Looker: [`running_total`](https://docs.cloud.google.com/looker/docs/reference/param-measure-types#running_total) ·
Holistics: [`running_total`](https://docs.holistics.io/reference/aql/running_total)

`sql` must be one `${measure}` reference `m` → `running_total(<model>.<m>)`. Drop `direction`.

```lookml
measure: cumulative_revenue { type: running_total  sql: ${total_sale_price} ;; }
```
```aml
measure cumulative_revenue {
  label: 'Cumulative Revenue'
  type: 'number'
  definition: @aql running_total(order_items.total_sale_price);;
}
```

Difference: Looker runs along the rows as displayed and sorted. AQL `running_total` runs along the date dimensions
of the query. The two agree only for reports sorted by date. Mark for review.

### `string`

Looker: [`string`](https://docs.cloud.google.com/looker/docs/reference/param-measure-types#string)

A non-aggregate type, and the type of a measure that has no `type`. AML `type: 'text'`. Translate `sql` as
described under [`number`](#number); the usual shape is one aggregate.

```lookml
measure: latest_status { type: string  sql: MAX(${status}) ;; }
```
```aml
measure latest_status {
  label: 'Latest Status'
  type: 'text'
  definition: @aql max(order_items.status);;
}
```

A measure with no `type` whose SQL is plain arithmetic (`${a} / ${b}`) is really a number: emit `type: 'number'`.

### `sum`

Looker: [`sum`](https://docs.cloud.google.com/looker/docs/reference/param-measure-types#sum)

`type: sum` → `sum(<operand>)`.

```lookml
measure: total_sale_price { type: sum  sql: ${sale_price} ;; }
```
```aml
measure total_sale_price {
  label: 'Total Sale Price'
  type: 'number'
  definition: @aql sum(order_items.sale_price);;
}
```

Difference: over no rows, a Looker sum may show 0 where AQL `sum` shows nothing; the docs of neither side settle
this. If a 0 is required, emit `coalesce(sum(<operand>), 0)`.

### `yesno`

Looker: [`yesno`](https://docs.cloud.google.com/looker/docs/reference/param-measure-types#yesno)

A non-aggregate type: `sql` is a condition over measures. AML `type: 'truefalse'`; translate the condition as
described under [`number`](#number).

```lookml
measure: is_large_total { type: yesno  sql: ${total_sale_price} > 1000 ;; }
```
```aml
measure is_large_total {
  label: 'Is Large Total'
  type: 'truefalse'
  definition: @aql order_items.total_sale_price > 1000;;
}
```

## Query parameters

### `approximate`

See [`count_distinct`](#count_distinct).

### `approximate_threshold`

Looker: [`approximate_threshold`](https://docs.cloud.google.com/looker/docs/reference/param-field-approximate-threshold)

Not needed. Drop it.

### `allow_approximate_optimization`

Looker: [`allow_approximate_optimization`](https://docs.cloud.google.com/looker/docs/reference/param-field-allow-approximate-optimization)

Drop it on the measure. It only matters for aggregate tables, which are unsupported; see
[aggregate-awareness.md](aggregate-awareness.md).

### `precision`

Looker: [`precision`](https://docs.cloud.google.com/looker/docs/reference/param-field-precision)

Not needed. Drop it. It tunes Looker's symmetric aggregates, which Holistics does not use.

### `sql_distinct_key`

See [distinct-key types](#average_distinct-median_distinct-percentile_distinct-sum_distinct).

## Value and formatting parameters

### `direction`

See [`percent_of_total`](#percent_of_total).

### `list_field`

See [`list`](#list).

### `percentile`

See [`percentile`](#percentile).
