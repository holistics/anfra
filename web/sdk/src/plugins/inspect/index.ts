/**
 * `inspect`: the panel's Queries and Structure tabs, and the command
 * that opens them. What the tabs show (the frame's snapshot, pick, highlight and locate) is the
 * runtime's, on every mount's panel, so tabs from other plugins have it too. `inspect` adds a log of
 * the frame's calls, kept per mount by its middleware. The tabs load the first time a host shows one.
 */
import type { TabContribution } from '../../host/contributions';
import { definePlugin, type Plugin } from '../../host/plugin';
import { PLUGIN_VERSION } from '../version';
import { QUERIES_TAB } from './model/types';
import { dropState, stateOf } from './state';

const tab = (id: string, title: string, icon: string, order: number, load: () => Promise<{ default: TabContribution['mount'] }>): TabContribution => ({
  id, title, icon, order, mount: async (root, context) => (await load()).default(root, context),
});

export function inspectPlugin (): Plugin {
  return definePlugin({
    name: 'inspect',
    version: PLUGIN_VERSION,
    host: {
      // Innermost, so the log has what reaches the plugin, and the trace of what it passed.
      middleware: [{
        name: 'inspect',
        match: '*',
        stage: 1000,
        handle: (request, context, next) => (context.mount ? stateOf(context.mount).record(request, context, next) : next(request)),
      }],
      mount: (mount) => {
        stateOf(mount);
        return () => dropState(mount);
      },
      commands: [{
        id: 'toggle',
        title: 'Inspect',
        icon: 'inspect',
        placement: 'primary',
        shortcut: 'Alt+Shift+I',
        run: { tab: QUERIES_TAB },
      }],
      tabs: [
        tab('queries', 'Queries', 'query', 30, () => import('./tabs/queries')),
        tab('structure', 'Structure', 'tree', 40, () => import('./tabs/structure')),
      ],
    },
  });
}
