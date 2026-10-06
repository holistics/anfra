# AQL for Data App queries

A query's `aql` can declare ad hoc dimensions and metrics before its `explore { }` block. anfra
compiles it with amql, applying the reader's filters, cross-filters, sorts, date drills and paging
to the `explore { }` before it runs. Stay inside the shapes below. Dataset fields are
`model.field`, exactly as in `Anfra.datasets`; dataset metrics are bare names. Query-local
declarations introduce additional names for that query.

**Always write an `explore { }`**. Pipe-style queries (`orders | select(...)`) run, but controls,
cross-filters, `setSort` and date drills need an `explore` to apply to, so such a query fails as
soon as a reader touches one.

```
explore {
  dimensions {
    region: users.region,
    period: date_trunc(orders.created_at, 'month'),
  }
  measures {
    revenue: orders | sum(orders.amount),
    order_count: count(orders.id),
    avg_order: sum(orders.amount) / count(orders.id),
  }
  filters {
    orders.status == 'completed',
  }
  having {
    revenue > 1000,
  }
  sorts {
    revenue desc
  }
}
```

## Syntax rules

- **Commas between items**, in every section. A trailing comma is fine; a newline alone is a
  syntax error.
- Sections are `dimensions`, `measures`, `filters`, `having`, `sorts`. Each at most once. There
  is no `limit`: paging is the SDK's (`pageSize`, `fetchMore`), and anfra turns it into
  LIMIT/OFFSET. A query with no `pageSize` returns every row. anfra can't page a pivot
  (`rows { }` / `columns { }`), so a pivot query declares no `pageSize`.
- `filters` takes row-level conditions only. A condition on an aggregate, a dataset metric or a
  query-local metric goes in `having`.
- **Alias everything** (`region: …`). The alias is the row key in `query.result.rows` and what
  `sorts` and `setSort` name. Aliases match `[a-zA-Z_][a-zA-Z0-9_]*` and must be unique in the query.
- Strings take single or double quotes. Comments are `//` to end of line only.

## Dimensions

Select a field reference or a truncated date:

- a field: `status: orders.status`
- a truncated date: `date_trunc(orders.created_at, 'month')`, or the equivalent
  `orders.created_at | month()`. Grains: `year`, `quarter`, `month`, `week`, `day`, `hour`,
  `minute`, always quoted in `date_trunc`.

For a computed dimension, declare it before `explore`, ending the declaration with a semicolon,
then select its `model.name` reference:

```
dimension beer_orders.band = case(
  when: beer_orders.price > 50,
  then: 'premium',
  else: 'standard'
);

explore {
  dimensions {
    product: beer_orders.product,
    band: beer_orders.band,
  }
  measures {
    orders: count(beer_orders.i),
    avgRating: avg(beer_orders.rating),
  }
  sorts { orders desc }
}
```

The declaration is query-local; it does not add a field to `Anfra.datasets`. Query-local
computed dimensions are skipped when cross-filtering (see [API.md](API.md#cross-filtering)).

## Measures

- Aggregate a field: `sum(orders.amount)`, or with the model explicit, `orders | sum(orders.amount)`.
  Names: `sum`, `avg`, `min`, `max`, `median`, `count`, `count_distinct`, `stdev`, `var`.
  Count rows with `count(orders.id)`, never `count(orders)`.
- An existing model measure: `orders.total_revenue`. A dataset metric: its bare name, `revenue`.
- Arithmetic between measures: `sum(orders.amount) / count(orders.id)`. The pipe binds looser than
  `+ - * /`, so parenthesise before piping.
- Filtered: `count(orders.id) | where(orders.status == 'refunded')`
- Share of total: `sum(orders.amount) / (sum(orders.amount) | of_all(users.region))`
- Null to zero: `coalesce(sum(orders.amount), 0)`

### Ad hoc metrics

Declare a reusable aggregate expression with `metric name = ...;` before `explore`. Reference its
bare name in `measures` and `having`:

```
metric aov = sum(beer_orders.price) / count(beer_orders.i);

explore {
  dimensions {
    product: beer_orders.product,
  }
  measures {
    revenue: beer_orders | sum(beer_orders.price),
    aov: aov,
  }
  having {
    aov > 10,
  }
  sorts { revenue desc }
}
```

## Filters

Static conditions every reader gets. Anything a reader changes is a control (`createFilter` +
`mapControl`), never string-built into the AQL.

```
filters {
  orders.status == 'completed',
  users.country in ['VN', 'SG'],
  users.name ilike '%smith%',
  orders.discount_code is not null,
  orders.created_at matches @(last 90 days),
}
```

- Operators: `==` `!=` `is` `is not` `>` `<` `>=` `<=` `in` `not in` `like` `ilike` `not like`
  `not ilike` `matches`, joined with `and` / `or`. Negate with `!(…)`: prefix `not` does not parse.
- Dates: `@2026`, `@2026-01-01`, `@(last 7 days)`, `@(2026-01-01 - 2026-03-31)`, `@(last year)`.
- Conditions here are row-level. A fixed condition on an aggregate or a metric goes in `having`
  (`aov > 10` above, or `having { sum(orders.amount) > 1000 }`); `filters` rejects it. For a
  reader-adjustable aggregate threshold, use an aggregated control mapping (see
  [API.md](API.md#mappings-are-explicit)).

## Sorts

The query's default order. Name result aliases only (never `model.field`), each with `asc` or
`desc`. When the reader chooses the order (a clickable column header), use
`query.setSort([{ field: 'revenue', direction: 'desc' }])` instead: a non-empty `setSort` replaces
this section for that run, and `setSort([])` brings it back.
