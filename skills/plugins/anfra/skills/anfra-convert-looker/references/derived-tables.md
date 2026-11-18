# LookML `derived_table` → AML query model

How to convert a LookML derived table: the SQL kind (`sql`), the native kind (`explore_source`), and the
parameters that persist it, which anfra does not support. The sections follow the derived-table parameters of
Looker's [`derived_table`](https://docs.cloud.google.com/looker/docs/reference/param-view-derived-table) page.

A view with a SQL derived table becomes a [query model](https://docs.holistics.io/reference/aml/query-model). The
SQL is carried over as it is:

```lookml
view: completed_orders {
  derived_table: {
    sql:
      WITH refunds AS (
        SELECT order_id, SUM(amount) AS refunded
        FROM public.refunds
        GROUP BY 1
      )
      SELECT o.id AS order_id, o.user_id, o.amount - COALESCE(r.refunded, 0) AS net_amount
      FROM public.orders o
      LEFT JOIN refunds r ON r.order_id = o.id
      WHERE o.status = 'complete' ;;
  }

  dimension: order_id { primary_key: yes  type: number  sql: ${TABLE}.order_id ;; }
  dimension: user_id { type: number  sql: ${TABLE}.user_id ;; }
  dimension: net_amount { type: number  sql: ${TABLE}.net_amount ;; }
}
```
```aml
Model completed_orders {
  type: 'query'
  label: 'Completed Orders'
  data_source_name: 'demodb'
  models: []

  query: @sql
    WITH refunds AS (
      SELECT order_id, SUM(amount) AS refunded
      FROM public.refunds
      GROUP BY 1
    )
    SELECT o.id AS order_id, o.user_id, o.amount - COALESCE(r.refunded, 0) AS net_amount
    FROM public.orders o
    LEFT JOIN refunds r ON r.order_id = o.id
    WHERE o.status = 'complete'
  ;;

  dimension order_id {
    label: 'Order Id'
    type: 'number'
    primary_key: true
    definition: @sql {{ #SOURCE.order_id }};;
  }
  dimension user_id {
    label: 'User Id'
    type: 'number'
    definition: @sql {{ #SOURCE.user_id }};;
  }
  dimension net_amount {
    label: 'Net Amount'
    type: 'number'
    definition: @sql {{ #SOURCE.net_amount }};;
  }
}
```

**The validator does not read SQL.** `anfra validate` checks the structure of a query model and the AQL of
its measures. It does not check the `query` text or the model references in it.

## Quick reference

| LookML | AML | Section |
|---|---|---|
| `derived_table` | `Model { type: 'query' }` | [derived_table](#derived_table) |
| `sql` | `query: @sql … ;;` | [sql](#sql) |
| `${view.SQL_TABLE_NAME}` in the SQL | `{{ #view AS alias }}` + `models: […]` | [References to other views](#references-to-other-views) |
| Liquid in the SQL | `param` + `{% filter() %}` / `{{ param }}`, or unsupported | [Liquid in the SQL](#liquid-in-the-sql) |
| `explore_source` (native derived table) | Dataset dimensions, a window dimension, or a SQL query model | [explore_source](#explore_source) |
| `column`, `filters` | `measure \| where(…) \| dimensionalize(key)` | [As Dataset dimensions](#as-dataset-dimensions) |
| `derived_column` | AQL window dimension, or SQL | [As a window dimension](#as-a-window-dimension) |
| `expression_custom_filter`, `sorts`, `limit`, `timezone` | parts of a hand-written SQL query | [As a SQL query model](#as-a-sql-query-model) |
| `bind_filters` | `param` + a condition on it | [bind_filters](#bind_filters) |
| `bind_all_filters` | unsupported; metrics as the closest | [bind_all_filters](#bind_all_filters) |
| `dev_filters` | unsupported | [dev_filters](#dev_filters) |
| `datagroup_trigger`, `sql_trigger_value`, `interval_trigger`, `persist_for`, `increment_key`, `increment_offset`, `materialized_view`, `publish_as_db_view`, `indexes`, `sortkeys`, `distribution`, `distribution_style`, `partition_keys`, `cluster_keys`, `table_compression`, `table_format` | unsupported | [Persistence parameters](#persistence-parameters) |
| `create_process`, `sql_create` | `query` = the `SELECT` | [create_process, sql_create](#create_process-sql_create) |

---

## `derived_table`

Looker: [`derived_table`](https://docs.cloud.google.com/looker/docs/reference/param-view-derived-table),
[Derived tables in Looker](https://docs.cloud.google.com/looker/docs/derived-tables) ·
Holistics: [AML Query Model](https://docs.holistics.io/reference/aml/query-model),
[Query Model](https://docs.holistics.io/docs/query-models)

What the block contains decides the conversion:

| The block has | Conversion |
|---|---|
| `sql` | a query model — see [`sql`](#sql) |
| `explore_source` | no single target — see [`explore_source`](#explore_source) |
| `create_process` or `sql_create` | a query model on the `SELECT` — see [`create_process`, `sql_create`](#create_process-sql_create) |

For a query model, everything in [view.md](view.md) about the name, label, `data_source_name`, file and fields
still applies. The model has `type: 'query'` and no `table_name`.

anfra inserts the model's SQL into each query as a CTE, as Looker does with a temporary derived table. The
parameters that persist a derived table are dropped; see [Persistence parameters](#persistence-parameters).

## `sql`

Looker: [`sql` (for derived tables)](https://docs.cloud.google.com/looker/docs/reference/param-view-sql-for-derived-table) ·
Holistics: [AML Query Model](https://docs.holistics.io/reference/aml/query-model)

`sql` → `query: @sql … ;;`. Copy the SQL text and change only this:

1. `${view.SQL_TABLE_NAME}` → a model reference (see [References to other views](#references-to-other-views)).
2. `@{constant}` → `${constant}` (see [manifest.md](manifest.md#constant)).
3. Liquid → see [Liquid in the SQL](#liquid-in-the-sql).
4. Remove a trailing `;`. Holistics embeds the query as a subquery or a CTE.

Leave everything else unchanged: physical table names, CTEs, window functions, dialect functions, SQL comments.
The SQL is not translated; it runs on the same database as before.

Do not replace a physical table (`FROM public.orders`) with a model reference, even when a model exists for that
table. A query model may select from tables directly.

The headline example at the top of this page shows the result.

**Fields.** The fields of the view convert as for a table view: `${TABLE}.col` → `{{ #SOURCE.col }}`, where
`#SOURCE` is now the query result (see [dimension.md](dimension.md)). Three points are specific to derived tables:

- A dimension without `sql` reads the column that has the dimension's name. Emit `{{ #SOURCE.<name> }}`.
- Do not add dimensions for result columns that the view does not declare.
- Do not guess a primary key from the SQL (`GROUP BY 1` is not proof). If the view declares none, report it:
  `type: count` and some AQL functions need one (see [measure.md](measure.md#count)).

Differences:

- A derived SQL that starts with `WITH` becomes a CTE inside a CTE. Most databases accept this. Mark for review on
  SQL Server and for BigQuery `WITH RECURSIVE`.
- Both tools filter the result of the derived table, not its inner query, unless the SQL has a templated filter.

### References to other views

Holistics: [Query Syntax](https://docs.holistics.io/reference/aml/query-model#query-syntax),
[Model Dependencies](https://docs.holistics.io/docs/query-models#model-dependencies)

1. Ignore references inside SQL comments.
2. `${v.SQL_TABLE_NAME}` → `{{ #v }}`.
3. **Move the alias inside the braces.** `${v.SQL_TABLE_NAME} AS a` and `${v.SQL_TABLE_NAME} a` → `{{ #v AS a }}`.
4. `models: […]` lists each referenced model once, in order of first appearance. Physical tables are not listed.
5. If `v` is not a view of the project (for example a view of an imported project that was not supplied), the
   reference cannot be resolved. Report it.

```lookml
view: user_order_facts {
  derived_table: {
    sql:
      SELECT co.user_id,
             u.country,
             COUNT(*)           AS lifetime_orders,
             SUM(co.net_amount) AS lifetime_revenue
      FROM ${completed_orders.SQL_TABLE_NAME} AS co
      JOIN ${users.SQL_TABLE_NAME} AS u ON u.id = co.user_id
      GROUP BY 1, 2 ;;
  }

  dimension: user_id { primary_key: yes  type: number  sql: ${TABLE}.user_id ;; }
  dimension: country { type: string  sql: ${TABLE}.country ;; }
  dimension: lifetime_orders { type: number  sql: ${TABLE}.lifetime_orders ;; }
  dimension: lifetime_revenue { type: number  sql: ${TABLE}.lifetime_revenue ;; }
  measure: average_lifetime_revenue { type: average  sql: ${lifetime_revenue} ;; }
}
```
```aml
Model user_order_facts {
  type: 'query'
  label: 'User Order Facts'
  data_source_name: 'demodb'
  models: [completed_orders, users]

  query: @sql
    SELECT co.user_id,
           u.country,
           COUNT(*)           AS lifetime_orders,
           SUM(co.net_amount) AS lifetime_revenue
    FROM {{ #completed_orders AS co }}
    JOIN {{ #users AS u }} ON u.id = co.user_id
    GROUP BY 1, 2
  ;;

  dimension user_id {
    label: 'User Id'
    type: 'number'
    primary_key: true
    definition: @sql {{ #SOURCE.user_id }};;
  }
  dimension country {
    label: 'Country'
    type: 'text'
    definition: @sql {{ #SOURCE.country }};;
  }
  dimension lifetime_orders {
    label: 'Lifetime Orders'
    type: 'number'
    definition: @sql {{ #SOURCE.lifetime_orders }};;
  }
  dimension lifetime_revenue {
    label: 'Lifetime Revenue'
    type: 'number'
    definition: @sql {{ #SOURCE.lifetime_revenue }};;
  }
  measure average_lifetime_revenue {
    label: 'Average Lifetime Revenue'
    type: 'number'
    definition: @aql average(user_order_facts.lifetime_revenue);;
  }
}
```

Mark every such model for review. The reference does not mean the same thing in the two tools:

- Looker replaces `${v.SQL_TABLE_NAME}` with a table name, so the SQL sees the **columns** of that table.
  Holistics replaces `{{ #v AS a }}` with a subquery over the model, which may expose the model's **dimensions by
  their names**. The example above is safe either way: every column it reads (`user_id`, `net_amount`, `id`,
  `country`) is also a dimension with that name.
- A column that has no dimension, or whose dimension has another name, may then not resolve. This is common with a
  `dimension_group`: the column is `created_at`, the dimension is `created`.
- If it does not resolve: for a table view, write the table name instead of `{{ #v }}`. For a derived table, add a
  hidden dimension for the missing column to `v`, or rename the column in the SQL.

A reference inlines the SQL of the referenced model. Looker read the persisted table instead when the view was
persisted; the rows are the same unless that table was out of date.

### Liquid in the SQL

Looker: [Templated filters and Liquid parameters](https://docs.cloud.google.com/looker/docs/templated-filters) ·
Holistics: [Dynamic Query Model](https://docs.holistics.io/docs/query-parameters)

| Liquid in the derived SQL | Conversion |
|---|---|
| `{% condition f %} col {% endcondition %}` | `param f` + `{% filter(f) %} col {% end %}` — see [filter.md](filter.md#condition-in-a-derived-table) |
| `{% date_start f %}`, `{% date_end f %}` | `{% filter(f) %}`, or two date params — see [filter.md](filter.md#date_start-date_end) |
| `{% parameter p %}`, `{{ p._parameter_value }}` | `param p` + `{{ p }}` — see [parameter.md](parameter.md#in-a-derived-table) |
| `{% incrementcondition %} … {% endincrementcondition %}` | removed — see [Persistence parameters](#persistence-parameters) |
| `_user_attributes['x']` | see [liquid.md](liquid.md) |
| `{% if %}`, `{% assign %}`, `{% for %}`, `_in_query`, `_is_selected`, `_is_filtered`, `_filters[…]`, `_model._name`, `_explore._name` | Unsupported in general: a query model has no template logic and cannot see which fields a report uses. See [liquid.md](liquid.md). |

---

## `explore_source`

Looker: [`explore_source`](https://docs.cloud.google.com/looker/docs/reference/param-view-explore-source),
[Creating native derived tables](https://docs.cloud.google.com/looker/docs/creating-ndts) ·
Holistics: [Level of Detail](https://docs.holistics.io/as-code/aql/learn/level-of-detail),
[`dimensionalize`](https://docs.holistics.io/reference/aql/dimensionalize)

A native derived table (NDT) is the result of an Explore query, used as a table. Holistics has no model that is
defined by a dataset query: `query` accepts SQL only. Do not write `query: @aql …`. It compiles, but it is not
checked and not supported.

So an NDT has no single target. Each one encodes an intent (facts per customer, a rank, a share of a total), and
the target depends on its shape. Use the first row that matches:

| The NDT | Conversion |
|---|---|
| has `bind_all_filters: yes` | Unsupported. Metrics as the closest — see [`bind_all_filters`](#bind_all_filters) |
| has `bind_filters` | a `param` and a condition on it — see [`bind_filters`](#bind_filters) |
| is the base view of an explore, or has `limit`, or its rows are not one per primary key of a model | a SQL query model — see [As a SQL query model](#as-a-sql-query-model) |
| has only dimension columns of one view, plus window `derived_column`s | a window dimension on that model — see [As a window dimension](#as-a-window-dimension) |
| has one row per primary key of a model, with measure columns, and is joined back on that key | Dataset dimensions — see [As Dataset dimensions](#as-dataset-dimensions) |

Mark every converted NDT for review.

The examples below use this explore and these two views:

```lookml
explore: orders {
  join: users {
    relationship: many_to_one
    sql_on: ${orders.user_id} = ${users.id} ;;
  }
}

view: orders {
  sql_table_name: public.orders ;;
  dimension: id { primary_key: yes  type: number  sql: ${TABLE}.id ;; }
  dimension: user_id { type: number  sql: ${TABLE}.user_id ;; }
  dimension: status { type: string  sql: ${TABLE}.status ;; }
  dimension: amount { type: number  sql: ${TABLE}.amount ;; }
  dimension_group: created { type: time  timeframes: [raw, date, month]  sql: ${TABLE}.created_at ;; }
  measure: count { type: count }
  measure: total_amount { type: sum  sql: ${amount} ;; }
}

view: users {
  sql_table_name: public.users ;;
  dimension: id { primary_key: yes  type: number  sql: ${TABLE}.id ;; }
  dimension: country { type: string  sql: ${TABLE}.country ;; }
}
```

### As Dataset dimensions

Holistics: [`dimensionalize`](https://docs.holistics.io/reference/aql/dimensionalize),
[`where`](https://docs.holistics.io/reference/aql/where)

Use this when all of the following hold. Otherwise use [a SQL query model](#as-a-sql-query-model).

- No `bind_filters`, `bind_all_filters`, `limit` or `derived_column`.
- Exactly one dimension column is the primary key of a model `E` of the dataset, or a foreign key that the explore
  joins to it. Other dimension columns, if any, are dimensions of `E`. No column is a timeframe (`created_month`).
- Every explore joins the NDT view to `E` on that key.

A `column` without `field` reads the field of the explore's base view that has the column's name.

1. **Emit no model for the NDT view.** Remove its join from the datasets.
2. **Each measure column** → a Dataset dimension with the column's name and `model: E`:
   `definition: @aql <measure> | dimensionalize(E.<key>);;`. Take the label, description and format from the
   dimension that the NDT view declares for the column.
3. **Dimension columns** need no field. They are dimensions of `E` already.
4. **`filters`** → `| where(…)` between the measure and `dimensionalize`. Convert each expression with
   [filter-expressions.md](filter-expressions.md). An empty value `""` emits nothing.
5. **Other fields of the NDT view** move to the dataset too: a dimension over a column → a Dataset dimension with
   `model: E`; a measure → a Dataset `metric`.
6. Rewrite every reference `ndt_view.field` in the project to `E.field`. If `E` already has a field with that
   name, prefix the new one with the NDT view's name.

```lookml
view: user_order_facts {
  derived_table: {
    explore_source: orders {
      column: user_id { field: users.id }
      column: lifetime_orders { field: orders.count }
      column: lifetime_revenue { field: orders.total_amount }
      filters: [orders.status: "complete"]
    }
  }

  dimension: user_id { primary_key: yes  hidden: yes  type: number }
  dimension: lifetime_orders { type: number }
  dimension: lifetime_revenue { type: number }
  dimension: is_repeat_customer { type: yesno  sql: ${lifetime_orders} > 1 ;; }
  measure: average_lifetime_revenue { type: average  sql: ${lifetime_revenue} ;; }
}
```
```aml
Dataset orders_dataset {
  label: 'Orders'
  data_source_name: 'demodb'
  models: [orders, users]
  relationships: [
    relationship(orders.user_id > users.id, true)
  ]

  dimension lifetime_orders {
    model: users
    label: 'Lifetime Orders'
    type: 'number'
    definition: @aql orders.count | where(orders.status == 'complete') | dimensionalize(users.id);;
  }
  dimension lifetime_revenue {
    model: users
    label: 'Lifetime Revenue'
    type: 'number'
    definition: @aql orders.total_amount | where(orders.status == 'complete') | dimensionalize(users.id);;
  }
  dimension is_repeat_customer {
    model: users
    label: 'Is Repeat Customer'
    type: 'truefalse'
    definition: @aql coalesce(users.lifetime_orders > 1, false);;
  }
  metric average_lifetime_revenue {
    label: 'Average Lifetime Revenue'
    type: 'number'
    definition: @aql average(users.lifetime_revenue);;
  }
}
```

A static NDT ignores the filters of the report. So does a dimension with `dimensionalize`: it is computed before
the report's filters apply. This is why the target is a dimension and not a metric.

Differences:

- **Keys without rows.** The NDT has a row only for users with a complete order; after Looker's left join the
  other users get NULL. Whether `dimensionalize` gives NULL or 0 for a count over no rows is not documented.
- **Inner joins.** A `type: inner` join to the NDT also removed the other users from the explore. That is lost.
- **Filters of the explore.** The NDT query runs through the explore, so the explore's `sql_always_where` and
  `always_filter` apply inside it. Check that those conditions also reach the new dimensions
  (see [explore.md](explore.md#sql_always_where)).
- **Field picker.** The fields move from the NDT view to `E`.
- **Computed per query.** The dimension is computed in every query that uses it, where Looker could persist the
  NDT.

### As a window dimension

Looker: [`derived_column`](https://docs.cloud.google.com/looker/docs/reference/param-view-explore-source) ·
Holistics: [Window functions](https://docs.holistics.io/reference/aql/window-function),
[`rank`](https://docs.holistics.io/reference/aql/rank),
[`previous`](https://docs.holistics.io/reference/aql/previous)

A `derived_column` is SQL over the column names of the NDT. Convert by what it computes:

| `derived_column` | Conversion |
|---|---|
| a scalar expression over columns (`lifetime_revenue / lifetime_orders`) | a Dataset dimension over the converted columns (`safe_divide(E.a, E.b)`), or an expression in the outer `SELECT` of a SQL query model |
| a window function, and all columns of the NDT are dimensions of one view `V`, with its primary key | an AQL dimension on model `V` (table below). No model for the NDT, no join. |
| a window function over a measure column (`RANK() OVER (ORDER BY total_amount DESC)`) | `rank(order: <measure> \| desc()) \| dimensionalize(<key>)` when the NDT fits [As Dataset dimensions](#as-dataset-dimensions); otherwise a SQL query model |
| anything else (frames, `NTILE`, `FIRST_VALUE`, dialect functions) | a SQL query model, with the SQL text kept |

| SQL | AQL dimension on `V` |
|---|---|
| `ROW_NUMBER() OVER (PARTITION BY p ORDER BY o)` | `rank(order: V.o, partition: V.p)` |
| `RANK() OVER (PARTITION BY p ORDER BY o DESC)` | `rank(order: V.o \| desc(), partition: V.p)` |
| `DENSE_RANK() OVER (…)` | `dense_rank(order: …, partition: …)` |
| `LAG(x, n) OVER (PARTITION BY p ORDER BY o)` | `previous(V.x, n, order: V.o, partition: V.p)` |
| `LEAD(x, n) OVER (PARTITION BY p ORDER BY o)` | `next(V.x, n, order: V.o, partition: V.p)` |

The names in the SQL are NDT column names. Map each one back to the explore field it was built from.

```lookml
view: order_sequence {
  derived_table: {
    explore_source: orders {
      column: order_id { field: orders.id }
      column: user_id {}
      column: created_raw {}
      derived_column: user_order_sequence {
        sql: ROW_NUMBER() OVER (PARTITION BY user_id ORDER BY created_raw) ;;
      }
    }
  }

  dimension: order_id { primary_key: yes  hidden: yes  type: number }
  dimension: user_order_sequence { type: number }
}
```

Add to `Model orders`:

```aml
dimension user_order_sequence {
  label: 'User Order Sequence'
  type: 'number'
  definition: @aql rank(order: orders.created, partition: orders.user_id);;
}
```

Differences:

- AQL has no `row_number`. `rank()` gives tied rows the same number (1, 1, 3); `ROW_NUMBER()` does not (1, 2, 3).
  The results are equal when the order is unique within the partition.
- A `RANK()` without `ORDER BY` relies on the `sorts` of the NDT. Use the first sort as the window order.
- The NDT computed the window over the rows of the explore. The AQL dimension computes it over the rows of the
  model. They differ if the explore's joins or `sql_always_where` change the set of rows.

### As a SQL query model

Looker: [`explore_source`](https://docs.cloud.google.com/looker/docs/reference/param-view-explore-source) ·
Holistics: [Field selection](https://docs.holistics.io/reference/aml/query-model#note-on-field-selection-syntax)

The fallback for every NDT. Write the SQL that Looker would generate for the NDT, against the converted models,
then continue as for a SQL derived table.

```sql
SELECT q.*, <derived_column sql> AS <derived column>          -- only with derived_columns
FROM (
  SELECT <dimension> AS <column>, <AGG(operand)> AS <column>
  FROM {{ #<base model> AS b }}
  LEFT JOIN {{ #<joined model> AS j }} ON <join condition>     -- only the joins that the columns and filters need
  WHERE <filters> AND <expression_custom_filter> AND <the explore's sql_always_where>
  GROUP BY <dimension positions>
  ORDER BY <sorts> LIMIT <limit>
) q
```

| NDT part | In the SQL |
|---|---|
| a dimension `column` | `{{ #b.<dimension> }}`, so that the dimension's own SQL is reused |
| a timeframe `column` (`created_month`) | the dialect's truncation of the base dimension: `DATE_TRUNC('month', {{ #b.created }})` |
| a measure `column` | a SQL aggregate over the measure's operand, by its Looker type: `count` → `COUNT({{ #b.<key> }})`; `sum` → `COALESCE(SUM(…), 0)`; `average` → `AVG(…)`; `count_distinct` → `COUNT(DISTINCT …)`. Measure `filters` → `CASE WHEN <condition> THEN <operand> END` inside the aggregate. |
| `filters` | SQL conditions. [filter-expressions.md](filter-expressions.md) gives the meaning of each expression; write it in the SQL of the database, not in AQL. |
| `expression_custom_filter` | a Looker expression (`${orders.status} = "pending"`, `diff_days(…)`). Translate it to a SQL condition by hand. |
| `sorts`, `limit` | `ORDER BY … LIMIT n` of the inner query. Drop a sort without a limit, unless a `derived_column` window relies on it. |
| `derived_column` | the outer `SELECT`, SQL text unchanged |
| `timezone` | Unsupported as a property. Convert time columns in the SQL by hand. `"query_timezone"` has no equivalent. |

Then `models:` lists the models used, and the model gets one dimension per column: `{{ #SOURCE.<column> }}`.

If a Holistics workspace is available, the SQL can be obtained instead of written: build the same query on the
converted dataset, copy the executed SQL, and paste it into the query model.

```lookml
view: monthly_status_stats {
  derived_table: {
    explore_source: orders {
      column: created_month {}
      column: status {}
      column: order_count { field: orders.count }
      column: total_amount {}
      expression_custom_filter: ${orders.amount} > 0 ;;
    }
  }

  dimension: created_month { type: date_month }
  dimension: status {}
  dimension: order_count { type: number }
  dimension: total_amount { type: number }
  measure: average_monthly_amount { type: average  sql: ${total_amount} ;; }
}
```
```aml
// Copies the logic of the measures orders.count and orders.total_amount.
Model monthly_status_stats {
  type: 'query'
  label: 'Monthly Status Stats'
  data_source_name: 'demodb'
  models: [orders]

  query: @sql
    SELECT
      DATE_TRUNC('month', {{ #o.created }}) AS created_month,
      {{ #o.status }}                       AS status,
      COUNT({{ #o.id }})                    AS order_count,
      COALESCE(SUM({{ #o.amount }}), 0)     AS total_amount
    FROM {{ #orders AS o }}
    WHERE {{ #o.amount }} > 0
    GROUP BY 1, 2
  ;;

  dimension created_month {
    label: 'Created Month'
    type: 'date'
    definition: @sql {{ #SOURCE.created_month }};;
  }
  dimension status {
    label: 'Status'
    type: 'text'
    definition: @sql {{ #SOURCE.status }};;
  }
  dimension order_count {
    label: 'Order Count'
    type: 'number'
    definition: @sql {{ #SOURCE.order_count }};;
  }
  dimension total_amount {
    label: 'Total Amount'
    type: 'number'
    definition: @sql {{ #SOURCE.total_amount }};;
  }
  measure average_monthly_amount {
    label: 'Average Monthly Amount'
    type: 'number'
    definition: @aql average(monthly_status_stats.total_amount);;
  }
}
```

Differences:

- **The SQL is a copy of the measure logic.** When a measure changes, a Looker NDT follows; this model does not.
  Name the source measures in a comment above the model.
- **Timeframe columns.** Looker gives the formatted value of the timeframe (`month` → the text `2014-09`).
  `DATE_TRUNC` gives a date. Check the dimensions and joins that use the column.
- **Fan-out.** A hand-written `SUM` over a join is not protected as the Looker measure was. Keep the inner query to
  the base model and many-to-one joins; otherwise aggregate each model in its own CTE first.
- **Measures of other types** (distinct-key types, percentiles, measures on the "one" side of a join): write the
  SQL by hand and report it.

### `bind_filters`

Looker: [`bind_filters`](https://docs.cloud.google.com/looker/docs/reference/param-view-explore-source#using_bind_filters) ·
Holistics: [Parameter Fields](https://docs.holistics.io/docs/modeling/param-fields),
[Dynamic Query Model](https://docs.holistics.io/docs/query-parameters)

`bind_filters { from_field: F  to_field: T }` applies the filter that the user puts on `F` to `T` inside the NDT
query. It behaves like `{% condition F %} T {% endcondition %}` (see [filter.md](filter.md)).

1. Declare a `param` for `F`, with the type of `F`, on the model that receives the NDT's fields. `F` is normally a
   `filter` field.
2. If `F` is an ordinary dimension, the binding cannot be kept: users must filter the param, not the dimension.
   Report it.
3. Add the condition, in one of two forms:
   - **AQL form**, when the NDT otherwise fits [As Dataset dimensions](#as-dataset-dimensions): add
     `| where(<T> matches (<model>.<param> | first()))` before `dimensionalize` for a date param, or
     `| where(<T> in <model>.<param>)` for a text or number param.
   - **SQL form**, when the NDT needs [a SQL query model](#as-a-sql-query-model): declare the param on the query
     model and write `WHERE {% filter(<param>) %} <T> {% end %}` in the inner query.
4. Several `bind_filters` → one param and one condition each.

```lookml
view: user_period_facts {
  derived_table: {
    explore_source: orders {
      column: user_id {}
      column: period_orders { field: orders.count }
      bind_filters: {
        from_field: user_period_facts.period
        to_field: orders.created_date
      }
    }
  }

  filter: period { type: date }
  dimension: user_id { primary_key: yes  hidden: yes  type: number }
  dimension: period_orders { type: number }
}
```

AQL form. Add to `Model users`:

```aml
param period {
  label: 'Period'
  type: 'date'
}
```

Add to the dataset:

```aml
dimension period_orders {
  model: users
  label: 'Period Orders'
  type: 'number'
  definition: @aql orders.count
    | where(orders.created matches (users.period | first()))
    | dimensionalize(users.id);;
}
```

SQL form of the same NDT:

```aml
Model user_period_facts {
  type: 'query'
  label: 'User Period Facts'
  data_source_name: 'demodb'
  models: [orders]

  param period {
    label: 'Period'
    type: 'date'
  }

  query: @sql
    SELECT
      {{ #o.user_id }}   AS user_id,
      COUNT({{ #o.id }}) AS period_orders
    FROM {{ #orders AS o }}
    WHERE {% filter(period) %} {{ #o.created }} {% end %}
    GROUP BY 1
  ;;

  dimension user_id {
    label: 'User Id'
    type: 'number'
    primary_key: true
    hidden: true
    definition: @sql {{ #SOURCE.user_id }};;
  }
  dimension period_orders {
    label: 'Period Orders'
    type: 'number'
    definition: @sql {{ #SOURCE.period_orders }};;
  }
}
```

Differences:

- **No value.** In Looker and in the SQL form, no filter means no condition. In the AQL form, the behavior of an
  empty param is not documented; give the dashboard filter a default.
- **Operators.** Looker and `{% filter() %}` pass the whole condition (ranges, lists, negations). The AQL form
  with `first()` uses one value.
- **Binding.** The param is a field of its own and must be mapped to a dashboard filter. In Looker the binding
  followed the filter field.

### `bind_all_filters`

Looker: [`bind_all_filters`](https://docs.cloud.google.com/looker/docs/reference/param-view-explore-source#using_bind_all_filters) ·
Holistics: [`rank`](https://docs.holistics.io/reference/aql/rank),
[`top`](https://docs.holistics.io/reference/aql/top),
[`of_all`](https://docs.holistics.io/reference/aql/of_all),
[Nested aggregation](https://docs.holistics.io/as-code/aql/learn/nested-aggregation)

Unsupported. `bind_all_filters: yes` recomputes the NDT for every report with all of the report's filters, and
gives the result as dimensions. A Holistics dimension is computed before the filters of the report. Only metrics
follow them.

Emit no model for the view, remove its joins, and list every field of the view in the conversion report. Propose
metrics for the intent:

| Intent of the NDT | Closest in Holistics | Lost |
|---|---|---|
| a rank that follows the filters ("top N"): a dimension column, a measure column and a `RANK()` derived column | a metric `rank(order: <measure> \| desc())`, and a condition `<dim> in top(N, <dim>, by: <measure>)` | the rank as a dimension to group or filter by |
| a share of the filtered total | `safe_divide(<measure>, <measure> \| of_all(<model>))`, or `percent_of_total()` (see [measure.md](measure.md#percent_of_total)) | nothing essential |
| an aggregate of an aggregate that must follow the filters (average of the totals per user) | a metric with nested aggregation: `E \| group(E.key) \| select(<measure>) \| average()` | the value per key as a dimension |

```lookml
view: country_rank {
  derived_table: {
    explore_source: orders {
      column: country { field: users.country }
      column: total_amount { field: orders.total_amount }
      derived_column: rank { sql: RANK() OVER (ORDER BY total_amount DESC) ;; }
      bind_all_filters: yes
    }
  }

  dimension: country { primary_key: yes  hidden: yes }
  dimension: rank { type: number }
}
```

Proposed metrics, in the dataset:

```aml
metric country_revenue_rank {
  label: 'Country Rank (by Total Amount)'
  type: 'number'
  definition: @aql rank(order: orders.total_amount | desc());;
}
metric total_amount_top_10_countries {
  label: 'Total Amount (Top 10 Countries)'
  type: 'number'
  definition: @aql orders.total_amount
    | where(users.country in top(10, users.country, by: orders.total_amount));;
}
```

Differences:

- `rank()` in a metric ranks the rows of the report. It equals the Looker rank only in a report grouped by
  exactly the NDT's dimension column.
- Whether `top()` inside `where()` follows the other filters of the report is not documented. Mark for review.
- A `type: inner` join to the NDT also limited the explore to the ranked keys. That is lost.

### `dev_filters`

Looker: [`dev_filters`](https://docs.cloud.google.com/looker/docs/reference/param-view-explore-source#creating_filters_for_development_mode)

Unsupported. anfra has no development copy of a derived table. Drop it.

---

## Persistence parameters

`datagroup_trigger`, `sql_trigger_value`, `interval_trigger`, `persist_for`, `increment_key`, `increment_offset`,
`materialized_view`, `publish_as_db_view`, `indexes`, `sortkeys`, `distribution`, `distribution_style`,
`partition_keys`, `cluster_keys`, `table_compression`, `table_format`, and `distkey` on the view

Looker: [Persistent derived tables](https://docs.cloud.google.com/looker/docs/derived-tables#persistent_derived_tables_pdts)

**Unsupported.** anfra does not persist query models and has no schedules. Drop these parameters and list them in
the conversion report, with the original LookML. Do not write a `persistence` block, a `Schedule` or a
`schedules.aml`: `anfra validate` accepts them, but anfra never builds the table.

- **Results do not change.** The query model's SQL runs as a CTE inside every query that uses it, as Looker does
  with a temporary derived table.
- **Queries can be slower,** and the database runs the SQL on every query. For a derived table that is expensive
  to compute, the user can build the table outside anfra (the tool that loads the warehouse, a scheduled SQL job)
  and replace the query model with a table model on it. Report each persisted derived table with its trigger, so
  that the user can decide.
- **`increment_key`:** also remove `{% incrementcondition %} … {% endincrementcondition %}` from the SQL, with the
  `WHERE` or `AND` that it leaves behind. The query then reads all rows.
- **`publish_as_db_view: yes`:** the database view that Looker kept is gone. Report that other tools that read it
  must be repointed.
- **`materialized_view: yes`:** either create the materialized view in the database and use a table model on it,
  or keep the SQL in the query model. Report the choice.

## `create_process`, `sql_create`

Looker: [`create_process`](https://docs.cloud.google.com/looker/docs/reference/param-view-create-process),
[`sql_create`](https://docs.cloud.google.com/looker/docs/reference/param-view-sql-create)

Both give Looker the statements that build the table `${SQL_TABLE_NAME}`. A query model needs only a `SELECT`, in
`query`:

| LookML statements | Conversion |
|---|---|
| `CREATE TABLE ${SQL_TABLE_NAME} AS <select>` | `query` = the `<select>` |
| `CREATE TABLE ${SQL_TABLE_NAME} (…)` + `INSERT INTO ${SQL_TABLE_NAME} <select>` | `query` = the `<select>`. The column types of the `CREATE TABLE` are lost; report them |
| more statements on the table (`CREATE INDEX`, `ANALYZE`, `GRANT`, `ALTER TABLE`) | drop them and report them |
| statements that create something else (a BigQuery ML model, a function, a staging table) | Unsupported. Build it in the database and use a table model on the result |

```lookml
view: order_totals {
  derived_table: {
    create_process: {
      sql_step:
        CREATE TABLE ${SQL_TABLE_NAME} AS
        SELECT user_id, SUM(amount) AS total_amount
        FROM public.orders
        GROUP BY 1 ;;
      sql_step: CREATE INDEX order_totals_user ON ${SQL_TABLE_NAME} (user_id) ;;
    }
    sql_trigger_value: SELECT CURRENT_DATE ;;
  }

  dimension: user_id { primary_key: yes  type: number  sql: ${TABLE}.user_id ;; }
  dimension: total_amount { type: number  sql: ${TABLE}.total_amount ;; }
}
```
```aml
Model order_totals {
  type: 'query'
  label: 'Order Totals'
  data_source_name: 'demodb'
  models: []

  query: @sql
    SELECT user_id, SUM(amount) AS total_amount
    FROM public.orders
    GROUP BY 1
  ;;

  dimension user_id {
    label: 'User Id'
    type: 'number'
    primary_key: true
    definition: @sql {{ #SOURCE.user_id }};;
  }
  dimension total_amount {
    label: 'Total Amount'
    type: 'number'
    definition: @sql {{ #SOURCE.total_amount }};;
  }
}
```

Report: the `CREATE INDEX` statement and `sql_trigger_value` were dropped.
