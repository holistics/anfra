# LookML `filter` field → AML `param`, dimension, or nothing

How to convert a LookML filter-only field (`filter:`) and the templated filters that read it. The sections follow
Looker's [`filter`](https://docs.cloud.google.com/looker/docs/reference/param-field-filter) page.

A filter field has no value of its own. What it becomes depends on what reads it. The most common case is a
templated filter in a derived table, which becomes a
[query parameter](https://docs.holistics.io/docs/query-parameters) of a query model:

```lookml
view: customer_facts {
  derived_table: {
    sql:
      SELECT customer_id, SUM(sale_price) AS lifetime_spend
      FROM public.orders
      WHERE {% condition order_region %} orders.region {% endcondition %}
      GROUP BY 1 ;;
  }

  filter: order_region { type: string }

  dimension: customer_id { primary_key: yes  type: number }
  dimension: lifetime_spend { type: number }
}
```
```aml
Model customer_facts {
  type: 'query'
  label: 'Customer Facts'
  data_source_name: 'demodb'
  models: []

  param order_region {
    label: 'Order Region'
    type: 'text'
  }

  query: @sql
    SELECT customer_id, SUM(sale_price) AS lifetime_spend
    FROM public.orders
    WHERE {% filter(order_region) %} orders.region {% end %}
    GROUP BY 1
  ;;

  dimension customer_id {
    label: 'Customer Id'
    type: 'number'
    primary_key: true
    definition: @sql {{ #SOURCE.customer_id }};;
  }
  dimension lifetime_spend {
    label: 'Lifetime Spend'
    type: 'number'
    definition: @sql {{ #SOURCE.lifetime_spend }};;
  }
}
```

## Quick reference

| LookML | AML | Section |
|---|---|---|
| `filter` | by its readers: `param`, a `truefalse` dimension, or nothing | [filter](#filter) |
| `type` | `type` of the param | [type](#type) |
| `sql` | a `truefalse` dimension, or nothing | [sql](#sql) |
| `default_value` | default of the dashboard filter | [default_value](#default_value) |
| `{% condition f %}` in a derived table | `{% filter(f) %} … {% end %}` | [In a derived table](#condition-in-a-derived-table) |
| `{% condition f %}` in a dimension or measure | AQL `m.dim in m.f` | [In a dimension or measure](#condition-in-a-dimension-or-measure) |
| `{% date_start f %}`, `{% date_end f %}` | `{% filter(f) %}`, or two date params | [date_start, date_end](#date_start-date_end) |
| period-over-period blocks | `relative_period()` measures | [Period-over-period blocks](#period-over-period-blocks) |
| `{% condition f %}` in `sql_on` | unsupported | [Conditional joins](#conditional-joins) |
| other subparameters (`label`, `description`, `hidden`, `suggest_*`, `convert_tz`, …) | see field.md | [field.md](field.md) |

---

## `filter`

Looker: [`filter`](https://docs.cloud.google.com/looker/docs/reference/param-field-filter),
[Templated filters](https://docs.cloud.google.com/looker/docs/templated-filters) ·
Holistics: [Parameter Fields](https://docs.holistics.io/docs/modeling/param-fields),
[Dynamic Query Model](https://docs.holistics.io/docs/query-parameters)

Never emit a filter field as a dimension or measure by default. Find everything that reads it, then decide:

| The filter is read by | It becomes |
|---|---|
| `{% condition f %}` in a derived table | `param f` on the query model — [below](#condition-in-a-derived-table) |
| `{% condition f %}` in a dimension or measure | `param f` on the model — [below](#condition-in-a-dimension-or-measure) |
| `{% date_start f %}` / `{% date_end f %}` | a `param`, two `date` params, or nothing — [below](#date_start-date_end) |
| its own `sql` only | a `truefalse` dimension, or nothing — see [`sql`](#sql) |
| nothing that is converted | nothing. Drop it and list it in the conversion report. |

When it becomes a param:

1. Keep the name. Declare the param on the model that reads it, also when the LookML declared the filter in another
   view. Report that the dashboard filter must be bound to this param.
2. `label`, `description`, `hidden` → the same properties (see [field.md](field.md)). Without a label, use the name
   in title case. `view_label`, `group_label`, `group_item_label` → drop.
3. `suggest_dimension`, `suggest_explore` → nothing in AML. Report the dimension, so that the dashboard filter lists
   its values. `suggestions: […]` → drop; use `allowed_values` only if the list is meant to be closed, because it
   restricts the input (see [parameter.md](parameter.md#allowed_value)).

Difference: a Looker filter carries a whole condition (operator, several values, relative dates). A param keeps
the whole condition only when it is read by `{% filter() %}` in a query model. Read in AQL, it gives only its values.

## `type`

Looker: [Dimension, filter, and parameter types](https://docs.cloud.google.com/looker/docs/reference/param-dimension-filter-parameter-types) ·
Holistics: [Parameter Fields](https://docs.holistics.io/docs/modeling/param-fields)

| LookML `type` | AML param `type` |
|---|---|
| `string`, or no `type` | `'text'` |
| `number` | `'number'` |
| `date`, `date_time` | `'datetime'` when the filtered expression is a timestamp; `'date'` when it is a date column (`datatype` tells which) |
| `yesno` | `'truefalse'` (most `yesno` filters have `sql`; see [`sql`](#sql)) |

```lookml
filter: event_name { type: string }
filter: received_range { type: date }
```
```aml
param event_name {
  label: 'Event Name'
  type: 'text'
}
param received_range {
  label: 'Received Range'
  type: 'datetime'
}
```

`convert_tz` has no counterpart on a param: the time-zone handling of the filtered column decides
(see [dimension-group.md](dimension-group.md)).

## `sql`

Looker: [Using the `sql` parameter with `filter`](https://docs.cloud.google.com/looker/docs/reference/param-field-filter#using_the_sql_parameter_with_filter)

The `sql` of a filter is a condition that Looker adds to the `WHERE` clause when the filter has a value. Convert by
its shape:

| `sql` of the filter | Conversion |
|---|---|
| a fixed condition without Liquid (usually `type: yesno`) | a `truefalse` dimension with that SQL. Users filter it with "is true". |
| only `{% condition f %} ${dim} {% endcondition %}` | nothing. Report "filter on `<dim>` instead". If `<dim>` is hidden, it must be made visible or users cannot filter on it. |
| `{% condition %}` inside other SQL (`EXISTS (SELECT …)`, `COALESCE(…)`) | `param f` plus a hidden `truefalse` dimension built per [In a dimension or measure](#condition-in-a-dimension-or-measure), if the SQL can be written in AQL. Otherwise unsupported; report it. |
| part of a period-over-period block | see [Period-over-period blocks](#period-over-period-blocks) |

```lookml
filter: latest_day_only {
  type: yesno
  sql: ${TABLE}.snapshot_date = (SELECT MAX(snapshot_date) FROM public.inventory) ;;
}
```
```aml
dimension latest_day_only {
  label: 'Latest Day Only'
  type: 'truefalse'
  definition: @sql {{ #SOURCE.snapshot_date }} = (SELECT MAX(snapshot_date) FROM public.inventory);;
}
```

Differences:

- The Looker docs say the condition applies "whenever the filter has a value"; they do not say that "No" negates
  it. The dimension is an ordinary yes/no field: "is false" selects the other rows.
- The dimension can be selected as a column. A filter field cannot.

## `default_value`

Looker: [`default_value`](https://docs.cloud.google.com/looker/docs/reference/param-field-default-value) ·
Holistics: [AML FilterBlock](https://docs.holistics.io/reference/aml/filter-block)

Unsupported on the param, as for parameters (see [parameter.md](parameter.md#default_value)). One thing differs:
the default of a filter is a **filter expression** (`"7 days"`, `"-Returned"`, `">100"`), not a plain value.
Translate it with [filter-expressions.md](filter-expressions.md) and report it as the default condition of the
dashboard filter. Append `Looker default: <expression>` to the `description`.

```lookml
filter: received_range {
  type: date
  default_value: "7 days"
}
```
```aml
param received_range {
  label: 'Received Range'
  type: 'datetime'
  description: 'Looker default: 7 days'
}
```

A param read by `{% filter() %}` in a query model works without a value (no restriction). A param read in AQL does
not: there the dashboard default is required.

---

## Reading the filter

Looker: [Templated filters](https://docs.cloud.google.com/looker/docs/templated-filters),
[Liquid variable reference](https://docs.cloud.google.com/looker/docs/liquid-variable-reference)

### `condition` in a derived table

Holistics: [Dynamic Query Model](https://docs.holistics.io/docs/query-parameters)

Applies to `derived_table.sql` and to a subquery in `sql_table_name`, which both become a query model
(see [derived-tables.md](derived-tables.md)).

1. `{% condition f %} EXPR {% endcondition %}` → `{% filter(f) %} EXPR {% end %}`. `EXPR` stays as it is.
2. Declare `param f` on that query model.
3. If `f` is a **dimension**, not a filter field: declare a param with the dimension's name and type. Report that a
   dashboard filter on the dimension must also be mapped to the param.
4. Persistence settings are dropped like on any derived table
   (see [derived-tables.md](derived-tables.md#persistence-parameters)).

The headline example at the top of this page shows the result. Several filters work the same way:

```lookml
derived_table: {
  sql:
    SELECT session_id,
           MIN(CASE WHEN {% condition first_event %} event {% endcondition %} THEN received_at END) AS first_event_at
    FROM public.tracks
    WHERE {% condition received_range %} received_at {% endcondition %}
    GROUP BY 1 ;;
}

filter: first_event { type: string }
filter: received_range { type: date }
```
```aml
Model session_funnel {
  type: 'query'
  label: 'Session Funnel'
  data_source_name: 'demodb'
  models: []

  param first_event {
    label: 'First Event'
    type: 'text'
  }
  param received_range {
    label: 'Received Range'
    type: 'datetime'
  }

  query: @sql
    SELECT session_id,
           MIN(CASE WHEN {% filter(first_event) %} event {% end %} THEN received_at END) AS first_event_at
    FROM public.tracks
    WHERE {% filter(received_range) %} received_at {% end %}
    GROUP BY 1
  ;;

  dimension session_id {
    label: 'Session Id'
    type: 'text'
    primary_key: true
    definition: @sql {{ #SOURCE.session_id }};;
  }
  dimension first_event_at {
    label: 'First Event At'
    type: 'datetime'
    definition: @sql {{ #SOURCE.first_event_at }};;
  }
}
```

The validator does not read the SQL text, so it does not check the `{% filter() %}` tags.

Differences:

- Without a value, both tools render `1=1`.
- Both apply the user's operator to `EXPR`, but each tool writes its own SQL per operator. Case sensitivity of
  "contains", NULLs under a negation and the end of a date range can differ (see
  [filter-expressions.md](filter-expressions.md)).
- Mark for review: the Holistics docs show `{% filter() %}` in a `WHERE` clause only. Its use inside a
  `CASE WHEN`, as above, is not documented.

### `condition` in a dimension or measure

Holistics: [Parameter Fields](https://docs.holistics.io/docs/modeling/param-fields)

`{% filter() %}` works only in the `query` of a query model. In the `sql` of a dimension or measure, rebuild the
condition in AQL from the values of the param:

| Filter type | `{% condition f %} ${dim} {% endcondition %}` becomes |
|---|---|
| string, number, no type | `m.dim in m.f` |
| date, date_time | `m.dim matches first(m.f)` (a range stays a range) |
| yesno | `m.dim == first(m.f)` |

Then convert the SQL around it to AQL:

| LookML `sql` | AQL |
|---|---|
| `CASE WHEN {% condition f %} ${name} {% endcondition %} THEN ${name} ELSE 'Other' END` | `case(when: m.name in m.f, then: m.name, else: 'Other')` |
| a `yesno` dimension: `{% condition f %} ${created_raw} {% endcondition %}` | `m.created_at matches first(m.f)` |
| `COUNT(DISTINCT CASE WHEN {% condition f %} ${event} {% endcondition %} THEN ${session_id} END)` | `count_distinct(m.session_id) \| where(m.event in m.f)` |
| two conditions on one filter | `m.a in m.f or m.b in m.f` |

```lookml
filter: brand_select {
  type: string
  suggest_dimension: brand
}

dimension: brand_comparison {
  type: string
  sql:
    CASE WHEN {% condition brand_select %} ${brand} {% endcondition %} THEN ${brand}
         ELSE 'All Other Brands' END ;;
}

filter: event_select { type: string }

measure: sessions_with_event {
  type: count_distinct
  sql: CASE WHEN {% condition event_select %} ${event} {% endcondition %} THEN ${session_id} END ;;
}

filter: focus_range { type: date }

dimension: is_in_focus_range {
  type: yesno
  sql: {% condition focus_range %} ${received_raw} {% endcondition %} ;;
}
```
```aml
param brand_select {
  label: 'Brand Select'
  type: 'text'
}
dimension brand_comparison {
  label: 'Brand Comparison'
  type: 'text'
  definition: @aql case(
    when: tracks.brand in tracks.brand_select, then: tracks.brand,
    else: 'All Other Brands'
  );;
}

param event_select {
  label: 'Event Select'
  type: 'text'
}
measure sessions_with_event {
  label: 'Sessions With Event'
  type: 'number'
  definition: @aql count_distinct(tracks.session_id) | where(tracks.event in tracks.event_select);;
}

param focus_range {
  label: 'Focus Range'
  type: 'datetime'
}
dimension is_in_focus_range {
  label: 'Is In Focus Range'
  type: 'truefalse'
  definition: @aql tracks.received_at matches first(tracks.focus_range);;
}
```

Mark every such field for review. The conversion changes behavior:

- **Operators are lost** for strings and numbers. `in` tests membership only: "contains", "is not", ">" no longer
  work on this filter.
- **No value is an error.** Looker renders `1=1` (every row matches). A report that reads an unset param in AQL
  fails, so the dashboard filter needs a default, and "no value = everything" cannot be reproduced.

If users rely on those operators, the alternative is to move the logic into a query model, where
[`{% filter() %}`](#condition-in-a-derived-table) keeps them.

In `sql_always_where`: when `EXPR` is a dimension, no param is needed; users filter on the dimension itself. See
[explore.md](explore.md#sql_always_where).

### `date_start`, `date_end`

Looker: [`date_start` and `date_end`](https://docs.cloud.google.com/looker/docs/liquid-variable-reference#usage_of_date_start_and_date_end) ·
Holistics: [Dynamic Query Model](https://docs.holistics.io/docs/query-parameters)

`{% date_start f %}` and `{% date_end f %}` give the two ends of date filter `f`. Holistics has no expression for
the ends of a filter. Convert by what the SQL does with them:

**The ends only compare a column** (`col >= {% date_start f %} AND col < {% date_end f %}`). This is a templated
filter written by hand.

- In a derived table → `{% filter(f) %} col {% end %}`, with a `datetime` or `date` param `f`.
- In `sql_always_where` → an ordinary date filter on the dimension; no field is emitted.
- `COALESCE({% date_start f %}, <default>)` → the default goes to the dashboard filter.

```lookml
derived_table: {
  sql:
    SELECT language, SUM(requests) AS requests
    FROM public.page_views
    WHERE day >= {% date_start date_filter %} AND day < {% date_end date_filter %}
    GROUP BY 1 ;;
}

filter: date_filter { type: date }
```
```aml
Model page_view_totals {
  type: 'query'
  label: 'Page View Totals'
  data_source_name: 'demodb'
  models: []

  param date_filter {
    label: 'Date Filter'
    type: 'date'
  }

  query: @sql
    SELECT language, SUM(requests) AS requests
    FROM public.page_views
    WHERE {% filter(date_filter) %} day {% end %}
    GROUP BY 1
  ;;

  dimension language {
    label: 'Language'
    type: 'text'
    primary_key: true
    definition: @sql {{ #SOURCE.language }};;
  }
  dimension requests {
    label: 'Requests'
    type: 'number'
    definition: @sql {{ #SOURCE.requests }};;
  }
}
```

**The ends are used as values** (number of days in the range, a shifted range, an end shown as a column). Replace
the filter with two `date` params, `f_start` and `f_end`. Read them with `{{ f_start }}` in a query model, or with
`first(m.f_start)` in AQL.

```lookml
filter: date_filter { type: date }

dimension: days_in_range {
  type: number
  sql: DATEDIFF(day, {% date_start date_filter %}, {% date_end date_filter %}) ;;
}
```
```aml
param date_filter_start {
  label: 'Date Filter (start)'
  type: 'date'
}
param date_filter_end {
  label: 'Date Filter (end, exclusive)'
  type: 'date'
}
dimension days_in_range {
  label: 'Days In Range'
  type: 'number'
  definition: @aql date_diff('day', first(tracks.date_filter_start), first(tracks.date_filter_end));;
}
```

Mark for review. Differences of the two-param form:

- The user gets two date pickers and cannot choose a relative range ("last 7 days"). Report it.
- Both params need a default on the dashboard. Looker yields NULL for a missing end, which is why the LookML often
  wraps the tags in `COALESCE`.
- Looker's ends are timestamps and the end is exclusive. Check each use for whether the last day is included.

`{% if f._is_filtered %} … {% endif %}` around these tags only guards the unset case. Convert the guarded branch
and drop the guard.

### Period-over-period blocks

Holistics: [`relative_period`](https://docs.holistics.io/reference/aql/relative_period),
[Period-over-period Comparison](https://docs.holistics.io/docs/period-comparison)

A common LookML block compares a period with the previous one by hand. Recognise it by its fields: a date filter
(`current_date_range`), a parameter (`compare_to`), dimensions such as `days_in_period`, `period_2_start`,
`period_2_end`, `period`, `day_in_period`, `order_for_period`, and `date_start` arithmetic with
`INTERVAL … {% parameter compare_to %}`.

Do not convert these fields one by one. Replace the block:

1. The date filter → no field. Users filter on the date dimension.
2. For each compared measure, add a companion measure with `relative_period()`.
3. `parameter: compare_to` → keep it as a `param` and choose the interval with `case()`
   (see [parameter.md](parameter.md#choosing-a-field-or-expression)).
4. The helper dimensions, and a `sql_always_where` or filter `sql` that restricts the explore to the two periods →
   drop.

```lookml
filter: current_date_range { type: date }

parameter: compare_to {
  type: unquoted
  allowed_value: { value: "Month" }
  allowed_value: { value: "Year" }
}

# … days_in_period, period_2_start, period_2_end, period, order_for_period …

measure: total_sale_price {
  type: sum
  sql: ${sale_price} ;;
}
```
```aml
param compare_to {
  label: 'Compare To'
  type: 'text'
  allowed_values: ['Month', 'Year']
}
measure total_sale_price {
  label: 'Total Sale Price'
  type: 'number'
  definition: @aql sum(order_items.sale_price);;
}
measure total_sale_price_previous {
  label: 'Total Sale Price (previous period)'
  type: 'number'
  definition: @aql case(
    when: 'Month' in order_items.compare_to,
      then: order_items.total_sale_price | relative_period(order_items.created_at, interval(-1 month)),
    when: 'Year' in order_items.compare_to,
      then: order_items.total_sale_price | relative_period(order_items.created_at, interval(-1 year))
  );;
}
```

Mark for review. Differences:

- Looker shows one measure pivoted by a period label. Holistics shows two measures; reports built on the pivot must
  be rebuilt.
- "Previous period of the same length as the filter" and a custom second range have no model expression. Leave them
  to the dashboard's period comparison and report them.
- `relative_period` needs the date dimension in the report, or a filter on it.

### Conditional joins

Looker: [`sql_on`, conditional joins](https://docs.cloud.google.com/looker/docs/reference/param-explore-join-sql-on#conditional_joins)

Unsupported. A relationship cannot depend on a param. Keep the key part of the `sql_on` as the relationship, and
report the `{% condition %}` part. See [explore.md](explore.md#sql_on).

### Unsupported uses

| Use | What to do |
|---|---|
| `{% condition f %}` in `sql_on` | Unsupported. See [Conditional joins](#conditional-joins). |
| A filter `sql` whose SQL cannot be written in AQL and contains `{% condition %}` | Unsupported. Report it. |
| Arithmetic on the ends of one range filter, where two date pickers are not acceptable | Unsupported. Report it. |
| `{% table_date_range prefix f %}` (BigQuery legacy SQL) | Unsupported. Filter the parsed table suffix with `{% filter(f) %}` and check partition pruning by hand. |
| `_filters['view.field']`, a filter read in `html`, `link`, `label` | Unsupported. See [liquid.md](liquid.md). |
