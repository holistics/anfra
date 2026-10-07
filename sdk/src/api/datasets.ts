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

/** Every dataset of the repo, in full, by name; and the repo's files that do not compile. */
export interface LoadedDatasets {
  datasets: Record<string, DatasetDescriptor>;
  diagnostics: CompileError[];
}

async function show (client: CoreClient, body: { fqn?: string }, signal?: AbortSignal) {
  const { data, error, response } = await client.POST('/core.show', { body, signal });
  if (!data) throw toSdkError(response.status, (error as { error?: ApiErrorBody } | undefined)?.error);
  return data;
}

/**
 * What a Data App is provisioned with: `core.show` of the repo, then of each of its datasets, in
 * parallel.
 */
export async function loadDatasets (client: CoreClient, signal?: AbortSignal): Promise<LoadedDatasets> {
  const repo = await show(client, {}, signal);
  if (repo.object.kind !== 'repo') throw new TransportError(`core.show answered a ${repo.object.kind}, not the repo.`);
  const shown = await Promise.all(repo.object.datasets.map((d) => show(client, { fqn: d.fqn }, signal)));
  const datasets: Record<string, DatasetDescriptor> = {};
  shown.forEach(({ object }) => {
    if (object.kind === 'dataset') datasets[object.fqn] = toDescriptor(object);
  });
  return { datasets, diagnostics: repo.diagnostics };
}
