import {
  describe, expect, it, vi,
} from 'vitest';
import { createSdk } from './sdk';
import { ValidationError } from '../common/errors';
import {
  salesDataset, stubBackend, testUser, type StubOptions,
} from './testSupport';
import type { App } from './app';
import type { Query } from './query';
import type { BackendQueryRequest, ColumnMeta } from '../common/types';

/**
 * A result column, for the stub responses below. None of these fixtures exercise a query-declared
 * alias, so `fieldName` (the real field) always matches `name`
 * (the row key) — see execute_aql_query_spec.rb for where they intentionally diverge.
 */
function dim (name: string, modelId?: string): ColumnMeta {
  return {
    name, fieldName: name, ...(modelId ? { modelId } : {}), label: name, adhoc: false, isMeasure: false,
  };
}
function measureCol (name: string, aggregation = 'sum'): ColumnMeta {
  return {
    name, fieldName: name, label: name, adhoc: false, isMeasure: true, aggregation,
  };
}

/**
 * A query is a raw AQL body now, so a request cannot be told apart by declared field count the
 * way the structured wire could. Each query below carries a `// marker` comment naming itself, and
 * the stub dispatches on that — matching how a real AQL program is free to carry a comment, but
 * giving the test harness a stable way to say "this response is for that query."
 */
function markerAql (marker: string, body: string): string {
  return `// ${marker}\nexplore {\n${body}\n}`;
}

const AQL = {
  byRegion: markerAql('byRegion', 'dimensions { region: users.region }\nmeasures { total: orders | sum(orders.amount) }'),
  detail: markerAql('detail', 'dimensions { status: orders.status }\nmeasures { total: orders | sum(orders.amount) }'),
  unlinked: markerAql('unlinked', 'dimensions { id: orders.id }'),
  totalsOnly: markerAql('totalsOnly', 'measures { total: orders | sum(orders.amount) }'),
  regionStatus: markerAql('regionStatus', 'dimensions { region: users.region, status: orders.status }\nmeasures { total: orders | sum(orders.amount) }'),
  monthly: markerAql('monthly', 'dimensions { month: date_trunc(orders.created_at, "month") }\nmeasures { total: orders | sum(orders.amount) }'),
  ids: markerAql('ids', 'dimensions { id: orders.id }'),
  elsewhere: markerAql('elsewhere', 'dimensions { region: users.region }'),
};

const FIELDS: Record<string, ColumnMeta[]> = {
  byRegion: [dim('region', 'users'), measureCol('total')],
  detail: [dim('status', 'orders'), measureCol('total')],
  unlinked: [dim('id', 'orders')],
  totalsOnly: [measureCol('total')],
  regionStatus: [dim('region', 'users'), dim('status', 'orders'), measureCol('total')],
  // What the AQL compiler really returns for `month: date_trunc(orders.created_at, "month")`: the
  // alias is the row key, but the column still points at the real field with a transformation on it.
  monthly: [{ ...dim('month', 'orders'), fieldName: 'created_at' }, measureCol('total')],
  ids: [dim('id', 'orders')],
  elsewhere: [dim('region', 'users')],
};

const ROWS: Record<string, unknown[][]> = {
  byRegion: [['APAC', 100]],
  detail: [['completed', 50]],
  unlinked: [[1]],
  totalsOnly: [[100]],
  regionStatus: [['APAC', 'completed', 40]],
  monthly: [['2026-01-01', 30]],
  ids: [[17]],
  elsewhere: [['APAC']],
};

function markerOf (aql: string): string {
  const match = /^\/\/ (\S+)/.exec(aql);
  return match ? match[1] : '';
}

function echoShape (request: BackendQueryRequest) {
  const marker = markerOf(request.aql);
  return {
    values: ROWS[marker] ?? [],
    columns: FIELDS[marker] ?? [],
    meta: { page: 1, pageSize: 1000, numRows: (ROWS[marker] ?? []).length },
  };
}

function build (submitQuery: StubOptions['submitQuery'] = echoShape) {
  const stub = stubBackend({ submitQuery });
  const sdk = createSdk({
    // A second dataset, so "one selection cannot cross datasets" is testable.
    datasets: { sales: salesDataset, other: { ...salesDataset, id: 43, name: 'other' } },
    user: testUser,
    backend: stub.backend,
  });
  return { stub, sdk };
}

