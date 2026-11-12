import type { Query } from '../query';
import type { Mapping } from '../mapping';
import type {
  Backend,
  BackendCondition,
  BackendDateDrill,
  BackendExportRequest,
  BackendFilter,
  BackendQueryRequest,
  BackendQueryResult,
  Condition,
  DateGrain,
  ExportOptions,
  QueryInput,
  QueryRequest,
  QueryResult,
  SelectionCondition,
} from '../../common/types';

export interface SubmitContext {
  backend: Backend;
  timezone?: string;
  bustCache?: boolean;
}

/**
 * The filter one control mapping contributes, if any.
 *
 * Mapped conditions (runtime, arriving through controls) and selection conditions (runtime,
 * arriving through cross-filter edges) both land in `input.filters` and are ANDed on top of
 * whatever the query's own AQL already filters — which is what dashboards do too, appending
 * cross-filter conditions to `viz_setting.filters` alongside the mapped ones.
 */
function toFilter (mapping: Mapping, condition: Condition | undefined): BackendFilter | undefined {
  // A control the reader has not set, or has cleared, contributes nothing rather than an
  // always-false clause. A date drill is not a filter at all; see `toDateDrill`.
  if (!condition || condition.operator === 'none' || condition.operator === 'transform_date_drill') return undefined;

  const needsValues = !['is_null', 'not_null', 'is_true', 'is_false', 'transform_pop_none'].includes(condition.operator);
  if (needsValues && !condition.values?.length) return undefined;

  // The aggregation belongs to the mapping, not the condition: the control supplies the operator
  // and values, the edge decides whether they apply to the column or to an aggregate over it.
  return {
    field: mapping.field,
    operator: condition.operator,
    values: condition.values ?? [],
    ...(condition.modifier ? { modifier: condition.modifier } : {}),
    ...(mapping.aggregation ? { aggregation: mapping.aggregation } : {}),
  };
}

/**
 * The date drill one mapping contributes, if any. A drill control still spells its state as a
 * `transform_date_drill` condition (`datetrunc <grain>`) internally, so selections can size their
 * ranges by it, but the backend receives it as what it is: a grain for the mapped field.
 */
function toDateDrill (mapping: Mapping, condition: Condition | undefined): BackendDateDrill | undefined {
  if (condition?.operator !== 'transform_date_drill') return undefined;
  const transformation = condition.values?.[0];
  if (typeof transformation !== 'string') return undefined;
  const grain = transformation.replace(/^datetrunc\s+/, '') as DateGrain;
  return { field: mapping.field, grain };
}

function toResult (query: Query, data: BackendQueryResult): QueryResult {
  // Classified by the backend, since a query is a raw AQL body and this package has no field
  // knowledge of its own to label columns from. See docs/adr/0001.
  const columns = data.columns ?? [];

  const rows = (data.values ?? []).map((row) => {
    const record: Record<string, unknown> = {};
    columns.forEach((column, index) => {
      record[column.name] = row[index];
    });
    return record;
  });

  const { pageSize } = query;
  return {
    columns,
    rows,
    meta: {
      // Only a paged query has a page to report.
      ...(pageSize !== undefined
        ? {
            page: data.meta?.page ?? query.page,
            pageSize: data.meta?.pageSize ?? pageSize,
            ...(data.meta?.hasMore !== undefined ? { hasMore: data.meta.hasMore } : {}),
          }
        : {}),
      numRows: data.meta?.numRows ?? rows.length,
    },
    ...(data.debug ? { debug: data.debug } : {}),
  };
}

/** What a selection contributes: structured conditions, or the one expression that replaced them. */
export interface SelectionInput {
  conditions: readonly SelectionCondition[];
  expression?: string;
}

/**
 * What a run and an export of a query share: the query, and everything that reaches it (mapped
 * controls, the selection, the reader's sort, the timezone). One builder, so a page of a query and
 * its export cannot drift apart.
 */
