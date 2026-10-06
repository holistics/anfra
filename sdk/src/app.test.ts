import { describe, expect, it } from 'vitest';
import { createSdk } from './sdk';
import { ValidationError } from './errors';
import { salesDataset, stubBackend, testUser } from './testSupport';
import type { Backend } from './types';
import type { App } from './app';

function sdk () {
  return createSdk({
    datasets: { sales: salesDataset },
    user: testUser,
    backend: stubBackend().backend,
  });
}

function monthlyRevenue (app: App) {
  return app.createQuery('revenue', {
    dataset: 'sales',
    aql: `
      explore {
        dimensions { month: date_trunc(orders.created_at, "month") }
        measures { total: orders | sum(orders.amount) }
      }
    `,
  });
}

describe('declaration', () => {
  it('requires a dataset and an AQL body — nothing else is validated client-side', () => {
    const app = sdk().createApp();

    // A query is a raw AQL body now: the SDK has no client-side field knowledge, so a syntax or
    // field error surfaces as a QueryError at execute, not here.
    expect(() => app.createQuery('q', { dataset: 'sales', aql: '' }))
      .toThrowError(/declares no AQL/);
    expect(() => app.createQuery('q', {} as never)).toThrow();
  });

  it('pages only a query that declares a whole-number page size', () => {
    const app = sdk().createApp();
    const aql = 'explore { measures { n: count(orders.id) } }';

    expect(() => app.createQuery('zero', { dataset: 'sales', aql, pageSize: 0 }))
      .toThrowError(/pageSize 0.*whole number of at least 1/);
    expect(() => app.createQuery('frac', { dataset: 'sales', aql, pageSize: 2.5 })).toThrow(ValidationError);
    expect(app.createQuery('big', { dataset: 'sales', aql, pageSize: 5000 }).pageSize).toBe(5000);
    expect(app.createQuery('all', { dataset: 'sales', aql }).pageSize).toBeUndefined();
  });

  it('rejects duplicate entity names across queries and controls', () => {
    const app = sdk().createApp();
    monthlyRevenue(app);

    expect(() => app.createFilter('revenue', { type: 'string' }))
      .toThrowError(/'revenue' is already declared/);
  });

  it('requires a type for a manual filter and infers one for a field-backed filter', () => {
    const app = sdk().createApp();

    expect(() => app.createFilter('freeform', {})).toThrowError(/needs a `type`/);
    expect(app.createFilter('region', { field: 'users.region' }).type).toBe('string');
    expect(app.createFilter('when', { field: 'orders.created_at' }).type).toBe('date');
  });

  it('distinguishes field-backed from manual filters', () => {
    const app = sdk().createApp();

    expect(app.createFilter('region', { field: 'users.region' }).isFieldBacked).toBe(true);
    expect(app.createFilter('label', { type: 'string', options: ['a', 'b'] }).isFieldBacked).toBe(false);
    expect(app.createFilter('label2', { type: 'string' }).options).toEqual([]);
  });
});