function byRegion (app: App, name = 'byRegion'): Query {
  return app.createQuery(name, { dataset: 'sales', aql: AQL.byRegion });
}

function detail (app: App, name = 'detail'): Query {
  return app.createQuery(name, { dataset: 'sales', aql: AQL.detail });
}

/** The Query Input the backend was last sent, for the request whose AQL carries this marker. */
function inputFor (stub: { queries: BackendQueryRequest[] }, marker: string): BackendQueryRequest['input'] {
  const matching = stub.queries.filter((request) => markerOf(request.aql) === marker);
  return matching[matching.length - 1]?.input;
}

/** The filters the backend was last sent, for the request whose AQL carries this marker. */
function filtersFor (stub: { queries: BackendQueryRequest[] }, marker: string): unknown {
  return inputFor(stub, marker)?.filters;
}

describe('declaring a cross-filter', () => {
  it('refuses a self-edge, because a source keeps the rows it is showing as selected', () => {
    const app = build().sdk.createApp();
    const query = byRegion(app);

    expect(() => app.mapCrossFilter(query, query)).toThrowError(/cannot cross-filter itself/);
  });

  it('accepts both directions: one selection is live at a time, so a pair is not a loop', () => {
    const app = build().sdk.createApp();
    const a = byRegion(app);
    const b = detail(app);

    app.mapCrossFilter(a, b);
    expect(() => app.mapCrossFilter(b, a)).not.toThrow();
    expect(app.crossFilters).toHaveLength(2);
  });

  it('refuses the same pair twice', () => {
    const app = build().sdk.createApp();
    const a = byRegion(app);
    const b = detail(app);
    app.mapCrossFilter(a, b);

    expect(() => app.mapCrossFilter(a, b)).toThrowError(/already declared/);
  });

  it('refuses to span datasets, since a field reference means nothing outside its own', () => {
    const app = build().sdk.createApp();
    const a = byRegion(app);
    const b = app.createQuery('elsewhere', { dataset: 'other', aql: AQL.elsewhere });

    expect(() => app.mapCrossFilter(a, b)).toThrowError(/spans two datasets/);
  });

  it('accepts a source with no dimensions — whether it has a selectable column is only known once it has a result', () => {
    // A query is a raw AQL body, so this can no longer be checked at declaration time. See
    // `deriving conditions from selected rows` below for where a measures-only source's selection
    // is silently empty instead. See docs/adr/0001.
    const app = build().sdk.createApp();
    const measuresOnly = app.createQuery('totals', { dataset: 'sales', aql: AQL.totalsOnly });

    expect(() => app.mapCrossFilter(measuresOnly, detail(app))).not.toThrow();
  });

  it('points each verb at the other when the wrong kind of source is passed', () => {
    const app = build().sdk.createApp();
    const query = byRegion(app);
    const filter = app.createFilter('region', { field: 'users.region', dataset: 'sales' });

    expect(() => app.mapCrossFilter(filter as never, query)).toThrowError(/app\.mapControl/);
    expect(() => app.mapControl(query as never, query, { field: 'users.region' }))
      .toThrowError(/app\.mapCrossFilter/);
  });

  it('serialises into the one mapping list, tagged, carrying no field', () => {
    const app = build().sdk.createApp();
    app.mapCrossFilter(byRegion(app), detail(app));

    expect(app.toJSON().mappings).toEqual([{
      kind: 'crossFilter',
      id: 'byRegion.detail',
      from: { kind: 'query', name: 'byRegion' },
      to: { kind: 'query', name: 'detail' },
    }]);
  });
});