export function buildQuery (
  query: Query,
  mappings: Mapping[],
  selection: SelectionInput,
  context: Pick<SubmitContext, 'timezone'>,
): QueryRequest {
  const mappedFilters = mappings
    .map((mapping) => toFilter(mapping, mapping.from.appliedCondition))
    .filter((filter): filter is BackendFilter => filter !== undefined);

  const dateDrills = mappings
    .map((mapping) => toDateDrill(mapping, mapping.from.appliedCondition))
    .filter((drill): drill is BackendDateDrill => drill !== undefined);

  // Derived from result values, so unlike a reader-driven control it never has the empty-value or
  // `none` cases. Values go over as they came back: strings, numbers and booleans.
  const selectionFilters: BackendFilter[] = selection.conditions.map((condition) => ({
    field: condition.field,
    operator: condition.operator,
    values: condition.values,
  }));

  // A selection the ANDed array could not express travels as a condition instead. Generated, not
  // author-written, so every literal in it went through `aql.literal`.
  const conditions: BackendCondition[] = selection.expression ? [{ expr: selection.expression }] : [];

  return {
    dataset: query.dataset.descriptor.name,
    aql: query.declaration.aql,
    input: {
      filters: [...mappedFilters, ...selectionFilters],
      conditions,
      sorts: query.sort.map((sort) => ({ field: sort.field, direction: sort.direction ?? 'asc' })),
      dateDrills,
      // Omitted entirely when the app declares none, so the backend applies its own default rather
      // than the SDK guessing the browser's.
      ...(context.timezone ? { timezone: context.timezone } : {}),
    },
  };
}

/** A fresh run of the query: built from the app's state, from its first page, with no seeds, so as of now. */
export function buildRequest (
  query: Query,
  mappings: Mapping[],
  selection: SelectionInput,
  context: SubmitContext,
): BackendQueryRequest {
  const { input, ...rest } = buildQuery(query, mappings, selection, context);
  return {
    ...rest,
    input: withPage(input, query),
    ...(context.bustCache ? { bustCache: true } : {}),
  };
}

/**
 * A later page of the query's run: the input the run came back with, replayed with the query's
 * page, so it continues that run (its seeds among it) rather than starting one as of now.
 */
export function pageRequest (query: Query, ran: QueryInput): BackendQueryRequest {
  return { dataset: query.dataset.descriptor.name, aql: query.declaration.aql, input: withPage(ran, query) };
}

// The input with the query's page: omitted for a query that declares no page size, so the backend
// returns every row.
function withPage (input: QueryInput, query: Query): QueryInput {
  const { pagination: _, ...rest } = input;
  return query.pageSize !== undefined ? { ...rest, pagination: { page: query.page, pageSize: query.pageSize } } : rest;
}

/**
 * The file's default name: the app's title and the query's name, or the query's name alone. A
 * title is free text, and the server refuses a name holding `/` or `\`, so those are replaced.
 */
export function defaultExportFilename (query: Query, title: string | undefined): string {
  const safeTitle = title?.replace(/[/\\]/g, '-').replace(/\p{Cc}/gu, '').trim();
  return safeTitle ? `${safeTitle} - ${query.name}` : query.name;
}

/**
 * An export of the query: the input its run came back with, so the file holds the result on screen
 * (as of that run), with no page, every row; or, before any run, the query as the app has it now.
 * And the file's format and name.
 * A CSV starts with a byte-order mark unless the author says otherwise, since a person opens it;
 * a format other than CSV gets no default it might not take.
 */
export function buildExportRequest (
  query: Query,
  mappings: Mapping[],
  selection: SelectionInput,
  context: Pick<SubmitContext, 'timezone'> & { title?: string },
  options: ExportOptions,
  ran?: QueryInput,
): BackendExportRequest {
  const format = options.format ?? 'csv';
  const formatOptions = format === 'csv' ? { bom: true, ...options.formatOptions } : options.formatOptions;
  const built = buildQuery(query, mappings, selection, context);
  const { pagination: _, ...input } = ran ?? built.input;
  return {
    ...built,
    input,
    format,
    ...(formatOptions ? { formatOptions } : {}),
    filename: options.filename?.trim() || defaultExportFilename(query, context.title),
  };
}

/** Runs a request of the query: its result, and the input it ran with, for its next page and its export. */
export async function runQuery (
  query: Query,
  request: BackendQueryRequest,
  backend: Backend,
  signal: AbortSignal,
): Promise<{ result: QueryResult, input: QueryInput }> {
  const data = await backend.submitQuery(request, signal);
  // As the backend ran it, or, from one that doesn't say, as it was sent.
  return { result: toResult(query, data), input: data.input ?? request.input };
}
