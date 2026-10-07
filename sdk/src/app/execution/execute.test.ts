import { describe, expect, it } from 'vitest';
import { createSdk } from '../sdk';
import { PermissionError, QueryError, ValidationError } from '../../common/errors';
import {
  salesDataset, stubBackend, testUser, type StubOptions,
} from '../testSupport';
import type { App } from '../app';
import type { BackendQueryResult } from '../../common/types';

function build (submitQuery: StubOptions['submitQuery'], extraDatasets: Record<string, typeof salesDataset> = {}) {
  const stub = stubBackend({ submitQuery });
  const sdk = createSdk({
    datasets: { sales: salesDataset, ...extraDatasets },
    user: testUser,
    backend: stub.backend,
  });
  return { stub, sdk };
}

const REVENUE_AQL = `
  explore {
    dimensions { region: users.region }
    measures { total: orders | sum(orders.amount) }
  }
`;

function revenueBy (app: App) {
  return app.createQuery('revenue', { dataset: 'sales', aql: REVENUE_AQL });
}

const twoRows: BackendQueryResult = {
  values: [['APAC', '181'], ['EMEA', '800']],
  columns: [
    {
      name: 'region', fieldName: 'region', modelId: 'users', label: 'Region', adhoc: false, isMeasure: false,
    },
    {
      name: 'total', fieldName: 'amount', label: 'Sum of Amount', adhoc: false, isMeasure: true, aggregation: 'sum',
    },
  ],
  meta: { numRows: 2 },
};

describe('request', () => {
  it('sends the raw AQL body verbatim, with no field knowledge of its own', async () => {
    const { stub, sdk } = build(twoRows);
    const app = sdk.createApp();
    revenueBy(app);

    await app.execute();

    expect(stub.queries).toHaveLength(1);
    expect(stub.queries[0]).toEqual({
      dataset: 'sales',
      aql: REVENUE_AQL,
      input: {
        filters: [], conditions: [], sorts: [], dateDrills: [],
      },
    });
  });

  it('sends a page only for a query that declares a page size', async () => {
    const { stub, sdk } = build(twoRows);
    const app = sdk.createApp();
    app.createQuery('revenue', { dataset: 'sales', aql: REVENUE_AQL, pageSize: 50 });

    await app.execute();

    expect(stub.queries[0]).toMatchObject({ page: 1, pageSize: 50 });
  });

  it('sends the aggregation on a mapped filter, the same key regardless of what it targets', async () => {
    const { stub, sdk } = build(twoRows);
    const app = sdk.createApp();
    const query = revenueBy(app);
    const big = app.createFilter('big', { type: 'number' });
    app.mapControl(big, query, { field: 'orders.id', aggregation: 'count' });
    big.setCondition({ operator: 'greater_than', values: [5] });

    await app.execute();

    expect(stub.queries[0].input.filters).toEqual([
      {
        field: 'orders.id', operator: 'greater_than', values: [5], aggregation: 'count',
      },
    ]);
  });

  it('sends a mapped date drill as a date drill, never as a filter', async () => {
    const { stub, sdk } = build(twoRows);
    const app = sdk.createApp();
    const query = revenueBy(app);
    const grain = app.createDateDrill('grain', { default: 'month' });
    app.mapControl(grain, query, { field: 'orders.created_at' });

    await app.execute();
    grain.setGrain('week');
    await app.execute();

    expect(stub.queries[0].input).toMatchObject({
      filters: [],
      dateDrills: [{ field: 'orders.created_at', grain: 'month' }],
    });
    expect(stub.queries[1].input.dateDrills).toEqual([{ field: 'orders.created_at', grain: 'week' }]);
  });

  it('omits timezone when the app declares none, leaving the default to the backend', async () => {
    const { stub, sdk } = build(twoRows);
    const app = sdk.createApp();
    revenueBy(app);

    await app.execute();

    expect(stub.queries[0]).not.toHaveProperty('timezone');
  });

  it('sends the app timezone when it declares one', async () => {
    const { stub, sdk } = build(twoRows);
    const app = sdk.createApp({ timezone: 'Asia/Singapore' });
    revenueBy(app);

    await app.execute();

    expect(stub.queries[0].timezone).toBe('Asia/Singapore');
  });

  it('ANDs conditions arriving through two mapped controls', async () => {
    const { stub, sdk } = build(twoRows);
    const app = sdk.createApp();
    const query = revenueBy(app);
    const status = app.createFilter('status', { type: 'string' });
    const region = app.createFilter('region', { field: 'users.region' });
    app.mapControl(status, query, { field: 'orders.status' });
    app.mapControl(region, query, { field: 'users.region' });
    status.setCondition({ operator: 'is', values: ['completed'] });
    region.setCondition({ operator: 'is', values: ['APAC'] });

    await app.execute();

    expect(stub.queries[0].input.filters).toEqual([
      { field: 'orders.status', operator: 'is', values: ['completed'] },
      { field: 'users.region', operator: 'is', values: ['APAC'] },
    ]);
  });

  it('contributes nothing for a control the reader has not set', async () => {
    const { stub, sdk } = build(twoRows);
    const app = sdk.createApp();
    const query = revenueBy(app);
    const region = app.createFilter('region', { field: 'users.region' });
    app.mapControl(region, query, { field: 'users.region' });

    await app.execute();

    expect(stub.queries[0].input.filters).toEqual([]);
  });

  it('hands the backend a signal it can watch for cancellation', async () => {
    const { stub, sdk } = build(twoRows);
    const app = sdk.createApp();
    revenueBy(app);

    await app.execute();

    expect(stub.calls[0].signal).toBeInstanceOf(AbortSignal);
  });
});