describe('deriving conditions from selected rows', () => {
  async function selecting (aql: string) {
    const app = build().sdk.createApp();
    const source = app.createQuery('source', { dataset: 'sales', aql });
    app.mapCrossFilter(source, detail(app));
    await app.execute();
    return { app, source };
  }

  it('turns one row into one condition per selectable column', async () => {
    const { app, source } = await selecting(AQL.regionStatus);

    source.select({ region: 'APAC', status: 'completed' });

    expect(app.selection?.conditions).toEqual([
      { field: 'orders.status', operator: 'is', values: ['completed'] },
      { field: 'users.region', operator: 'is', values: ['APAC'] },
    ]);
    expect(app.selection?.lossy).toBe(false);
  });

  it('never conditions on a measure: its value describes the bucket, it does not identify it', async () => {
    const { app, source } = await selecting(AQL.byRegion);

    source.select({ region: 'APAC', total: 181 });

    expect(app.selection?.conditions).toEqual([
      { field: 'users.region', operator: 'is', values: ['APAC'] },
    ]);
  });

  it('produces no selection from a measures-only source, once its result shows it has no selectable column', async () => {
    const app = build().sdk.createApp();
    const measuresOnly = app.createQuery('totals', { dataset: 'sales', aql: AQL.totalsOnly });
    app.mapCrossFilter(measuresOnly, detail(app));
    await app.execute();

    measuresOnly.select({ total: 100 });

    expect(app.selection).toBeUndefined();
  });

  it('collects several rows of one dimension into one condition, losing nothing', async () => {
    const { app, source } = await selecting(AQL.byRegion);

    source.select([{ region: 'APAC' }, { region: 'EMEA' }, { region: 'APAC' }]);

    expect(app.selection?.conditions).toEqual([
      { field: 'users.region', operator: 'is', values: ['APAC', 'EMEA'] },
    ]);
    expect(app.selection?.lossy).toBe(false);
  });

  it('says exactly what was picked, as one condition, when the collapse would have widened it', async () => {
    const warn = vi.spyOn(console, 'warn').mockImplementation(() => {});
    const { app, source } = await selecting(AQL.regionStatus);

    // Field by field these become region IN (APAC, EMEA) AND status IN (completed, pending), which
    // also admits (APAC, pending). An AQL condition is the only form that can say otherwise.
    source.select([
      { region: 'APAC', status: 'completed' },
      { region: 'EMEA', status: 'pending' },
    ]);

    expect(app.selection?.expression).toBe(
      'or(and(users.region is "APAC", orders.status is "completed"), '
      + 'and(users.region is "EMEA", orders.status is "pending"))',
    );
    // The structured array is left empty: the two must never both reach the payload.
    expect(app.selection?.conditions).toEqual([]);
    expect(app.selection?.lossy).toBe(false);
    expect(warn).not.toHaveBeenCalled();
    warn.mockRestore();
  });

  it('stays lossless across dimensions when the rows do form a full rectangle', async () => {
    const { app, source } = await selecting(AQL.regionStatus);

    source.select([
      { region: 'APAC', status: 'completed' },
      { region: 'APAC', status: 'pending' },
      { region: 'EMEA', status: 'completed' },
      { region: 'EMEA', status: 'pending' },
    ]);

    expect(app.selection?.lossy).toBe(false);
  });

  it('narrows to the named columns for a chart that renders fewer than the query returned', async () => {
    const { app, source } = await selecting(AQL.regionStatus);

    source.select({ region: 'APAC', status: 'completed' }, { fields: ['region'] });

    expect(app.selection?.conditions).toEqual([
      { field: 'users.region', operator: 'is', values: ['APAC'] },
    ]);
  });

  it('rejects a narrowing that names a measure, and one that names nothing', async () => {
    const { source } = await selecting(AQL.byRegion);

    expect(() => source.select({ region: 'APAC' }, { fields: ['total'] }))
      .toThrowError(/it is a measure/);
    expect(() => source.select({ region: 'APAC' }, { fields: ['regoin'] }))
      .toThrowError(/Did you mean 'region'\?/);
  });

  it('matches a null value with is_null rather than an empty is', async () => {
    const { app, source } = await selecting(AQL.byRegion);

    source.select({ region: null });

    expect(app.selection?.conditions).toEqual([
      { field: 'users.region', operator: 'is_null', values: [] },
    ]);
  });

  it('clears the selection when nothing is selected', async () => {
    const { app, source } = await selecting(AQL.byRegion);
    source.select({ region: 'APAC' });

    source.select([]);

    expect(app.selection).toBeUndefined();
  });
});

