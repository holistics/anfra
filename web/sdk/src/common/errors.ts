import type { components } from '../api/schema';

/**
 * A Data App's errors are the API's error document, not classes picked for it
 * (docs/designs/errors.md, "In a Data App"): every failure of a call is a `DataAppError` with the
 * envelope's fields, and the one other class, `ValidationError`, is what a declaration throws. An
 * app branches on `scope` (who must act) and `code` (what happened), and reads `details`.
 */

/** Who must act: the reader's input or state, the calling program (the app), or the server side. */
export type ErrorScope = 'user' | 'client' | 'server';

/**
 * The codes every backend uses where they apply, so an app that branches on them runs on any host.
 * Each reuses an anfra or anfra Cloud code where one exists.
 */
export const GENERIC_CODES = {
  query_invalid: 'user',
  query_failed: 'user',
  forbidden: 'user',
  unauthenticated: 'user',
  invalid_request: 'client',
  declaration_invalid: 'client',
  connection_failed: 'server',
  call_failed: 'server',
  internal_server_error: 'server',
} as const satisfies Record<string, ErrorScope>;

export type GenericCode = keyof typeof GENERIC_CODES;

type Schemas = components['schemas'];
/** Every error body the core API documents, from its OpenAPI schema. */
type ApiErrorBodies = Schemas[Extract<keyof Schemas, `ErrorBody${string}`>];

/** The codes the core API documents. */
export type ApiErrorCode = ApiErrorBodies['code'];

/** A code: a generic one, anfra's own, or another backend's (any string). */
// eslint-disable-next-line @typescript-eslint/ban-types -- keeps the known codes suggested
export type ErrorCode = GenericCode | ApiErrorCode | (string & {});

/** The `details` a code carries, typed for the core API's codes; `unknown` for any other. */
export type ErrorDetails<C extends string> = C extends ApiErrorCode
  ? Extract<ApiErrorBodies, { code: C }> extends { details?: infer D } ? D : undefined
  : unknown;

/** One step the failure happened in, as the API names it: its text, for a person. */
export interface ErrorContextEntry {
  step?: string;
  text: string;
}

export interface DataAppErrorInit {
  code: ErrorCode;
  scope: ErrorScope;
  message: string;
  status?: number;
  details?: unknown;
  context?: ErrorContextEntry[];
  requestId?: string;
  entity?: string;
  cause?: unknown;
}

export class DataAppError extends Error {
  /** What happened: a generic code, or the backend's own. */
  readonly code: ErrorCode;

  /** Who must act: `user` (the reader's input or state), `client` (the app), `server`. */
  readonly scope: ErrorScope;

  /** The HTTP status the backend answered, when it answered one. */
  readonly status?: number;

  /** Facts to act on, typed by code: a query's diagnostics, the fields at fault, a row limit. */
  readonly details?: unknown;

  /** The steps the failure happened in, outermost first. */
  readonly context?: ErrorContextEntry[];

  /** The id of the server request that failed: the line to look for in its log. */
  readonly requestId?: string;

  /** The query, control or dataset that caused this. */
  readonly entity?: string;

  readonly cause?: unknown;

  constructor (init: DataAppErrorInit) {
    super(init.message);
    this.name = new.target.name;
    this.code = init.code;
    this.scope = init.scope;
    if (init.status !== undefined) this.status = init.status;
    if (init.details !== undefined) this.details = init.details;
    if (init.context !== undefined) this.context = init.context;
    if (init.requestId !== undefined) this.requestId = init.requestId;
    if (init.entity !== undefined) this.entity = init.entity;
    if (init.cause !== undefined) this.cause = init.cause;
    // Restore the prototype chain, which is lost when subclassing Error under ES5 downlevelling.
    Object.setPrototypeOf(this, new.target.prototype);
  }

  /** This error, naming `entity`: the same failure, said of the query or control it concerns. */
  about (entity: string): DataAppError {
    if (this.entity === entity) return this;
    return new DataAppError({ ...fieldsOf(this), entity });
  }
}

/**
 * Thrown synchronously at declaration time. This is the primary feedback channel for an agent
 * authoring an app: it has no REPL, so the thrown error is the whole loop. Messages must carry the
 * offending value and, where we can compute one, a suggestion.
 */
export class ValidationError extends DataAppError {
  constructor (message: string, entity?: string) {
    super({
      code: 'declaration_invalid', scope: 'client', message, entity,
    });
  }

