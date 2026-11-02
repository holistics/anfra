import {
  DataAppError, contextOf, fromHttpStatus, toScope,
} from '../common/errors';

/** The core API's error body, as every host answers it. */
export interface ApiErrorBody {
  code: string;
  scope?: 'user' | 'client' | 'server' | string;
  message: string;
  details?: unknown;
  context?: { step?: string, text?: string }[];
  status?: number;
  /** The request's id, as the server's log names it. */
  request_id?: string;
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
 * The message, said from the details where those hold the facts: a refused query's diagnostics
 * with their lines, a refused input's fields. Its own message is only its code's sentence ("The
 * query is invalid."), and a message read alone must still say what is wrong.
 */
function messageOf (body: ApiErrorBody): string {
  return diagnosticsOf(body.details) ?? violationsOf(body.details) ?? body.message;
}

/**
 * The SDK error an API error is: the API's own document, passed through. Nothing is re-mapped, so a
 * code a host adds (anfra Cloud does) reaches the app as it is, by its scope. A body without a code
 * (a proxy's error page) is read by its status.
 */
export function toSdkError (status: number, body: ApiErrorBody | undefined): DataAppError {
  if (!body?.code) return fromHttpStatus(status, body?.message || `The server answered ${status}.`, { requestId: body?.request_id });
  return new DataAppError({
    code: body.code,
    scope: toScope(body.scope),
    message: messageOf(body),
    status: body.status ?? status,
    details: body.details,
    context: contextOf(body.context),
    requestId: body.request_id,
  });
}
