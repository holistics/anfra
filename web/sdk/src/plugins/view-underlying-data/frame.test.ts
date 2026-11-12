import {
  afterEach, describe, expect, it, vi,
} from 'vitest';
import { DataAppError } from '../../common/errors';
import { flush, testFrame } from '../../app/testFrame';
import { numberTypes, textTypes } from '../../app/testSupport';
import ui from '../../app/ui/body';
import type { BackendQueryRequest, BackendQueryResult, ColumnMeta } from '../../common/types';
import setupVud from './frame';
import body from './body';
import { PAGE_SIZE } from './model';

afterEach(() => {
  vi.restoreAllMocks();
  document.body.innerHTML = '';
  document.documentElement.style.overflow = '';
});

const region: ColumnMeta = {
  name: 'region', modelId: 'users', fieldName: 'region', label: 'Region', adhoc: false, isMeasure: false, ...textTypes,
};
const total: ColumnMeta = {
  name: 'total', modelId: 'orders', fieldName: 'amount', label: 'total', adhoc: false, isMeasure: true, aggregation: 'sum', ...numberTypes,
};
const count: ColumnMeta = {
  name: 'n', modelId: 'orders', fieldName: 'id', label: 'n', adhoc: false, isMeasure: true, aggregation: 'count', ...numberTypes,
};

const aql = 'explore { dimensions { region: users.region } measures { total: sum(orders.amount), n: count(orders.id) } }';

/** The rows behind the point, one page of `size` of them. */
function page (size: number, hasMore: boolean): BackendQueryResult {
  const columns: ColumnMeta[] = [
    { name: 'id', modelId: 'orders', fieldName: 'id', label: 'Id', adhoc: false, isMeasure: false, ...numberTypes },
    { name: 'amount', modelId: 'orders', fieldName: 'amount', label: 'Amount', adhoc: false, isMeasure: false, ...numberTypes },
    { name: 'status', modelId: 'orders', fieldName: 'status', label: 'Status', adhoc: false, isMeasure: false, ...textTypes },
  ];
  return {
    columns,
    values: Array.from({ length: size }, (_, i) => [String(i + 1), String(1000 + i), i === 0 ? null : 'paid']),
    meta: { hasMore },
  };
}

type PageAnswer = BackendQueryResult | Error | ((request: BackendQueryRequest, signal: AbortSignal) => Promise<BackendQueryResult>);

function setup ({ plugins = ['core', 'view-underlying-data', 'export'], pages = [page(2, true), page(1, false)] }: { plugins?: string[], pages?: PageAnswer[] } = {}) {
  document.body.innerHTML = '<section data-anfra-block="chart"><span id="bar">bar</span></section>';
  const underlying: BackendQueryRequest[] = [];
  const exports: unknown[] = [];
  const frame = testFrame({
    setups: [['view-underlying-data', setupVud]],
    bodies: { anfra: ui, 'view-underlying-data': body },
    plugins: plugins.map((name) => ({ name, version: 'test' })),
    answers: {
      'core.submitQuery': (request: BackendQueryRequest, signal: AbortSignal) => {
        if (request.aql === aql) return { columns: [region, total, count], values: [['APAC', '1500', '3'], ['EMEA', null, '0']] };
        underlying.push(request);
        const next = pages.shift();
        if (next instanceof Error) throw next;
        if (typeof next === 'function') return next(request, signal);
        return next ?? page(0, false);
      },
      'export.query': (request: unknown) => {
        exports.push(request);
        return { url: 'http://host.test/x.csv', filename: 'x.csv', format: 'csv', rowCount: 3, expiresAt: new Date().toISOString() };
      },
    },
  });
  const app = frame.anfra.createApp({ title: 'Sales', timezone: 'Asia/Singapore' });
  const query = app.createQuery('revenue', { dataset: 'sales', aql });
  return {
    frame, app, query, underlying, exports,
  };
}

