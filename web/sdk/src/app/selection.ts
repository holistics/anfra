/**
 * Turning the rows a reader picked into query conditions.
 *
 * Real dataset dimensions only. A measure's value describes the bucket rather than identifying it,
 * so filtering by it would narrow the target to rows that happen to share an aggregate — Dashboard
 * as Code skips them too (`buildFieldValueMap.ts`, "skip measures when cross-filtering"). An adhoc
 * column is skipped for a different reason: it is query-local, so a condition naming it would reach
 * a target query that has never heard of it and fail to resolve there. See docs/adr/0001.
 *
 * Column identity comes from the query's own last result (`ColumnMeta[]`), classified server-side —
 * a query is a raw AQL body now, so the SDK has no client-side declaration to read this from.
 */
import { ValidationError } from '../common/errors';
import { withSuggestion } from './suggest';
import {
  and as aqlAnd,
  dateRange as aqlDateRange,
  equals as aqlEquals,
  or as aqlOr,
  type AqlValue,
} from './aql';
import type { Query } from './query';
import type {
  ColumnMeta, ConditionValue, DateGrain, Row, Selection, SelectionCondition,
} from '../common/types';

const TIMESTAMP = /^(\d{4})-(\d{2})-(\d{2})(?:[T ](\d{2}):(\d{2})(?::(\d{2}))?)?/;

const FIXED_MS: Partial<Record<DateGrain, number>> = {
  week: 7 * 86400000,
  day: 86400000,
  hour: 3600000,
  minute: 60000,
};

interface Stamp {
  year: number;
  month: number;
  day: number;
  hour: number;
  minute: number;
  second: number;
  hasTime: boolean;
}

function parseStamp (value: ConditionValue): Stamp | undefined {
  if (typeof value !== 'string') return undefined;
  const match = TIMESTAMP.exec(value);
  if (!match) return undefined;

  return {
    year: Number(match[1]),
    month: Number(match[2]) - 1,
    day: Number(match[3]),
    hour: Number(match[4] ?? 0),
    minute: Number(match[5] ?? 0),
    second: Number(match[6] ?? 0),
    hasTime: match[4] !== undefined,
  };
}

/**
 * Formats through UTC getters on purpose.
 *
 * `Date.UTC` plus `getUTC*` is pure calendar arithmetic — it never consults the host's zone, so a
 * bucket labelled `2026-01-01` in the app's reporting timezone stays `2026-01-01`. Going through
 * local getters would shift it by the browser's offset and silently filter the wrong day.
 */
function format (ms: number, hasTime: boolean): string {
  const date = new Date(ms);
  const pad = (value: number): string => String(value).padStart(2, '0');
  const day = `${date.getUTCFullYear()}-${pad(date.getUTCMonth() + 1)}-${pad(date.getUTCDate())}`;
  if (!hasTime) return day;
  return `${day} ${pad(date.getUTCHours())}:${pad(date.getUTCMinutes())}:${pad(date.getUTCSeconds())}`;
}

function toMs (stamp: Stamp): number {
  return Date.UTC(stamp.year, stamp.month, stamp.day, stamp.hour, stamp.minute, stamp.second);
}

/** The same value, in the shape `addGrain` emits, so the two can be compared. */
function normalise (value: ConditionValue): string | undefined {
  const stamp = parseStamp(value);
  if (!stamp) return undefined;
  return format(toMs(stamp), stamp.hasTime);
}

/** The start of the bucket after `value`. Undefined when the value is not a timestamp. */
function addGrain (value: ConditionValue, grain: DateGrain): string | undefined {
  const stamp = parseStamp(value);
  if (!stamp) return undefined;

  const {
    year, month, day, hour, minute, second, hasTime,
  } = stamp;

  // Calendar grains move by component so month lengths and leap years take care of themselves;
  // the rest are exact durations.
  if (grain === 'year') return format(Date.UTC(year + 1, month, day, hour, minute, second), hasTime);
  if (grain === 'quarter') return format(Date.UTC(year, month + 3, day, hour, minute, second), hasTime);
  if (grain === 'month') return format(Date.UTC(year, month + 1, day, hour, minute, second), hasTime);

  return format(toMs(stamp) + FIXED_MS[grain]!, hasTime);
}

function grainOf (transformation?: string): DateGrain | undefined {
  if (!transformation?.startsWith('datetrunc ')) return undefined;
  return transformation.slice('datetrunc '.length) as DateGrain;
}

