/**
 * `inspect`: the panel's Structure and Data tabs, and the command that opens them. What the tabs
 * show (the frame's snapshot, pick, highlight and locate) is the runtime's, on every mount's panel,
 * so tabs from other plugins have it too.
 */
import { definePlugin, type Plugin } from '../../host/plugin';
import { PLUGIN_VERSION } from '../version';

export function inspectPlugin (): Plugin {
  return definePlugin({
    name: 'inspect',
    version: PLUGIN_VERSION,
  });
}
