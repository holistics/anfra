import {
  afterEach, describe, expect, it, vi,
} from 'vitest';
import { inspect, revenueResult, type Harness } from './tabs/testSupport';

let harness: Harness | undefined;

afterEach(() => {
  harness?.unmount();
  harness = undefined;
  vi.restoreAllMocks();
});

describe('inspect', () => {
  it('opens the panel at Overview from its command, and lists its five tabs in order', async () => {
    harness = await inspect();
    expect(harness.host.tabs.map((t) => t.id)).toEqual(['inspect.overview', 'inspect.data', 'inspect.queries', 'inspect.structure', 'inspect.declaration']);
    // The SDK's Pick first, since Structure shows the selection.
    expect(harness.host.commands.map((c) => c.id)).toEqual(['panel.pick', 'inspect.toggle']);
    expect(harness.host.commands[1]).toMatchObject({ id: 'inspect.toggle', shortcut: 'Alt+Shift+I' });
    expect(harness.mounted.panel).toMatchObject({ open: true, tab: 'inspect.overview' });
  });

  it("logs the frame's calls with what they asked and a few rows, and not the host page's", async () => {
    harness = await inspect();
    const request = { dataset: 'sales', aql: 'explore sales {}', input: { filters: [] }, page: 1 };
    await harness.frameCall('core.submitQuery', request);
    await harness.mounted.call('core.submitQuery', request);
    const entries = harness.state.requests.entries();
    expect(entries).toHaveLength(1);
    expect(entries[0]).toMatchObject({
      call: 'core.submitQuery', caller: 'frame', status: 'ok', dataset: 'sales', aql: 'explore sales {}', page: 1,
      numRows: 2, columns: ['region', 'revenue'], preview: revenueResult.values,
    });
  });

  it('logs a failed call as an error', async () => {
    harness = await inspect({ submitQuery: () => { throw new Error('warehouse down'); } });
    await expect(harness.frameCall('core.submitQuery', { dataset: 'sales', aql: 'x', input: {} })).rejects.toThrow();
    expect(harness.state.requests.entries()[0]).toMatchObject({ status: 'error', error: 'Error: warehouse down' });
  });
});
