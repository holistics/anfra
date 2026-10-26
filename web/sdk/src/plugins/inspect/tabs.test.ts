import {
  afterEach, describe, expect, it, vi,
} from 'vitest';
import type { BridgeMessage } from '../../common/bridge';
import { salesDataset, testUser } from '../../app/testSupport';
import { createHost, type MountedDataApp } from '../../host/runtime';
import { corePlugin } from '../core';
import { inspectPlugin } from '.';

let mounted: MountedDataApp | undefined;
let posted: BridgeMessage[] = [];

afterEach(() => {
  mounted?.unmount();
  mounted = undefined;
  posted = [];
  vi.restoreAllMocks();
});

const snapshot = {
  apps: [{
    title: 'Sales',
    hasChanges: false,
    declaration: {},
    queries: { revenue: { state: 'success', isDirty: false, rowCount: 12, signature: 'sig' } },
    controls: {},
  }],
  structure: {
    nodes: [{
      key: '0', kind: 'block', id: 'trend', label: 'Trend', problems: [], children: [
        { key: '0.0', kind: 'query', name: 'revenue', app: 0, problems: [], children: [] },
      ],
    }],
    usage: { 'query:0/revenue': { markers: 1, blocks: ['trend'] } },
  },
};

async function open () {
  const host = createHost({ plugins: [corePlugin({ submitQuery: vi.fn(), fieldSuggestions: vi.fn() }), inspectPlugin()], frameScript: 'x' });
  mounted = host.mount({
    container: document.body, definition: '', datasets: { sales: salesDataset }, user: testUser,
  });
  vi.spyOn(mounted.frame.contentWindow!, 'postMessage').mockImplementation((message: unknown) => { posted.push(message as BridgeMessage); });
  await mounted.run('inspect.toggle');
  window.dispatchEvent(new MessageEvent('message', {
    data: { type: 'anfra:event', name: 'anfra.snapshot', payload: snapshot }, source: mounted.frame.contentWindow,
  }));
  return { host, panel: mounted.panel };
}

async function tab (id: string) {
  const element = document.createElement('div');
  document.body.append(element);
  const unmount = mounted!.panel.mountTab(id, element);
  // The tabs' code loads on first use.
  await vi.waitFor(() => expect(element.shadowRoot?.querySelector('.pane')).not.toBeNull());
  return { root: element.shadowRoot!, unmount };
}

describe("inspect's tabs", () => {
  it('opens the panel at Structure from its command, and lists both tabs in order', async () => {
    const { host, panel } = await open();
    expect(host.tabs.map((t) => t.id)).toEqual(['inspect.structure', 'inspect.data']);
    expect(host.commands[0]).toMatchObject({ id: 'inspect.toggle', shortcut: 'Alt+Shift+I' });
    expect(panel).toMatchObject({ open: true, tab: 'inspect.structure' });
  });

  it('draws the structure, and selects and locates a node on click', async () => {
    const { panel } = await open();
    const { root } = await tab('inspect.structure');
    expect(root.querySelector('[data-testid="node-0"] .name')?.textContent).toBe('trend');
    root.querySelector<HTMLElement>('[data-testid="node-0.0"]')!.click();
    expect(panel.selection).toBe('0.0');
    expect(posted).toContainEqual({ type: 'anfra:event', name: 'anfra.locate', payload: { node: '0.0' } });
    expect(root.querySelector('[data-testid="node-detail"] h5')?.textContent).toBe('query');
    expect(root.querySelector('[data-testid="inspect-query-revenue"] [data-testid="query-usage"]')?.textContent).toContain('In 1 place: trend');
  });

  it('turns pick mode on from its toolbar', async () => {
    const { panel } = await open();
    const { root } = await tab('inspect.structure');
    root.querySelector<HTMLElement>('[data-testid="pick"]')!.click();
    expect(panel.picking).toBe(true);
    expect(root.querySelector('[data-testid="pick"]')?.getAttribute('aria-pressed')).toBe('true');
  });

  it('opens Data on a query\'s node, which shows every app\'s queries', async () => {
    const { panel } = await open();
    const structure = await tab('inspect.structure');
    structure.root.querySelector<HTMLElement>('[data-testid="node-0.0"] [data-testid="view-data"]')!.click();
    expect(panel).toMatchObject({ tab: 'inspect.data', selection: '0.0' });
    const { root } = await tab('inspect.data');
    expect(root.querySelector('[data-testid="inspect-query-revenue"] [data-testid="query-state"]')?.textContent).toBe('success');
    expect(root.querySelector('[data-testid="inspect-query-revenue"]')?.classList.contains('flash')).toBe(true);
  });

  it('waits for the frame, and stops drawing once taken down', async () => {
    const { panel } = await open();
    panel.hide();
    panel.show();
    const { root, unmount } = await tab('inspect.data');
    expect(root.querySelector('[data-testid="inspect-waiting"]')).not.toBeNull();
    unmount();
    expect(root.childNodes).toHaveLength(0);
  });
});
