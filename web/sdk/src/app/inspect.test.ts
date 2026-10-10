import {
  describe, expect, it, vi,
} from 'vitest';
import { createSdk } from './sdk';
import { PermissionError } from '../common/errors';
import {
  salesDataset, stubBackend, testUser, type StubOptions,
} from './testSupport';
import type { App } from './app';
import type { BackendQueryResult } from '../common/types';

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
  meta: { page: 1, pageSize: 1000, numRows: 2 },
  debug: { executedAql: 'explore { … }', fromCache: false, executedAt: new Date('2026-09-16T04:11:22Z') },
};

function build (submitQuery: StubOptions['submitQuery'] = twoRows) {
  const stub = stubBackend({ submitQuery });
  return createSdk({
    datasets: { sales: salesDataset },
    user: testUser,
    backend: stub.backend,
  });
}

function revenueApp (app: App) {
  const revenue = app.createQuery('revenue', {
    dataset: 'sales',
    aql: `
      explore {
        dimensions { region: users.region }
        measures { total: orders | sum(orders.amount) }
      }
    `,
  });
  const region = app.createFilter('region', { field: 'users.region', dataset: 'sales' });
  app.mapControl(region, revenue, { field: 'users.region' });
  return { revenue, region };
}

describe('the app registry', () => {
  it('keeps every app, because the bootstrap holds none of them', () => {
    // The author's own variable is the only other reference; a host has nothing to read from.
    const sdk = build();
    const first = sdk.createApp({ title: 'first' });
    const second = sdk.createApp({ title: 'second' });

    expect(sdk.apps).toEqual([first, second]);
  });

  it('is empty for a document that never builds one', () => {
    // A data app may render static HTML and call `createApp` never. That is not an error state.
    expect(build().apps).toEqual([]);
  });
});

describe('declaring', () => {
  it('notifies subscribers, so a host inspecting sees a dynamic app grow at once', () => {
    const app = build().createApp();
    const seen = vi.fn();
    app.subscribe(seen);
    const revenue = app.createQuery('revenue', {
      dataset: 'sales',
      aql: 'explore { dimensions { region: users.region } measures { total: orders | sum(orders.amount) } }',
    });
    const region = app.createFilter('region', { field: 'users.region', dataset: 'sales' });
    app.createDateDrill('grain');
    app.mapControl(region, revenue, { field: 'users.region' });
    expect(seen).toHaveBeenCalledTimes(4);
    expect(Object.keys(app.toInspectJSON().queries)).toEqual(['revenue']);
  });
});

