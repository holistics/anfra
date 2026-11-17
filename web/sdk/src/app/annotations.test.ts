import { afterEach, describe, expect, it, vi } from 'vitest';
import { testFrame } from './testFrame';
import type { FrameKernel } from './kernel';
import type { PluginSetup } from './bootstrap';
import type { BackendQueryResult, ColumnMeta } from '../common/types';
import { numberTypes, textTypes } from './testSupport';

afterEach(() => {
  document.body.innerHTML = '';
});

const dateTypes = { semantic_type: 'date', arrow_type: { type: 'date', label: 'date32' }, db_type: 'date' } as const;

const COLUMNS: ColumnMeta[] = [
  { name: 'month', modelId: 'orders', fieldName: 'created_at', label: 'Month', adhoc: false, isMeasure: false, ...dateTypes },
  { name: 'revenue', modelId: 'orders', fieldName: 'amount', label: 'Revenue', adhoc: false, isMeasure: true, aggregation: 'sum', ...numberTypes },
  { name: 'status', modelId: 'orders', fieldName: 'status', label: 'Status', adhoc: false, isMeasure: false, ...textTypes },
];

const result = (rows: unknown[][], meta?: BackendQueryResult['meta']): BackendQueryResult => ({ columns: COLUMNS, values: rows, ...(meta ? { meta } : {}) });

const MONTHS = [
  ['2026-01-01', '10', 'paid'],
  ['2026-02-01', '30', 'paid'],
  ['2026-03-01', '90', 'paid'],
];

/**
 * A frame with two plugins whose kernels the test holds, and a query whose runs answer from
 * `answers`, one run each, the last repeating.
 */
function frame (answers: BackendQueryResult[] = [result(MONTHS)]) {
  const kernels: Record<string, FrameKernel> = {};
  const setups: PluginSetup[] = ['anomalies', 'targets'].map((name) => [name, (kernel: FrameKernel) => { kernels[name] = kernel; }]);
  let run = 0;
  const f = testFrame({
    setups,
    answers: {
      'core.submitQuery': () => answers[Math.min(run++, answers.length - 1)],
    },
  });
  const app = f.anfra.createApp();
  const query = app.createQuery('revenue', { dataset: 'sales', aql: 'explore {}' });
  return {
    f, app, query, anomalies: kernels.anomalies, targets: kernels.targets,
  };
}

