import { describe, expect, test } from 'vitest';
import type {
  ColumnMeta, InspectedApp, InspectedStructure, LineageResult,
} from '../../common/types';
import {
  buildLineageGraph, filterGraph, focusGraph, hiddenCount, LINEAGE_PRESETS, lineageTargets, matchPreset,
  orderForLayout, pathOf, showsCrossFilters, targetKey, type LineageGraph,
} from './model';
import fixture from './fixtures/ecommerce-lineage.json';

// What core.lineage answered for ecommerce.revenue, ecommerce.order_count and orders.status.
const answered = fixture as unknown as LineageResult;

const column = (fieldName: string, modelId?: string, extra: Partial<ColumnMeta> = {}): ColumnMeta =>
  ({
    name: fieldName,
    fieldName,
    modelId,
    label: fieldName,
    adhoc: false,
    isMeasure: !modelId,
    ...extra,
  }) as ColumnMeta;

const app = {
  title: 'Sales',
  declaration: {
    queries: {
      revenueByStatus: { dataset: 'ecommerce', aql: '' },
      orders: { dataset: 'ecommerce', aql: '' },
      later: { dataset: 'ecommerce', aql: '' },
    },
    filters: {},
    dateDrills: {},
    mappings: [],
  },
  queries: {
    revenueByStatus: {
      columns: [column('status', 'orders'), column('revenue'), column('x', undefined, { adhoc: true })],
    },
    orders: { columns: [column('order_count')] },
    later: { columns: [] },
  },
} as unknown as InspectedApp;

const structure: InspectedStructure = {
  nodes: [
    {
      key: '0',
      kind: 'block',
      id: 'revenue',
      label: 'Revenue',
      problems: [],
      children: [{ key: '0.0', kind: 'query', app: 0, name: 'revenueByStatus', problems: [], children: [] }],
    },
    { key: '1', kind: 'block', id: 'kpis', problems: [], children: [] },
  ],
  usage: {
    'query:0/orders': { markers: 1, blocks: ['kpis'] },
    'query:0/later': { markers: 1, blocks: ['kpis'] },
  },
};

const traced = new Map(answered.results.map((r) => [targetKey(r.target), r]));

const graph = buildLineageGraph([app], structure, traced, { ecommerce: 'E-commerce' });
const ids = (g: LineageGraph) => g.nodes.map((n) => n.id).sort();
const edgeIds = (g: LineageGraph) => g.edges.map((e) => e.id).sort();

