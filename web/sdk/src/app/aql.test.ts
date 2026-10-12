import { describe, expect, it } from 'vitest';
import {
  and, dateRange, equals, literal, numberLiteral, or,
} from './aql';

// Written from char codes: raw, these terminate a line in most tooling.
const LS = String.fromCharCode(0x2028);
const PS = String.fromCharCode(0x2029);

/**
 * The grammar these strings have to satisfy is not checked here — this package has no parser, by
 * the same choice ADR 0001 records for author-written AQL. Every form below was validated
 * against `@holistics/amql`'s `parse` out of band, including that each literal round-trips back to
 * the original string, so these assertions pin the exact text that was proven to work.
 */
describe('literal', () => {
  it('quotes a string with backslash escapes, not doubled quotes', () => {
    // AQL rejects SQL's `""` form outright, so this is the difference between working and not.
    expect(literal('he said "hi"')).toBe('"he said \\"hi\\""');
    expect(literal('back\\slash')).toBe('"back\\\\slash"');
  });

  it('leaves an apostrophe alone inside double quotes', () => {
    expect(literal("it's")).toBe('"it\'s"');
  });

  it('escapes control characters rather than emitting them raw', () => {
    // A raw newline inside a literal is a parse error, not a multi-line string.
    expect(literal('new\nline')).toBe('"new\\nline"');
    expect(literal('tab\there')).toBe('"tab\\there"');
  });

  it('escapes the two line terminators JSON leaves raw', () => {
    // The whole reason this is not just `JSON.stringify`: both are legal unescaped in JSON and
    // both end a line in AQL's lexer.
    expect(literal(`a${LS}b`)).toBe('"a\\u2028b"');
    expect(literal(`a${PS}b`)).toBe('"a\\u2029b"');
    expect(literal(`a${LS}b`)).not.toContain(LS);
  });

  it('writes numbers without exponent notation', () => {
    expect(literal(0)).toBe('0');
    expect(literal(-12.5)).toBe('-12.5');
    // `String(1e21)` is "1e+21", which AQL's number literal has no form for.
    expect(literal(1e21)).toBe('1000000000000000000000');
  });

  it('refuses a number with no AQL spelling', () => {
    // Coercing these would produce a condition that silently matches nothing.
    expect(() => literal(NaN)).toThrowError(RangeError);
    expect(() => literal(Infinity)).toThrowError(RangeError);
  });

  it("writes a number column's value from its digits, unquoted", () => {
    // Through a JS number, the first would round to ...992.
    expect(literal('9007199254740993', 'number')).toBe('9007199254740993');
    expect(literal('12345.67', 'number')).toBe('12345.67');
    expect(literal('0.10', 'number')).toBe('0.10');
    expect(literal('-0.5', 'number')).toBe('-0.5');
    // Other columns' strings stay strings.
    expect(literal('17', 'text')).toBe('"17"');
    expect(literal('17')).toBe('"17"');
  });

  it("refuses a number column's value with no AQL spelling, as it refuses a JS one", () => {
    for (const value of ['NaN', 'Infinity', '-Infinity']) {
      expect(() => literal(value, 'number'), value).toThrowError(RangeError);
    }
  });

  it("refuses a number column's value that is not a number", () => {
    for (const value of ['abc', '', '1.', '.5', '01', '+1', '1e']) {
      expect(() => literal(value, 'number'), value).toThrowError(TypeError);
    }
  });

  it('writes booleans and null bare', () => {
    expect(literal(true)).toBe('true');
    expect(literal(false)).toBe('false');
    expect(literal(null)).toBe('null');
  });
});

describe('numberLiteral', () => {
  it('writes an exponent out in full, which AQL has no form for', () => {
    expect(numberLiteral('1e-7')).toBe('0.0000001');
    expect(numberLiteral('1e+21')).toBe('1000000000000000000000');
    expect(numberLiteral('1.5e21')).toBe('1500000000000000000000');
    expect(numberLiteral('-1.25E2')).toBe('-125');
    expect(numberLiteral('12.5e-1')).toBe('1.25');
    expect(numberLiteral('0.5e1')).toBe('5');
    expect(numberLiteral('-2.5e-3')).toBe('-0.0025');
    expect(numberLiteral('7e0')).toBe('7');
  });
});

describe('equals', () => {
  it('uses the infix form for a value', () => {
    expect(equals('orders.status', 'open')).toBe('orders.status is "open"');
  });

  it('uses the function form for null, which `is` will not take as an operand', () => {
    expect(equals('orders.status', null)).toBe('is(orders.status, null)');
  });

  it("writes a number column's value as a number", () => {
    expect(equals('orders.id', '17', 'number')).toBe('orders.id is 17');
  });

  it('escapes the value, not the field', () => {
    expect(equals('orders.name', 'a"b')).toBe('orders.name is "a\\"b"');
  });
});

describe('dateRange', () => {
  it('emits the end-exclusive date phrase with boundaries unquoted', () => {
    // `@(...)` is its own grammar; quoting the boundaries breaks it. They are SDK-formatted, never
    // row text, so nothing here needs escaping.
    expect(dateRange('orders.t', '2026-01-01', '2026-02-01'))
      .toBe('orders.t matches @(2026-01-01 until 2026-02-01)');
  });
});

describe('and / or', () => {
  it('combines operands', () => {
    expect(or(['a is 1', 'b is 2'])).toBe('or(a is 1, b is 2)');
    expect(and(['a is 1', 'b is 2'])).toBe('and(a is 1, b is 2)');
  });

  it('nests', () => {
    expect(or([and(['a is 1', 'b is 2']), and(['a is 3', 'b is 4'])]))
      .toBe('or(and(a is 1, b is 2), and(a is 3, b is 4))');
  });

  it('drops the call for a single operand', () => {
    // Legal either way, but the wrapper survives into `executedAql`, where someone reads it.
    expect(or(['a is 1'])).toBe('a is 1');
  });

  it('refuses an empty operand list', () => {
    expect(() => or([])).toThrowError(RangeError);
  });
});
