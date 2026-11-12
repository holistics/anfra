/**
 * `core`: running a Data App's queries and suggesting a filter's values. Every host installs it:
 * the app model makes its calls, through the pipeline like any caller, so a host swaps or wraps it
 * as it would any plugin's. No frame half: the kernel is its frame side (`src/app/core.ts`), which
 * answers the host's `core.execute` and tells it `core.results`.
 */
import { invalidRequest } from '../../common/errors';
import type {
  Backend, BackendQueryRequest, ExecuteSummary, FieldSuggestionsRequest, RefreshOptions,
} from '../../common/types';
import { definePlugin, type MountContext, type Plugin } from '../../host/plugin';
import type { CommandState } from '../../host/contributions';
import { PLUGIN_VERSION } from '../version';

/** What `core` answers with: a Backend's two required methods, `coreApiBackend` on any host. */
export type CoreBackend = Pick<Backend, 'submitQuery' | 'fieldSuggestions'>;

/** How often the Refresh command's age of cached figures is read again, while one shows. */
const AGE_TICK_MS = 60_000;

/** Each frame's refresh, and what its last `core.results` said. */
interface CoreState {
  refreshing: boolean;
  /** While a cached result shows: when the oldest one on screen ran, if the frame said. */
  cache?: { since?: number };
  /** Signals the command's state each minute while a cached result shows. */
  ticker?: ReturnType<typeof setInterval>;
}

/** The frame's `core.results`, checked: `{ cached, cachedSince? }`, or dropped. */
function checkResults (payload: unknown): { cached: boolean, cachedSince?: string } | undefined {
  const { cached, cachedSince } = (payload ?? {}) as { cached?: unknown, cachedSince?: unknown };
  if (typeof cached !== 'boolean') return undefined;
  if (cachedSince === undefined) return { cached };
  if (typeof cachedSince !== 'string' || Number.isNaN(Date.parse(cachedSince))) return undefined;
  return { cached, cachedSince };
}

const UNITS: readonly [Intl.RelativeTimeFormatUnit, number][] = [
  ['day', 86_400_000], ['hour', 3_600_000], ['minute', 60_000],
];

/** How long ago `at` was, for a reader: `5 minutes ago`, `2 hours ago`, `a moment ago`. */
export function timeAgo (at: number, now = Date.now()): string {
  const elapsed = Math.max(0, now - at);
  const format = new Intl.RelativeTimeFormat('en', { numeric: 'always' });
  for (const [unit, ms] of UNITS) {
    if (elapsed >= ms) return format.format(-Math.floor(elapsed / ms), unit);
  }
  return 'a moment ago';
}

export function corePlugin (backend: CoreBackend): Plugin {
  const states = new WeakMap<MountContext, CoreState>();
  const stopTicker = (state: CoreState) => {
    if (state.ticker !== undefined) clearInterval(state.ticker);
    state.ticker = undefined;
  };
  return definePlugin({
    name: 'core',
    version: PLUGIN_VERSION,
    host: {
      mount (mount) {
        const state: CoreState = { refreshing: false };
        states.set(mount, state);
        const show = (cache: CoreState['cache']) => {
          if (cache?.since === state.cache?.since && !cache === !state.cache) return;
          state.cache = cache;
          stopTicker(state);
          // Read again each minute, so the age shown stays true.
          if (cache?.since !== undefined) state.ticker = setInterval(() => mount.commandsChanged(), AGE_TICK_MS);
          mount.commandsChanged();
        };
        const stopResults = mount.on('core.results', ({ cached, cachedSince }: { cached: boolean, cachedSince?: string }) => {
          show(cached ? (cachedSince ? { since: Date.parse(cachedSince) } : {}) : undefined);
        });
        // A new document has shown no result yet.
        const stopReady = mount.onReady(() => show(undefined));
        return () => {
          stopResults();
          stopReady();
          stopTicker(state);
        };
      },
      calls: {
        submitQuery: { frame: true, handler: (request: BackendQueryRequest, { signal }) => backend.submitQuery(request, signal) },
        fieldSuggestions: { frame: true, handler: (request: FieldSuggestionsRequest, { signal }) => backend.fieldSuggestions(request, signal) },
        /**
         * Run every query of every app in the frame again, with its current inputs, skipping any
         * result cache unless `bustCache` is false: the frame's `core.execute`, one at a time.
         */
        async refresh (request: RefreshOptions | null, { mount, call, signal }): Promise<ExecuteSummary> {
          const state = mount && states.get(mount);
          if (!mount || !state) throw invalidRequest('A refresh is for a mounted Data App.');
          const bustCache = request?.bustCache ?? true;
          if (typeof bustCache !== 'boolean') throw invalidRequest(`bustCache is true or false, not ${JSON.stringify(bustCache)}.`);
          if (state.refreshing) throw invalidRequest('A refresh is running already.');
          state.refreshing = true;
          mount.commandsChanged();
          try {
            return await call<ExecuteSummary>('core.execute', { force: true, bustCache }, { signal });
          } finally {
            state.refreshing = false;
            mount.commandsChanged();
          }
        },
      },
      events: { results: checkResults },
      commands: [{
        id: 'refresh',
        title: 'Refresh',
        icon: 'refresh',
        placement: 'primary',
        shortcut: 'Alt+Shift+R',
        run: { call: 'core.refresh', request: { bustCache: true } },
        state: ({ mount }): CommandState => {
          const state = states.get(mount);
          if (state?.refreshing) return { disabled: 'Refreshing…' };
          if (!state?.cache) return {};
          return { detail: state.cache.since !== undefined ? `Data from ${timeAgo(state.cache.since)}` : 'Data from a cache' };
        },
      }],
    },
  });
}
