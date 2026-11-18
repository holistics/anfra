# References inside LookML `sql` → AML

How to rewrite the substitution operators that appear inside LookML SQL blocks (`sql`, `sql_distinct_key`,
`sql_start`, `sql_end`, `sql_table_name`, `derived_table: sql`). The sections follow Looker's
[Incorporating SQL and referring to LookML objects](https://docs.cloud.google.com/looker/docs/sql-and-referring-to-lookml)
page.

The SQL text is copied unchanged. Only the references are rewritten:

```lookml
view: users {
  dimension: first_name { type: string  sql: ${TABLE}.first_name ;; }
  dimension: last_name  { type: string  sql: ${TABLE}.last_name ;; }
  dimension: full_name  { type: string  sql: CONCAT(${first_name}, ' ', ${users.last_name}) ;; }
}
```
```aml
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
```

## Quick reference

| LookML | AML | Section |
|---|---|---|
| SQL text around the references | unchanged, inside `@sql … ;;` | [SQL text](#sql-text) |
| `${TABLE}.col` | `{{ #SOURCE.col }}` | [`${TABLE}`](#table) |
| `${field}`, `${own_view.field}` | `{{ field }}` in `@sql`, only if the target is an `@sql` dimension; `<model>.<field>` in `@aql` | [`${field}`](#field) |
| `${field::type}` | the same reference, suffix dropped | [`${field}`](#field) |
| `${created_date}`, `${created_raw}` (dimension-group timeframe) | the dimension emitted for that timeframe | [Timeframes](#dimension-group-timeframes) |
| `${other_view.field}` | the field moves to the Dataset, written in AQL | [`${view.field}`](#viewfield) |
| `${view.SQL_TABLE_NAME}` | `{{ #view }}` in a query model | [`SQL_TABLE_NAME`](#viewsql_table_name) |
| `@{constant}` | `${constant}` | [`@{constant}`](#constant) |
| Liquid (`{% … %}`, `{{ … }}`) | see [liquid.md](liquid.md), [parameter.md](parameter.md), [filter.md](filter.md) | — |

The same reference has two spellings in AML, depending on the language of the definition it ends up in:

| | in `@sql` | in `@aql` |
|---|---|---|
| a column of the model's table | `{{ #SOURCE.col }}` | not available: make it a dimension first |
| an `@sql` dimension of the same model | `{{ dim }}` | `<model>.<dim>` |
| an `@aql` dimension of the same model | not available | `<model>.<dim>` |
| a measure of the same model | not available: measures are AQL | `<model>.<measure>` |
| a field of another model | not available | `<other_model>.<field>`, in a Dataset only |
| a Dataset dimension | not available | `<its model>.<name>` |
| a Dataset metric | not available | `<name>`, without a model |

**`@sql` can read only `@sql`.** A `{{ field }}` reference inside `@sql` must point at a dimension whose own
definition is `@sql`. A reference to an `@aql` field is invalid: a query that uses the field fails with E210
("Cannot reference aql field from sql"). The validator reports it only when an AQL expression (a measure, a
metric) uses the `@sql` field, so a model can pass validation and still be wrong. The other direction is
fine: `@aql` can read `@sql` and `@aql` fields alike.

---

## SQL text

Looker: [SQL dialect, SQL blocks](https://docs.cloud.google.com/looker/docs/sql-and-referring-to-lookml) ·
Holistics: [AML Field — definition](https://docs.holistics.io/reference/aml/field)

Looker pastes a SQL block into the query it sends to the database. Holistics does the same with an `@sql`
definition. So:

1. **Copy the SQL unchanged.** No dialect translation, no re-formatting, no function mapping. This assumes the
   Holistics data source is the same database as the Looker connection.
2. **Rewrite only the references**, by the sections below.
3. **Translate SQL to AQL only where AML requires AQL:**
   - a measure ([measure.md](measure.md));
   - a field that uses two views ([`${view.field}`](#viewfield));
   - a field that references an `@aql` field ([`${field}`](#field)).

   In all cases, first move the SQL that uses a single view into a hidden helper dimension, unchanged. What is
   left to translate is then as small as possible.
4. Trim the block. A `--` comment on the last line would swallow the `;;` terminator: move it to an AML `//` comment
   above the field.
5. Escape a literal `${` that is left after the rewrite as `\${`. After conversion, every `${…}` in an AML file is
   AML [string interpolation](https://docs.holistics.io/reference/aml/string-interpolation).

Copy these patterns as they are, and list them in the conversion report for review:

| Pattern | Why it needs a look |
|---|---|
| The view's own name used as a table alias: `orders.status` written in view `orders` | Looker aliases each view by its name in the generated SQL. Holistics does not document its alias. When the alias is the field's own view, rewrite `orders.status` → `{{ #SOURCE.status }}`. |
| A bare column name: `sql: revenue ;;` | Works while the name is unambiguous in the final query. |
| A correlated subquery: `(SELECT MAX(x) FROM t WHERE t.user_id = users.id)` | The link to the outer row usually goes through the view's alias (first row). |
| A literal `{{` or `{%` that is not Looker Liquid | Collides with the template syntax of Holistics. |

The validator does not read `@sql` text. A typo in SQL, or in a `{{ … }}` reference inside it, is not reported.

## `${TABLE}`

Looker: [Substitution operator](https://docs.cloud.google.com/looker/docs/sql-and-referring-to-lookml),
[`sql` (for field)](https://docs.cloud.google.com/looker/docs/reference/param-field-sql) ·
Holistics: [AML Field — definition](https://docs.holistics.io/reference/aml/field)

`${TABLE}` is the table of the view that the field belongs to. Rewrite it together with what follows it:

| LookML | AML, inside `@sql` |
|---|---|
| `${TABLE}.col` | `{{ #SOURCE.col }}` |
| `${TABLE}."Col Name"`, `` ${TABLE}.`col` ``, `${TABLE}.[col]` | `{{ #SOURCE."Col Name" }}`: the quoted identifier goes inside the braces, quotes included |
| `${TABLE}.rec.leaf` (a field of a record or struct column) | `{{ #SOURCE.rec }}.leaf`: only the first identifier is the column |
| `${TABLE}.@{const}` | `{{ #SOURCE.${const} }}` (see [`@{constant}`](#constant)) |
| no `sql` at all | `{{ #SOURCE.<field name> }}`: Looker assumes a column named like the field |
| bare `${TABLE}`, `${TABLE}.*` | unsupported: `#SOURCE` always needs a column. Report it. These appear in views over `UNNEST(…)`; see [explore.md](explore.md). |

```lookml
dimension: country { type: string  sql: ${TABLE}."Country Code" ;; }
dimension: city    { type: string  sql: ${TABLE}.address.city ;; }
```
```aml
dimension country {
  label: 'Country'
  type: 'text'
  definition: @sql {{ #SOURCE."Country Code" }};;
}
dimension city {
  label: 'City'
  type: 'text'
  definition: @sql {{ #SOURCE.address }}.city;;
}
```

`{{ #SOURCE.col }}` is valid only in the `@sql` definition of a model field. An `@aql` definition cannot name a
column: emit a dimension for the column and reference the dimension (see [measure.md](measure.md#sql)).

Mark quoted identifiers and record paths for review. They compile, but they have not been run against a database.

## `${field}`

Looker: [Substitution operator](https://docs.cloud.google.com/looker/docs/sql-and-referring-to-lookml),
[Scoping and naming](https://docs.cloud.google.com/looker/docs/sql-and-referring-to-lookml) ·
Holistics: [AML Field — definition](https://docs.holistics.io/reference/aml/field)

A reference to a dimension or measure of the same view. Resolve the name against the view after `extends` and
refinements are applied ([extends-and-refinements.md](extends-and-refinements.md)), then rewrite it:

| Reference in | points at | AML |
|---|---|---|
| a dimension, dimension group or filter `sql` | a dimension defined with `@sql` | `{{ dim }}` |
| a dimension, dimension group or filter `sql` | a dimension defined with `@aql` | `<model>.<dim>`, and the referencing field is written in `@aql` (see below) |
| a measure `sql` or `sql_distinct_key` | a dimension | `<model>.<dim>` (measures are written in AQL) |
| a measure `sql` (type `number`, `string`, `yesno`, `date`) | a measure | `<model>.<measure>` |

```lookml
view: users {
  dimension: age      { type: number  sql: ${TABLE}.age ;; }
  dimension: is_adult { type: yesno   sql: ${age} >= 18 ;; }
  measure: average_age { type: average  sql: ${age} ;; }
}
```
```aml
dimension age {
  label: 'Age'
  type: 'number'
  definition: @sql {{ #SOURCE.age }};;
}
dimension is_adult {
  label: 'Is Adult'
  type: 'truefalse'
  definition: @sql {{ age }} >= 18;;
}
measure average_age {
  label: 'Average Age'
  type: 'number'
  definition: @aql average(users.age);;
}
```

(`yesno` dimensions get a NULL-safe wrapper; see [dimension.md](dimension.md#yesno).)

Special cases, in this order:

1. **`${own_view.field}`** is the same as `${field}`. Strip the view name first. Otherwise the reference is
   mistaken for a reference to another view.
2. **`${field::type}`** (`::string`, `::number`, `::date`, `::datetime`, `::boolean`): resolve the field and drop the
   suffix. On a field that is not a date or time, also report it.
3. **The name is a dimension-group timeframe** (`${created_date}`): see
   [Dimension-group timeframes](#dimension-group-timeframes).
4. **The target is a `parameter` or `filter` field**: not a value. See [parameter.md](parameter.md),
   [filter.md](filter.md).
5. **The target is a field that moved to the Dataset** (it references another view): the referencing field moves
   too. See [`${view.field}`](#viewfield).
6. **The name is not found** after inheritance is resolved: the LookML is broken, or a parent view is missing. Keep
   the field, emit `{{ name }}` unchanged and report it. Validation does not catch it.

### A reference to an `@aql` dimension

Several conversions produce `@aql` dimensions: the timeframe aliases and explicit timeframes of a dimension group,
`duration` fields, and others (see [dimension.md](dimension.md), [dimension-group.md](dimension-group.md)). An
`@sql` definition cannot reference them. So convert the referenced dimension first, look at the language of its
definition, and when it is `@aql`:

1. Write the referencing field in `@aql`. Translate its SQL by the table in
   [Step 4](#step-4-write-the-definition-in-aql).
2. Keep SQL that AQL cannot express in a hidden `@sql` helper dimension (`<field>__<suffix>`), as in Step 4. The
   helper may reference `@sql` dimensions only. If the untranslatable part itself needs the `@aql` field, use an
   SQL passthrough function (`sql_number('FN', …)` and its siblings); if that does not fit either, report the field
   for manual work.
3. The rule chains. The field is now `@aql`, so every field that references it must be `@aql` too. Convert fields
   in dependency order.

```lookml
dimension_group: created { type: time  timeframes: [raw, year]  sql: ${TABLE}.created_at ;; }
dimension: is_created_this_year { type: yesno  sql: ${created_year} = EXTRACT(YEAR FROM CURRENT_DATE) ;; }
```
```aml
dimension created {
  label: 'Created'
  type: 'datetime'
  definition: @sql {{ #SOURCE.created_at }};;
}
dimension created_year {
  label: 'Created Year'
  type: 'number'
  hidden: true
  definition: @aql date_part('year', orders.created);;
}
dimension is_created_this_year__current_year {
  label: 'Is Created This Year Current Year'
  type: 'number'
  hidden: true
  definition: @sql EXTRACT(YEAR FROM CURRENT_DATE);;
}
dimension is_created_this_year {
  label: 'Is Created This Year'
  type: 'truefalse'
  definition: @aql orders.created_year == orders.is_created_this_year__current_year;;
}
```

Difference: the translated part follows AQL semantics, not those of the database (see
[Differences](#differences)). Mark the field for review.

Looker replaces `${dim}` with the SQL of that dimension, and Holistics resolves `{{ dim }}` to the definition of
that dimension. The result is the same only if the referenced dimension was converted faithfully, so convert `tier`,
`case` and `yesno` dimensions by [dimension.md](dimension.md) before relying on references to them.

## Dimension-group timeframes

Looker: [`dimension_group`](https://docs.cloud.google.com/looker/docs/reference/param-field-dimension-group),
[LookML field type references](https://docs.cloud.google.com/looker/docs/sql-and-referring-to-lookml)

A dimension group generates one field per timeframe, named `<group>_<timeframe>`. In AML a `type: time` group is
one base dimension named after the group, plus explicit dimensions for some timeframes
(see [dimension-group.md](dimension-group.md)).

1. **Recognise.** A name that is not a field of the view, and that splits into `<group>_<timeframe>` where `<group>`
   is a dimension group of the view. Match the longest group name: with groups `created` and `created_at`,
   `${created_at_date}` belongs to `created_at`.
2. **Rewrite** to the dimension that
   [dimension-group.md](dimension-group.md#referencing-the-generated-fields) gives for that timeframe:

   | Reference | AML dimension |
   |---|---|
   | `${g_raw}`; `${g_time}` on a `datetime` base; `${g_date}` on a `date` base | the base `g` |
   | a timeframe that has an explicit dimension (`${g_day_of_week}`), a duration field (`${days_g}`) | the same name |
   | any other truncation timeframe (`${g_date}` on a `datetime` base, `${g_month}`) | the hidden alias dimension `g_<timeframe>` defined in dimension-group.md |
   | `${g}`, the group name alone | `g_yesno` if the group has the `yesno` timeframe; otherwise an error in Looker: report it, do not guess a timeframe |

3. Write it as for any [`${field}`](#field): `{{ … }}` in `@sql` when the dimension is `@sql` (the base);
   `<model>.<name>` in `@aql`. The alias dimensions, explicit timeframes and duration fields are `@aql`, so a field
   that references one is written in `@aql` (see
   [A reference to an `@aql` dimension](#a-reference-to-an-aql-dimension)).

```lookml
dimension_group: created { type: time  timeframes: [raw, date]  sql: ${TABLE}.created_at ;; }
dimension_group: shipped { type: time  timeframes: [raw, date]  sql: ${TABLE}.shipped_at ;; }
dimension: days_to_ship  { type: number  sql: DATEDIFF(day, ${created_raw}, ${shipped_raw}) ;; }
```
```aml
dimension created {
  label: 'Created'
  type: 'datetime'
  definition: @sql {{ #SOURCE.created_at }};;
}
dimension shipped {
  label: 'Shipped'
  type: 'datetime'
  definition: @sql {{ #SOURCE.shipped_at }};;
}
dimension days_to_ship {
  label: 'Days To Ship'
  type: 'number'
  definition: @sql DATEDIFF(day, {{ created }}, {{ shipped }});;
}
```

Difference: `_raw` is the column itself, so the rewrite is exact. For every other timeframe, Looker substitutes the
SQL it generates for that timeframe (converted to the query time zone and, for most timeframes, formatted as a
string), and the surrounding SQL was written against that. Report every non-`raw` timeframe reference that sits
inside a larger expression.

## `${view.field}`

Looker: [Scoping and naming](https://docs.cloud.google.com/looker/docs/sql-and-referring-to-lookml),
[Error: Unknown or inaccessible field](https://docs.cloud.google.com/looker/docs/best-practices/error-unknown-or-inaccessible-field) ·
Holistics: [Dataset Fields](https://docs.holistics.io/reference/aml/dataset-field),
[AML Metric](https://docs.holistics.io/reference/aml/metric),
[Cross-model queries](https://docs.holistics.io/as-code/aql/learn/cross-model)

A field of view `A` whose SQL names `${X.f}`, a field of another view, only works in explores that join `A` and
`X`. An AML model sees only its own table, so such a field cannot stay in `Model A`. It moves to each Dataset that
contains both models, and its definition is written in AQL.

This section covers field SQL. The same syntax in `sql_on` and `sql_always_where` is covered by
[explore.md](explore.md).

```lookml
view: orders {
  dimension: amount { type: number  sql: ${TABLE}.amount ;; }
  measure: count        { type: count }
  measure: total_amount { type: sum  sql: ${amount} ;; }

  dimension: customer_email { type: string  sql: COALESCE(${users.email}, ${TABLE}.guest_email) ;; }
  measure: amount_per_user  { type: number  sql: ${total_amount} / NULLIF(${users.count}, 0) ;; }
  measure: total_customer_age { type: sum  sql: ${users.age} ;; }
  measure: distinct_customer_emails { type: count_distinct  sql: ${customer_email} ;; }
}

explore: orders {
  join: users {
    relationship: many_to_one
    sql_on: ${orders.user_id} = ${users.id} ;;
  }
}
```

`Model orders` keeps the fields that use only `orders`, plus a hidden helper for `${TABLE}.guest_email`:

```aml
dimension customer_email__guest_email {
  label: 'Customer Email (guest_email)'
  type: 'text'
  hidden: true
  definition: @sql {{ #SOURCE.guest_email }};;
}
measure count {
  label: 'Count'
  type: 'number'
  definition: @aql count(orders.id);;
}
measure total_amount {
  label: 'Total Amount'
  type: 'number'
  definition: @aql sum(orders.amount);;
}
```

The four other fields go to the Dataset:

```aml
Dataset orders_dataset {
  label: 'Orders'
  data_source_name: 'demodb'
  models: [orders, users]
  relationships: [
    relationship(orders.user_id > users.id, true)
  ]

  dimension customer_email {
    model: orders
    label: 'Customer Email'
    type: 'text'
    definition: @aql coalesce(users.email, orders.customer_email__guest_email);;
  }

  metric amount_per_user {
    label: 'Amount Per User'
    type: 'number'
    definition: @aql safe_divide(orders.total_amount, users.count);;
  }
  metric total_customer_age {
    label: 'Total Customer Age'
    type: 'number'
    definition: @aql sum(orders, users.age);;
  }
  metric distinct_customer_emails {
    label: 'Distinct Customer Emails'
    type: 'number'
    definition: @aql count_distinct(orders.customer_email);;
  }
}
```

`distinct_customer_emails` has no reference to another view of its own. It moves because `customer_email` moved.

### Step 1: find the fields that move

A field of view `A` moves when:

- its `sql`, `sql_distinct_key`, `sql_start` or `sql_end` contains `${X.f}` with `X` ≠ `A` (strip
  [`${own_view.field}`](#field) first), or its `filters` name a field of another view; or
- it references, through `${field}`, a field of `A` that moves.

Repeat the second rule until nothing changes. A moved field is not emitted in `Model A`.

### Step 2: find the model behind `X`

`X` is the name the view has **in the explore**: the join name. It differs from the view name when the join uses
`from`. Use the model that [explore.md](explore.md#from) gives that join in that Dataset.

```lookml
# view: tickets
dimension: owner_name { type: string  sql: ${owner.full_name} ;; }

explore: tickets {
  join: owner {
    from: users
    relationship: many_to_one
    sql_on: ${tickets.owner_id} = ${owner.id} ;;
  }
}
```
```aml
Dataset tickets_dataset {
  label: 'Tickets'
  data_source_name: 'demodb'
  models: [tickets, owner]
  relationships: [
    relationship(tickets.owner_id > owner.id, true)
  ]

  dimension owner_name {
    model: tickets
    label: 'Owner Name'
    type: 'text'
    definition: @aql owner.full_name;;
  }
}
```

If the same reference resolves to different models in different explores, write one definition per Dataset.

### Step 3: emit the field in each Dataset

- A dimension, or a timeframe dimension of a dimension group → `dimension <name> { model: …  … }` in the Dataset.
- A measure → `metric <name> { … }` in the Dataset.

Keep the name, and give it the `label`, `type`, `description`, `format` and `hidden` it would have had in the
model.

Repeat the block in **every Dataset that contains `A` and every model the field uses**. In a Dataset that lacks one
of them, omit the field. This matches Looker, where the field is unusable in an explore without the join. List the
omitted fields in the conversion report.

All metrics of a Dataset share one namespace without a model prefix. If two moved measures of different views have
the same name in one Dataset, name them `<view>_<name>`.

### Step 4: write the definition in AQL

An `@aql` definition cannot contain SQL text, so the SQL around the references is translated. Rewrite the
references first:

| LookML | AQL |
|---|---|
| `${X.f}` | `<model of X>.<f>` |
| `${X.m}` where `m` is a moved measure | `m`, the bare metric name |
| `${f}` (a field of `A`) | `A.<f>`; the bare name if `f` is a moved measure |
| `${TABLE}.col` | the dimension of `A` that has exactly this SQL; otherwise a new hidden helper dimension `<field>__<col>` in `Model A` |

Then keep as much SQL as possible untranslated: **a sub-expression that uses only one view moves unchanged, as
`@sql`, into a hidden helper dimension of that model** (`hidden: true`, name `<field>__<suffix>`). Only the part
that really combines two models is translated:

| SQL | AQL |
|---|---|
| a lone reference: `sql: ${users.email} ;;` | `users.email` |
| `+ - * /`, parentheses, number and string literals | the same |
| `=`, `<>`, `!=`, `<`, `>`, `<=`, `>=` | `==`, `!=`, `!=`, `<`, `>`, `<=`, `>=` |
| `AND`, `OR`, `NOT`, `IS NULL`, `IS NOT NULL` | `and`, `or`, `not`, `is null`, `is not null` |
| `COALESCE(a, b, …)`, `NULLIF(a, b)` | `coalesce(a, b, …)`, `nullif(a, b)` |
| `a / NULLIF(b, 0)` | `safe_divide(a, b)` |
| `CASE WHEN c THEN x … ELSE y END` | `case(when: c, then: x, …, else: y)` |
| `CONCAT(a, b)`, `a \|\| b` | `concat(a, b)` |
| `CAST(a AS …)`, `a::…` | `cast(a, '<type>')` |
| `DATEDIFF(day, a, b)` and other dialect date arithmetic | `date_diff('day', a, b)` |
| any other function call `FN(a, b)` | the AQL function if one exists; otherwise `sql_text('FN', a, b)`, `sql_number(…)`, `sql_date(…)`, `sql_datetime(…)` or `sql_truefalse(…)`, by the type of the result |

AQL functions: [null and zero](https://docs.holistics.io/reference/aql/null-and-zero-functions),
[logical](https://docs.holistics.io/reference/aql/logical-functions),
[text](https://docs.holistics.io/reference/aql/text-functions),
[time intelligence](https://docs.holistics.io/reference/aql/time-intelligence-functions),
[SQL passthrough](https://docs.holistics.io/reference/aql/sql-passthrough-functions),
[operators](https://docs.holistics.io/reference/aql/operator).

The rows down to `CASE` are mechanical. For the rest, check the argument order and the types by hand, and mark the
field for review.

```lookml
# view: orders
dimension: order_size {
  type: string
  sql: CASE WHEN ${users.tier} = 'vip' THEN 'VIP' WHEN ${amount} >= 100 THEN 'Large' ELSE 'Standard' END ;;
}
dimension: days_since_signup {
  type: number
  sql: DATEDIFF(day, ${users.signup_date}, ${created_raw}) ;;
}
dimension: shipping_label {
  type: string
  sql: CONCAT(UPPER(TRIM(${TABLE}.ship_city)), ' / ', ${users.country}) ;;
}
```

`UPPER(TRIM(${TABLE}.ship_city))` uses only `orders`, so it stays SQL, in a helper of `Model orders`:

```aml
dimension shipping_label__local {
  label: 'Shipping Label (local)'
  type: 'text'
  hidden: true
  definition: @sql UPPER(TRIM({{ #SOURCE.ship_city }}));;
}
```

In the Dataset:

```aml
dimension order_size {
  model: orders
  label: 'Order Size'
  type: 'text'
  definition: @aql case(
    when: users.tier == 'vip', then: 'VIP',
    when: orders.amount >= 100, then: 'Large',
    else: 'Standard'
  );;
}
dimension days_since_signup {
  model: orders
  label: 'Days Since Signup'
  type: 'number'
  definition: @aql date_diff('day', users.signup_date, orders.created);;
}
dimension shipping_label {
  model: orders
  label: 'Shipping Label'
  type: 'text'
  definition: @aql concat(orders.shipping_label__local, ' / ', users.country);;
}
```

### Step 5: measures

Convert the measure by [measure.md](measure.md), with these additions:

- **An aggregate over a dimension of another view names the measure's own model as the table to aggregate.**
  `type: sum  sql: ${users.age}` in view `orders` → `sum(orders, users.age)`, not `sum(users.age)`. The first form
  adds the user's age once per order, which is what Looker computes. The second adds each user's age once.
  See [aggregator functions](https://docs.holistics.io/reference/aql/aggregator-functions).
- **A non-aggregate measure** (`type: number`) is an expression over measures:
  `${total_amount} / NULLIF(${users.count}, 0)` → `safe_divide(orders.total_amount, users.count)`.
- **A filter on a field of another view** → `| where(…)`:
  `filters: [users.tier: "vip"]` on a `count` → `count(orders.id) | where(users.tier == 'vip')`
  (see [measure.md](measure.md#filters)).
- **A Dataset metric is AQL only.** There is no `@sql` fallback.
- **An aggregate in the other direction**, a measure of `users` over `${orders.amount}`: AQL rejects
  `sum(users, orders.amount)` with a fan-out error. Emit `sum(orders.amount)` and report the field.

### Step 6: choose `model` for a Dataset dimension

`model` is required. The dimension is listed under that model in the field picker and is referenced as
`<model>.<name>`.

AQL reads another model only from the "many" side to the "one" side. So `model` must be a model from which every
other model in the definition is reached through many-to-one or one-to-one relationships. Otherwise validation
fails with `E104 Fan-out error`.

| Situation | `model` |
|---|---|
| `A` qualifies: a field of `orders` reads `${users.email}` (the usual case) | `A` |
| `A` does not qualify, but a referenced model `M` does: a field of `users` reads `${orders.status}` | `M`. Report it: the field appears under another model, and references to it become `M.<name>`. |
| no model qualifies (many-to-many, or two unrelated "many" sides) | unsupported as a dimension. Report it for manual work. |

Take the relationship direction from the Dataset ([explore.md](explore.md)). If it differs between Datasets, decide
per Dataset.

### Step 7: rewrite references to the moved field

Everywhere else (other fields, drill fields, `fields` lists of the explore):

- a moved dimension → `<model chosen in step 6>.<name>`
- a moved measure → `<name>`, without a model

### Differences

Mark every moved field for review.

- **The field exists only in Datasets.** It cannot be used in the SQL of a query model.
- **Translated SQL follows AQL semantics**, not those of the database: NULL handling in `coalesce` and comparisons,
  division, text comparison.
- **Join type.** In Looker, a cross-view dimension follows the join `type` of the explore (rows dropped by an inner
  join, NULLs from an outer join). In AQL the relationship behaves like a left join from the dimension's `model`.
  See [explore.md](explore.md#type).
- **Direction.** Looker evaluates the SQL on the joined rows, whatever the direction of the join. AQL does not
  (step 6).

## `${view.SQL_TABLE_NAME}`

Looker: [Substitution operator](https://docs.cloud.google.com/looker/docs/sql-and-referring-to-lookml),
[`sql` (for derived table)](https://docs.cloud.google.com/looker/docs/reference/param-view-sql-for-derived-table) ·
Holistics: [Query Model](https://docs.holistics.io/reference/aml/query-model#query-syntax)

`${v.SQL_TABLE_NAME}` is the table name of view `v`, or of its derived table. The conversion depends on where it
occurs:

| Where | Conversion |
|---|---|
| `derived_table: sql` | `{{ #v }}` in the `query` of the query model, and `v` in its `models: […]` list. An alias moves inside the braces: `${v.SQL_TABLE_NAME} AS s` → `{{ #v AS s }}`. Full rules: [derived-tables.md](derived-tables.md#references-to-other-views). |
| `sql_trigger_value` | dropped with the trigger (see [derived-tables.md](derived-tables.md)) |
| `sql_table_name` | a view over another view's table (see [view.md](view.md#sql_table_name)) |
| the `sql` of a dimension or measure, or `sql_always_where`, in a subquery `(SELECT … FROM ${v.SQL_TABLE_NAME})` | Unsupported: `{{ #model }}` exists only in a query model. Manual: when `v` is a table view, write its table name; otherwise make the subquery a query model and join it. Report it. |
| `${SQL_TABLE_NAME}` (the view's own table, in `sql_create` or `create_process`) | dropped with the statement that holds it (see [derived-tables.md](derived-tables.md#create_process-sql_create)) |

```lookml
view: user_order_facts {
  derived_table: {
    sql:
      SELECT o.user_id, COUNT(*) AS lifetime_orders
      FROM ${orders.SQL_TABLE_NAME} AS o
      GROUP BY 1 ;;
  }
  dimension: user_id { primary_key: yes  type: number  sql: ${TABLE}.user_id ;; }
  dimension: lifetime_orders { type: number  sql: ${TABLE}.lifetime_orders ;; }
}
```
```aml
Model user_order_facts {
  type: 'query'
  label: 'User Order Facts'
  data_source_name: 'demodb'
  models: [orders]

  query: @sql
    SELECT o.user_id, COUNT(*) AS lifetime_orders
    FROM {{ #orders AS o }}
    GROUP BY 1 ;;

  dimension user_id {
    label: 'User Id'
    type: 'number'
    primary_key: true
    definition: @sql {{ #SOURCE.user_id }};;
  }
  dimension lifetime_orders {
    label: 'Lifetime Orders'
    type: 'number'
    definition: @sql {{ #SOURCE.lifetime_orders }};;
  }
}
```

Difference: Looker inserts a table name, so the SQL sees the columns of that table. Holistics inserts the model,
which may expose its dimensions by their names instead. Mark these models for review; the details are in
[derived-tables.md](derived-tables.md#references-to-other-views).

## `@{constant}`

Looker: [LookML constants](https://docs.cloud.google.com/looker/docs/sql-and-referring-to-lookml),
[`constant`](https://docs.cloud.google.com/looker/docs/reference/param-manifest-constant) ·
Holistics: [AML Constant](https://docs.holistics.io/reference/aml/constant),
[String interpolation](https://docs.holistics.io/reference/aml/string-interpolation)

`@{NAME}` is replaced by the value of the constant, as text. The constant itself becomes an AML `const`
(see [manifest.md](manifest.md#constant)). At each use, either keep the reference as `${NAME}` or inline the value:

| Where `@{NAME}` occurs | Conversion |
|---|---|
| `sql_table_name` | keep: `@{SCHEMA}.orders` → `table_name: '${SCHEMA}.orders'` (see [view.md](view.md#with-a-lookml-constant)) |
| `derived_table: sql`, `sql_create`, `sql_step` | keep: `${NAME}` inside `@sql` |
| the `sql` of a dimension or dimension group | keep: `${NAME}` inside `@sql`, also inside a column reference: `${TABLE}.@{COL}` → `{{ #SOURCE.${COL} }}` |
| a definition that ends up as `@aql` | inline the value, as an AQL literal |
| `connection` | keep, as the bare name: `data_source_name: NAME` (see [model.md](model.md)) |
| `label`, `description`, `group_label` | keep: `'${NAME} Users'` |
| `html`, `link`, `value_format` | inline, then convert the result by [field.md](field.md) |
| a constant whose value contains Liquid | inline, then convert the result by [liquid.md](liquid.md) |

Inlining is always correct. Keeping the reference preserves the single point of change that the LookML author set
up, which matters most for schema, project and connection names.

```lookml
constant: SCHEMA { value: "analytics" }
constant: LOOKBACK_DAYS { value: "30" }
constant: TENANT_COLUMN { value: "tenant_id" }

view: orders {
  sql_table_name: @{SCHEMA}.orders ;;

  dimension: tenant { type: string  sql: ${TABLE}.@{TENANT_COLUMN} ;; }
  dimension_group: created { type: time  timeframes: [raw, date]  sql: ${TABLE}.created_at ;; }
  dimension: is_recent {
    type: yesno
    sql: ${created_raw} >= DATEADD(day, -@{LOOKBACK_DAYS}, CURRENT_DATE) ;;
  }
}
```
```aml
const SCHEMA = 'analytics'
const LOOKBACK_DAYS = '30'
const TENANT_COLUMN = 'tenant_id'

Model orders {
  type: 'table'
  label: 'Orders'
  data_source_name: 'demodb'
  table_name: '${SCHEMA}.orders'

  dimension tenant {
    label: 'Tenant'
    type: 'text'
    definition: @sql {{ #SOURCE.${TENANT_COLUMN} }};;
  }
  dimension created {
    label: 'Created'
    type: 'datetime'
    definition: @sql {{ #SOURCE.created_at }};;
  }
  dimension is_recent {
    label: 'Is Recent'
    type: 'truefalse'
    definition: @sql {{ created }} >= DATEADD(day, -${LOOKBACK_DAYS}, CURRENT_DATE);;
  }
}
```

Two different `${…}` are in play: LookML's `${field}`, which becomes `{{ field }}`, and AML's `${const}`, which is
interpolation. After conversion, every `${` left in an AML file is AML interpolation.
