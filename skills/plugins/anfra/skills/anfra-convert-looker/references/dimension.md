# LookML `dimension` → AML `dimension`

How to convert a LookML `dimension` and every dimension `type`. The sections follow Looker's
[`dimension`](https://docs.cloud.google.com/looker/docs/reference/param-field-dimension) page, then the
[Dimension, filter, and parameter types](https://docs.cloud.google.com/looker/docs/reference/param-dimension-filter-parameter-types)
page in its own order.

A dimension becomes one [AML dimension](https://docs.holistics.io/reference/aml/field) in the model of its view:

```lookml
view: users {
  dimension: id        { primary_key: yes  type: number  sql: ${TABLE}.id ;; }
  dimension: email     { }
  dimension: age       { type: number  sql: ${TABLE}.age ;; }
  dimension: full_name { type: string  sql: CONCAT(${first_name}, ' ', ${last_name}) ;; }
  dimension: is_adult  { type: yesno   sql: ${age} >= 18 ;; }
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
    primary_key: true
    definition: @sql {{ #SOURCE.id }};;
  }
  dimension email {
    label: 'Email'
    type: 'text'
    definition: @sql {{ #SOURCE.email }};;
  }
  dimension age {
    label: 'Age'
    type: 'number'
    definition: @sql {{ #SOURCE.age }};;
  }
  dimension first_name {
    label: 'First Name'
    type: 'text'
    definition: @sql {{ #SOURCE.first_name }};;
  }
  dimension last_name {
    label: 'Last Name'
    type: 'text'
    definition: @sql {{ #SOURCE.last_name }};;
  }
  dimension full_name {
    label: 'Full Name'
    type: 'text'
    definition: @sql CONCAT({{ first_name }}, ' ', {{ last_name }});;
  }
  dimension is_adult {
    label: 'Is Adult'
    type: 'truefalse'
    definition: @sql CASE WHEN ({{ age }} >= 18) THEN TRUE ELSE FALSE END;;
  }
}
```

An AML dimension accepts only `label`, `type`, `definition`, `description`, `hidden`, `format` and `primary_key`.
`label` is required. `type` is one of `'text'`, `'number'`, `'date'`, `'datetime'`, `'truefalse'`. Any other property
is a compile error, so a LookML parameter without a counterpart is dropped, never copied.

## Quick reference

| LookML | AML | Section |
|---|---|---|
| `dimension` | model `dimension` | [dimension](#dimension) |
| `sql` | `definition: @sql …;;` | [sql](#sql) |
| `primary_key` | `primary_key: true` | [primary_key](#primary_key) |
| `type` | `type` + a definition shape | [type](#type) |
| `type: bin`, `bins` | same as `tier` | [bin](#bin) |
| `type: distance`, `start_location_field`, `end_location_field`, `units` | `'number'`, distance formula in AQL | [distance](#distance) |
| `type: custom_calendar`, `duration`, `time` | dimension-group types | [dimension-group.md](dimension-group.md) |
| `type: location`, `sql_latitude`, `sql_longitude` | two `'number'` dimensions | [location](#location) |
| `type: number` | `'number'` | [number](#number) |
| `type: string`, no `type` | `'text'` | [string](#string) |
| `type: tier`, `tiers`, `style` | `'text'`, AQL `case()` | [tier](#tier) |
| `type: unquoted` | parameter-only type | [parameter.md](parameter.md) |
| `type: yesno` | `'truefalse'`, NULL-safe definition | [yesno](#yesno) |
| `type: zipcode` | `'text'` | [zipcode](#zipcode) |
| `type: date`, `date_time`, `date_raw` | `'date'` / `'datetime'` | [date, date_time, date_raw](#date-date_time-date_raw) |
| other `type: date_*` | AQL over a datetime dimension | [Other date_* types](#other-date_-types) |
| `type: duration_*`, `sql_start`, `sql_end` | `'number'`, AQL `date_diff()` | [Individual duration types](#individual-duration-types) |
| `type: int` | `'number'` | [number](#number) |
| `case`, `alpha_sort` | `'text'`, SQL `CASE` | [case](#case) |
| every other field parameter | see [field.md](field.md) | [Other parameters](#other-parameters) |

---

## Structural parameters

### `dimension`

Looker: [`dimension`](https://docs.cloud.google.com/looker/docs/reference/param-field-dimension) ·
Holistics: [AML Field](https://docs.holistics.io/reference/aml/field)

`dimension: <name> { … }` → `dimension <name> { … }` inside the model of the view.

1. **Name.** Keep the name unchanged.
2. **`label`.** Required in AML. Use the LookML `label`, or the name in title case (`first_name` →
   `'First Name'`). See [field.md](field.md#label).
3. **`type`.** See [`type`](#type).
4. **`definition`.** See [`sql`](#sql).
5. **Placement.** A dimension whose `sql` references another view (`${other_view.field}`) cannot live in the model.
   It becomes a Dataset-level dimension (see [sql-references.md](sql-references.md)).

A `dimension_group` is covered by [dimension-group.md](dimension-group.md).

### `sql`

Looker: [`sql` (for field)](https://docs.cloud.google.com/looker/docs/reference/param-field-sql) ·
Holistics: [AML Field — definition](https://docs.holistics.io/reference/aml/field)

`sql` → `definition: @sql … ;;`. Copy the SQL text unchanged and rewrite only the references:

| LookML | AML, inside `@sql` |
|---|---|
| no `sql` | `{{ #SOURCE.<dimension name> }}` (Looker's default: the column named like the dimension) |
| `${TABLE}.col` | `{{ #SOURCE.col }}` |
| `${field}` (same view) | `{{ field }}` |
| `${view.field}` where `view` is the dimension's own view | `{{ field }}` |
| `${other_view.field}` | not allowed in a model; see [sql-references.md](sql-references.md) |
| `${created_date}`, `${created_raw}` (a timeframe of a dimension group) | see [dimension-group.md](dimension-group.md) |
| `@{constant}` | `${constant}` (see [manifest.md](manifest.md)) |
| Liquid (`{% … %}`, `{{ … }}`) | see [liquid.md](liquid.md), [parameter.md](parameter.md), [filter.md](filter.md) |

```lookml
dimension: email { }
dimension: country_code { type: string  sql: UPPER(${TABLE}.country) ;; }
dimension: full_name { type: string  sql: CONCAT(${first_name}, ' ', ${last_name}) ;; }
```
```aml
dimension email {
  label: 'Email'
  type: 'text'
  definition: @sql {{ #SOURCE.email }};;
}
dimension country_code {
  label: 'Country Code'
  type: 'text'
  definition: @sql UPPER({{ #SOURCE.country }});;
}
dimension full_name {
  label: 'Full Name'
  type: 'text'
  definition: @sql CONCAT({{ first_name }}, ' ', {{ last_name }});;
}
```

Keep `@sql` as the default: the same SQL runs in the same database, so nothing is reinterpreted. Use
`definition: @aql … ;;` only where a section below says so (Looker generates the SQL itself for `tier`, `date_*`
and `duration_*`, so there is no SQL text to copy) or where the dimension spans two views.

Two rules for `@aql` definitions:

- Reference a dimension with its model name: `users.age`, not `age`.
- AQL cannot see table columns. An expression over `${TABLE}.col` needs a dimension for that column first.

Notes:

- The validator does not check SQL text, and it does not compare the declared `type` with the expression. Set
  `type` correctly yourself.
- An `@sql` definition that references a dimension defined in `@aql` (`{{ age_tier }}`) compiles. It has not been
  checked at run time. Mark for review.

## Query parameters

### `primary_key`

Looker: [`primary_key`](https://docs.cloud.google.com/looker/docs/reference/param-field-primary-key) ·
Holistics: [AML Field](https://docs.holistics.io/reference/aml/field)

`primary_key: yes` → `primary_key: true`. `primary_key: no` → omit it.

- A compound key is an ordinary dimension whose SQL concatenates columns. Convert the SQL as usual and mark it.
- A view without a primary key gets none. Do not invent one. Report the view: a `count` measure needs a key
  (see [measure.md](measure.md)).
- Emit at most one `primary_key: true` per model. The validator does not reject two.
- A key whose `sql` references another view cannot be a model dimension. Report it for manual work.

```lookml
dimension: pk {
  primary_key: yes
  hidden: yes
  sql: CONCAT(${TABLE}.tenant_id, '-', ${TABLE}.id) ;;
}
```
```aml
dimension pk {
  label: 'Pk'
  type: 'text'
  hidden: true
  primary_key: true
  definition: @sql CONCAT({{ #SOURCE.tenant_id }}, '-', {{ #SOURCE.id }});;
}
```

## Value and formatting parameters

### `type`

Looker: [`type` (for dimension, filter, or parameter)](https://docs.cloud.google.com/looker/docs/reference/param-dimension-filter-parameter-types) ·
Holistics: [AML Field](https://docs.holistics.io/reference/aml/field)

`type` selects the AML `type` and, for some types, the shape of the definition. The sections below follow the order
of the Looker page.

| LookML `type` | AML `type` | Definition |
|---|---|---|
| `string`, or no `type` | `'text'` | [`sql`](#sql) as is |
| `number`, `int` | `'number'` | [`sql`](#sql) as is |
| `yesno` | `'truefalse'` | SQL wrapped so it is never NULL |
| `date`, `date_time`, `date_raw` | `'date'` / `'datetime'` | SQL as is, or a cast |
| other `date_*` | `'date'`, `'datetime'`, `'number'` or `'text'` | AQL over a datetime dimension |
| `duration_*` | `'number'` | AQL `date_diff()` |
| `tier`, `bin` | `'text'` | AQL `case()` |
| `location` | — | replaced by two `'number'` dimensions |
| `zipcode` | `'text'` | [`sql`](#sql) as is |
| `distance` | `'number'` | distance formula in AQL |
| a dimension with `case:` | `'text'`, whatever `type` says | SQL `CASE` |

Neither tool casts the value: the declared type drives filter operators and formatting only.

### `bin`

Looker: [`bin`](https://docs.cloud.google.com/looker/docs/reference/param-dimension-filter-parameter-types#bin)

`type: bin` with `bins: [...]` is the same as `type: tier` with `tiers: [...]`. Convert it as a [`tier`](#tier).

### `distance`

Looker: [`distance`](https://docs.cloud.google.com/looker/docs/reference/param-dimension-filter-parameter-types#distance) ·
Holistics: [AQL math functions](https://docs.holistics.io/reference/aql/math-functions)

AML has no distance type. Emit a `'number'` dimension that computes the great-circle (haversine) distance from the
latitude and longitude dimensions of the two locations (see [`location`](#location)).

- `start_location_field`, `end_location_field` → the latitude/longitude pair of each location.
- `units` → the Earth radius in that unit: `kilometers` 6371, `meters` 6371000, `miles` 3958.8,
  `nautical_miles` 3440.1, `feet` 20902231, `yards` 6967410.

```lookml
dimension: distance_to_store {
  type: distance
  start_location_field: home_location
  end_location_field: store_location
  units: kilometers
}
```
```aml
dimension distance_to_store {
  label: 'Distance to Store'
  type: 'number'
  definition: @aql 2 * 6371 * asin(sqrt(
    pow(sin(radians(users.store_latitude - users.home_latitude) / 2), 2)
    + cos(radians(users.home_latitude)) * cos(radians(users.store_latitude))
      * pow(sin(radians(users.store_longitude - users.home_longitude) / 2), 2)
  )) ;;
}
```

When the two locations are in different views, the result is a Dataset-level dimension
(see [sql-references.md](sql-references.md)).

Difference: Looker does not document its formula. The haversine formula is the standard spherical approximation;
small differences are possible. Mark for review.

### `custom_calendar`, `duration`, `time`

Looker: [`custom_calendar`](https://docs.cloud.google.com/looker/docs/reference/param-dimension-filter-parameter-types#custom_calendar),
[`duration`](https://docs.cloud.google.com/looker/docs/reference/param-dimension-filter-parameter-types#duration),
[`time`](https://docs.cloud.google.com/looker/docs/reference/param-dimension-filter-parameter-types#time)

These are types of a `dimension_group`, not of a `dimension`. See [dimension-group.md](dimension-group.md). On a
plain `dimension` they are a LookML error; report the field.

### `location`

Looker: [`location`](https://docs.cloud.google.com/looker/docs/reference/param-dimension-filter-parameter-types#location) ·
Holistics: [Point map](https://docs.holistics.io/guides/map/create-point-map)

Unsupported as a single field: AML has no point type. A Holistics map takes a latitude field and a longitude field,
so convert the location into those two `'number'` dimensions and emit no dimension for the location itself.

- `sql_latitude` / `sql_longitude` is a plain `${dim}` → that dimension already exists. Emit nothing more. If it was
  hidden and the location was not, remove its `hidden`.
- Otherwise → create `<name>_latitude` and `<name>_longitude` holding the SQL.

```lookml
dimension: store_location {
  type: location
  sql_latitude: ${TABLE}.store_lat ;;
  sql_longitude: ${TABLE}.store_lng ;;
}
```
```aml
dimension store_location_latitude {
  label: 'Store Location Latitude'
  type: 'number'
  definition: @sql {{ #SOURCE.store_lat }};;
}
dimension store_location_longitude {
  label: 'Store Location Longitude'
  type: 'number'
  definition: @sql {{ #SOURCE.store_lng }};;
}
```

Report that the location was replaced by the pair. Fields that reference `${store_location}` and location filters
(box, "within N miles") need manual work.

### `number`

Looker: [`number`](https://docs.cloud.google.com/looker/docs/reference/param-dimension-filter-parameter-types#number) ·
Holistics: [AML Field](https://docs.holistics.io/reference/aml/field)

`type: number` → `type: 'number'`, definition from [`sql`](#sql). `type: int` (removed from Looker) is the same.

```lookml
dimension: age { type: number  sql: ${TABLE}.age ;; }
```
```aml
dimension age {
  label: 'Age'
  type: 'number'
  definition: @sql {{ #SOURCE.age }};;
}
```

### `string`

Looker: [`string`](https://docs.cloud.google.com/looker/docs/reference/param-dimension-filter-parameter-types#string) ·
Holistics: [AML Field](https://docs.holistics.io/reference/aml/field)

`type: string` → `type: 'text'`, definition from [`sql`](#sql).

A dimension with no `type` is also `'text'`: `string` is Looker's default, even over a numeric column. Do not guess
another type.

```lookml
dimension: city { sql: ${TABLE}.city ;; }
```
```aml
dimension city {
  label: 'City'
  type: 'text'
  definition: @sql {{ #SOURCE.city }};;
}
```

Difference: Looker's string filters are case-sensitive by default; Holistics text filters are case-insensitive
(see [field.md](field.md#case_sensitive)).

### `tier`

Looker: [`tier`](https://docs.cloud.google.com/looker/docs/reference/param-dimension-filter-parameter-types#tier),
[`style`](https://docs.cloud.google.com/looker/docs/reference/param-dimension-filter-parameter-types#style) ·
Holistics: [AQL `case`](https://docs.holistics.io/reference/aql/logical-functions)

`type: tier` → a `'text'` dimension whose AQL `case()` returns the tier label.

1. **Operand.** `sql: ${dim}` → `<model>.<dim>`. Any other SQL (or no `sql`) → first create a hidden helper
   dimension `<name>__value` of type `'number'` holding that SQL, and use `<model>.<name>__value`.
2. **Buckets.** For tiers `t0 < t1 < … < tn`, write one `when` for "below `t0`", one for each `[ti, ti+1)`, one for
   "`tn` and above", and an `else` for NULL.
3. **Labels** depend on `style`:

| Bucket | `classic` (default) | `interval` | `integer` | `relational` |
|---|---|---|---|---|
| `x < t0` | `T00 (-inf,t0)` | `(-inf,t0)` | `Below t0` | `< t0` |
| `ti <= x < ti+1` | `T01 [ti,ti+1)`, `T02 …` | `[ti,ti+1)` | `ti to (ti+1 − 1)` | `>= ti and < ti+1` |
| `x >= tn` | `T<n+1> [tn,inf)` | `[tn,inf)` | `tn or Above` | `>= tn` |
| NULL | `TXX Undefined` | `Undefined` | `Undefined` | `Undefined` |

4. **Sort order.** For every style except `classic`, also emit a hidden `<name>__sort` dimension that returns the
   bucket index. Text sorts alphabetically (`'10 to 19'` before `'20 to 29'` but also before `'5 to 9'`); Looker sorts
   tiers by position.

```lookml
dimension: age_tier {
  type: tier
  tiers: [0, 10, 20, 30]
  style: integer
  sql: ${age} ;;
}
```
```aml
dimension age_tier {
  label: 'Age Tier'
  type: 'text'
  definition: @aql case(
    when: users.age < 0, then: 'Below 0',
    when: users.age >= 0 and users.age < 10, then: '0 to 9',
    when: users.age >= 10 and users.age < 20, then: '10 to 19',
    when: users.age >= 20 and users.age < 30, then: '20 to 29',
    when: users.age >= 30, then: '30 or Above',
    else: 'Undefined'
  ) ;;
}
dimension age_tier__sort {
  label: 'Age Tier (sort order)'
  type: 'number'
  hidden: true
  definition: @aql case(
    when: users.age < 0, then: 0,
    when: users.age >= 0 and users.age < 10, then: 1,
    when: users.age >= 10 and users.age < 20, then: 2,
    when: users.age >= 20 and users.age < 30, then: 3,
    when: users.age >= 30, then: 4
  ) ;;
}
```

A tier over a SQL expression, with the default `classic` style:

```lookml
dimension: spend_tier {
  type: tier
  tiers: [0, 100, 500]
  sql: ${TABLE}.total_spend / 100.0 ;;
}
```
```aml
dimension spend_tier__value {
  label: 'Spend Tier (value)'
  type: 'number'
  hidden: true
  definition: @sql {{ #SOURCE.total_spend }} / 100.0;;
}
dimension spend_tier {
  label: 'Spend Tier'
  type: 'text'
  definition: @aql case(
    when: users.spend_tier__value < 0, then: 'T00 (-inf,0)',
    when: users.spend_tier__value >= 0 and users.spend_tier__value < 100, then: 'T01 [0,100)',
    when: users.spend_tier__value >= 100 and users.spend_tier__value < 500, then: 'T02 [100,500)',
    when: users.spend_tier__value >= 500, then: 'T03 [500,inf)',
    else: 'TXX Undefined'
  ) ;;
}
```

Differences:

- **Label text.** Looker's page does not show every label: the below-first-tier label of the non-integer styles, the
  `Undefined` labels, and how decimals are printed (`10` or `10.0`) are not documented. If saved filters or reports
  match on label text, compare with real Looker output. Mark for review.
- The sort dimension works only when a report author adds it and sorts by it
  (see [field.md](field.md#order_by_field)).
- A `value_format` on a tier formats the numbers inside the labels in Looker (`$10 to $19`). Write the format into
  the label strings by hand.
- Looker can list tiers that have no rows; Holistics shows only values present in the data.

### `unquoted`

Looker: [`unquoted`](https://docs.cloud.google.com/looker/docs/reference/param-dimension-filter-parameter-types#unquoted)

Valid only on a `parameter` field. See [parameter.md](parameter.md).

### `yesno`

Looker: [`yesno`](https://docs.cloud.google.com/looker/docs/reference/param-dimension-filter-parameter-types#yesno) ·
Holistics: [Truefalse type](https://docs.holistics.io/reference/aql/type-truefalse)

`type: yesno` → `type: 'truefalse'`, with the SQL wrapped so that it never returns NULL.

A Looker `yesno` has two values: a NULL condition shows **No**. An AML `truefalse` has three: NULL stays NULL, forms
its own group, and is not matched by `== false`. The wrapper removes the third value.

1. **Default:** `definition: @sql CASE WHEN (<sql>) THEN TRUE ELSE FALSE END;;`
2. **No wrapper** when the SQL is, at top level, `<x> IS NULL`, `<x> IS NOT NULL` or `[NOT] EXISTS (…)`. These cannot
   be NULL.
3. **`sql` is a date/time value** (Looker then answers "does it have a value?") → `@sql <sql> IS NOT NULL;;`. You can
   see this only when `sql` references a time dimension. For a plain column, mark for review.
4. **Cross-view condition** → Dataset-level dimension, `@aql coalesce(<condition>, false) ;;`
   (see [sql-references.md](sql-references.md)).

```lookml
dimension: is_gift    { type: yesno  sql: ${TABLE}.is_gift ;; }
dimension: is_paid    { type: yesno  sql: ${status} = 'paid' ;; }
dimension: is_shipped { type: yesno  sql: ${TABLE}.shipped_at IS NOT NULL ;; }
```
```aml
dimension is_gift {
  label: 'Is Gift'
  type: 'truefalse'
  definition: @sql CASE WHEN ({{ #SOURCE.is_gift }}) THEN TRUE ELSE FALSE END;;
}
dimension is_paid {
  label: 'Is Paid'
  type: 'truefalse'
  definition: @sql CASE WHEN ({{ status }} = 'paid') THEN TRUE ELSE FALSE END;;
}
dimension is_shipped {
  label: 'Is Shipped'
  type: 'truefalse'
  definition: @sql {{ #SOURCE.shipped_at }} IS NOT NULL;;
}
```

Because of the wrapper, a measure filter `is_gift: "no"` converts to `orders.is_gift == false` and still matches the
rows where the column is NULL, as in Looker (see [measure.md](measure.md)).

Differences:

- Holistics shows `true` / `false`; Looker shows `Yes` / `No`. If the strings matter, add a text dimension next to
  the boolean one:

```aml
dimension is_gift_label {
  label: 'Is Gift (Yes/No)'
  type: 'text'
  definition: @aql case(when: orders.is_gift, then: 'Yes', else: 'No') ;;
}
```

- Another field that references the dimension gets the wrapped, never-NULL value. `CASE WHEN ${d} THEN …` behaves the
  same; `NOT ${d}` and `${d} IS NULL` differ from Looker for NULL input.
- Databases without a boolean type (SQL Server, Oracle) have no `TRUE` / `FALSE` literals. Whether a `THEN 1 ELSE 0`
  expression is accepted as `truefalse` there is not documented. Mark for review.

### `zipcode`

Looker: [`zipcode`](https://docs.cloud.google.com/looker/docs/reference/param-dimension-filter-parameter-types#zipcode) ·
Holistics: [Map](https://docs.holistics.io/docs/charts/map)

`type: zipcode` → `type: 'text'`, definition from [`sql`](#sql). The map layer that Looker attaches to a zip code is
lost (see [field.md](field.md#map_layer_name)).

```lookml
dimension: zip { type: zipcode  sql: ${TABLE}.zip ;; }
```
```aml
dimension zip {
  label: 'Zip'
  type: 'text'
  definition: @sql {{ #SOURCE.zip }};;
}
```

### Individual time and date types

Looker: [Individual time and date types](https://docs.cloud.google.com/looker/docs/reference/param-dimension-filter-parameter-types#individual-timeframes) ·
Holistics: [AQL time intelligence functions](https://docs.holistics.io/reference/aql/time-intelligence-functions),
[Timezone settings](https://docs.holistics.io/docs/datetimes/timezones)

A `date_*` dimension is one timeframe of a `dimension_group` of `type: time`, written as its own dimension. Use the
same conversion as [dimension-group.md](dimension-group.md) gives for that timeframe; if the two docs disagree,
dimension-group.md wins.

`datatype` and `convert_tz`: see [dimension-group.md](dimension-group.md#datatype) and
[dimension-group.md](dimension-group.md#convert_tz).

#### `date`, `date_time`, `date_raw`

| LookML | AML |
|---|---|
| `type: date_time` | `type: 'datetime'`, definition from [`sql`](#sql) |
| `type: date_raw` | `type: 'datetime'` (`'date'` with `datatype: date`), `hidden: true`, definition from [`sql`](#sql) |
| `type: date` over a DATE column (`datatype: date`) | `type: 'date'`, definition from [`sql`](#sql) |
| `type: date` over a timestamp, `convert_tz: no` | `type: 'date'`, `definition: @sql CAST(<sql> AS DATE);;` |
| `type: date` over a timestamp, otherwise | `type: 'date'`, `definition: @aql cast(<model>.<base>, 'date') ;;` |
| `type: date_date` (removed from Looker) | same as `date` |

`<base>` is a `'datetime'` dimension of the model with the same SQL: an existing `date_time` or `date_raw`
dimension, or the dimension made from a dimension group over the same column. If there is none, create a hidden one
named `<name>__base`.

```lookml
dimension: created_time { type: date_time  sql: ${TABLE}.created_at ;; }
dimension: created_date { type: date  sql: ${TABLE}.created_at ;; }
dimension: birth_date   { type: date  datatype: date  sql: ${TABLE}.birth_date ;; }
```
```aml
dimension created_time {
  label: 'Created Time'
  type: 'datetime'
  definition: @sql {{ #SOURCE.created_at }};;
}
dimension created_date {
  label: 'Created Date'
  type: 'date'
  definition: @aql cast(users.created_time, 'date') ;;
}
dimension birth_date {
  label: 'Birth Date'
  type: 'date'
  definition: @sql {{ #SOURCE.birth_date }};;
}
```

Differences:

- LookML often does not say whether a column is a DATE or a timestamp. Without `datatype: date` and without the
  database schema, you cannot know. Mark `type: date` dimensions over a plain column for review.
- Looker converts a timestamp to the query time zone before taking the date. Whether the AQL `cast` in a model
  dimension is applied after Holistics' time zone conversion is not documented. Mark for review.

#### Other `date_*` types

Find or create the base `'datetime'` dimension `<b>` as above (several `date_*` dimensions over one column share it),
then:

| LookML `type` | AML `type` | AQL definition |
|---|---|---|
| `date_hour` | `'datetime'` | `date_trunc(<b>, 'hour')` |
| `date_minute` | `'datetime'` | `date_trunc(<b>, 'minute')` |
| `date_week` | `'date'` | `cast(date_trunc(<b>, 'week'), 'date')` |
| `date_month` | `'date'` | `cast(date_trunc(<b>, 'month'), 'date')` |
| `date_quarter` | `'date'` | `cast(date_trunc(<b>, 'quarter'), 'date')` |
| `date_year` | `'number'` | `date_part('year', <b>)` |
| `date_time_of_day` | `'text'` | `date_format(<b>, '%H:%M')` |
| `date_hour_of_day` | `'number'` | `date_part('hour', <b>)` |
| `date_day_of_week` | `'text'` | `date_format(<b>, '%A')` |
| `date_day_of_week_index` | `'number'` | `date_part('dow', <b>)` |
| `date_day_of_month` | `'number'` | `date_part('day', <b>)` |
| `date_month_num` | `'number'` | `date_part('month', <b>)` |
| `date_month_name` | `'text'` | `date_format(<b>, '%B')` |
| `date_quarter_of_year` | `'text'` | `date_format(<b>, 'Q%q')` |
| `date_week_of_year` | `'number'` | `date_part('week', <b>)` |
| `date_second`, `date_millisecond`, `date_microsecond`, `date_hourX`, `date_minuteX`, `date_millisecondX`, `date_day_of_year` | | no AQL unit; use the form of the same timeframe in [dimension-group.md](dimension-group.md) |
| `date_fiscal_*` | | needs `fiscal_month_offset`; see [dimension-group.md](dimension-group.md) |

```lookml
dimension: created_month       { type: date_month        sql: ${TABLE}.created_at ;; }
dimension: created_year        { type: date_year         sql: ${TABLE}.created_at ;; }
dimension: created_day_of_week { type: date_day_of_week  sql: ${TABLE}.created_at ;; }
```
```aml
dimension created_at__base {
  label: 'Created At (base)'
  type: 'datetime'
  hidden: true
  definition: @sql {{ #SOURCE.created_at }};;
}
dimension created_month {
  label: 'Created Month'
  type: 'date'
  definition: @aql cast(date_trunc(users.created_at__base, 'month'), 'date') ;;
}
dimension created_year {
  label: 'Created Year'
  type: 'number'
  definition: @aql date_part('year', users.created_at__base) ;;
}
dimension created_day_of_week {
  label: 'Created Day of Week'
  type: 'text'
  definition: @aql date_format(users.created_at__base, '%A') ;;
}
```

Differences:

- **Display.** Looker shows a month as `2017-09` and a quarter as `2017-Q3`. The AML dimension is a real date (first
  day of the period), so it sorts and filters as a date.
- **Sort order.** Looker sorts day and month names by calendar order; a `'text'` dimension sorts alphabetically.
- **Week start.** Looker uses the model's `week_start_day`; Holistics uses one organization setting
  (see [model.md](model.md#week_start_day)).
- **Day-of-week index and week-of-year.** Looker's index is 0 = Monday; the AQL result depends on the organization's
  week start setting. Week numbering is not specified by Looker. Mark both for review.
- **`date_format` patterns** are not supported by every database. Mark `date_format` dimensions for review.

### Individual duration types

Looker: [Individual duration types](https://docs.cloud.google.com/looker/docs/reference/param-dimension-filter-parameter-types#individual-durations) ·
Holistics: [AQL time intelligence functions](https://docs.holistics.io/reference/aql/time-intelligence-functions),
[Datetime literals](https://docs.holistics.io/reference/aql/datetime-literal)

`type: duration_<unit>` with `sql_start` and `sql_end` → a `'number'` dimension in AQL.

1. Turn `sql_start` and `sql_end` into AQL operands:
   - `${x}` naming a time dimension or a `raw` timeframe → `<model>.<that datetime dimension>`;
   - `CURRENT_TIMESTAMP`, `NOW()`, `GETDATE()`, `CURRENT_DATE` → `@now`;
   - any other SQL → a hidden helper dimension `<name>__start` / `<name>__end` of type `'datetime'` holding the SQL;
   - a reference to another view → the result is a Dataset-level dimension
     (see [sql-references.md](sql-references.md)).
2. Write the difference:

| LookML `type` | AQL definition |
|---|---|
| `duration_day`, `duration_week`, `duration_month`, `duration_quarter`, `duration_year` | `date_diff('<unit>', <start>, <end>)` |
| `duration_second` | `epoch(<end>) - epoch(<start>)` |
| `duration_minute` | `floor((epoch(<end>) - epoch(<start>)) / 60)` |
| `duration_hour` | `floor((epoch(<end>) - epoch(<start>)) / 3600)` |

```lookml
dimension: days_to_first_order {
  type: duration_day
  sql_start: ${created_time} ;;
  sql_end: ${first_order_time} ;;
}
dimension: months_as_customer {
  type: duration_month
  sql_start: ${created_time} ;;
  sql_end: CURRENT_TIMESTAMP ;;
}
dimension: minutes_to_first_order {
  type: duration_minute
  sql_start: ${created_time} ;;
  sql_end: ${first_order_time} ;;
}
```
```aml
dimension days_to_first_order {
  label: 'Days to First Order'
  type: 'number'
  definition: @aql date_diff('day', users.created_time, users.first_order_time) ;;
}
dimension months_as_customer {
  label: 'Months as Customer'
  type: 'number'
  definition: @aql date_diff('month', users.created_time, @now) ;;
}
dimension minutes_to_first_order {
  label: 'Minutes to First Order'
  type: 'number'
  definition: @aql floor((epoch(users.first_order_time) - epoch(users.created_time)) / 60) ;;
}
```

Difference: neither tool specifies how a difference in weeks, months or years is rounded, and Looker's SQL depends on
the database. Results can differ by one near a boundary and for negative durations. Mark for review.

Use the same form as [dimension-group.md](dimension-group.md) uses for a `type: duration` group.

### `case`

Looker: [`case`](https://docs.cloud.google.com/looker/docs/reference/param-field-case),
[`alpha_sort`](https://docs.cloud.google.com/looker/docs/reference/param-field-case#alpha_sort) ·
Holistics: [AML Field](https://docs.holistics.io/reference/aml/field)

`case:` → a `'text'` dimension with a SQL `CASE`.

1. Each `when: { sql: <cond> ;; label: "<L>" }`, in written order → `WHEN <cond> THEN '<L>'`. Rewrite the references
   in the condition as in [`sql`](#sql). Double any single quote in a label.
2. `else: "<E>"` → `ELSE '<E>'`. No `else` → no `ELSE` branch.
3. The type is `'text'` whatever `type` says. A `sql:` next to `case:` is ignored.
4. If two `when`s have the same label, keep only the first one and report it.
5. **Sort order.** With `alpha_sort: yes`, nothing more is needed. Otherwise Looker lists the values in written
   order: also emit a hidden `<name>__sort` dimension with the same conditions returning 0, 1, 2, …
6. A condition that references another view → Dataset-level dimension (see [sql-references.md](sql-references.md)).

```lookml
dimension: order_segment {
  case: {
    when: { sql: ${lifetime_orders} = 0 ;;  label: "None" }
    when: { sql: ${lifetime_orders} > 0 AND ${lifetime_orders} < 3 ;;  label: "1-2" }
    when: { sql: ${lifetime_orders} >= 3 ;;  label: "3+" }
    else: "Unknown"
  }
}
```
```aml
dimension order_segment {
  label: 'Order Segment'
  type: 'text'
  definition: @sql CASE
    WHEN {{ lifetime_orders }} = 0 THEN 'None'
    WHEN {{ lifetime_orders }} > 0 AND {{ lifetime_orders }} < 3 THEN '1-2'
    WHEN {{ lifetime_orders }} >= 3 THEN '3+'
    ELSE 'Unknown'
  END;;
}
dimension order_segment__sort {
  label: 'Order Segment (sort order)'
  type: 'number'
  hidden: true
  definition: @sql CASE
    WHEN {{ lifetime_orders }} = 0 THEN 0
    WHEN {{ lifetime_orders }} > 0 AND {{ lifetime_orders }} < 3 THEN 1
    WHEN {{ lifetime_orders }} >= 3 THEN 2
    ELSE 3
  END;;
}
```

Differences:

- Looker gives a `case` dimension a fixed filter drop-down of all labels. Holistics filter suggestions come from the
  data, so a label with no rows is not offered.
- The sort dimension works only when a report author adds it and sorts by it
  (see [field.md](field.md#order_by_field)).
- Rule 4 follows the worked example on the Looker page; the page's own description says the opposite (the last
  condition wins). Mark duplicate labels for review.
- Without an `else`, a row that matches no condition is assumed to be NULL in Looker too. Not verified.

## Other parameters

Every other parameter of a dimension is shared with the other field kinds and is described in
[field.md](field.md):

| Parameters | See |
|---|---|
| `label`, `description`, `hidden`, `group_label`, `group_item_label`, `view_label`, `label_from_parameter` | [field.md](field.md#label) |
| `value_format`, `value_format_name`, `html` | [field.md](field.md#value_format) |
| `drill_fields`, `link`, `action`, `tags`, `alias`, `synonyms` | [field.md](field.md#drill_fields) |
| `order_by_field` | [field.md](field.md#order_by_field) |
| `case_sensitive`, `can_filter`, `skip_drill_filter`, `suggestions`, `suggest_dimension`, `suggest_explore`, `suggestable`, `full_suggestions`, `bypass_suggest_restrictions`, `suggest_persist_for` | [field.md](field.md#case_sensitive) |
| `required_access_grants`, `required_fields`, `fanout_on` | [field.md](field.md#required_access_grants) |
| `map_layer_name`, `allow_fill`, `string_datatype` | [field.md](field.md#map_layer_name) |
| `datatype`, `convert_tz` | [dimension-group.md](dimension-group.md#datatype) |
