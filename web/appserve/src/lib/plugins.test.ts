import { coreApiBackend } from '@holistics/anfra-sdk/api';
import { describe, expect, it } from 'vitest';
import { installedPlugins } from './plugins';

const backend = coreApiBackend('/api', { datasets: {} });
const names = (settings: string[]) => installedPlugins(settings, backend).map((p) => p.name);

describe('installedPlugins', () => {
  it('installs core and every plugin the settings leave on', () => {
    expect(names(['inspect', 'lineage', 'view-underlying-data', 'render', 'export']))
      .toEqual(['core', 'export', 'lineage', 'render', 'inspect', 'view-underlying-data']);
  });

  it('leaves out a plugin the settings turn off, never core', () => {
    expect(names(['inspect', 'export'])).toEqual(['core', 'export', 'inspect']);
    expect(names([])).toEqual(['core']);
  });

  it('ignores a name it has no plugin for', () => {
    expect(names(['charts', 'render'])).toEqual(['core', 'render']);
  });
});
