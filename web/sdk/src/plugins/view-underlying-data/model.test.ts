import { describe, expect, it } from 'vitest';
import type { RowsTarget } from '../../common/menu';
import type { ColumnMeta, DatasetDescriptor, QueryRequest } from '../../common/types';
import { numberTypes, textTypes } from '../../app/testSupport';
import {
  aggregateFunction, candidates, NO_DATA, plan, preambleOf, visibleDimensions,
} from './model';

const dataset: DatasetDescriptor = {
  id: 'shop',
  name: 'shop',
  data_models: [
    {
      id: 'orders',
      name: 'orders',
      label: 'Orders',
      fields: [
        { name: 'id', label: 'Id', type: 'number', is_custom_measure: false },
        { name: 'status', label: 'Status', type: 'text', is_custom_measure: false },
        { name: 'amount', label: 'Amount', type: 'number', is_custom_measure: false },
        { name: 'secret', label: 'Secret', type: 'text', is_custom_measure: false, is_hidden: true },
        { name: 'cutoff', type: 'date', is_custom_measure: false, is_param: true },
        { name: 'total_orders', label: 'Total Orders', type: 'number', is_custom_measure: true },
      ],
    },
    {
      id: 'users',
      name: 'users',
      fields: [{ name: 'region', label: 'Region', type: 'text', is_custom_measure: false }],
    },
  ],
  metrics: [{ name: 'aov', type: 'number' }],
};

const region: ColumnMeta = {
  name: 'region', fieldName: 'region', modelId: 'users', label: 'Region', adhoc: false, isMeasure: false, ...textTypes,
};
const revenue: ColumnMeta = {
  name: 'revenue', fieldName: 'amount', modelId: 'orders', label: 'revenue', adhoc: false, isMeasure: true, aggregation: 'sum', ...numberTypes,
};
const orders: ColumnMeta = {
  name: 'orders', fieldName: 'total_orders', modelId: 'orders', label: 'Total Orders', adhoc: false, isMeasure: true, aggregation: 'count', ...numberTypes,
};
const aov: ColumnMeta = {
  name: 'aov', fieldName: 'aov', label: 'AOV', adhoc: false, isMeasure: true, ...numberTypes,
};
const ratio: ColumnMeta = {
  name: 'ratio', fieldName: 'ratio', label: 'ratio', adhoc: true, isMeasure: true, ...numberTypes,
};

const target = (over: Partial<RowsTarget> = {}): RowsTarget => ({
  kind: 'rows',
  app: 0,
  query: 'q',
  rows: [{ region: 'APAC', revenue: '10', orders: '2', aov: '5' }],
  fields: ['region'],
  conditions: [{ field: 'users.region', operator: 'is', values: ['APAC'] }],
  ...over,
});

describe('aggregateFunction', () => {
  it('spells an aggregation as its AQL function, and has none for a custom one', () => {
    expect(aggregateFunction('sum')).toBe('sum');
    expect(aggregateFunction('count distinct')).toBe('count_distinct');
    expect(aggregateFunction('approx count distinct')).toBe('approx_count_distinct');
    expect(aggregateFunction('custom')).toBeUndefined();
  });
});

describe('candidates', () => {
  const columns = [region, revenue, orders, aov, ratio];

  it('offers each measure that names a model, and none that doesn\'t', () => {
    expect(candidates(target(), columns, dataset).map((c) => [c.column.name, c.model.name, c.expression])).toEqual([
      ['revenue', 'orders', 'sum(orders.amount)'],
      ['orders', 'orders', 'orders.total_orders'],
    ]);
  });

  it('offers only the measure the definition named', () => {
    expect(candidates(target({ measure: 'orders' }), columns, dataset).map((c) => c.column.name)).toEqual(['orders']);
    expect(candidates(target({ measure: 'aov' }), columns, dataset)).toEqual([]);
  });

  it('says why when the point has no value for the measure', () => {
    const [only] = candidates(target({ measure: 'revenue', rows: [{ region: 'APAC', revenue: null }] }), columns, dataset);
    expect(only.disabled).toBe(NO_DATA);
  });

  it('offers nothing for a custom aggregate or a model the dataset doesn\'t have', () => {
    expect(candidates(target(), [{ ...revenue, aggregation: 'custom' }], dataset)).toEqual([]);
    expect(candidates(target(), [{ ...revenue, modelId: 'refunds' }], dataset)).toEqual([]);
  });
});

