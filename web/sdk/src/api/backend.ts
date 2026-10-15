import { QueryError, TransportError } from '../common/errors';
import type {
  Backend, BackendExportRequest, BackendLineageRequest, BackendQueryRequest, BackendQueryResult, ColumnMeta, ConditionValue,
  DatasetDescriptor, ExportResult, FieldSuggestionsRequest, LineageResult,
} from '../common/types';
import { coreClient, type CoreClient } from './client';
import type { components } from './schema';
import { toSdkError, type ApiErrorBody } from './errors';
import { readEvents } from './stream';

type QueryRunInput = components['schemas']['QueryRunInput'];
type Column = components['schemas']['Column'];
type ExportInput = components['schemas']['ExportInput'];
type QueryResult = components['schemas']['QueryResult'];
type QueryEvent = components['schemas']['QueryCompiled'] | components['schemas']['QueryHeader']
  | components['schemas']['QueryRecords'] | components['schemas']['QuerySummary']
  | components['schemas']['StreamStart'] | components['schemas']['StreamStatus'];

/** How many values a filter control is offered at most. */
export const SUGGESTION_LIMIT = 100;

export interface CoreApiBackendOptions {
  /**
   * The datasets the frame was provisioned with. Field suggestions splice a field into AQL, so they
   * only run for a field one of these defines.
   */
  datasets?: Record<string, DatasetDescriptor>;
}

function isAbort (err: unknown): boolean {
  return err instanceof Error && err.name === 'AbortError';
}

/** The core.query input a Backend request is: the AQL, its Query Input, its paging and timezone. */
export function toQueryInput (request: BackendQueryRequest): QueryRunInput {
  return {
    query: request.aql,
    dataset: request.dataset,
    // The Query Input passes through in the names amql and this SDK share.
    input: request.input as QueryRunInput['input'],
    ...(request.pageSize !== undefined ? { page: request.page ?? 1, page_size: request.pageSize } : {}),
    ...(request.timezone ? { timezone: request.timezone } : {}),
  };
}

/** The core.query.export input an export request is: the query as a run sends it, and the file. */
export function toExportInput (request: BackendExportRequest): ExportInput {
  return {
    query: request.aql,
    dataset: request.dataset,
    input: request.input as ExportInput['input'],
    ...(request.timezone ? { timezone: request.timezone } : {}),
    // The server names its formats; one it does not know it refuses, with a message the author sees.
    ...(request.format ? { format: request.format as ExportInput['format'] } : {}),
    ...(request.formatOptions ? { format_options: request.formatOptions } : {}),
    ...(request.filename ? { filename: request.filename } : {}),
  };
}

/**
 * The API's columns as a Data App takes them, their types required. The API's column leaves them
 * out where nothing can say them (a SQL query, a query not yet run), but a Data App's query is an
 * explore that ran, whose every column has them: one without is a server older than this SDK, and
 * a Data App deciding by `typeof` instead would be wrong in silence.
 */
function toColumns (columns: Column[]): ColumnMeta[] {
  return columns.map((column) => {
    const { semantic_type: semanticType, arrow_type: arrowType, db_type: dbType } = column;
    if (semanticType === undefined || arrowType === undefined || dbType === undefined) {
      throw new QueryError(
        `The server answered column '${column.name}' without its types (semantic_type, arrow_type, db_type), `
        + 'which a Data App decides by: the server is older than this SDK.',
      );
    }
    return { ...column, semantic_type: semanticType, arrow_type: arrowType, db_type: dbType };
  });
}

/** A call's answer, or the SDK error its failure is. */
async function callCoreAPI<T> (call: () => Promise<{ data?: T, error?: unknown, response: Response }>): Promise<T> {
  let result;
  try {
    result = await call();
  } catch (err) {
    if (isAbort(err)) throw err;
    throw new TransportError(`The server could not be reached: ${(err as Error).message}`);
  }
  const { data, error, response } = result;
  if (!data) throw toSdkError(response.status, (error as { error?: ApiErrorBody } | undefined)?.error);
  return data;
}

/**
 * A query, streamed (`Accept: application/x-ndjson`), gathered as it arrives into the page's
 * shape: the page with `page_size`, and every row without — which one JSON answer, a page, cannot
 * carry. Every query goes this way, so there is one path; the server holds one batch at a time.
 */