describe('lineage', () => {
  test('targets: each field once, ad hoc columns left out', () => {
    expect(lineageTargets([app])).toEqual([
      { parent: 'ecommerce', child: 'order_count' },
      { parent: 'ecommerce', child: 'revenue' },
      { parent: 'orders', child: 'status' },
    ]);
  });

  test('a control links to the queries it is mapped to, carrying their fields', () => {
    const withControl = {
      ...app,
      controls: {
        status: { condition: { values: ['paid'] }, appliedCondition: { values: ['paid'] }, isDirty: false },
        other: { condition: undefined },
      },
      declaration: {
        ...app.declaration,
        mappings: [
          {
            kind: 'control',
            from: { kind: 'filter', name: 'status' },
            to: { name: 'orders' },
            field: 'orders.status',
          },
          { kind: 'control', from: { kind: 'filter', name: 'gone' }, to: { name: 'missing' }, field: 'x' },
        ],
      },
    } as unknown as InspectedApp;
    const g = buildLineageGraph([withControl], structure, undefined);
    expect(g.nodes.find((n) => n.id === 'control:0/status')?.kind).toBe('control');
    expect(g.nodes.find((n) => n.id === 'control:0/status')?.isSet).toBe(true);
    expect(g.nodes.some((n) => n.id === 'control:0/gone')).toBe(false);
    expect(g.edges.find((e) => e.id === 'control:0/status->query:0/orders')?.fields).toEqual([
      'metric:ecommerce.order_count',
    ]);
  });

  describe('a control follows the applied condition', () => {
    const build = (control: Record<string, unknown>) => {
      const withControl = {
        ...app,
        controls: { status: control },
        declaration: {
          ...app.declaration,
          mappings: [
            {
              kind: 'control',
              from: { kind: 'filter', name: 'status' },
              to: { name: 'orders' },
              field: 'orders.status',
            },
          ],
        },
      } as unknown as InspectedApp;
      const g = buildLineageGraph([withControl], structure, undefined);
      return {
        card: g.nodes.find((n) => n.id === 'control:0/status'),
        edge: g.edges.find((e) => e.id === 'control:0/status->query:0/orders'),
      };
    };
    const paid = { values: ['paid'] };

    test('typed but not applied: not set, with the typed value pending', () => {
      const { card, edge } = build({ condition: paid, appliedCondition: undefined, isDirty: true });
      expect(card?.isSet).toBe(false);
      expect(card?.value).toBe('not set');
      expect(edge?.active).toBe(false);
      expect(card?.pending ?? '').toMatch(/paid/);
    });

    test('applied then cleared: still set, with the clearing pending', () => {
      const { card, edge } = build({ condition: undefined, appliedCondition: paid, isDirty: true });
      expect(card?.isSet).toBe(true);
      expect(edge?.active).toBe(true);
      expect(card?.value ?? '').toMatch(/paid/);
      expect(card?.pending).toBe('not set');
    });

    test('applied and clean: set, nothing pending', () => {
      const { card, edge } = build({ condition: paid, appliedCondition: paid, isDirty: false });
      expect(card?.isSet).toBe(true);
      expect(edge?.active).toBe(true);
      expect(card?.pending).toBe(undefined);
    });
  });

  test('a cross-filter links two queries, active while its source is selected', () => {
    const base = {
      ...app,
      declaration: {
        ...app.declaration,
        mappings: [
          { kind: 'crossFilter', id: 'x1', from: { name: 'orders' }, to: { name: 'orders' } },
          { kind: 'crossFilter', id: 'x2', from: { name: 'orders' }, to: { name: 'missing' } },
        ],
      },
    };
    const idle = buildLineageGraph([base as unknown as InspectedApp], structure, undefined);
    const edge = idle.edges.find((e) => e.kind === 'crossFilter');
    expect(edge?.id).toBe('crossFilter:0/x1');
    expect(edge?.active).toBe(false);
    expect(idle.edges.filter((e) => e.kind === 'crossFilter').length).toBe(1);

    const applied = { ...base, appliedSelection: { source: 'orders' } } as unknown as InspectedApp;
    const live = buildLineageGraph([applied], structure, undefined);
    expect(live.edges.find((e) => e.kind === 'crossFilter')?.active).toBe(true);

    expect(filterGraph(live, { hidden: new Set(), crossFilters: false }).edges.some((e) => e.kind === 'crossFilter')).toBe(false);
    expect(filterGraph(live, { hidden: new Set(['block']), crossFilters: true }).edges.some(
        (e) => e.kind === 'crossFilter',
      )).toBe(true);
    expect(filterGraph(live, { hidden: new Set(['query']), crossFilters: true }).edges.some(
        (e) => e.kind === 'crossFilter',
      )).toBe(false);
  });

  test('layout order puts cross-filtered queries side by side', () => {
    const q = (id: string) => ({ id, kind: 'query' as const, label: id });
    const e = (source: string, target: string) => ({
      id: `${source}->${target}`,
      source,
      target,
      fields: [],
      kind: 'crossFilter' as const,
    });
    const nodes = [q('a'), q('b'), q('c'), { id: 'm', kind: 'metric' as const, label: 'm' }];
    const order = orderForLayout({ nodes, edges: [e('a', 'c')] }).map((n) => n.id);
    expect(order).toEqual(['a', 'c', 'b', 'm']);
    expect(orderForLayout({ nodes, edges: [] })).toBe(nodes);
  });

  test('presets match the view they set, and only that', () => {
    for (const p of LINEAGE_PRESETS)
      expect(matchPreset({ hidden: new Set(p.hidden), crossFilters: p.crossFilters })?.id).toBe(p.id);
    expect(matchPreset({ hidden: new Set(['table']), crossFilters: false })).toBe(undefined);
  });

  test('the hidden count counts only what the graph has', () => {
    const counts = { block: 2, query: 3, metric: 1 };
    // No cross-filters in the graph: turning them off hides nothing.
    expect(hiddenCount({ hidden: new Set(), crossFilters: false }, counts, 0)).toBe(0);
    // Cross-filters there and off: one hidden.
    expect(hiddenCount({ hidden: new Set(), crossFilters: false }, counts, 2)).toBe(1);
    // A hidden kind the graph has none of is not counted.
    expect(hiddenCount({ hidden: new Set(['table']), crossFilters: true }, counts, 2)).toBe(0);
    // Hiding queries hides their cross-filters too, toggle on or not.
    expect(hiddenCount({ hidden: new Set(['query']), crossFilters: true }, counts, 2)).toBe(2);
  });

  test('cross-filters are drawn only with the toggle and the queries', () => {
    expect(showsCrossFilters({ hidden: new Set(), crossFilters: true })).toBe(true);
    expect(showsCrossFilters({ hidden: new Set(), crossFilters: false })).toBe(false);
    expect(showsCrossFilters({ hidden: new Set(['query']), crossFilters: true })).toBe(false);
  });

  test('the page down to its tables, shared models and tables once', () => {
    expect(ids(graph)).toEqual([
      'block:kpis',
      'block:revenue',
      'dataset:ecommerce',
      'dimension:orders.status',
      'metric:ecommerce.order_count',
      'metric:ecommerce.revenue',
      'model:order_items',
      'model:orders',
      'model:products',
      'query:0/later',
      'query:0/orders',
      'query:0/revenueByStatus',
      'table:ecommerce_duckdb/public.order_items',
      'table:ecommerce_duckdb/public.orders',
      'table:ecommerce_duckdb/public.products',
    ]);
    const byId = new Map(graph.nodes.map((n) => [n.id, n]));
    expect(byId.get('model:orders')?.chips).toEqual(['id', 'status']);
    expect(byId.get('model:order_items')?.chips).toEqual(['quantity']);
    expect(byId.get('dataset:ecommerce')?.label).toBe('E-commerce');
    expect(byId.get('block:revenue')?.structureKey).toBe('0');
    expect(byId.get('query:0/revenueByStatus')?.structureKey).toBe('0.0');
  });

  test("a metric's path reaches the models it reads, not every model of its dataset", () => {
    const path = pathOf(graph, 'metric:ecommerce.revenue');
    expect([...path.nodes].sort()).toEqual([
      'block:revenue',
      'dataset:ecommerce',
      'metric:ecommerce.revenue',
      'model:order_items',
      'model:products',
      'query:0/revenueByStatus',
      'table:ecommerce_duckdb/public.order_items',
      'table:ecommerce_duckdb/public.products',
    ]);
  });

  test("a model's path goes up through the fields that read it", () => {
    const path = pathOf(graph, 'model:orders');
    expect([...path.nodes].sort()).toEqual([
      'block:kpis',
      'block:revenue',
      'dataset:ecommerce',
      'dimension:orders.status',
      'metric:ecommerce.order_count',
      'model:orders',
      'query:0/orders',
      'query:0/revenueByStatus',
      'table:ecommerce_duckdb/public.orders',
    ]);
  });

  describe('a cross-filter is not lineage', () => {
    // `orders` (order_count) cross-filters `revenueByStatus` (revenue, status): different metrics.
    const crossed = buildLineageGraph(
      [
        {
          ...app,
          declaration: {
            ...app.declaration,
            mappings: [
              {
                kind: 'crossFilter',
                id: 'x1',
                from: { name: 'orders' },
                to: { name: 'revenueByStatus' },
              },
            ],
          },
        } as unknown as InspectedApp,
      ],
      structure,
      traced,
    );

    test('pinning the source shows what it filters, not what that is built from', () => {
      const path = pathOf(crossed, 'query:0/orders');
      expect([...path.nodes].sort()).toEqual([
        'block:kpis',
        'dataset:ecommerce',
        'metric:ecommerce.order_count',
        'model:orders',
        'query:0/orders',
        'query:0/revenueByStatus',
        'table:ecommerce_duckdb/public.orders',
      ]);
      expect(path.edges.has('crossFilter:0/x1')).toBeTruthy();
    });

    test('pinning the target walks up through the cross-filter to its source', () => {
      const path = pathOf(crossed, 'query:0/revenueByStatus');
      expect(path.nodes.has('query:0/orders')).toBeTruthy();
      expect(path.edges.has('crossFilter:0/x1')).toBeTruthy();
      expect(path.nodes.has('metric:ecommerce.revenue')).toBeTruthy();
      expect(path.nodes.has('model:products')).toBeTruthy();
      expect(path.nodes.has('metric:ecommerce.order_count')).toBeFalsy();
    });
  });

  test('a query that has not run still links to its block', () => {
    expect(pathOf(graph, 'query:0/later').nodes.has('block:kpis')).toBeTruthy();
  });

  test('hiding a kind joins across it, keeping what each edge carries', () => {
    const shown = filterGraph(graph, { hidden: new Set(['query', 'dataset']), crossFilters: true });
    expect(ids(shown).some((id) => id.startsWith('query:') || id.startsWith('dataset:'))).toBeFalsy();
    expect(edgeIds(shown).includes('block:revenue->metric:ecommerce.revenue')).toBeTruthy();
    expect(edgeIds(shown).includes('metric:ecommerce.revenue->model:products')).toBeTruthy();
    expect(edgeIds(shown).includes('metric:ecommerce.revenue->model:orders')).toBeFalsy();
    expect(edgeIds(shown).includes('block:revenue->metric:ecommerce.order_count')).toBeFalsy();
  });

  describe('hiding a kind keeps a filter edge a filter edge', () => {
    const controlled = (condition: unknown) => {
      const withControl = {
        ...app,
        controls: { status: { condition, appliedCondition: condition, isDirty: false } },
        declaration: {
          ...app.declaration,
          mappings: [
            {
              kind: 'control',
              from: { kind: 'filter', name: 'status' },
              to: { name: 'orders' },
              field: 'orders.status',
            },
          ],
        },
      } as unknown as InspectedApp;
      return buildLineageGraph([withControl], structure, traced);
    };

    test('a control joins across a hidden query as a control, set or not', () => {
      for (const [condition, active] of [
        [{ values: ['paid'] }, true],
        [undefined, false],
      ] as const) {
        const shown = filterGraph(controlled(condition), { hidden: new Set(['query']), crossFilters: true });
        const joined = shown.edges.find((e) => e.id === 'control:0/status->metric:ecommerce.order_count');
        expect(joined?.kind).toBe('control');
        expect(joined?.active).toBe(active);
      }
    });

    test('a filter edge and a plain edge between the same nodes both survive', () => {
      const edge = (id: string, source: string, target: string, extra = {}) => ({
        id,
        source,
        target,
        fields: ['f'],
        ...extra,
      });
      const node = (id: string, kind: LineageGraph['nodes'][number]['kind']) => ({ id, kind, label: id });
      const crowded: LineageGraph = {
        nodes: [node('x', 'block'), node('mid', 'query'), node('mid2', 'query'), node('y', 'metric')],
        edges: [
          edge('x->mid', 'x', 'mid'),
          edge('mid->y', 'mid', 'y'),
          edge('x->mid2', 'x', 'mid2', { kind: 'control', active: true }),
          edge('mid2->y', 'mid2', 'y'),
        ],
      };
      const shown = filterGraph(crowded, { hidden: new Set(['query']), crossFilters: true });
      expect(shown.edges.map((e) => [e.id, e.kind]).sort()).toEqual([
          ['x->y', undefined],
          ['x->y:control', 'control'],
        ].sort());
      // The same, in the other order.
      const swapped = {
        ...crowded,
        edges: [...crowded.edges.slice(2), ...crowded.edges.slice(0, 2)],
      };
      expect(filterGraph(swapped, { hidden: new Set(['query']), crossFilters: true })
          .edges.map((e) => [e.id, e.kind])
          .sort()).toEqual([
          ['x->y', undefined],
          ['x->y:control', 'control'],
        ].sort());
    });

    test('two joined control edges merge: fields are the union, active the OR', () => {
      const edge = (id: string, source: string, target: string, fields: string[], active: boolean) => ({
        id,
        source,
        target,
        fields,
        kind: 'control' as const,
        active,
      });
      const node = (id: string, kind: LineageGraph['nodes'][number]['kind']) => ({ id, kind, label: id });
      const two: LineageGraph = {
        nodes: [node('c', 'control'), node('q1', 'query'), node('q2', 'query'), node('m', 'metric')],
        edges: [
          edge('c->q1', 'c', 'q1', ['a'], false),
          edge('c->q2', 'c', 'q2', ['b'], true),
          { id: 'q1->m', source: 'q1', target: 'm', fields: ['a'] },
          { id: 'q2->m', source: 'q2', target: 'm', fields: ['b'] },
        ],
      };
      const [joined, ...rest] = filterGraph(two, { hidden: new Set(['query']), crossFilters: true }).edges;
      expect(rest.length).toBe(0);
      expect(joined).toBeTruthy();
      expect(joined.id).toBe('c->m');
      expect([...joined.fields].sort()).toEqual(['a', 'b']);
      expect(joined.active).toBe(true);
    });
  });

  test('focus keeps the node and its path', () => {
    const focused = focusGraph(graph, 'metric:ecommerce.order_count');
    expect(ids(focused)).toEqual([
      'block:kpis',
      'dataset:ecommerce',
      'metric:ecommerce.order_count',
      'model:orders',
      'query:0/orders',
      'table:ecommerce_duckdb/public.orders',
    ]);
  });

  test('without the catalog, a dimension still reaches its model, a metric stops at its dataset', () => {
    const bare = buildLineageGraph([app], structure, undefined);
    expect(ids(bare).includes('model:orders')).toBeTruthy();
    expect(ids(bare).some((id) => id.startsWith('table:'))).toBeFalsy();
    expect(bare.edges.filter((e) => e.source === 'dataset:ecommerce').map((e) => e.target)).toEqual(['model:orders']);
  });
});