describe('kernel.annotate', () => {
  it('sets the plugin\'s annotations on the query, replacing its last set, and [] clears it', async () => {
    const { app, query, anomalies } = frame();
    await app.execute();

    expect(anomalies.annotate(query, [{ kind: 'line', anchor: { column: 'revenue', value: '40' } }], { result: query.result! })).toBe(true);
    expect(query.annotations.map((a) => a.kind)).toEqual(['line']);

    anomalies.annotate(query, [
      { kind: 'mark', anchor: { rows: [{ month: '2026-03-01' }], measure: 'revenue' }, label: 'Spike', severity: 'warning' },
    ], { result: query.result! });
    expect(query.annotations).toEqual([{
      kind: 'mark',
      anchor: { rows: [{ month: '2026-03-01' }], measure: 'revenue', index: [2] },
      label: 'Spike',
      severity: 'warning',
      plugin: 'anomalies',
      key: 'anomalies:0',
    }]);

    anomalies.annotate(query, [], { result: query.result! });
    expect(query.annotations).toEqual([]);
  });

  it('ignores annotations computed from a result that is no longer the query\'s', async () => {
    const { app, query, anomalies } = frame([result(MONTHS), result(MONTHS.slice(0, 2))]);
    await app.execute();
    const old = query.result!;
    await app.refresh();

    expect(anomalies.annotate(query, [{ kind: 'line', anchor: { column: 'revenue', value: '40' } }], { result: old })).toBe(false);
    expect(query.annotations).toEqual([]);
  });

  it('throws on a malformed annotation', async () => {
    const { app, query, anomalies } = frame();
    await app.execute();
    const annotate = (annotation: unknown) => () => anomalies.annotate(query, [annotation as never], { result: query.result! });

    expect(annotate({ kind: 'arrow', anchor: { column: 'revenue', value: '1' } })).toThrow(/kind 'arrow'/);
    expect(annotate({ kind: 'line', anchor: { column: 'profit', value: '1' } })).toThrow(/no column 'profit'/);
    expect(annotate({ kind: 'band', anchor: { column: 'month', from: '2026-01-01' } })).toThrow(/from.*to/);
    expect(annotate({ kind: 'line', anchor: { column: 'revenue', value: 40 } })).toThrow(/string/);
    expect(annotate({ kind: 'mark', anchor: { rows: [{ month: '2026-03-01' }], measure: 'status' } })).toThrow(/'status'.*not a measure/);
    expect(annotate({ kind: 'mark', anchor: { rows: [] } })).toThrow(/rows/);
    expect(annotate({ kind: 'mark', anchor: { rows: [{ revenue: '90' }] } })).toThrow(/'revenue'.*measure/);
    expect(annotate({ kind: 'mark', anchor: { rows: [{ month: '2026-03-01' }] }, severity: 'fatal' })).toThrow(/severity/);
    expect(annotate({ kind: 'series', anchor: { measure: 'revenue', points: [{ at: { month: '2026-04-01' } }] } })).toThrow(/value.*bounds/);
    expect(annotate({ kind: 'series', anchor: { measure: 'revenue', points: [{ at: { month: '2026-04-01' }, lower: '1' }] } })).toThrow(/value.*bounds/);
    expect(annotate({ kind: 'series', anchor: { measure: 'month', points: [{ at: { month: '2026-04-01' }, value: '1' }] } })).toThrow(/'month'.*not a measure/);
    expect(annotate({ kind: 'series', anchor: { measure: 'revenue', points: [{ at: { quarter: 'Q2' }, value: '1' }] } })).toThrow(/no column 'quarter'/);
    expect(() => anomalies.annotate(query, [
      { kind: 'line', id: 'mean', anchor: { column: 'revenue', value: '1' } },
      { kind: 'line', id: 'mean', anchor: { column: 'revenue', value: '2' } },
    ], { result: query.result! })).toThrow(/id 'mean'/);
    expect(query.annotations).toEqual([]);
  });

  it('takes a series through points the result lacks, by value, by bounds or both', async () => {
    const { app, query, anomalies } = frame();
    await app.execute();
    const points = [
      { at: { month: '2026-04-01' }, value: '90' },
      { at: { month: '2026-05-01' }, lower: '80', upper: '100' },
      { at: { month: '2026-06-01' }, value: '90', lower: '80', upper: '100' },
    ];
    anomalies.annotate(query, [{ kind: 'series', id: 'forecast', anchor: { measure: 'revenue', points } }], { result: query.result! });
    expect(query.annotations).toEqual([{
      kind: 'series', id: 'forecast', anchor: { measure: 'revenue', points }, plugin: 'anomalies', key: 'anomalies:forecast',
    }]);
  });
});

