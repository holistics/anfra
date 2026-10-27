import {
  afterEach, describe, expect, it, vi,
} from 'vitest';
import {
  byTestId, inspect, salesApp, salesSnapshot, type Harness,
} from './testSupport';

let harness: Harness | undefined;

afterEach(() => {
  harness?.unmount();
  harness = undefined;
  vi.restoreAllMocks();
});

const TAB = 'inspect.declaration';

/** Open the wiring matrix, as a click on its summary does. */
async function openWiring (root: ShadowRoot, app = 0): Promise<HTMLElement> {
  const details = root.querySelector<HTMLDetailsElement>(`details[data-key="wiring:${app}"]`)!;
  details.open = true;
  details.dispatchEvent(new Event('toggle'));
  return vi.waitFor(() => {
    const table = byTestId(root, `wiring-${app}`);
    if (!table) throw new Error('not open yet');
    return table;
  });
}

describe('Declaration', () => {
  it('counts the queries, controls, mappings and cross-filters', async () => {
    harness = await inspect({ tab: TAB });
    const { root } = await harness.tab(TAB);
    expect([...byTestId(root, 'counts')!.children].map((pill) => pill.textContent))
      .toEqual(['2 queries', '2 controls', '1 mapping', '1 cross-filter']);
  });

  it('lists the checks in order, each opening its entity in Queries', async () => {
    harness = await inspect({ tab: TAB });
    const { root } = await harness.tab(TAB);
    const rows = [...byTestId(root, 'checks')!.querySelectorAll('button.check')];
    expect(rows.map((row) => row.textContent)).toEqual([
      '⚠month is mapped to no query, so it has no effect.',
      'ℹorders has no controls or cross-filters, so it never changes.',
    ]);
    expect(rows[0]?.querySelector('.mark')?.getAttribute('aria-label')).toBe('warning');
    (rows[0] as HTMLElement).click();
    expect(harness.mounted.panel.tab).toBe('inspect.queries');
    expect(harness.state.queries.open.has('control:0/month')).toBe(true);
  });

  it('sets apart the entities a check names, and never a name inside a field', async () => {
    const app = salesApp();
    app.declaration.mappings.push({ kind: 'control', id: 'm2', from: { kind: 'dateDrill', name: 'month' }, to: { kind: 'query', name: 'revenue' }, field: 'orders.created_at' });
    harness = await inspect({ tab: TAB, snapshot: salesSnapshot([app]) });
    const { root } = await harness.tab(TAB);
    const row = [...byTestId(root, 'checks')!.querySelectorAll('button.check')].find((check) => check.textContent!.includes('grain'))!;
    expect(row.textContent).toBe("⚠month → revenue: orders.created_at isn't in the query's AQL, so the grain never changes.");
    expect([...row.querySelectorAll('code.entity')].map((name) => name.textContent)).toEqual(['month', 'revenue']);
  });

  it('says so when there is nothing to check', async () => {
    const app = salesApp();
    app.declaration = { ...app.declaration, dateDrills: {}, mappings: [...app.declaration.mappings, { kind: 'crossFilter', id: 'c2', from: { kind: 'query', name: 'revenue' }, to: { kind: 'query', name: 'orders' } }] };
    harness = await inspect({ tab: TAB, snapshot: salesSnapshot([app]) });
    const { root } = await harness.tab(TAB);
    expect(byTestId(root, 'checks-none')?.textContent).toBe('No issues found.');
  });

  it('shows the declaration as its JSON, the text the same as JSON.stringify writes it', async () => {
    const app = salesApp();
    app.declaration.queries.leak = { dataset: 'sales', aql: 'explore sales {\n\tmeasures {\n\t  cancelled: revenue | where(orders.status == "cancelled")\n\t}\n}' };
    harness = await inspect({ tab: TAB, snapshot: salesSnapshot([app]) });
    const { root } = await harness.tab(TAB);
    const block = byTestId(root, 'declaration-0')!;
    const declaration = harness.mounted.panel.snapshot!.apps[0]!.declaration;
    expect(block.querySelector('pre')!.textContent).toBe(JSON.stringify(declaration, null, 2));
    expect(JSON.parse(block.querySelector('pre')!.textContent!)).toEqual(JSON.parse(JSON.stringify(declaration)));
    expect(block.querySelector('.tok-key')?.textContent).toBe('"title"');
    expect(block.querySelector('.row-action')?.getAttribute('aria-label')).toBe('Copy the declaration');
  });

  it("shows a query's AQL across its lines, highlighted, under its key and without the indent it shares", async () => {
    // As a template literal in a definition leaves it: a newline first, every line indented.
    const app = salesApp({ declaration: { ...salesApp().declaration, queries: { revenue: { dataset: 'sales', aql: '\n      explore sales {\n        measures { revenue }\n      }\n    ' } } } });
    harness = await inspect({ tab: TAB, snapshot: salesSnapshot([app]) });
    const { root } = await harness.tab(TAB);
    const aql = [...byTestId(root, 'declaration-0')!.querySelectorAll('.tok-str')].find((string) => string.querySelector('.json-br'))!;
    expect(aql.querySelectorAll('.json-br')).toHaveLength(4);
    expect(aql.querySelector('.json-br')?.getAttribute('data-indent')).toBe('        ');
    expect([...aql.querySelectorAll('.json-dedent')].map((indent) => indent.textContent)).toEqual(['      ', '      ', '      ', '    ']);
    expect(aql.querySelector('.tok-kw')?.textContent).toBe('explore');
    expect(aql.textContent).toBe(JSON.stringify(app.declaration.queries.revenue!.aql));
  });

  it('says so when nothing is declared, and leaves the wiring out with no queries', async () => {
    const app = salesApp();
    app.declaration = { title: 'Sales', queries: {}, filters: {}, dateDrills: {}, mappings: [] };
    harness = await inspect({ tab: TAB, snapshot: salesSnapshot([app]) });
    const { root } = await harness.tab(TAB);
    expect(byTestId(root, 'declaration-0')?.textContent).toBe('Nothing declared.');
    expect(root.querySelector('details[data-key="wiring:0"]')).toBeNull();
  });

  it('draws the wiring, collapsed and last, queries down and controls across', async () => {
    harness = await inspect({ tab: TAB });
    const { root } = await harness.tab(TAB);
    const group = byTestId(root, 'app-0')!;
    const details = group.lastElementChild as HTMLDetailsElement;
    expect(details.dataset.key).toBe('wiring:0');
    expect(details.open).toBe(false);
    const table = await openWiring(root);
    expect([...table.querySelectorAll('th')].map((th) => th.textContent)).toEqual(['query', 'region', 'month', 'cross-filtered by']);
    const mapped = byTestId(root, 'wiring-cell-revenue-region')!;
    expect(mapped.textContent).toBe('●');
    expect(mapped.querySelector('span')?.getAttribute('data-tip')).toBe('orders.region');
    expect(byTestId(root, 'wiring-cell-orders-region')?.textContent).toBe('·');
    expect(byTestId(root, 'wiring-by-revenue')?.textContent).toBe('orders');
    expect(byTestId(root, 'wiring-by-orders')?.textContent).toBe('—');
    byTestId(root, 'wiring-query-orders')!.click();
    expect(harness.mounted.panel.tab).toBe('inspect.queries');
    expect(harness.state.queries.open.has('query:0/orders')).toBe(true);
  });

  it('groups by app when there are several', async () => {
    harness = await inspect({ tab: TAB, snapshot: salesSnapshot([salesApp(), salesApp({ title: 'Ops' })]) });
    const { root } = await harness.tab(TAB);
    expect(byTestId(root, 'app-0')?.querySelector('.app-title')?.textContent).toBe('App 1 · Sales');
    expect(byTestId(root, 'app-1')?.querySelector('.app-title')?.textContent).toBe('App 2 · Ops');
    byTestId(root, 'check-1-0')!.click();
    expect(harness.state.queries.open.has('control:1/month')).toBe(true);
  });

  it('names no app with only one', async () => {
    harness = await inspect({ tab: TAB });
    const { root } = await harness.tab(TAB);
    expect(root.querySelector('.app-title')).toBeNull();
  });

  it('flashes a check linked to from elsewhere, once', async () => {
    harness = await inspect({ tab: 'inspect.overview' });
    harness.state.reveal(harness.mounted.panel, { check: 1, app: 0 });
    expect(harness.mounted.panel.tab).toBe(TAB);
    const { root, unmount } = await harness.tab(TAB);
    const row = root.querySelector<HTMLElement>('[data-check="0:1"]')!;
    expect(row.classList.contains('flash')).toBe(true);
    unmount();
    const again = await harness.tab(TAB);
    expect(again.root.querySelector('[data-check="0:1"]')?.classList.contains('flash')).toBe(false);
    harness.state.reveal(harness.mounted.panel, { check: 0, app: 0 });
    expect(again.root.querySelector('[data-check="0:0"]')?.classList.contains('flash')).toBe(true);
  });

  it('flashes a linked check once the frame has drawn it', async () => {
    harness = await inspect({ tab: TAB, snapshot: salesSnapshot([]) });
    const { root } = await harness.tab(TAB);
    expect(byTestId(root, 'declaration-no-apps')).not.toBeNull();
    harness.state.reveal(harness.mounted.panel, { check: 0, app: 0 });
    harness.post(salesSnapshot());
    expect(root.querySelector('[data-check="0:0"]')?.classList.contains('flash')).toBe(true);
  });

  it('waits for the frame', async () => {
    harness = await inspect();
    harness.mounted.panel.hide();
    harness.mounted.panel.show(TAB);
    const { root } = await harness.tab(TAB);
    expect(byTestId(root, 'inspect-waiting')).not.toBeNull();
  });
});
