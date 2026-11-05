/**
 * A Data App's runtime errors and console lines, as its frame reports them to its host
 * (docs/designs/errors/data-apps.md). The frame runs code anyone may write, so the host
 * reads each event through `checkErrorEvent` / `checkConsoleEvent`: only the fields named here,
 * each string capped. Never rows or a request's params.
 */

/** Whose code failed, or which of the SDK's calls: the first part of an error's kind. */
export type AppErrorSource = 'call' | 'app' | 'plugin' | 'host';

/**
 * Where a runtime error came from, as `<source>.<what>` in snake_case. Known today:
 *
 * - `call.query`: a query's run ended in `error`, whether or not the app shows it;
 * - `call.<api>`: a call the app made and left unhandled, named as the app called it
 *   (`call.fetch_more`, `call.export`, `call.load_options`, `call.lineage`; `Anfra.foo.bar()` is
 *   `call.foo_bar`);
 * - `app.uncaught`: an uncaught error or unhandled rejection in the frame;
 * - `app.listener`: a `subscribe()` listener threw;
 * - `plugin.uncaught`: a failure the SDK caught to keep the app running (a plugin's setup, answer
 *   or drawing, the menu);
 * - `host.internal`: a call the host failed and showed the frame only as `call_failed`.
 *
 * More may come under these sources: branch on the source, and treat a kind you don't know by it.
 */
export type AppErrorKind = `${AppErrorSource}.${string}`;

const KIND = /^(call|app|plugin|host)\.[a-z][a-z0-9_]*$/;

/** Whether `kind` is a well-formed kind under a known source. */
export function isErrorKind (kind: unknown): kind is AppErrorKind {
  return typeof kind === 'string' && kind.length <= 100 && KIND.test(kind);
}

/** `fetchMore` as a kind's part: `fetch_more`. */
export function snakeCase (name: string): string {
  return name.replace(/([a-z0-9])([A-Z])/g, '$1_$2').replace(/[^A-Za-z0-9]+/g, '_').toLowerCase();
}

/** The call each failed call's error came from, as the app called it (`fetch_more`). */
const callTags = new WeakMap<object, string>();

/**
 * Note on `err` the call it failed, as the app called it: the outermost wins, since it is what the
 * app wrote (`query.export()` over `app.exportQuery`).
 */
export function tagCall (err: unknown, api: string): void {
  if (typeof err === 'object' && err !== null) callTags.set(err, api);
}

/**
 * The call `err` failed, if one was noted: on it, or on the error it wraps (a `call_failed` that
 * hides the original).
 */
export function callOf (err: unknown): string | undefined {
  for (let e = err, depth = 0; typeof e === 'object' && e !== null && depth < 5; e = (e as { cause?: unknown }).cause, depth += 1) {
    const tag = callTags.get(e);
    if (tag) return tag;
  }
  return undefined;
}

/** `to`, with the call `from` failed, if one was noted: the same failure, said again. */
export function copyCallTag<T extends object> (from: object, to: T): T {
  const tag = callTags.get(from);
  if (tag) callTags.set(to, tag);
  return to;
}

/**
 * `fn`, noting on each rejection the call `api`: a call the app leaves unhandled is then reported
 * by its name. What it throws at once, or answers, it passes on as it is.
 */
export function namedCall<F extends (...args: never[]) => unknown> (fn: F, api: string): F {
  return function (this: unknown, ...args: Parameters<F>) {
    const out = fn.apply(this, args);
    if (out && typeof (out as PromiseLike<unknown>).then === 'function') {
      return (out as Promise<unknown>).then(undefined, (err: unknown) => {
        tagCall(err, api);
        throw err;
      });
    }
    return out;
  } as F;
}

/** One runtime error of a mounted Data App. */
export interface AppErrorEvent {
  kind: AppErrorKind;
  /** The error's code and scope, when it is one of the SDK's (`DataAppError`); absent for an app's own `TypeError`. */
  code?: string;
  scope?: string;
  message: string;
  /** The server request that failed: its line in the server's log has the cause. */
  requestId?: string;
  /** The query or control it concerns. */
  entity?: string;
  /** Where it was thrown, truncated; not for `call.*`, whose stack is the SDK's. */
  stack?: string;
  /** For `host.internal`: the call the host failed (`export.query`). */
  call?: string;
  /** When it happened, ISO 8601. */
  at: string;
  /** The mount it happened in (set by the host): its id, and its title if it was given one. */
  mount: number;
  title?: string;
}

/** One `console.error` / `console.warn` line of a mounted Data App, reported only when the host captures them. */
export interface AppConsoleEvent {
  level: 'error' | 'warn';
  message: string;
  at: string;
  mount: number;
  title?: string;
}

