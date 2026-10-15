import { describe, expect, it } from 'vitest';
import { QueryError } from '../../common/errors';
import { stubBackend, textTypes, type StubOptions } from '../../app/testSupport';
import { flush, testFrame } from '../../app/testFrame';
import type { Query } from '../../app/query';
import type { ExportOptions, ExportResult } from '../../common/types';
import setupExport from './frame';

type Exportable = Query & { export: (options?: ExportOptions) => Promise<ExportResult> };

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
  const frame = testFrame({
    setups: [['export', setupExport]],
    answers: {
      'core.submitQuery': (request, signal) => stub.backend.submitQuery(request, signal),
      ...(stub.backend.exportQuery ? { 'export.query': (request, signal) => stub.backend.exportQuery!(request, signal) } : {}),
    },
  });
  const sdk = frame.anfra;
  const app = sdk.createApp({ ...(title ? { title } : {}), timezone: 'Asia/Singapore' });
  const query = app.createQuery('revenue', { dataset: 'sales', aql, pageSize: 2 }) as Exportable;
  return {
    stub, sdk, app, query, frame,
  };
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

  it('is the export plugin\'s: absent where the host did not install it', () => {
    const frame = testFrame({ answers: {} });
    const query = frame.anfra.createApp().createQuery('revenue', { dataset: 'sales', aql });

    expect(frame.anfra.plugins.has('export')).toBe(false);
    expect((query as Partial<Exportable>).export).toBeUndefined();
  });

  it('asks the host by the plugin\'s call, and is the same function each time it is read', async () => {
    const { query, frame } = setup();

    expect(query.export).toBe(query.export);
    await query.export();
    expect(frame.calls.map((call) => call.name)).toEqual(['export.query']);
  });

  it('is cancelled by its own signal, not by query.abort()', async () => {
    let seen: AbortSignal | undefined;
    const { query } = setup({
      exportQuery: (_request, signal) => {
        seen = signal;
        return new Promise<ExportResult>(() => {});
      },
    });
    const controller = new AbortController();

    const pending = query.export({ signal: controller.signal }).catch((err: unknown) => err);
    query.abort();
    await flush();
    expect(seen?.aborted).toBe(false);
    controller.abort();
    await flush();
    expect(seen?.aborted).toBe(true);
    expect(await pending).toMatchObject({ name: 'AbortError' });
  });
});
