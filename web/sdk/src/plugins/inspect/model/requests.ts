/**
 * The request log: every call the frame makes through the host's plugins (its queries, exports,
 * lineage, renderings), recorded host-side with what the snapshots leave out, how long it took and
 * a few of its rows. `inspect`'s middleware fills one per mount.
 */
import type {
  BackendExportRequest,
  BackendLineageRequest,
  BackendQueryRequest,
  BackendQueryResult,
  ConditionValue,
  ExportFormatOptions,
  ExportResult,
  FieldSuggestionsRequest,
  InspectedApp,
  LineageResult,
  QueryInput,
} from '../../../common/types';
import type { CallContext } from '../../../host/plugin';

/** The calls the log knows the shape of; any other is logged by its name, with its request as is. */
export type KnownCall = 'core.submitQuery' | 'core.fieldSuggestions' | 'export.query' | 'lineage.trace';

export interface RequestEntry {
  id: number;
  /** The call's full name, `<plugin>.<name>`: `core.submitQuery` for a query. */
  call: KnownCall | (string & {});
  /** Who made it: `frame`, or `plugin <name>` for a plugin's call made for the frame. */
  caller: string;
  /** The middleware it passed through before this log, by name, outermost first. */
  trace?: string[];
  /** When it was sent, as an ISO time. */
  at: string;
  /** Absent while pending. */
  durationMs?: number;
  status: 'pending' | 'ok' | 'error' | 'aborted';
  /** The dataset it queried; absent for a call that queries none. */
  dataset?: string;
  /** The query's declared AQL; filters, sorts and drills are in `input`. */
  aql?: string;
  input?: QueryInput;
  page?: number;
  pageSize?: number;
  bustCache?: boolean;
  /** The timezone it ran in; absent when the app declares none, leaving the backend's default. */
  timezone?: string;
  /** For an export: the file format asked for, its options and the file's name. */
  format?: string;
  formatOptions?: ExportFormatOptions;
  filename?: string;
  /** For a field suggestion: `model.field` and what was typed. */
  field?: string;
  q?: string;
  /** For lineage: the fields traced, as `parent.child`. */
  targets?: string[];
  /** For a call the log doesn't know: its request, as the frame sent it. */
  request?: unknown;
  fromCache?: boolean;
  numRows?: number;
  columns?: string[];
  /** The first rows, as the backend returned them, positional by `columns`. */
  preview?: unknown[][];
  error?: string;
}

export interface RequestLog {
  /** Oldest first. The same array until the log changes. */
  entries(): readonly RequestEntry[];
  subscribe (listener: () => void): () => void;
}

export interface RecordOptions {
  /** How many requests to keep; the oldest go first. */
  limit?: number;
  /** How many rows of each result to keep. */
  previewRows?: number;
}

/** A middleware's `handle`: records the call, then passes it on unchanged. */
export type RecordCall = (
  request: unknown,
  context: CallContext,
  next: (request?: unknown) => Promise<unknown>,
) => Promise<unknown>;

/** A log, and what fills it. */
export interface RequestRecorder {
  log: RequestLog;
  /**
   * Records one of the frame's calls, as a middleware's handle. The host page's own calls are
   * passed on unrecorded. The log fills whether or not anyone is inspecting.
   */
  record: RecordCall;
}

/** What the log keeps of a request, by call: what it asked. */
function describe (call: string, request: unknown): Partial<RequestEntry> {
  switch (call) {
    case 'core.submitQuery': {
      const r = request as BackendQueryRequest;
      return {
        dataset: r.dataset,
        aql: r.aql,
        input: r.input,
        page: r.page,
        pageSize: r.pageSize,
        timezone: r.timezone,
        bustCache: r.bustCache,
      };
    }
    case 'core.fieldSuggestions': {
      const r = request as FieldSuggestionsRequest;
      return { dataset: r.dataset, field: `${r.model}.${r.field}`, q: r.q };
    }
    case 'export.query': {
      const r = request as BackendExportRequest;
      return {
        dataset: r.dataset,
        aql: r.aql,
        input: r.input,
        timezone: r.timezone,
        format: r.format,
        formatOptions: r.formatOptions,
        filename: r.filename,
      };
    }
    case 'lineage.trace': {
      const r = request as BackendLineageRequest;
      return { targets: r.targets.map((t) => `${t.parent}.${t.child}`) };
    }
    case 'render.save':
      // Not the picture itself: a file the frame made, as big as it is.
      return { filename: (request as { filename?: string } | null)?.filename };
    default:
      return { request };
  }
}

