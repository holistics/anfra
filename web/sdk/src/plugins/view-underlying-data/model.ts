/**
 * What "View underlying data" shows for a point: the measure it is about, the model that measure
 * aggregates (its base model), and the query that lists that model's rows behind the point. Pure:
 * the frame half decides with it as the menu opens, and the body draws what it plans.
 *
 * A model's measure (`orders.total_revenue`) or a plain aggregate of one model field
 * (`sum(orders.amount)`) resolves by the model its result column names, and shows that model's rows
 * without the measure. Any other measure (a dataset metric, a ratio, a filtered aggregate written
 * either way, `count(orders.id) | where(...)` or `orders | where(...) | count(orders.id)`, a metric
 * the query declares) resolves by the models anfra-node says it aggregates, one candidate per
 * model, and shows the measure as a column, recomputed for each row: a ratio has its value per row,
 * and a filtered measure 0 or nothing where its filter excludes the row. A measure with no models
 * gets no entry.
 */
import type {
  BackendCondition, BackendFilter, BackendSort, ColumnMeta, DatasetDescriptor, QueryRequest, Row, SelectionCondition,
} from '../../common/types';
import {
  dateRange as aqlDateRange, equals as aqlEquals, or as aqlOr, type AqlValue,
} from '../../app/aql';
import type { RowsTarget } from '../../common/menu';
import { bareNames, exploreFilters, preambleOf } from './aqlText';

type Model = DatasetDescriptor['data_models'][number];
type Field = Model['fields'][number];

const DATE_TYPES = new Set(['date', 'datetime', 'timestamp']);

/** How many rows a page of the table holds. */
export const PAGE_SIZE = 100;

/** A measure of the point, with the model it aggregates. */
export interface Candidate {
  column: ColumnMeta;
  model: Model;
  /**
   * The measure as AQL: the model's measure or the aggregate rebuilt from the column, or else the
   * AQL the query wrote it as.
   */
  expression: string;
  /**
   * Whether the measure is a column of every row. Not for a model measure or a plain aggregate,
   * where a row is usually one record, and a count is 1 and a sum repeats a field.
   */
  showMeasure: boolean;
  /** Why it can't be shown for this point, if it can't. */
  disabled?: string;
}

/** What one of the modal's columns is. */
export interface PlannedColumn {
  /** Its key in each row. */
  name: string;
  label: string;
  /** Its values are numbers: aligned right. */
  numeric: boolean;
}

/** A condition the point puts on the rows, as the reader reads it; `warning` for one it couldn't. */
export interface Chip {
  text: string;
  warning?: boolean;
}

/** Everything the modal needs: what it says, and the request its pages are made from. */
export interface Plan {
  title: string;
  chips: Chip[];
  columns: PlannedColumn[];
  /** The rows behind the point, with no page: each page adds one, the download none. */
  request: QueryRequest;
}

export const NO_DATA = 'No data is available for this point';

/** `count distinct` → `count_distinct`: an aggregation's name as an AQL function, the reverse of anfra-node's naming. */
export function aggregateFunction (aggregation: string): string | undefined {
  const name = aggregation.trim().toLowerCase().replace(/\s+/g, '_');
  return /^[a-z][a-z0-9_]*$/.test(name) && name !== 'custom' ? name : undefined;
}

const modelOf = (dataset: DatasetDescriptor, name: string) => dataset.data_models.find((model) => model.name === name);

/** The measure as AQL, when its column names a model: the model's own measure, or the aggregate of one of its fields. */
function measureExpression (column: ColumnMeta, model: Model): string | undefined {
  const field = model.fields.find((candidate) => candidate.name === column.fieldName);
  if (!field || field.is_param) return undefined;
  const ref = `${model.name}.${field.name}`;
  if (field.is_custom_measure) return ref;
  const fn = column.aggregation ? aggregateFunction(column.aggregation) : undefined;
  return fn ? `${fn}(${ref})` : undefined;
}

