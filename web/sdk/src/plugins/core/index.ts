/**
 * `core`: running a Data App's queries and suggesting a filter's values. Every host installs it:
 * the app model makes its calls, through the pipeline like any caller, so a host swaps or wraps it
 * as it would any plugin's. No frame half: the kernel is its frame side (`src/app/core.ts`), which
 * answers the host's `core.execute`.
 */
import { invalidRequest } from '../../common/errors';
import type {
  Backend, BackendQueryRequest, ExecuteSummary, FieldSuggestionsRequest,
} from '../../common/types';
import { definePlugin, type MountContext, type Plugin } from '../../host/plugin';
import type { CommandState } from '../../host/contributions';
import { PLUGIN_VERSION } from '../version';

/** What `core` answers with: a Backend's two required methods, `coreApiBackend` on any host. */
export type CoreBackend = Pick<Backend, 'submitQuery' | 'fieldSuggestions'>;

/** Each frame's refresh: whether one is running. */
interface CoreState {
  refreshing: boolean;
}

export function corePlugin (backend: CoreBackend): Plugin {
  const states = new WeakMap<MountContext, CoreState>();
  return definePlugin({
    name: 'core',
    version: PLUGIN_VERSION,
    host: {
      mount (mount) {
        states.set(mount, { refreshing: false });
      },
      calls: {
        submitQuery: { frame: true, handler: (request: BackendQueryRequest, { signal }) => backend.submitQuery(request, signal) },
        fieldSuggestions: { frame: true, handler: (request: FieldSuggestionsRequest, { signal }) => backend.fieldSuggestions(request, signal) },
        /**
         * Run every query of every app in the frame again, with its current inputs, as each app's
         * `refresh()` does: the frame's `core.execute`, one at a time.
         */
        async refresh (_request: unknown, { mount, call, signal }): Promise<ExecuteSummary> {
          const state = mount && states.get(mount);
          if (!mount || !state) throw invalidRequest('A refresh is for a mounted Data App.');
          if (state.refreshing) throw invalidRequest('A refresh is running already.');
          state.refreshing = true;
          mount.commandsChanged();
          try {
            return await call<ExecuteSummary>('core.execute', null, { signal });
          } finally {
            state.refreshing = false;
            mount.commandsChanged();
          }
        },
      },
      commands: [{
        id: 'refresh',
        title: 'Refresh',
        icon: 'refresh',
        placement: 'primary',
        run: { call: 'core.refresh' },
        state: ({ mount }): CommandState => (states.get(mount)?.refreshing ? { disabled: 'Refreshing…' } : {}),
      }],
    },
  });
}
