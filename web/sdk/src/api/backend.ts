import { QueryError, TransportError } from '../common/errors';
import type {
  Backend, BackendQueryRequest, BackendQueryResult, ColumnMeta, ConditionValue, DatasetDescriptor, FieldSuggestionsRequest,
} from '../common/types';
import { coreClient, type CoreClient } from './client';
import type { components } from './schema';
import { toSdkError, type ApiErrorBody } from './errors';

type QueryRunInput = components['schemas']['QueryRunInput'];
type Column = components['schemas']['Column'];

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

async function runQuery (client: CoreClient, body: QueryRunInput, signal: AbortSignal) {
  let result;
  try {
    result = await client.POST('/core.query', { body, signal });
  } catch (err) {
    if (isAbort(err)) throw err;
    throw new TransportError(`The server could not be reached: ${(err as Error).message}`);
  }
  const { data, error, response } = result;
  if (!data) throw toSdkError(response.status, (error as { error?: ApiErrorBody } | undefined)?.error);
  return data;
}

/**
 * A Backend over the core API: a Data App's queries as `core.query`, and its field suggestions as
 * one `core.query` too. The same on every host: `/api` on `anfra serve`, `/api/o/{org}` on
 * anfra-cloud. A host's page uses it; a Data App's frame never does.
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
  };
}
