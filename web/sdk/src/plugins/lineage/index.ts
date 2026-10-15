/** `lineage`: tracing fields through the repo's catalog (`Anfra.lineage()`). */
import type { Backend, BackendLineageRequest } from '../../common/types';
import { definePlugin, type Plugin } from '../../host/plugin';
import { PLUGIN_VERSION } from '../version';
import setup from 'anfra-sdk:setup/lineage';

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
    },
    frame: { setup },
  });
}