/** A value as the reader reads it: a number in their locale, anything else as the result has it. */
export function display (value: unknown, column: ColumnMeta | undefined, format: Intl.NumberFormat): string {
  if (value === null || value === undefined) return '';
  if (column?.semantic_type === 'number' && (typeof value === 'string' || typeof value === 'number')) {
    // A number arrives as a string with every digit; formatting the string keeps them.
    if (typeof value === 'string' && !/^-?\d+(\.\d+)?(e[+-]?\d+)?$/i.test(value)) return value;
    return format.format(value as number);
  }
  if (typeof value === 'object') return JSON.stringify(value);
  return String(value);
}

/** Whether every row of the point leaves this measure empty. */
const isEmpty = (rows: readonly Row[], measure: string) => rows.every((row) => row[measure] === null || row[measure] === undefined);

/** AQL with no whitespace, to tell whether two spellings are the same. */
const compact = (aql: string) => aql.replace(/\s+/g, '');

/**
 * A measure's candidates, before the point decides whether they can be shown. A column names a
 * model and an aggregation for more than its plain aggregate: `orders | where(...) | count(orders.id)`
 * names `orders.id` and `count` too. It is the plain aggregate only when the AQL the query wrote is
 * that aggregate, or anfra-node didn't say the AQL.
 */
function measureCandidates (column: ColumnMeta, dataset: DatasetDescriptor): Omit<Candidate, 'disabled'>[] {
  const model = column.modelId ? modelOf(dataset, column.modelId) : undefined;
  const expression = model && measureExpression(column, model);
  if (model && expression && (!column.expression || compact(column.expression) === compact(expression))) {
    return [{ column, model, expression, showMeasure: false }];
  }
  if (!column.expression) return [];
  const { expression: written } = column;
  return (column.models ?? []).flatMap((name) => {
    const of = modelOf(dataset, name);
    return of ? [{ column, model: of, expression: written, showMeasure: true }] : [];
  });
}

/**
 * The measures a point offers: the one the definition named, or else each the query has. Each
 * once per model it resolves to; empty when none resolves, and so no entry shows.
 */
export function candidates (target: RowsTarget, columns: readonly ColumnMeta[], dataset: DatasetDescriptor): Candidate[] {
  const measures = columns.filter((column) => column.isMeasure && (target.measure === undefined || column.name === target.measure));
  return measures.flatMap((column) => measureCandidates(column, dataset).map((candidate) => ({
    ...candidate, ...(isEmpty(target.rows, column.name) ? { disabled: NO_DATA } : {}),
  })));
}

const LABEL = 'View underlying data';

/** What a candidate is called: its measure, and its model when the measure aggregates several. */
function nameOf (candidate: Candidate): string {
  const measure = candidate.column.label || candidate.column.name;
  return (candidate.column.models?.length ?? 0) > 1 ? `${measure} (${candidate.model.label || candidate.model.name})` : measure;
}

/** A candidate's menu entry: the bare entry when the point offers only it, else naming it. */
export function entryLabel (candidate: Candidate, offered: readonly Candidate[]): string {
  return offered.length > 1 ? `${LABEL}: ${nameOf(candidate)}` : LABEL;
}

/** The base model's fields a reader sees: its dimensions, not hidden, in the model's order. */
export function visibleDimensions (model: Model): Field[] {
  return model.fields.filter((field) => !field.is_custom_measure && !field.is_hidden && !field.is_param);
}

const OPERATOR_WORDS: Record<string, string> = {
  is: 'is', is_not: 'is not', matches: 'in', contains: 'contains', greater_than: '>', less_than: '<', between: 'between',
};

/** The dataset's field a `model.field` reference names, if it has one. */
function fieldOf (dataset: DatasetDescriptor, ref: string): Field | undefined {
  const dot = ref.lastIndexOf('.');
  return dot > 0 ? modelOf(dataset, ref.slice(0, dot))?.fields.find((field) => field.name === ref.slice(dot + 1)) : undefined;
}

