/**
 * A dynamic app (a chart or dashboard builder) changing what it declared while it runs: editing a
 * query or a control in place, and removing one, or an edge, with what hangs off it.
 */
import {
  describe, expect, it, vi,
} from 'vitest';
import { createSdk } from './sdk';
import { ValidationError } from '../common/errors';
import {
  numberTypes, salesDataset, stubBackend, testUser, textTypes, type StubOptions,
} from './testSupport';
import type {
  BackendQueryRequest, BackendQueryResult, ColumnMeta, ConditionValue,
} from '../common/types';

const dim = (name: string, modelId: string): ColumnMeta => ({
  name, fieldName: name, modelId, label: name, adhoc: false, isMeasure: false, ...textTypes,
});
const measure = (name: string): ColumnMeta => ({
  name, fieldName: name, label: name, adhoc: false, isMeasure: true, aggregation: 'sum', ...numberTypes,
});

const AQL = {
  byRegion: '// byRegion\nexplore { dimensions { region: users.region } measures { total: orders | sum(orders.amount) } }',
  byStatus: '// byStatus\nexplore { dimensions { status: orders.status } measures { total: orders | sum(orders.amount) } }',
};

function shape (request: BackendQueryRequest): BackendQueryResult {
  if (request.aql.startsWith('// byStatus')) {
    return { columns: [dim('status', 'orders'), measure('total')], values: [['completed', 50], ['pending', 10]] };
  }
  return { columns: [dim('region', 'users'), measure('total')], values: [['APAC', 100], ['EMEA', 70]] };
}

function build (submitQuery: StubOptions['submitQuery'] = shape, fieldSuggestions?: StubOptions['fieldSuggestions']) {
  const stub = stubBackend({ submitQuery, fieldSuggestions });
  const sdk = createSdk({
    datasets: { sales: salesDataset, other: { ...salesDataset, id: 43, name: 'other' } },
    user: testUser,
    backend: stub.backend,
  });
  return { stub, app: sdk.createApp() };
}

/** A promise and what settles it, for a backend answer the test holds back. */
function deferred<T> () {
  let resolve!: (value: T) => void;
  const promise = new Promise<T>((r) => { resolve = r; });
  return { promise, resolve };
}

