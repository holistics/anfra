# LookML fields → AML fields

How to convert every parameter of a LookML field. The sections follow the order of Looker's
[Field parameters](https://docs.cloud.google.com/looker/docs/reference/param-field) page.

This doc covers the parameters that fields share: labels, grouping, formats, drills, links, filter settings.
Each kind of field has its own doc for its types, its `sql` and its type-specific parameters:

| LookML field | Becomes | Doc |
|---|---|---|
| `dimension` | model `dimension` | [dimension.md](dimension.md) |
| `dimension_group` | one or more model `dimension`s | [dimension-group.md](dimension-group.md) |
| `measure` | model `measure` | [measure.md](measure.md) |
| `filter` | `param`, a `truefalse` dimension, or nothing | [filter.md](filter.md) |
| `parameter` | `param` | [parameter.md](parameter.md) |

```lookml
view: orders {
  dimension: amount {
    label: "Order Amount"
    description: "Amount before tax."
    type: number
    value_format_name: usd
    sql: ${TABLE}.amount ;;
  }
  dimension: internal_note {
    hidden: yes
    type: string
    sql: ${TABLE}.internal_note ;;
  }
}
```
```aml
dimension amount {
  label: 'Order Amount'
  type: 'number'
  description: 'Amount before tax.'
  format: '[$$]#,###0.00'
  definition: @sql {{ #SOURCE.amount }};;
}
dimension internal_note {
  label: 'Internal Note'
  type: 'text'
  hidden: true
  definition: @sql {{ #SOURCE.internal_note }};;
}
```

Where the settings live:

| Concern | LookML | Holistics |
|---|---|---|
| label, description, hidden, value format | on the field | on the field ([AML Field](https://docs.holistics.io/reference/aml/field)) |
| grouping in the field picker | on the field | per Dataset, in its `view` block |
| drill paths | on the field or view | per Dataset, in its `context` block |
| links, actions, HTML | on the field | per visualization, outside the models |
| filter suggestions | on the field | automatic; nothing to write |

So grouping and drill settings written once on a view are repeated in every Dataset that contains the model.

## Quick reference

| LookML parameter | Field types | AML | Section |
|---|---|---|---|
| `dimension` | D | `dimension` | [dimension.md](dimension.md) |
| `dimension_group` | DG | `dimension`s | [dimension-group.md](dimension-group.md) |
| `filter` | F | see doc | [filter.md](filter.md) |
| `measure` | M | `measure` | [measure.md](measure.md) |
| `parameter` | P | `param` | [parameter.md](parameter.md) |
| `intervals`, `timeframes` | DG | explicit dimensions | [dimension-group.md](dimension-group.md) |
| `action` | D M | unsupported | [action](#action) |
| `drill_fields` | D DG M | Dataset `context { analysis { … } }` | [drill_fields](#drill_fields) |
| `link` | D M | unsupported | [link](#link) |
| `tags` | all | unsupported | [tags](#tags) |
| `alias` | all | not needed | [alias](#alias) |
| `alpha_sort` | D | see `case` | [dimension.md](dimension.md) |
| `description` | all | `description` | [description](#description) |
| `group_label` | D DG M F | Dataset `view { … group … }` | [group_label](#group_label) |
| `group_item_label` | D DG M F | unsupported | [group_item_label](#group_item_label) |
| `hidden` | all | `hidden` | [hidden](#hidden) |
| `label` | all | `label` | [label](#label) |
| `label_from_parameter` | D M | unsupported | [label_from_parameter](#label_from_parameter) |
| `order_by_field` | D DG M | unsupported | [order_by_field](#order_by_field) |
| `style` | D | see `type: tier` | [dimension.md](dimension.md) |
| `view_label` | all | unsupported | [view_label](#view_label) |
| `can_filter` | D DG M | unsupported | [can_filter](#can_filter) |
| `case_sensitive` | D F | unsupported | [case_sensitive](#case_sensitive) |
| `filters` | M | `where(…)` | [measure.md](measure.md) |
| `skip_drill_filter` | D | unsupported | [skip_drill_filter](#skip_drill_filter) |
| `allowed_value` | P | `allowed_values` | [parameter.md](parameter.md) |
| `bypass_suggest_restrictions`, `full_suggestions`, `suggest_dimension`, `suggest_explore`, `suggest_persist_for`, `suggestable`, `suggestions` | D DG M F P | unsupported | [suggestions](#suggestion-parameters) |
| `allow_approximate_optimization`, `approximate`, `approximate_threshold` | M | see doc | [measure.md](measure.md) |
| `convert_tz`, `datatype` | D DG M F P | see doc | [dimension-group.md](dimension-group.md) |
| `fanout_on` | D DG M | unsupported | [fanout_on](#fanout_on) |
| `precision` | M | not needed | [measure.md](measure.md) |
| `primary_key` | D | `primary_key` | [dimension.md](dimension.md) |
| `required_access_grants` | all | unsupported | [required_access_grants](#required_access_grants) |
| `required_fields` | D M | not needed | [required_fields](#required_fields) |
| `sql_distinct_key` | M | nested aggregation | [measure.md](measure.md) |
| `case` | D | `CASE` dimension | [dimension.md](dimension.md) |
| `default_value` | F P | see docs | [filter.md](filter.md), [parameter.md](parameter.md) |
| `direction` | M | see doc | [measure.md](measure.md) |
| `html` | D DG M | unsupported | [html](#html) |
| `list_field`, `percentile` | M | see doc | [measure.md](measure.md) |
| `sql` | D DG M F | `definition` | [sql](#sql) |
| `sql_start`, `sql_end` | D DG | `date_diff(…)` | [dimension-group.md](dimension-group.md), [dimension.md](dimension.md) |
| `sql_latitude`, `sql_longitude`, `start_location_field`, `end_location_field`, `units`, `tiers` | D | see the type | [dimension.md](dimension.md) |
| `synonyms` | all | text in `description` | [synonyms](#synonyms) |
| `type` | all | `type` | [type](#type) |
| `string_datatype` | D | not needed | [string_datatype](#string_datatype) |
| `value_format` | D M | `format` | [value_format](#value_format) |
| `value_format_name` | D M | `format` | [value_format_name](#value_format_name) |
| `allow_fill` | D DG | unsupported | [allow_fill](#allow_fill) |
| `map_layer_name` | D | unsupported | [map_layer_name](#map_layer_name) |
| `decimals`, `format` | D M | `format` | [Parameters to avoid](#parameters-to-avoid) |

D = dimension, DG = dimension group, M = measure, F = filter, P = parameter.

---

## Structural parameters

### `dimension` | `dimension_group` | `measure` | `filter` | `parameter`

See the doc of each field kind (table at the top). Rules that hold for all of them:

- **Name.** Keep the field name unchanged. Other fields refer to it.
- **`label` and `type` are required** on every AML field. See [`label`](#label) and [`type`](#type).
- **Place.** A field goes into the Model made from its view. A field whose `sql` uses a field of another view
  cannot live in a model; it becomes a Dataset `dimension` or `metric` (see [sql-references.md](sql-references.md)).

### `intervals`, `timeframes`

See [`intervals`](dimension-group.md#intervals) and [`timeframes`](dimension-group.md#timeframes) in dimension-group.md.

## Action and linking parameters

### `action`

Looker: [`action`](https://docs.cloud.google.com/looker/docs/reference/param-field-action)

Unsupported. Drop it and keep its LookML in the conversion report.

### `drill_fields`

Looker: [`drill_fields` (for fields)](https://docs.cloud.google.com/looker/docs/reference/param-field-drill-fields),
[`drill_fields` (for views)](https://docs.cloud.google.com/looker/docs/reference/param-view-drill-fields),
[`set`](https://docs.cloud.google.com/looker/docs/reference/param-view-set) ·
Holistics: [View underlying data](https://docs.holistics.io/docs/interactions/view-underlying-data),
[Drill-down](https://docs.holistics.io/docs/interactions/drill-down)

Drill settings are not field properties in AML. Write them in the `context { analysis { … } }` block of every
Dataset that contains the model.

1. **Expand sets first.** Replace each `set_name*` by the members of the set, recursively, and apply `-field`
   exclusions. A `set` has no AML counterpart; it exists only during conversion.
2. **Measure with `drill_fields`** → one `underlying_data` entry for the measure, with one `view` that lists the
   drill fields as `r(model.dimension)`.
   - Only dimensions are allowed. Drop the measures in the list.
   - Drop fields of views that are not in this Dataset.
   - A dimension-group timeframe (`created_date`) → the dimension that replaces it
     (see [dimension-group.md](dimension-group.md)).
3. **Measure without `drill_fields`**, in a view that has a view-level `drill_fields` → use the view-level list.
4. **`drill_fields: []`** → emit nothing.
5. **Dimension with `drill_fields`** → one `breakdown` group: the dimension itself, then the listed dimensions.
6. **Dimension group with `drill_fields`** → drop. Holistics offers every date grain when drilling into a date.

```lookml
view: orders {
  set: order_detail { fields: [id, status, city, total_amount] }

  dimension: country { drill_fields: [city] }
  measure: order_count {
    type: count
    drill_fields: [order_detail*]
  }
}
```
```aml
Dataset orders_dataset {
  label: 'Orders'
  data_source_name: 'demodb'
  models: [orders]
  relationships: []

  context {
    analysis {
      breakdown {
        group country_drill {
          label: 'Country'
          fields: [r(orders.country), r(orders.city)]
        }
      }
      underlying_data {
        metric order_count {
          view order_detail {
            label: 'Order Detail'
            fields: [r(orders.id), r(orders.status), r(orders.city)]
          }
        }
      }
    }
  }
}
```

Differences. Mark every converted drill block for review:

- A Looker drill runs a new query that can contain measures and a sort. The Holistics underlying-data view is a
  list of rows with dimensions only.
- Holistics shows underlying data and breakdowns without any setup. The conversion only narrows the field lists.
  Once one `breakdown` group is declared, the default list of all dimensions is replaced by the declared groups.
- The Holistics docs show `underlying_data` for a Dataset `metric`. Whether it also applies to a model `measure`
  named this way is not confirmed, and the validator does not check the name.

### `link`

Looker: [`link`](https://docs.cloud.google.com/looker/docs/reference/param-field-link) ·
Holistics: [Actions](https://docs.holistics.io/docs/actions)

Unsupported. Links are set per visualization in Holistics, not on the field. Drop it and report it with its LookML:

| Link | What to report |
|---|---|
| to an external URL built from `{{ value }}` | re-create as a "Go to URL" action on the Data Table column in each report |
| to Looker content (`/dashboards/…`, `/explore/…`) | has no meaning after the migration; drop |
| `icon_url` | no equivalent |

### `tags`

Looker: [`tags`](https://docs.cloud.google.com/looker/docs/reference/param-field-tags)

Unsupported. Drop it. If the tags carry meaning for readers, append them to the `description` (`Tags: email`).

## Display parameters

### `alias`

Looker: [`alias`](https://docs.cloud.google.com/looker/docs/reference/param-field-alias)

Not needed. Drop it. It only keeps old field names working in saved Looker URLs.

### `alpha_sort`

See [`case`](dimension.md#case) in dimension.md.

### `description`

Looker: [`description`](https://docs.cloud.google.com/looker/docs/reference/param-field-description) ·
Holistics: [AML Field](https://docs.holistics.io/reference/aml/field)

`description: "…"` → `description: '…'`. Escape `'` as `\'`. For text with line breaks use a `'''` string.

```lookml
dimension: status {
  description: "The order's current state."
}
```
```aml
dimension status {
  label: 'Status'
  type: 'text'
  description: 'The order\'s current state.'
}
```

A description that contains Liquid or a localization key: see [liquid.md](liquid.md) and
[manifest.md](manifest.md#localization_settings).

### `group_label`

Looker: [`group_label`](https://docs.cloud.google.com/looker/docs/reference/param-field-group-label) ·
Holistics: [Custom dataset views](https://docs.holistics.io/docs/datasets/custom-views)

Not a field property in AML. Groups are written in the `view` block of each Dataset that contains the model.

1. If any visible field of a model has a `group_label`, emit `model <name> { … }` inside the Dataset's `view`.
2. One `group <slug>` per distinct `group_label`. Slug: lower case, every other character → `_`
   (`"Shipping Address"` → `shipping_address`).
3. List **every** visible field of the model: grouped ones inside their group, the others directly. A model that
   lists fields shows only the listed ones.
4. Leave out hidden fields. Keep the order of the LookML file.

```lookml
view: orders {
  dimension: id { type: number }
  dimension: city    { group_label: "Geography" }
  dimension: country { group_label: "Geography" }
}
```
```aml
Dataset orders_grouped_dataset {
  label: 'Orders'
  data_source_name: 'demodb'
  models: [orders]
  relationships: []

  view {
    model orders {
      field id
      group geography {
        field city
        field country
      }
    }
  }
}
```

Differences:

- A group has a name only, no `label`. The title shown comes from the slug, not from the original text.
- The field list must be updated whenever a field is added to the model.
- An explore-level `fields` list uses the same `view` block; write one block that serves both
  (see [explore.md](explore.md#fields)).
- A dimension group forms its own group in Looker's field picker; see [dimension-group.md](dimension-group.md).

### `group_item_label`

Looker: [`group_item_label`](https://docs.cloud.google.com/looker/docs/reference/param-field-group-item-label)

Unsupported. Drop it. The field shows its full `label` inside the group.

### `hidden`

Looker: [`hidden`](https://docs.cloud.google.com/looker/docs/reference/param-field-hidden) ·
Holistics: [AML Field](https://docs.holistics.io/reference/aml/field)

`hidden: yes` → `hidden: true`. `hidden: no` or absent → omit. A hidden field stays usable in other definitions.

A view with `fields_hidden_by_default: yes`: see [view.md](view.md#fields_hidden_by_default).

Hiding is not access control, in Looker or in Holistics.

### `label`

Looker: [`label` (for field)](https://docs.cloud.google.com/looker/docs/reference/param-field-label) ·
Holistics: [AML Field](https://docs.holistics.io/reference/aml/field)

`label: "X"` → `label: 'X'`. AML requires a label, so without one write the label Looker would show: the field
name with `_` replaced by spaces and each word capitalized.

```lookml
dimension: total_sale_price { type: number }
measure: aov { label: "Average Order Value"  type: average  sql: ${total_sale_price} ;; }
```
```aml
dimension total_sale_price {
  label: 'Total Sale Price'
  type: 'number'
}
measure aov {
  label: 'Average Order Value'
  type: 'number'
  definition: @aql average(orders.total_sale_price);;
}
```

- Labels of the dimensions made from a `dimension_group`: see [dimension-group.md](dimension-group.md).
- A label that contains Liquid or a localization key: see [liquid.md](liquid.md) and
  [manifest.md](manifest.md#localization_settings).

### `label_from_parameter`

Looker: [`label_from_parameter`](https://docs.cloud.google.com/looker/docs/reference/param-field-label-from-parameter)

Unsupported. An AML label is fixed text. Drop it and keep the field's static `label` (or the default label).
The parameter itself still converts; see [parameter.md](parameter.md).

### `order_by_field`

Looker: [`order_by_field`](https://docs.cloud.google.com/looker/docs/reference/param-field-order-by-field)

Unsupported. AML has no "sort by another field" setting. Drop it, convert both fields as usual, and report the pair
(`month_name` → sort by `month_num`). If the sort field was hidden, keep it hidden.

After conversion the field sorts by its own values. To get the old order, each report must include the sort field
and sort by it.

### `style`

See [`type: tier`](dimension.md#tier) in dimension.md.

### `view_label`

Looker: [`view_label` (for field)](https://docs.cloud.google.com/looker/docs/reference/param-field-view-label) ·
Holistics: [Custom dataset views](https://docs.holistics.io/docs/datasets/custom-views)

Unsupported. In a Dataset `view` a model field can only be listed under its own model. Drop it and report it.
Manual options, for a person to choose:

- leave the field under its own model and put the intended heading in its label (`'Customer: Lifetime Value'`);
- for a measure, define it as a Dataset `metric` and place it in a top-level `group` of the Dataset `view`.

`view_label` on a view, explore or join is different: see [view.md](view.md#parameters-to-avoid) and
[explore.md](explore.md#view_label).

## Filter parameters

### `can_filter`

Looker: [`can_filter`](https://docs.cloud.google.com/looker/docs/reference/param-field-can-filter)

Unsupported. Drop it. After conversion users can filter on the field.

### `case_sensitive`

Looker: [`case_sensitive` (for field)](https://docs.cloud.google.com/looker/docs/reference/param-field-case-sensitive)

Unsupported as a field setting. Drop it.

Difference that applies to every text dimension, with or without this parameter: Looker filters are case-sensitive
by default, and the text operators of Holistics dashboard filters are case-insensitive. Inside AQL you choose:
`like` or `ilike` (see [model.md](model.md#case_sensitive)).

### `filter`

See [filter.md](filter.md).

### `filters`

See [`filters`](measure.md#filters) in measure.md.

### `skip_drill_filter`

Looker: [`skip_drill_filter`](https://docs.cloud.google.com/looker/docs/reference/param-field-skip-drill-filter)

Unsupported. Drop it.

## Filter suggestion parameters

### `allowed_value`

See [`allowed_value`](parameter.md#allowed_value) in parameter.md.

### Suggestion parameters

Looker: [`bypass_suggest_restrictions`](https://docs.cloud.google.com/looker/docs/reference/param-field-bypass-suggest-restrictions),
[`full_suggestions`](https://docs.cloud.google.com/looker/docs/reference/param-field-full-suggestions),
[`suggest_dimension`](https://docs.cloud.google.com/looker/docs/reference/param-field-suggest-dimension),
[`suggest_explore`](https://docs.cloud.google.com/looker/docs/reference/param-field-suggest-explore),
[`suggest_persist_for`](https://docs.cloud.google.com/looker/docs/reference/param-field-suggest-persist-for),
[`suggestable`](https://docs.cloud.google.com/looker/docs/reference/param-field-suggestable),
[`suggestions`](https://docs.cloud.google.com/looker/docs/reference/param-field-suggestions)

Unsupported. Holistics builds filter suggestions from the field's values and has no setting for it in the model.
Drop all seven.

Report `suggest_dimension` and `suggest_explore` when they point at a small lookup table: they were written to make
suggestions fast, and that effect is lost.

## Query parameters

### `allow_approximate_optimization`, `approximate`, `approximate_threshold`

See [measure.md](measure.md).

### `convert_tz`, `datatype`

See [`convert_tz`](dimension-group.md#convert_tz) and [`datatype`](dimension-group.md#datatype) in dimension-group.md.

### `fanout_on`

Looker: [`fanout_on`](https://docs.cloud.google.com/looker/docs/reference/param-field-fanout-on)

Unsupported. It belongs to BigQuery Legacy SQL repeated records. Drop it and report the field for manual review.

### `precision`

See [measure.md](measure.md).

### `primary_key`

See [`primary_key`](dimension.md#primary_key) in dimension.md.

### `required_access_grants`

Looker: [`required_access_grants` (for field)](https://docs.cloud.google.com/looker/docs/reference/param-field-required-access-grants) ·
Holistics: [Column-level permission](https://docs.holistics.io/docs/access-control/column-level-permission)

Unsupported. AML cannot hide a field from some users.

This is a **security-relevant loss**: after conversion the field is visible to everyone who can open the Dataset.
List every such field in the conversion report. Manual workaround: column-level permission on the dimension. It
masks the values; it does not hide the field.

### `required_fields`

Looker: [`required_fields`](https://docs.cloud.google.com/looker/docs/reference/param-field-required-fields)

Not needed. Drop it. It adds fields to the query for the `html` or `link` of this field, which are not converted.
If the field's `sql` really uses another field, the converted definition already references it.

### `sql_distinct_key`

See [measure.md](measure.md).

## Value and formatting parameters

### `case`

See [`case`](dimension.md#case) in dimension.md.

### `default_value`

See `default_value` in [filter.md](filter.md#default_value) and [parameter.md](parameter.md#default_value).

### `direction`

See [measure.md](measure.md).

### `end_location_field`, `start_location_field`, `sql_latitude`, `sql_longitude`, `units`, `tiers`

See the matching type (`distance`, `location`, `tier`) in [dimension.md](dimension.md).

### `html`

Looker: [`html`](https://docs.cloud.google.com/looker/docs/reference/param-field-html) ·
Holistics: [Data formats](https://docs.holistics.io/docs/data-format)

Unsupported. An AML field has no HTML template. Drop it and report it with its LookML:

| `html` does | What to report |
|---|---|
| wraps the value in a link or image | build the HTML in a text dimension and show it with the HTML format, set in the report |
| colours or icons from `{% if value … %}` | conditional formatting, set in each visualization |

### `list_field`, `percentile`

See [measure.md](measure.md).

### `sql`

Looker: [`sql` (for field)](https://docs.cloud.google.com/looker/docs/reference/param-field-sql) ·
Holistics: [AML Field](https://docs.holistics.io/reference/aml/field)

`sql` → `definition`. What the definition looks like depends on the field kind:

| Field | See |
|---|---|
| dimension | [dimension.md](dimension.md) |
| dimension group (`sql`, `sql_start`, `sql_end`) | [dimension-group.md](dimension-group.md) |
| measure (the `sql` is the operand of the aggregate) | [measure.md](measure.md) |
| filter | [filter.md](filter.md) |

How `${TABLE}`, `${field}`, `${view.field}`, `${view.SQL_TABLE_NAME}` and `@{constant}` are rewritten:
[sql-references.md](sql-references.md). Liquid inside `sql`: [liquid.md](liquid.md).

### `synonyms`

Looker: [`synonyms`](https://docs.cloud.google.com/looker/docs/reference/param-field-synonyms) ·
Holistics: [AML Field](https://docs.holistics.io/reference/aml/field)

No property of its own. Append the synonyms to the `description`.

```lookml
dimension: email {
  description: "Login address."
  synonyms: ["e-mail", "login"]
}
```
```aml
dimension email {
  label: 'Email'
  type: 'text'
  description: 'Login address. Also known as: e-mail, login.'
}
```

### `type`

Looker: [dimension, filter and parameter types](https://docs.cloud.google.com/looker/docs/reference/param-dimension-filter-parameter-types),
[dimension group types](https://docs.cloud.google.com/looker/docs/reference/param-field-dimension-group),
[measure types](https://docs.cloud.google.com/looker/docs/reference/param-measure-types) ·
Holistics: [AML Field](https://docs.holistics.io/reference/aml/field)

Every AML field needs a `type`: `'text'`, `'number'`, `'date'`, `'datetime'` or `'truefalse'`. A LookML type says
more than that (for a measure it names the aggregation), so each field kind has its own table:
[dimension.md](dimension.md), [dimension-group.md](dimension-group.md), [measure.md](measure.md),
[filter.md](filter.md), [parameter.md](parameter.md).

### `string_datatype`

Looker: [`string_datatype`](https://docs.cloud.google.com/looker/docs/reference/param-field-string-datatype)

Not needed. Drop it.

### `value_format`

Looker: [`value_format`](https://docs.cloud.google.com/looker/docs/reference/param-field-value-format) ·
Holistics: [Number format](https://docs.holistics.io/reference/aml/number-format)

`value_format: "…"` → `format: '…'`, on fields of AML type `'number'` only. On other types drop it.

A Holistics pattern has this order: currency prefix, integer part, decimals, percent, scale letter, suffix.
Translate the Excel-style string piece by piece:

| In `value_format` | In `format` |
|---|---|
| integer part (`0`, `#`, `#,##0`) | always `#,###` |
| N digits after the `.` (`.00`, `.0#`) | `0.` followed by N zeros: `#,###0.00` |
| leading `$`, `"€"`, `\£` | `[$$]`, `[$€]`, `[$£]` in front |
| `%` | `%` (both tools multiply by 100) |
| `\%` (literal sign, no scaling) | `\\%` |
| trailing `,` / `,,` / `,,,` | `,"K"` / `,,"M"` / `,,,"B"`; the text Looker printed after it is replaced by the letter |
| quoted text suffix (`" GiB"`) | `[$ GiB]` at the end; not together with a currency prefix or `%` |
| `positive;negative` sections | keep the first section |
| sections that only switch K/M/B by size (`[>=1000000]0.0,,"M";[>=1000]0.0,"K";0`) | the first section's decimals and `"A"`: `#,###0.0"A"` |
| `@{constant}` | resolve the constant first ([manifest.md](manifest.md#constant)) |

```lookml
measure: revenue   { type: sum  sql: ${TABLE}.amount ;;  value_format: "$#,##0.00" }
measure: revenue_m { type: sum  sql: ${TABLE}.amount ;;  value_format: "$0.0,,\" M\"" }
measure: paid_rate { type: number  sql: ${paid_count} / ${order_count} ;;  value_format: "0.0%" }
```
```aml
measure revenue {
  label: 'Revenue'
  type: 'number'
  definition: @aql sum(orders.amount);;
  format: '[$$]#,###0.00'
}
measure revenue_m {
  label: 'Revenue M'
  type: 'number'
  definition: @aql sum(orders.amount);;
  format: '[$$]#,###0.0,,"M"'
}
measure paid_rate {
  label: 'Paid Rate'
  type: 'number'
  definition: @aql orders.paid_count / orders.order_count;;
  format: '#,###0.0%'
}
```

Cannot be translated → drop the format and report it: time formats (`[hh]:mm:ss`), zero padding (`00000`),
text in front of the number (`"Bug #"0`), phone masks.

Differences:

- Holistics always prints the thousands separator. `"0"` and `"#.00"` showed none in Looker.
- Styling of negative numbers (parentheses, colour) is lost.
- Optional decimals (`0.##`) become fixed decimals.
- The validator does not check pattern strings, and Holistics falls back to its default format when a pattern is
  invalid. Check the formats in a report.

### `value_format_name`

Looker: [`value_format_name`](https://docs.cloud.google.com/looker/docs/reference/param-field-value-format-name) ·
Holistics: [Number format](https://docs.holistics.io/reference/aml/number-format)

A built-in name → a fixed `format` pattern, on fields of AML type `'number'` only:

| `value_format_name` | `format` |
|---|---|
| `decimal_0` | `'#,###'` |
| `decimal_1` … `decimal_4` | `'#,###0.0'` … `'#,###0.0000'` |
| `usd_0`, `gbp_0`, `eur_0` | `'[$$]#,###'`, `'[$£]#,###'`, `'[$€]#,###'` |
| `usd`, `gbp`, `eur` | `'[$$]#,###0.00'`, `'[$£]#,###0.00'`, `'[$€]#,###0.00'` |
| `percent_0` | `'#,###%'` |
| `percent_1` … `percent_4` | `'#,###0.0%'` … `'#,###0.0000%'` |
| `id` | `'none'` |

```lookml
measure: average_amount {
  type: average
  sql: ${TABLE}.amount ;;
  value_format_name: usd
}
```
```aml
measure average_amount {
  label: 'Average Amount'
  type: 'number'
  definition: @aql average(orders.amount);;
  format: '[$$]#,###0.00'
}
```

Any other name is a `named_value_format` of the model file → `format: fmt_<name>`, a constant
(see [model.md](model.md#named_value_format)).

Difference: `id` in Looker prints a rounded integer without separators. `'none'` prints the raw value, which is the
same for integer ids only.

## Visualization parameters

### `allow_fill`

Looker: [`allow_fill`](https://docs.cloud.google.com/looker/docs/reference/param-field-allow-fill) ·
Holistics: [Show all dimension values](https://docs.holistics.io/docs/show-all-dim-values)

Unsupported. Drop it. Filling missing values is a setting of each report in Holistics.

### `map_layer_name`

Looker: [`map_layer_name`](https://docs.cloud.google.com/looker/docs/reference/param-field-map-layer-name)

Unsupported. Drop it; the dimension itself converts as usual. Report it, so the map can be chosen in the
visualization (see [model.md](model.md#map_layer)).

## Parameters to avoid

| Parameter | Conversion |
|---|---|
| [`decimals`](https://docs.cloud.google.com/looker/docs/reference/param-field-decimals) | Removed from Looker. `decimals: N` → a number format with N decimals (`decimals: 2` → `format: '#,###0.00'`). |
| [`format`](https://docs.cloud.google.com/looker/docs/reference/param-field-format) | Removed from Looker. Treat as [`value_format`](#value_format). |
