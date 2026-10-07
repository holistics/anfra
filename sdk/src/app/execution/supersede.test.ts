import { describe, expect, it } from 'vitest';
import { createSdk } from '../sdk';
import { salesDataset, testUser } from '../testSupport';
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
    },
    {
      name: 'total', fieldName: 'amount', label: 'Sum of Amount', adhoc: false, isMeasure: true, aggregation: 'sum',
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
