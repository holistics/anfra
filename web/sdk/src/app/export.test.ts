import { describe, expect, it } from 'vitest';
import { createSdk } from './sdk';
import { QueryError, TransportError } from '../common/errors';
import { salesDataset, stubBackend, testUser, textTypes, type StubOptions } from './testSupport';
import type { ExportResult } from '../common/types';

const link: ExportResult = {
  url: 'http://127.0.0.1:7878/exports/download/t0k3n/revenue.csv',
  filename: 'revenue.csv',
  format: 'csv',
  rowCount: 3,
  expiresAt: new Date('2026-10-13T10:00:00Z'),
};

const aql = 'explore { dimensions { region: users.region } measures { total: sum(orders.amount) } }';

function setup (options: StubOptions = { exportQuery: link }, title?: string) {
  const stub = stubBackend({
    submitQuery: {
      columns: [{
        name: 'region', modelId: 'users', fieldName: 'region', label: 'Region', adhoc: false, isMeasure: false, ...textTypes,
      }],
      values: [['APAC']],
    },
    ...options,
  });
  const sdk = createSdk({ datasets: { sales: salesDataset }, user: testUser, backend: stub.backend });
  const app = sdk.createApp({ ...(title ? { title } : {}), timezone: 'Asia/Singapore' });
  const query = app.createQuery('revenue', { dataset: 'sales', aql, pageSize: 2 });
  return { stub, sdk, app, query };
}

describe('query.export', () => {
  it('sends what the query was last run with, every row, and answers the link', async () => {
    const { stub, app, query } = setup();
    const region = app.createFilter('region', { field: 'users.region' });
    app.mapControl(region, query, { field: 'users.region' });
    region.setCondition({ operator: 'is', values: ['APAC'] });
    query.setSort([{ field: 'total', direction: 'desc' }]);
    await app.execute();

    await expect(query.export()).resolves.toEqual(link);

    const [request] = stub.exports;
    expect(request).toEqual({
      dataset: 'sales',
      aql,
      input: {
        filters: [{ field: 'users.region', operator: 'is', values: ['APAC'] }],
        conditions: [],
        sorts: [{ field: 'total', direction: 'desc' }],
        dateDrills: [],
      },
      timezone: 'Asia/Singapore',
      format: 'csv',
      formatOptions: { bom: true },
      filename: 'revenue',
    });
    // An export is the whole result: no page, whatever the query declares.
    expect(request).not.toHaveProperty('page');
    expect(request).not.toHaveProperty('pageSize');
  });

  it('leaves out a control the reader changed but has not applied', async () => {
    const { stub, app, query } = setup();
    const region = app.createFilter('region', { field: 'users.region' });
    app.mapControl(region, query, { field: 'users.region' });
    region.setCondition({ operator: 'is', values: ['APAC'] });
    await app.execute();
    region.setCondition({ operator: 'is', values: ['EMEA'] });

    await query.export();

    expect(stub.exports[0].input.filters).toEqual([{ field: 'users.region', operator: 'is', values: ['APAC'] }]);
  });

  it('carries the selection the query was last run with', async () => {
    const { stub, app, query } = setup();
    const byRegion = app.createQuery('byRegion', { dataset: 'sales', aql });
    app.mapCrossFilter(byRegion, query);
    await app.execute();
    byRegion.select({ region: 'APAC' });
    await app.execute();

    await query.export();

    expect(stub.exports[0].input.filters).toEqual([{ field: 'users.region', operator: 'is', values: ['APAC'] }]);
  });

  it('names the file after the app and the query, with what a file name cannot hold replaced', async () => {
    const { stub, query } = setup(undefined, 'Sales / Q3');

    await query.export();
    await query.export({ filename: '  mine.csv ' });

    expect(stub.exports.map((r) => r.filename)).toEqual(['Sales - Q3 - revenue', 'mine.csv']);
  });

  it('passes the format and its options through, a BOM by default only for CSV', async () => {
    const { stub, query } = setup();

    await query.export({ formatOptions: { header: 'names', bom: false } });
    await query.export({ format: 'xlsx' });

    expect(stub.exports[0]).toMatchObject({ format: 'csv', formatOptions: { header: 'names', bom: false } });
    expect(stub.exports[1].format).toBe('xlsx');
    expect(stub.exports[1]).not.toHaveProperty('formatOptions');
  });

  it('changes nothing on the query, even when it fails', async () => {
    const { app, query } = setup({ exportQuery: new QueryError('format_options.header: not an option of xlsx') });
    await app.execute();
    const before = { state: query.state, result: query.result, error: query.error };

    const err = await query.export().catch((e: unknown) => e);

    expect(err).toBeInstanceOf(QueryError);
    expect((err as QueryError).entity).toBe('revenue');
    expect({ state: query.state, result: query.result, error: query.error }).toEqual(before);
  });

  it('rejects with a TransportError naming the query when the host cannot export', async () => {
    const { stub, sdk, query } = setup({});

    expect(sdk.features.export).toBe(false);
    const err = await query.export().catch((e: unknown) => e);
    expect(err).toBeInstanceOf(TransportError);
    expect((err as TransportError).message).toMatch(/cannot make exports/);
    expect((err as TransportError).entity).toBe('revenue');
    expect(stub.exports).toEqual([]);
  });

  it('says whether the host can export, from the Backend or from the host', () => {
    const backend = stubBackend({ exportQuery: link }).backend;
    const env = { datasets: { sales: salesDataset }, user: testUser, backend };

    expect(createSdk(env).features.export).toBe(true);
    expect(createSdk({ ...env, features: { export: false } }).features.export).toBe(false);
  });

  it('is cancelled by its own signal, not by query.abort()', async () => {
    let seen: AbortSignal | undefined;
    const { query } = setup({
      exportQuery: (_request, signal) => {
        seen = signal;
        return new Promise<ExportResult>((resolve) => { setTimeout(() => resolve(link), 0); });
      },
    });
    const controller = new AbortController();

    const pending = query.export({ signal: controller.signal });
    query.abort();
    expect(seen?.aborted).toBe(false);
    controller.abort();
    expect(seen?.aborted).toBe(true);
    await pending;
  });
});
