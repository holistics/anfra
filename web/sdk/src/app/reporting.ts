/**
 * The frame's half of the error stream: what goes wrong at runtime in a Data App is reported to its
 * host as `anfra.error`, whether or not anyone inspects it, and its console lines as
 * `anfra.console` when the host captures them (docs/designs/errors.md, "In a Data App").
 *
 * The app model reports through `reportError`, which does nothing until a frame's bootstrap
 * installs a reporter: outside a frame (tests, an SDK used on its own page) nothing is sent.
 */
import {
  MESSAGE_LIMIT, createBudget, describeError, truncate, type AppErrorKind,
} from '../common/runtimeErrors';
import type { FrameKernel } from './kernel';

type Reporter = (kind: Exclude<AppErrorKind, 'hidden'>, err: unknown) => void;

let reporter: Reporter | undefined;

/** Report a runtime error to the host, if this is a frame. Never throws. */
export function reportError (kind: Exclude<AppErrorKind, 'hidden'>, err: unknown): void {
  try { reporter?.(kind, err); } catch { /* reporting must not add a failure of its own */ }
}

/**
 * How many runtime errors a frame sends in a minute, at most, and console lines apart from them: a
 * render loop that throws or warns every frame must not flood its host, and its console lines must
 * not crowd out its errors.
 */
export const EVENTS_PER_MINUTE = 120;

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
  // What is over a budget is still in the frame's own console, and how much of it is said there.
  const dropped = (what: string) => (count: number) => (win as Window & typeof globalThis).console?.warn(
    `anfra: ${count} ${what} over the ${EVENTS_PER_MINUTE} a minute a frame sends were not sent to the host.`,
  );
  const errors = createBudget(EVENTS_PER_MINUTE, dropped('runtime errors'), now);
  const lines = createBudget(EVENTS_PER_MINUTE, dropped('console lines'), now);

  reporter = (kind, err) => {
    if (errors.take()) own.emit('error', describeError(kind, err));
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
        // The SDK's own notes (`anfra: …`) say what it reports as an error already, through
        // `reportError`: each one that writes such a note must also report it.
        if (typeof args[0] === 'string' && args[0].startsWith('anfra:')) return;
        try {
          if (lines.take()) own.emit('console', { level, message: truncate(args.map(textOf).join(' '), MESSAGE_LIMIT), at: new Date().toISOString() });
        } catch { /* a line that cannot be sent is still in the frame's console */ }
      };
    }
  }

  return () => {
    reporter = undefined;
    errors.dispose();
    lines.dispose();
    win.removeEventListener('error', onError);
    win.removeEventListener('unhandledrejection', onRejection);
    for (const [level, original] of Object.entries(originals)) console[level as 'error' | 'warn'] = original;
  };
}