describe('results', () => {
  it('keys rows by column name, keeping the backend\'s column classification', async () => {
    const { sdk } = build(twoRows);
    const app = sdk.createApp();
    const query = revenueBy(app);

    await app.execute();

    expect(query.state).toBe('success');
    expect(query.result?.rows).toEqual([
      { region: 'APAC', total: '181' },
      { region: 'EMEA', total: '800' },
    ]);
    expect(query.result?.columns).toEqual(twoRows.columns);
    // Unpaged: every row, so there is no page to report.
    expect(query.result?.meta).toEqual({ numRows: 2 });
  });

  it('fills in paging metadata the backend leaves out', async () => {
    const { sdk } = build({ columns: twoRows.columns, values: twoRows.values });
    const app = sdk.createApp();
    const query = app.createQuery('revenue', { dataset: 'sales', aql: REVENUE_AQL, pageSize: 50 });

    await app.execute();

    expect(query.result?.meta).toEqual({ page: 1, pageSize: 50, numRows: 2 });
  });

  it('surfaces provenance when the backend sends it', async () => {
    const debug = {
      executedAql: 'explore { }',
      executedSql: 'SELECT 1',
      fromCache: true,
      executedAt: new Date('2026-09-12T04:11:22Z'),
    };
    const { sdk } = build({ ...twoRows, debug });
    const app = sdk.createApp();
    const query = revenueBy(app);

    await app.execute();

    expect(query.result?.debug).toEqual(debug);
  });

  it('surfaces a backend failure as a QueryError carrying its message', async () => {
    const { sdk } = build(new Error('Syntax error near ORDER'));
    const app = sdk.createApp();
    const query = revenueBy(app);

    const summary = await app.execute();

    expect(summary.failed).toEqual(['revenue']);
    expect(query.error).toBeInstanceOf(QueryError);
    expect(query.error?.message).toBe('Syntax error near ORDER');
  });

  it('keeps the DataAppError a backend chose, so a permission failure reads as one', async () => {
    const { sdk } = build(new PermissionError('Not allowed'));
    const app = sdk.createApp();
    const query = revenueBy(app);

    await app.execute();

    expect(query.error).toBeInstanceOf(PermissionError);
  });
});

