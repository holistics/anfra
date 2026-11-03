/**
 * `view-underlying-data`: "View underlying data" on a chart point's menu (`query.menu`), a modal
 * listing the rows of the model behind the point. Frame-only: its queries are `core`'s and its
 * download `export`'s, made through the pipeline. The modal's view is the frame body, so frames
 * whose readers never open it don't carry it.
 */
import { definePlugin, type Plugin } from '../../host/plugin';
import { PLUGIN_VERSION } from '../version';
import setup from 'anfra-sdk:setup/view-underlying-data';
import body from 'anfra-sdk:body/view-underlying-data';

export function viewUnderlyingDataPlugin (): Plugin {
  return definePlugin({
    name: 'view-underlying-data',
    version: PLUGIN_VERSION,
    requires: ['core'],
    frame: { setup, body },
  });
}