describe('date-drilled dimensions', () => {
  // A query is a raw AQL body now, so there is no declared "starting grain" for a date column —
  // only a mapped, applied date drill can size a range. Every scenario here needs one, defaulting
  // to month; tests that drill further just re-apply a different grain. See docs/adr/0001.
  async function monthly () {
    const app = build().sdk.createApp();
    const source = app.createQuery('source', { dataset: 'sales', aql: AQL.monthly });
    const grain = app.createDateDrill('grain', { default: 'month' });
    app.mapControl(grain, source, { field: 'orders.created_at' });
    app.mapCrossFilter(source, detail(app));
    await app.execute();
    return { app, source, grain };
  }

  it('sends the whole bucket as a range, not the instant the label happens to be', async () => {
    const { app, source } = await monthly();

    source.select({ month: '2026-01-01' });

    // `until` is end-exclusive in the Holistics date phrase grammar, so this is exactly January.
    expect(app.selection?.conditions).toEqual([
      { field: 'orders.created_at', operator: 'matches', values: ['2026-01-01 until 2026-02-01'] },
    ]);
    expect(app.selection?.lossy).toBe(false);
  });

  it('spans consecutive buckets with one range and calls it lossless', async () => {
    const { app, source } = await monthly();

    source.select([{ month: '2026-01-01' }, { month: '2026-02-01' }]);

    expect(app.selection?.conditions[0].values).toEqual(['2026-01-01 until 2026-03-01']);
    expect(app.selection?.lossy).toBe(false);
  });

  it('sizes the range by the drilled grain — the column carries none of its own now', async () => {
    // A query is a raw AQL body, so there is no declared "starting grain" to fall back to; only a
    // mapped date drill can supply one. See docs/adr/0001.
    const { app, source, grain } = await monthly();

    grain.setGrain('quarter');
    await app.execute();

    source.select({ month: '2027-01-01' });

    expect(app.selection?.conditions[0].values).toEqual(['2027-01-01 until 2027-04-01']);
    expect(app.selection?.lossy).toBe(false);
  });

  it('treats adjacent buckets at the drilled grain as contiguous', async () => {
    const { app, source, grain } = await monthly();

    grain.setGrain('quarter');
    await app.execute();

    // Against a month grain these look non-adjacent, and were wrongly flagged lossy.
    source.select([{ month: '2026-10-01' }, { month: '2027-01-01' }]);

    expect(app.selection?.conditions[0].values).toEqual(['2026-10-01 until 2027-04-01']);
    expect(app.selection?.lossy).toBe(false);
  });

  it('ignores a drill the reader has not applied yet', async () => {
    // The rows on screen were bucketed by the applied grain (month, from `monthly()`'s own setup
    // run), so that is what sizes the range — not the pending, not-yet-applied quarter.
    const { app, source, grain } = await monthly();

    grain.setGrain('quarter');

    source.select({ month: '2027-01-01' });

    expect(app.selection?.conditions[0].values).toEqual(['2027-01-01 until 2027-02-01']);
  });

  it('sends one range per bucket rather than widening across the gap', async () => {
    const warn = vi.spyOn(console, 'warn').mockImplementation(() => {});
    const { app, source } = await monthly();

    // A single `matches` would have had to span 2026-01-01 until 2026-04-01, silently including
    // February. Two ranges under an `or` include exactly the two months picked.
    source.select([{ month: '2026-01-01' }, { month: '2026-03-01' }]);

    expect(app.selection?.expression).toBe(
      'or(orders.created_at matches @(2026-01-01 until 2026-02-01), '
      + 'orders.created_at matches @(2026-03-01 until 2026-04-01))',
    );
    expect(app.selection?.lossy).toBe(false);
    expect(warn).not.toHaveBeenCalled();
    warn.mockRestore();
  });

  it('keeps the time component when the server sends one, and does not shift it', async () => {
    const { app, source } = await monthly();

    source.select({ month: '2026-01-01T00:00:00.000Z' });

    expect(app.selection?.conditions[0].values)
      .toEqual(['2026-01-01 00:00:00 until 2026-02-01 00:00:00']);
  });
});

