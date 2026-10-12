# Implementation: revamp query result data types

Implements [result-values.md](../designs/result-values.md) and [sdk-numbers.md](../designs/sdk-numbers.md): a query's values encoded (every number a string), each column's `semantic_type`, `arrow_type` and `db_type`, and numbers kept exact on their way back in Query Input.

## Where it stands

| Piece | State |
|---|---|
| Integers as strings | Done: canal-query 2.16.0's `json_output_data_options.integers_as_strings`, always on (`internal/sidecar/canalquery/client.go`). |
| The header before the rows (`fields`, DB `types`) | Done: canal-query 2.17.0's `stream_header`, always on. |
| Floats as strings in JavaScript's spelling | Done: canal-query 2.17.1, at any depth. |
| Arrow types in the header | Done: canal-query 2.17.1's `stream_result_options.header_arrow_types`, which replaces 2.17.0's `stream_header` with `stream_result_options.header`. |
| A column's AMQL type | Done: anfra-node 0.4.0 (amql 7.2.0)'s `ExploreColumn.type`, from the compile's type checker. |
| Exact numbers in Query Input | Done: anfra-node 0.4.0. A number is a string, written into its literal as given; a JSON number is refused. |
| Column types in the core | Built: `query.Column`, the header's Arrow types (`canalquery.ArrowType`), Query Input values documented as strings. `manifest.yml` names canal-query 2.17.1 and anfra-node 0.4.0. |
| The SDK's handling | Built: types on `ColumnMeta`, `aql.literal` by semantic type, a number filter refusing a JS number. |
| The skill and the demo apps | Done locally, not run in a browser. The skill's source (`anfra-skills`, `plugins/anfra/skills/build-data-app`) and the number guidance of `anfra-demo-data`'s older copy (`build-anfra-app`). The demo apps that gave number filters JS numbers (headcount-planner, insight-builder, nlq, sheet), sorted or formatted numbers as text (funnel-analysis, nlq, metric-catalog), or rounded a number into AQL (metric-catalog). |

## Order

```
1 canal-query ──┐
                ├─> 3 core ──> 4 SDK ──> 5 skill and demo apps
2 amql ─────────┘
```

Steps 1 and 2 are upstream and independent. The core lands step 3 once both have released. Each step ships with its tests.

## 1. canal-query