describe('plan', () => {
  const base: QueryRequest = {
    dataset: 'shop',
    aql: 'explore { dimensions { region: users.region } measures { revenue: sum(orders.amount) } }',
    input: {
      filters: [{ field: 'orders.status', operator: 'is', values: ['paid'] }],
      conditions: [{ expr: 'orders.amount > 0' }],
      sorts: [{ field: 'revenue', direction: 'desc' }],
      dateDrills: [{ field: 'orders.created_at', grain: 'month' }],
    },
    timezone: 'Asia/Singapore',
  };

  it('is the measure and the base model\'s visible dimensions, where the point and the query\'s inputs hold', () => {
    const [candidate] = candidates(target({ measure: 'revenue' }), [region, revenue], dataset);
    const planned = plan(target({ measure: 'revenue' }), candidate, base, [region, revenue], dataset);
    expect(visibleDimensions(candidate.model).map((field) => field.name)).toEqual(['id', 'status', 'amount']);
    expect(planned).toEqual({
      title: 'View underlying data: revenue',
      chips: [{ text: 'Region is APAC' }],
      columns: [
        { name: 'revenue', label: 'revenue', isMeasure: true },
        { name: 'id', label: 'Id', isMeasure: false },
        { name: 'status', label: 'Status', isMeasure: false },
        { name: 'amount', label: 'Amount', isMeasure: false },
      ],
      request: {
        dataset: 'shop',
        aql: [
          'explore {',
          '  dimensions {',
          '    id: orders.id,',
          '    status: orders.status,',
          '    amount: orders.amount',
          '  }',
          '  measures {',
          '    revenue: sum(orders.amount)',
          '  }',
          '}',
        ].join('\n'),
        input: {
          filters: [
            { field: 'orders.status', operator: 'is', values: ['paid'] },
            { field: 'users.region', operator: 'is', values: ['APAC'] },
          ],
          conditions: [{ expr: 'orders.amount > 0' }],
          sorts: [
            { field: 'id', direction: 'asc' },
            { field: 'status', direction: 'asc' },
            { field: 'amount', direction: 'asc' },
          ],
          dateDrills: [],
        },
        timezone: 'Asia/Singapore',
      },
    });
  });

  it('keeps the measure\'s alias apart from a dimension of the same name', () => {
    const amount = { ...revenue, name: 'amount' };
    const [candidate] = candidates(target({ measure: 'amount', rows: [{ amount: '1' }] }), [amount], dataset);
    const planned = plan(target({ measure: 'amount' }), candidate, base, [amount], dataset);
    expect(planned.columns.map((column) => column.name)).toEqual(['amount', 'id', 'status', 'amount_2']);
    expect(planned.request.aql).toContain('amount_2: orders.amount');
  });

  it('carries a point\'s expression as a condition, and warns of a dimension it can\'t filter by', () => {
    const bucket: ColumnMeta = {
      name: 'bucket', fieldName: 'bucket', label: 'bucket', adhoc: true, isMeasure: false, ...textTypes,
    };
    const point = target({ measure: 'revenue', conditions: [], expression: '(users.region == "APAC")' });
    const [candidate] = candidates(point, [region, bucket, revenue], dataset);
    const planned = plan(point, candidate, base, [region, bucket, revenue], dataset);
    expect(planned.chips).toEqual([{ text: 'Matches the selected point' }, { text: 'Not filtered by bucket', warning: true }]);
    expect(planned.request.input.conditions).toEqual([{ expr: 'orders.amount > 0' }, { expr: '(users.region == "APAC")' }]);
  });

  it('keeps what the query declares before its explore, which the point\'s conditions may name', () => {
    expect(preambleOf('explore { measures { n: count(orders.id) } }')).toBe('');
    const declared = {
      ...base,
      aql: 'dimension orders.kind = case(when: orders.amount > 100, then: "Big", else: "Small");\nexplore { dimensions { kind: orders.kind } measures { revenue: sum(orders.amount) } }',
    };
    const [candidate] = candidates(target({ measure: 'revenue' }), [region, revenue], dataset);
    const { aql } = plan(target({ measure: 'revenue' }), candidate, declared, [region, revenue], dataset).request;
    expect(aql.startsWith('dimension orders.kind = case(when: orders.amount > 100, then: "Big", else: "Small");\nexplore {')).toBe(true);
  });

  it('sends a condition on a field the query declares itself as AQL, which compiles with its declarations', () => {
    const kind: ColumnMeta = {
      name: 'kind', fieldName: 'kind', modelId: 'orders', label: 'kind', adhoc: false, isMeasure: false, ...textTypes,
    };
    const point = target({
      measure: 'revenue',
      conditions: [
        { field: 'orders.kind', operator: 'is', values: ['Big'] },
        { field: 'users.region', operator: 'is', values: ['APAC'] },
      ],
    });
    const [candidate] = candidates(point, [kind, region, revenue], dataset);
    const { input } = plan(point, candidate, base, [kind, region, revenue], dataset).request;
    expect(input.filters).toEqual([
      { field: 'orders.status', operator: 'is', values: ['paid'] },
      { field: 'users.region', operator: 'is', values: ['APAC'] },
    ]);
    expect(input.conditions).toEqual([{ expr: 'orders.amount > 0' }, { expr: 'orders.kind is "Big"' }]);
  });

  it('sorts by every dimension, dates first and newest first, so pages don\'t repeat rows', () => {
    const dated: DatasetDescriptor = {
      ...dataset,
      data_models: [{
        ...dataset.data_models[0],
        fields: [...dataset.data_models[0].fields, { name: 'created_at', label: 'Created At', type: 'datetime', is_custom_measure: false }],
      }, dataset.data_models[1]],
    };
    const [candidate] = candidates(target({ measure: 'revenue' }), [region, revenue], dated);
    expect(plan(target({ measure: 'revenue' }), candidate, base, [region, revenue], dated).request.input.sorts).toEqual([
      { field: 'created_at', direction: 'desc' },
      { field: 'id', direction: 'asc' },
      { field: 'status', direction: 'asc' },
      { field: 'amount', direction: 'asc' },
    ]);
  });

  it('says what an empty value and a date range are', () => {
    const point = target({
      measure: 'revenue',
      conditions: [
        { field: 'users.region', operator: 'is_null', values: [] },
        { field: 'orders.created_at', operator: 'matches', values: ['2026-01-01 until 2026-02-01'] },
      ],
    });
    const [candidate] = candidates(point, [region, revenue], dataset);
    expect(plan(point, candidate, base, [region, revenue], dataset).chips).toEqual([
      { text: 'Region is empty' },
      { text: 'created_at in 2026-01-01 until 2026-02-01' },
    ]);
  });
});