describe('execute', () => {
  it('resolves with per-query outcomes instead of rejecting, so one failure blanks nothing', async () => {
    const { sdk } = build(
      (request) => (request.dataset === 'sales' ? twoRows : new Error('boom')),
      { unlucky: { ...salesDataset, id: 99, name: 'unlucky' } },
    );
    const app = sdk.createApp();
    const good = revenueBy(app);
    const bad = app.createQuery('broken', {
      dataset: 'unlucky',
      aql: 'explore { measures { total: orders | sum(orders.amount) } }',
    });

    const summary = await app.execute();

    expect(summary.succeeded).toEqual(['revenue']);
    expect(summary.failed).toEqual(['broken']);
    expect(good.result?.rows).toHaveLength(2);
    expect(bad.result).toBeUndefined();
  });

  it('re-runs only what would produce something new', async () => {
    const { stub, sdk } = build(twoRows);
    const app = sdk.createApp();
    const query = revenueBy(app);
    const region = app.createFilter('region', { field: 'users.region' });
    app.mapControl(region, query, { field: 'users.region' });

    await app.execute();
    expect(stub.calls).toHaveLength(1);

    await app.execute();
    expect(stub.calls).toHaveLength(1);

    region.setCondition({ operator: 'is', values: ['APAC'] });
    expect(query.isDirty).toBe(true);
    expect(app.hasChanges).toBe(true);

    await app.execute();
    expect(stub.calls).toHaveLength(2);
    expect(app.hasChanges).toBe(false);
  });

  it('treats a timezone change as changing the rows', async () => {
    const { stub, sdk } = build(twoRows);
    const app = sdk.createApp();
    revenueBy(app);

    await app.execute();
    app.setTimezone('Asia/Tokyo');
    await app.execute();

    expect(stub.calls).toHaveLength(2);
  });

  it('treats a sort override as changing the rows, without touching the AQL', async () => {
    const { stub, sdk } = build(twoRows);
    const app = sdk.createApp();
    const query = revenueBy(app);

    await app.execute();
    query.setSort([{ field: 'total', direction: 'desc' }]);
    expect(query.isDirty).toBe(true);
    await app.execute();

    expect(stub.calls).toHaveLength(2);
    expect(stub.queries[1].aql).toBe(REVENUE_AQL);
    expect(stub.queries[1].input.sorts).toEqual([{ field: 'total', direction: 'desc' }]);
  });

  it('re-runs everything on force, cache-busting on refresh', async () => {
    const { stub, sdk } = build(twoRows);
    const app = sdk.createApp();
    revenueBy(app);

    await app.execute();
    await app.refresh();

    expect(stub.calls).toHaveLength(2);
    expect(stub.queries[0]).not.toHaveProperty('bustCache');
    expect(stub.queries[1].bustCache).toBe(true);
  });

  it('notifies subscribers of any entity through one app subscription', async () => {
    const { sdk } = build(twoRows);
    const app = sdk.createApp();
    const query = revenueBy(app);
    const region = app.createFilter('region', { field: 'users.region' });

    let notifications = 0;
    const unsubscribe = app.subscribe(() => { notifications += 1; });

    region.setCondition({ operator: 'is', values: ['APAC'] });
    await app.execute();

    expect(notifications).toBeGreaterThan(1);
    expect(query.state).toBe('success');

    unsubscribe();
    const before = notifications;
    region.setCondition({ operator: 'is', values: ['EMEA'] });
    expect(notifications).toBe(before);
  });
});

describe('paging', () => {
  it('appends the next page rather than replacing the rows', async () => {
    const page2: BackendQueryResult = {
      values: [['LATAM', '12']],
      columns: twoRows.columns,
      meta: { page: 2, pageSize: 2, numRows: 1 },
    };
    let call = 0;
    const { stub, sdk } = build(() => {
      call += 1;
      return call === 1 ? { ...twoRows, meta: { page: 1, pageSize: 2, numRows: 2 } } : page2;
    });
    const app = sdk.createApp();
    const query = app.createQuery('revenue', { dataset: 'sales', aql: REVENUE_AQL, pageSize: 2 });

    await app.execute();
    expect(query.hasMore).toBe(true);

    await query.fetchMore();

    expect(stub.queries.map((request) => [request.page, request.pageSize])).toEqual([[1, 2], [2, 2]]);
    expect(query.result?.rows).toHaveLength(3);
    expect(query.result?.meta.numRows).toBe(3);
    expect(query.page).toBe(2);
    expect(query.hasMore).toBe(false);
  });

  it('has no more pages for a query that declares no page size, and says so on fetchMore', async () => {
    const { stub, sdk } = build(twoRows);
    const app = sdk.createApp();
    const query = revenueBy(app);

    await app.execute();
    expect(query.hasMore).toBe(false);

    await expect(query.fetchMore()).rejects.toThrowError(
      new ValidationError("Query 'revenue' declares no pageSize, so it has no more pages.", 'revenue'),
    );
    expect(stub.queries).toHaveLength(1);
  });
});