describe('what a selection reaches', () => {
  async function wired () {
    const { stub, sdk } = build();
    const app = sdk.createApp();
    const source = byRegion(app);
    const target = detail(app);
    const unlinked = app.createQuery('unlinked', { dataset: 'sales', aql: AQL.unlinked });
    app.mapCrossFilter(source, target);
    await app.execute();
    return {
      stub, app, source, target, unlinked,
    };
  }

  it('conditions declared targets and leaves everything else alone', async () => {
    const { stub, app, source } = await wired();
    source.select({ region: 'APAC' });
    await app.execute();

    expect(filtersFor(stub, 'detail')).toEqual([
      { field: 'users.region', operator: 'is', values: ['APAC'] },
    ]);
    // The source shows which rows are selected; filtering it would erase them.
    expect(filtersFor(stub, 'byRegion')).toEqual([]);
    expect(filtersFor(stub, 'unlinked')).toEqual([]);
  });

  it('re-runs only the queries the selection reaches', async () => {
    const { stub, app, source } = await wired();
    const afterFirst = stub.calls.length;

    source.select({ region: 'APAC' });
    await app.execute();

    expect(stub.calls.length - afterFirst).toBe(1);
  });

  it('keeps the highlight when moving a selection re-runs the query it moves to', async () => {
    // A source is never filtered by its own selection, but it does re-run when it stops being
    // another query's target — which is exactly what moving the selection here does. Its rows come
    // back as new objects, so an identity match would drop the highlight at that moment.
    const { sdk } = build();
    const app = sdk.createApp();
    const first = byRegion(app);
    const second = detail(app);
    app.mapCrossFilter(first, second);
    app.mapCrossFilter(second, first);

    await app.execute();
    first.select(first.result!.rows[0]);
    await app.execute();

    const picked = second.result!.rows[0];
    second.select(picked);
    await app.execute();

    expect(second.selectedRows).toHaveLength(1);
    expect(second.selectedRows[0]).toBe(second.result!.rows[0]);
  });

  it('records which columns a selection was derived from', async () => {
    const { app, source } = await wired();

    source.select({ region: 'APAC', total: 1 });

    expect(app.selection?.fields).toEqual(['region']);
  });

  it('replaces a selection made in another query rather than accumulating', async () => {
    const { app, source, target } = await wired();
    app.mapCrossFilter(target, source);

    source.select({ region: 'APAC' });
    target.select({ status: 'completed' });

    expect(app.selection?.source).toBe('detail');
    expect(app.selection?.conditions).toEqual([
      { field: 'orders.status', operator: 'is', values: ['completed'] },
    ]);
    expect(source.selectedRows).toEqual([]);
    expect(target.selectedRows).toEqual([{ status: 'completed', total: 50 }]);
  });

  it('sends a numeric value as a number, which the query API accepts', async () => {
    const { stub, sdk } = build();
    const app = sdk.createApp();
    const source = app.createQuery('idsQuery', { dataset: 'sales', aql: AQL.ids });
    app.mapCrossFilter(source, detail(app));
    await app.execute();

    source.select({ id: 17 });
    await app.execute();

    expect(filtersFor(stub, 'detail')).toEqual([
      { field: 'orders.id', operator: 'is', values: [17] },
    ]);
  });
});

describe('pending and applied', () => {
  async function wired () {
    const { stub, sdk } = build();
    const app = sdk.createApp();
    const source = byRegion(app);
    app.mapCrossFilter(source, detail(app));
    await app.execute();
    return { stub, app, source };
  }

  it('produces no selection before the source has ever run — its columns are not known yet', () => {
    // A query is a raw AQL body now, so `select()` before the first result has nothing to
    // classify columns against. See docs/adr/0001.
    const { sdk } = build();
    const app = sdk.createApp();
    const source = byRegion(app);
    app.mapCrossFilter(source, detail(app));

    source.select({ region: 'APAC' });

    expect(app.selection).toBeUndefined();
    expect(app.hasChanges).toBe(false);
  });

  it('becomes an unapplied change once made, and settles after the next run', async () => {
    const { app, source } = await wired();
    expect(app.hasChanges).toBe(false);

    source.select({ region: 'APAC' });
    expect(app.hasChanges).toBe(true);

    await app.execute();
    expect(app.hasChanges).toBe(false);
    expect(app.appliedSelection?.source).toBe('byRegion');
  });

  it('clears back to nothing selected', async () => {
    const { app, source } = await wired();
    source.select({ region: 'APAC' });
    await app.execute();

    app.clearSelection();

    expect(app.selection).toBeUndefined();
    expect(app.hasChanges).toBe(true);
  });

  it('notifies subscribers, so a chart can redraw its highlight', async () => {
    const { app, source } = await wired();
    const seen = vi.fn();
    app.subscribe(seen);

    source.select({ region: 'APAC' });

    expect(seen).toHaveBeenCalled();
    expect(source.selectedRows).toEqual([{ region: 'APAC', total: 100 }]);
  });
});

