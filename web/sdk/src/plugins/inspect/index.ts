/**
 * `inspect`: the panel's Overview, Data, Queries, Structure and Declaration tabs, the command
 * that opens them, and Inspect in the menu on a chart's rows, which reveals their query.
 * What the tabs show (the frame's snapshot, pick, highlight and locate, the reveal) is the runtime's,
 * on every mount's panel, so tabs from other plugins have it too. `inspect` adds a log of
 * the frame's calls, kept per mount by its middleware. The tabs load the first time a host shows one.
 */
import type { TabContribution } from '../../host/contributions';
import { definePlugin, type Plugin } from '../../host/plugin';
import { PLUGIN_VERSION } from '../version';
import { OVERVIEW_TAB } from './model/types';
import { dropState, stateOf } from './state';

const tab = (id: string, title: string, icon: string, order: number, load: () => Promise<{ default: TabContribution['mount'] }>, selection?: TabContribution['selection']): TabContribution => ({
  id,
  title,
  icon,
  order,
  selection,
  mount: async (root, context) => {
    stateOf(context.mount).follow(context.panel);
    return (await load()).default(root, context);
  },
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
        const state = stateOf(mount);
        // Kept from the first, whether or not the panel is open: it lists them once it is.
        const off = [mount.on('anfra.error', state.addError), mount.on('anfra.console', state.addConsole)];
        return () => {
          off.forEach((stop) => stop());
          dropState(mount);
        };
      },
      commands: [{
        id: 'toggle',
        title: 'Inspect',
        icon: 'inspect',
        placement: 'primary',
        shortcut: 'Alt+Shift+I',
        run: { tab: OVERVIEW_TAB },
      }, {
        // In the menu on a chart's rows: the panel's reveal of their query, on the host's tab
        // for one (Data unless it set another). Never on a block: a block isn't revealed.
        id: 'reveal',
        title: 'Inspect',
        icon: 'inspect',
        target: 'rows',
        run: { panel: 'reveal' },
      }],
      tabs: [
        tab('overview', 'Overview', 'overview', 10, () => import('./tabs/overview')),
        tab('data', 'Data', 'table', 20, () => import('./tabs/data')),
        tab('queries', 'Queries', 'query', 30, () => import('./tabs/queries')),
        tab('structure', 'Structure', 'tree', 40, () => import('./tabs/structure'), 'pick'),
        tab('declaration', 'Declaration', 'code', 50, () => import('./tabs/declaration')),
      ],
    },
  });
}