describe('how long annotations last', () => {
  const line = { kind: 'line', anchor: { column: 'revenue', value: '40' } } as const;

  async function annotated () {
    const made = frame();
    await made.app.execute();
    made.anomalies.annotate(made.query, [line], { result: made.query.result! });
    expect(made.query.annotations).toHaveLength(1);
    return made;
  }

  it('go when a refresh replaces the result', async () => {
    const { app, query } = await annotated();
    await app.refresh();
    expect(query.annotations).toEqual([]);
  });

  it('go when changed inputs rerun the query', async () => {
    const { app, query } = await annotated();
    const control = app.createFilter('status', { field: 'orders.status' });
    app.mapControl(control, query, { field: 'orders.status' });
    control.setCondition({ operator: 'is', values: ['paid'] });
    await app.execute();
    expect(query.annotations).toEqual([]);
  });

  it('go when a re-sort reruns the query', async () => {
    const { app, query } = await annotated();
    query.setSort([{ field: 'revenue', direction: 'desc' }]);
    await app.execute();
    expect(query.annotations).toEqual([]);
  });

  it('go when the definition updates the query, before it reruns, and stay gone after', async () => {
    const { app, query } = await annotated();
    const notified = vi.fn();
    query.subscribe(notified);
    query.update({ aql: 'explore { dimensions { orders.status } }' });
    expect(query.annotations).toEqual([]);
    expect(notified).toHaveBeenCalled();
    await app.execute();
    expect(query.annotations).toEqual([]);
  });

  it('can\'t be set from the result the query had before the definition updated it', async () => {
    const { query, anomalies } = await annotated();
    const before = query.result!;
    query.update({ aql: 'explore { dimensions { orders.status } }' });
    // The old result is still shown until the rerun, but it is no longer current.
    expect(query.result).toBe(before);
    expect(anomalies.annotate(query, [line], { result: before })).toBe(false);
    expect(query.annotations).toEqual([]);
  });

  it('go when the query is removed', async () => {
    const { query, anomalies } = await annotated();
    const result = query.result!;
    query.remove();
    expect(query.annotations).toEqual([]);
    expect(anomalies.annotate(query, [line], { result })).toBe(false);
  });

  it('stay across fetchMore, which extends the result, and a row it brings resolves', async () => {
    const paged = frame([
      result(MONTHS.slice(0, 2), { page: 1, pageSize: 2, numRows: 2, hasMore: true }),
      result(MONTHS.slice(2), { page: 2, pageSize: 2, numRows: 1, hasMore: false }),
    ]);
    paged.query.update({ aql: 'explore {}', pageSize: 2 });
    await paged.app.execute();
    const before = paged.query.result!;
    paged.anomalies.annotate(paged.query, [line, { kind: 'mark', anchor: { rows: [{ month: '2026-03-01' }] } }], { result: before });
    expect(paged.query.annotations.map((a) => a.kind)).toEqual(['line']);

    await paged.query.fetchMore();

    expect(paged.query.annotations.map((a) => a.kind)).toEqual(['line', 'mark']);
    expect(paged.query.annotations[1].anchor).toMatchObject({ index: [2] });
    // A result read before the page still counts as current.
    expect(paged.anomalies.annotate(paged.query, [line], { result: before })).toBe(true);
  });
});

describe('query.annotations', () => {
  it('merges every plugin\'s set, each with its plugin and a stable key', async () => {
    const { app, query, anomalies, targets } = frame();
    await app.execute();
    anomalies.annotate(query, [
      { kind: 'mark', id: 'march', anchor: { rows: [{ month: '2026-03-01' }] }, severity: 'critical' },
      { kind: 'focus', anchor: { rows: [{ month: '2026-02-01' }, { month: '2026-03-01' }] } },
    ], { result: query.result! });
    targets.annotate(query, [{ kind: 'band', anchor: { column: 'revenue', from: '20', to: '50' }, label: 'Target zone' }], { result: query.result! });

    expect(query.annotations.map((a) => [a.plugin, a.key, a.kind])).toEqual([
      ['anomalies', 'anomalies:march', 'mark'],
      ['anomalies', 'anomalies:1', 'focus'],
      ['targets', 'targets:0', 'band'],
    ]);
    expect(query.annotations).toBe(query.annotations);
  });

  it('drops a rows anchor\'s row the result lacks, and the annotation when it lacks them all', async () => {
    const { app, query, anomalies } = frame();
    await app.execute();
    anomalies.annotate(query, [
      { kind: 'mark', anchor: { rows: [{ month: '2026-03-01' }, { month: '2027-01-01' }] } },
      { kind: 'mark', anchor: { rows: [{ month: '2027-01-01' }] } },
      { kind: 'focus', anchor: { rows: [{ status: 'paid' }] } },
    ], { result: query.result! });

    expect(query.annotations.map((a) => a.anchor)).toEqual([
      { rows: [{ month: '2026-03-01' }], index: [2] },
      { rows: [{ status: 'paid' }], index: [0, 1, 2] },
    ]);
  });

  it('notifies subscribers when a plugin sets or clears its set', async () => {
    const { app, query, anomalies } = frame();
    await app.execute();
    let seen = 0;
    query.subscribe(() => { seen += 1; });
    anomalies.annotate(query, [{ kind: 'line', anchor: { column: 'revenue', value: '40' } }], { result: query.result! });
    anomalies.annotate(query, [], { result: query.result! });
    anomalies.annotate(query, [], { result: query.result! });
    expect(seen).toBe(2);
  });
});
