import { FILTER_AGGREGATIONS, type FilterAggregation } from '../common/aggregations';
import { ValidationError } from '../common/errors';
import { withSuggestion } from './suggest';
import type { Aggregation, DatasetDescriptor } from '../common/types';

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
