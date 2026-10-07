import { describe, expect, it, vi } from 'vitest';
import { coreClient } from './client';
import { loadDatasets, toDescriptor } from './datasets';
import type { components } from './schema';

type ShowDataset = components['schemas']['ShowDataset'];

const sales: ShowDataset = {
  kind: 'dataset',
  fqn: 'shop.sales',
  name: 'sales',
  label: 'Sales',
  models: [{
    fqn: 'shop.orders',
    name: 'orders',
    label: 'Orders',
    fields: [
      { fqn: 'shop.orders.status', name: 'status', label: 'Status', role: 'dimension', type: 'text', hidden: false },
      { fqn: 'shop.orders.total', name: 'total', role: 'measure', type: 'number', aggregation: 'sum', hidden: false },
      { fqn: 'shop.sales.big', name: 'big', role: 'dimension', type: 'truefalse', hidden: false, definedInDataset: true },
    ],
  }],
  metrics: [{ fqn: 'shop.sales.revenue', name: 'revenue', label: 'Revenue', role: 'metric', type: 'number', hidden: false }],
};

describe('toDescriptor', () => {
  it('names a dataset and its models by fqn, as queries do', () => {
    expect(toDescriptor(sales)).toEqual({
      id: 'shop.sales',
      name: 'shop.sales',
      label: 'Sales',
      data_models: [{
        id: 'shop.orders',
        name: 'shop.orders',
        label: 'Orders',
        fields: [
          { name: 'status', label: 'Status', type: 'text', is_custom_measure: false },
          { name: 'total', type: 'number', is_custom_measure: true },
          { name: 'big', type: 'truefalse', is_custom_measure: false, defined_in_dataset: true },
        ],
      }],
      metrics: [{ name: 'revenue', label: 'Revenue', type: 'number' }],
    });
  });
});

describe('loadDatasets', () => {
  it('shows the repo, then each of its datasets, and keys them by fqn', async () => {
    const sent: unknown[] = [];
    const fetchImpl = vi.fn(async (input: Request) => {
      const body = await input.clone().json() as { fqn?: string };
      sent.push(body);
      const answer = body.fqn
        ? { object: sales, diagnostics: [] }
        : { object: { kind: 'repo', datasets: [{ kind: 'dataset', fqn: 'shop.sales', name: 'sales' }] }, diagnostics: [{ filePath: 'x.aml', message: 'broken' }] };
      return new Response(JSON.stringify(answer), { status: 200, headers: { 'Content-Type': 'application/json' } });
    });
    const loaded = await loadDatasets(coreClient('http://host/api', fetchImpl as unknown as typeof fetch));
    expect(sent).toEqual([{}, { fqn: 'shop.sales' }]);
    expect(Object.keys(loaded.datasets)).toEqual(['shop.sales']);
    expect(loaded.diagnostics).toEqual([{ filePath: 'x.aml', message: 'broken' }]);
  });
});