/** What the log keeps of an answer, by call: how many rows, and the first of them. */
function summarize (call: string, result: unknown, previewRows: number): Partial<RequestEntry> {
  switch (call) {
    case 'core.submitQuery': {
      const r = result as BackendQueryResult;
      return {
        fromCache: r.debug?.fromCache,
        numRows: r.meta?.numRows ?? r.values.length,
        columns: r.columns.map((column) => column.name),
        preview: r.values.slice(0, previewRows),
      };
    }
    case 'core.fieldSuggestions':
      return { numRows: (result as ConditionValue[]).length };
    case 'export.query':
      return { numRows: (result as ExportResult).rowCount };
    case 'lineage.trace':
      return { numRows: (result as LineageResult).results.length };
    default:
      return {};
  }
}

const callerOf = (context: CallContext) =>
  context.caller.kind === 'plugin' ? `plugin ${context.caller.plugin}` : context.caller.kind;

/** A log for one mounted frame. Make it before the frame, so its first requests are in it. */
export function createRequestLog ({ limit = 200, previewRows = 20 }: RecordOptions = {}): RequestRecorder {
  let entries: readonly RequestEntry[] = [];
  let nextId = 1;
  const listeners = new Set<() => void>();
  // A listener that throws mustn't reach the frame's request.
  const changed = () => {
    for (const listener of listeners) {
      try {
        listener();
      } catch (err) {
        console.error(err);
      }
    }
  };
  const add = (entry: RequestEntry) => {
    entries = [...entries.slice(Math.max(0, entries.length - limit + 1)), entry];
    changed();
  };
  const settle = (id: number, patch: Partial<RequestEntry>) => {
    entries = entries.map((entry) => (entry.id === id ? { ...entry, ...patch } : entry));
    changed();
  };

  return {
    async record (request, context, next) {
      if (context.origin.kind === 'host') return next();
      const id = nextId++;
      const started = performance.now();
      // The request is the frame's whatever the log makes of it: one it can't read is still sent.
      let details: Partial<RequestEntry> = {};
      try {
        details = describe(context.name, request);
      } catch (err) {
        console.error(err);
      }
      add({
        ...details,
        id,
        call: context.name,
        caller: callerOf(context),
        // A copy: the host keeps adding the middleware after this one.
        ...(context.trace.length > 1 && { trace: context.trace.slice(0, -1) }),
        at: new Date().toISOString(),
        status: 'pending',
      });
      const durationMs = () => Math.round(performance.now() - started);
      let result: unknown;
      try {
        result = await next();
      } catch (err) {
        settle(id, {
          status: context.signal.aborted ? 'aborted' : 'error',
          durationMs: durationMs(),
          error: err instanceof Error ? `${err.name}: ${err.message}` : String(err),
        });
        throw err;
      }
      let summary: Partial<RequestEntry> = {};
      try {
        summary = summarize(context.name, result, previewRows);
      } catch (err) {
        console.error(err);
      }
      settle(id, { status: 'ok', durationMs: durationMs(), ...summary });
      return result;
    },
    log: {
      entries: () => entries,
      subscribe (listener) {
        listeners.add(listener);
        return () => listeners.delete(listener);
      },
    },
  };
}

/** A query a request is attributed to. */
export interface QueryRef {
  app: number;
  name: string;
}

/**
 * The queries a request may belong to: those that declare its dataset and AQL. A request carries
 * the declared AQL, not the executed one, so this is exact unless two queries declare the same.
 */
export function queriesOf (
  entry: Pick<BackendQueryRequest, 'dataset' | 'aql'> & { call: RequestEntry['call'] },
  apps: InspectedApp[],
): QueryRef[] {
  if (entry.call !== 'core.submitQuery' && entry.call !== 'export.query') return [];
  return apps.flatMap((app, index) =>
    Object.entries(app.declaration.queries)
      .filter(([, q]) => q.dataset === entry.dataset && q.aql === entry.aql)
      .map(([name]) => ({ app: index, name })),
  );
}

/** Each query's requests, newest first, keyed `<app>/<name>`; and the requests no query claims. */
export function attributeRequests (entries: readonly RequestEntry[], apps: InspectedApp[]) {
  const byQuery = new Map<string, { entry: RequestEntry; shared: boolean }[]>();
  const other: RequestEntry[] = [];
  for (let i = entries.length - 1; i >= 0; i--) {
    const entry = entries[i] as RequestEntry;
    const refs =
      entry.aql && entry.dataset ? queriesOf({ ...entry, dataset: entry.dataset, aql: entry.aql }, apps) : [];
    if (refs.length === 0) other.push(entry);
    for (const ref of refs) {
      const key = `${ref.app}/${ref.name}`;
      const item = { entry, shared: refs.length > 1 };
      const list = byQuery.get(key);
      if (list) list.push(item);
      else byQuery.set(key, [item]);
    }
  }
  return { byQuery, other };
}
