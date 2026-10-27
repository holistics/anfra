import {
  afterEach, describe, expect, it, vi,
} from 'vitest';
import type { BackendLineageRequest, LineageResult } from '../../common/types';
import { byTestId, inspect, salesSnapshot, type Harness } from '../inspect/tabs/testSupport';
import { lineagePlugin } from '.';

const TAB = 'lineage.lineage';

/** The catalog's answer: `orders.region` reads the `orders` model, over `public.orders`; `sales.revenue` has no diagram. */
function traced (request: BackendLineageRequest): LineageResult {
  return {
    results: request.targets.map((target) => (target.parent === 'orders'
      ? {
        target,
        diagram: {
          cards: [{
            id: 'm', type: 'aml.model', label: 'Orders', props: { fqn: 'orders', name: 'orders', table_name: 'public.orders', data_source_name: 'shop' },
            sections: [{ nodes: [{ id: 'f', type: 'aml.dimension', label: 'Region', props: { fqn: 'orders.region', name: 'region' } }] }],
          }],
        },
      }
      : { target })),
  } as unknown as LineageResult;
}

let harness: Harness | undefined;

afterEach(() => {
  harness?.unmount();
  harness = undefined;
  vi.restoreAllMocks();
});

async function lineage (options: { trace?: (request: BackendLineageRequest) => unknown, select?: string } = {}) {
  const trace = vi.fn(async (request: BackendLineageRequest) => (options.trace ?? traced)(request));
  harness = await inspect({ tab: TAB, plugins: [lineagePlugin({ lineage: trace } as never)] });
  if (options.select) harness.mounted.panel.show(TAB, options.select);
  const tab = await harness.tab(TAB);
  // The trace settles, and the tab draws what it answered.
  await vi.waitFor(() => expect(byTestId(tab.root, 'lineage-canvas')?.hidden).toBe(false));
  await vi.waitFor(() => expect(byTestId(tab.root, 'card-table:shop/public.orders') ?? byTestId(tab.root, 'lineage-failed')).not.toBeNull());
  return { ...tab, trace, harness };
}

const card = (root: ShadowRoot, id: string) => byTestId(root, `card-${id}`);

