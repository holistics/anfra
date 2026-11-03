/**
 * What "View underlying data" shows for a point: the measure it is about, the model that measure
 * aggregates (its base model), and the query that lists that model's rows behind the point. Pure:
 * the frame half decides with it as the menu opens, and the body draws what it plans.
 *
 * A point's measure resolves when its result column names a model: a model's measure
 * (`orders.total_revenue`), or an aggregate of one model field (`sum(orders.amount)`). A dataset
 * metric or any other expression doesn't, and gets no entry.
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
  /** The measure as AQL: the model's measure, or the aggregate rebuilt from the column. */
  expression: string;
  /** Why it can't be shown for this point, if it can't. */
  disabled?: string;
}

/** What one of the modal's columns is. */
export interface PlannedColumn {
  /** Its key in each row. */
  name: string;
  label: string;
  isMeasure: boolean;
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

/** Whether every row of the point leaves this measure empty. */
const isEmpty = (rows: readonly Row[], measure: string) => rows.every((row) => row[measure] === null || row[measure] === undefined);

/**
 * The measures a point offers: the one the definition named, or else each the query has. Only
 * those whose column resolves to a model; empty when none does, and so no entry shows.
 */
export function candidates (target: RowsTarget, columns: readonly ColumnMeta[], dataset: DatasetDescriptor): Candidate[] {
  const measures = columns.filter((column) => column.isMeasure && (target.measure === undefined || column.name === target.measure));
  return measures.flatMap((column) => {
    const model = column.modelId ? modelOf(dataset, column.modelId) : undefined;
    const expression = model && measureExpression(column, model);
    if (!model || !expression) return [];
    return [{
      column, model, expression, ...(isEmpty(target.rows, column.name) ? { disabled: NO_DATA } : {}),
    }];
  });
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

/** An identifier the explore can alias a column with, not already taken. */
function freeAlias (wanted: string, taken: Set<string>): string {
  const base = /^[A-Za-z_][A-Za-z0-9_]*$/.test(wanted) ? wanted : 'value';
  let alias = base;
  for (let n = 2; taken.has(alias); n += 1) alias = `${base}_${n}`;
  taken.add(alias);
  return alias;
}

/**
 * The modal for one measure of a point: the measure, then the base model's visible dimensions,
 * grouped by them, where the point's conditions and the query's own inputs hold.
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
  const measureAlias = freeAlias(candidate.column.name, taken);
  planned.push({ name: measureAlias, label: candidate.column.label || candidate.column.name, isMeasure: true });
  const sorts: BackendSort[] = [];
  const dimensionLines = dimensions.map((field) => {
    const alias = freeAlias(field.name, taken);
    planned.push({ name: alias, label: field.label || field.name, isMeasure: false });
    sorts.push({ field: alias, direction: DATE_TYPES.has(field.type) ? 'desc' : 'asc' });
    return `    ${alias}: ${candidate.model.name}.${field.name}`;
  });
  // Pages are LIMIT and OFFSET, so the order must be total, or a page may repeat rows another
  // showed: every dimension, which the rows are grouped by. Dates go first, newest first.
  sorts.sort((a, b) => Number(b.direction === 'desc') - Number(a.direction === 'desc'));
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
    '  measures {',
    `    ${measureAlias}: ${candidate.expression}`,
    '  }',
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
  // A query-local dimension can't become a condition: the rows span all its values.
  columns.filter((column) => !column.isMeasure && column.adhoc).forEach((column) => {
    chips.push({ text: `Not filtered by ${column.label || column.name}`, warning: true });
  });
  dropped.forEach((condition) => chips.push({ text: `Not filtered by ${condition}`, warning: true }));

  return {
    title: `View underlying data: ${candidate.column.label || candidate.column.name}`,
    chips,
    columns: planned,
    request: {
      dataset: base.dataset,
      aql,
      input: {
        filters: [...base.input.filters, ...pointFilters],
        conditions: [...base.input.conditions, ...pointConditions],
        sorts,
        dateDrills: [],
      },
      ...(base.timezone ? { timezone: base.timezone } : {}),
    },
  };
}
