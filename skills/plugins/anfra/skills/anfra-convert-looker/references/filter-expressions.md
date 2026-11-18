# Looker filter expressions → AQL conditions

How to convert a Looker filter expression (`"Web,App"`, `"-NULL"`, `">=18 AND <=25"`, `"7 days"`) into an AQL
condition. The sections follow Looker's
[Filter expressions](https://docs.cloud.google.com/looker/docs/filter-expressions) page.

A filter expression is not a LookML parameter. It is the *value* of one: the string after the field name in a
measure's `filters`, in `always_filter`, in `access_filter`, and so on. It becomes a condition on the filtered
dimension, most often inside [`where()`](https://docs.holistics.io/reference/aql/where):

```lookml
view: events {
  sql_table_name: public.events ;;

  dimension: id { primary_key: yes  type: number  sql: ${TABLE}.id ;; }
  dimension: channel { type: string  sql: ${TABLE}.channel ;; }
  dimension: amount { type: number  sql: ${TABLE}.amount ;; }
  dimension: is_paid { type: yesno  sql: ${TABLE}.paid_at IS NOT NULL ;; }
  dimension_group: created { type: time  timeframes: [raw, date, month]  sql: ${TABLE}.created_at ;; }

  measure: web_or_app_count { type: count  filters: [channel: "Web,App"] }
  measure: mid_count { type: count  filters: [amount: ">=18 AND <=25"] }
  measure: last_7_days_count { type: count  filters: [created_date: "7 days"] }
  measure: paid_web_count { type: count  filters: [channel: "Web", is_paid: "yes", amount: ">0"] }
}
```
```aml
Model events {
  type: 'table'
  label: 'Events'
  data_source_name: 'demodb'
  table_name: 'public.events'

  dimension id {
    label: 'Id'
    type: 'number'
    primary_key: true
    definition: @sql {{ #SOURCE.id }};;
  }
  dimension channel {
    label: 'Channel'
    type: 'text'
    definition: @sql {{ #SOURCE.channel }};;
  }
  dimension amount {
    label: 'Amount'
    type: 'number'
    definition: @sql {{ #SOURCE.amount }};;
  }
  dimension is_paid {
    label: 'Is Paid'
    type: 'truefalse'
    definition: @sql {{ #SOURCE.paid_at }} IS NOT NULL;;
  }
  dimension created {
    label: 'Created'
    type: 'datetime'
    definition: @sql {{ #SOURCE.created_at }};;
  }

  measure web_or_app_count {
    label: 'Web Or App Count'
    type: 'number'
    definition: @aql count(events.id) | where(events.channel in ['Web', 'App']);;
  }
  measure mid_count {
    label: 'Mid Count'
    type: 'number'
    definition: @aql count(events.id) | where(events.amount >= 18 and events.amount <= 25);;
  }
  measure last_7_days_count {
    label: 'Last 7 Days Count'
    type: 'number'
    definition: @aql count(events.id) | where(events.created matches @(6 days ago - today));;
  }
  measure paid_web_count {
    label: 'Paid Web Count'
    type: 'number'
    definition: @aql count(events.id)
      | where(events.channel == 'Web', events.is_paid == true, events.amount > 0);;
  }
}
```

In the tables below, `f` stands for the filtered dimension, written in full in real code (`events.channel`).

## Quick reference

| Field type | Example expressions | AQL | Section |
|---|---|---|---|
| any | `""` (empty string) | no condition | [Before you convert](#before-you-convert) |
| string | `"Web"`, `"Web,App"`, `"%web%"`, `"-Unknown"`, `"EMPTY"`, `"-NULL"` | `==`, `in`, `like`, `!=`, `is null` | [String](#string) |
| number | `"5"`, `">0"`, `">=18 AND <=25"`, `"[5, 90)"`, `"NOT 66, 99"` | `==`, `>`, `>=`, `in`, `and` / `or` | [Number](#number) |
| yesno | `"yes"`, `"no"` | `f == true`, `f == false` | [Yes/no](#yesno) |
| date, time | `"7 days"`, `"last month"`, `"before 2018-01-01"`, `"2018-05-10 to 2018-05-13"` | `f matches @(…)`, `f < @…`, `f >= @…` | [Date and time](#date-and-time) |
| date, time | `"FY2018"`, `"2018-Q4"`, `"Monday"` | unsupported as a literal | [Date and time](#unsupported-date-expressions) |
| location | `"40 miles from 36.97, -122.03"` | unsupported | [Location](#location) |
| any | a value with `{{ _user_attributes['x'] }}` or other Liquid | see liquid.md | [User attributes and Liquid](#user-attributes-and-liquid) |

---

## Before you convert

Looker: [Filter expressions](https://docs.cloud.google.com/looker/docs/filter-expressions) ·
Holistics: [AQL Condition](https://docs.holistics.io/reference/aql/aql-condition),
[AQL operators](https://docs.holistics.io/reference/aql/operator)

1. **Find the filtered field first. Its type selects the grammar.** The same text means different things:
   `"2022"` is a year on a date field, the number 2022 on a number field, the text `2022` on a string field.
2. **A `dimension_group` timeframe resolves to another dimension.** A truncation timeframe (`created_date`,
   `created_month`, `created_time`) → the base date/datetime dimension, with the date grammar. Any other timeframe
   (`created_day_of_week`, `created_hour_of_day`) → the explicit dimension emitted for it, with the string or number
   grammar. See [dimension-group.md](dimension-group.md).
3. **An empty string means "is any value".** Emit no condition for that field.
4. **Several fields are combined with AND.** In a measure, each field becomes one argument of `where()`. Looker has
   no OR across fields.
5. **Escape `'` in text literals** for AQL.

Where the expression appears decides where the condition goes:

| The expression is the value of | The condition goes | See |
|---|---|---|
| measure `filters` | `\| where(…)` after the aggregate | [measure.md](measure.md#filters) |
| `always_filter`, `conditionally_filter`, explore `filters` | nowhere in AML: a comment and a report line for the dashboard filter | [explore.md](explore.md) |
| `access_filter` | a Dataset `permission` | [explore.md](explore.md) |
| `filters` of an `explore_source` | the derived table | [derived-tables.md](derived-tables.md) |
| `filters` of an `aggregate_table` query | nowhere: the aggregate table is dropped | [aggregate-awareness.md](aggregate-awareness.md) |
| `default_value` of a `filter` field | the default of the dashboard filter | [filter.md](filter.md#default_value) |

The `default_value` of a `parameter` is a plain value, not a filter expression. Do not parse it
(see [parameter.md](parameter.md#default_value)).

## String

Looker: [String](https://docs.cloud.google.com/looker/docs/filter-expressions#string) ·
Holistics: [Text operators](https://docs.holistics.io/reference/aql/operator#text)

1. Split the value on commas. `^,` is a literal comma, not a separator.
2. Trim each term. Unescape `^x` → `x`, but remember whether a `%` or `_` was escaped.
3. Convert each term:

| Term | Meaning | AQL |
|---|---|---|
| `FOO` | is FOO | `f == 'FOO'` |
| `FOO%`, `%FOO`, `%FOO%`, `F%OD`, `_UF` | pattern (unescaped `%` or `_`) | `f like 'FOO%'` |
| `EMPTY` | is NULL or empty | `f is null or f == ''` |
| `NULL` | is NULL | `f is null` |
| `-FOO` | is not FOO | `f != 'FOO'` |
| `-FOO%`, `-%FOO%` | does not match | `f not like 'FOO%'` |
| `-EMPTY` | is not NULL and not empty | `f is not null and f != ''` |
| `-NULL` | is not NULL | `f is not null` |

4. Combine the terms:
   - positive terms → OR. Several plain values → `f in ['A', 'B']`.
   - negative terms → AND. Several plain values → `f not in ['A', 'B']`.
   - positive and negative groups → AND.
5. If the value has **only** negative `-FOO` / `-FOO%` terms, add `f is null or` in front, so that NULL rows are kept
   (see Differences). With a positive term this is not needed: the positive term already excludes NULL.

```lookml
measure: web_count         { type: count  filters: [channel: "Web"] }
measure: web_or_app_count  { type: count  filters: [channel: "Web,App"] }
measure: not_unknown_count { type: count  filters: [channel: "-Unknown,-Rest of World"] }
measure: metrics_count     { type: count  filters: [channel: "metrics.%,-%.key"] }
measure: has_channel_count { type: count  filters: [channel: "-NULL"] }
measure: blank_count       { type: count  filters: [channel: "EMPTY"] }
measure: santa_cruz_count  { type: count  filters: [city: "Santa Cruz^, CA"] }
```
```aml
measure web_count {
  label: 'Web Count'
  type: 'number'
  definition: @aql count(events.id) | where(events.channel == 'Web');;
}
measure web_or_app_count {
  label: 'Web Or App Count'
  type: 'number'
  definition: @aql count(events.id) | where(events.channel in ['Web', 'App']);;
}
measure not_unknown_count {
  label: 'Not Unknown Count'
  type: 'number'
  definition: @aql count(events.id)
    | where(events.channel is null or events.channel not in ['Unknown', 'Rest of World']);;
}
measure metrics_count {
  label: 'Metrics Count'
  type: 'number'
  definition: @aql count(events.id)
    | where(events.channel like 'metrics.%' and events.channel not like '%.key');;
}
measure has_channel_count {
  label: 'Has Channel Count'
  type: 'number'
  definition: @aql count(events.id) | where(events.channel is not null);;
}
measure blank_count {
  label: 'Blank Count'
  type: 'number'
  definition: @aql count(events.id) | where(events.channel is null or events.channel == '');;
}
measure santa_cruz_count {
  label: 'Santa Cruz Count'
  type: 'number'
  definition: @aql count(events.id) | where(events.city == 'Santa Cruz, CA');;
}
```

Differences:

- **NULL under a negation.** Mark for review. Looker's page says only "is any value except FOO". The SQL that
  Looker generates for `-FOO` is known to keep NULL rows (`col <> 'FOO' OR col IS NULL`), which is why step 5 adds
  `f is null or`. This cannot be confirmed from the Looker docs.
- **Case sensitivity.** Looker follows the `case_sensitive` setting of the model or explore. AQL `==` and `like`
  follow the database. With `case_sensitive: no`, emit `f ilike '<pattern>'` for patterns and
  `lower(f) == '<lowercased value>'` for equality (see [model.md](model.md#case_sensitive)).
- **A term that looks like a number or a date** stays text on a string field: `"2022"` → `f == '2022'`.

Unsupported: a term with both an escaped wildcard (`^%`, `^_`) and an unescaped one needs a `LIKE` escape
character. Report it.

## Number

Looker: [Number](https://docs.cloud.google.com/looker/docs/filter-expressions#number) ·
Holistics: [Number operators](https://docs.holistics.io/reference/aql/operator#number)

| Expression | AQL |
|---|---|
| `5`, `=5` | `f == 5` |
| `NOT 5`, `<>5`, `!=5` | `f != 5` |
| `1, 3, 5, 7` | `f in [1, 3, 5, 7]` |
| `NOT 66, 99, 4` | `f not in [66, 99, 4]` |
| `>1`, `>=7`, `<=56` | `f > 1`, `f >= 7`, `f <= 56` |
| `>=5.5 AND <=10`, `5.5 to 10` | `f >= 5.5 and f <= 10` |
| `1 to`, `to 10` | `f >= 1`, `f <= 10` |
| `NOT 3 to 80.44` | `f < 3 or f > 80.44` |
| `>10 AND <=20 OR 90` | `(f > 10 and f <= 20) or f == 90` |
| `>1 AND <100, NOT 2` | `f > 1 and f < 100 and f != 2` |
| `(1, 7)`, `[5, 90]`, `(12, 20]`, `[5, 90)` | `(` → `>`, `[` → `>=`, `)` → `<`, `]` → `<=`. `[5, 90)` → `f >= 5 and f < 90` |
| `(500, inf)`, `(500,)`, `(-inf, 10]`, `(,10]` | one side only: `f > 500`, `f <= 10` |
| `[0,9],[20,29]` | `(f >= 0 and f <= 9) or (f >= 20 and f <= 29)` |
| `NOT (3,12)` | `f <= 3 or f >= 12` |
| `NULL` | `f is null` |
| `NOT NULL` | `f is not null` |

Precedence:

- `AND` binds tighter than `OR`. A comma between positive terms is an `OR`.
- Negated terms are combined with AND, with each other and with the positive terms.
- A leading `NOT` in a list without another `NOT` negates every term: `NOT 66, 99, 4` excludes all three.

```lookml
measure: positive_count { type: count  filters: [amount: ">0"] }
measure: range_or_count { type: count  filters: [amount: ">10 AND <=20 OR 90"] }
measure: interval_count { type: count  filters: [amount: "[5, 90)"] }
measure: not_list_count { type: count  filters: [amount: "NOT 66, 99, 4"] }
measure: no_amount_count { type: count  filters: [amount: "NULL"] }
```
```aml
measure positive_count {
  label: 'Positive Count'
  type: 'number'
  definition: @aql count(events.id) | where(events.amount > 0);;
}
measure range_or_count {
  label: 'Range Or Count'
  type: 'number'
  definition: @aql count(events.id)
    | where((events.amount > 10 and events.amount <= 20) or events.amount == 90);;
}
measure interval_count {
  label: 'Interval Count'
  type: 'number'
  definition: @aql count(events.id) | where(events.amount >= 5 and events.amount < 90);;
}
measure not_list_count {
  label: 'Not List Count'
  type: 'number'
  definition: @aql count(events.id)
    | where(events.amount is null or events.amount not in [66, 99, 4]);;
}
measure no_amount_count {
  label: 'No Amount Count'
  type: 'number'
  definition: @aql count(events.id) | where(events.amount is null);;
}
```

Differences:

- **NULL under a negation.** As for strings: when the value has only negated terms (`NOT 5`, `NOT 66, 99, 4`), add
  `f is null or` in front and mark for review.
- **`NOT (3,12)`.** Mark for review. Looker's page describes it as "x < 3 and x > 12", which no number satisfies.
  The table gives the complement of the interval instead.

Unsupported: a contradictory expression such as `NOT >1, 2, <100`, which Looker turns into `IS NULL`. Report it.

## Yes/no

Looker: [Boolean](https://docs.cloud.google.com/looker/docs/filter-expressions#boolean) ·
Holistics: [Truefalse operators](https://docs.holistics.io/reference/aql/operator#truefalse)

| Expression | AQL |
|---|---|
| `yes`, `Yes`, `TRUE`, `true` | `f == true` |
| `no`, `No`, `FALSE`, `false` | `f == false` |

```lookml
measure: paid_count   { type: count  filters: [is_paid: "yes"] }
measure: unpaid_count { type: count  filters: [is_paid: "no"] }
```
```aml
measure paid_count {
  label: 'Paid Count'
  type: 'number'
  definition: @aql count(events.id) | where(events.is_paid == true);;
}
measure unpaid_count {
  label: 'Unpaid Count'
  type: 'number'
  definition: @aql count(events.id) | where(events.is_paid == false);;
}
```

In Looker, "no" also matches rows where the condition is NULL. `f == false` does the same only because a converted
`yesno` dimension never returns NULL (see [dimension.md](dimension.md#yesno)). If the dimension was converted
without that wrapper, emit `f is null or f == false` for "no".

Do not write `not f`: it is a syntax error in AQL.

## Date and time

Looker: [Date and time](https://docs.cloud.google.com/looker/docs/filter-expressions#date_and_time) ·
Holistics: [Datetime literals](https://docs.holistics.io/reference/aql/datetime-literal),
[Natural Time Expression](https://docs.holistics.io/docs/datetimes/relative-dates),
[Complete and incomplete time periods](https://docs.holistics.io/docs/filters/date-filters#complete-and-incomplete-time-periods)

`f` is the base date or datetime dimension (see [Before you convert](#before-you-convert)). Replace the slashes of
a Looker date with dashes: `2018/05/10` → `2018-05-10`.

**The two tools count periods differently.** A bare Looker interval (`7 days`) includes the current, incomplete
period: today and the 6 days before. Holistics `last 7 days` covers complete periods only: the 7 days before today.
Never convert `7 days` to `last 7 days`.

Holistics writes a range as `start - end` or `start to end` (the end period is included), or as `start until end`
(the end is excluded).

### Relative periods

| Looker | Meaning | AQL |
|---|---|---|
| `today`, `yesterday`, `tomorrow` | that day | `f matches @(today)` |
| `this week`, `this month`, `this quarter`, `this year` | the whole current period | `f matches @(this month)` |
| `1 day`, `1 month` | the current period | `f matches @(today)`, `f matches @(this month)` |
| `N days` (N > 1) | today and the N−1 days before | `f matches @(<N−1> days ago - today)` |
| `N weeks`, `N months`, `N hours`, … (N > 1) | the current period and the N−1 before | `f matches @(<N−1> months ago - this month)` |
| `N days ago` | that single day | `f matches @(N days ago)` |
| `N days ago for N days` | the N complete days before today | `f matches @(last N days)` |
| `last week`, `last month`, `last year` | the previous complete period | `f matches @(last 1 month)` |
| `N <unit> ago for M <unit>` (M < N) | M periods, starting N periods ago | `f matches @(N days ago until <N−M> days ago)`; when N−M is 0, end with `this <unit> begin` |
| `last N days` | see Differences | as `N days`; mark for review |
| `N days from now` | that single day | `f matches @(N days from now)` |
| `next week`, `next month` | the next complete period | `f matches @(next 1 week)` |
| `this year to second` | from the start of the year until now | `f matches @(this year begin until now)` |
| `this week to day` | from the start of the week until the start of today | `f matches @(this week begin until today)` |

### Absolute dates

| Looker | Meaning | AQL |
|---|---|---|
| `2018`, `2018/05`, `2018/05/29` | the whole year, month or day | `f matches @2018`, `f matches @2018-05`, `f matches @2018-05-29` |
| `before 2018-01-01` | strictly before the start | `f < @2018-01-01` |
| `before N days ago` | strictly before that day | `f < @(N days ago)` |
| `after 2018-10-05` | on or after the start | `f >= @2018-10-05` |
| `2018-05-10 to 2018-05-13` | start included, end excluded | `f matches @(2018-05-10 until 2018-05-13)` |
| `2018-05-10 for 3 days` | 3 days from the start | compute the end: `f matches @(2018-05-10 until 2018-05-13)` |

### Combinations and NULL

| Looker | AQL |
|---|---|
| `A, B` | `f matches @(…) or f matches @(…)` |
| `NULL` | `f is null` |
| `NOT NULL` | `f is not null` |

```lookml
measure: last_7_days_count   { type: count  filters: [created_date: "7 days"] }
measure: prev_7_days_count   { type: count  filters: [created_date: "7 days ago for 7 days"] }
measure: last_6_months_count { type: count  filters: [created_month: "6 months"] }
measure: last_month_count    { type: count  filters: [created_date: "last month"] }
measure: before_cutoff_count { type: count  filters: [created_date: "before 2018-01-01"] }
measure: since_cutoff_count  { type: count  filters: [created_date: "after 2018-10-05"] }
measure: window_count        { type: count  filters: [created_date: "2018-05-10 to 2018-05-13"] }
```
```aml
measure last_7_days_count {
  label: 'Last 7 Days Count'
  type: 'number'
  definition: @aql count(events.id) | where(events.created matches @(6 days ago - today));;
}
measure prev_7_days_count {
  label: 'Prev 7 Days Count'
  type: 'number'
  definition: @aql count(events.id) | where(events.created matches @(last 7 days));;
}
measure last_6_months_count {
  label: 'Last 6 Months Count'
  type: 'number'
  definition: @aql count(events.id) | where(events.created matches @(5 months ago - this month));;
}
measure last_month_count {
  label: 'Last Month Count'
  type: 'number'
  definition: @aql count(events.id) | where(events.created matches @(last 1 month));;
}
measure before_cutoff_count {
  label: 'Before Cutoff Count'
  type: 'number'
  definition: @aql count(events.id) | where(events.created < @2018-01-01);;
}
measure since_cutoff_count {
  label: 'Since Cutoff Count'
  type: 'number'
  definition: @aql count(events.id) | where(events.created >= @2018-10-05);;
}
measure window_count {
  label: 'Window Count'
  type: 'number'
  definition: @aql count(events.id) | where(events.created matches @(2018-05-10 until 2018-05-13));;
}
```

**The validator does not check the meaning of a date literal.** It rejects nonsense, but it also accepts phrasings
that the Holistics docs do not list, such as `@(7 days ago for 7 days)` and `@(2018-Q4)`. Use only the forms in the
tables above.

Differences:

- **`last N days`.** Mark for review. Looker's page documents `last 3 days` as "2 days ago through the end of the
  current day", so it includes today, while `last week` and `last month` mean the previous complete period.
- **`before` and `after`.** Looker's `after` includes its start and `before` excludes it. Use `>=` and `<` with a
  day literal only. With a period literal (`f > @(yesterday)`), AQL compares against the whole period.
- **Range ends.** Mark `start - end` ranges whose two ends have different units for review; the Holistics docs say
  only that the end point is included.
- **Weeks.** A Looker week starts on Monday unless `week_start_day` says otherwise. Holistics uses the workspace's
  [Week Start Day](https://docs.holistics.io/docs/datetimes/week-start-day) setting.
- **Time zone.** "Today" follows the query time zone in Looker, and the workspace or dashboard
  [time zone](https://docs.holistics.io/docs/datetimes/timezones) in Holistics.

### Unsupported date expressions

| Looker | What to do |
|---|---|
| `FY2018`, `FY2018-Q1`, `this fiscal year`, `2 fiscal quarters` | Unsupported. Filter on the fiscal dimensions (see [dimension-group.md](dimension-group.md)), or compute the dates and write an absolute range. |
| `2018-Q4` | Unsupported as a literal. Write the range: `f matches @(2018-10-01 until 2019-01-01)`. |
| `Monday`, `after Monday`, `before Monday` (most recent weekday) | Unsupported. Report it. |

## Location

Looker: [Location](https://docs.cloud.google.com/looker/docs/filter-expressions#location)

Unsupported. AQL has no distance or bounding-box operator, so `40 miles from 36.97, -122.03` and
`inside box from 72.33, -173.14 to 14.39, -61.70` have no conversion. Report them.

`NULL` and `-NULL` on a location → a null check on both the latitude and the longitude dimension that replace the
location (see [dimension.md](dimension.md)).

## User attributes and Liquid

Looker: [User attribute values](https://docs.cloud.google.com/looker/docs/filter-expressions#user_attribute_values) ·
Holistics: [User attributes and variables](https://docs.holistics.io/reference/aml/user-attributes-and-variables)

A value such as `"{{ _user_attributes['region'] }}"` is not parsed with the grammars above. Resolve the Liquid
first (see [liquid.md](liquid.md)); a user attribute becomes `H.current_user.<attribute>` or a Dataset
`permission`. Then apply this page to the text that is left.

Unsupported: Liquid that produces part of the expression at run time (an operator, a unit, a list of values).
Report it.
