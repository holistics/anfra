# Result values and column types

> Status: built, on canal-query 2.17.1 and anfra-node 0.4.0: values (canal-query's `json_output_data_options`, always on), the column types (`internal/command/query/result.go`, from the header canal-query streams with `stream_result_options.header_arrow_types`), and numbers sent back as strings (anfra-node's Query Input). See [the implementation](../implementations/revamp-query-result-data-types.md).

A query's answer is a table: `fields`, then `records`, one array per row. JSON's own types can't carry what a database returns: a `bigint` past 2^53, a `numeric(38, 10)`, a float that is `NaN`. Reading them as JSON numbers loses digits or fails, silently, in the readers that matter most (JavaScript, and anything that parses JSON into float64).

So a result's values are **encoded**, and its columns **declare their types**. A reader reads each value by its column's type, never by guessing from the JSON.

## The values

| A column of | Its values are | Example |
|---|---|---|
| numbers: integers, decimals, floats | strings, with every digit | `"9007199254740993"`, `"12345.67"`, `"0.30000000000000004"` |
| a float's non-finite values | `"NaN"`, `"Infinity"`, `"-Infinity"` | as JavaScript's `Number()` reads them |
| everything else | unchanged: strings, booleans, objects and lists as canal-query writes them | `"paid"`, `true`, `"2026-01-01"` |
| any type, no value | `null` | |

The same rules hold inside a list, a map or a struct, at any depth.

**Every number is a string, floats included,** so the rule is one line and a reader never needs more than the semantic type to decode a value. A float is written in its shortest form that reads back the same, and in an exponent only below 1e-6 or from 1e21, as JavaScript's `String(n)` writes it (`"1234567.5"`, not `"1.2345675e+06"`). The other reasons:

- An integer past 2^53, or a decimal, read as a JSON number in JavaScript loses digits without a sign.
- JSON has no number for `NaN` or the infinities.
- A column holds one kind of value: never strings in some rows and numbers in others.

The cost is a few bytes per value, and a reader that has to decode. Decoding is what the column types are for.

## The column types

Each column declares three types, each with its own job:

| Type | What it is | Who decides on it |
|---|---|---|
| `semantic_type` | What the column means, in AML's types, the ones a model declares and `core.show` gives a field (`text`, `number`, `date`, `datetime`, `truefalse`, `json`, `unknown`), plus `bit`. Present only when AQL typed the column. | Everyone. It is enough for nearly everything: how to decode, compare, format, chart. |
| `arrow_type` | The Arrow type canal-query read the column as: an integer and its width, a decimal and its precision and scale, a float, a timestamp and its unit and zone | A reader that needs more than the meaning: exact arithmetic (an integer or a decimal and its scale, vs a float), a date's precision, and, later, reading results as Arrow rather than JSON. |
| `db_type` | The database's own name for it: `int8`, `NUMERIC(38,10)`, `TIMESTAMP_NTZ` | No one. It is for development and debugging. Deciding on it couples a reader to one database, and its spelling varies by database and driver. |

### Where each comes from

- **`db_type`**: canal-query's header, which it sends before the first row (`types`).
- **`arrow_type`**: canal-query's header (`stream_result_options.header_arrow_types`, 2.17.1). See [Arrow types](#arrow-types).
- **`semantic_type`**:
  - For an AQL query, AMQL's type for the column: what the query means. A `date_trunc(…, 'month')` column is a `date` even where the database answers a timestamp. It is the type AMQL's type checker gives the column's expression when the query compiles, not the type its field declares. A count of a text field is a `number`, a truncated datetime is a `date`, and an adhoc expression has no field to declare one. anfra-node passes it with the column (`ExploreColumn.type`, anfra-node 0.4.0), beside `label` and `isMeasure`.
  - AMQL's types are a compiler's, and not all of them are meanings: `range` is AQL's own and never a column's, and `bit` is how AMQL compiles `truefalse` for a database with no boolean. The core maps them to semantic types in one table (`internal/command/query/result.go`), so AMQL's vocabulary can change without changing the API's. The schema's enum is that table's values, so it can neither miss a type the core answers nor list one it never does; the SDK's `SemanticType` fails its typecheck when it differs from the schema's (`api/backend.test.ts`):

    | AMQL type | `semantic_type` |
    |---|---|
    | `text`, `number`, `date`, `datetime`, `truefalse`, `json`, `unknown`: AML's own | the same |
    | `bit` | `bit` |
    | `time`, `duration`, `binary`, `composite`, `range`, and any type AMQL adds | `unknown`, until AML says what it means |

  - **No AQL, no semantic type.** A SQL query's columns, and an AQL query's that is not an explore, have none: `semantic_type` is absent, and a reader has `arrow_type` and `db_type`. It is never derived from either. Meaning comes from the database's own type, which a Holistics connector maps one way per database (`json` and `xml` as composite, an `interval` as a duration, an array as unknown). An Arrow type has lost that: canal-query reads Postgres' `jsonb`, `xml`, `interval`, `int4range` and arrays all as `utf8`, and a ClickHouse array as a `list`. A mapping from Arrow could only contradict the database's.

