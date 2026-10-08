import {
  PermissionError, QueryError, TransportError, type DataAppError,
} from '../common/errors';

/** The core API's error body, as every host answers it. */
export interface ApiErrorBody {
  code: string;
  scope?: 'user' | 'client' | 'server' | string;
  message: string;
  details?: unknown;
}

interface Diagnostic { message: string, line?: number, column?: number }
interface Violation { field: string, message: string }

function diagnosticsOf (details: unknown): string | undefined {
  const diags = (details as { diagnostics?: Diagnostic[] } | undefined)?.diagnostics;
  if (!Array.isArray(diags) || diags.length === 0) return undefined;
  return diags.map((d) => {
    if (d.line && d.column) return `line ${d.line}:${d.column}: ${d.message}`;
    if (d.line) return `line ${d.line}: ${d.message}`;
    return d.message;
  }).join('\n');
}

function violationsOf (details: unknown): string | undefined {
  const vs = (details as { violations?: Violation[] } | undefined)?.violations;
  if (!Array.isArray(vs) || vs.length === 0) return undefined;
  return vs.map((v) => `${v.field}: ${v.message}`).join('\n');
}

/**
 * The SDK error an API error is, by what a Data App author can do about it: a query to fix
 * (`QueryError`), something they may not see (`PermissionError`), or the server failing
 * (`TransportError`). A code this SDK does not know is read by its scope, so a host that adds codes
 * (anfra-cloud does) still lands each one somewhere sensible.
 */
export function toSdkError (status: number, body: ApiErrorBody | undefined): DataAppError {
  if (status === 401 || status === 403) return new PermissionError(body?.message ?? 'You may not see this.');
  if (!body?.code) return new TransportError(`The server answered ${status}.`, status);
  switch (body.code) {
    case 'query_invalid':
      return new QueryError(diagnosticsOf(body.details) ?? body.message);
    case 'validation_failed':
    case 'invalid_request':
      return new QueryError(violationsOf(body.details) ?? body.message);
    case 'query_failed':
      return new QueryError(body.message);
    case 'sidecar_unavailable':
      return new TransportError(body.message, status);
    default:
      return body.scope === 'server' || status >= 500
        ? new TransportError(body.message, status)
        : new QueryError(body.message);
  }
}