/** A field reference's label, from the dataset; its own name when the dataset has none. */
function labelOf (dataset: DatasetDescriptor, ref: string): string {
  return fieldOf(dataset, ref)?.label || ref.slice(ref.lastIndexOf('.') + 1);
}

function chipOf (dataset: DatasetDescriptor, condition: SelectionCondition): Chip {
  const label = labelOf(dataset, condition.field);
  if (condition.operator === 'is_null') return { text: `${label} is empty` };
  const word = OPERATOR_WORDS[condition.operator] ?? condition.operator.replace(/_/g, ' ');
  return { text: `${label} ${word} ${condition.values.map(String).join(', ')}` };
}

/**
 * A condition as AQL, for a field the query declares itself (`dimension orders.kind = ...;`): the
 * API checks a structured filter's field against the dataset, while an AQL condition compiles with
 * the query's declarations. Undefined when it has no AQL spelling here.
 */
function asExpression (condition: SelectionCondition, columns: readonly ColumnMeta[]): string | undefined {
  const { field, operator, values } = condition;
  const semanticType = columns.find((column) => column.modelId && `${column.modelId}.${column.fieldName}` === field)?.semantic_type;
  try {
    if (operator === 'is_null') return aqlEquals(field, null);
    if (operator === 'is' && values.length) return aqlOr(values.map((value) => aqlEquals(field, value as AqlValue, semanticType)));
    if (operator === 'matches' && values.length === 1 && typeof values[0] === 'string') {
      const [start, end] = values[0].split(' until ');
      if (start && end) return aqlDateRange(field, start, end);
    }
  } catch { /* a value AQL can't spell */ }
  return undefined;
}

/**
 * What the modal is of: the point's value and its measure (`1,234 players`), or the measure alone
 * when the point has several rows, and so no one value; with its model when it aggregates several.
 */
function subjectOf (target: RowsTarget, candidate: Candidate): string {
  const { column } = candidate;
  const label = nameOf(candidate);
  if (target.rows.length !== 1) return label;
  const text = display(target.rows[0][column.name], column, new Intl.NumberFormat(undefined, { maximumFractionDigits: 20 }));
  return text ? `${text} ${label}` : label;
}

/** An identifier the explore can alias a column with, not already taken. */
function freeAlias (wanted: string, taken: Set<string>): string {
  const base = /^[A-Za-z_][A-Za-z0-9_]*$/.test(wanted) ? wanted : 'value';
  let alias = base;
  for (let n = 2; taken.has(alias); n += 1) alias = `${base}_${n}`;
  taken.add(alias);
  return alias;
}

/**
 * The modal for one measure of a point: the base model's visible dimensions, grouped by them, where
 * the point's conditions and the query's own inputs hold. A model measure or a plain aggregate isn't
 * a column: a row is usually one record (a visible dimension is unique), where a count is 1 and a
 * sum repeats a field; the point's value goes in the title instead. Any other measure is a column,
 * recomputed for each row, and so is the measure of a model with no visible dimension.
 *
 * Built on the query's request as it runs now: its declarations, its own `filters { }`, its
 * controls' filters, its selection, its timezone. A condition of its `filters` that names one of the
 * query's columns by its alias can't be kept, since the explore has its own: it is left out, and
 * said so.
 * Its sorts and date drills are dropped: a sort names one of the query's own columns, which the
 * explore may not have, and a drill would truncate the model's dates, while the point's condition
 * already spans the bucket the reader clicked. The rows are sorted by every dimension instead, so
 * each page follows the last.
 */
