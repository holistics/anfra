# LookML `aggregate_table` → AML `PreAggregate`

> **Not in use.** anfra does not run pre-aggregates or their schedules yet, so the conversion drops every
> `aggregate_table` (see [references/aggregate-awareness.md](../references/aggregate-awareness.md)). This page is
> the conversion for Holistics, kept for when anfra supports it.

How to convert the aggregate tables of a LookML explore. The sections follow the parameters of Looker's
[`aggregate_table`](https://docs.cloud.google.com/looker/docs/reference/param-explore-aggregate-table) page.

An aggregate table becomes one [PreAggregate](https://docs.holistics.io/reference/aml/pre-aggregate) of the
dataset that the explore converts to, plus a `PreAggregateSchedule` that refreshes it.

```lookml
view: orders {
  sql_table_name: shop.orders ;;
  dimension: id { primary_key: yes  type: number  sql: ${TABLE}.id ;; }
  dimension: amount { type: number  sql: ${TABLE}.amount ;; }
  dimension_group: created { type: time  timeframes: [raw, date, month, year]  sql: ${TABLE}.created_at ;; }

  measure: count { type: count }
  measure: total_amount { type: sum  sql: ${amount} ;; }
}

explore: orders {
  join: users { relationship: many_to_one  sql_on: ${orders.user_id} = ${users.id} ;; }

  aggregate_table: rollup__monthly_revenue {
    query: {
      dimensions: [orders.created_month]
      measures: [orders.total_amount, orders.count]
    }
    materialization: {
      sql_trigger_value: SELECT CURRENT_DATE() ;;
    }
  }
}
```

The view converts to a model with a `created` datetime dimension and the measures `count(orders.id)` and
`sum(orders.amount)` (see [dimension-group.md](../references/dimension-group.md), [measure.md](../references/measure.md)). The aggregate table
goes into the dataset:

```aml
Dataset orders_dataset {
  label: 'Orders'
  data_source_name: 'demodb'
  models: [orders, users]
  relationships: [
    relationship(orders.user_id > users.id, true)
  ]

  pre_aggregates: {
    rollup__monthly_revenue: PreAggregate {
      dimension orders__created_month {
        for: r(orders.created)
        time_granularity: 'month'
      }
      measure orders__total_amount {
        for: r(orders.amount)
        aggregation_type: 'sum'
      }
      measure orders__count {
        for: r(orders.id)
        aggregation_type: 'count'
      }
      persistence: FullPersistence {
        schema: 'persisted'
      }
    }
  }
}
```

And in `schedules.aml`:

```aml
const schedules = [
  // Looker: sql_trigger_value: SELECT CURRENT_DATE()
  PreAggregateSchedule {
    cron: '0 0 * * *'
    object: orders_dataset
    pre_aggregates: ['rollup__monthly_revenue']
  }
]
```

Aggregate Awareness in Holistics is in closed beta
(see [FAQs](https://docs.holistics.io/docs/aggregate-awareness/faqs)). Say so in the conversion report.

**The validator checks little here.** `anfra validate` checks the structure of a `PreAggregate` and the
value of `aggregation_type` and `time_granularity`. It does not check what `for:` points at. A pre-aggregate that
validates can still fail or stay unused at run time.

## The main difference: a pre-aggregate is a column mapping, not a query

Read this before converting; it explains most of the rules below.

| | Looker `aggregate_table` | Holistics `PreAggregate` |
|---|---|---|
| Declared as | a saved query: dimensions, measures, filters, time zone | a list of columns. Each column is `for:` a model dimension; a measure column also has an `aggregation_type` |
| Measures | named by the LookML measure (`orders.total_amount`) | named by the dimension that the measure aggregates, and the aggregation (`orders.amount`, `'sum'`). An AQL measure cannot be put in a pre-aggregate |
| Filters | part of the query. The table holds only the filtered rows | none. A pre-aggregate always holds all rows |
| Time zone | per table | always the organization's time zone |
| Refresh | datagroup, SQL trigger, or `persist_for` | a cron schedule |
| Which table is used | the best fit | the first one in the dataset that can answer the query |
| Any measure type | yes, when the explore query is exactly the table's query | no. Only the listed aggregation types, and non-additive ones only at exactly the pre-aggregate's dimensions |

Holistics: [Aggregate Awareness](https://docs.holistics.io/docs/aggregate-awareness),
[Quick Start](https://docs.holistics.io/docs/aggregate-awareness/quick-start)

## Quick reference

| LookML | AML | Section |
|---|---|---|
| `aggregate_table` | one entry of the dataset's `pre_aggregates` | [aggregate_table](#aggregate_table) |
| `query` | the dimensions and measures of the `PreAggregate` | [query](#query) |
| `dimensions` | `dimension <name> { for: r(model.field) }`, with `time_granularity` for a timeframe | [dimensions](#dimensions) |
| `measures` | `measure <name> { for: r(model.dimension)  aggregation_type: '…' }` | [measures](#measures) |
| `filters` | unsupported; each filtered field becomes a dimension | [filters](#filters) |
| `timezone` | unsupported | [timezone](#timezone) |
| `sorts`, `limit`, `pivots` and other query parameters | dropped | [sorts and the rest](#sorts-and-the-rest) |
| `materialization` | `persistence` in the `PreAggregate` + a `PreAggregateSchedule` | [materialization](#materialization) |
| `datagroup_trigger`, `sql_trigger_value`, `persist_for` | `FullPersistence { }` + a cron line | [Persistence strategy](#persistence-strategy) |
| `increment_key` | `IncrementalPersistence { }` | [increment_key, increment_offset](#increment_key-increment_offset) |
| `increment_offset` | unsupported | [increment_key, increment_offset](#increment_key-increment_offset) |
| `publish_as_db_view` | `view_name` in the persistence block | [publish_as_db_view](#publish_as_db_view) |
| `cluster_keys`, `distribution`, `distribution_style`, `indexes`, `partition_keys`, `sortkeys` | `custom_ddl` in the persistence block | [Table optimizations](#table-optimizations) |

---

## `aggregate_table`

Looker: [`aggregate_table`](https://docs.cloud.google.com/looker/docs/reference/param-explore-aggregate-table),
[Aggregate awareness](https://docs.cloud.google.com/looker/docs/aggregate_awareness) ·
Holistics: [AML PreAggregate](https://docs.holistics.io/reference/aml/pre-aggregate),
[AML Dataset](https://docs.holistics.io/reference/aml/dataset)

1. Merge refinements first. Aggregate tables are often added in `explore: +name { … }` blocks
   (see [extends-and-refinements.md](../references/extends-and-refinements.md)).
2. Each `aggregate_table: <name>` → one entry `<name>: PreAggregate { … }` in the `pre_aggregates` block of the
   dataset converted from the explore (see [explore.md](../references/explore.md)). Keep the name.
3. Name the columns `<model>__<field>`: `orders__created_month`, `orders__total_amount`. A column name must be
   unique in its pre-aggregate. It is also the column name of the table that Holistics builds.
4. Convert the body by the sections below. A pre-aggregate must have a `persistence` block.
5. **Order the entries from coarse to fine.** Holistics uses the first pre-aggregate that can answer a query, so
   put the smallest table first: fewer dimensions before more; for the same dimensions, a coarser
   `time_granularity` before a finer one (`year`, `quarter`, `month`, `week`, `day`, `hour`, `minute`). Otherwise
   keep the LookML order.
6. **Remove duplicates.** Aggregate tables that differ only by their filters become identical after the
   [filters](#filters) are dropped. Emit one pre-aggregate for each distinct set of dimensions and measures. If
   one pre-aggregate is contained in another (a subset of its columns, at the same granularity), keep both and
   report the overlap.

Differences:

- **Selection.** Looker picks the best-fitting table. Holistics picks the first eligible one. Rule 5 is a guess
  at "smallest first", since row counts are not known at conversion time.
- **Exact match.** Looker serves any measure type from a table whose query equals the explore query. Holistics has
  no such match; see [measures](#measures).
- **Fresh data.** Looker can add rows that are newer than the table to the result. Holistics serves what the table
  holds until the next scheduled build.
- **Before the first build.** A pre-aggregate is not used until its persistence has run once.
- **Table identity.** Holistics ties the persisted table to the definitions of the mapped fields, the
  relationships and the data source. After any of them changes, the pre-aggregate is unused until the next build.
  See [How Holistics identifies tables](https://docs.holistics.io/docs/aggregate-awareness/persistence#how-holistics-identifies-tables).

## `query`

Looker: [`query`](https://docs.cloud.google.com/looker/docs/reference/param-explore-aggregate-table#query)

The block itself emits nothing. Convert its sub-parameters.

### `dimensions`

Looker: [Field factors, Timeframe factors](https://docs.cloud.google.com/looker/docs/aggregate_awareness#field_factors) ·
Holistics: [Dimension mapping](https://docs.holistics.io/reference/aml/pre-aggregate#dimension-mapping),
[Dimension Awareness](https://docs.holistics.io/docs/aggregate-awareness/dimension-awareness),
[Time-granularity Awareness](https://docs.holistics.io/docs/aggregate-awareness/time-granularity-awareness),
[Join Awareness](https://docs.holistics.io/docs/aggregate-awareness/join-awareness)

For each entry `view.field`. A name without a view belongs to the base view of the explore.

1. **Plain dimension** → `dimension <view>__<field> { for: r(<view>.<field>) }`. A dimension of a joined view
   maps in the same way.
2. **Timeframe of a dimension group** (`created_date`, `created_month`) → `for:` the base dimension of the group
   (`created`; see [dimension-group.md](../references/dimension-group.md)) and a `time_granularity` from the table below.
3. **Plain `type: date` dimension** → rule 1 with `time_granularity: 'day'`. Plain `type: date_time` → rule 1,
   no granularity.
4. Two timeframes of one group in the same aggregate table (`created_date` and `created_month`) → one dimension,
   at the finer granularity. The coarser one is computed from it at query time.

| Looker timeframe | `time_granularity` |
|---|---|
| `year`, `quarter`, `month`, `week`, `hour`, `minute` | the same word: `'year'`, `'quarter'`, `'month'`, `'week'`, `'hour'`, `'minute'` |
| `date` | `'day'` |
| `time`, `raw`, `second`, `millisecond`, `microsecond` | none: the column holds the full timestamp. Such a table is rarely smaller than the source; report it |
| `minuteN`, `hourN` (`minute15`, `hour6`) | `'minute'` / `'hour'`. Finer than in Looker: correct, but a larger table; report it |
| `fiscal_year`, `fiscal_quarter`, `fiscal_month_num`, … | none. Map the explicit dimension emitted for that timeframe, as a plain dimension (rule 1) |
| `day_of_week`, `hour_of_day`, `month_name`, `week_of_year`, … | none. Map the explicit dimension emitted for that timeframe, as a plain dimension (rule 1) |

`'second'` and `'date'` are not valid values of `time_granularity`.

`dimensions: [orders.created_date, orders.status, users.country]`:

```aml
dimension orders__created_date {
  for: r(orders.created)
  time_granularity: 'day'
}
dimension orders__status {
  for: r(orders.status)
}
dimension users__country {
  for: r(users.country)
}
```

Differences:

- **Date parts from a daily table.** Looker answers a query by day of month, day of year or week of year from a
  daily table. Holistics documents only "that granularity or coarser". A report by `created_day_of_week` may not
  be served by the `'day'` pre-aggregate unless that dimension is itself in the pre-aggregate. Mark for review.
- **Week start.** `week` starts on the model's `week_start_day` in Looker and on the organization's Week Start Day
  in Holistics (see [model.md](../references/model.md#week_start_day)).
- **Time zone.** See [timezone](#timezone).
- **Joined dimensions.** Holistics can also serve the dimensions of a joined model from a pre-aggregate that holds
  only the foreign key (`orders.user_id`). The conversion keeps the Looker layout; the user may prefer that design.

### `measures`

Looker: [Measure type factors](https://docs.cloud.google.com/looker/docs/aggregate_awareness#measure_type_factors) ·
Holistics: [Measure mapping](https://docs.holistics.io/reference/aml/pre-aggregate#measure-mapping),
[Metric Awareness](https://docs.holistics.io/docs/aggregate-awareness/metric-awareness),
[How can I use my existing AQL measure in PreAggregate measure?](https://docs.holistics.io/docs/aggregate-awareness/faqs)

A pre-aggregate measure is not the LookML measure. It is one pair of (dimension, aggregation). So first convert
each listed measure to AQL (see [measure.md](../references/measure.md)), then map the aggregates inside it:

- `for:` is the dimension inside the AQL aggregate. For `sum(orders.amount)` it is `r(orders.amount)`. If the
  conversion made a hidden helper dimension for the measure's `sql`, `for:` is that helper.
- Never point `for:` at a measure. It validates, and fails at run time.
- Emit each pair once per pre-aggregate, even when several measures need it.

| LookML measure (converted AQL) | Pre-aggregate measure | Served for coarser queries |
|---|---|---|
| `type: count` → `count(m.<pk>)` | `for: r(m.<pk>)`, `'count'` | yes |
| `type: sum` → `sum(m.x)` | `for: r(m.x)`, `'sum'` | yes |
| `type: min`, `max` | `for: r(m.x)`, `'min'` / `'max'` | yes |
| `type: average` → `average(m.x)` | two measures on `m.x`: `'sum'` and `'count'`. Reuse a `'sum'` that is already there; name the count `<model>__<measure>__count`. Do not emit `'avg'` | yes: Holistics combines the sum and the count |
| `type: count_distinct` → `count_distinct(m.x)` | `for: r(m.x)`, `'count distinct'` | no: only at exactly the dimensions and granularity of the pre-aggregate |
| `type: median` → `median(m.x)` | `for: r(m.x)`, `'median'` | no |
| `type: number` over other measures (`${a} / NULLIF(${b}, 0)`) | nothing of its own. Map the measures that it references | as its parts |
| `type: number`, `date` or `string` with one SQL aggregate (`MAX(…)`) | as the AQL aggregate that it converts to | as that aggregate |
| a measure with `filters` → `<aggregate> \| where(<condition>)` | the aggregate without the filter, and every field of the condition as a pre-aggregate [dimension](#dimensions) | not documented. Mark for review |
| `running_total`, `percent_of_total`, `percent_of_previous` | the measure that it wraps | yes |
| `sum_distinct`, `average_distinct`, `median_distinct`, `percentile_distinct` | if it converted to a plain aggregate (the key is the primary key): as that aggregate. Otherwise Unsupported | — |
| `count_distinct` with `approximate: yes` → `approx_count_distinct(m.x)` | Unsupported | — |
| `percentile`, `list` | Unsupported | — |
| a measure left in SQL (`definition: @sql …`) | Unsupported by default. `for: r(m.<measure>)`, `'custom'` works only when the model measure declares its own `aggregation_type`; see the FAQ above | — |

"Unsupported": omit the measure from the pre-aggregate and report it. Queries that use it run on the source tables.

Valid `aggregation_type` values: `count`, `count distinct`, `sum`, `avg`, `min`, `max`, `median`, `stdev`,
`stdevp`, `var`, `varp`, `custom`. **Write `'count distinct'` with a space.** The validator rejects
`'count_distinct'`.

If no listed measure gives a pre-aggregate measure, drop the pre-aggregate and report it. Keep a pre-aggregate
without measures only when the aggregate table had none (a list of distinct values).

`allow_approximate_optimization: yes` on a measure → drop it. Holistics does not build distinct counts from
sketches; a `'count distinct'` measure is served at the exact granularity only.

```lookml
measure: count { type: count }
measure: total_amount { type: sum  sql: ${amount} ;; }
measure: average_amount { type: average  sql: ${amount} ;; }
measure: max_amount { type: max  sql: ${amount} ;; }
measure: unique_users { type: count_distinct  sql: ${user_id} ;; }
measure: completed_count { type: count  filters: [status: "complete"] }
measure: average_order_value { type: number  sql: ${total_amount} / NULLIF(${count}, 0) ;; }
```

`measures: [orders.count, orders.total_amount, orders.average_amount, orders.max_amount, orders.unique_users,
orders.completed_count, orders.average_order_value]`:

```aml
measure orders__count {
  for: r(orders.id)
  aggregation_type: 'count'
}
measure orders__total_amount {
  for: r(orders.amount)
  aggregation_type: 'sum'
}
measure orders__average_amount__count {
  for: r(orders.amount)
  aggregation_type: 'count'
}
measure orders__max_amount {
  for: r(orders.amount)
  aggregation_type: 'max'
}
measure orders__unique_users {
  for: r(orders.user_id)
  aggregation_type: 'count distinct'
}
```

- `average_amount` is served from `orders__total_amount` and `orders__average_amount__count`.
- `average_order_value` is served from `orders__total_amount` and `orders__count`.
- `completed_count` needs `orders__count` and a dimension on `orders.status`.

Differences:

- **Non-additive measures.** In Looker, `count_distinct`, `median`, `percentile` and `list` are served only by a
  table that exactly matches the query. In Holistics, `'count distinct'` and `'median'` are served at exactly the
  pre-aggregate's dimensions; `percentile` and `list` are not served at all.
- **Derived measures.** Holistics serves more of them from one additive pre-aggregate: ratios, differences,
  running totals and period comparisons over `sum`, `count`, `min` and `max`.
- **Measures of a joined view.** How Holistics builds a pre-aggregate that mixes a measure of one model with
  dimensions of another is not documented. Mark for review.

### `filters`

Looker: [Filter factors](https://docs.cloud.google.com/looker/docs/aggregate_awareness#filter_factors)

**Unsupported.** A `PreAggregate` has no filter; it always covers all rows. For each `field: "expression"`:

1. Drop the condition.
2. Add the field as a [dimension](#dimensions) of the pre-aggregate if it is not one already. A date or timeframe
   field goes in at `'day'` granularity, or at the granularity of a dimension of the same group that is already
   there, if that one is finer. Without the column, a report that filters on the field cannot use the
   pre-aggregate.
3. An empty expression (`""`) → nothing.
4. A filter on a measure, or on a `filter` or `parameter` field → cannot be a dimension. Drop it and report it.
5. Report the dimensions that were added, per pre-aggregate.

`filters: [orders.created_date: "after 2019-01-01", orders.is_test: "No"]`, in a table that already has
`orders.created_date` as a dimension. Only `is_test` is added:

```aml
dimension orders__is_test {
  for: r(orders.is_test)
}
```

Differences:

- **Size.** The Looker table holds only the filtered rows. The pre-aggregate holds all rows, multiplied by the
  added dimensions. With many or high-cardinality added dimensions, it can approach the size of the source table.
  The user must decide whether to keep it. Aggregate tables that Looker generated from dashboard tiles (names like
  `rollup__created_date__0`) are usually of this kind: one filtered query each. A few general pre-aggregates,
  designed by hand, often serve better than a converted copy of each one.
- **Generality.** The pre-aggregate serves any filter value and any date range, not only the ones in the LookML.
- **Filter values.** They belong to the dashboard tiles and are applied there as report filters.
- **Access filters.** A query that has a row-level permission or a default filter on a field can use the
  pre-aggregate only if that field is a dimension of it. These fields are not added automatically; report them.

### `timezone`

Looker: [Time zone factors](https://docs.cloud.google.com/looker/docs/aggregate_awareness#time_zone_factors) ·
Holistics: [How could I handle timezone when querying with pre-aggregated tables?](https://docs.holistics.io/docs/aggregate-awareness/faqs),
[Timezone settings](https://docs.holistics.io/docs/datetimes/timezones)

**Unsupported.** Drop it and report it.

Pre-aggregates always use the organization's time zone. A query or dashboard in another time zone does not use
any pre-aggregate. Looker instead builds one table per time zone. If the LookML time zone is not the organization's
time zone, the day and month boundaries of the converted table differ from Looker's.

### `sorts` and the rest

`sorts`, `limit`, `pivots`, and the other query parameters that Looker writes when the LookML is copied from an
Explore or a dashboard → drop. The row order of a table has no meaning.

## `materialization`

Looker: [`materialization`](https://docs.cloud.google.com/looker/docs/reference/param-explore-aggregate-table#materialization) ·
Holistics: [Pre-aggregate Persistence](https://docs.holistics.io/docs/aggregate-awareness/persistence),
[AML Persistence](https://docs.holistics.io/reference/aml/persistence)

An aggregate table is a persistent derived table, and its `materialization` parameters are the derived-table ones.
Convert them as in [derived-table-persistence.md](derived-table-persistence.md), with the changes below.

### Persistence strategy

`datagroup_trigger`, `sql_trigger_value`, `persist_for`

Looker: [`datagroup_trigger`](https://docs.cloud.google.com/looker/docs/reference/param-explore-aggregate-table#datagroup_trigger),
[`sql_trigger_value`](https://docs.cloud.google.com/looker/docs/reference/param-explore-aggregate-table#sql_trigger_value),
[`persist_for`](https://docs.cloud.google.com/looker/docs/reference/param-explore-aggregate-table#persist_for) ·
Holistics: [Schedule for PreAggregate Persistence](https://docs.holistics.io/reference/aml/persistence#schedule-for-preaggregate-persistence)

1. `persistence: FullPersistence { schema: '<schema>' }` inside the `PreAggregate`. `schema` is required and is
   not in the LookML; ask the user for it, as for derived tables.
2. Turn the trigger into a cron line by the rules and the table in
   [derived-table-persistence.md, Persistence strategy](derived-table-persistence.md#persistence-strategy).
   Holistics has no trigger that reacts to data; a trigger that reads data needs a cron chosen by a person.
3. Emit a `PreAggregateSchedule { cron, object: <dataset>, pre_aggregates: ['<name>', …] }` in the same
   `const schedules = [ … ]` as the `Schedule` entries of query models. Put the pre-aggregates of one dataset that
   share a cron in one entry. Keep the Looker trigger as a comment.

```lookml
datagroup: ecommerce_etl {
  sql_trigger: SELECT MAX(completed_at) FROM etl.jobs ;;
  max_cache_age: "24 hours"
}

explore: orders {
  aggregate_table: rollup__monthly_revenue {
    query: { … }
    materialization: { sql_trigger_value: SELECT CURRENT_DATE() ;; }
  }
  aggregate_table: rollup__daily_by_status {
    query: { … }
    materialization: { datagroup_trigger: ecommerce_etl }
  }
}
```
```aml
const schedules = [
  // Looker: sql_trigger_value: SELECT CURRENT_DATE()
  PreAggregateSchedule {
    cron: '0 0 * * *'
    object: orders_dataset
    pre_aggregates: ['rollup__monthly_revenue']
  },
  // TODO: set the time. Looker: datagroup ecommerce_etl, sql_trigger: SELECT MAX(completed_at) FROM etl.jobs
  PreAggregateSchedule {
    cron: '0 5 * * *'
    object: orders_dataset
    pre_aggregates: ['rollup__daily_by_status']
  }
]
```

The differences listed in [derived-table-persistence.md](derived-table-persistence.md#persistence-strategy)
apply: a cron rebuilds on time, whether or not the data changed.

Not generated: `persistence: ExternalPersistence { table_name: '<schema>.<table>' }` maps a pre-aggregate to a
table that another tool builds. Looker always builds its aggregate tables itself. Mention it to a user who already
builds roll-up tables elsewhere. See
[External persistence](https://docs.holistics.io/docs/aggregate-awareness/persistence#external-persistence).

### `increment_key`, `increment_offset`

Looker: [`increment_key`](https://docs.cloud.google.com/looker/docs/reference/param-explore-aggregate-table#increment_key),
[`increment_offset`](https://docs.cloud.google.com/looker/docs/reference/param-explore-aggregate-table#increment_offset) ·
Holistics: [IncrementalPersistence](https://docs.holistics.io/docs/aggregate-awareness/persistence#incrementalpersistence)

1. `increment_key: "view.field"` → `persistence: IncrementalPersistence { … }` with `incremental_column` and
   `primary_key` both set to the **pre-aggregate column** of that field (`'orders__created_date'`). These names
   are columns of the pre-aggregate, not of the source model.
2. If the field is not among the dimensions, add it, at its granularity.
3. `increment_offset` → Unsupported. Drop it and report it.

```lookml
materialization: {
  datagroup_trigger: ecommerce_etl
  increment_key: "orders.created_date"
  increment_offset: 3
}
```
```aml
persistence: IncrementalPersistence {
  schema: 'persisted'
  incremental_column: 'orders__created_date'
  primary_key: 'orders__created_date'
}
```

Mark every incremental pre-aggregate for review:

- Looker rebuilds the latest period, and `increment_offset` more periods, on every run. Holistics "appends new
  records and updates existing ones" by `primary_key`. Whether the latest, unfinished period is computed again on
  each run is not stated. See the table in
  [derived-table-persistence.md](derived-table-persistence.md#increment_key-increment_offset).
- A pre-aggregate with several dimensions has many rows per date, so the date column is not unique. Whether
  Holistics then replaces all rows of that date is not stated.
- Late data in older periods, which `increment_offset` covered, is not picked up.

If the user cannot confirm the behaviour, emit `FullPersistence` and report the change.

### `publish_as_db_view`

Looker: [`publish_as_db_view`](https://docs.cloud.google.com/looker/docs/reference/param-explore-aggregate-table#publish_as_db_view)

`publish_as_db_view: yes` → `view_name: '<aggregate table name>'` in the persistence block. The name of the
database view differs from Looker's; see
[derived-table-persistence.md](derived-table-persistence.md#publish_as_db_view).

```aml
persistence: FullPersistence {
  schema: 'persisted'
  view_name: 'rollup__daily_by_status'
}
```

### Table optimizations

`cluster_keys`, `distribution`, `distribution_style`, `indexes`, `partition_keys`, `sortkeys`

Looker: [`materialization`](https://docs.cloud.google.com/looker/docs/reference/param-explore-aggregate-table#materialization)

→ `custom_ddl` in the persistence block, by the dialect table in
[derived-table-persistence.md, Table optimizations](derived-table-persistence.md#table-optimizations). Replace
each column name by the name of the pre-aggregate column (`<model>__<field>`).

BigQuery, with `partition_keys: ["created_date"]` and `cluster_keys: ["status"]`:

```aml
persistence: FullPersistence {
  schema: 'persisted'
  custom_ddl: @sql
    CREATE TABLE {{ persisted_table }}
    PARTITION BY orders__created_date
    CLUSTER BY orders__status
    AS {{ parsed_query }};
  ;;
}
```

Mark for review: the DDL text is not validated, and the type of a column with a `time_granularity` (date or
timestamp) is not documented. Run the DDL once on the target database. These parameters change performance only.

## Complete example

A daily table with a joined dimension, filters, non-additive measures and an incremental build. The view is the
one under [measures](#measures).

```lookml
aggregate_table: rollup__daily_by_status {
  query: {
    dimensions: [orders.created_date, orders.status, users.country]
    measures: [orders.count, orders.total_amount, orders.average_amount, orders.max_amount,
               orders.unique_users, orders.completed_count, orders.average_order_value]
    filters: [orders.created_date: "after 2019-01-01", orders.is_test: "No"]
    timezone: "America/Los_Angeles"
  }
  materialization: {
    datagroup_trigger: ecommerce_etl
    increment_key: "orders.created_date"
    increment_offset: 3
    publish_as_db_view: yes
  }
}
```
```aml
rollup__daily_by_status: PreAggregate {
  dimension orders__created_date {
    for: r(orders.created)
    time_granularity: 'day'
  }
  dimension orders__status {
    for: r(orders.status)
  }
  dimension users__country {
    for: r(users.country)
  }
  // Looker filter orders.is_test: "No" -> the field becomes a dimension
  dimension orders__is_test {
    for: r(orders.is_test)
  }

  measure orders__count {
    for: r(orders.id)
    aggregation_type: 'count'
  }
  measure orders__total_amount {
    for: r(orders.amount)
    aggregation_type: 'sum'
  }
  // average_amount = total_amount / average_amount__count
  measure orders__average_amount__count {
    for: r(orders.amount)
    aggregation_type: 'count'
  }
  measure orders__max_amount {
    for: r(orders.amount)
    aggregation_type: 'max'
  }
  // served at exactly this granularity only
  measure orders__unique_users {
    for: r(orders.user_id)
    aggregation_type: 'count distinct'
  }

  persistence: IncrementalPersistence {
    schema: 'persisted'
    view_name: 'rollup__daily_by_status'
    incremental_column: 'orders__created_date'
    primary_key: 'orders__created_date'
  }
}
```

In the dataset, this entry goes after `rollup__monthly_revenue`: it has more dimensions and a finer granularity.

Report for this table: the two filters and the time zone were dropped; `orders__is_test` was added;
`increment_offset: 3` was dropped; the incremental build and `completed_count` are marked for review; the cron of
`ecommerce_etl` must be set by a person.