const settle = async () => { for (let i = 0; i < 4; i += 1) await flush(); };
const uiRoot = () => document.querySelector('[data-anfra-layer] [data-part="anfra-ui"]')?.shadowRoot;
const entries = () => [...(uiRoot()?.querySelectorAll<HTMLButtonElement>('[role="menuitem"]') ?? [])];
const content = () => uiRoot()?.querySelector('[role="dialog"] .content')?.shadowRoot;

/** Right-click the bar, with the definition reporting this row of the query. */
function rightClick (query: ReturnType<typeof setup>['query'], row: Record<string, unknown>, measure?: string) {
  const bar = document.getElementById('bar')!;
  bar.addEventListener('contextmenu', (event) => { query.menu(event, row, measure ? { measure } : {}); }, { once: true });
  const event = new MouseEvent('contextmenu', { bubbles: true, cancelable: true });
  bar.dispatchEvent(event);
  return event;
}

describe('the View underlying data entry', () => {
  it('offers one entry per measure when the definition names none, and only the named one when it does', async () => {
    const { app, query } = setup();
    await app.execute();
    rightClick(query, { region: 'APAC', total: '1500', n: '3' });
    await settle();
    expect(entries().map((entry) => entry.textContent)).toEqual(['View underlying data: total', 'View underlying data: n']);

    rightClick(query, { region: 'APAC', total: '1500', n: '3' }, 'n');
    await settle();
    expect(entries().map((entry) => entry.textContent)).toEqual(['View underlying data']);
  });

  it('can\'t be picked on a point with no value, and says why', async () => {
    const { app, query } = setup();
    await app.execute();
    rightClick(query, { region: 'EMEA', total: null, n: '0' }, 'total');
    await settle();
    expect(entries()[0].getAttribute('aria-disabled')).toBe('true');
    expect(entries()[0].textContent).toContain('No data is available for this point');
  });

  it('is left out, keeping the browser\'s menu, when no measure names a model', async () => {
    const { frame, query } = setup();
    // Never run: no result says what its columns are.
    expect(rightClick(query, { region: 'APAC' }).defaultPrevented).toBe(false);
    expect(frame.calls.map((call) => call.name)).not.toContain('anfra.body');
  });
});

