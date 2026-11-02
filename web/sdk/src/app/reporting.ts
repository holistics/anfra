/**
 * The frame's half of the error stream: what goes wrong at runtime in a Data App is reported to its
 * host as `anfra.error`, whether or not anyone inspects it, and its console lines as
 * `anfra.console` when the host captures them (docs/designs/errors.md, "In a Data App").
 *
 * The app model reports through `reportError`, which does nothing until a frame's bootstrap
 * installs a reporter: outside a frame (tests, an SDK used on its own page) nothing is sent.
 */
import { MESSAGE_LIMIT, STACK_LIMIT, truncate, type AppErrorKind } from '../common/runtimeErrors';
import type { FrameKernel } from './kernel';

type Reporter = (kind: Exclude<AppErrorKind, 'hidden'>, err: unknown) => void;

let reporter: Reporter | undefined;

/** Report a runtime error to the host, if this is a frame. Never throws. */
export function reportError (kind: Exclude<AppErrorKind, 'hidden'>, err: unknown): void {
  try { reporter?.(kind, err); } catch { /* reporting must not add a failure of its own */ }
}

/** How many events a frame sends in a minute, at most: a render loop that throws every frame must not flood its host. */
export const EVENTS_PER_MINUTE = 120;

/** The fields of an error that are reported: its shape, never a query's rows or a request's params. */
export function describeError (kind: AppErrorKind, err: unknown): Record<string, unknown> {
  const e = err as { message?: unknown, code?: unknown, scope?: unknown, requestId?: unknown, entity?: unknown, stack?: unknown } | null | undefined;
  const isObject = typeof e === 'object' && e !== null;
  const out: Record<string, unknown> = {
    kind,
    message: truncate(isObject && typeof e.message === 'string' ? e.message : String(err), MESSAGE_LIMIT),
    at: new Date().toISOString(),
  };
  if (!isObject) return out;
  for (const key of ['code', 'scope', 'requestId', 'entity'] as const) {
    if (typeof e[key] === 'string') out[key] = e[key];
  }
  if (kind !== 'query' && typeof e.stack === 'string') out.stack = truncate(e.stack, STACK_LIMIT);
  return out;
}

/** One console argument as text: an error's message, an object as JSON, as far as it goes. */
function textOf (arg: unknown): string {
  if (typeof arg === 'string') return arg;
  if (arg instanceof Error) return `${arg.name}: ${arg.message}`;
  try {
    return JSON.stringify(arg) ?? String(arg);
  } catch {
    return String(arg);
  }
}

/**
 * Install the frame's reporting: its uncaught errors and unhandled rejections, what the app model
 * reports, and, when `captureConsole`, its `console.error` and `console.warn` lines.
 */
export function installReporting (own: FrameKernel, win: Window, captureConsole: boolean, now: () => number = Date.now): () => void {
  let windowStart = now();
  let sent = 0;
  const allowed = () => {
    const t = now();
    if (t - windowStart >= 60_000) {
      windowStart = t;
      sent = 0;
    }
    sent += 1;
    return sent <= EVENTS_PER_MINUTE;
  };

  reporter = (kind, err) => {
    if (allowed()) own.emit('error', describeError(kind, err));
  };

  const onError = (event: ErrorEvent) => reportError('uncaught', event.error ?? event.message);
  const onRejection = (event: PromiseRejectionEvent) => reportError('uncaught', event.reason);
  win.addEventListener('error', onError);
  win.addEventListener('unhandledrejection', onRejection);

  const console = (win as Window & typeof globalThis).console;
  const originals: Partial<Record<'error' | 'warn', (...args: unknown[]) => void>> = {};
  if (captureConsole && console) {
    for (const level of ['error', 'warn'] as const) {
      const original = console[level];
      originals[level] = original;
      console[level] = (...args: unknown[]) => {
        original.apply(console, args);
        // The SDK's own notes (`anfra: …`) say what is reported as an error already.
        if (typeof args[0] === 'string' && args[0].startsWith('anfra:')) return;
        try {
          if (allowed()) own.emit('console', { level, message: truncate(args.map(textOf).join(' '), MESSAGE_LIMIT), at: new Date().toISOString() });
        } catch { /* a line that cannot be sent is still in the frame's console */ }
      };
    }
  }

  return () => {
    reporter = undefined;
    win.removeEventListener('error', onError);
    win.removeEventListener('unhandledrejection', onRejection);
    for (const [level, original] of Object.entries(originals)) console[level as 'error' | 'warn'] = original;
  };
}
