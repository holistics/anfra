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
  function api (status: number, body: unknown) {
    const sent: unknown[] = [];
    const fetchImpl = vi.fn(async (input: Request) => {
      sent.push(await input.clone().json());
      return new Response(JSON.stringify(body), { status, headers: { 'Content-Type': 'application/json' } });
    });
    return { client: coreClient('http://host/api', fetchImpl as unknown as typeof fetch), sent };
  }

  it('shows the repo once, and keys its datasets by fqn', async () => {
    const { client, sent } = api(200, {
      object: { kind: 'repo', datasets: [sales] },
      diagnostics: [{ filePath: 'x.aml', message: 'broken' }],
    });
    const loaded = await loadDatasets(client);
    expect(sent).toEqual([{}]);
    expect(Object.keys(loaded.datasets)).toEqual(['shop.sales']);
    expect(loaded.diagnostics).toEqual([{ filePath: 'x.aml', message: 'broken' }]);
  });

  it('leaves out a dataset shown only in outline, whose diagnostic says why', async () => {
    const why = { message: 'Dataset "shop.broken" can\'t be shown in full: boom' };
    const { client } = api(200, {
      object: { kind: 'repo', datasets: [sales, { kind: 'dataset', fqn: 'shop.broken', name: 'broken' }] },
      diagnostics: [why],
    });
    const loaded = await loadDatasets(client);
    expect(Object.keys(loaded.datasets)).toEqual(['shop.sales']);
    expect(loaded.diagnostics).toEqual([why]);
  });

  it('fails as the API does', async () => {
    const { client } = api(500, { error: { code: 'internal_server_error', scope: 'server', message: 'Something went wrong.' } });
    await expect(loadDatasets(client)).rejects.toThrow('Something went wrong.');
  });
});
