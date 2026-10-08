import type { Aggregation } from './types';

/**
 * The aggregations a *filter* may carry — a narrower set than a measure may, for two reasons.
 *
 * The `running *` four are window functions, and the compiler discards a result-field filter whose
 * fragment still carries `hasWindowFunction`: silently, leaving the rows unfiltered. A rejection
 * here is the only place that failure is visible.
 *
 * `custom` means "the field defines its own aggregation", which a filter never needs to say. A
 * dataset-level metric is rejected outright below, and a model field that is already a measure
 * routes through the aggregated branch on its own. The wire accepts `custom` regardless — the
 * dashboard path sends it for a metric — so this is an SDK narrowing, not a transport limit.
 *
 * Declared `satisfies readonly Aggregation[]` so the subset relationship is checked rather than
 * asserted, and the type below is derived from the one list the runtime check uses.
 */
export const FILTER_AGGREGATIONS = [
  'sum', 'avg', 'max', 'min', 'count', 'count distinct', 'median',
  'stdev', 'stdevp', 'var', 'varp',
] as const satisfies readonly Aggregation[];

export type FilterAggregation = typeof FILTER_AGGREGATIONS[number];