describe('the modal', () => {
  it('lists the rows behind the point a page at a time, through core.submitQuery', async () => {
    const { app, query, underlying } = setup();
    await app.execute();
    rightClick(query, { region: 'APAC', total: '1500', n: '3' }, 'total');
    await settle();
    entries()[0].click();
    await settle();

    expect(uiRoot()!.querySelector('[role="dialog"] h2')!.textContent).toBe(`View underlying data: ${(1500).toLocaleString()} total`);
    const root = content()!;
    expect(root.querySelector('[data-testid="vud-conditions"]')!.textContent).toBe('Region is APAC');
    expect([...root.querySelectorAll('th')].map((th) => th.textContent)).toEqual(['Id', 'Amount', 'Status', 'Created At']);
    expect(underlying[0]).toEqual({
      dataset: 'sales',
      aql: expect.stringContaining('amount: orders.amount'),
      input: {
        filters: [{ field: 'users.region', operator: 'is', values: ['APAC'] }], conditions: [], sorts: expect.arrayContaining([{ field: 'created_at', direction: 'desc' }]), dateDrills: [],
        timezone: 'Asia/Singapore',
        pagination: { page: 1, pageSize: PAGE_SIZE },
      },
    });
    const rows = () => [...root.querySelectorAll('tbody tr')].map((tr) => [...tr.children].map((td) => td.textContent));
    expect(rows()).toEqual([
      ['1', (1000).toLocaleString(), '', ''],
      ['2', (1001).toLocaleString(), 'paid', ''],
    ]);
    expect(root.querySelector('[data-testid="vud-count"]')!.textContent).toBe('Showing the first 2 rows');

    const more = root.querySelector<HTMLButtonElement>('[data-testid="vud-more"]')!;
    expect(more.hidden).toBe(false);
    more.click();
    await settle();
    expect(underlying[1].input.pagination?.page).toBe(2);
    expect(root.querySelectorAll('tbody tr')).toHaveLength(3);
    expect(root.querySelector('[data-testid="vud-count"]')!.textContent).toBe('All 3 rows');
    expect(more.hidden).toBe(true);
  });

  it('stamps its pages and its download with the app, the query and its variant, for the host\'s request log', async () => {
    const { app, query, frame } = setup();
    await app.execute();
    rightClick(query, { region: 'APAC', total: '1500', n: '3' }, 'total');
    await settle();
    entries()[0].click();
    await settle();
    content()!.querySelector<HTMLButtonElement>('[data-testid="vud-download"]')!.click();
    await settle();

    const stamped = frame.calls.filter((call) => call.name !== 'anfra.body').map((call) => [call.name, call.stamp]);
    expect(stamped).toEqual([
      ['core.submitQuery', { app: 0, query: 'revenue' }],
      ['core.submitQuery', { app: 0, query: 'revenue', variant: 'underlying' }],
      ['export.query', { app: 0, query: 'revenue', variant: 'underlying' }],
    ]);
  });

  it('shows an empty value muted, and a number in the reader\'s locale', async () => {
    const { app, query } = setup();
    await app.execute();
    rightClick(query, { region: 'APAC', total: '1500', n: '3' }, 'total');
    await settle();
    entries()[0].click();
    await settle();
    const first = content()!.querySelector('tbody tr')!;
    const cells = [...first.querySelectorAll('td')];
    expect(cells[1].textContent).toBe((1000).toLocaleString());
    expect(cells.map((cell) => cell.classList.contains('number'))).toEqual([true, true, false, false]);
    // `status` is null in the first row, and the result has no `created_at`: both empty.
    expect(cells.map((cell) => cell.classList.contains('empty'))).toEqual([false, false, true, true]);
  });

  it('downloads every row as a CSV through export.query', async () => {
    const { app, query, exports } = setup();
    await app.execute();
    rightClick(query, { region: 'APAC', total: '1500', n: '3' }, 'total');
    await settle();
    entries()[0].click();
    await settle();
    content()!.querySelector<HTMLButtonElement>('[data-testid="vud-download"]')!.click();
    await settle();
    expect(exports).toEqual([expect.objectContaining({
      dataset: 'sales',
      aql: expect.stringContaining('amount: orders.amount'),
      format: 'csv',
      formatOptions: { bom: true },
      filename: 'Sales - revenue - underlying data',
    })]);
    expect(exports[0]).not.toHaveProperty('page');
  });

  it('has no download when the host has no export', async () => {
    const { app, query } = setup({ plugins: ['core', 'view-underlying-data'] });
    await app.execute();
    rightClick(query, { region: 'APAC', total: '1500', n: '3' }, 'total');
    await settle();
    entries()[0].click();
    await settle();
    expect(content()!.querySelector('[data-testid="vud-download"]')).toBeNull();
  });

  it('stops loading rows when it closes', async () => {
    const signals: AbortSignal[] = [];
    const { app, query } = setup({
      pages: [(_request: BackendQueryRequest, signal: AbortSignal) => {
        signals.push(signal);
        return new Promise<BackendQueryResult>(() => {});
      }],
    });
    await app.execute();
    rightClick(query, { region: 'APAC', total: '1500', n: '3' }, 'total');
    await settle();
    entries()[0].click();
    await settle();
    expect(signals).toHaveLength(1);
    expect(signals[0].aborted).toBe(false);
    uiRoot()!.querySelector<HTMLButtonElement>('[role="dialog"] .close')!.click();
    await settle();
    expect(signals[0].aborted).toBe(true);
  });

  it('says what went wrong when the rows don\'t load', async () => {
    const { app, query } = setup({ pages: [new DataAppError({ code: 'internal_server_error', scope: 'server', message: 'The warehouse is asleep.', entity: 'revenue' })] });
    await app.execute();
    rightClick(query, { region: 'APAC', total: '1500', n: '3' }, 'total');
    await settle();
    entries()[0].click();
    await settle();
    const state = content()!.querySelector('[data-testid="vud-state"]')!;
    expect(state.textContent).toContain('The warehouse is asleep.');
    expect(state.classList.contains('error')).toBe(true);
  });
});
