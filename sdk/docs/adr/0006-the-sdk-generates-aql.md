# The SDK generates AQL, and `src/aql.ts` is the trust boundary

The SDK writes AQL in exactly one place: a selection the structured `filters` array cannot express
(ADR 0005), which becomes one `aql_conditions` entry — an OR of per-row ANDs. Everywhere else it
passes AQL through without reading it.

Every literal in that generated text comes from a result row. A product name containing a quote is
not an edge case, so escaping is the whole of the job, and it is deliberately confined to one small
module rather than spread across the selection code that calls it.

## The escaping rules, and why each one is there

These were established against the real parser rather than assumed, which matters because three of
the four are not what a first guess would produce.

**`JSON.stringify` is the base.** AQL string literals take backslash escapes — `\"`, `\\`, `\n` —
and reject SQL's doubled-quote form. That is JSON's shape exactly, and its output round-trips back
to the original string through the AQL parser.

**U+2028 and U+2029 are escaped by hand.** They are legal raw inside a JSON string, so
`JSON.stringify` leaves them alone, and they terminate a line in AQL's lexer. This is the one place
JSON's rules and AQL's diverge, and it is the reason this cannot be "just use `JSON.stringify`".
The two characters are built from char codes in the source rather than written literally, because
raw they are a hazard to every editor that opens the file.

**Numbers past 1e21 are expanded.** `String(1e21)` is `"1e+21"`, and AQL's number literal has no
exponent form. A fixed expansion keeps the value readable by the parser.

**`NaN` and `Infinity` are refused, not coerced.** Neither has an AQL spelling, and a coerced value
produces a condition that silently matches nothing — a wrong answer that looks like an empty result.
Throwing is what lets the caller fall back to the lossy structured form instead, which shows too
much rather than too little.

Two smaller shapes belong to the same boundary. `null` needs the prefix form `is(field, null)`,
because `is` does not accept null as an infix operand. And date bucket boundaries are placed
verbatim inside `@(start until end)` — they are SDK-formatted, never row text, and the date literal
is its own grammar that quoting would break.

## The package holds no parser, and will not grow one

Nothing here validates grammar. A malformed expression fails at execute as a `QueryError`, the same
trade ADR 0001 makes for author-written AQL. Adding a parser to check the SDK's own output would be
checking generated text against a grammar the generator already encodes — the useful version of that
check is the round-trip property the escaping rules were established against, and it belongs in this
package's tests rather than in its runtime.

## Consequences

- **Escaping bugs are correctness bugs, not crashes.** A mis-escaped value produces a syntactically
  valid condition matching the wrong rows. The tests carry the round-trip cases for that reason, and
  they are the guard — there is no runtime signal for getting this wrong.
- **A one-operand `and`/`or` is collapsed.** Legal, but the extra call survives into `executedAql`,
  where a person has to read it in the Inspect panel.
- **This module is the place to look when a selection filters wrongly.** Confining generation to one
  file is what makes that statement true, and is worth more than the small amount of indirection it
  costs the selection code.
