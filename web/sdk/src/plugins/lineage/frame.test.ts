import { describe, expect, it } from 'vitest';
import type { LineageResult } from '../../common/types';
import { stubBackend } from '../../app/testSupport';
import { testFrame } from '../../app/testFrame';
import setupLineage from './frame';

const traced: LineageResult = {
  results: [{ target: { parent: 'orders', child: 'amount' }, query: 'needed_by:(fqn:orders.amount)', error: 'no catalog' }],
};

function sdk (stub = stubBackend({ lineage: traced })) {
  const frame = testFrame({
    setups: [['lineage', setupLineage]],
    answers: { 'lineage.trace': (request, signal) => stub.backend.lineage!(request, signal) },
  });
  return { stub, sdk: frame.anfra };
}

describe('Anfra.lineage', () => {
  const amount = { parent: 'orders', child: 'amount' };
  const aov = { parent: 'sales', child: 'aov' };

  it('asks the Backend for the fields, with the page, and answers what it answers', async () => {
    const { stub, sdk: anfra } = sdk();
    const controller = new AbortController();
    await expect(anfra.lineage!([amount, aov], { limit: 5, offset: 10, signal: controller.signal }))
      .resolves.toEqual(traced);
    expect(stub.calls).toEqual([{
      method: 'lineage', request: { targets: [amount, aov], limit: 5, offset: 10 }, signal: expect.any(AbortSignal),
    }]);
  });

  it('takes one field alone, and sends no page it was not given', async () => {
    const { stub, sdk: anfra } = sdk();
    await anfra.lineage!(amount);
    expect(stub.calls[0]).toMatchObject({ method: 'lineage', request: { targets: [amount] } });
    expect(stub.calls[0].request).not.toHaveProperty('limit');
  });

  it('takes a parent alone, sending no child', async () => {
    const { stub, sdk: anfra } = sdk();
    await anfra.lineage!([{ parent: 'orders' }, amount]);
    // Strict, so a `child: undefined` key fails it.
    expect(stub.calls[0].request).toStrictEqual({ targets: [{ parent: 'orders' }, amount] });
  });

  it.each([
    [[]], ['orders.amount'], [['orders.amount']], [{ child: 'amount' }], [{ parent: ' ' }],
    [[amount, { parent: 'orders', child: ' ' }]], [{ parent: 'orders', child: null }],
  ])('refuses %j, asking nothing', async (targets) => {
    const { stub, sdk: anfra } = sdk();
    await expect(anfra.lineage!(targets as never)).rejects.toMatchObject({ code: 'invalid_request', scope: 'client' });
    expect(stub.calls).toEqual([]);
  });

  it('is absent where the host did not install the plugin', () => {
    const frame = testFrame();
    expect(frame.anfra.plugins.has('lineage')).toBe(false);
    expect(frame.anfra.lineage).toBeUndefined();
  });
});