export function plan (target: RowsTarget, candidate: Candidate, base: QueryRequest, columns: readonly ColumnMeta[], dataset: DatasetDescriptor): Plan {
  const dimensions = visibleDimensions(candidate.model);
  const taken = new Set<string>();
  const planned: PlannedColumn[] = [];
  const sorts: BackendSort[] = [];
  const dimensionLines = dimensions.map((field) => {
    const alias = freeAlias(field.name, taken);
    planned.push({ name: alias, label: field.label || field.name, numeric: field.type === 'number' });
    sorts.push({ field: alias, direction: DATE_TYPES.has(field.type) ? 'desc' : 'asc' });
    return `    ${alias}: ${candidate.model.name}.${field.name}`;
  });
  // Pages are LIMIT and OFFSET, so the order must be total, or a page may repeat rows another
  // showed: every dimension, which the rows are grouped by. Dates go first, newest first.
  sorts.sort((a, b) => Number(b.direction === 'desc') - Number(a.direction === 'desc'));
  const measureLines: string[] = [];
  if (candidate.showMeasure || !dimensionLines.length) {
    const alias = freeAlias(candidate.column.name, taken);
    planned.push({ name: alias, label: candidate.column.label || candidate.column.name, numeric: true });
    measureLines.push(`    ${alias}: ${candidate.expression}`);
  }
  if (candidate.showMeasure && dimensions.length) {
    // An explore leaves out a row where every measure is empty, as a filtered measure is on a row
    // its filter excludes: a count of the model's own rows keeps it. Not one of the modal's
    // columns, though a download has it.
    const alias = freeAlias('record_count', taken);
    measureLines.push(`    ${alias}: count(${candidate.model.name}.${dimensions[0].name})`);
  }
  const preamble = preambleOf(base.aql);
  const aliases = new Set(columns.map((column) => column.name));
  const kept: string[] = [];
  const dropped: string[] = [];
  exploreFilters(base.aql).forEach((condition) => {
    ([...bareNames(condition)].some((name) => aliases.has(name)) ? dropped : kept).push(condition);
  });
  const aql = [
    ...(preamble ? [preamble] : []),
    'explore {',
    ...(dimensionLines.length ? ['  dimensions {', dimensionLines.join(',\n'), '  }'] : []),
    ...(measureLines.length ? ['  measures {', measureLines.join(',\n'), '  }'] : []),
    ...(kept.length ? ['  filters {', kept.map((condition) => `    ${condition}`).join(',\n'), '  }'] : []),
    '}',
  ].join('\n');

  const pointFilters: BackendFilter[] = [];
  const pointConditions: BackendCondition[] = target.expression ? [{ expr: target.expression }] : [];
  target.conditions.forEach((condition) => {
    const expr = fieldOf(dataset, condition.field) ? undefined : asExpression(condition, columns);
    if (expr) pointConditions.push({ expr });
    else pointFilters.push({ field: condition.field, operator: condition.operator, values: [...condition.values] });
  });
  const chips: Chip[] = target.expression
    ? [{ text: 'Matches the selected point' }]
    : target.conditions.map((condition) => chipOf(dataset, condition));
  // A dimension the point has no condition on (query-local, or left out by `fields`): the rows span
  // all its values.
  const conditioned = new Set(target.fields);
  columns.filter((column) => !column.isMeasure && !conditioned.has(column.name)).forEach((column) => {
    chips.push({ text: `Not filtered by ${column.label || column.name}`, warning: true });
  });
  dropped.forEach((condition) => chips.push({ text: `Not filtered by ${condition}`, warning: true }));

  return {
    title: `${LABEL}: ${subjectOf(target, candidate)}`,
    chips,
    columns: planned,
    request: {
      dataset: base.dataset,
      aql,
      input: {
        ...base.input,
        filters: [...base.input.filters, ...pointFilters],
        conditions: [...base.input.conditions, ...pointConditions],
        sorts,
        dateDrills: [],
      },
      // Cached as the query it drills into is.
      ...(base.cacheDuration ? { cacheDuration: base.cacheDuration } : {}),
    },
  };
}
