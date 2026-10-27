import {
  afterEach, describe, expect, it, vi,
} from 'vitest';
import { byTestId, inspect, type Harness } from './testSupport';

let harness: Harness | undefined;

afterEach(() => {
  harness?.unmount();
  harness = undefined;
  vi.restoreAllMocks();
});

describe('Structure', () => {
  it('draws the tree, and selects and locates a node on click', async () => {
    harness = await inspect({ tab: 'inspect.structure' });
    const { root } = await harness.tab('inspect.structure');
    expect(byTestId(root, 'node-0')?.querySelector('.name')?.textContent).toBe('trend');
    byTestId(root, 'node-0.0')!.click();
    expect(harness.mounted.panel.selection).toBe('0.0');
    expect(harness.posted).toContainEqual({ type: 'anfra:event', name: 'anfra.locate', payload: { node: '0.0' } });
    expect(byTestId(root, 'node-detail')?.querySelector('h5')?.textContent).toBe('query');
  });

  it('collapses a node, and opens it again when something under it is selected', async () => {
    harness = await inspect({ tab: 'inspect.structure' });
    const { root } = await harness.tab('inspect.structure');
    byTestId(root, 'node-0')!.querySelector<HTMLElement>('.twist')!.click();
    expect(byTestId(root, 'node-0.0')).toBeNull();
    harness.mounted.panel.select('0.1');
    expect(byTestId(root, 'node-0.1')?.getAttribute('aria-selected')).toBe('true');
  });

  it('turns pick mode on from its toolbar', async () => {
    harness = await inspect({ tab: 'inspect.structure' });
    const { root } = await harness.tab('inspect.structure');
    byTestId(root, 'pick')!.click();
    expect(harness.mounted.panel.picking).toBe(true);
    expect(byTestId(root, 'pick')?.getAttribute('aria-pressed')).toBe('true');
  });

  it('shows a query in Queries', async () => {
    harness = await inspect({ tab: 'inspect.structure' });
    const { root } = await harness.tab('inspect.structure');
    byTestId(root, 'node-0.0')!.querySelector<HTMLElement>('[data-testid="show-in-queries"]')!.click();
    expect(harness.mounted.panel.tab).toBe('inspect.queries');
  });

  it('waits for the frame, and stops drawing once taken down', async () => {
    harness = await inspect();
    harness.mounted.panel.hide();
    harness.mounted.panel.show('inspect.structure');
    const { root, unmount } = await harness.tab('inspect.structure');
    expect(byTestId(root, 'inspect-waiting')).not.toBeNull();
    unmount();
    expect(root.childNodes).toHaveLength(0);
  });
});
