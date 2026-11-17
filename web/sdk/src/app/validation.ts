import { FILTER_AGGREGATIONS, type FilterAggregation } from '../common/aggregations';
import { ValidationError } from '../common/errors';
import { suggest, withSuggestion } from './suggest';
import type { Aggregation, DatasetDescriptor, WeekStartDay } from '../common/types';

export interface ResolvedField {
  /**
   * The model's *name*, not a database id, and absent for a dataset-level metric. Backends resolve
   * fields by name, the way AQL does; `data_models[].id` is not reliably populated.
   */
  modelName?: string;
  fieldName: string;
  isMetric: boolean;
  type: string;
  label: string;
}



/**
 * Indexes one dataset's fields so validation is a lookup rather than a scan, and so we have a
 * candidate list to draw typo suggestions from.
 */
export class DatasetIndex {
  readonly descriptor: DatasetDescriptor;

  private readonly fields = new Map<string, ResolvedField>();

  readonly references: string[];

  constructor (descriptor: DatasetDescriptor) {
    this.descriptor = descriptor;

    descriptor.data_models.forEach((model) => {
      model.fields.forEach((field) => {
        const modelName = String(model.name);
        this.fields.set(`${modelName}.${field.name}`, {
          modelName,
          fieldName: field.name,
          isMetric: false,
          type: field.type,
          label: field.label ?? field.name,
        });
      });
    });

    // A bare name is a dataset-level metric. This mirrors how the server splits a field reference:
    // it pops the last dot-separated segment and treats an absent model prefix as `is_metric`.
    descriptor.metrics.forEach((metric) => {
      this.fields.set(metric.name, {
        fieldName: metric.name,
        isMetric: true,
        type: metric.type,
        label: metric.label ?? metric.name,
      });
    });

    this.references = [...this.fields.keys()];
  }

  resolve (reference: string, entity: string): ResolvedField {
    const field = this.fields.get(reference);
    if (field) return field;

    throw new ValidationError(
      withSuggestion(
        `Unknown field '${reference}' on dataset '${this.descriptor.name}'.`,
        reference,
        this.references,
      ),
      entity,
    );
  }
}

export function resolveDataset (
  datasets: Record<string, DatasetIndex>,
  uname: string,
  entity: string,
): DatasetIndex {
  const dataset = datasets[uname];
  if (dataset) return dataset;

  throw new ValidationError(
    withSuggestion(
      `Unknown dataset '${uname}'.`,
      uname,
      Object.keys(datasets),
    ),
    entity,
  );
}

/**
 * The one check a filter's aggregation still needs, at the one point an author can still be told
 * about it. Whether the target is itself a metric — which would make the aggregation a silent
 * no-op — can no longer be checked here: a query is a raw AQL body now, so the SDK has no client-
 * side knowledge of what a `Mapping.field` reference resolves to. That failure mode moves to the
 * server. See docs/adr/0001.
 */
export function assertValidFilterAggregation (aggregation: string, entity: string): void {
  if (!(FILTER_AGGREGATIONS as readonly string[]).includes(aggregation)) {
    throw new ValidationError(
      withSuggestion(
        `Invalid filter aggregation '${aggregation}'.`,
        aggregation,
        [...FILTER_AGGREGATIONS],
      ),
      entity,
    );
  }
}

export function assertUniqueName (
  taken: Iterable<string>,
  name: string,
  kind: string,
): void {
  if (!name) {
    throw new ValidationError(`A ${kind} needs a name.`);
  }
  for (const existing of taken) {
    if (existing === name) {
      throw new ValidationError(`'${name}' is already declared in this app.`, name);
    }
  }
}

/** Milliseconds per unit, as Go's `time.ParseDuration` names them: the server reads durations with it. */
const DURATION_UNITS: Record<string, number> = { ns: 1e-6, us: 1e-3, 'µs': 1e-3, ms: 1, s: 1_000, m: 60_000, h: 3_600_000 };

/** The longest a result can be cached: the server's `querycache.MaxDuration`. */
const MAX_CACHE_DURATION_MS = 168 * 3_600_000;

/** A duration in milliseconds, as Go's `time.ParseDuration` reads it; undefined when it isn't one, or is negative. */
export function parseDuration (text: string): number | undefined {
  if (text === '0') return 0;
  // Each part a number and its unit, from where the last ended: a failed match resets lastIndex,
  // so where the parts ended is kept apart.
  const part = /(\d+(?:\.\d*)?|\.\d+)(ns|us|µs|ms|s|m|h)/y;
  let total = 0;
  let end = 0;
  for (let match = part.exec(text); match; match = part.exec(text)) {
    total += parseFloat(match[1]) * DURATION_UNITS[match[2]];
    end = part.lastIndex;
  }
  return end > 0 && end === text.length ? total : undefined;
}

/**
 * How an entity came to have a setting: declared (`createApp`, `createQuery`), or set later (a setter,
 * `query.update`). A message says it as the author did it.
 */
export type Giving = 'declares' | 'sets';

/** "The app declares weekStartDay "Monday"", or "Query 'q' sets pageSize to 0". */
export function gave (declarer: string, setting: string, value: unknown, how: Giving): string {
  const shown = JSON.stringify(value);
  return how === 'sets' ? `${declarer} sets ${setting} to ${shown}` : `${declarer} declares ${setting} ${shown}`;
}

/** The days a week can start on, as the core API takes them. */
export const WEEK_START_DAYS: readonly WeekStartDay[] = ['monday', 'tuesday', 'wednesday', 'thursday', 'friday', 'saturday', 'sunday'];

/**
 * Refuses a week start day the server would refuse, before any query runs, suggesting the day a
 * different case or a typo meant. `declarer` says who gave it (`The app`), `setting` which setting it
 * is (`weekStartDay`, `query.default.input.weekStartDay`), and `how` how it was given.
 */
export function assertWeekStartDay (day: unknown, declarer: string, setting: string, how: Giving = 'declares'): void {
  if (day === undefined || WEEK_START_DAYS.includes(day as WeekStartDay)) return;
  const said = `${gave(declarer, setting, day, how)}.`;
  const meant = typeof day === 'string' ? suggest(day.trim(), [...WEEK_START_DAYS]) : undefined;
  if (meant) throw new ValidationError(`${said} Did you mean ${JSON.stringify(meant)}?`);
  const choices = WEEK_START_DAYS.map((d) => JSON.stringify(d));
  throw new ValidationError(`${said} Use one of: ${choices.slice(0, -1).join(', ')}, or ${choices[choices.length - 1]}.`);
}

/**
 * Refuses a cache duration the server would refuse, before any query runs: one that isn't a
 * duration, or is over 7 days. `declarer` says who gave it (`Query 'revenue'`, `The app`), `setting`
 * which setting it is, and `how` how it was given.
 */
export function assertCacheDuration (duration: unknown, declarer: string, setting: string, entity?: string, how: Giving = 'declares'): void {
  if (duration === undefined) return;
  const ms = typeof duration === 'string' ? parseDuration(duration) : undefined;
  if (ms === undefined) {
    throw new ValidationError(`${gave(declarer, setting, duration, how)}, which isn't a duration. Use one such as 30s, 10m or 2h.`, entity);
  }
  if (ms > MAX_CACHE_DURATION_MS) {
    throw new ValidationError(`${gave(declarer, setting, duration, how)}, which is over 168h (7 days), the longest a result can be cached.`, entity);
  }
}
