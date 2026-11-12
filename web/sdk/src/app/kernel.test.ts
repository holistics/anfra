import { describe, expect, it } from 'vitest';
import { testFrame } from './testFrame';
import type { FrameKernel } from './kernel';
import { textTypes } from './testSupport';

describe('kernel.queryRequest', () => {
  it('is what the query runs with now: its AQL, what reaches it, and the timezone, with no page', async () => {
    let kernel!: FrameKernel;
    const f = testFrame({
      setups: [['notes', (k) => { kernel = k; }]],
      answers: {
        'core.submitQuery': () => ({
          columns: [{ name: 'region', modelId: 'users', fieldName: 'region', label: 'Region', adhoc: false, isMeasure: false, ...textTypes }],
          values: [['APAC']],
        }),
      },
    });
    const app = f.anfra.createApp({ timezone: 'Asia/Singapore' });
    const aql = 'explore { dimensions { region: users.region } }';
    const query = app.createQuery('revenue', { dataset: 'sales', aql, pageSize: 10 });
    const region = app.createFilter('region', { field: 'users.region' });
    app.mapControl(region, query, { field: 'users.region' });
    region.setCondition({ operator: 'is', values: ['APAC'] });
    query.setSort([{ field: 'region', direction: 'desc' }]);
    await app.execute();
    // Not applied yet: the request is what the rows on screen were run with.
    region.setCondition({ operator: 'is', values: ['EMEA'] });

    expect(kernel.queryRequest(query)).toEqual({
      dataset: 'sales',
      aql,
      input: {
        filters: [{ field: 'users.region', operator: 'is', values: ['APAC'] }],
        conditions: [],
        sorts: [{ field: 'region', direction: 'desc' }],
        dateDrills: [],
        timezone: 'Asia/Singapore',
      },
    });
  });

  it('refuses a query no app has', () => {
    let kernel!: FrameKernel;
    const f = testFrame({ setups: [['notes', (k) => { kernel = k; }]] });
    const other = testFrame().anfra.createApp().createQuery('revenue', { dataset: 'sales', aql: 'explore {}' });
    f.anfra.createApp();
    expect(() => kernel.queryRequest(other)).toThrow(/belongs to no app/);
  });
});

describe('kernel.call', () => {
  it('adds the id of the query the stamp names, now, and none for a query the app lacks', async () => {
    let kernel!: FrameKernel;
    const f = testFrame({ setups: [['notes', (k) => { kernel = k; }]], answers: { 'notes.look': () => null } });
    const app = f.anfra.createApp();
    const first = app.createQuery('revenue', { dataset: 'sales', aql: 'explore {}' });
    first.remove();
    const again = app.createQuery('revenue', { dataset: 'sales', aql: 'explore {}' });

    await kernel.call('notes.look', null, undefined, { app: 0, query: 'revenue', variant: 'notes' });
    await kernel.call('notes.look', null, undefined, { app: 0, query: 'revenue', id: first.id } as never);
    await kernel.call('notes.look', null, undefined, { app: 0, query: 'gone' });

    expect(f.calls.map((call) => call.stamp)).toEqual([
      { app: 0, query: 'revenue', id: again.id, variant: 'notes' },
      { app: 0, query: 'revenue', id: again.id },
      { app: 0, query: 'gone' },
    ]);
  });
});
