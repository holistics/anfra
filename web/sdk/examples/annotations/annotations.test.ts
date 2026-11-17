// The annotations example, tested in a test frame as the usage guide says
// (docs/usage/sdk-plugins.md, Testing): it sets one annotation of each kind on every result.
import { describe, expect, it } from 'vitest';
import { testFrame } from '@holistics/anfra-sdk/testing';
import setup from './frame';

const columns = [
  {
    name: 'month', fieldName: 'created_at', modelId: 'orders', label: 'Month', adhoc: false, isMeasure: false,
    semantic_type: 'date', arrow_type: { type: 'date', label: 'date32' }, db_type: 'date',
  },
  {
    name: 'revenue', fieldName: 'amount', modelId: 'orders', label: 'Revenue', adhoc: false, isMeasure: true, aggregation: 'sum',
    semantic_type: 'number', arrow_type: { type: 'decimal', label: 'decimal(38, 2)' }, db_type: 'numeric',
  },
];

function frame (runs: unknown[][][]) {
  let run = 0;
  const f = testFrame({
    setups: [['annotations-example', setup]],
    datasets: { sales: { name: 'sales', label: 'Sales', data_models: [], metrics: [] } as never },
    answers: { 'core.submitQuery': () => ({ columns, values: runs[Math.min(run++, runs.length - 1)] }) },
  });
  const app = f.anfra.createApp();
  return { app, query: app.createQuery('revenue', { dataset: 'sales', aql: 'explore {}' }) };
}

describe('the annotations example', () => {
  it('sets one annotation of each kind from trivial rules', async () => {
    const { app, query } = frame([[
      ['2026-01-01', '10'], ['2026-02-01', '30'], ['2026-03-01', '90'], ['2026-04-01', '50'],
    ]]);
    await app.execute();

    expect(query.annotations.map((annotation) => [annotation.kind, annotation.anchor])).toEqual([
      ['mark', { rows: [{ month: '2026-03-01' }], measure: 'revenue', index: [2] }],
      ['focus', { rows: [{ month: '2026-03-01' }, { month: '2026-04-01' }, { month: '2026-02-01' }], measure: 'revenue', index: [1, 2, 3] }],
      ['line', { column: 'revenue', value: '45' }],
      ['band', { column: 'month', from: '2026-02-01', to: '2026-04-01' }],
      ['series', {
        measure: 'revenue',
        points: [
          { at: { month: '2026-05-01' }, value: '50', lower: '45', upper: '55' },
          { at: { month: '2026-06-01' }, value: '50', lower: '45', upper: '55' },
          { at: { month: '2026-07-01' }, value: '50', lower: '45', upper: '55' },
        ],
      }],
    ]);
  });

  it('writes the forecast\'s periods as the rows do, for a timestamp column', async () => {
    const { app, query } = frame([[['2026-01-01T00:00:00Z', '10'], ['2026-02-01T00:00:00Z', '30'], ['2026-03-01T00:00:00Z', '20']]]);
    await app.execute();
    const series = query.annotations.find((annotation) => annotation.kind === 'series');
    expect(series?.kind === 'series' && series.anchor.points.map((point) => point.at)).toEqual([
      { month: '2026-04-01T00:00:00Z' }, { month: '2026-05-01T00:00:00Z' }, { month: '2026-06-01T00:00:00Z' },
    ]);
    expect(query.annotations.find((annotation) => annotation.kind === 'band')?.anchor).toEqual({ column: 'month', from: '2026-01-01T00:00:00Z', to: '2026-03-01T00:00:00Z' });
  });

  it('annotates each new result', async () => {
    const { app, query } = frame([
      [['2026-01-01', '10'], ['2026-02-01', '30']],
      [['2026-01-01', '70'], ['2026-02-01', '30']],
    ]);
    await app.execute();
    await app.refresh();
    expect(query.annotations.find((annotation) => annotation.kind === 'mark')?.anchor).toMatchObject({ rows: [{ month: '2026-01-01' }] });
  });

  it('sets a target zone on the measure, and no series, when the periods aren\'t dates', async () => {
    const { app, query } = frame([[['APAC', '10'], ['EMEA', '30']]]);
    await app.execute();
    expect(query.annotations.map((annotation) => annotation.kind)).toEqual(['mark', 'focus', 'line', 'band']);
    expect(query.annotations.find((annotation) => annotation.kind === 'band')?.anchor).toEqual({ column: 'revenue', from: '18', to: '22' });
  });
});