describe('editing a query', () => {
  it('swaps the declaration, keeps its result until the rerun, and only marks it dirty', async () => {
    const { stub, app } = build();
    const query = app.createQuery('chart', { dataset: 'sales', aql: AQL.byRegion });
    await app.execute();
    const before = query.result;

    query.update({ aql: AQL.byStatus });

    expect(query.declaration).toEqual({ dataset: 'sales', aql: AQL.byStatus });
    expect(query.result).toBe(before);
    expect(query.isDirty).toBe(true);
    expect(stub.queries).toHaveLength(1);

    await app.execute();
    expect(stub.queries).toHaveLength(2);
    expect(stub.queries[1].aql).toBe(AQL.byStatus);
    expect(query.result?.rows).toEqual([{ status: 'completed', total: 50 }, { status: 'pending', total: 10 }]);
  });

  it('keeps its edges, its plugin members and its name', async () => {
    const { app } = build();
    const query = app.createQuery('chart', { dataset: 'sales', aql: AQL.byRegion });
    const other = app.createQuery('other', { dataset: 'sales', aql: AQL.byStatus });
    const filter = app.createFilter('region', { field: 'users.region', dataset: 'sales' });
    app.mapControl(filter, query, { field: 'users.region' });
    app.mapCrossFilter(query, other);

    query.update({ aql: AQL.byStatus });

    expect(app.queries.chart).toBe(query);
    expect(app.mappings.map((edge) => edge.id)).toEqual(['region.chart.users.region', 'chart.other']);
  });

  it('goes back to the first page', async () => {
    const { stub, app } = build((request) => ({
      ...shape(request), meta: { hasMore: true },
    }));
    const query = app.createQuery('table', { dataset: 'sales', aql: AQL.byRegion, pageSize: 2 });
    await app.execute();
    await query.fetchMore();
    expect(query.page).toBe(2);

    query.update({ pageSize: 5 });
    expect(query.page).toBe(1);

    await app.execute();
    expect(stub.queries.at(-1)).toMatchObject({ input: { pagination: { page: 1, pageSize: 5 } } });
  });

  it('drops the input its last run came back with, so a page after an edit does not replay the old one', async () => {
    const { stub, app } = build((request) => ({
      ...shape(request), meta: { hasMore: true },
    }));
    const query = app.createQuery('table', { dataset: 'sales', aql: AQL.byRegion, pageSize: 2 });
    query.setSort([{ field: 'total', direction: 'desc' }]);
    await app.execute();
    expect(query.ranInput?.sorts).toHaveLength(1);

    query.update({ aql: AQL.byStatus });
    expect(query.ranInput).toBeUndefined();

    await app.execute();
    await query.fetchMore();
    const sent = stub.queries.at(-1);
    expect(sent?.aql).toBe(AQL.byStatus);
    expect(sent?.input.pagination).toEqual({ page: 2, pageSize: 2 });
    expect(sent?.input.sorts ?? []).toEqual([]);
  });

  it('has no more pages until the rerun: fetchMore sends nothing, and the rerun starts at page 1', async () => {
    const { stub, app } = build((request) => ({
      ...shape(request), meta: { hasMore: true },
    }));
    const query = app.createQuery('table', { dataset: 'sales', aql: AQL.byRegion, pageSize: 2 });
    await app.execute();
    const rows = query.result?.rows;

    query.update({ aql: AQL.byStatus });
    expect(query.hasMore).toBe(false);
    await query.fetchMore();

    expect(stub.queries).toHaveLength(1);
    expect(query.result?.rows).toBe(rows);
    expect(query.isDirty).toBe(true);

    await app.execute();
    expect(stub.queries).toHaveLength(2);
    expect(stub.queries[1]).toMatchObject({ aql: AQL.byStatus, input: { pagination: { page: 1, pageSize: 2 } } });
    expect(query.hasMore).toBe(true);
  });

  it('drops a run in flight: its late answer lands nothing, and the next execute runs the new AQL', async () => {
    let hold: ReturnType<typeof deferred<BackendQueryResult>> | undefined;
    const { stub, app } = build((request) => {
      if (stub.queries.length === 2) {
        hold = deferred<BackendQueryResult>();
        return hold.promise;
      }
      return shape(request);
    });
    const query = app.createQuery('chart', { dataset: 'sales', aql: AQL.byRegion });
    await app.execute();
    const before = query.result;
    const running = app.refresh();
    await vi.waitFor(() => expect(hold).toBeDefined());
    expect(query.state).toBe('executing');

    query.update({ aql: AQL.byStatus });
    expect(query.state).toBe('success');
    const listener = vi.fn();
    query.subscribe(listener);
    hold!.resolve(shape({ aql: AQL.byRegion } as BackendQueryRequest));

    expect(await running).toEqual({ succeeded: [], failed: [] });
    expect(query.result).toBe(before);
    expect(query.ranInput).toBeUndefined();
    expect(query.state).toBe('success');
    expect(listener).not.toHaveBeenCalled();
    expect(query.isDirty).toBe(true);

    await app.execute();
    expect(stub.queries.at(-1)?.aql).toBe(AQL.byStatus);
    expect(query.result?.rows).toEqual([{ status: 'completed', total: 50 }, { status: 'pending', total: 10 }]);
  });

  it('goes back to idle when its first run is in flight', async () => {
    const pending = deferred<BackendQueryResult>();
    const { app } = build(() => pending.promise);
    const query = app.createQuery('chart', { dataset: 'sales', aql: AQL.byRegion });
    const running = app.execute();
    expect(query.state).toBe('executing');

    query.update({ aql: AQL.byStatus });
    pending.resolve(shape({ aql: AQL.byRegion } as BackendQueryRequest));

    await running;
    expect(query.state).toBe('idle');
    expect(query.result).toBeUndefined();
  });

  it('drops a page in flight: the old AQL\'s page is not appended', async () => {
    let hold: ReturnType<typeof deferred<BackendQueryResult>> | undefined;
    const { stub, app } = build((request) => {
      if (request.input.pagination?.page === 2) {
        hold = deferred<BackendQueryResult>();
        return hold.promise;
      }
      return { ...shape(request), meta: { hasMore: true } };
    });
    const query = app.createQuery('table', { dataset: 'sales', aql: AQL.byRegion, pageSize: 2 });
    await app.execute();
    const more = query.fetchMore();
    await vi.waitFor(() => expect(hold).toBeDefined());

    query.update({ aql: AQL.byStatus });
    hold!.resolve({ ...shape({ aql: AQL.byRegion } as BackendQueryRequest), meta: { hasMore: true } });
    await more;

    expect(query.result?.rows).toHaveLength(2);
    expect(query.page).toBe(1);
    expect(query.state).toBe('success');
    expect(query.hasMore).toBe(false);

    await app.execute();
    expect(stub.queries.at(-1)).toMatchObject({ aql: AQL.byStatus, input: { pagination: { page: 1, pageSize: 2 } } });
    expect(query.result?.rows).toHaveLength(2);
    expect(query.hasMore).toBe(true);
  });

  it('validates the new declaration as createQuery does, and changes nothing when it fails', () => {
    const { app } = build();
    const query = app.createQuery('chart', { dataset: 'sales', aql: AQL.byRegion });

    expect(() => query.update({ aql: '  ' })).toThrowError(ValidationError);
    expect(() => query.update({ pageSize: 0 })).toThrowError(/whole number of at least 1/);
    expect(() => query.update({ dataset: 'nowhere' })).toThrowError(ValidationError);
    expect(query.declaration).toEqual({ dataset: 'sales', aql: AQL.byRegion });
  });

  it('drops the page size when given undefined', () => {
    const { app } = build();
    const query = app.createQuery('table', { dataset: 'sales', aql: AQL.byRegion, pageSize: 2 });

    query.update({ pageSize: undefined });

    expect(query.pageSize).toBeUndefined();
    expect(query.declaration).toEqual({ dataset: 'sales', aql: AQL.byRegion });
  });

  it('clears the sort and a selection it sourced when its AQL changes', async () => {
    const { app } = build();
    const query = app.createQuery('chart', { dataset: 'sales', aql: AQL.byRegion });
    const target = app.createQuery('target', { dataset: 'sales', aql: AQL.byStatus });
    app.mapCrossFilter(query, target);
    await app.execute();
    query.setSort([{ field: 'total', direction: 'desc' }]);
    query.select({ region: 'APAC', total: 100 });
    await app.execute();
    expect(app.appliedSelection?.source).toBe('chart');

    query.update({ aql: AQL.byStatus });

    expect(query.sort).toEqual([]);
    expect(app.selection).toBeUndefined();
    expect(app.appliedSelection).toBeUndefined();
    expect(target.isDirty).toBe(true);
  });

  it('takes the sort it is given in place of clearing it', () => {
    const { app } = build();
    const query = app.createQuery('chart', { dataset: 'sales', aql: AQL.byRegion });
    query.setSort([{ field: 'total' }]);

    query.update({ aql: AQL.byStatus, sort: [{ field: 'status', direction: 'asc' }] });

    expect(query.sort).toEqual([{ field: 'status', direction: 'asc' }]);
  });

  it('clears the sort and the selection when its dataset changes', async () => {
    const { app } = build();
    const query = app.createQuery('chart', { dataset: 'sales', aql: AQL.byRegion });
    await app.execute();
    query.setSort([{ field: 'total' }]);
    query.select({ region: 'APAC', total: 100 });

    query.update({ dataset: 'other' });

    expect(query.sort).toEqual([]);
    expect(app.selection).toBeUndefined();
    expect(query.dataset.descriptor.name).toBe('other');
  });

  it('keeps the sort and the selection when only its page size changes, or its AQL is the same', async () => {
    const { app } = build();
    const query = app.createQuery('chart', { dataset: 'sales', aql: AQL.byRegion });
    await app.execute();
    query.setSort([{ field: 'total' }]);
    query.select({ region: 'APAC', total: 100 });

    query.update({ pageSize: 10 });
    query.update({ aql: AQL.byRegion });

    expect(query.sort).toEqual([{ field: 'total' }]);
    expect(app.selection?.source).toBe('chart');
  });

  it('refuses a dataset change that would make a cross-filter span datasets, naming the edges', () => {
    const { app } = build();
    const query = app.createQuery('chart', { dataset: 'sales', aql: AQL.byRegion });
    const target = app.createQuery('target', { dataset: 'sales', aql: AQL.byStatus });
    const source = app.createQuery('source', { dataset: 'sales', aql: AQL.byStatus });
    app.mapCrossFilter(query, target);
    app.mapCrossFilter(source, query);
    query.setSort([{ field: 'total' }]);

    expect(() => query.update({ dataset: 'other', aql: AQL.byStatus })).toThrowError(
      /'chart\.target'.*'source\.chart'/,
    );
    expect(query.declaration).toEqual({ dataset: 'sales', aql: AQL.byRegion });
    expect(query.sort).toEqual([{ field: 'total' }]);
  });

  it('is one run for several edits, at the next app.execute()', async () => {
    const { stub, app } = build();
    const a = app.createQuery('a', { dataset: 'sales', aql: AQL.byRegion });
    const b = app.createQuery('b', { dataset: 'sales', aql: AQL.byRegion });
    await app.execute();

    a.update({ aql: AQL.byStatus });
    a.update({ pageSize: 3 });
    b.update({ aql: AQL.byStatus });
    expect(stub.queries).toHaveLength(2);

    await app.execute();
    expect(stub.queries).toHaveLength(4);
  });
});