describe('mapping', () => {
  it('is always explicit, and carries the field verbatim — the SDK does not resolve it', () => {
    // A query is a raw AQL body now, so the SDK has no client-side way to check whether
    // `options.field` resolves to anything on the target query. A typo surfaces as a QueryError at
    // execute, not here. See docs/adr/0001.
    const app = sdk().createApp();
    const query = monthlyRevenue(app);
    const region = app.createFilter('region', { field: 'users.region' });

    const mapping = app.mapControl(region, query, { field: 'users.region' });
    expect(mapping.id).toBe('region.revenue.users.region');
    expect(mapping.field).toBe('users.region');
  });

  it('includes the field in the id, so two fields between one pair do not collide', () => {
    const app = sdk().createApp();
    const query = monthlyRevenue(app);
    const control = app.createFilter('anything', { type: 'string' });

    const first = app.mapControl(control, query, { field: 'users.region' });
    const second = app.mapControl(control, query, { field: 'orders.status' });

    expect(first.id).not.toBe(second.id);
  });

  it('rejects a duplicate mapping', () => {
    const app = sdk().createApp();
    const query = monthlyRevenue(app);
    const region = app.createFilter('region', { field: 'users.region' });

    app.mapControl(region, query, { field: 'users.region' });
    expect(() => app.mapControl(region, query, { field: 'users.region' }))
      .toThrowError(/is already declared/);
  });

  // An aggregated filter conditions `sum(orders.amount)` rather than the column. The edge carries
  // the aggregation, not the control: the same control could condition a raw column on one query
  // and an aggregate on another.
  it('carries an aggregation on the edge', () => {
    const app = sdk().createApp();
    const query = monthlyRevenue(app);
    const control = app.createFilter('big', { type: 'number' });

    const mapping = app.mapControl(control, query, { field: 'orders.amount', aggregation: 'sum' });

    expect(mapping.aggregation).toBe('sum');
    expect(mapping.toJSON()).toMatchObject({ field: 'orders.amount', aggregation: 'sum' });
  });

  // `running *` are window functions, and the compiler discards such a condition in silence — the
  // rows come back unfiltered with no error anywhere. This rejection is the only visible failure,
  // and it needs no field resolution to check: it is a plain enum membership test.
  it('rejects a windowed aggregation, which the compiler would silently drop', () => {
    const app = sdk().createApp();
    const query = monthlyRevenue(app);
    const control = app.createFilter('big', { type: 'number' });

    expect(() => app.mapControl(control, query, {
      field: 'orders.amount',
      aggregation: 'running sum' as never,
    })).toThrowError(/Invalid filter aggregation 'running sum'/);
  });

  it('rejects a control that belongs to another app', () => {
    const shared = sdk();
    const a = shared.createApp();
    const b = shared.createApp();
    const query = monthlyRevenue(a);
    const stray = b.createFilter('region', { field: 'users.region' });

    expect(() => a.mapControl(stray, query, { field: 'users.region' }))
      .toThrowError(/belongs to a different app/);
  });

  it('allows a control with no mappings', () => {
    const app = sdk().createApp();
    app.createFilter('unused', { type: 'string' });
    expect(app.mappings).toHaveLength(0);
  });

  it('cross-filter does not check for a selectable column upfront — that is only known once the source has a result', () => {
    // A query is a raw AQL body, so whether it has any real (non-adhoc, non-measure) dimension is
    // unknowable until it has run. `mapCrossFilter` no longer refuses this at declaration time; see
    // `deriveSelection` in selection.ts for where the check actually happens. See docs/adr/0001.
    const app = sdk().createApp();
    const from = monthlyRevenue(app);
    const to = app.createQuery('other', { dataset: 'sales', aql: 'explore { measures { n: 1 } }' });

    expect(() => app.mapCrossFilter(from, to)).not.toThrow();
  });
});

describe('toJSON', () => {
  it('carries the declaration and nothing about the environment or the reader', () => {
    const app = sdk().createApp({ title: 'Sales', timezone: 'Asia/Singapore' });
    const query = monthlyRevenue(app);
    const region = app.createFilter('region', { field: 'users.region' });
    app.createDateDrill('grain', { default: 'month' });
    app.mapControl(region, query, { field: 'users.region' });

    region.setCondition({ operator: 'is', values: ['APAC'] });

    const json = app.toJSON();

    expect(json.title).toBe('Sales');
    expect(json.timezone).toBe('Asia/Singapore');
    expect(Object.keys(json.queries)).toEqual(['revenue']);
    expect(Object.keys(json.dateDrills)).toEqual(['grain']);
    expect(JSON.stringify(json)).not.toContain('APAC');
    expect(JSON.stringify(json)).not.toContain('42');
  });

  it('serialises edges generically, so cross-filtering later is not a migration', () => {
    const app = sdk().createApp();
    const query = monthlyRevenue(app);
    const region = app.createFilter('region', { field: 'users.region' });
    app.mapControl(region, query, { field: 'users.region' });

    expect(app.toJSON().mappings[0]).toEqual({
      kind: 'control',
      id: 'region.revenue.users.region',
      from: { kind: 'filter', name: 'region' },
      to: { kind: 'query', name: 'revenue' },
      field: 'users.region',
    });
  });

  it('round-trips: rebuilding from JSON produces identical JSON', () => {
    const instance = sdk();
    const app = instance.createApp({ title: 'Sales', timezone: 'UTC' });
    const query = monthlyRevenue(app);
    const byRegion = app.createQuery('byRegion', {
      dataset: 'sales',
      aql: `
        explore {
          dimensions { region: users.region }
          measures { total: orders | sum(orders.amount) }
        }
      `,
    });
    const region = app.createFilter('region', { field: 'users.region', dataset: 'sales' });
    app.mapControl(region, query, { field: 'users.region' });
    app.mapCrossFilter(byRegion, query);

    const json = app.toJSON();

    const rebuilt = instance.createApp({ title: json.title, timezone: json.timezone });
    Object.entries(json.queries).forEach(([name, declaration]) => rebuilt.createQuery(name, declaration));
    Object.entries(json.filters).forEach(([name, declaration]) => rebuilt.createFilter(name, declaration));
    json.mappings.forEach((edge) => {
      // The tag is what makes one list rebuildable: no guessing the kind from which fields are set.
      if (edge.kind === 'crossFilter') {
        rebuilt.mapCrossFilter(rebuilt.queries[edge.from.name], rebuilt.queries[edge.to.name]);
      } else {
        rebuilt.mapControl(
          rebuilt.controls[edge.from.name],
          rebuilt.queries[edge.to.name],
          { field: edge.field },
        );
      }
    });

    expect(rebuilt.toJSON()).toEqual(json);
  });
});

