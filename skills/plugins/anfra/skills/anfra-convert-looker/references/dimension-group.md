# LookML `dimension_group` → AML dimensions

How to convert a `dimension_group` and its parameters. The sections follow the order of Looker's
[`dimension_group`](https://docs.cloud.google.com/looker/docs/reference/param-field-dimension-group) page.

AML has no dimension group. The difference to keep in mind:

- In Looker, a `type: time` group generates one field per timeframe (`created_date`, `created_month`, …) and other
  LookML refers to those fields by name.
- In Holistics, one `date` / `datetime` dimension serves every grain. The grain is chosen when the dimension is used:
  [date drill](https://docs.holistics.io/docs/interactions/date-drills) in a report, `orders.created | month()` in AQL.

So a `type: time` group becomes:

1. **one base dimension**, named after the group. It stands for every timeframe that only truncates the value
   (`raw`, `time`, `date`, `week`, `month`, `quarter`, `year`, `hour`, `minute`, `second`, …);
2. **one explicit dimension per other timeframe** (`day_of_week`, `fiscal_quarter`, `hour6`, …), with the name Looker
   generates, defined in AQL over the base;
3. **a hidden alias dimension** for a truncation timeframe only when other SQL references it by name
   (`${created_date}`), see [Referencing the generated fields](#referencing-the-generated-fields).

A `type: duration` group becomes one number dimension per interval.

```lookml
view: orders {
  dimension_group: created {
    type: time
    timeframes: [raw, time, date, week, month, quarter, year, day_of_week]
    sql: ${TABLE}.created_at ;;
    description: "When the order was placed"
  }
}
```
```aml
Model orders {
  type: 'table'
  label: 'Orders'
  data_source_name: 'demodb'
  table_name: 'orders'

  dimension created {
    label: 'Created'
    type: 'datetime'
    description: 'When the order was placed'
    definition: @sql {{ #SOURCE.created_at }};;
  }
  dimension created_day_of_week {
    label: 'Created Day of Week'
    type: 'text'
    description: 'When the order was placed'
    definition: @aql date_format(orders.created, '%A');;
  }
}
```

## Quick reference

| LookML | AML | Section |
|---|---|---|
| `dimension_group` | one or more `dimension`s | [dimension_group](#dimension_group) |
| `type: duration` | one `number` dimension per interval | [Duration type](#duration-type-dimension-groups) |
| `intervals` | `date_diff()` / `epoch()` arithmetic | [intervals](#intervals) |
| `sql_start`, `sql_end` | operands of the duration | [sql_start, sql_end](#sql_start-sql_end) |
| `type: time` | one base `date` / `datetime` dimension | [Time type](#time-type-dimension-groups) |
| `sql` | `definition` of the base dimension | [sql](#sql) |
| `timeframes`: truncations | not emitted; grain of the base | [timeframes](#timeframes) |
| `timeframes`: parts, buckets, fiscal | one explicit dimension each | [timeframes](#timeframes) |
| `convert_tz` | no per-field switch; `date` type or review | [convert_tz](#convert_tz) |
| `datatype` | `type: 'date'` or `'datetime'` | [datatype](#datatype) |
| `type: custom_calendar`, `custom_timeframes`, `based_on_calendar` | unsupported | [Custom calendar type](#custom-calendar-type-dimension-groups) |
| `label`, `group_label`, `description`, `hidden` | spread over the emitted dimensions | [Naming](#naming-the-emitted-dimensions) |
| `${group_timeframe}` references | base, explicit or alias dimension | [Referencing](#referencing-the-generated-fields) |
| other field parameters | as for any field | [field.md](field.md) |

---

## `dimension_group`

Looker: [`dimension_group`](https://docs.cloud.google.com/looker/docs/reference/param-field-dimension-group) ·
Holistics: [AML Field](https://docs.holistics.io/reference/aml/field)

1. Resolve `extends` and refinements first (see [extends-and-refinements.md](extends-and-refinements.md)). A group
   without `type` in a refinement inherits it from the view it refines.
2. Convert by `type`: [`duration`](#duration-type-dimension-groups), [`time`](#time-type-dimension-groups) or
   [`custom_calendar`](#custom-calendar-type-dimension-groups).
3. The emitted dimensions go into the model of the view. If the group's SQL references another view
   (`${dates.date_raw}`), they become Dataset dimensions instead (see [sql-references.md](sql-references.md)).

## Duration type dimension groups

Looker: [`type: duration`](https://docs.cloud.google.com/looker/docs/reference/param-field-dimension-group#type_duration) ·
Holistics: [`date_diff`, `epoch`](https://docs.holistics.io/reference/aql/time-intelligence-functions)

Emit one `type: 'number'` dimension per interval. There is no base dimension.

```lookml
dimension_group: to_ship {
  type: duration
  intervals: [hour, day]
  sql_start: ${created_raw} ;;
  sql_end: ${shipped_raw} ;;
}
```
```aml
dimension hours_to_ship {
  label: 'Hours To Ship'
  type: 'number'
  definition: @aql floor((epoch(orders.shipped) - epoch(orders.created)) / 3600);;
}
dimension days_to_ship {
  label: 'Days To Ship'
  type: 'number'
  definition: @aql date_diff('day', orders.created, orders.shipped);;
}
```

### `intervals`

Looker: [`intervals`](https://docs.cloud.google.com/looker/docs/reference/param-field-dimension-group#intervals)

Each interval becomes `dimension <interval>s_<group>` (Looker's name: plural interval first, `days_to_ship`).
If `intervals` is omitted, emit all eight.

| Interval | Name | `@aql` definition |
|---|---|---|
| `second` | `seconds_<group>` | `epoch(<end>) - epoch(<start>)` |
| `minute` | `minutes_<group>` | `floor((epoch(<end>) - epoch(<start>)) / 60)` |
| `hour` | `hours_<group>` | `floor((epoch(<end>) - epoch(<start>)) / 3600)` |
| `day` | `days_<group>` | `date_diff('day', <start>, <end>)` |
| `week` | `weeks_<group>` | `date_diff('week', <start>, <end>)` |
| `month` | `months_<group>` | `date_diff('month', <start>, <end>)` |
| `quarter` | `quarters_<group>` | `date_diff('quarter', <start>, <end>)` |
| `year` | `years_<group>` | `date_diff('year', <start>, <end>)` |

Differences. Mark duration dimensions for review:

- Looker floors every duration, and `month`, `quarter` and `year` count complete periods only (26 Sep → 25 Oct is
  0 months). The AQL docs do not state how `date_diff` rounds; results can differ by one unit.
- Looker shows the value with its unit ("9 days"); the AML dimension is a bare number.

### `sql_start`, `sql_end`

Looker: [`sql_start`](https://docs.cloud.google.com/looker/docs/reference/param-field-dimension-group#sql_start),
[`sql_end`](https://docs.cloud.google.com/looker/docs/reference/param-field-dimension-group#sql_end) ·
Holistics: [Datetime literals](https://docs.holistics.io/reference/aql/datetime-literal)

AQL functions take dimensions, not SQL text. Convert each side to an operand:

| `sql_start` / `sql_end` | Operand |
|---|---|
| `${g_raw}`, `${g_time}`, `${g_date}` or `${dim}` of this view | that dimension: `orders.created` (see [Referencing](#referencing-the-generated-fields)) |
| `CURRENT_TIMESTAMP`, `NOW()`, `GETDATE()`, `SYSDATE` | `@now` |
| `CURRENT_DATE` | `@today` |
| any other SQL of this view | a hidden helper dimension `<group>__start` / `<group>__end` that holds the SQL unchanged |
| a field of another view | the duration dimensions become Dataset dimensions (see [sql-references.md](sql-references.md)) |
| Liquid | see [liquid.md](liquid.md) |

The helper is `type: 'datetime'`, or `'date'` when the group has `datatype: date`. For `datatype: epoch` and
`yyyymmdd`, convert the helper as in [`datatype`](#datatype).

```lookml
dimension_group: since_signup {
  type: duration
  intervals: [day, month]
  sql_start: COALESCE(${TABLE}.activated_at, ${TABLE}.registered_at) ;;
  sql_end: CURRENT_TIMESTAMP ;;
}
```
```aml
dimension since_signup__start {
  label: 'Since Signup (start)'
  type: 'datetime'
  hidden: true
  definition: @sql COALESCE({{ #SOURCE.activated_at }}, {{ #SOURCE.registered_at }});;
}
dimension days_since_signup {
  label: 'Days Since Signup'
  type: 'number'
  definition: @aql date_diff('day', users.since_signup__start, @now);;
}
dimension months_since_signup {
  label: 'Months Since Signup'
  type: 'number'
  definition: @aql date_diff('month', users.since_signup__start, @now);;
}
```

### Referencing intervals from another LookML field

`${days_to_ship}` keeps its name: the dimension exists. Rewrite the syntax as for any field reference
(`{{ days_to_ship }}` in `@sql`, `orders.days_to_ship` in `@aql`; see [sql-references.md](sql-references.md)).

### Field type references (`::date`, `::datetime`)

`${created_date::date}`, `${created::datetime}` and similar suffixes inside `sql_start` / `sql_end` → drop the
suffix and use the dimension. The AML dimension already has a date type.

## Time type dimension groups

Looker: [`type: time`](https://docs.cloud.google.com/looker/docs/reference/param-field-dimension-group#type_time) ·
Holistics: [AML Field](https://docs.holistics.io/reference/aml/field),
[Date drill](https://docs.holistics.io/docs/interactions/date-drills)

Emit the base dimension:

1. **Name:** the group name. If the view already has a field with that name, use `<group>_time` for a `datetime`
   base and `<group>_date` for a `date` base.
2. **Type:** `'datetime'`, or `'date'` as decided by [`datatype`](#datatype) and [`convert_tz`](#convert_tz).
3. **Definition:** from [`sql`](#sql).
4. **Label, description, hidden:** see [Naming](#naming-the-emitted-dimensions).
5. If the group lists no truncation timeframe at all (for example `timeframes: [day_of_week]`), still emit the base,
   with `hidden: true`. The explicit dimensions are defined over it.

Then add the explicit dimensions that [`timeframes`](#timeframes) asks for.

### `sql`

Looker: [`sql`](https://docs.cloud.google.com/looker/docs/reference/param-field-sql)

Converted like the `sql` of a dimension (see [dimension.md](dimension.md) and
[sql-references.md](sql-references.md)): `${TABLE}.col` → `{{ #SOURCE.col }}`, other SQL is kept unchanged. Without
`sql`, the column is named like the group.

### `timeframes`

Looker: [`timeframes`](https://docs.cloud.google.com/looker/docs/reference/param-field-dimension-group#timeframes) ·
Holistics: [Time intelligence functions](https://docs.holistics.io/reference/aql/time-intelligence-functions),
[Date parts](https://docs.holistics.io/docs/datetimes/date-parts)

Each timeframe is either served by the base dimension (emit nothing) or becomes an explicit dimension named
`<group>_<timeframe>`. In the definitions, `<b>` is the base dimension written with its model (`orders.created`).

If `timeframes` is omitted, Looker generates a default set. Convert the group as if it listed: `date`,
`day_of_month`, `day_of_week`, `day_of_week_index`, `day_of_year`, `hour`, `hour_of_day`, `minute`, `month`,
`month_name`, `month_num`, `quarter`, `quarter_of_year`, `time`, `time_of_day`, `week`, `week_of_year`, `year`.

| Timeframe | Emit | AML `type` | `@aql` definition |
|---|---|---|---|
| **Special** | | | |
| `raw` | nothing (base) | | |
| `yesno` | `<group>_yesno` | `'truefalse'` | `<b> is not null` |
| **Time** | | | |
| `time` | nothing (base) | | |
| `time_of_day` | explicit | `'text'` | `date_format(<b>, '%H:%M')` |
| `hour` | nothing (base at `hour` grain) | | |
| `hour_of_day` | explicit | `'number'` | `date_part('hour', <b>)` |
| `hourX` | explicit | `'datetime'` | see [hourX](#hourx) |
| `minute` | nothing (base at `minute` grain) | | |
| `minuteX` | explicit | `'datetime'` | see [minuteX](#minutex) |
| `second`, `millisecond`, `microsecond` | nothing (base, no truncation) | | see [below](#second-millisecond-microsecond-millisecondx) |
| `millisecondX` | unsupported | | see [below](#second-millisecond-microsecond-millisecondx) |
| **Date** | | | |
| `date` | nothing (base at `day` grain) | | |
| **Week** | | | |
| `week` | nothing (base at `week` grain) | | |
| `day_of_week` | explicit | `'text'` | `date_format(<b>, '%A')` |
| `day_of_week_index` | explicit | `'number'` | `date_part('dow', <b>)` |
| **Month** | | | |
| `month` | nothing (base at `month` grain) | | |
| `month_num` | explicit | `'number'` | `date_part('month', <b>)` |
| `fiscal_month_num` | explicit | `'number'` | see [Fiscal timeframes](#fiscal-timeframes) |
| `month_name` | explicit | `'text'` | `date_format(<b>, '%B')` |
| `day_of_month` | explicit | `'number'` | `date_part('day', <b>)` |
| **Quarter** | | | |
| `quarter` | nothing (base at `quarter` grain) | | |
| `fiscal_quarter` | explicit | `'text'` | see [Fiscal timeframes](#fiscal-timeframes) |
| `quarter_of_year` | explicit | `'text'` | `concat('Q', cast(date_part('quarter', <b>), 'text'))` |
| `fiscal_quarter_of_year` | explicit | `'text'` | see [Fiscal timeframes](#fiscal-timeframes) |
| **Year** | | | |
| `year` | nothing (base at `year` grain) | | |
| `fiscal_year` | explicit | `'text'` | see [Fiscal timeframes](#fiscal-timeframes) |
| `day_of_year` | explicit | `'number'` | `date_diff('day', <b> \| year(), <b>) + 1` |
| `week_of_year` | explicit | `'number'` | `date_part('week', <b>)` |

```lookml
dimension_group: created {
  type: time
  timeframes: [date, month, hour_of_day, day_of_week, month_name, quarter_of_year, day_of_year]
  sql: ${TABLE}.created_at ;;
}
```
```aml
dimension created {
  label: 'Created'
  type: 'datetime'
  definition: @sql {{ #SOURCE.created_at }};;
}
dimension created_hour_of_day {
  label: 'Created Hour of Day'
  type: 'number'
  definition: @aql date_part('hour', orders.created);;
}
dimension created_day_of_week {
  label: 'Created Day of Week'
  type: 'text'
  definition: @aql date_format(orders.created, '%A');;
}
dimension created_month_name {
  label: 'Created Month Name'
  type: 'text'
  definition: @aql date_format(orders.created, '%B');;
}
dimension created_quarter_of_year {
  label: 'Created Quarter of Year'
  type: 'text'
  definition: @aql concat('Q', cast(date_part('quarter', orders.created), 'text'));;
}
dimension created_day_of_year {
  label: 'Created Day of Year'
  type: 'number'
  definition: @aql date_diff('day', orders.created | year(), orders.created) + 1;;
}
```

Differences that apply to all timeframes:

- **Restricting grains is not possible.** Looker shows only the listed timeframes. A Holistics user can pick any
  grain from year to minute on the base dimension.
- **Value shape.** Looker's `month` is the text `2014-09`, `quarter` is `2017-Q3`, `year` is the number `2017`.
  Holistics returns a truncated date and formats it by grain.

#### Special timeframes

- `raw`: the base dimension. Difference: Looker never converts `raw` to the query time zone and hides it from users.
  The base dimension is visible and is converted like any `datetime` (see [`convert_tz`](#convert_tz)).
- `yesno`: Looker names this field like the group (`${created}`). That name is taken by the base, so emit
  `<group>_yesno` and rewrite references and filters (`created: "yes"`) to it.

```lookml
dimension_group: shipped {
  type: time
  timeframes: [date, yesno]
  sql: ${TABLE}.shipped_at ;;
}
```
```aml
dimension shipped {
  label: 'Shipped'
  type: 'datetime'
  definition: @sql {{ #SOURCE.shipped_at }};;
}
dimension shipped_yesno {
  label: 'Shipped Yesno'
  type: 'truefalse'
  definition: @aql orders.shipped is not null;;
}
```

#### Time timeframes

`time`, `hour` and `minute` are the base. `time_of_day` and `hour_of_day` are explicit (see the table).

#### Date timeframes

`date` is the base at `day` grain (`orders.created | day()` in AQL).

#### Week timeframes

- `week` starts on the day set by the model's `week_start_day` in Looker, and on the organization's Week Start Day
  in Holistics. See [model.md](model.md#week_start_day).
- `day_of_week` is text and sorts alphabetically; Looker sorts it in calendar order. Sort by `day_of_week_index`
  where the order matters.
- `day_of_week_index`: Looker counts from 0 at the first day of the week. AQL `date_part('dow', …)` is also 0-based
  and follows the organization's Week Start Day, but its numbering is not verified against Looker's. Mark for review.

#### Month timeframes

`month_name` is text and sorts alphabetically; sort by `month_num` where the order matters. The names produced by
`date_format` depend on the database locale.

#### Quarter timeframes

No special handling beyond the table.

#### Year timeframes

- `week_of_year` follows ISO 8601 on both sides. It is not affected by `week_start_day`.
- `day_of_year`: AQL has no day-of-year part. The `date_diff` form in the table assumes `date_diff('day', …)`
  counts calendar days. Mark for review.

#### Fiscal timeframes

Holistics has no fiscal calendar setting. The model file's `fiscal_month_offset`
(see [model.md](model.md#fiscal_month_offset)) is compiled into the dimensions. Let `N` be the offset (0 if absent).

1. `N ≠ 0`: emit a hidden helper `dimension <group>__fiscal`, same type as the base, defined
   `<b> - interval(N month)`. For a negative offset use `<b> + interval(|N| month)`. `N = 0`: use the base itself.
2. Define the fiscal dimensions over the helper `<f>`:

| Timeframe | AML `type` | `@aql` definition |
|---|---|---|
| `fiscal_year` | `'text'` | `concat('FY', cast(date_part('year', <f>), 'text'))` |
| `fiscal_quarter` | `'text'` | `concat('FY', cast(date_part('year', <f>), 'text'), '-Q', cast(date_part('quarter', <f>), 'text'))` |
| `fiscal_quarter_of_year` | `'text'` | `concat('Q', cast(date_part('quarter', <f>), 'text'))` |
| `fiscal_month_num` | `'number'` | `date_part('month', <f>)` |

```lookml
# model file
fiscal_month_offset: 3

# view
dimension_group: created {
  type: time
  timeframes: [date, fiscal_year, fiscal_quarter]
  sql: ${TABLE}.created_at ;;
}
```
```aml
dimension created {
  label: 'Created'
  type: 'datetime'
  definition: @sql {{ #SOURCE.created_at }};;
}
dimension created__fiscal {
  label: 'Created (fiscal-shifted)'
  type: 'datetime'
  hidden: true
  definition: @aql orders.created - interval(3 month);;
}
dimension created_fiscal_year {
  label: 'Created Fiscal Year'
  type: 'text'
  definition: @aql concat('FY', cast(date_part('year', orders.created__fiscal), 'text'));;
}
dimension created_fiscal_quarter {
  label: 'Created Fiscal Quarter'
  type: 'text'
  definition: @aql concat('FY', cast(date_part('year', orders.created__fiscal), 'text'), '-Q', cast(date_part('quarter', orders.created__fiscal), 'text'));;
}
```

Differences:

- The fiscal dimensions are text or number. They group and filter, but they are not a date grain: no date drill into
  a fiscal quarter, no fiscal period comparison.
- The exact text Looker shows (`FY2017-Q3` or `2017-Q3`) is not consistent in Looker's docs. Check it when reports
  filter on these values.
- If model files with different offsets include the same view, one model cannot serve both. Report it.

#### `hourX`

Looker: [Using `hourX`](https://docs.cloud.google.com/looker/docs/reference/param-field-dimension-group#using_hourx)

`hour2`, `hour3`, `hour4`, `hour6`, `hour8`, `hour12` → an explicit `datetime` dimension with one `case` branch per
bucket, highest first: the start of the day plus a fixed interval.

```lookml
dimension_group: created {
  type: time
  timeframes: [time, hour6]
  sql: ${TABLE}.created_at ;;
}
```
```aml
dimension created_hour6 {
  label: 'Created Hour6'
  type: 'datetime'
  definition: @aql case(
    when: date_part('hour', orders.created) >= 18, then: (orders.created | day()) + interval(18 hour),
    when: date_part('hour', orders.created) >= 12, then: (orders.created | day()) + interval(12 hour),
    when: date_part('hour', orders.created) >= 6, then: (orders.created | day()) + interval(6 hour),
    else: orders.created | day()
  );;
}
```

#### `minuteX`

Looker: [Using `minuteX`](https://docs.cloud.google.com/looker/docs/reference/param-field-dimension-group#using_minutex)

`minute2` … `minute30` → an explicit `datetime` dimension that floors the Unix time to `X × 60` seconds.

```lookml
dimension_group: created {
  type: time
  timeframes: [time, minute15]
  sql: ${TABLE}.created_at ;;
}
```
```aml
dimension created_minute15 {
  label: 'Created Minute15'
  type: 'datetime'
  definition: @aql from_unixtime(floor(epoch(orders.created) / 900) * 900);;
}
```

Mark `hourX` and `minuteX` dimensions for review. The `minuteX` bucket is computed in UTC, so it equals Looker's
only when the reporting time zone's offset is a multiple of X minutes. How `from_unixtime` interacts with time zone
conversion is not documented.

#### `second`, `millisecond`, `microsecond`, `millisecondX`

Looker: [Using `millisecondX`](https://docs.cloud.google.com/looker/docs/reference/param-field-dimension-group#using_millisecondx),
[Dialect support](https://docs.cloud.google.com/looker/docs/reference/param-field-dimension-group#dialect_support_for_milliseconds_and_microseconds)

- `second`, `millisecond`, `microsecond`: use the base dimension as it is. Holistics has no grain finer than minute
  and does not truncate, so the value equals Looker's only if the column holds no finer precision.
- `millisecondX`: unsupported in AQL. Point references at the base dimension. If the bucket itself is needed, a
  person writes an `@sql` dimension in the database's dialect.

### `convert_tz`

Looker: [`convert_tz`](https://docs.cloud.google.com/looker/docs/reference/param-field-convert-tz) ·
Holistics: [Timezone settings](https://docs.holistics.io/docs/datetimes/timezones)

Unsupported as a property. Holistics converts every `datetime` to the organization's (or dashboard's) time zone and
has no per-field switch. A `date` dimension carries no time and is treated here as not converted.

| LookML | Emit |
|---|---|
| `convert_tz` absent or `yes` | `datetime` base. Conversion is automatic on both sides. |
| `convert_tz: no` with `datatype: date` | `date` base. Drop the flag. |
| `convert_tz: no`, timestamp column, no sub-day timeframe | `date` base defined as `CAST(<sql> AS DATE)`, so every grain keeps the database value |
| `convert_tz: no`, timestamp column, with sub-day timeframes (`time`, `hour`, `hour_of_day`, …) | unsupported: `datetime` base, and report it |

```lookml
dimension_group: snapshot {
  type: time
  timeframes: [date, week, month]
  convert_tz: no
  sql: ${TABLE}.snapshot_ts ;;
}
```
```aml
dimension snapshot {
  label: 'Snapshot'
  type: 'date'
  definition: @sql CAST({{ #SOURCE.snapshot_ts }} AS DATE);;
}
```

For the unsupported case a person chooses one of: set the organization time zone to the database time zone; ask
Holistics support to turn the conversion off; or split the field into a `date` dimension and a text time-of-day
dimension.

Differences:

- Looker can convert per user (User Specific Time Zones). Holistics converts per organization or per dashboard.
- In Looker a `convert_tz: no` field is shown unconverted but still filtered in the query time zone. With the
  `date` form above, Holistics shows and filters the same value.

### `datatype`

Looker: [`datatype`](https://docs.cloud.google.com/looker/docs/reference/param-field-datatype) ·
Holistics: [AML Field](https://docs.holistics.io/reference/aml/field)

`datatype` decides the type of the base dimension.

| `datatype` | Base dimension |
|---|---|
| absent, `timestamp`, `datetime` | `type: 'datetime'`, SQL unchanged |
| `date` | `type: 'date'`, SQL unchanged |
| `epoch` | hidden number dimension `<group>__epoch` holding the SQL; base `type: 'datetime'` defined `from_unixtime(<model>.<group>__epoch)` |
| `yyyymmdd` | `type: 'date'`, `@sql` that parses the integer in the database's dialect |

```lookml
dimension_group: scanned {
  type: time
  timeframes: [time, date]
  datatype: epoch
  sql: ${TABLE}.scanned_ts ;;
}

dimension_group: booked {
  type: time
  timeframes: [date, month]
  datatype: yyyymmdd
  sql: ${TABLE}.booked_on ;;
}
```
```aml
dimension scanned__epoch {
  label: 'Scanned (epoch seconds)'
  type: 'number'
  hidden: true
  definition: @sql {{ #SOURCE.scanned_ts }};;
}
dimension scanned {
  label: 'Scanned'
  type: 'datetime'
  definition: @aql from_unixtime(orders.scanned__epoch);;
}
dimension booked {
  label: 'Booked'
  type: 'date'
  definition: @sql TO_DATE(CAST({{ #SOURCE.booked_on }} AS VARCHAR), 'YYYYMMDD');;
}
```

(The `yyyymmdd` expression above is PostgreSQL.)

Notes:

- The type matters more in AML than in Looker: it decides which grains, operators and time zone handling apply.
  `anfra validate` does not check the declared type against the SQL.
- Without `datatype`, the column may still be a date (`${TABLE}.x::date`, `DATE(...)`). Use `type: 'date'` only when
  the SQL visibly returns a date and no sub-day timeframe is listed. Otherwise keep `datetime`.
- With a `date` base, sub-day timeframes (`time`, `hour`, `hour_of_day`, …) have no meaning. Emit them anyway and
  report the group.
- On a `type: duration` group, `datatype` describes `sql_start` and `sql_end`. Apply the table to the helper
  dimensions.

## Custom calendar type dimension groups

Looker: [`type: custom_calendar`](https://docs.cloud.google.com/looker/docs/reference/param-field-dimension-group#custom_calendar_type)

Unsupported, together with `custom_timeframes` and `based_on_calendar`. Holistics has no custom-calendar grain.

Closest workaround, for a person to review:

1. Convert the group's own column as an ordinary [time type](#time-type-dimension-groups) base dimension.
2. Convert the calendar view as an ordinary model; its columns become plain dimensions.
3. In the Dataset, relate a `date` dimension of the fact model to the calendar model's reference date (many-to-one),
   so users can group by the calendar model's dimensions (see [explore.md](explore.md)).

Lost: the custom periods are not grains of the date. No date drill and no period comparison in custom periods.

## Localizing dimension groups

Unsupported. AML labels are plain strings (see [manifest.md](manifest.md#localization_settings)).

## Naming the emitted dimensions

Looker: [`label`](https://docs.cloud.google.com/looker/docs/reference/param-field-label),
[`group_label`](https://docs.cloud.google.com/looker/docs/reference/param-field-group-label),
[`description`](https://docs.cloud.google.com/looker/docs/reference/param-field-description)

Let `L` be the group's `label`, or without one the group name in title case (`created_at` → `Created At`).

| Emitted dimension | `label` |
|---|---|
| base | `L` |
| explicit timeframe, alias | `L` + the timeframe's title: `Created Day of Week`, `Created Fiscal Quarter`, `Created Hour6` |
| duration interval | plural interval + `L`: `Days To Ship` |
| helper (`__fiscal`, `__epoch`, `__start`, `__end`) | `L` + its role in parentheses; always `hidden: true` |

- Do not append "Date" to the base label. At month grain "Created Date" would be misleading.
- `description` → copy to the base and to every explicit dimension.
- `hidden: yes` → `hidden: true` on every emitted dimension. Alias and helper dimensions are always hidden.
- `group_label`, `group_item_label`, `view_label` → as for any field (see [field.md](field.md#group_label)), applied
  to the base and the explicit dimensions alike. Looker's automatic group ("Created Date" containing Date, Week,
  Month) is not reproduced: those timeframes are one field now.

```lookml
dimension_group: shipped {
  label: "Dispatch"
  type: time
  timeframes: [date, month, day_of_week]
  hidden: yes
  sql: ${TABLE}.shipped_at ;;
}
```
```aml
dimension shipped {
  label: 'Dispatch'
  type: 'datetime'
  hidden: true
  definition: @sql {{ #SOURCE.shipped_at }};;
}
dimension shipped_day_of_week {
  label: 'Dispatch Day of Week'
  type: 'text'
  hidden: true
  definition: @aql date_format(orders.shipped, '%A');;
}
```

## Referencing the generated fields

Looker: [Dimension groups must be referenced by their individual dimensions](https://docs.cloud.google.com/looker/docs/reference/param-field-dimension-group#things_to_consider) ·
Holistics: [Time intelligence functions](https://docs.holistics.io/reference/aql/time-intelligence-functions)

Other LookML refers to `${created_date}`, `${orders.created_month}`, or lists `created_date` in a field list. To
recognise such a name, split it as `<group>_<timeframe>` and match the longest group name (with groups `created` and
`created_at`, `${created_at_date}` belongs to `created_at`).

**Inside SQL** (`sql`, `sql_on`, `sql_always_where`, `sql_start`, `sql_end`, measure `sql`):

| Reference | Rewrite to |
|---|---|
| `${g_raw}`; `${g_time}` on a `datetime` base; `${g_date}` on a `date` base | the base: `{{ g }}` in `@sql`, `<model>.g` in `@aql` |
| an explicit timeframe (`${g_day_of_week}`) or a duration field (`${days_g}`) | the same name, in `@aql` only |
| `${g}` when the group has the `yesno` timeframe | `g_yesno` |
| `${g}` otherwise | an error in Looker. Report it. |
| any other truncation timeframe | a hidden alias dimension `g_<timeframe>`, emitted once, in `@aql` only |

Every dimension in this table except the base is defined with `@aql`. An `@sql` definition cannot reference an
`@aql` field (E210). So a field that references one of them is written in `@aql`: see
[sql-references.md](sql-references.md#a-reference-to-an-aql-dimension).

Alias dimensions reproduce the value Looker substitutes:

| Alias | AML `type` | `@aql` definition (`datetime` base) |
|---|---|---|
| `g_date` | `'date'` | `cast(<b>, 'date')` |
| `g_week` | `'date'` | `cast(<b> \| week(), 'date')` (on a `date` base: `<b> \| week()`) |
| `g_month` | `'text'` | `date_format(<b>, '%Y-%m')` |
| `g_quarter` | `'text'` | `date_format(<b>, '%Y-Q%q')` |
| `g_year` | `'number'` | `date_part('year', <b>)` |
| `g_hour` | `'text'` | `date_format(<b>, '%Y-%m-%d %H')` |
| `g_minute` | `'text'` | `date_format(<b>, '%Y-%m-%d %H:%M')` |
| `g_second` | `'text'` | `date_format(<b>, '%Y-%m-%d %H:%M:%S')` |

Drop `::date`, `::datetime` and `::string` suffixes on these references.

```lookml
dimension_group: created { type: time  timeframes: [raw, date]  sql: ${TABLE}.created_at ;; }
dimension_group: shipped { type: time  timeframes: [date]  datatype: date  sql: ${TABLE}.shipped_on ;; }

dimension: is_same_day_dispatch { type: yesno  sql: ${shipped_date} = ${created_date} ;; }
measure: active_days { type: count_distinct  sql: ${created_date} ;; }
measure: first_order { type: date  sql: MIN(${created_raw}) ;; }
measure: count_last_7_days { type: count  filters: [created_date: "7 days"] }
```
```aml
dimension created {
  label: 'Created'
  type: 'datetime'
  definition: @sql {{ #SOURCE.created_at }};;
}
dimension shipped {
  label: 'Shipped'
  type: 'date'
  definition: @sql {{ #SOURCE.shipped_on }};;
}
dimension created_date {
  label: 'Created Date'
  type: 'date'
  hidden: true
  definition: @aql cast(orders.created, 'date');;
}
dimension is_same_day_dispatch {
  label: 'Is Same Day Dispatch'
  type: 'truefalse'
  definition: @aql orders.shipped == orders.created_date;;
}
measure active_days {
  label: 'Active Days'
  type: 'number'
  definition: @aql count_distinct(orders.created_date);;
}
measure first_order {
  label: 'First Order'
  type: 'date'
  definition: @aql cast(min(orders.created), 'date');;
}
measure count_last_7_days {
  label: 'Count Last 7 Days'
  type: 'number'
  definition: @aql count(orders.id) | where(orders.created matches @(6 days ago - today));;
}
```

`shipped` is a `date` base, so `${shipped_date}` is the base itself. `created` is a `datetime` base, so
`${created_date}` needs the alias. The alias is `@aql`, so `is_same_day_dispatch` is written in `@aql` too. Measures are covered in [measure.md](measure.md), the `filters` condition in
[filter-expressions.md](filter-expressions.md).

**Outside SQL:**

| Where | Rewrite |
|---|---|
| field lists: `drill_fields`, `set`, explore `fields`, `suggest_dimension`, `order_by_field` | truncation timeframe → the base, once per list; explicit timeframe → the same name |
| filter keys: measure `filters`, `always_filter`, `access_filter` | truncation timeframe → a condition on the base (see [filter-expressions.md](filter-expressions.md)); explicit timeframe → a condition on that dimension |
| derived-table columns and keys (`column`, `increment_key`, `partition_keys`) | the base at that grain (see [derived-tables.md](derived-tables.md)) |

Mark for review:

- Every non-`raw` reference inside a larger SQL expression. Looker substitutes the timeframe's own SQL there: time
  zone converted and, for most timeframes, a formatted string. The alias gives a typed value (`date`, `number`)
  where Looker gave text.
- Alias dimensions used as join keys in `sql_on`. Whether `cast(<b>, 'date')` is taken before or after Holistics'
  time zone conversion is not documented. For joins prefer a `date` base.
- Every field rewritten from SQL to `@aql` because it references an alias: the comparison and NULL handling are
  those of AQL.

## Things to consider

- **`primary_key` on a group.** Looker allows it only with a single timeframe. Put `primary_key: true` on the base
  only if that timeframe equals the base (`raw`, `time` on a `datetime` base, `date` on a `date` base). Otherwise
  report it.
- **Timestamps with time zone information.** A cast that the LookML author wrote in `sql` (for example Snowflake
  `TO_TIMESTAMP_NTZ(...)`) is carried over unchanged.
- **Individual time or duration dimensions** (`dimension` with `type: date`, `type: date_month`,
  `type: duration_day`, …): see [dimension.md](dimension.md).
- **First day of the week:** see [model.md](model.md#week_start_day).
- **`timezone_values`** is not a parameter of the group. It lists the zone names accepted by a native derived table
  (see [derived-tables.md](derived-tables.md)).

## Other parameters

All other parameters of a dimension group (`drill_fields`, `html`, `tags`, `alias`, `allow_fill`, `can_filter`,
`suggest_*`, `required_access_grants`, …) are converted as for any field: see [field.md](field.md). Two of them
behave differently on a group:

- `drill_fields` on a time group limits which finer timeframes a user can drill to. Unsupported: Holistics date
  drill offers every grain. Drop it.
- `alias` lists old *group* names. Unsupported. Drop it.
