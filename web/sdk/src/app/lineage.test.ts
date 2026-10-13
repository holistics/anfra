import { describe, expect, it } from 'vitest';
import { createSdk } from './sdk';
import { TransportError, ValidationError } from '../common/errors';
import type { LineageResult } from '../common/types';
import { salesDataset, stubBackend, testUser } from './testSupport';

const traced: LineageResult = {
  results: [{ target: { parent: 'orders', child: 'amount' }, query: 'needed_by:(fqn:orders.amount)', error: 'no catalog' }],
};

function sdk (stub = stubBackend({ lineage: traced })) {
  return { stub, sdk: createSdk({ datasets: { sales: salesDataset }, user: testUser, backend: stub.backend }) };
}

describe('Anfra.lineage', () => {
  const amount = { parent: 'orders', child: 'amount' };
  const aov = { parent: 'sales', child: 'aov' };

  it('asks the Backend for the fields, with the page, and answers what it answers', async () => {
    const { stub, sdk: anfra } = sdk();
    const controller = new AbortController();
    await expect(anfra.lineage([amount, aov], { limit: 5, offset: 10, signal: controller.signal }))
      .resolves.toEqual(traced);
    expect(stub.calls).toEqual([{
      method: 'lineage', request: { targets: [amount, aov], limit: 5, offset: 10 }, signal: controller.signal,
    }]);
  });

  it('takes one field alone, and sends no page it was not given', async () => {
    const { stub, sdk: anfra } = sdk();
    await anfra.lineage(amount);
    expect(stub.calls[0]).toMatchObject({ method: 'lineage', request: { targets: [amount] } });
    expect(stub.calls[0].request).not.toHaveProperty('limit');
  });

  it.each([
    [[]], ['orders.amount'], [['orders.amount']], [{ parent: 'orders' }], [[amount, { parent: 'orders', child: ' ' }]],
  ])('refuses %j, asking nothing', async (targets) => {
    const { stub, sdk: anfra } = sdk();
    await expect(anfra.lineage(targets as never)).rejects.toThrowError(ValidationError);
    expect(stub.calls).toEqual([]);
  });

  it('fails as a TransportError on a host that cannot trace lineage', async () => {
    const { sdk: anfra } = sdk(stubBackend());
    await expect(anfra.lineage(amount)).rejects.toThrowError(TransportError);
  });
});
