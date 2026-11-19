import { describe, expect, it } from 'vitest';
import type { RowsTarget } from '../../common/menu';
import type { ColumnMeta, DatasetDescriptor, QueryRequest } from '../../common/types';
import { numberTypes, textTypes } from '../../app/testSupport';
import {
  candidates, entryLabel, NO_DATA, plan, visibleDimensions,
} from './model';
import { preambleOf } from './aqlText';

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
  name: 'revenue', fieldName: 'amount', modelId: 'orders', label: 'revenue', adhoc: false, isMeasure: true, aggregation: 'sum',
  models: ['orders'], expression: 'sum(orders.amount)', ...numberTypes,
};
const orders: ColumnMeta = {
  name: 'orders', fieldName: 'total_orders', modelId: 'orders', label: 'Total Orders', adhoc: false, isMeasure: true, aggregation: 'count',
  models: ['orders'], expression: 'orders.total_orders', ...numberTypes,
};
const aov: ColumnMeta = {
  name: 'aov', fieldName: 'aov', label: 'AOV', adhoc: false, isMeasure: true, ...numberTypes,
};
const ratio: ColumnMeta = {
  name: 'ratio', fieldName: 'ratio', label: 'ratio', adhoc: true, isMeasure: true, ...numberTypes,
};
/** A dataset metric, as anfra-node describes it: its models, and its AQL. */
const aovMetric: ColumnMeta = { ...aov, models: ['orders'], expression: 'aov' };
/** A ratio over two models. */
const perUser: ColumnMeta = {
  ...ratio, name: 'per_user', fieldName: 'per_user', label: 'Per User', models: ['orders', 'users'], expression: 'count(orders.id) * 1.0 / count(users.region)',
};
/** A filtered aggregate of one model field. */
const paid: ColumnMeta = {
  ...ratio, name: 'paid', fieldName: 'paid', label: 'paid', models: ['orders'], expression: "count(orders.id) | where(orders.status == 'paid')",
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

describe('candidates', () => {
  const columns = [region, revenue, orders, aov, ratio];

  it('offers each measure anfra-node gives models for, with its AQL, and none it doesn\'t', () => {
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

  it('offers nothing for a model the dataset doesn\'t have', () => {
    expect(candidates(target(), [{ ...revenue, models: ['refunds'] }], dataset)).toEqual([]);
  });
});

describe('candidates of a metric, a ratio or a filtered measure', () => {
  it('offers a dataset metric on the model it aggregates, its AQL from expression', () => {
    expect(candidates(target({ measure: 'aov' }), [region, aovMetric], dataset).map((c) => [c.column.name, c.model.name, c.expression]))
      .toEqual([['aov', 'orders', 'aov']]);
  });

  it('offers one candidate per model of a measure over several, in the order of its models', () => {
    const point = target({ measure: 'per_user', rows: [{ region: 'APAC', per_user: '1.5' }] });
    expect(candidates(point, [region, perUser], dataset).map((c) => [c.model.name, c.expression])).toEqual([
      ['orders', 'count(orders.id) * 1.0 / count(users.region)'],
      ['users', 'count(orders.id) * 1.0 / count(users.region)'],
    ]);
  });

  it('offers a filtered aggregate, an adhoc measure', () => {
    const point = target({ measure: 'paid', rows: [{ region: 'APAC', paid: '4' }] });
    expect(candidates(point, [region, paid], dataset).map((c) => [c.model.name, c.expression]))
      .toEqual([['orders', "count(orders.id) | where(orders.status == 'paid')"]]);
  });

  it('offers a filtered aggregate written model first with its whole AQL, though its column names a field and an aggregation', () => {
    // As anfra-node 0.16.0 describes it: the field and aggregation of the aggregate after the pipe.
    const filtered: ColumnMeta = {
      ...revenue, name: 'paid_revenue', label: 'paid_revenue', models: ['orders'],
      expression: "orders | where(orders.status == 'paid') | sum(orders.amount)",
    };
    const point = target({ measure: 'paid_revenue', rows: [{ region: 'APAC', paid_revenue: '4' }] });
    expect(candidates(point, [region, filtered], dataset).map((c) => [c.model.name, c.expression]))
      .toEqual([['orders', "orders | where(orders.status == 'paid') | sum(orders.amount)"]]);
  });

  it('offers nothing for a measure with no models, and skips a model the dataset doesn\'t have', () => {
    expect(candidates(target({ measure: 'aov' }), [region, { ...aovMetric, models: [] }], dataset)).toEqual([]);
    expect(candidates(target({ measure: 'aov' }), [region, { ...aovMetric, expression: undefined }], dataset)).toEqual([]);
    const point = target({ measure: 'per_user', rows: [{ per_user: '1' }] });
    expect(candidates(point, [{ ...perUser, models: ['refunds', 'users'] }], dataset).map((c) => c.model.name)).toEqual(['users']);
  });
});

describe('entryLabel', () => {
  it('is the bare entry for one candidate, names the measure among several, and the model of a measure over several', () => {
    const point = target({ rows: [{ region: 'APAC', revenue: '10', per_user: '1.5' }] });
    const one = candidates(target({ measure: 'revenue' }), [region, revenue], dataset);
    expect(one.map((c) => entryLabel(c, one))).toEqual(['View underlying data']);
    const several = candidates(point, [region, revenue, perUser], dataset);
    expect(several.map((c) => entryLabel(c, several))).toEqual([
      'View underlying data: revenue',
      'View underlying data: Per User (Orders)',
      'View underlying data: Per User (users)',
    ]);
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
      timezone: 'Asia/Singapore',
    },
  };

  it('is the base model\'s visible dimensions and the measure, where the point and the query\'s inputs hold, its value in the title', () => {
    const [candidate] = candidates(target({ measure: 'revenue' }), [region, revenue], dataset);
    const planned = plan(target({ measure: 'revenue' }), candidate, base, [region, revenue], dataset);
    expect(visibleDimensions(candidate.model).map((field) => field.name)).toEqual(['id', 'status', 'amount']);
    expect(planned).toEqual({
      title: `View underlying data: ${(10).toLocaleString()} revenue`,
      chips: [{ text: 'Region is APAC' }],
      columns: [
        { name: 'id', label: 'Id', numeric: true },
        { name: 'status', label: 'Status', numeric: false },
        { name: 'amount', label: 'Amount', numeric: true },
        { name: 'revenue', label: 'revenue', numeric: true },
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
          '    revenue: sum(orders.amount),',
          '    record_count: count(orders.id)',
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
          timezone: 'Asia/Singapore',
        },
      },
    });
  });

  it('shows the point\'s value in the title only when the point is one row', () => {
    const point = target({ measure: 'revenue', rows: [{ region: 'APAC', revenue: '1234.5' }, { region: 'EMEA', revenue: '10' }] });
    const [candidate] = candidates(point, [region, revenue], dataset);
    expect(plan(point, candidate, base, [region, revenue], dataset).title).toBe('View underlying data: revenue');
    const one = target({ measure: 'revenue', rows: [{ region: 'APAC', revenue: '1234.5' }] });
    expect(plan(one, candidate, base, [region, revenue], dataset).title).toBe(`View underlying data: ${(1234.5).toLocaleString(undefined, { maximumFractionDigits: 20 })} revenue`);
  });

  it('is the measure alone for a model with no visible dimension', () => {
    const bare: DatasetDescriptor = {
      ...dataset,
      data_models: [{
        id: 'events', name: 'events', fields: [
          { name: 'id', type: 'number', is_custom_measure: false, is_hidden: true },
          { name: 'n', label: 'Events', type: 'number', is_custom_measure: true },
        ],
      }],
    };
    const n: ColumnMeta = { ...orders, name: 'n', modelId: 'events', fieldName: 'n', label: 'Events', models: ['events'], expression: 'events.n' };
    const point = target({ measure: 'n', rows: [{ n: '3' }] });
    const [candidate] = candidates(point, [n], bare);
    const planned = plan(point, candidate, base, [n], bare);
    expect(planned.columns).toEqual([{ name: 'n', label: 'Events', numeric: true }]);
    expect(planned.request.aql).toBe(['explore {', '  measures {', '    n: events.n', '  }', '}'].join('\n'));
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

  it('keeps the query\'s own filters, and leaves out and warns of one that names a column by its alias', () => {
    const filtered = {
      ...base,
      aql: `explore {
        dimensions { region: users.region }
        measures { revenue: sum(orders.amount) }
        filters {
          orders.status == 'paid', // only paid
          region != 'EMEA',
        }
      }`,
    };
    const [candidate] = candidates(target({ measure: 'revenue' }), [region, revenue], dataset);
    const planned = plan(target({ measure: 'revenue' }), candidate, filtered, [region, revenue], dataset);
    expect(planned.request.aql.endsWith([
      '    record_count: count(orders.id)',
      '  }',
      '  filters {',
      "    orders.status == 'paid'",
      '  }',
      '}',
    ].join('\n'))).toBe(true);
    expect(planned.chips).toEqual([{ text: 'Region is APAC' }, { text: "Not filtered by region != 'EMEA'", warning: true }]);
  });

  it('warns of each dimension the point has no condition on, whether query-local or left out by fields', () => {
    const category: ColumnMeta = {
      name: 'category', fieldName: 'status', modelId: 'orders', label: 'Category', adhoc: false, isMeasure: false, ...textTypes,
    };
    const [candidate] = candidates(target({ measure: 'revenue' }), [region, category, revenue], dataset);
    expect(plan(target({ measure: 'revenue' }), candidate, base, [region, category, revenue], dataset).chips).toEqual([
      { text: 'Region is APAC' },
      { text: 'Not filtered by Category', warning: true },
    ]);
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

  // The count keeps each row: an explore leaves out a row where every measure is empty, as a
  // filtered measure is on a row its filter excludes.
  it('shows a metric\'s column on every row, recomputed per row of its model, and keeps a row where it is empty', () => {
    const point = target({ measure: 'aov' });
    const [candidate] = candidates(point, [region, aovMetric], dataset);
    const planned = plan(point, candidate, base, [region, aovMetric], dataset);
    expect(planned.columns).toEqual([
      { name: 'id', label: 'Id', numeric: true },
      { name: 'status', label: 'Status', numeric: false },
      { name: 'amount', label: 'Amount', numeric: true },
      { name: 'aov', label: 'AOV', numeric: true },
    ]);
    expect(planned.request.aql).toBe([
      'explore {',
      '  dimensions {',
      '    id: orders.id,',
      '    status: orders.status,',
      '    amount: orders.amount',
      '  }',
      '  measures {',
      '    aov: aov,',
      '    record_count: count(orders.id)',
      '  }',
      '}',
    ].join('\n'));
    expect(planned.request.input.sorts.map((sort) => sort.field)).toEqual(['id', 'status', 'amount']);
  });

  it('names the model in the title of a measure over several models', () => {
    const point = target({ measure: 'per_user', rows: [{ region: 'APAC', per_user: '1.5' }] });
    const [, users] = candidates(point, [region, perUser], dataset);
    const planned = plan(point, users, base, [region, perUser], dataset);
    expect(planned.title).toBe(`View underlying data: ${(1.5).toLocaleString()} Per User (users)`);
    expect(planned.request.aql).toContain('    region: users.region\n  }\n  measures {\n    per_user: count(orders.id) * 1.0 / count(users.region),\n    record_count: count(users.region)\n  }');
  });

  it('drills a query-local metric: the plan keeps the declarations its expression compiles with', () => {
    const local: ColumnMeta = {
      ...ratio, name: 'big', fieldName: 'big', label: 'big', models: ['orders'], expression: 'big',
    };
    const declared = {
      ...base,
      aql: 'metric big = count(orders.id) | where(orders.amount > 100);\nexplore { dimensions { region: users.region } measures { big } }',
    };
    const point = target({ measure: 'big', rows: [{ region: 'APAC', big: '2' }] });
    const [candidate] = candidates(point, [region, local], dataset);
    const { aql } = plan(point, candidate, declared, [region, local], dataset).request;
    expect(aql.startsWith('metric big = count(orders.id) | where(orders.amount > 100);\nexplore {')).toBe(true);
    expect(aql).toContain('  measures {\n    big: big,\n    record_count: count(orders.id)\n  }');
  });

  it('reads a dimension the query gives bare by its model->field key, filtering by it and keeping a filter on its field', () => {
    const bare: ColumnMeta = { ...region, name: 'users->region' };
    const point = target({ measure: 'revenue', rows: [{ 'users->region': 'APAC', revenue: '10' }], fields: ['users->region'] });
    const filtered = { ...base, aql: "explore { dimensions { users.region } measures { revenue: sum(orders.amount) } filters { users.region != 'EMEA' } }" };
    const [candidate] = candidates(point, [bare, revenue], dataset);
    const planned = plan(point, candidate, filtered, [bare, revenue], dataset);
    expect(planned.chips).toEqual([{ text: 'Region is APAC' }]);
    expect(planned.request.aql).toContain("  filters {\n    users.region != 'EMEA'\n  }");
  });
});
