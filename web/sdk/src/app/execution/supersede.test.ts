import { describe, expect, it } from 'vitest';
import { createSdk } from '../sdk';
import { salesDataset, testUser, textTypes, numberTypes } from '../testSupport';
import type { Backend, BackendQueryResult } from '../../common/types';

interface Deferred {
  resolve: (value: unknown) => void;
  promise: Promise<unknown>;
}

function deferred (): Deferred {
  let resolve!: (value: unknown) => void;
  const promise = new Promise<unknown>((r) => { resolve = r; });
  return { resolve, promise };
}

const rows = (region: string): BackendQueryResult => ({
  values: [[region, '1']],
  columns: [
    {
      name: 'region', fieldName: 'region', modelId: 'users', label: 'Region', adhoc: false, isMeasure: false,
      ...textTypes,
    },
    {
      name: 'total', fieldName: 'amount', label: 'Sum of Amount', adhoc: false, isMeasure: true, aggregation: 'sum',
      ...numberTypes,
    },
  ],
  meta: { page: 1, pageSize: 1000, numRows: 1 },
});

describe('superseding execute', () => {
  it('lets the newer run own the result, and aborts the older request', async () => {
    const gates = [deferred(), deferred()];
    const aborted: boolean[] = [];
    let call = 0;

    // Resolves even after an abort, like a backend that can't cancel: the SDK must still ignore it.
    const backend: Backend = {
      async submitQuery (_request, signal) {
        const index = call++;
        aborted[index] = false;
        signal.addEventListener('abort', () => { aborted[index] = true; });

        await gates[index].promise;
        return rows(index === 0 ? 'FIRST' : 'SECOND');
      },
      async fieldSuggestions () { return []; },
    };

    const sdk = createSdk({ datasets: { sales: salesDataset }, user: testUser, backend });
    const app = sdk.createApp();
    const query = app.createQuery('revenue', {
      dataset: 'sales',
      aql: `
        explore {
          dimensions { region: users.region }
          measures { total: orders | sum(orders.amount) }
        }
      `,
    });
    const region = app.createFilter('region', { field: 'users.region' });
    app.mapControl(region, query, { field: 'users.region' });

    const first = app.execute();
    // Change the input so the second run considers the query dirty and supersedes the first.
    region.setCondition({ operator: 'is', values: ['EMEA'] });
    const second = app.execute();

    expect(aborted[0]).toBe(true);

    // The superseded call answers last, after the newer one has already landed.
    gates[1].resolve(undefined);
    await second;
    gates[0].resolve(undefined);
    const [firstSummary, secondSummary] = await Promise.all([first, second]);

    // The superseded run reports nothing rather than claiming a success it did not write.
    expect(firstSummary).toEqual({ succeeded: [], failed: [] });
    expect(secondSummary.succeeded).toEqual(['revenue']);
    expect(query.result?.rows).toEqual([{ region: 'SECOND', total: '1' }]);
    expect(query.state).toBe('success');
  });
});

describe('execute with a run in flight for the current inputs', () => {
  /** A backend whose every request hangs until its gate opens, recording whether it was aborted. */
  function gatedBackend () {
    const gates: Deferred[] = [];
    const aborted: boolean[] = [];
    const backend: Backend = {
      async submitQuery (_request, signal) {
        const index = gates.length;
        gates.push(deferred());
        aborted[index] = false;
        signal.addEventListener('abort', () => { aborted[index] = true; });
        await gates[index].promise;
        return rows(`RUN ${index}`);
      },
      async fieldSuggestions () { return []; },
    };
    return { backend, gates, aborted };
  }

  function setup () {
    const gated = gatedBackend();
    const app = createSdk({ datasets: { sales: salesDataset }, user: testUser, backend: gated.backend }).createApp();
    const query = app.createQuery('revenue', {
      dataset: 'sales',
      aql: `
        explore {
          dimensions { region: users.region }
          measures { total: orders | sum(orders.amount) }
        }
      `,
    });
    const region = app.createFilter('region', { field: 'users.region' });
    app.mapControl(region, query, { field: 'users.region' });
    return { ...gated, app, query, region };
  }

  it('leaves the first run alone', async () => {
    const { app, gates, aborted } = setup();

    const first = app.execute();
    expect(app.hasChanges).toBe(false);
    const second = app.execute();

    expect(aborted[0]).toBe(false);
    expect(gates).toHaveLength(1);

    // The second call waits for the first run, and reports it as its own.
    let settled = false;
    void second.then(() => { settled = true; });
    await Promise.resolve();
    expect(settled).toBe(false);

    gates[0].resolve(undefined);
    const [firstSummary, secondSummary] = await Promise.all([first, second]);
    expect(firstSummary.succeeded).toEqual(['revenue']);
    expect(secondSummary.succeeded).toEqual(['revenue']);
  });

  it('leaves a later run alone once its inputs were applied', async () => {
    const { app, query, region, gates, aborted } = setup();

    const first = app.execute();
    gates[0].resolve(undefined);
    await first;

    region.setCondition({ operator: 'is', values: ['EMEA'] });
    const second = app.execute();
    expect(app.hasChanges).toBe(false);
    const third = app.execute();

    expect(aborted[1]).toBe(false);
    expect(gates).toHaveLength(2);

    gates[1].resolve(undefined);
    const [, thirdSummary] = await Promise.all([second, third]);
    expect(thirdSummary.succeeded).toEqual(['revenue']);
    expect(query.result?.rows).toEqual([{ region: 'RUN 1', total: '1' }]);
  });

  it('leaves the first run alone when the query is executed on its own', async () => {
    const { app, query, gates, aborted } = setup();

    const first = app.execute();
    const alone = query.execute();

    expect(aborted[0]).toBe(false);
    expect(gates).toHaveLength(1);

    gates[0].resolve(undefined);
    const [, aloneSummary] = await Promise.all([first, alone]);
    expect(aloneSummary.succeeded).toEqual(['revenue']);
  });

  it('sends the run again when it was aborted just before', async () => {
    const { app, gates } = setup();

    const first = app.execute();
    app.abort();
    const second = app.execute();

    expect(gates).toHaveLength(2);

    gates.forEach((gate) => gate.resolve(undefined));
    const [, secondSummary] = await Promise.all([first, second]);
    expect(secondSummary.succeeded).toEqual(['revenue']);
  });

  it('sends the run again when asked to skip the cache', async () => {
    const { app, gates, aborted } = setup();

    const first = app.execute();
    const busting = app.execute({ bustCache: true });

    expect(aborted[0]).toBe(true);
    expect(gates).toHaveLength(2);

    gates.forEach((gate) => gate.resolve(undefined));
    await Promise.all([first, busting]);
  });

  it('leaves the run alone when a listener executes the query as it starts', async () => {
    const { app, query, gates, aborted } = setup();
    const reacting: Promise<unknown>[] = [];
    query.subscribe(() => {
      if (query.state === 'executing' && reacting.length === 0) reacting.push(query.execute());
    });

    const first = app.execute();

    expect(aborted[0]).toBe(false);
    expect(gates).toHaveLength(1);

    gates[0].resolve(undefined);
    await Promise.all([first, ...reacting]);
  });

  it('still aborts and resends on refresh', async () => {
    const { app, gates, aborted } = setup();

    const first = app.execute();
    const refreshed = app.refresh();

    expect(aborted[0]).toBe(true);
    expect(gates).toHaveLength(2);

    gates.forEach((gate) => gate.resolve(undefined));
    await Promise.all([first, refreshed]);
  });
});