describe('cross-filter and controls together', () => {
  it('ANDs a selection alongside the conditions arriving through mappings', async () => {
    const { stub, sdk } = build();
    const app = sdk.createApp();
    const source = byRegion(app);
    const target = detail(app);
    const status = app.createFilter('statusFilter', { field: 'orders.status', dataset: 'sales' });
    app.mapControl(status, target, { field: 'orders.status' });
    app.mapCrossFilter(source, target);

    status.setCondition({ operator: 'is', values: ['completed'] });
    await app.execute();
    source.select({ region: 'APAC' });
    await app.execute();

    expect(filtersFor(stub, 'detail')).toEqual([
      { field: 'orders.status', operator: 'is', values: ['completed'] },
      { field: 'users.region', operator: 'is', values: ['APAC'] },
    ]);
  });
});

describe('ValidationError is what every refusal throws', () => {
  it('names the entity so an agent can find the declaring line', () => {
    const app = build().sdk.createApp();
    const query = byRegion(app);

    try {
      app.mapCrossFilter(query, query);
      expect.unreachable();
    } catch (error) {
      expect(error).toBeInstanceOf(ValidationError);
      expect((error as ValidationError).entity).toBe('byRegion');
    }
  });
});

describe('a selection that only an AQL condition can express', () => {
  it('reaches the target as an aql_condition, and leaves filters alone', async () => {
    const { stub, sdk } = build();
    const app = sdk.createApp();
    const source = app.createQuery('source', { dataset: 'sales', aql: AQL.regionStatus });
    const target = detail(app);
    app.mapCrossFilter(source, target);
    await app.execute();

    source.select([
      { region: 'APAC', status: 'completed' },
      { region: 'EMEA', status: 'pending' },
    ]);
    await app.execute();

    const input = inputFor(stub, 'detail');
    expect(input.conditions).toEqual([{
      expr: 'or(and(users.region is "APAC", orders.status is "completed"), '
        + 'and(users.region is "EMEA", orders.status is "pending"))',
    }]);
    // The structured array carries nothing from the selection — one or the other, never both.
    expect(input.filters).toEqual([]);
  });

  it('re-runs when one expression selection replaces another', async () => {
    // Two expression selections in a row both leave `conditions` empty. Keyed on the conditions
    // alone they are indistinguishable, the query looks clean, and `execute()` sends nothing at
    // all — the reader clicks a third bucket and the dashboard simply stops responding.
    const { stub, sdk } = build();
    const app = sdk.createApp();
    const source = app.createQuery('source', { dataset: 'sales', aql: AQL.monthly });
    const grain = app.createDateDrill('grain', { default: 'month' });
    app.mapControl(grain, source, { field: 'orders.created_at' });
    app.mapCrossFilter(source, detail(app));
    await app.execute();

    // Non-contiguous under a month grain (February and April are skipped), which is what makes
    // the field-by-field collapse lossy and routes the selection through an expression.
    source.select([{ month: '2026-01-01' }, { month: '2026-03-01' }]);
    await app.execute();
    const afterTwo = stub.calls.length;

    source.select([{ month: '2026-01-01' }, { month: '2026-03-01' }, { month: '2026-05-01' }]);
    expect(app.hasChanges).toBe(true);
    await app.execute();

    expect(stub.calls.length).toBeGreaterThan(afterTwo);
    expect(inputFor(stub, 'detail').conditions[0].expr).toBe(
      'or(orders.created_at matches @(2026-01-01 until 2026-02-01), '
      + 'orders.created_at matches @(2026-03-01 until 2026-04-01), '
      + 'orders.created_at matches @(2026-05-01 until 2026-06-01))',
    );
    // The drill reaches its own query as a date drill, while the ranges above are sized by it.
    expect(inputFor(stub, 'monthly')).toMatchObject({
      filters: [],
      dateDrills: [{ field: 'orders.created_at', grain: 'month' }],
    });
  });

  it('leaves a selection the structured form can express on the structured path', async () => {
    // The proven path stays in charge whenever it can say what the selection means.
    const { stub, sdk } = build();
    const app = sdk.createApp();
    const source = byRegion(app);
    app.mapCrossFilter(source, detail(app));
    await app.execute();

    source.select({ region: 'APAC' });
    await app.execute();

    const input = inputFor(stub, 'detail');
    expect(input.conditions).toEqual([]);
    expect(input.filters).toEqual([
      { field: 'users.region', operator: 'is', values: ['APAC'] },
    ]);
  });
});
