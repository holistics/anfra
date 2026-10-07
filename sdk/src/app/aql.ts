/**
 * Building AQL expression text.
 *
 * The SDK generates AQL in one place only: a selection the structured `filters` array cannot
 * express, which becomes one `aql_conditions` entry. Every value in that text comes from a result
 * row, so escaping is the whole job here — a product name containing a quote is not an edge case,
 * it is Tuesday.
 *
 * Escaping is JSON's, with one addition. AQL string literals take backslash escapes (`\"`, `\\`,
 * `\n`) and reject SQL's doubled-quote form, so `JSON.stringify` produces exactly the right shape
 * and its output round-trips back to the original string through the parser. What it does not
 * escape is U+2028 and U+2029, which are legal raw inside a JSON string but terminate a line in
 * AQL's lexer — those two are escaped by hand.
 *
 * Nothing here validates the *grammar*. A bad expression surfaces as a `QueryError` at execute,
 * the same way an author's own AQL body does; see docs/adr/0001.
 */

// Built from char codes rather than written literally: raw, these two characters terminate a
// line in most tooling, so a source file carrying them is a hazard to every editor that opens it.
const LINE_SEPARATOR = String.fromCharCode(0x2028);
const PARAGRAPH_SEPARATOR = String.fromCharCode(0x2029);

/** What a result row can hold, plus the null a grouped row uses for "no value". */
export type AqlValue = string | number | boolean | null;

/**
 * One value as AQL source text.
 *
 * Numbers are refused rather than coerced when they are not finite: `NaN` and `Infinity` have no
 * AQL spelling, and emitting them would produce a condition that silently matches nothing.
 */
export function literal (value: AqlValue): string {
  if (value === null) return 'null';
  if (typeof value === 'boolean') return value ? 'true' : 'false';

  if (typeof value === 'number') {
    if (!Number.isFinite(value)) {
      throw new RangeError(`Cannot express ${value} in AQL.`);
    }
    // Exponent notation is what `String()` reaches for past 1e21, and AQL's number literal has no
    // exponent form. A fixed expansion keeps the value readable by the parser.
    return Math.abs(value) >= 1e21 ? BigInt(value).toString() : String(value);
  }

  return JSON.stringify(value)
    .split(LINE_SEPARATOR)
    .join('\\u2028')
    .split(PARAGRAPH_SEPARATOR)
    .join('\\u2029');
}

/** `<field> is <value>`, or the null form, which `is` does not accept as an infix operand. */
export function equals (field: string, value: AqlValue): string {
  return value === null ? `is(${field}, null)` : `${field} is ${literal(value)}`;
}

/**
 * `<field> matches @(<start> until <end>)` — the end-exclusive date phrase.
 *
 * `start` and `end` are SDK-formatted bucket boundaries, never row text, so they are placed
 * verbatim: the `@(...)` date literal is its own grammar and quoting them would break it.
 */
export function dateRange (field: string, start: string, end: string): string {
  return `${field} matches @(${start} until ${end})`;
}

function combine (fn: 'and' | 'or', parts: readonly string[]): string {
  if (parts.length === 0) {
    throw new RangeError(`\`${fn}\` needs at least one operand.`);
  }
  // A one-operand and/or is legal but noisy, and the extra call survives into `executedAql` where
  // someone has to read it.
  return parts.length === 1 ? parts[0] : `${fn}(${parts.join(', ')})`;
}

export const and = (parts: readonly string[]): string => combine('and', parts);
export const or = (parts: readonly string[]): string => combine('or', parts);