async function runQuery (client: CoreClient, body: QueryRunInput, signal: AbortSignal): Promise<QueryResult> {
  const stream = await callCoreAPI(() => client.POST('/core.query', {
    body, signal, parseAs: 'stream', headers: { Accept: 'application/x-ndjson' },
  }));
  if (!stream) throw new TransportError('The server answered the query with no body.');
  const result: QueryResult = { sql: '', columns: [], result: { fields: [], records: [] }, has_more: false };
  try {
    await readEvents<QueryEvent>(stream, (event) => {
      switch (event.type) {
        case 'compiled':
          result.sql = event.sql;
          if (event.aql) result.aql = event.aql;
          break;
        case 'header':
          result.columns = event.columns;
          result.result.fields = event.fields;
          break;
        case 'rows':
          for (const row of event.records) result.result.records.push(row);
          break;
        case 'summary':
          result.has_more = event.has_more;
          break;
      }
    });
  } catch (err) {
    if (isAbort(err) || !(err instanceof Error) || err.name !== 'TypeError') throw err;
    throw new TransportError(`The connection to the server failed: ${err.message}`);
  }
  return result;
}

/**
 * A Backend over the core API: a Data App's queries as `core.query`, its field suggestions as one
 * `core.query` too, its exports as `core.query.export`, and its lineage as `core.lineage`. The same on
 * every host: `/api` on `anfra serve`, `/api/o/{org}` on anfra-cloud. A host's page uses it; a Data App's frame never does.
 */
export function coreApiBackend (base: string | CoreClient, options: CoreApiBackendOptions = {}): Backend {
  const client = typeof base === 'string' ? coreClient(base) : base;

  return {
    async submitQuery (request: BackendQueryRequest, signal: AbortSignal): Promise<BackendQueryResult> {
      const data = await runQuery(client, toQueryInput(request), signal);
      const values = data.result.records;
      return {
        columns: toColumns(data.columns),
        values,
        meta: {
          ...(request.pageSize !== undefined ? { page: request.page ?? 1, pageSize: request.pageSize } : {}),
          numRows: values.length,
        },
        debug: {
          ...(data.aql ? { executedAql: data.aql } : {}),
          executedSql: data.sql,
          fromCache: false,
          executedAt: new Date(),
        },
      };
    },

    /**
     * A field-backed filter's values: one distinct-values explore, sorted, capped, narrowed for a
     * text field by what the reader typed (as a Query Input value, never spliced). Only strings,
     * numbers and booleans come back.
     */
    async fieldSuggestions (request: FieldSuggestionsRequest, signal: AbortSignal): Promise<ConditionValue[]> {
      const model = options.datasets?.[request.dataset]?.data_models.find((m) => m.name === request.model);
      const field = model?.fields.find((f) => f.name === request.field);
      if (!field) {
        throw new QueryError(`Unknown field ${request.model}.${request.field} in dataset ${request.dataset}.`);
      }
      const ref = `${request.model}.${request.field}`;
      const q = request.q.trim();
      const data = await runQuery(client, {
        query: `explore { dimensions { value: ${ref} } }`,
        dataset: request.dataset,
        input: {
          filters: q && field.type === 'text' ? [{ field: ref, operator: 'contains', values: [q] }] : [],
          sorts: [{ field: 'value', direction: 'asc' }],
        },
        page: 1,
        page_size: SUGGESTION_LIMIT,
      }, signal);
      return data.result.records
        .map((row) => row[0])
        .filter((v): v is ConditionValue => typeof v === 'string' || typeof v === 'number' || typeof v === 'boolean');
    },

    /** An export as `core.query.export`: the server runs the query to completion, then answers its link. */
    async exportQuery (request: BackendExportRequest, signal: AbortSignal): Promise<ExportResult> {
      const link = await callCoreAPI(() => client.POST('/core.query.export', { body: toExportInput(request), signal }));
      return {
        url: link.url,
        filename: link.filename,
        format: link.format,
        rowCount: link.row_count,
        expiresAt: new Date(link.expires_at),
      };
    },

    /** Lineage as `core.lineage`, whose answer is the SDK's as it is. */
    lineage (request: BackendLineageRequest, signal: AbortSignal): Promise<LineageResult> {
      return callCoreAPI(() => client.POST('/core.lineage', { body: request, signal }));
    },
  };
}
