import {
  afterEach, describe, expect, it, vi,
} from 'vitest';
import {
  byTestId, inspect, revenueResult, salesSnapshot, type Harness,
} from './tabs/testSupport';

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
    expect(harness.host.commands.map((c) => c.id)).toEqual(['panel.pick', 'core.refresh', 'inspect.toggle', 'inspect.reveal']);
    expect(harness.host.commands[2]).toMatchObject({ id: 'inspect.toggle', shortcut: 'Alt+Shift+I' });
    expect(harness.mounted.panel).toMatchObject({ open: true, tab: 'inspect.overview' });
  });

  it("logs the frame's calls with what they asked and a few rows, and not the host page's", async () => {
    harness = await inspect();
    const request = { dataset: 'sales', aql: 'explore sales {}', input: { filters: [], pagination: { page: 1, pageSize: 10 } } };
    await harness.frameCall('core.submitQuery', request);
    await harness.mounted.call('core.submitQuery', request);
    const entries = harness.state.requests.entries();
    expect(entries).toHaveLength(1);
    expect(entries[0]).toMatchObject({
      call: 'core.submitQuery', caller: 'frame', status: 'ok', dataset: 'sales', aql: 'explore sales {}', input: request.input,
      numRows: 2, columns: ['region', 'revenue'], preview: revenueResult.values,
    });
  });

  it('logs a failed call as an error', async () => {
    harness = await inspect({ submitQuery: () => { throw new Error('warehouse down'); } });
    await expect(harness.frameCall('core.submitQuery', { dataset: 'sales', aql: 'x', input: {} })).rejects.toThrow();
    expect(harness.state.requests.entries()[0]).toMatchObject({ status: 'error', error: 'Error: warehouse down' });
  });

  describe("the menu's Inspect", () => {
    const snapshot = salesSnapshot();
    const block = {
      kind: 'node', node: { ...snapshot.structure.nodes[0], children: [] }, ancestors: [], rect: { x: 0, y: 0, width: 1, height: 1 },
    };
    const rows = {
      kind: 'rows', app: 0, query: 'orders', rows: [{ status: 'paid' }], fields: ['status'], conditions: [], node: block,
    };
    const card = (root: ShadowRoot, name: string) => byTestId(root, `table-0/${name}`)!.querySelector('details')!;

    it('is offered once on a rows menu and on a block', async () => {
      harness = await inspect();
      const rowsMenu = await harness.frameCall('anfra.menu', rows);
      const nodeMenu = await harness.frameCall('anfra.menu', block);
      expect(rowsMenu).toEqual([{ id: 'inspect.reveal', label: 'Inspect' }]);
      expect(nodeMenu).toEqual([{ id: 'inspect.reveal', label: 'Inspect' }]);
    });

    it("reveals the rows' query on Data", async () => {
      harness = await inspect();
      harness.mounted.panel.hide();
      await harness.frameCall('anfra.run', { id: 'inspect.reveal', target: rows });
      expect(harness.mounted.panel).toMatchObject({ open: true, tab: 'inspect.data' });
      harness.post(snapshot);
      const { root } = await harness.tab('inspect.data');
      expect(card(root, 'orders').open).toBe(true);
      await vi.waitFor(() => expect(card(root, 'orders').classList.contains('flash')).toBe(true));
    });

    it('selects a block, and reveals the first query in it on Data', async () => {
      harness = await inspect();
      await harness.frameCall('anfra.run', { id: 'inspect.reveal', target: block });
      expect(harness.mounted.panel).toMatchObject({ tab: 'inspect.data', selection: '0', revealed: { entity: 'query', name: 'revenue' } });
      const { root } = await harness.tab('inspect.data');
      expect(card(root, 'revenue').open).toBe(true);
    });
  });
});
