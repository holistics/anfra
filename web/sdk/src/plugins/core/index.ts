/**
 * `core`: running a Data App's queries and suggesting a filter's values. Every host installs it:
 * the app model makes its calls, through the pipeline like any caller, so a host swaps or wraps it
 * as it would any plugin's. No frame half: the kernel is its frame side.
 */
import type { Backend, BackendQueryRequest, FieldSuggestionsRequest } from '../../common/types';
import { definePlugin, type Plugin } from '../../host/plugin';
import { PLUGIN_VERSION } from '../version';

/** What `core` answers with: a Backend's two required methods, `coreApiBackend` on any host. */
export type CoreBackend = Pick<Backend, 'submitQuery' | 'fieldSuggestions'>;

export function corePlugin (backend: CoreBackend): Plugin {
  return definePlugin({
    name: 'core',
    version: PLUGIN_VERSION,
    host: {
      calls: {
        submitQuery: { frame: true, handler: (request: BackendQueryRequest, { signal }) => backend.submitQuery(request, signal) },
        fieldSuggestions: { frame: true, handler: (request: FieldSuggestionsRequest, { signal }) => backend.fieldSuggestions(request, signal) },
      },
    },
  });
}
