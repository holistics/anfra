// The SDK plugins a Data App gets on anfra serve: core, always, and those the repo's settings leave
// on (the context's `plugins`, apps.plugins in .anfra/config.yml with config.local.yml over it).
import type { Backend } from '@holistics/anfra-sdk/common';
import type { Plugin } from '@holistics/anfra-sdk/host';
import {
  corePlugin, exportPlugin, inspectPlugin, lineagePlugin, renderPlugin, viewUnderlyingDataPlugin,
} from '@holistics/anfra-sdk/plugins';

// Every plugin a repo's settings turn on or off, by name, in the order they install: the panel's
// tabs follow it.
const optional: Record<string, (backend: Backend) => Plugin> = {
  export: (backend) => exportPlugin(backend),
  lineage: (backend) => lineagePlugin(backend),
  render: () => renderPlugin(),
  inspect: () => inspectPlugin(),
  'view-underlying-data': () => viewUnderlyingDataPlugin(),
};

/** core, and each plugin `on` names, over `backend`. A name anfra serve has no plugin for is left out. */
export function installedPlugins (on: readonly string[], backend: Backend): Plugin[] {
  return [
    corePlugin(backend),
    ...Object.entries(optional).filter(([name]) => on.includes(name)).map(([, make]) => make(backend)),
  ];
}
