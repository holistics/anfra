import { TransportError } from '../common/errors';
import type { DatasetDescriptor } from '../common/types';
import type { CoreClient } from './client';
import type { components } from './schema';
import { toSdkError, type ApiErrorBody } from './errors';

type ShowDataset = components['schemas']['ShowDataset'];
type CompileError = components['schemas']['CompileError'];

/**
 * A dataset as the SDK takes it, from `core.show`'s. A dataset and a model are named by their fqn,
 * the name a query uses; a dataset-level dimension is a field of the model it is declared on.
 */
export function toDescriptor (dataset: ShowDataset): DatasetDescriptor {
  return {
    id: dataset.fqn,
    name: dataset.fqn,
    ...(dataset.label ? { label: dataset.label } : {}),
    data_models: (dataset.models ?? []).map((model) => ({
      id: model.fqn,
      name: model.fqn,
      ...(model.label ? { label: model.label } : {}),
      fields: model.fields.map((field) => ({
        name: field.name,
        ...(field.label ? { label: field.label } : {}),
        type: field.type,
        is_custom_measure: field.role === 'measure',
        ...(field.definedInDataset ? { defined_in_dataset: true } : {}),
      })),
    })),
    metrics: (dataset.metrics ?? []).map((metric) => ({
      name: metric.name,
      ...(metric.label ? { label: metric.label } : {}),
      type: metric.type,
    })),
  };
}

/**
 * Every dataset of the repo that could be shown in full, by name; and what is wrong in the repo for
 * them (its files that do not compile, a data source it does not configure, a dataset it cannot
 * show in full).
 */
export interface LoadedDatasets {
  datasets: Record<string, DatasetDescriptor>;
  diagnostics: CompileError[];
}

/**
 * What a Data App is provisioned with: `core.show` of the repo, which answers every dataset in
 * full. A dataset that cannot be shown in full comes in outline, with a diagnostic saying why: it
 * is left out, rather than failing the rest, as a Data App that does not use it still runs.
 */
export async function loadDatasets (client: CoreClient, signal?: AbortSignal): Promise<LoadedDatasets> {
  const { data, error, response } = await client.POST('/core.show', { body: {}, signal });
  if (!data) throw toSdkError(response.status, (error as { error?: ApiErrorBody } | undefined)?.error);
  if (data.object.kind !== 'repo') throw new TransportError(`core.show answered a ${data.object.kind}, not the repo.`);
  const datasets: Record<string, DatasetDescriptor> = {};
  data.object.datasets.filter((d) => d.models).forEach((d) => { datasets[d.fqn] = toDescriptor(d); });
  return { datasets, diagnostics: data.diagnostics };
}
