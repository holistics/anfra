# Numbers in the SDK

> Status: built: the column types on `ColumnMeta`, AQL number literals by type (`app/aql.ts`), and a number filter refusing a JS number (`app/controls.ts`). Helpers are for later.

A query's numbers reach a Data App as the API answers them: strings, with every digit, under columns that say what they are ([result-values.md](result-values.md)). The SDK doesn't wrap or convert them, and adds no helpers yet.

## What the SDK exposes, and why

What the SDK could give a Data App about its values serves one of two purposes:

1. **Semantics:** what a value is, so a Data App can decide what to do with it. Does this column hold numbers? Is this one exact, or would reading it as a JS number lose digits? How is it spelled?
2. **Convenience:** doing it with ease: formatting, converting, sorting, extracting a column for a chart.

**Semantics are vital now; convenience comes later, gradually,** as Data Apps show which operations they repeat. A helper exposed is an API kept. A semantic one is needed for an app to be right. A convenience one is only nicer, and can wait until its shape is clear.

The semantics are already exposed by the result itself:

| A Data App needs to know | It reads |
|---|---|
| that a column holds numbers | `semantic_type: "number"` |
| how its values are spelled | the contract: a string in JSON's number syntax, or `"NaN"`, `"Infinity"`, `"-Infinity"`; `null` for none. JavaScript's `Number(v)` reads every one of them. |
| whether reading it as a JS number keeps every digit | `arrow_type`: a `floating_point` always does. An `int` of `bit_width` 64, or a `decimal`, may not. |
| how it was stored, while debugging | `db_type` |

Everything a Data App does with a number, it can do with the platform:

- Display: `Intl.NumberFormat`, which formats a decimal string exactly (`new Intl.NumberFormat('en').format("12345.67")`).
- Compute, plot or sort: `Number(v)`, lossy where `arrow_type` says so. Note that `Number(null)` is `0`, not `null`.
- Exact processing: a decimal library, which reads the strings as they are.

**Values:** `query.result.rows` as today. A `number` column's values are strings: `"9007199254740993"`, `"12345.67"`, `"NaN"`. `null` is `null`.

**Types:** `query.result.columns[i]`: `semantic_type`, `arrow_type`, `db_type`, as the API answers them, and always there. A Data App decides by `semantic_type`, never by `typeof`. The API's column leaves them out where nothing can say them (a SQL query, a query not yet run), but a Data App's query is an explore that ran, so `ColumnMeta` requires them: a host whose core API leaves them out fails the query, naming the column (`coreApiBackend`), rather than leaving a Data App to guess by `typeof`.

### Why values, not objects

- **The values are already hard to get wrong.** A string can't lose a digit. A wrapper would add a second representation of the same value, one that doesn't survive `postMessage`, `JSON.stringify` or a chart library unchanged.
- **What a Data App sees is what the API answers.** An author debugging a result reads the same `"12345.67"` in the inspect panel, in the network log and in `query.result.rows`.
- **A richer representation belongs to a later format, not to the JSON.** Efficient, exact processing in the browser is what streaming Arrow to the frontend would give: typed columns, 64-bit integers, decimals with their scale. Wrapping JSON values now would build half of that in a shape Arrow wouldn't keep.

## Helpers, later

The SDK should later add helpers that make processing values easier and harder to get wrong: affordance as well as convenience. Their shape needs more thought than this design gives it. A helper that reads a number, for example, has to serve an app that wants JS numbers and one that wants a decimal library alike, without turning into a per-call type mapping.

One idea is **data mappings**, as database clients have them (node-postgres's `types.setTypeParser`, mysql2's `typeCast`, Drizzle's `mode`). A Data App would choose once how values of a type are read, keyed by `semantic_type` or `arrow_type` and applied as rows are built. It would be opt-in, and the Data App would own the representation it picks: plain numbers, `BigInt`, a decimal library.

Exact arithmetic is not for the SDK now. A Data App asks the query for the aggregate, which the database computes exactly. In the browser, it would come with Arrow.

## The SDK's own uses

These aren't exposed. They are how the SDK itself handles values, and must follow [result-values.md](result-values.md#values-sent-back):

- **Filter values** (controls, cross-filters) go back as the strings they came as. That is the exact form, so a cross-filter on a large ID matches that ID and no other.
- **AQL literals.** A cross-filter that becomes an AQL expression writes a `number` column's value as an AQL number literal, `17`, not `"17"`: `aql.literal` takes the column's `semantic_type`. A quoted number is an AQL type error.
- **Selections** compare values as strings. Within one column that is exact, since canal-query writes each number one way: a decimal with its column's scale, a float in its shortest form.
- **A number filter's values are strings**, as results give them. A filter of type `number` refuses a JS number where the app hands it over (`setCondition`, `default`, `options`), naming the string to pass instead, since a JS number may already have lost digits and nothing after can tell. The SDK never converts one: that would hide the loss the core API's refusal exists to catch. A reader's typed-in value is kept as the string typed, and Query Input checks it is a number.

## What still goes wrong, and what catches it

Strings make digit loss impossible, but not every mistake loud: `"1" + 2` is `"12"`, and `"10" < "9"` is `true`. Those are the mistakes left, and these catch them:

- **The skill** (`build-anfra-app`: `API.md`, `example.html`) teaches deciding by `semantic_type`, reading a number with `Number(v)` where `arrow_type` says that keeps it, and displaying it with `Intl.NumberFormat`. Agents write most Data Apps, and they follow its examples.
- **The values are uniform.** Every number in every row is a string, so a mistake shows on the first run, in every row, not only for the rows with a large ID. That is the gap left by numbers that become strings only past 2^53.