function distinct (values: ConditionValue[]): ConditionValue[] {
  const seen = new Set<string>();
  return values.filter((value) => {
    const key = `${typeof value}:${String(value)}`;
    if (seen.has(key)) return false;
    seen.add(key);
    return true;
  });
}

/** True when the buckets are consecutive, so one range covers them and nothing else. */
function isContiguous (values: ConditionValue[], grain: DateGrain): boolean {
  const stamps = values.map(normalise);
  if (stamps.some((stamp) => stamp === undefined)) return false;

  const sorted = (stamps as string[]).slice().sort();
  for (let index = 1; index < sorted.length; index += 1) {
    if (addGrain(sorted[index - 1], grain) !== sorted[index]) return false;
  }
  return true;
}

interface Derived {
  condition: SelectionCondition;
  /** Distinct target rows this condition admits, for the lossless check. */
  admits: number;
  lossy: boolean;
}

/**
 * A date-drilled bucket is an interval, not an instant: the reader clicked "January", and the rows
 * behind it run to the end of the month. Filtering `is '2026-01-01'` would match midnight alone.
 */
function dateRange (field: string, values: ConditionValue[], grain: DateGrain): Derived | undefined {
  const stamps = values.map(normalise);
  if (stamps.some((stamp) => stamp === undefined)) return undefined;

  const sorted = (stamps as string[]).slice().sort();
  const start = sorted[0];
  const end = addGrain(sorted[sorted.length - 1], grain);
  if (!end) return undefined;

  const contiguous = isContiguous(values, grain);

  return {
    // End-exclusive: `until` is the start of the bucket after the last one selected.
    condition: { field, operator: 'matches', values: [`${start} until ${end}`] },
    admits: contiguous ? values.length : Number.POSITIVE_INFINITY,
    lossy: !contiguous,
  };
}

function deriveOne (field: string, transformation: string | undefined, raw: unknown[]): Derived {
  const present = raw.filter((value) => value !== null && value !== undefined) as ConditionValue[];
  const values = distinct(present);

  if (!values.length) {
    return { condition: { field, operator: 'is_null', values: [] }, admits: 1, lossy: false };
  }

  // A mix of nulls and values wants `X, Y or null`, which the wire's single operator cannot say.
  const droppedNulls = present.length !== raw.length;

  const grain = grainOf(transformation);
  if (grain) {
    const range = dateRange(field, values, grain);
    if (range) return { ...range, lossy: range.lossy || droppedNulls };
  }

  return {
    condition: { field, operator: 'is', values },
    admits: values.length,
    lossy: droppedNulls,
  };
}

/** A column's field reference, under the same dotted-or-bare grammar the wire uses. */
function fieldReferenceOf (column: ColumnMeta): string {
  return column.modelId ? `${column.modelId}.${column.fieldName}` : column.fieldName;
}

/**
 * Builds the selection for `rows` picked out of `query`, or undefined when nothing is selected.
 *
 * `fields` names result column names — the space the author's rendering code is already in — and
 * narrows which of them take part.
 *
 * @param transformations The grain each date column is *currently drawn at*, keyed by field
 *   reference. Only a mapped date drill can supply one: a query is a raw AQL body now, so there is
 *   no declared "starting grain" to fall back to. Anything not named here is not date-bucketed.
 */
/**
 * The rows, exactly, as one AQL condition: an OR of per-row ANDs.
 *
 * Reached only when the structured form would be lossy. `undefined` when any value has no AQL
 * spelling, which leaves the lossy structured condition in place rather than sending something
 * that means something else.
 */
function selectionExpression (
  columnsByAlias: ReadonlyMap<string, ColumnMeta>,
  aliases: readonly string[],
  rows: readonly Row[],
  transformations: Readonly<Record<string, string>>,
): string | undefined {
  try {
    const perRow = rows.map((row) => aqlAnd(aliases.map((alias) => {
      const field = fieldReferenceOf(columnsByAlias.get(alias)!);
      const raw = row[alias] ?? null;

      // A drilled bucket is a range, not an instant — the same end-exclusive span the structured
      // form sends, just one per row instead of one widened to cover them all.
      const grain = grainOf(transformations[field]);
      if (grain && typeof raw === 'string') {
        const start = normalise(raw);
        const end = addGrain(raw, grain);
        if (start && end) return aqlDateRange(field, start, end);
      }

      if (raw !== null && typeof raw !== 'string' && typeof raw !== 'number' && typeof raw !== 'boolean') {
        throw new TypeError('no AQL spelling');
      }
      return aqlEquals(field, raw as AqlValue);
    })));

    return aqlOr(perRow);
  } catch {
    // Includes `literal` refusing NaN or Infinity. Falling back is the safe direction: a lossy
    // condition shows too much, a wrong one shows the wrong thing.
    return undefined;
  }
}

