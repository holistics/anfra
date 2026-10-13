import type {
  Backend,
  BackendExportRequest,
  BackendLineageRequest,
  BackendQueryRequest,
  BackendQueryResult,
  ColumnMeta,
  ConditionValue,
  DatasetDescriptor,
  ExportResult,
  FieldSuggestionsRequest,
  LineageResult,
  User,
} from '../common/types';

/** A text column's types, as the core API answers them, for a result column fixture. */
export const textTypes = {
  semantic_type: 'text', arrow_type: { type: 'utf8', label: 'utf8' }, db_type: 'text',
} as const satisfies Pick<ColumnMeta, 'semantic_type' | 'arrow_type' | 'db_type'>;

/** A number column's types, as the core API answers them, for a result column fixture. */
export const numberTypes = {
  semantic_type: 'number',
  arrow_type: { type: 'decimal', precision: 38, scale: 2, bit_width: 128, label: 'decimal(38, 2)' },
  db_type: 'numeric',
} as const satisfies Pick<ColumnMeta, 'semantic_type' | 'arrow_type' | 'db_type'>;

export const testUser: User = {
  id: 7,
  name: 'Ada Lovelace',
  email: 'ada@example.com',
  role: 'analyst',
  timezone: 'Asia/Singapore',
  permissions: { canViewGeneratedSql: true, canExportData: true },
};

export const salesDataset: DatasetDescriptor = {
  id: 42,
  name: 'sales',
  label: 'Sales',
  data_models: [
    {
      id: 101,
      name: 'orders',
      label: 'Orders',
      fields: [
        {
          name: 'id', label: 'Id', type: 'number', is_custom_measure: false,
        },
        {
          name: 'amount', label: 'Amount', type: 'number', is_custom_measure: false,
        },
        {
          name: 'status', label: 'Status', type: 'text', is_custom_measure: false,
        },
        {
          name: 'created_at', label: 'Created At', type: 'datetime', is_custom_measure: false,
        },
      ],
    },
    {
      id: 102,
      name: 'users',
      label: 'Users',
      fields: [
        {
          name: 'id', label: 'Id', type: 'number', is_custom_measure: false,
        },
        {
          name: 'region', label: 'Region', type: 'text', is_custom_measure: false,
        },
      ],
    },
  ],
  metrics: [
    { name: 'aov', label: 'Average Order Value', type: 'number' },
  ],
};

export type StubCall =
  | { method: 'submitQuery', request: BackendQueryRequest, signal: AbortSignal }
  | { method: 'fieldSuggestions', request: FieldSuggestionsRequest, signal: AbortSignal }
  | { method: 'exportQuery', request: BackendExportRequest, signal: AbortSignal }
  | { method: 'lineage', request: BackendLineageRequest, signal: AbortSignal };

type Answer<Req, Res> = Res | Error | ((request: Req, signal: AbortSignal) => Res | Error | Promise<Res | Error>);

export interface StubOptions {
  /** Answers `submitQuery`: a result, an `Error` to reject with, or a function of the request. */
  submitQuery?: Answer<BackendQueryRequest, BackendQueryResult>;
  /** Answers `fieldSuggestions`, likewise. */
  fieldSuggestions?: Answer<FieldSuggestionsRequest, ConditionValue[]>;
  /** Answers `exportQuery`, likewise. Left out, the Backend has no `exportQuery`: a host that cannot export. */
  exportQuery?: Answer<BackendExportRequest, ExportResult>;
  /** Answers `lineage`, likewise. Left out, the Backend has no `lineage`: a host that cannot trace it. */
  lineage?: Answer<BackendLineageRequest, LineageResult>;
}

export interface Stub {
  backend: Backend;
  calls: StubCall[];
  /** Just the query requests, in order — what most tests assert on. */
  readonly queries: BackendQueryRequest[];
  /** Just the export requests, in order. */
  readonly exports: BackendExportRequest[];
}

function abortError (): Error {
  const err = new Error('Aborted');
  err.name = 'AbortError';
  return err;
}

async function answer<Req, Res> (option: Answer<Req, Res> | undefined, request: Req, signal: AbortSignal, method: string): Promise<Res> {
  if (signal.aborted) throw abortError();
  if (option === undefined) throw new Error(`No stub for ${method}`);
  const value = typeof option === 'function'
    ? await (option as (r: Req, s: AbortSignal) => Res | Error | Promise<Res | Error>)(request, signal)
    : option;
  if (value instanceof Error) throw value;
  return value;
}

/** A `Backend` stand-in, so the whole SDK is testable with no server. */
export function stubBackend (options: StubOptions = {}): Stub {
  const calls: StubCall[] = [];

  const backend: Backend = {
    submitQuery (request, signal) {
      calls.push({ method: 'submitQuery', request, signal });
      return answer(options.submitQuery, request, signal, 'submitQuery');
    },
    fieldSuggestions (request, signal) {
      calls.push({ method: 'fieldSuggestions', request, signal });
      return answer(options.fieldSuggestions, request, signal, 'fieldSuggestions');
    },
  };
  if (options.exportQuery !== undefined) {
    backend.exportQuery = (request, signal) => {
      calls.push({ method: 'exportQuery', request, signal });
      return answer(options.exportQuery, request, signal, 'exportQuery');
    };
  }
  if (options.lineage !== undefined) {
    backend.lineage = (request, signal) => {
      calls.push({ method: 'lineage', request, signal });
      return answer(options.lineage, request, signal, 'lineage');
    };
  }

  return {
    backend,
    calls,
    get queries () {
      return calls.flatMap((call) => (call.method === 'submitQuery' ? [call.request] : []));
    },
    get exports () {
      return calls.flatMap((call) => (call.method === 'exportQuery' ? [call.request] : []));
    },
  };
}