describe('controls', () => {
  it('hands on the values the backend suggests', async () => {
    const stub = stubBackend({ fieldSuggestions: ['APAC', 2, true] });
    const app = createSdk({ datasets: { sales: salesDataset }, user: testUser, backend: stub.backend }).createApp();
    const region = app.createFilter('region', { field: 'users.region', dataset: 'sales' });

    expect(await region.loadOptions()).toEqual(['APAC', 2, true]);
    expect(region.options).toEqual(['APAC', 2, true]);
  });

  it('spells the date drill so the author never writes a transform operator', () => {
    const app = sdk().createApp();

    expect(app.createDateDrill('grain', { default: 'month' }).condition)
      .toEqual({ operator: 'transform_date_drill', values: ['datetrunc month'] });
    // No grain means no drill, rather than a grain the author did not ask for.
    expect(app.createDateDrill('off').condition).toEqual({ operator: 'none' });
  });

  it('is not dirty before the first execution, so a URL-set value is not an unapplied change', () => {
    const app = sdk().createApp();
    const region = app.createFilter('region', { field: 'users.region' });

    region.setCondition({ operator: 'is', values: ['APAC'] });

    expect(region.isDirty).toBe(false);
    expect(app.hasChanges).toBe(false);
  });

  it('rejects an unknown grain by name', () => {
    const app = sdk().createApp();
    expect(() => app.createDateDrill('g', { default: 'fortnight' as never }))
      .toThrowError(ValidationError);
  });
});

describe('environment', () => {
  it('refuses to build without datasets, naming what the host must do', () => {
    expect(() => createSdk({ user: testUser } as never))
      .toThrowError(/The host must call createSdk\(\{ datasets \}\)/);
  });

  it('builds with no datasets, for a viewer entitled to none', () => {
    const empty = createSdk({ datasets: {}, user: testUser, backend: stubBackend().backend });

    expect(empty.datasets).toEqual({});
  });

  it('does not offer an empty list of datasets to pick from', () => {
    const app = createSdk({ datasets: {}, user: testUser, backend: stubBackend().backend }).createApp();

    expect(() => app.createFilter('region', { field: 'users.region' }))
      .toThrowError(/the environment has no datasets/);
  });

  it('blames the app, not the host, when it declares a query against no datasets', () => {
    const app = createSdk({ datasets: {}, user: testUser, backend: stubBackend().backend }).createApp();

    expect(() => app.createQuery('revenue', { dataset: 'sales', aql: 'explore {}' }))
      .toThrowError(/Unknown dataset 'sales'/);
  });

  it('exposes datasets so an agent can check a field before writing it', () => {
    expect(Object.keys(sdk().datasets)).toEqual(['sales']);
  });

  it('refuses to build without a user, naming what the host must do', () => {
    expect(() => createSdk({ datasets: { sales: salesDataset } } as never))
      .toThrowError(/The host must call createSdk\(\{ user \}\)/);
  });

  it('refuses to build without a backend, naming what the host must do', () => {
    expect(() => createSdk({ datasets: { sales: salesDataset }, user: testUser } as never))
      .toThrowError(/The host must call createSdk\(\{ backend \}\)/);
  });

  it('exposes the user so an app can greet them without awaiting anything', () => {
    expect(sdk().user.name).toBe('Ada Lovelace');
    expect(sdk().user.permissions.canViewGeneratedSql).toBe(true);
  });

  it('freezes permissions, so author code cannot grant itself a capability', () => {
    const { user } = sdk();
    expect(() => {
      (user.permissions as { canViewGeneratedSql: boolean }).canViewGeneratedSql = false;
    }).toThrow();
    expect(user.permissions.canViewGeneratedSql).toBe(true);
  });

  it('keeps the user out of the declaration, which is what gets persisted', () => {
    const app = sdk().createApp({ title: 'Sales' });
    expect(app.toJSON()).not.toHaveProperty('user');
  });
});

