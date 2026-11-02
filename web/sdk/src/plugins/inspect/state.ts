/**
 * What `inspect` keeps per mount, beyond the panel: the frame's request log, and the tabs' state
 * that outlives a tab (a tab is taken down when another is shown). A remount starts afresh.
 */
import type { MountContext } from '../../host/plugin';
import type { Panel } from '../../host/panel';
import type { AppConsoleEvent, AppErrorEvent } from '../../common/runtimeErrors';
import { createRequestLog, type RequestLog, type RequestRecorder } from './model/requests';
import {
  DATA_TAB, DECLARATION_TAB, QUERIES_TAB, STRUCTURE_TAB, type QueriesFilter, type RevealTarget,
} from './model/types';

/** A runtime error as the panel keeps it: the same one again counts up instead of filling the list. */
export interface RuntimeEntry {
  event: AppErrorEvent;
  count: number;
}

/** How many runtime errors, and console lines, a mount keeps: the newest. */
export const RUNTIME_LIMIT = 50;
export const CONSOLE_LIMIT = 200;

export interface InspectState {
  readonly requests: RequestLog;
  /**
   * The frame's runtime errors since it loaded, newest first, but a query's: the query's own state
   * says whether it is failing now.
   */
  readonly errors: readonly RuntimeEntry[];
  /** The frame's console lines, oldest first, when the host captures them. */
  readonly console: readonly AppConsoleEvent[];
  addError: (event: AppErrorEvent) => void;
  addConsole: (event: AppConsoleEvent) => void;
  readonly record: RequestRecorder['record'];
  /**
   * Queries' search and filter, its open rows (`query:<app>/<name>`) and closed sections
   * (`<app>:queries`, `<app>:controls`, `<app>:selection`, `other`).
   */
  queries: { search: string, filter: QueriesFilter, open: Set<string>, closed: Set<string> };
  /** Data's open tables, `<app>/<name>`. */
  data: { open: Set<string> };
  /** The last link followed, for the tab it opened to bring into view; a new nonce repeats it. */
  readonly revealed: { target: RevealTarget, nonce: number } | undefined;
  /** Follow a link: open its tab, set what the tab shows, and say what to bring into view. */
  reveal: (panel: Panel, target: RevealTarget) => void;
  /** Each change to the log or a reveal. Returns the function that stops listening. */
  subscribe: (listener: () => void) => () => void;
}

const states = new WeakMap<MountContext, InspectState>();

/** The mount's state, made on first use. */
export function stateOf (mount: MountContext): InspectState {
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

function createState (): InspectState {
  const recorder = createRequestLog();
  const listeners = new Set<() => void>();
  const changed = () => listeners.forEach((listener) => {
    try { listener(); } catch { /* one listener must not stop the others */ }
  });
  let nonce = 0;
  const errors: RuntimeEntry[] = [];
  const lines: AppConsoleEvent[] = [];
  const state: InspectState = {
    requests: recorder.log,
    errors,
    console: lines,
    addError (event) {
      if (event.kind === 'query') return;
      const same = errors.find((entry) => entry.event.kind === event.kind && entry.event.message === event.message && entry.event.call === event.call);
      if (same) {
        same.count += 1;
        same.event = event;
        errors.splice(errors.indexOf(same), 1);
        errors.unshift(same);
      } else {
        errors.unshift({ event, count: 1 });
        errors.length = Math.min(errors.length, RUNTIME_LIMIT);
      }
      changed();
    },
    addConsole (event) {
      lines.push(event);
      if (lines.length > CONSOLE_LIMIT) lines.splice(0, lines.length - CONSOLE_LIMIT);
      changed();
    },
    record: recorder.record,
    queries: { search: '', filter: 'all', open: new Set(), closed: new Set() },
    data: { open: new Set() },
    revealed: undefined,
    reveal (panel, target) {
      (state as { revealed: InspectState['revealed'] }).revealed = { target, nonce: ++nonce };
      if ('node' in target) {
        panel.show(STRUCTURE_TAB, target.node);
      } else if ('entity' in target) {
        const key = `${target.entity}:${target.app}/${target.name}`;
        state.queries.open.add(key);
        state.queries.closed.delete(`${target.app}:${target.entity === 'query' ? 'queries' : 'controls'}`);
        // Nothing that would hide it.
        state.queries.search = '';
        state.queries.filter = 'all';
        panel.show(QUERIES_TAB);
      } else if ('table' in target) {
        state.data.open.add(`${target.app}/${target.table}`);
        panel.show(DATA_TAB);
      } else if ('check' in target) {
        panel.show(DECLARATION_TAB);
      } else {
        if (target.tab === QUERIES_TAB) {
          state.queries.search = '';
          state.queries.filter = target.filter ?? 'all';
        }
        panel.show(target.tab);
      }
      changed();
    },
    subscribe (listener) {
      listeners.add(listener);
      return () => { listeners.delete(listener); };
    },
  };
  recorder.log.subscribe(changed);
  return state;
}
