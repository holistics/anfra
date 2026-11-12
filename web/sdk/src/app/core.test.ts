import { describe, expect, it } from 'vitest';
import { flush, testFrame } from './testFrame';
import { textTypes } from './testSupport';
import type { BackendQueryRequest, BackendQueryResult } from '../common/types';

const aql = 'explore { dimensions { region: users.region } }';

const result = (debug?: BackendQueryResult['debug']): BackendQueryResult => ({
  columns: [{ name: 'region', modelId: 'users', fieldName: 'region', label: 'Region', adhoc: false, isMeasure: false, ...textTypes }],
  values: [['APAC']],
  ...(debug ? { debug } : {}),
});

/** A frame whose queries the host answers with `answer`, every request kept. */
function frame (answer: (request: BackendQueryRequest) => BackendQueryResult | Error = () => result()) {
  const requests: BackendQueryRequest[] = [];
  const f = testFrame({
    answers: {
      'core.submitQuery': (request: BackendQueryRequest) => {
        requests.push(request);
        const answered = answer(request);
        if (answered instanceof Error) throw answered;
        return answered;
      },
    },
  });
  return { ...f, requests };
}

const results = (f: { events: { name: string, payload: unknown }[] }) => f.events.filter((event) => event.name === 'core.results').map((event) => event.payload);

describe('core.execute, from the host', () => {
  it('runs every query of every app again with its current inputs, and answers the combined summary', async () => {
    const f = frame((request) => (request.aql.includes('broken') ? new Error('no') : result()));
    const sales = f.anfra.createApp();
    const revenue = sales.createQuery('revenue', { dataset: 'sales', aql });
    const region = sales.createFilter('region', { field: 'users.region' });
    sales.mapControl(region, revenue, { field: 'users.region' });
    region.setCondition({ operator: 'is', values: ['APAC'] });
    const other = f.anfra.createApp();
    other.createQuery('orders', { dataset: 'sales', aql });
    other.createQuery('failing', { dataset: 'sales', aql: 'explore { broken }' });
    await sales.execute();
    await other.execute();
    f.requests.length = 0;

    const summary = await f.call('core.execute', { force: true, bustCache: true });

    expect(summary).toEqual({ succeeded: ['revenue', 'orders'], failed: ['failing'] });
    expect(f.requests).toHaveLength(3);
    expect(f.requests.every((request) => request.bustCache === true)).toBe(true);
    expect(f.requests[0].input.filters).toEqual([{ field: 'users.region', operator: 'is', values: ['APAC'] }]);
  });

  it('sends no bustCache when the host asks for none', async () => {
    const f = frame();
    f.anfra.createApp().createQuery('revenue', { dataset: 'sales', aql });
    await f.call('core.execute', { force: true, bustCache: false });
    expect(f.requests).toHaveLength(1);
    expect(f.requests[0]).not.toHaveProperty('bustCache');
  });

  it('refuses options that are not ones', async () => {
    const f = frame();
    await expect(f.call('core.execute', { force: 'yes' })).rejects.toMatchObject({ code: 'invalid_request' });
  });

  it('is not something a definition can reach', () => {
    const f = frame();
    expect(f.anfra).not.toHaveProperty('core');
  });
});

describe('core.results, to the host', () => {
  it('says nothing is cached once results settle that came from no cache', async () => {
    const f = frame();
    const app = f.anfra.createApp();
    app.createQuery('revenue', { dataset: 'sales', aql });
    await app.execute();
    await flush();
    expect(results(f)).toEqual([{ cached: false }]);
  });

  it('says a cached result shows, since the oldest cached run', async () => {
    const at = { revenue: '2026-11-12T08:00:00.000Z', orders: '2026-11-12T07:00:00.000Z' };
    const f = frame((request) => {
      if (request.aql.includes('fresh')) return result({ fromCache: false, executedAt: new Date() });
      const name = request.aql.includes('orders') ? 'orders' : 'revenue';
      return result({ fromCache: true, executedAt: new Date(at[name]) });
    });
    const app = f.anfra.createApp();
    app.createQuery('revenue', { dataset: 'sales', aql });
    app.createQuery('orders', { dataset: 'sales', aql: `${aql} // orders` });
    app.createQuery('live', { dataset: 'sales', aql: `${aql} // fresh` });
    await app.execute();
    await flush();
    expect(results(f)).toEqual([{ cached: true, cachedSince: at.orders }]);
  });

  it('is sent once per change, after the queries settle, not while they run', async () => {
    let cached = true;
    const f = frame(() => result({ fromCache: cached, executedAt: new Date('2026-11-12T08:00:00.000Z') }));
    const app = f.anfra.createApp();
    app.createQuery('revenue', { dataset: 'sales', aql });
    await app.execute();
    await flush();
    await app.refresh({ bustCache: false });
    await flush();
    cached = false;
    await app.refresh();
    await flush();
    expect(results(f)).toEqual([{ cached: true, cachedSince: '2026-11-12T08:00:00.000Z' }, { cached: false }]);
  });
});
