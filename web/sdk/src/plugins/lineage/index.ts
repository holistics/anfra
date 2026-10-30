/**
 * `lineage`: tracing fields through the repo's catalog (`Anfra.lineage()`), and the panel's Lineage
 * tab, which draws where each number on the page comes from. The tab loads the first time a host
 * shows it.
 */
import type { Backend, BackendLineageRequest } from '../../common/types';
import { definePlugin, type Plugin } from '../../host/plugin';
import { PLUGIN_VERSION } from '../version';
import setup from 'anfra-sdk:setup/lineage';
import { dropState, stateOf } from './state';

/** `backend` must have `lineage` (`coreApiBackend` does). */
export function lineagePlugin (backend: Pick<Backend, 'lineage'>): Plugin {
  const { lineage } = backend;
  if (typeof lineage !== 'function') throw new Error('lineagePlugin: the Backend has no lineage.');
  return definePlugin({
    name: 'lineage',
    version: PLUGIN_VERSION,
    host: {
      calls: {
        trace: { frame: true, handler: (request: BackendLineageRequest, { signal }) => lineage.call(backend, request, signal) },
      },
      mount: (mount) => {
        stateOf(mount);
        return () => dropState(mount);
      },
      tabs: [{
        id: 'lineage',
        title: 'Lineage',
        icon: 'lineage',
        // Between `inspect`'s Queries (30) and Structure (40).
        order: 35,
        selection: true,
        mount: async (root, context) => (await import('./tab')).default(root, context),
      }],
    },
    frame: { setup },
  });
}