describe('editing a control', () => {
  it('keeps the reader\'s condition when only the label, options or default change', async () => {
    const { app } = build();
    const filter = app.createFilter('status', { type: 'string', options: ['a', 'b'], label: 'Status' });
    filter.setCondition({ operator: 'is', values: ['a'] });
    await app.execute();
    filter.setCondition({ operator: 'is', values: ['b'] });

    filter.update({ label: 'State', options: ['a', 'b', 'c'], default: { operator: 'is', values: ['c'] } });

    expect(filter.condition).toEqual({ operator: 'is', values: ['b'] });
    expect(filter.appliedCondition).toEqual({ operator: 'is', values: ['a'] });
    expect(filter.declaration.label).toBe('State');
    expect(filter.options).toEqual(['a', 'b', 'c']);
    filter.reset();
    expect(filter.condition).toEqual({ operator: 'is', values: ['c'] });
  });

  it('re-resolves a changed field, resets the condition to the new default and drops loaded options', async () => {
    const { app } = build(shape, ['APAC', 'EMEA']);
    const query = app.createQuery('chart', { dataset: 'sales', aql: AQL.byRegion });
    const filter = app.createFilter('pick', { field: 'users.region', dataset: 'sales' });
    const mapping = app.mapControl(filter, query, { field: 'users.region' });
    await filter.loadOptions();
    filter.setCondition({ operator: 'is', values: ['APAC'] });
    await app.execute();

    filter.update({ field: 'orders.amount', default: { operator: 'is', values: ['5'] } });

    expect(filter.type).toBe('number');
    expect(filter.condition).toEqual({ operator: 'is', values: ['5'] });
    expect(filter.appliedCondition).toEqual({ operator: 'is', values: ['APAC'] });
    expect(filter.options).toEqual([]);
    expect(query.isDirty).toBe(true);
    expect(app.mappings).toEqual([mapping]);
    expect(mapping.field).toBe('users.region');
  });

  it('checks a changed declaration as createFilter does, and changes nothing when it fails', () => {
    const { app } = build();
    const filter = app.createFilter('pick', { field: 'users.region', dataset: 'sales' });

    expect(() => filter.update({ field: 'users.nope' })).toThrowError(ValidationError);
    expect(() => filter.update({ type: 'number', default: { operator: 'is', values: [5 as unknown as string] } }))
      .toThrowError(/give its values as strings/);
    expect(filter.declaration).toEqual({ field: 'users.region', dataset: 'sales' });
    expect(filter.type).toBe('string');
  });

  it('drops a loadOptions in flight when its field changes', async () => {
    const pending = deferred<ConditionValue[]>();
    const { app } = build(shape, () => pending.promise);
    const filter = app.createFilter('pick', { field: 'users.region', dataset: 'sales' });
    const listener = vi.fn();
    filter.subscribe(listener);
    const loading = filter.loadOptions();

    filter.update({ field: 'orders.status' });
    listener.mockClear();
    pending.resolve(['APAC']);

    await expect(loading).rejects.toMatchObject({ name: 'AbortError' });
    expect(filter.options).toEqual([]);
    expect(listener).not.toHaveBeenCalled();
  });

  it('keeps a date drill\'s grain when its label or default change, and resets to the new default', () => {
    const { app } = build();
    const drill = app.createDateDrill('grain', { default: 'month' });
    drill.setGrain('year');

    drill.update({ label: 'Grain', default: 'week' });

    expect(drill.condition).toEqual({ operator: 'transform_date_drill', values: ['datetrunc year'] });
    expect(drill.declaration).toEqual({ label: 'Grain', default: 'week' });
    drill.reset();
    expect(drill.condition).toEqual({ operator: 'transform_date_drill', values: ['datetrunc week'] });
    expect(() => drill.update({ default: 'decade' as never })).toThrowError(/Invalid grain/);
  });
});

