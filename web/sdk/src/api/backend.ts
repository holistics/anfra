import { DataAppError, connectionFailed, invalidRequest } from '../common/errors';
import type {
  Backend, BackendExportRequest, BackendLineageRequest, BackendQueryRequest, BackendQueryResult, ColumnMeta, ConditionValue,
  DatasetDescriptor, ExportResult, FieldSuggestionsRequest, LineageResult,
} from '../common/types';
import { coreClient, type CoreClient } from './client';
import type { components } from './schema';
import { toSdkError, type ApiErrorBody } from './errors';
import { readEvents } from './stream';

type QueryRequestParams = components['schemas']['QueryRequestParams'];
type Column = components['schemas']['Column'];
type QueryExportRequestParams = components['schemas']['QueryExportRequestParams'];
type QueryResult = components['schemas']['QueryResult'];
type QueryEvent = components['schemas']['QueryCompiled'] | components['schemas']['QueryWarehouse'] | components['schemas']['QueryHeader']
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

/** The core.query request params a Backend request is: the AQL, its dataset and its input. */
export function toQueryRequestParams (request: BackendQueryRequest): QueryRequestParams {
  return { query: request.aql, dataset: request.dataset, input: request.input };
}

/** The core.query.export request params an export request is: the query as a run sends it, every row, and the file. */
export function toExportRequestParams (request: BackendExportRequest): QueryExportRequestParams {
  return {
    query: request.aql,
    dataset: request.dataset,
    input: request.input,
    // The server names its formats; one it does not know it refuses, with a message the author sees.
    ...(request.format ? { format: request.format as QueryExportRequestParams['format'] } : {}),
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
      throw new DataAppError({
        code: 'internal_server_error',
        scope: 'server',
        message: `The server answered column '${column.name}' without its types (semantic_type, arrow_type, db_type), `
          + 'which a Data App decides by: the server is older than this SDK.',
      });
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
    throw connectionFailed(`The server could not be reached: ${(err as Error).message}`);
  }
  const { data, error, response } = result;
  if (!data) throw toSdkError(response.status, (error as { error?: ApiErrorBody } | undefined)?.error);
  return data;
}

/**
 * A query, streamed (`Accept: application/x-ndjson`), gathered as it arrives into the page's
 * shape: the page with `input.pagination`, and every row without — which one JSON answer, a page, cannot
 * carry. Every query goes this way, so there is one path; the server holds one batch at a time.
 */
async function runQuery (client: CoreClient, body: QueryRequestParams, signal: AbortSignal): Promise<QueryResult> {
  const stream = await callCoreAPI(() => client.POST('/core.query', {
    body, signal, parseAs: 'stream', headers: { Accept: 'application/x-ndjson' },
  }));
  if (!stream) throw connectionFailed('The server answered the query with no body.');
  const result: QueryResult = { sql: '', columns: [], result: { fields: [], records: [] }, has_more: false, timings: { steps: [], end_ms: 0 } };
  try {
    await readEvents<QueryEvent>(stream, (event) => {
      switch (event.type) {
        case 'compiled':
          result.sql = event.sql;
          if (event.aql) result.aql = event.aql;
          if (event.input) result.input = event.input;
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
          result.timings = event.timings;
          break;
      }
    });
  } catch (err) {
    if (isAbort(err) || !(err instanceof Error) || err.name !== 'TypeError') throw err;
    throw connectionFailed(`The connection to the server failed: ${err.message}`);
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
      const data = await runQuery(client, toQueryRequestParams(request), signal);
      const values = data.result.records;
      const { pagination } = request.input;
      return {
        columns: toColumns(data.columns),
        values,
        meta: {
          ...(pagination ? { page: pagination.page, pageSize: pagination.pageSize, hasMore: data.has_more } : {}),
          numRows: values.length,
        },
        debug: {
          ...(data.aql ? { executedAql: data.aql } : {}),
          executedSql: data.sql,
          fromCache: false,
          timings: {
            steps: data.timings.steps.map((s) => ({ step: s.step, startMs: s.start_ms })),
            endMs: data.timings.end_ms,
          },
          executedAt: new Date(),
        },
        // The input as it ran: the one sent, with the seeds the server generated for it. The rest
        // the server answers with is what was sent, or its defaults, which a replay needn't send.
        ...(data.input?.seeds ? { input: { ...request.input, seeds: data.input.seeds } } : {}),
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
        throw invalidRequest(`Unknown field ${request.model}.${request.field} in dataset ${request.dataset}.`);
      }
      const ref = `${request.model}.${request.field}`;
      const q = request.q.trim();
      const data = await runQuery(client, {
        query: `explore { dimensions { value: ${ref} } }`,
        dataset: request.dataset,
        input: {
          filters: q && field.type === 'text' ? [{ field: ref, operator: 'contains', values: [q] }] : [],
          sorts: [{ field: 'value', direction: 'asc' }],
          pagination: { page: 1, pageSize: SUGGESTION_LIMIT },
        },
      }, signal);
      return data.result.records
        .map((row) => row[0])
        .filter((v): v is ConditionValue => typeof v === 'string' || typeof v === 'number' || typeof v === 'boolean');
    },

    /** An export as `core.query.export`: the server runs the query to completion, then answers its link. */
    async exportQuery (request: BackendExportRequest, signal: AbortSignal): Promise<ExportResult> {
      const link = await callCoreAPI(() => client.POST('/core.query.export', { body: toExportRequestParams(request), signal }));
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
