import {
  afterEach, describe, expect, it, vi,
} from 'vitest';
import { testUser } from '../../../app/testSupport';
import { DATA_TAB } from '../model/types';
import { toCsv } from './data';
import {
  byTestId, inspect, REVENUE_AQL, salesApp, salesSnapshot, type Harness,
} from './testSupport';

let harness: Harness | undefined;

afterEach(() => {
  harness?.unmount();
  harness = undefined;
  vi.restoreAllMocks();
});

const REVENUE = { dataset: 'sales', aql: REVENUE_AQL, input: {}, page: 1 };

const cardOf = (root: ShadowRoot, name: string, app = 0) => byTestId(root, `table-${app}/${name}`)?.querySelector<HTMLDetailsElement>('details') ?? null;

/** Open a card as the reader does, and wait for its body. */
async function open (card: HTMLDetailsElement | null): Promise<HTMLDetailsElement> {
  card!.querySelector('summary')!.click();
  await vi.waitFor(() => expect(card!.querySelector('.collapsible-body')).not.toBeNull());
  return card!;
}

describe('Data', () => {
  it('draws one closed card per query, in declaration order', async () => {
    harness = await inspect({ tab: DATA_TAB });
    const { root } = await harness.tab(DATA_TAB);
    const names = [...root.querySelectorAll('details .name')].map((element) => element.textContent);
    expect(names).toEqual(['revenue', 'orders']);
    expect(cardOf(root, 'revenue')!.open).toBe(false);
    expect(cardOf(root, 'orders')!.querySelector('.pill.error')?.textContent).toBe("couldn't load");
  });

  it("shows the rows of the query's latest request, with the labels as headers", async () => {
    harness = await inspect({ tab: DATA_TAB });
    await harness.frameCall('core.submitQuery', REVENUE);
    const { root } = await harness.tab(DATA_TAB);
    const card = await open(cardOf(root, 'revenue'));
    const rows = byTestId(card, 'table-rows')!;
    expect([...rows.querySelectorAll('th')].map((th) => th.textContent)).toEqual(['region', 'revenue']);
    expect(byTestId(card, 'column-revenue')?.dataset.tip).toContain('Metric · sum');
    expect([...rows.querySelectorAll('tbody tr')].map((tr) => [...tr.querySelectorAll('td')].map((td) => td.textContent))).toEqual([['EU', '1200.5'], ['US', '980']]);
    expect(byTestId(card, 'table-count')?.textContent).toContain('2 rows');
    expect(byTestId(card, 'download-csv')).not.toBeNull();
  });

  it('says why a table has no rows', async () => {
    const executing = salesApp();
    executing.queries.revenue = { ...executing.queries.revenue!, state: 'executing' };
    harness = await inspect({ tab: DATA_TAB, snapshot: salesSnapshot([executing]) });
    const { root } = await harness.tab(DATA_TAB);
    expect(byTestId(await open(cardOf(root, 'revenue')), 'table-loading')?.textContent).toBe('Loading this table…');
    expect(byTestId(await open(cardOf(root, 'orders')), 'table-error')?.textContent).toBe("This table couldn't load.");

    // Succeeded, but its request isn't in the log.
    harness.post(salesSnapshot());
    expect(byTestId(cardOf(root, 'revenue')!, 'table-gone')?.textContent).toBe("This table's rows aren't available here.");
  });

  it('says when the result is empty', async () => {
    harness = await inspect({ tab: DATA_TAB, submitQuery: () => ({ columns: [], values: [], meta: { numRows: 0 } }) });
    await harness.frameCall('core.submitQuery', REVENUE);
    const { root } = await harness.tab(DATA_TAB);
    const card = await open(cardOf(root, 'revenue'));
    expect(byTestId(card, 'table-rows')).toBeNull();
    expect(byTestId(card, 'table-count')?.textContent).toContain('No rows.');
  });

  it("doesn't show a request another query may have made", async () => {
    const app = salesApp();
    app.declaration.queries.copy = { dataset: 'sales', aql: REVENUE_AQL };
    app.queries.copy = { ...app.queries.revenue! };
    harness = await inspect({ tab: DATA_TAB, snapshot: salesSnapshot([app]) });
    await harness.frameCall('core.submitQuery', REVENUE);
    const { root } = await harness.tab(DATA_TAB);
    expect(byTestId(await open(cardOf(root, 'revenue')), 'table-gone')).not.toBeNull();
  });

  it('skips a later page', async () => {
    harness = await inspect({ tab: DATA_TAB });
    await harness.frameCall('core.submitQuery', REVENUE);
    await harness.frameCall('core.submitQuery', { ...REVENUE, page: 2 });
    const { root } = await harness.tab(DATA_TAB);
    expect(byTestId(await open(cardOf(root, 'revenue')), 'table-rows')).not.toBeNull();
  });

  it('narrows the tables by search, and Escape clears it', async () => {
    harness = await inspect({ tab: DATA_TAB });
    const { root } = await harness.tab(DATA_TAB);
    const search = byTestId<HTMLInputElement>(root, 'data-search')!;
    search.value = 'rev';
    search.dispatchEvent(new Event('input'));
    expect(cardOf(root, 'revenue')).not.toBeNull();
    expect(cardOf(root, 'orders')).toBeNull();
    search.value = 'zzz';
    search.dispatchEvent(new Event('input'));
    expect(byTestId(root, 'data-no-match')?.textContent).toBe('No tables match "zzz".');
    // The box is kept across redraws, so it keeps the focus.
    expect(byTestId(root, 'data-search')).toBe(search);
    search.dispatchEvent(new KeyboardEvent('keydown', { key: 'Escape' }));
    expect(search.value).toBe('');
    expect(cardOf(root, 'orders')).not.toBeNull();
  });

  it('keeps the open cards across a switch of tab', async () => {
    harness = await inspect({ tab: DATA_TAB });
    await harness.frameCall('core.submitQuery', REVENUE);
    const first = await harness.tab(DATA_TAB);
    await open(cardOf(first.root, 'revenue'));
    expect(harness.state.data.open.has('0/revenue')).toBe(true);
    first.unmount();
    const { root } = await harness.tab(DATA_TAB);
    expect(cardOf(root, 'revenue')!.open).toBe(true);
    expect(byTestId(cardOf(root, 'revenue')!, 'table-rows')).not.toBeNull();
    expect(cardOf(root, 'orders')!.open).toBe(false);
  });

  it('opens and flashes a table a link reveals, once', async () => {
    harness = await inspect({ tab: DATA_TAB });
    harness.state.reveal(harness.mounted.panel, { table: 'orders', app: 0 });
    const first = await harness.tab(DATA_TAB);
    expect(cardOf(first.root, 'orders')!.open).toBe(true);
    await vi.waitFor(() => expect(cardOf(first.root, 'orders')!.classList.contains('flash')).toBe(true));
    first.unmount();
    const { root } = await harness.tab(DATA_TAB);
    await Promise.resolve();
    expect(cardOf(root, 'orders')!.classList.contains('flash')).toBe(false);
    // A link followed while the tab is up.
    harness.state.reveal(harness.mounted.panel, { table: 'revenue', app: 0 });
    expect(cardOf(root, 'revenue')!.open).toBe(true);
    await vi.waitFor(() => expect(cardOf(root, 'revenue')!.classList.contains('flash')).toBe(true));
    expect(cardOf(root, 'orders')!.classList.contains('flash')).toBe(false);
  });

  it("opens the query's row in Queries", async () => {
    harness = await inspect({ tab: DATA_TAB });
    const { root } = await harness.tab(DATA_TAB);
    byTestId(cardOf(root, 'revenue')!, 'show-query')!.click();
    expect(harness.mounted.panel.tab).toBe('inspect.queries');
    expect(harness.state.queries.open.has('query:0/revenue')).toBe(true);
  });

  it('offers no download when the reader may not export', async () => {
    harness = await inspect({ tab: DATA_TAB, user: { ...testUser, permissions: { ...testUser.permissions, canExportData: false } } });
    await harness.frameCall('core.submitQuery', REVENUE);
    const { root } = await harness.tab(DATA_TAB);
    const card = await open(cardOf(root, 'revenue'));
    expect(byTestId(card, 'table-rows')).not.toBeNull();
    expect(byTestId(card, 'download-csv')).toBeNull();
  });

  it('offers no download for part of a table', async () => {
    harness = await inspect({ tab: DATA_TAB, submitQuery: () => ({ columns: [], values: [['EU', 1]], meta: { numRows: 40 } }) });
    await harness.frameCall('core.submitQuery', REVENUE);
    const { root } = await harness.tab(DATA_TAB);
    const card = await open(cardOf(root, 'revenue'));
    expect(byTestId(card, 'table-count')?.textContent).toContain('First 1 of 40 rows');
    expect(byTestId(card, 'download-csv')).toBeNull();
  });

  it('groups the tables by app when there are several', async () => {
    harness = await inspect({ tab: DATA_TAB, snapshot: salesSnapshot([salesApp(), salesApp({ title: 'Other' })]) });
    const { root } = await harness.tab(DATA_TAB);
    expect([...root.querySelectorAll('.app-title')].map((element) => element.textContent)).toEqual(['App 1 · Sales', 'App 2 · Other']);
    expect(cardOf(root, 'revenue', 1)).not.toBeNull();
  });
});

describe('toCsv', () => {
  it('writes a BOM, CRLF lines and the labels as headers', () => {
    expect(toCsv(['region', 'revenue'], [['EU', 1200.5], ['US', null]], { region: 'Region' }))
      .toBe('﻿Region,revenue\r\nEU,1200.5\r\nUS,');
  });

  it('quotes what must be, and guards text a spreadsheet would run', () => {
    expect(toCsv(['a'], [['x,y'], ['say "hi"'], ['line\nbreak'], ['=SUM(A1)'], ['+1'], ['-1'], ['@x'], ['\tx'], ['\rx'], [-1], [{ k: 1 }]], {}))
      .toBe(['﻿a', '"x,y"', '"say ""hi"""', '"line\nbreak"', "'=SUM(A1)", "'+1", "'-1", "'@x", "'\tx", '"\'\rx"', '-1', '"{""k"":1}"'].join('\r\n'));
  });
});