describe('removing', () => {
  it('takes a query\'s edges with it, dirties their targets and frees its name', async () => {
    const { stub, app } = build();
    const query = app.createQuery('chart', { dataset: 'sales', aql: AQL.byRegion });
    const target = app.createQuery('target', { dataset: 'sales', aql: AQL.byStatus });
    const filter = app.createFilter('region', { field: 'users.region', dataset: 'sales' });
    app.mapControl(filter, query, { field: 'users.region' });
    app.mapCrossFilter(query, target);
    await app.execute();
    query.select({ region: 'APAC', total: 100 });
    await app.execute();
    expect(stub.queries.at(-1)?.input.filters).toEqual([{ field: 'users.region', operator: 'is', values: ['APAC'] }]);

    query.remove();

    expect(app.queries).not.toHaveProperty('chart');
    expect(app.mappings).toEqual([]);
    expect(app.selection).toBeUndefined();
    expect(app.appliedSelection).toBeUndefined();
    expect(target.isDirty).toBe(true);
    expect(app.controls.region).toBe(filter);
    await app.execute();
    expect(stub.queries.at(-1)?.input.filters).toEqual([]);
    expect(() => app.createQuery('chart', { dataset: 'sales', aql: AQL.byStatus })).not.toThrow();
  });

  it('takes a control\'s mappings with it, and its targets rerun without its condition', async () => {
    const { stub, app } = build();
    const query = app.createQuery('chart', { dataset: 'sales', aql: AQL.byRegion });
    const filter = app.createFilter('region', { field: 'users.region', dataset: 'sales' });
    app.mapControl(filter, query, { field: 'users.region' });
    filter.setCondition({ operator: 'is', values: ['APAC'] });
    await app.execute();

    filter.remove();

    expect(app.controls).toEqual({});
    expect(app.mappings).toEqual([]);
    expect(query.isDirty).toBe(true);
    await app.execute();
    expect(stub.queries.at(-1)?.input.filters).toEqual([]);
    expect(() => app.createFilter('region', { type: 'string' })).not.toThrow();
  });

  it('gives a query created again by its name a new id, in its stamp and its snapshot', async () => {
    const { stub, app } = build();
    const submit = vi.spyOn(stub.backend, 'submitQuery');
    const first = app.createQuery('chart', { dataset: 'sales', aql: AQL.byRegion });
    await app.execute();
    first.remove();
    const again = app.createQuery('chart', { dataset: 'sales', aql: AQL.byRegion });
    await app.execute();

    expect(again.id).not.toBe(first.id);
    expect(submit.mock.calls.map((call) => call[2])).toEqual([
      { app: 0, query: 'chart', id: first.id }, { app: 0, query: 'chart', id: again.id },
    ]);
    expect(app.toInspectJSON().queries.chart?.id).toBe(again.id);
  });

  it('aborts a query\'s run in flight and drops its late result without notifying', async () => {
    const pending = deferred<BackendQueryResult>();
    let seen: AbortSignal | undefined;
    // A backend that ignores the abort and answers anyway.
    const { app } = build((_request, signal) => {
      seen = signal;
      return pending.promise;
    });
    const query = app.createQuery('chart', { dataset: 'sales', aql: AQL.byRegion });
    const running = app.execute();
    await vi.waitFor(() => expect(seen).toBeDefined());
    const listener = vi.fn();
    query.subscribe(listener);

    query.remove();
    pending.resolve(shape({ aql: AQL.byRegion } as BackendQueryRequest));

    expect(await running).toEqual({ succeeded: [], failed: [] });
    expect(seen?.aborted).toBe(true);
    expect(query.result).toBeUndefined();
    expect(listener).not.toHaveBeenCalled();
  });

  it('aborts a fetchMore in flight', async () => {
    let hold: ReturnType<typeof deferred<BackendQueryResult>> | undefined;
    const signals: AbortSignal[] = [];
    const { app } = build((request, signal) => {
      signals.push(signal);
      if (request.input.pagination?.page === 2) {
        hold = deferred<BackendQueryResult>();
        return hold.promise;
      }
      return { ...shape(request), meta: { hasMore: true } };
    });
    const query = app.createQuery('table', { dataset: 'sales', aql: AQL.byRegion, pageSize: 2 });
    await app.execute();
    const more = query.fetchMore();
    await vi.waitFor(() => expect(hold).toBeDefined());

    query.remove();
    hold!.resolve(shape({ aql: AQL.byRegion } as BackendQueryRequest));

    await more;
    expect(signals[1].aborted).toBe(true);
    expect(query.result?.rows).toHaveLength(2);
  });

  it('aborts a filter\'s loadOptions in flight and drops its late list', async () => {
    const pending = deferred<ConditionValue[]>();
    let seen: AbortSignal | undefined;
    const { app } = build(shape, (_request, signal) => {
      seen = signal;
      return pending.promise;
    });
    const filter = app.createFilter('pick', { field: 'users.region', dataset: 'sales' });
    const loading = filter.loadOptions();

    filter.remove();
    pending.resolve(['APAC']);

    await expect(loading).rejects.toMatchObject({ name: 'AbortError' });
    expect(seen?.aborted).toBe(true);
    expect(filter.options).toEqual([]);
  });

  it('leaves a removed entity dead: its methods throw', () => {
    const { app } = build();
    const query = app.createQuery('chart', { dataset: 'sales', aql: AQL.byRegion });
    const filter = app.createFilter('status', { type: 'string' });
    const drill = app.createDateDrill('grain');
    query.remove();
    filter.remove();
    drill.remove();

    const removed = /'chart' was removed from its app/;
    expect(() => query.update({ aql: AQL.byStatus })).toThrowError(removed);
    expect(() => query.remove()).toThrowError(removed);
    expect(() => query.setSort([])).toThrowError(removed);
    expect(() => query.select([])).toThrowError(removed);
    expect(() => query.execute()).toThrowError(removed);
    expect(() => query.locate()).toThrowError(removed);
    expect(() => filter.setCondition({ operator: 'is', values: ['a'] })).toThrowError(/'status' was removed/);
    expect(() => filter.update({ label: 'x' })).toThrowError(/'status' was removed/);
    expect(() => drill.setGrain('year')).toThrowError(/'grain' was removed/);
    expect(() => app.mapCrossFilter(query, query)).toThrowError(ValidationError);
  });

  it('removes one edge alone, and dirties its target', async () => {
    const { stub, app } = build();
    const query = app.createQuery('chart', { dataset: 'sales', aql: AQL.byRegion });
    const target = app.createQuery('target', { dataset: 'sales', aql: AQL.byStatus });
    const filter = app.createFilter('region', { field: 'users.region', dataset: 'sales' });
    const mapping = app.mapControl(filter, target, { field: 'users.region' });
    const cross = app.mapCrossFilter(query, target);
    filter.setCondition({ operator: 'is', values: ['EMEA'] });
    await app.execute();
    query.select({ region: 'APAC', total: 100 });
    await app.execute();

    cross.remove();
    expect(app.mappings).toEqual([mapping]);
    expect(target.isDirty).toBe(true);
    await app.execute();
    expect(stub.queries.at(-1)?.input.filters).toEqual([{ field: 'users.region', operator: 'is', values: ['EMEA'] }]);

    mapping.remove();
    expect(app.mappings).toEqual([]);
    expect(target.isDirty).toBe(true);
    await app.execute();
    expect(stub.queries.at(-1)?.input.filters).toEqual([]);

    expect(() => mapping.remove()).toThrowError(/was removed from its app/);
    expect(() => app.mapControl(filter, target, { field: 'users.region' })).not.toThrow();
  });
});

