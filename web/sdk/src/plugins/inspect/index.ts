/**
 * `inspect`: the panel's Structure and Data tabs, and the command that opens them. What the tabs
 * show (the frame's snapshot, pick, highlight and locate) is the runtime's, on every mount's panel,
 * so tabs from other plugins have it too. The tabs load the first time a host shows one.
 */
import { definePlugin, type Plugin } from '../../host/plugin';
import { PLUGIN_VERSION } from '../version';

export function inspectPlugin (): Plugin {
  return definePlugin({
    name: 'inspect',
    version: PLUGIN_VERSION,
    host: {
      commands: [{
        id: 'toggle',
        title: 'Inspect',
        icon: 'inspect',
        placement: 'primary',
        shortcut: 'Alt+Shift+I',
        run: { tab: 'inspect.structure' },
      }],
      tabs: [
        {
          id: 'structure',
          title: 'Structure',
          icon: 'tree',
          order: 10,
          mount: async (root, context) => (await import('./tabs')).structureTab(root, context),
        },
        {
          id: 'data',
          title: 'Data',
          icon: 'table',
          order: 20,
          mount: async (root, context) => (await import('./tabs')).dataTab(root, context),
        },
      ],
    },
  });
}