1. **Floats** in JavaScript's spelling: the shortest form that reads back, an exponent only below 1e-6 or from 1e21, `"NaN"`, `"Infinity"`, `"-Infinity"`; at any depth in lists, maps and structs.
2. **Arrow types in the header**, beside `fields` and `types`: one per field, as [Arrow types](../designs/result-values.md#arrow-types) shapes them. That is Schema.fbs's `Type` as snake_case JSON (`type`, then that type's fields: `bit_width`, `is_signed`, `precision`, `scale`, `unit`, `timezone`), `children` for a nested type, and `label`, today's arrow-go string.
   - The key: `arrow_types`, as in the result metadata, which keeps its strings for its existing readers.
   - Tests: one type of each kind, nested types, and a type's defaults written out (`bit_width: 128` for a decimal).
3. Release, then bump `manifest.yml`'s `canal_query` here.

## 2. amql and anfra-node (holistics-core)

1. **The column's type.** `ExploreColumn` gets `type`: the type AMQL's type checker gives the column's expression when the query compiles, in AMQL's names (`number`, `text`, `date`, `datetime`, `time`, `truefalse`, …). Not the field's declared type: a count of a text field is a `number`, a truncated datetime a `date`, and an adhoc expression has no field.
   - First, find where the compile's type check keeps each output expression's type, and how `explore_columns.ts` can reach it.
   - Tests: a dimension, an aggregated text field, a date-truncated datetime, an adhoc expression, a dataset metric.
2. **Exact numbers in Query Input** (`packages/amql/src/query_input.ts`, `literalFor`). For a `number` field:
   - A string in JSON's number syntax goes into the number literal as its digits, never through `Number()`. An exponent is written out in full, since AQL's number literal has none (`"1e-7"` is `0.0000001`).
   - A JSON number is refused, even `17`: send it as a string, `"17"`. So nothing needs a JSON number's source text, and the core reads values as it does today.
   - Anything else is refused, as today: `"abc"`, `"NaN"`, `"Infinity"`, `true`.
   - An exponent past a float64's range (`"1e400"`) is refused: no result writes one. A number past that range is written in full.
   - Tests: `"9007199254741007"` compiles to that literal, `"0.1000000000000000055511151231257827"` keeps its digits, `"1e21"` is written out, `17` and `9007199254741007` as JSON numbers are refused.
3. Release anfra-node, then bump it here.

## 3. core

1. **Arrow types from the header.** `canalquery.Header` gets `ArrowTypes []ArrowType`. `ArrowType` holds `Type`, `Label`, the type's own fields and `Children`. It is decoded as canal-query sends it, and kept as raw JSON beside the fields the core reads, so a type canal-query adds later passes through unchanged. A header without them leaves them empty: a generic check, not a version check.
2. **The header reaches the result.** `query.RunResult` carries the header (fields, DB types, Arrow types), not only `Fields`. `query.Execute` and `ExecuteSQL` fill it from `canalquery.Result.Header`.
3. **`query.Column`** in `internal/command/query`: amql's `ExploreColumn` keys as they are (camelCase), plus `semantic_type`, `arrow_type` and `db_type` (snake_case). It replaces `describeFields`' output.
   - `semantic_type`: the column's AMQL `type`, through one table (AML's types and `bit` as they are; AMQL's other types `unknown`). Absent when AMQL typed nothing: never derived from the Arrow or database type, which can only contradict the meaning a database's type has ([result-values.md](../designs/result-values.md#where-each-comes-from)).
   - `arrow_type`, `db_type`: as the header gives them, absent when it doesn't.
   - A SQL query's columns keep the `fieldName`, `label` and `adhoc: true` they had (the field's name), so existing readers don't break. Dropping them is a separate, breaking change.
   - The AMQL-to-semantic table has a test listing each of AMQL's types.
4. **Query Input numbers as strings.** The schema and docs for filter `values`: a `number` field's value is a string in JSON's number syntax; a JSON number is refused. Values stay `[]any`: a string passes through unchanged.
5. **Exports** describe columns with `query.Column` (`headerRow`). No change to what a CSV holds.
6. **Docs and contract.**
   - The `Records` doc in `result.go`: every number a string, the non-finite spellings, nested values.
   - `columns`' new keys in the schema: `semantic_type` as an enum, `arrow_type` as an object.
   - Regenerate `api/openapi.yaml` and `web/sdk/src/api/schema.d.ts`.
   - [data-apps.md](../designs/data-apps.md): `columns` mixes amql's camelCase with the core's snake_case keys, and why.
   - Both designs' status, once built.
7. **Tests.**
   - Unit: the header with Arrow types (`canalquery`), the column's types for an AQL and a SQL query, the AMQL-to-semantic table.
   - Against real sidecars (`TestCommandsAgainstRealSidecars`, local Postgres): an `int8` past 2^53, a `numeric(38, 10)`, a `float8` of 1234567.5 and of `'Infinity'`, each column's three types, a filter on the large `int8` matching its row and no other.

## 4. SDK (`web/sdk`)

1. **Types on `ColumnMeta`** (`common/types.ts`): `semantic_type`, `arrow_type`, `db_type`, as the API answers them, required. `coreApiBackend` fails a query whose columns come without them: a host that serves Data Apps must send them.
2. **AQL literals by type.** `aql.literal` (`app/aql.ts`) takes the column's `semantic_type`, and writes a `number` value's string as its digits (`17`, not `"17"`). The selection code that builds a condition (`app/selection.ts`) passes each column's type.
   - Test: a cross-filter over two fields, one a number, compiles and matches.
3. **Values sent back as they came.** Filter and cross-filter values go back as the API's strings, never converted. A filter of type `number` refuses a JS number where the app hands it over, naming the string to pass: converting it would hide digits it may already have lost, which is what the core API's refusal catches. Field suggestions (`api/backend.ts`) keep a `number` field's values as strings. A typed-in value for a number control stays the string typed.
4. **No number helpers** ([sdk-numbers.md](../designs/sdk-numbers.md#helpers-later)).

## 5. The skill and the demo apps

1. The `build-data-app` skill (`references/api.md`, `references/example.html`): every number arrives as a string; decide by `semantic_type`; `Number(v)` where `arrow_type` says it keeps every digit; `Intl.NumberFormat` to display. Its source is the repo that publishes the skill, and the copy in `anfra-demo-data` follows it.
2. `anfra-demo-data`, where a number is used as a JS number without reading it first:
   - `funnel-analysis.html:1584`: sorts with `>`/`<` on strings.
   - `metric-catalog.html:467`, `nlq.html:918`: format only when `typeof v === 'number'`.
   - `drill.html`: compares filter values with `===`. Check, since its options also come from `core.query`.

## What breaks

- **Numbers in answers.** Integers were JSON numbers before 2.16.0. Every number is a string now.
- **Query Input.** A `number` field's filter value is a string. A JSON number is refused. Before, it was rounded silently.
- **canal-query 2.17.0's `stream_header`** is gone: anfra needs canal-query 2.17.1.

The core's commit for these is a breaking one (`feat!:`), with the release notes saying what to change.

## Open

- How a `json` value is written, and `bit` as booleans: see [result-values.md](../designs/result-values.md#open-questions).