describe('app.hasChanges', () => {
  it('is true after a query is edited, and false again once it reran', async () => {
    const { app } = build();
    const query = app.createQuery('chart', { dataset: 'sales', aql: AQL.byRegion });
    await app.execute();
    expect(app.hasChanges).toBe(false);

    query.update({ aql: AQL.byStatus });
    expect(app.hasChanges).toBe(true);

    await app.execute();
    expect(app.hasChanges).toBe(false);
  });

  it('is true after a query, a control or an edge is removed and leaves a target dirty', async () => {
    const { app } = build();
    const source = app.createQuery('source', { dataset: 'sales', aql: AQL.byRegion });
    const target = app.createQuery('target', { dataset: 'sales', aql: AQL.byStatus });
    const filter = app.createFilter('region', { field: 'users.region', dataset: 'sales' });
    const status = app.createFilter('status', { field: 'orders.status', dataset: 'sales' });
    app.mapControl(filter, target, { field: 'users.region' });
    const mapping = app.mapControl(status, target, { field: 'orders.status' });
    app.mapCrossFilter(source, target);
    filter.setCondition({ operator: 'is', values: ['EMEA'] });
    status.setCondition({ operator: 'is', values: ['pending'] });
    await app.execute();
    source.select({ region: 'APAC', total: 100 });
    await app.execute();
    expect(app.hasChanges).toBe(false);

    mapping.remove();
    expect(app.hasChanges).toBe(true);
    await app.execute();
    expect(app.hasChanges).toBe(false);

    filter.remove();
    expect(app.hasChanges).toBe(true);
    await app.execute();
    expect(app.hasChanges).toBe(false);

    source.remove();
    expect(app.hasChanges).toBe(true);
    await app.execute();
    expect(app.hasChanges).toBe(false);
  });

  it('is true for a query created after the first execute, until it runs; not before the first', async () => {
    const { app } = build();
    app.createQuery('first', { dataset: 'sales', aql: AQL.byRegion });
    expect(app.hasChanges).toBe(false);
    await app.execute();

    app.createQuery('second', { dataset: 'sales', aql: AQL.byStatus });
    expect(app.hasChanges).toBe(true);

    await app.execute();
    expect(app.hasChanges).toBe(false);
  });

  it('does not count a query whose first run failed', async () => {
    const { app } = build(() => new Error('down'));
    app.createQuery('chart', { dataset: 'sales', aql: AQL.byRegion });
    await app.execute();

    expect(app.queries.chart.state).toBe('error');
    expect(app.hasChanges).toBe(false);
  });

  it('is false while execute runs with the current inputs, as soon as it starts', async () => {
    let hold: ReturnType<typeof deferred<BackendQueryResult>> | undefined;
    const { app } = build((request) => {
      if (request.input.filters?.length) {
        hold = deferred<BackendQueryResult>();
        return hold.promise;
      }
      return shape(request);
    });
    const query = app.createQuery('chart', { dataset: 'sales', aql: AQL.byRegion });
    const filter = app.createFilter('region', { field: 'users.region', dataset: 'sales' });
    app.mapControl(filter, query, { field: 'users.region' });
    await app.execute();

    filter.setCondition({ operator: 'is', values: ['APAC'] });
    expect(app.hasChanges).toBe(true);
    const running = app.execute();
    expect(query.state).toBe('executing');
    expect(app.hasChanges).toBe(false);

    hold!.resolve(shape({ aql: AQL.byRegion } as BackendQueryRequest));
    await running;
    expect(app.hasChanges).toBe(false);
  });

  it('is true when an edit supersedes the run in flight', async () => {
    let hold: ReturnType<typeof deferred<BackendQueryResult>> | undefined;
    const { app } = build((request) => {
      if (hold === undefined && request.input.filters?.length) {
        hold = deferred<BackendQueryResult>();
        return hold.promise;
      }
      return shape(request);
    });
    const query = app.createQuery('chart', { dataset: 'sales', aql: AQL.byRegion });
    const filter = app.createFilter('region', { field: 'users.region', dataset: 'sales' });
    app.mapControl(filter, query, { field: 'users.region' });
    await app.execute();
    filter.setCondition({ operator: 'is', values: ['APAC'] });
    const running = app.execute();
    expect(app.hasChanges).toBe(false);

    query.update({ aql: AQL.byStatus });
    expect(app.hasChanges).toBe(true);
    hold!.resolve(shape({ aql: AQL.byRegion } as BackendQueryRequest));
    await running;
    expect(app.hasChanges).toBe(true);

    await app.execute();
    expect(app.hasChanges).toBe(false);
  });

  it('is false while a page is fetched', async () => {
    let hold: ReturnType<typeof deferred<BackendQueryResult>> | undefined;
    const { app } = build((request) => {
      if (request.input.pagination?.page === 2) {
        hold = deferred<BackendQueryResult>();
        return hold.promise;
      }
      return { ...shape(request), meta: { hasMore: true } };
    });
    const query = app.createQuery('table', { dataset: 'sales', aql: AQL.byRegion, pageSize: 2 });
    await app.execute();
    const more = query.fetchMore();
    expect(app.hasChanges).toBe(false);

    hold!.resolve(shape({ aql: AQL.byRegion } as BackendQueryRequest));
    await more;
    expect(app.hasChanges).toBe(false);
  });

  it('does not count a run that failed after one that succeeded', async () => {
    let fail = false;
    const { app } = build((request) => {
      if (fail) throw new Error('down');
      return shape(request);
    });
    app.createQuery('chart', { dataset: 'sales', aql: AQL.byRegion });
    await app.execute();
    fail = true;
    await app.refresh();

    expect(app.queries.chart.state).toBe('error');
    expect(app.hasChanges).toBe(false);
  });
});