**`bit`** is a true or false stored as a bit, in a database with no boolean (SQL Server, Oracle). It is not `truefalse`, because its values are not always booleans: a SQL Server `BIT` column is written `true`/`false`, but AMQL's own `CASE WHEN … THEN 1 ELSE 0 END` answers an integer, written `"1"`/`"0"`. A reader treats both `true` and `"1"` as true. A `truefalse` that held `"0"` would read as true in JavaScript, silently.

**When AMQL and Arrow disagree**, each keeps its job: `semantic_type` says what a value means, `arrow_type` says how it was read. A column AMQL types `number` that canal-query read as `utf8` (a database type canal-query can't map) is a `number` written as text. A reader converts it as a number and checks it parses.

### Arrow types

An Arrow type is JSON that follows Arrow's own definition of a type: the `Type` union of `format/Schema.fbs`, the schema every Arrow IPC stream and file carries. `type` is the union member's name, and the other keys are that member's fields, with Schema.fbs's names and enum values:

| Schema.fbs | `arrow_type` |
|---|---|
| `table Int { bitWidth; is_signed }` | `{"type": "int", "bit_width": 64, "is_signed": true, "label": "int64"}` |
| `table FloatingPoint { precision }` | `{"type": "floating_point", "precision": "DOUBLE", "label": "float64"}` |
| `table Decimal { precision; scale; bitWidth }` | `{"type": "decimal", "precision": 38, "scale": 2, "bit_width": 128, "label": "decimal(38, 2)"}` |
| `table Timestamp { unit; timezone }` | `{"type": "timestamp", "unit": "MICROSECOND", "timezone": "UTC", "label": "timestamp[us, tz=UTC]"}` |
| `table Utf8 {}` | `{"type": "utf8", "label": "utf8"}` |
| `table List {}` | `{"type": "list", "children": [{"name": "item", "type": {…}}], "label": "list<item: int64, nullable>"}` |

Names are snake_case, the API's case: `FloatingPoint` is `floating_point`, `bitWidth` is `bit_width`, and `Struct_` is `struct`. A nested type (a list, a map, a struct) lists its children's names and types in `children`, as a Schema.fbs field does. A field Schema.fbs gives a default (`Decimal.bitWidth = 128`) is always written.

`label` is the type as canal-query spells it today, arrow-go's `DataType.String()`: for a person reading a result, or a log. No one decides on it. Its spelling is arrow-go's, not Arrow's, and PyArrow and arrow-rs spell the same type differently.

Why Schema.fbs, and not something else:

- **It is Arrow's canonical, versioned definition of a type.** A result streamed as Arrow later carries the same schema natively, so the JSON and the Arrow agree member for member.
- **Arrow's integration-test JSON** is close (Schema.fbs as camelCase JSON), but Arrow calls it "designed exclusively for Arrow's integration tests" and "not canonical", and its case is not the API's.
- **A library's strings** (`decimal(38, 2)`) have no specification, and each library writes its own. They are kept as `label`, but never decided on.

### The shape

The types go on `columns`, which already has one entry per field, in order, for both AQL and SQL queries:

```jsonc
"columns": [
  { "name": "revenue", "label": "Revenue", "isMeasure": true, …,
    "semantic_type": "number",
    "arrow_type": { "type": "decimal", "precision": 38, "scale": 2, "bit_width": 128, "label": "decimal(38, 2)" },
    "db_type": "NUMERIC(38,2)" }
]
```

One entry per column, rather than parallel lists beside `fields`, so the types can't drift from their column by an index. A SQL query's columns have a name and the types, and none of the dataset's facts (label, field, measure).

**Keys the core defines are snake_case, the API's case; keys passed through from amql keep amql's.** `columns` keeps amql's camelCase names (`fieldName`, `isMeasure`) as [data-apps.md](data-apps.md) explains: they are amql's, and the core passes them through unconverted. The types are not amql's. The core adds them, from canal-query and its own mapping, so they are `semantic_type`, `arrow_type` and `db_type`. A column mixes the two cases, and each key's case says whose it is. The core column becomes a type of its own (`query.Column`) that holds amql's facts and the types, since `db_type` and `arrow_type` are canal-query's and not anfra-node's.

## Each reader

| Reader | Does |
|---|---|
| **The SDK** | Hands a Data App the values and the types as they are. See [sdk-numbers.md](sdk-numbers.md). |
| **MCP and HTTP** | Get the strings and the types. An agent reads `"12345.67"` as text it can quote exactly. |
| **The CLI** | Prints values as they come. A YAML number shows quoted (`"17"`). Unquoting `number` columns by their type is a later nicety. |
| **CSV exports** | Write each value as it comes, so a number keeps every digit and a float has no exponent below 1e21. `NaN` and the infinities are written as words. |

## Values sent back

A caller sends values back in a query's Query Input: a control's value, a cross-filter's selection, often a value a result gave it. The contract runs the other way too: **a number is exact from the request to the SQL.**

**In `filters`**, a value for a `number` field (or a metric, or an aggregate such as a count) is **a string in JSON's number syntax**, as a result gives it: `"9007199254741007"`, `"21.25"`, `"1e-7"`. It is what the SDK sends, and a person or an agent writes `"17"`, not `17`.

Anything else is refused as a violation on that value, never rounded:

- A JSON number, even `17`, is refused with: send it as a string, `"17"`. Whoever wrote it may already have lost digits (JavaScript's `JSON.stringify` does, silently), and nothing after can tell. Taking the small ones that every reader keeps would need each number's source text, read past the JSON parse at every layer; one rule is simpler for every caller, and the refusal says what to send.
- A value that is not a number (`"abc"`, `"NaN"`, `"Infinity"`, `" 17"`, `true`) is refused, as it is today.
- An exponent past a float64's range (`"1e400"`, `"1e-400"`) is refused. Only a float is written with an exponent, so no result gives one, and it is most likely a mistake. A number past that range is written in full.

A filter's other values are not values of a `number` field and keep their JSON types: `last` and `next` count periods with a JSON integer (`3`).

Every layer keeps the digits it was given. The core and anfra-node pass the string through. anfra-node writes its digits into the AQL number literal, never through `Number()`, and writes an exponent out in full, since AQL's number literal has none. The SQL gets the number as it was sent.

**In `conditions`**, which are AQL text, a number is an AQL number literal, unquoted: `nums.id is 17`. AQL refuses `nums.id is "17"` as a type error, as it should: a condition is AQL, typed by AQL. Whoever writes the text writes a literal for the column's type. The SDK's `aql.literal` does this from `semantic_type`, writing a `number` column's string as its digits. AQL number literals are exact, so a condition keeps every digit.

Other types go back as they came: a date or datetime as its ISO string, a `truefalse` as a boolean, text as a string.

## What it takes

| Where | Change |
|---|---|
| canal-query | Floats in JavaScript's spelling (in progress). Arrow types in the header, as [Arrow types](#arrow-types) shapes them, with today's string as `label`. |
| amql / anfra-node | `ExploreColumn` gets the type AMQL's type checker gives its expression. Query Input writes a number's digits into its literal as given, and refuses a JSON number. |
| core | `query.Column`; the types from the header passed into `QueryResult`; the AMQL-to-semantic table. |
| SDK | The types on `columns`, and values sent back by column type: filter values as strings, AQL number literals ([sdk-numbers.md](sdk-numbers.md)). |

## Open questions

- **How a `json` value is written.** A Postgres `jsonb` arrives as a string of JSON text (`"{\"a\": 1}"`). Another database's JSON may be read as an Arrow struct, and written as an object. One spelling, like numbers', is to be decided.
- **`bit` as booleans.** AMQL could write `CAST(CASE … END AS BIT)` on SQL Server, so a `bit` is always written `true`/`false`, and could be `truefalse`. Oracle before 23ai has no boolean to cast to. To verify against both.
