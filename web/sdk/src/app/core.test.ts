import { describe, expect, it } from 'vitest';
import { testFrame } from './testFrame';
import { textTypes } from './testSupport';
import type { BackendQueryRequest, BackendQueryResult } from '../common/types';

const aql = 'explore { dimensions { region: users.region } }';

const result = (): BackendQueryResult => ({
  columns: [{ name: 'region', modelId: 'users', fieldName: 'region', label: 'Region', adhoc: false, isMeasure: false, ...textTypes }],
  values: [['APAC']],
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

    const summary = await f.call('core.execute');

    expect(summary).toEqual({ succeeded: ['revenue', 'orders'], failed: ['failing'] });
    expect(f.requests).toHaveLength(3);
    expect(f.requests.every((request) => request.bustCache === true)).toBe(true);
    expect(f.requests[0].input.filters).toEqual([{ field: 'users.region', operator: 'is', values: ['APAC'] }]);
  });

  it('is not something a definition can reach', () => {
    const f = frame();
    expect(f.anfra).not.toHaveProperty('core');
  });
});