export function deriveSelection (
  query: Query,
  rows: readonly Row[],
  fields?: readonly string[],
  transformations: Readonly<Record<string, string>> = {},
): Selection | undefined {
  const columns = query.result?.columns ?? [];
  const columnsByAlias = new Map(columns.map((column) => [column.name, column]));

  fields?.forEach((alias) => {
    const column = columnsByAlias.get(alias);
    if (column) {
      if (column.adhoc) {
        throw new ValidationError(
          `Query '${query.name}' cannot cross-filter on '${alias}': it is a query-local AQL `
          + 'expression, which only this query\'s result carries. The target query would receive a '
          + 'condition naming a field it has never heard of.',
          query.name,
        );
      }
      if (column.isMeasure) {
        throw new ValidationError(
          `Query '${query.name}' cannot cross-filter on '${alias}': it is a measure, and a selection `
          + 'conditions dimensions only.',
          query.name,
        );
      }
      return;
    }
    throw new ValidationError(
      withSuggestion(
        `Query '${query.name}' has no column '${alias}' in its last result.`,
        alias,
        [...columnsByAlias.keys()],
      ),
      query.name,
    );
  });

  // Narrowed silently rather than refused: a query whose result is all adhoc or all measures
  // simply produces no selection when nothing is named, and one with a mix should still
  // cross-filter on the columns that do travel.
  const dimensionLike = columns.filter((column) => !column.isMeasure);
  const selectable = dimensionLike.filter((column) => !column.adhoc).map((column) => column.name);
  const aliases = fields ? [...fields] : selectable;
  if (!rows.length || !aliases.length) return undefined;

  const conditions: SelectionCondition[] = [];
  let admitted = 1;
  // Two separate reasons a selection can be lossy, and only one of them is fixable by spelling the
  // condition differently.
  //
  // Dropping an adhoc dimension conditions on less than the reader picked: an adhoc field is
  // query-local, so no condition of any shape can name it in the target. Measures are never
  // counted here — they were never eligible for selection to begin with, so their presence is not
  // a drop. Not flagged when `fields` was given — there the narrowing was the author's own choice.
  const droppedColumns = !fields && selectable.length !== dimensionLike.length;
  // The field-by-field collapse, which an AQL condition *can* say exactly.
  let collapseLossy = false;

  aliases.forEach((alias) => {
    const column = columnsByAlias.get(alias)!;
    const field = fieldReferenceOf(column);
    // Drilled grain first: sizing a range by a grain that is no longer on screen picks a span the
    // reader never selected — a third of a quarter, or thirty times a day.
    const derived = deriveOne(field, transformations[field], rows.map((row) => row[alias]));
    conditions.push(derived.condition);
    admitted *= derived.admits;
    collapseLossy = collapseLossy || derived.lossy;
  });

  // The per-field collapse is lossless exactly when the cross-product of what the conditions admit
  // is no larger than the set of rows the reader actually picked.
  const picked = new Set(rows.map((row) => JSON.stringify(aliases.map((alias) => row[alias] ?? null))));
  if (admitted > picked.size) collapseLossy = true;

  // Stable order, so a query's signature does not change just because two fields swapped places.
  const sorted = conditions.sort((a, b) => a.field.localeCompare(b.field));
  const base = { source: query.name, rows: [...rows], fields: aliases };

  if (collapseLossy) {
    // The structured array is ANDed field by field and cannot say `(A and B) or (C and D)`. An AQL
    // condition can, so a selection that would have over-selected goes over as one expression
    // instead. `droppedColumns` survives it: an expression fixes the collapse, never the fact that
    // a query-local or measure column cannot be named in the target.
    const expression = selectionExpression(columnsByAlias, aliases, rows, transformations);
    if (expression) {
      return {
        ...base, conditions: [], expression, lossy: droppedColumns,
      };
    }
  }

  return { ...base, conditions: sorted, lossy: droppedColumns || collapseLossy };
}