describe('loadOptions', () => {
  it('asks the backend for the field\'s values by dataset, model and field name', async () => {
    const stub = stubBackend({ fieldSuggestions: ['APAC', 'EMEA'] });
    const app = createSdk({ datasets: { sales: salesDataset }, user: testUser, backend: stub.backend }).createApp();
    const region = app.createFilter('region', { field: 'users.region' });

    expect(await region.loadOptions('AP')).toEqual(['APAC', 'EMEA']);
    expect(stub.calls).toHaveLength(1);
    expect(stub.calls[0]).toMatchObject({
      method: 'fieldSuggestions',
      // Names, not descriptor ids: the backend resolves the field the same way AQL does.
      request: {
        dataset: 'sales', model: 'users', field: 'region', q: 'AP',
      },
    });
    expect(stub.calls[0].signal).toBeInstanceOf(AbortSignal);
    expect(region.options).toEqual(['APAC', 'EMEA']);
  });

  it('passes the caller\'s signal, so a stale lookup can be cancelled', async () => {
    const stub = stubBackend({ fieldSuggestions: ['APAC'] });
    const app = createSdk({ datasets: { sales: salesDataset }, user: testUser, backend: stub.backend }).createApp();
    const region = app.createFilter('region', { field: 'users.region' });
    const controller = new AbortController();

    await region.loadOptions('A', controller.signal);

    expect(stub.calls[0].signal).toBe(controller.signal);
  });

  it('asks nothing for a filter on a metric, which has no values to suggest', async () => {
    const stub = stubBackend({ fieldSuggestions: ['x'] });
    const app = createSdk({ datasets: { sales: salesDataset }, user: testUser, backend: stub.backend }).createApp();
    const aov = app.createFilter('aov', { field: 'aov', type: 'number' });

    expect(await aov.loadOptions()).toEqual([]);
    expect(stub.calls).toHaveLength(0);
  });
});

describe('the date drill toggle', () => {
  function appWith (features?: { dateDrill?: boolean }) {
    return createSdk({
      datasets: { sales: salesDataset },
      user: testUser,
      backend: stubBackend().backend,
      ...(features ? { features } : {}),
    }).createApp();
  }

  it('refuses the control when the host says date drill is off', () => {
    // Without this the control declares fine, the `transform_date_drill` condition goes on the
    // wire, the server ignores it, and every query succeeds at the declared grain — a feature that
    // silently does nothing rather than one that says it is unavailable.
    const app = appWith({ dateDrill: false });

    expect(() => app.createDateDrill('grain', { default: 'month' }))
      .toThrowError(ValidationError);
    expect(() => app.createDateDrill('grain2', { default: 'month' }))
      .toThrowError(/interactive_control:date_drill/);
  });

  it('allows it when the host said nothing', () => {
    // Absent means the host did not say; only an explicit false blocks.
    expect(() => appWith().createDateDrill('grain', { default: 'month' })).not.toThrow();
  });

  it('allows it when the host says it is on', () => {
    expect(() => appWith({ dateDrill: true }).createDateDrill('grain', {})).not.toThrow();
  });
});

describe('superseding a run', () => {
  /** A backend whose nth query hangs until released, so a later run can overtake an in-flight one. */
  function gatedBackend (hangOnCall: number): Backend & { release: () => void } {
    let release: () => void = () => {};
    const gate = new Promise<void>((resolve) => { release = resolve; });
    let calls = 0;

    return {
      async submitQuery () {
        calls += 1;
        if (calls === hangOnCall) await gate;
        return {
          values: [['2026-01-01', 30]],
          columns: [{
            name: 'month', fieldName: 'month', label: 'month', adhoc: false, isMeasure: false,
          }],
          meta: { page: 1, pageSize: 1000, numRows: 1 },
        };
      },
      async fieldSuggestions () { return []; },
      release: () => release(),
    };
  }

  it('lands the result of a run a later execute left alone', async () => {
    const backend = gatedBackend(2);
    const app = createSdk({
      datasets: { sales: salesDataset },
      user: testUser,
      backend,
    }).createApp();
    const revenue = monthlyRevenue(app);

    await app.execute();

    // `refresh()` forces a re-run of a query nothing has dirtied, and that request hangs.
    const forced = app.refresh();
    expect(revenue.state).toBe('executing');

    // Nothing changed since the first run, so this execute has no targets: it neither re-runs the
    // query nor aborts it, and the hanging request is still the only one that can resolve it.
    await app.execute();

    backend.release();
    await forced;

    // The whole point: a run counter would have called this superseded and dropped the result,
    // stranding the query as `executing` with nothing in flight to move it.
    expect(revenue.state).toBe('success');
    expect(revenue.result?.rows).toHaveLength(1);
  });
});
