import {
  afterEach, describe, expect, it, vi,
} from 'vitest';
import {
  byTestId, inspect, REVENUE_AQL, salesSnapshot, type Harness,
} from './testSupport';
import { lineagePlugin } from '../../lineage';

const QUERIES = 'inspect.queries';

let harness: Harness | undefined;

afterEach(() => {
  harness?.unmount();
  harness = undefined;
  vi.restoreAllMocks();
});

/** Click a collapsible's summary, as a reader does, and wait for it to toggle. */
const toggle = async (details: HTMLElement | null) => {
  const toggled = new Promise((resolve) => details!.addEventListener('toggle', resolve, { once: true }));
  details!.querySelector('summary')!.click();
  await toggled;
  return details!;
};

const open = async (details: HTMLElement | null) => ((details as HTMLDetailsElement).open ? details! : toggle(details));

const count = (section: HTMLElement | null) => section?.querySelector('summary .count')?.textContent;

const search = (root: ShadowRoot, text: string) => {
  const input = byTestId<HTMLInputElement>(root, 'queries-search')!;
  input.value = text;
  input.dispatchEvent(new Event('input'));
};

describe('Queries', () => {
  it("draws each app's line, its sections and a row per query and control", async () => {
    harness = await inspect({ tab: QUERIES });
    const { root } = await harness.tab(QUERIES);
    expect(byTestId(root, 'app-line')?.textContent).toBe('Sales · Asia/Singapore · no unapplied changes');
    expect(count(byTestId(root, 'section-queries-0'))).toBe('2');
    expect(count(byTestId(root, 'section-controls-0'))).toBe('2');
    expect(byTestId(root, 'query-0/revenue')?.querySelector('.rows')?.textContent).toBe('2');
    expect(byTestId(root, 'control-0/region')).not.toBeNull();
    expect(byTestId(root, 'section-selection-0')).toBeNull();
    expect(byTestId(root, 'queries-filter-errors')?.textContent).toBe('Errors (1)');
  });

  it("shows a query's error on its line, closed or not", async () => {
    harness = await inspect({ tab: QUERIES });
    const { root } = await harness.tab(QUERIES);
    const row = byTestId<HTMLDetailsElement>(root, 'query-0/orders')!;
    expect(row.open).toBe(false);
    expect(byTestId(row, 'query-error')?.textContent).toBe('QueryError: Unknown field orders.status');
  });

  it('narrows by search and filter, and keeps both across a remount of the tab', async () => {
    harness = await inspect({ tab: QUERIES });
    const first = await harness.tab(QUERIES);
    search(first.root, 'rev');
    expect(byTestId(first.root, 'query-0/orders')).toBeNull();
    expect(count(byTestId(first.root, 'section-queries-0'))).toBe('1 of 2');
    expect(byTestId(first.root, 'section-controls-0')?.querySelector('.note')?.textContent).toBe('No controls match "rev".');
    first.unmount();

    const { root } = await harness.tab(QUERIES);
    expect(byTestId<HTMLInputElement>(root, 'queries-search')?.value).toBe('rev');
    expect(byTestId(root, 'query-0/orders')).toBeNull();
    search(root, '');
    byTestId(root, 'queries-filter-errors')!.click();
    expect(byTestId(root, 'queries-filter-errors')?.getAttribute('aria-pressed')).toBe('true');
    expect(byTestId(root, 'query-0/revenue')).toBeNull();
    expect(byTestId(root, 'query-0/orders')).not.toBeNull();
    expect(byTestId(root, 'section-controls-0')?.querySelector('.note')?.textContent).toBe('Controls have no error state.');
    expect(harness.state.queries.filter).toBe('errors');
  });

  it('opens a row to its general facts and keeps it open across a snapshot', async () => {
    harness = await inspect({ tab: QUERIES });
    const { root } = await harness.tab(QUERIES);
    await open(byTestId(root, 'query-0/revenue'));
    expect(harness.state.queries.open.has('query:0/revenue')).toBe(true);
    const general = byTestId(byTestId(root, 'query-0/revenue')!, 'general')!;
    expect(general.textContent).toContain('sales');
    expect(general.textContent).toContain('In 1 place');
    expect(general.textContent).toContain('Cross-filtersNo query');
    harness.post(salesSnapshot());
    expect(byTestId<HTMLDetailsElement>(root, 'query-0/revenue')?.open).toBe(true);
  });

  it('shows the code a query declared and ran, a tab at a time', async () => {
    harness = await inspect({ tab: QUERIES });
    const { root } = await harness.tab(QUERIES);
    const row = await open(byTestId(root, 'query-0/revenue'));
    const code = await open(byTestId(row, 'code'));
    const tabs = [...code.querySelectorAll<HTMLButtonElement>('.code-tab')];
    expect(tabs.map((t) => t.textContent)).toEqual(['Declared AQL', 'Executed AQL', 'Executed SQL', 'Signature']);
    expect(tabs[1]?.getAttribute('aria-selected')).toBe('true');
    expect(code.querySelector('.code-text')?.textContent).toBe('explore sales { ... }');
    tabs[2]!.click();
    expect(code.querySelector('.code-text')?.textContent).toBe('SELECT region, SUM(amount) FROM orders GROUP BY 1');
    tabs[0]!.click();
    expect(code.querySelector('.code-text')?.textContent).toBe(REVENUE_AQL);
  });

  it("lists a query's filters, active first, with a link to the control", async () => {
    harness = await inspect({ tab: QUERIES });
    const { root } = await harness.tab(QUERIES);
    const row = await open(byTestId(root, 'query-0/revenue'));
    const filters = byTestId(row, 'filters')!;
    // `region` is set; `orders` selects nothing, so its cross-filter is inactive.
    expect(byTestId(filters, 'filters-active')?.textContent).toBe('Active (1)');
    expect(byTestId(filters, 'filter-list')?.children).toHaveLength(1);
    byTestId(filters, 'filters-all')!.click();
    expect(byTestId(byTestId(root, 'query-0/revenue')!, 'filter-list')?.children).toHaveLength(2);
    byTestId(byTestId(root, 'query-0/revenue')!, 'filter-source')!.click();
    expect(byTestId<HTMLDetailsElement>(root, 'control-0/region')?.open).toBe(true);
  });

  it('lists the requests made for a query, opened to their rows', async () => {
    harness = await inspect({ tab: QUERIES });
    const { root } = await harness.tab(QUERIES);
    await harness.frameCall('core.submitQuery', { dataset: 'sales', aql: REVENUE_AQL, input: {} });
    const row = await open(byTestId(root, 'query-0/revenue'));
    expect(byTestId(row, 'general')?.textContent).toMatch(/database · \d+ ms/);
    const requests = await open(byTestId(row, 'requests'));
    expect(requests.querySelector('summary .count')?.textContent).toBe('· 1');
    const request = requests.querySelector<HTMLElement>('[data-testid^="request-"]')!;
    expect(request.querySelector('summary')?.textContent).toContain('database · 2 rows');
    await open(request);
    expect(byTestId(request, 'request-preview')?.querySelectorAll('tbody tr')).toHaveLength(2);
    expect(byTestId(root, 'section-other')).toBeNull();
  });

  it('lists requests no query claims under Other requests', async () => {
    harness = await inspect({ tab: QUERIES });
    const { root } = await harness.tab(QUERIES);
    await harness.frameCall('core.submitQuery', { dataset: 'sales', aql: 'explore sales { measures { revenue } }', input: {} });
    const other = byTestId(root, 'section-other')!;
    expect(count(other)).toBe('1');
    const card = await open(other.querySelector<HTMLElement>('[data-testid^="request-"]'));
    expect(card.querySelector('.name')?.textContent).toBe('sales');
    expect(card.querySelector('dl')?.textContent).toContain('core.submitQuery');
  });

  it('opens and flashes the row a link reveals, clearing what hides it', async () => {
    harness = await inspect({ tab: QUERIES });
    const { root } = await harness.tab(QUERIES);
    search(root, 'rev');
    await toggle(byTestId(root, 'section-queries-0'));
    expect(harness.state.queries.closed.has('0:queries')).toBe(true);
    harness.state.reveal(harness.mounted.panel, { entity: 'query', app: 0, name: 'orders' });
    const row = byTestId<HTMLDetailsElement>(root, 'query-0/orders')!;
    expect(row.open).toBe(true);
    expect(row.classList.contains('flash')).toBe(true);
    expect(byTestId<HTMLInputElement>(root, 'queries-search')?.value).toBe('');
  });

  it('waits for the frame', async () => {
    harness = await inspect();
    harness.mounted.panel.hide();
    harness.mounted.panel.show(QUERIES);
    const { root } = await harness.tab(QUERIES);
    expect(byTestId(root, 'inspect-waiting')).not.toBeNull();
  });

  it('shows a query in Lineage through its marker, when lineage is installed', async () => {
    harness = await inspect({ tab: QUERIES, plugins: [lineagePlugin({ lineage: vi.fn(async () => ({ results: [] })) } as never)] });
    const { root } = await harness.tab(QUERIES);
    await open(byTestId(root, 'query-0/revenue'));
    byTestId(byTestId(root, 'query-0/revenue')!, 'show-in-lineage')!.click();
    expect(harness.mounted.panel.tab).toBe('lineage.lineage');
    expect(harness.mounted.panel.selection).toBe('0.0');
    // `orders` is drawn nowhere on the page.
    await open(byTestId(root, 'query-0/orders'));
    expect(byTestId(byTestId(root, 'query-0/orders')!, 'show-in-lineage')).toBeNull();
  });

  it('offers no Lineage link without lineage', async () => {
    harness = await inspect({ tab: QUERIES });
    const { root } = await harness.tab(QUERIES);
    await open(byTestId(root, 'query-0/revenue'));
    expect(byTestId(root, 'show-in-lineage')).toBeNull();
  });
});
