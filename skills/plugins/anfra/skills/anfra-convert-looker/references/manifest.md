# LookML project manifest → AML constants

How to convert every parameter of `manifest.lkml`. The sections follow the order of Looker's
[Project manifest parameters](https://docs.cloud.google.com/looker/docs/reference/param-project-manifest) page.

The manifest is not converted as a file. Only its `constant`s have an AML counterpart; they go to a `manifest.aml`
file at the project root.

```lookml
project_name: "shop"

constant: CONNECTION_NAME {
  value: "demodb"
  export: override_required
}
```
```aml
const CONNECTION_NAME = 'demodb'
```

## Quick reference

| LookML parameter | AML | Section |
|---|---|---|
| `project_name` | not needed | [project_name](#project_name) |
| `new_lookml_runtime` | drop | [new_lookml_runtime](#new_lookml_runtime) |
| `local_dependency`, `remote_dependency` | unsupported; convert the imported files together | [dependencies](#local_dependency-remote_dependency) |
| `constant` | `const` | [constant](#constant) |
| `localization_settings` | unsupported | [localization_settings](#localization_settings) |
| `application` | unsupported | [application](#application) |
| `visualization` | unsupported | [visualization](#visualization) |

---

## Structural parameters

### `project_name`

Looker: [`project_name`](https://docs.cloud.google.com/looker/docs/reference/param-manifest-project-name)

Not needed. A Holistics project is a repository and has no name in AML. Drop it; it can serve as the root folder name.

### `new_lookml_runtime`

Looker: [`new_lookml_runtime`](https://docs.cloud.google.com/looker/docs/reference/param-manifest-new-lookml-runtime)

Looker-internal. Drop.

### `local_dependency`, `remote_dependency`

Looker: [`local_dependency`](https://docs.cloud.google.com/looker/docs/reference/param-manifest-local-dependency),
[`remote_dependency`](https://docs.cloud.google.com/looker/docs/reference/param-manifest-remote-dependency)

Unsupported. AML cannot import another project. Instead:

1. Get the imported project's files (the other project on the same Looker instance, or the Git `url` at `ref`).
2. Convert them together with the importing project, into the same AML project. An `include: "//name/…"` line then
   resolves like a local include.
3. `override_constant: NAME { value: "…" }` → use this value for the imported constant `NAME`, instead of the
   default declared in the imported project.

If the imported files are not available, every view, explore and field defined there is an unresolved reference.
List them in the conversion report. Do not guess their definitions.

### `constant`

Looker: [`constant`](https://docs.cloud.google.com/looker/docs/reference/param-manifest-constant) ·
Holistics: [AML Constant](https://docs.holistics.io/reference/aml/constant),
[String interpolation](https://docs.holistics.io/reference/aml/string-interpolation)

`constant: NAME { value: "…" }` → `const NAME = '…'` in `manifest.aml`.

1. Write the value in a single-quoted string. Escape `'` as `\'`, `\` as `\\`, and a literal `${` as `\${`.
2. Keep the quotes even when the value is a number (`value: "30"`). LookML constants are always text.
3. A value that uses other constants: `@{A}` → `${A}`. Declare it after the constants it uses.
4. A value that contains Liquid (`{% if … %}`) is not emitted as a `const`. Inline it at each place of use and
   convert the result there (see [liquid.md](liquid.md)).
5. If the name is already taken by a Model or Dataset, add the suffix `_const` and rename its uses. All top-level
   AML objects share one namespace.
6. `export` → drop. It only says whether an importing LookML project may override the value.

```lookml
constant: PROJECT { value: "acme-warehouse" }
constant: DATASET { value: "sales" }
constant: SALES_TABLE_PREFIX { value: "@{PROJECT}.@{DATASET}" }
constant: LOOKBACK_DAYS { value: "30" }
```
```aml
const PROJECT = 'acme-warehouse'

const DATASET = 'sales'

const SALES_TABLE_PREFIX = '${PROJECT}.${DATASET}'

const LOOKBACK_DAYS = '30'
```

Where a constant is used, `@{NAME}` becomes `${NAME}` inside an AML string or an `@sql` block. Where a whole value
is the constant (`connection: "@{NAME}"`), use the bare name.

```lookml
view: orders {
  sql_table_name: `@{SALES_TABLE_PREFIX}.orders` ;;

  dimension: created_at { type: date_time }
  dimension: is_recent {
    type: yesno
    sql: ${created_at} >= DATEADD(day, -@{LOOKBACK_DAYS}, CURRENT_DATE) ;;
  }
}
```
```aml
Model orders {
  type: 'table'
  label: 'Orders'
  data_source_name: 'demodb'
  table_name: '`${SALES_TABLE_PREFIX}.orders`'

  dimension created_at {
    label: 'Created At'
    type: 'datetime'
  }
  dimension is_recent {
    label: 'Is Recent'
    type: 'truefalse'
    definition: @sql {{ created_at }} >= DATEADD(day, -${LOOKBACK_DAYS}, CURRENT_DATE);;
  }
}
```

After conversion, every `${…}` left in an AML file is AML interpolation. LookML's own `${field}` references are
rewritten to `{{ field }}` (see [sql-references.md](sql-references.md)).

Notes:

- Constants in Looker are visible only inside their own project; AML constants are visible in the whole project.
- A constant with `export: override_required` often holds a placeholder (`"GCP Project ID"`) that the user was
  meant to replace. Report these so they get a real value.

## Localization parameters

### `localization_settings`

Looker: [`localization_settings`](https://docs.cloud.google.com/looker/docs/reference/param-manifest-localization-settings)

Unsupported. AML labels are plain strings. Look up each label and description key in the strings file of the
`default_locale` and emit the resulting text. The other locales are lost.

## Extension framework parameters

### `application`

Looker: [`application`](https://docs.cloud.google.com/looker/docs/reference/param-manifest-application)

Unsupported. Drop.

## Custom visualization parameters

### `visualization`

Looker: [`visualization`](https://docs.cloud.google.com/looker/docs/reference/param-manifest-visualization)

Unsupported. Drop.