/** The longest a message may be, and a stack. */
export const MESSAGE_LIMIT = 2000;
export const STACK_LIMIT = 4000;

/** `text` cut to `limit` characters, saying so. */
export function truncate (text: string, limit: number): string {
  return text.length <= limit ? text : `${text.slice(0, limit)}… (truncated)`;
}

/** The fields of an error that are reported: its shape, never a query's rows or a request's params. */
export function describeError (kind: AppErrorKind, err: unknown): Omit<AppErrorEvent, 'mount' | 'title'> {
  const e = err as { message?: unknown, code?: unknown, scope?: unknown, requestId?: unknown, entity?: unknown, stack?: unknown } | null | undefined;
  const isObject = typeof e === 'object' && e !== null;
  const out: Omit<AppErrorEvent, 'mount' | 'title'> = {
    kind,
    message: truncate(isObject && typeof e.message === 'string' ? e.message : String(err), MESSAGE_LIMIT),
    at: new Date().toISOString(),
  };
  if (!isObject) return out;
  for (const key of ['code', 'scope', 'requestId', 'entity'] as const) {
    const value = e[key];
    if (typeof value === 'string') out[key] = value;
  }
  if (!kind.startsWith('call.') && typeof e.stack === 'string') out.stack = truncate(e.stack, STACK_LIMIT);
  return out;
}

/**
 * A budget of so many events a minute: `take()` says whether one more may go now. Those refused
 * are counted, and `onDropped` told the count once their minute is over.
 */
export function createBudget (perMinute: number, onDropped: (count: number) => void = () => {}, now: () => number = Date.now): { take (): boolean, dispose (): void } {
  let windowStart = now();
  let taken = 0;
  let dropped = 0;
  let timer: ReturnType<typeof setTimeout> | undefined;
  const flush = () => {
    timer = undefined;
    const count = dropped;
    dropped = 0;
    if (count > 0) {
      try { onDropped(count); } catch { /* telling must not fail the next event */ }
    }
  };
  return {
    take () {
      const t = now();
      if (t - windowStart >= 60_000) {
        if (timer !== undefined) clearTimeout(timer);
        flush();
        windowStart = t;
        taken = 0;
      }
      if (taken < perMinute) {
        taken += 1;
        return true;
      }
      dropped += 1;
      // Said when the minute is over, whether or not another event comes.
      if (timer === undefined) timer = setTimeout(flush, Math.max(0, windowStart + 60_000 - t));
      return false;
    },
    dispose () {
      if (timer !== undefined) clearTimeout(timer);
      flush();
    },
  };
}

const str = (value: unknown, limit: number): string | undefined => (typeof value === 'string' ? truncate(value, limit) : undefined);
const SHORT = 200;

/** The fields of an error the frame reports, as the host takes them: the frame's word is not trusted. */
export function checkErrorEvent (payload: unknown, mount: { id: number, title?: string }): AppErrorEvent | undefined {
  const p = payload as Record<string, unknown> | null;
  if (!p || typeof p !== 'object') return undefined;
  const kind = p.kind;
  // `host.*` is the host's own to report: a frame cannot claim one.
  if (!isErrorKind(kind) || kind.startsWith('host.')) return undefined;
  const event: AppErrorEvent = {
    kind,
    message: str(p.message, MESSAGE_LIMIT) || 'An error with no message.',
    at: typeof p.at === 'string' && !Number.isNaN(Date.parse(p.at)) ? p.at : new Date().toISOString(),
    mount: mount.id,
  };
  const code = str(p.code, SHORT);
  const scope = str(p.scope, SHORT);
  const requestId = str(p.requestId, SHORT);
  const entity = str(p.entity, SHORT);
  const stack = kind.startsWith('call.') ? undefined : str(p.stack, STACK_LIMIT);
  if (code) event.code = code;
  if (scope) event.scope = scope;
  if (requestId) event.requestId = requestId;
  if (entity) event.entity = entity;
  if (stack) event.stack = stack;
  if (mount.title !== undefined) event.title = mount.title;
  return event;
}

/** A console line the frame reports, as the host takes it. */
export function checkConsoleEvent (payload: unknown, mount: { id: number, title?: string }): AppConsoleEvent | undefined {
  const p = payload as Record<string, unknown> | null;
  if (!p || typeof p !== 'object' || (p.level !== 'error' && p.level !== 'warn')) return undefined;
  const event: AppConsoleEvent = {
    level: p.level,
    message: str(p.message, MESSAGE_LIMIT) ?? '',
    at: typeof p.at === 'string' && !Number.isNaN(Date.parse(p.at)) ? p.at : new Date().toISOString(),
    mount: mount.id,
  };
  if (mount.title !== undefined) event.title = mount.title;
  return event;
}