  override about (entity: string): DataAppError {
    return this.entity === entity ? this : new ValidationError(this.message, entity);
  }
}

/**
 * True when `err` is one of the SDK's errors: this copy's (a subclass too), or another copy's, by
 * name and shape. A host may load a plugin built against its own copy of the SDK, whose classes are
 * not this copy's, so `instanceof` alone would miss its errors.
 */
export function isDataAppError (err: unknown): err is DataAppError {
  if (err instanceof DataAppError) return true;
  const e = err as DataAppError | undefined;
  return e instanceof Error && (e.name === 'DataAppError' || e.name === 'ValidationError') && typeof e.code === 'string';
}

/** True when `err` is a `DataAppError` with `code`, whose `details` are then typed. */
export function hasErrorCode<C extends ErrorCode> (err: unknown, code: C): err is DataAppError & { code: C, details?: ErrorDetails<C> } {
  return isDataAppError(err) && err.code === code;
}

/** Each step's text, without its params, which can name a dataset or a path. */
export function contextOf (context: unknown): ErrorContextEntry[] | undefined {
  if (!Array.isArray(context)) return undefined;
  const entries = context
    .filter((c) => typeof (c as ErrorContextEntry | undefined)?.text === 'string')
    .map((c: ErrorContextEntry) => (typeof c.step === 'string' ? { step: c.step, text: c.text } : { text: c.text }));
  return entries.length ? entries : undefined;
}

/** A call the app made wrong: a plugin's refusal. */
export function invalidRequest (message: string, entity?: string): DataAppError {
  return new DataAppError({
    code: 'invalid_request', scope: 'client', message, entity,
  });
}

/** Something this reader may not see or do. */
export function forbidden (message: string, entity?: string): DataAppError {
  return new DataAppError({
    code: 'forbidden', scope: 'user', message, entity,
  });
}

/** The server could not be reached, or its answer was cut or garbled. */
export function connectionFailed (message: string, status?: number): DataAppError {
  return new DataAppError({
    code: 'connection_failed', scope: 'server', message, status,
  });
}

/** A call that failed for a reason the app is not shown. */
export function callFailed (cause?: unknown): DataAppError {
  return new DataAppError({
    code: 'call_failed', scope: 'server', message: 'The call failed.', cause,
  });
}

export interface FromHttpStatusOptions {
  /** The code a 4xx other than 401 and 403 is: the refusal of what was called. Default `invalid_request`. */
  refused?: ErrorCode;
  requestId?: string;
  entity?: string;
}

/**
 * The error a backend without anfra's error document answers, from what it has: 401 is
 * `unauthenticated`, 403 `forbidden`, any other 4xx the refusal of what was called, 5xx
 * `internal_server_error`, and no status `connection_failed`. The table every such backend uses, so
 * hosts classify alike.
 */
export function fromHttpStatus (status: number | undefined, message: string, options: FromHttpStatusOptions = {}): DataAppError {
  const { refused = 'invalid_request', requestId, entity } = options;
  let code: ErrorCode;
  if (status === undefined || status === 0) code = 'connection_failed';
  else if (status === 401) code = 'unauthenticated';
  else if (status === 403) code = 'forbidden';
  else if (status >= 400 && status < 500) code = refused;
  else code = 'internal_server_error';
  return new DataAppError({
    code, scope: scopeOf(code, status), message, status: status || undefined, requestId, entity,
  });
}

/** A code's scope: a generic code's own, else by status (any 4xx the user's, else the server's). */
function scopeOf (code: ErrorCode, status: number | undefined): ErrorScope {
  if (code in GENERIC_CODES) return GENERIC_CODES[code as GenericCode];
  return status !== undefined && status >= 400 && status < 500 ? 'user' : 'server';
}

/** A scope as given, or `server` for anything else: an unknown scope is the server's to answer for. */
export function toScope (scope: unknown): ErrorScope {
  return scope === 'user' || scope === 'client' || scope === 'server' ? scope : 'server';
}

function fieldsOf (err: DataAppError): DataAppErrorInit {
  return {
    code: err.code,
    scope: err.scope,
    message: err.message,
    status: err.status,
    details: err.details,
    context: err.context,
    requestId: err.requestId,
    entity: err.entity,
    cause: err.cause,
  };
}