describe('Lineage', () => {
  it('traces the fields the queries return, and draws the page down to its tables', async () => {
    const { root, trace } = await lineage();
    expect(trace).toHaveBeenCalledTimes(1);
    expect(trace.mock.calls[0][0].targets).toEqual([{ parent: 'orders', child: 'region' }, { parent: 'sales', child: 'revenue' }]);
    for (const id of ['control:0/region', 'block:trend', 'query:0/revenue', 'dimension:orders.region', 'metric:sales.revenue', 'dataset:sales', 'model:orders', 'table:shop/public.orders']) {
      expect(card(root, id), id).not.toBeNull();
    }
    expect(card(root, 'control:0/region')?.querySelector('.lvalue')?.textContent).toContain('is EU');
    expect(card(root, 'model:orders')?.querySelector('.chips')?.textContent).toBe('region');
    expect(root.querySelector('[data-edge="query:0/revenue->metric:sales.revenue"]')).not.toBeNull();
  });

  it("pins a card's path on click, dimming the rest, and lets go on a second click", async () => {
    const { root } = await lineage();
    card(root, 'metric:sales.revenue')!.click();
    expect(card(root, 'metric:sales.revenue')?.classList.contains('selected')).toBe(true);
    // A metric with no model stops at its dataset: the model the dimension reads is off its path.
    expect(card(root, 'dataset:sales')?.classList.contains('on-path')).toBe(true);
    expect(card(root, 'model:orders')?.classList.contains('dim')).toBe(true);
    card(root, 'metric:sales.revenue')!.click();
    expect(card(root, 'model:orders')?.classList.contains('dim')).toBe(false);
  });

  it('outlines a pinned query on the page', async () => {
    const { root, harness } = await lineage();
    card(root, 'query:0/revenue')!.click();
    expect(harness.posted).toContainEqual({ type: 'anfra:event', name: 'anfra.highlight', payload: '0.0' });
  });

  it('hides a kind from the Show menu, and joins across it', async () => {
    const { root } = await lineage();
    byTestId(root, 'lineage-show')!.click();
    byTestId(root, 'show-dataset')!.click();
    expect(card(root, 'dataset:sales')).toBeNull();
    expect(root.querySelector('[data-edge="dimension:orders.region->model:orders"]')).not.toBeNull();
    // Datasets, and the cross-filters the default view leaves out.
    expect(byTestId(root, 'lineage-show')?.textContent).toContain('2 hidden');
    expect(byTestId<HTMLSelectElement>(root, 'lineage-preset')?.value).toBe('custom');
  });

  it('switches to a preset', async () => {
    const { root } = await lineage();
    const preset = byTestId<HTMLSelectElement>(root, 'lineage-preset')!;
    preset.value = 'interaction';
    preset.dispatchEvent(new Event('change'));
    expect(card(root, 'model:orders')).toBeNull();
    expect(card(root, 'control:0/region')).not.toBeNull();
  });

  it("focuses on a card's path, until let go", async () => {
    const { root } = await lineage();
    card(root, 'metric:sales.revenue')!.querySelector<HTMLElement>('[data-testid="card-focus"]')!.click();
    expect(card(root, 'model:orders')).toBeNull();
    expect(byTestId(root, 'lineage-focus')?.textContent).toContain('Metric: revenue');
    byTestId(root, 'lineage-unfocus')!.click();
    expect(card(root, 'model:orders')).not.toBeNull();
  });

  it('locates a card on the page, and shows it in Structure', async () => {
    const { root, harness } = await lineage();
    card(root, 'query:0/revenue')!.querySelector<HTMLElement>('[data-testid="card-locate"]')!.click();
    expect(harness.posted).toContainEqual({ type: 'anfra:event', name: 'anfra.locate', payload: { node: '0.0' } });
    card(root, 'query:0/revenue')!.querySelector<HTMLElement>('[data-testid="card-structure"]')!.click();
    expect(harness.mounted.panel.tab).toBe('inspect.structure');
    expect(harness.mounted.panel.selection).toBe('0.0');
  });

  it("pins the first card in the selected node's subtree", async () => {
    const { root } = await lineage({ select: '0' });
    expect(card(root, 'block:trend')?.classList.contains('selected')).toBe(true);
  });

  it('pins again when the selection changes, and says when nothing it holds is drawn', async () => {
    const { root, harness } = await lineage();
    harness.mounted.panel.select('0.1');
    expect(card(root, 'control:0/region')?.classList.contains('selected')).toBe(true);
    // The second `trend` block holds nothing the canvas draws.
    harness.mounted.panel.select('1');
    expect(byTestId(root, 'lineage-missed')).not.toBeNull();
  });

  it('says when the trace failed, and still draws what the page says', async () => {
    const { root } = await lineage({ trace: () => { throw new Error('no catalog'); } });
    expect(byTestId(root, 'lineage-failed')?.textContent).toContain('no catalog');
    // The dimension still reaches the model its column names, without its table.
    expect(card(root, 'model:orders')).not.toBeNull();
    expect(root.querySelector('[data-testid^="card-table:"]')).toBeNull();
  });

  it('keeps the trace across a switch of tab, and traces again when the fields change', async () => {
    const { trace, unmount, harness } = await lineage();
    unmount();
    const again = await harness.tab(TAB);
    expect(card(again.root, 'table:shop/public.orders')).not.toBeNull();
    expect(trace).toHaveBeenCalledTimes(1);
    const snapshot = salesSnapshot();
    snapshot.apps[0].queries.revenue.columns = snapshot.apps[0].queries.revenue.columns.slice(1);
    harness.post(snapshot);
    await vi.waitFor(() => expect(trace).toHaveBeenCalledTimes(2));
  });

  it('opens a large view of the same canvas, keeping the pin when it closes', async () => {
    const { root } = await lineage();
    card(root, 'query:0/revenue')!.click();
    byTestId(root, 'lineage-expand')!.click();
    const large = byTestId<HTMLDialogElement>(root, 'lineage-expanded')!;
    expect(large.open).toBe(true);
    expect(large.querySelector('[data-testid="card-query:0/revenue"]')?.classList.contains('selected')).toBe(true);
    byTestId(root, 'lineage-close')!.click();
    expect(large.open).toBe(false);
    expect(card(root, 'query:0/revenue')?.classList.contains('selected')).toBe(true);
  });

  it('waits for the frame', async () => {
    harness = await inspect({ plugins: [lineagePlugin({ lineage: vi.fn() } as never)] });
    harness.mounted.panel.hide();
    harness.mounted.panel.show(TAB);
    const { root } = await harness.tab(TAB);
    expect(byTestId(root, 'lineage-waiting')).not.toBeNull();
  });
});
