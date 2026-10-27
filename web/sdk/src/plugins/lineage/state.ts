/**
 * What the Lineage tab keeps per mount, beyond the panel: the last trace, and what the reader
 * chose to see. A tab is taken down when another is shown; this outlives it. A remount starts afresh.
 */
import type { LineageResult, LineageTarget } from '../../common/types';
import type { MountContext } from '../../host/plugin';
import { targetKey, type LineageKind, type Traced } from './model';

/** What the catalog answered for the fields asked (`key`): a trace outlives the tab that started it. */
export interface LineageTrace {
  key: string;
  status: 'loading' | 'ready' | 'failed';
  traced: Traced;
  error?: string;
}

export interface LineageView {
  hidden: Set<LineageKind>;
  /** Whether the cross-filters between queries are drawn. */
  crossFilters: boolean;
  /** The card the canvas is narrowed to, with its path. */
  focus?: string;
  /** The card whose path is pinned. */
  selected?: string;
}

export interface LineageState {
  readonly trace: LineageTrace | undefined;
  view: LineageView;
  /**
   * Trace `targets` unless this set of fields is traced or being traced. A failed trace is traced
   * again only when `retry` says so: the tab retries once per mount.
   */
  traceFor: (targets: readonly LineageTarget[], run: (targets: LineageTarget[]) => Promise<LineageResult>, retry: boolean) => void;
  /** Tell the tabs the state changed. */
  changed: () => void;
  /** Each change. Returns the function that stops listening. */
  subscribe: (listener: () => void) => () => void;
}

const states = new WeakMap<MountContext, LineageState>();

/** The mount's state, made on first use. */
export function stateOf (mount: MountContext): LineageState {
  let state = states.get(mount);
  if (!state) {
    state = createState();
    states.set(mount, state);
  }
  return state;
}

/** Forget a mount's state, when it is unmounted. */
export function dropState (mount: MountContext): void {
  states.delete(mount);
}

/** Each set of fields traced, by its key: the targets' keys, sorted, joined. */
export const traceKey = (targets: readonly LineageTarget[]) => targets.map(targetKey).sort().join('|');

function createState (): LineageState {
  const listeners = new Set<() => void>();
  const changed = () => listeners.forEach((listener) => {
    try { listener(); } catch { /* one listener must not stop the others */ }
  });
  let trace: LineageTrace | undefined;
  const state: LineageState = {
    get trace () { return trace; },
    view: { hidden: new Set(), crossFilters: false },
    traceFor (targets, run, retry) {
      const key = traceKey(targets);
      if (trace?.key === key && (trace.status !== 'failed' || !retry)) return;
      if (targets.length === 0) {
        trace = { key, status: 'ready', traced: new Map() };
        return;
      }
      // The tab asking is drawing: it reads the trace as loading without being told.
      trace = { key, status: 'loading', traced: new Map() };
      run([...targets]).then(
        (result) => {
          if (trace?.key !== key) return;
          trace = { key, status: 'ready', traced: new Map(result.results.map((r) => [targetKey(r.target), r])) };
          changed();
        },
        (error: unknown) => {
          if (trace?.key !== key) return;
          trace = { key, status: 'failed', traced: new Map(), error: error instanceof Error ? error.message : String(error) };
          changed();
        },
      );
    },
    changed,
    subscribe (listener) {
      listeners.add(listener);
      return () => { listeners.delete(listener); };
    },
  };
  return state;
}