describe('toInspectJSON', () => {
  it('carries the declaration through unchanged', () => {
    const app = build().createApp({ title: 'Sales' });
    revenueApp(app);

    expect(app.toInspectJSON().declaration).toEqual(app.toJSON());
  });

  it('reports the runtime state `toJSON` leaves out', async () => {
    const sdk = build();
    const app = sdk.createApp();
    const { region } = revenueApp(app);
    region.setCondition({ operator: 'is', values: ['APAC'] });
    await app.execute();

    const snapshot = app.toInspectJSON();

    expect(snapshot.queries.revenue).toMatchObject({
      state: 'success',
      isDirty: false,
      rowCount: 2,
      selectedRowCount: 0,
    });
    expect(snapshot.controls.region).toMatchObject({
      kind: 'filter',
      condition: { operator: 'is', values: ['APAC'] },
      appliedCondition: { operator: 'is', values: ['APAC'] },
      isDirty: false,
    });
    expect(snapshot.hasChanges).toBe(false);
  });

  it('shows a change the reader has made but not applied', async () => {
    const sdk = build();
    const app = sdk.createApp();
    const { region } = revenueApp(app);
    await app.execute();

    region.setCondition({ operator: 'is', values: ['EMEA'] });

    const snapshot = app.toInspectJSON();
    expect(snapshot.controls.region).toMatchObject({
      condition: { operator: 'is', values: ['EMEA'] },
      isDirty: true,
    });
    // The pending and applied values differ, which is the whole point of showing both.
    expect(snapshot.controls.region.appliedCondition).not.toEqual(snapshot.controls.region.condition);
    expect(snapshot.hasChanges).toBe(true);
  });

  it('reports no unapplied change before the first run', () => {
    // `isDirty` is false before anything is applied: there is nothing to differ from, so a control
    // set from a URL at startup is not an unapplied edit.
    const app = build().createApp();
    const { region } = revenueApp(app);

    region.setCondition({ operator: 'is', values: ['EMEA'] });

    expect(app.toInspectJSON().controls.region.isDirty).toBe(false);
    expect(app.toInspectJSON().hasChanges).toBe(false);
  });

  it('carries provenance, so the panel can say what ran', async () => {
    const sdk = build();
    const app = sdk.createApp();
    revenueApp(app);
    await app.execute();

    expect(app.toInspectJSON().queries.revenue.debug).toMatchObject({
      executedAql: 'explore { … }',
      fromCache: false,
    });
  });

  it('counts rows rather than carrying them', async () => {
    // Rows are the one unbounded thing in the payload, and the one thing the preview already shows.
    const sdk = build();
    const app = sdk.createApp();
    revenueApp(app);
    await app.execute();

    const query = app.toInspectJSON().queries.revenue;
    expect(query.rowCount).toBe(2);
    expect(query).not.toHaveProperty('rows');
    expect(query.columns).toHaveLength(2);
  });

  it('projects an error field by field, since a cloned one loses its own properties', async () => {
    const sdk = build(new PermissionError('Nope'));
    const app = sdk.createApp();
    revenueApp(app);
    await app.execute();

    const { error } = app.toInspectJSON().queries.revenue;
    // The class name is what tells a permission failure from a broken query, and it is the first
    // thing lost when a DataAppError is cloned rather than projected.
    expect(error).toMatchObject({ name: 'PermissionError', message: 'Nope' });
    // `cause` can hold whatever the backend threw, such as an AbortError; that need not cross a port.
    expect(error).not.toHaveProperty('cause');
  });

  it('keeps `entity` when the error carries one', async () => {
    // A QueryError names the query it is about. A PermissionError does not: it is raised by the
    // backend, which does not know whose query it is. The snapshot nests errors under the query
    // name either way, so the panel can always say which one failed. A plain Error from the
    // backend is not a DataAppError at all, which is what routes it through `toDataAppError`'s
    // fallback — the one path that stamps the query's own name onto it.
    const sdk = build(new Error('not json'));
    const app = sdk.createApp();
    revenueApp(app);
    await app.execute();

    expect(app.toInspectJSON().queries.revenue.error).toMatchObject({
      name: 'QueryError',
      entity: 'revenue',
    });
  });
});

describe('the snapshot survives a structured clone', () => {
  // The reason the projection exists at all. `structuredClone` throws on functions and
  // `AbortController`, and silently drops getters — so a snapshot that clones is the only proof
  // that a host will receive what these tests assert.
  it('clones when idle', () => {
    const app = build().createApp({ title: 'Sales' });
    revenueApp(app);

    const snapshot = app.toInspectJSON();
    expect(structuredClone(snapshot)).toEqual(snapshot);
  });

  it('clones after a successful run', async () => {
    const sdk = build();
    const app = sdk.createApp();
    revenueApp(app);
    await app.execute();

    const snapshot = app.toInspectJSON();
    expect(structuredClone(snapshot)).toEqual(snapshot);
  });

  it('clones while a query is in flight, which is when an AbortController is live', async () => {
    // `Query.controller` is set for the duration of every run — precisely when someone watching
    // the panel would be looking. Posting the live object here would throw.
    let release: (value: unknown) => void = () => {};
    const pending = new Promise((resolve) => { release = resolve; });
    const sdk = build(async () => { await pending; return twoRows; });
    const app = sdk.createApp();
    revenueApp(app);

    const run = app.execute();
    const snapshot = app.toInspectJSON();
    expect(snapshot.queries.revenue.state).toBe('executing');
    expect(() => structuredClone(snapshot)).not.toThrow();

    release(undefined);
    await run;
  });

  it('clones a selection whose rows are the author’s own objects', async () => {
    // Selection rows are kept verbatim, so one may hold a function a chart library attached. The
    // snapshot counts them instead of carrying them, which is what keeps it cloneable.
    const sdk = build();
    const app = sdk.createApp();
    const { revenue } = revenueApp(app);
    const target = app.createQuery('detail', {
      dataset: 'sales',
      aql: 'explore { dimensions { status: orders.status } }',
    });
    app.mapCrossFilter(revenue, target);
    // A query is a raw AQL body now, so `select()` needs a result to classify columns against.
    await app.execute();

    revenue.select([{ region: 'APAC', onClick: () => {} } as never]);

    const snapshot = app.toInspectJSON();
    expect(snapshot.selection).toMatchObject({ source: 'revenue', rowCount: 1, lossy: false });
    expect(() => structuredClone(snapshot)).not.toThrow();
  });
});
