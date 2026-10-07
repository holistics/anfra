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
 * Every dataset of the repo that could be shown, in full, by name; the ones that could not, and
 * why; and what is wrong in the repo for them (its files that do not compile, a data source it
 * does not configure).
 */
export interface LoadedDatasets {
  datasets: Record<string, DatasetDescriptor>;
  failed: { fqn: string, message: string }[];
  diagnostics: CompileError[];
}

async function show (client: CoreClient, body: { fqn?: string }, signal?: AbortSignal) {
  const { data, error, response } = await client.POST('/core.show', { body, signal });
  if (!data) throw toSdkError(response.status, (error as { error?: ApiErrorBody } | undefined)?.error);
  return data;
}

/**
 * What a Data App is provisioned with: `core.show` of the repo, then of each of its datasets, in
 * parallel. A dataset that cannot be shown is left out and reported, rather than failing the rest:
 * a Data App that does not use it still runs.
 */
export async function loadDatasets (client: CoreClient, signal?: AbortSignal): Promise<LoadedDatasets> {
  const repo = await show(client, {}, signal);
  if (repo.object.kind !== 'repo') throw new TransportError(`core.show answered a ${repo.object.kind}, not the repo.`);
  const fqns = repo.object.datasets.map((d) => d.fqn);
  const shown = await Promise.allSettled(fqns.map((fqn) => show(client, { fqn }, signal)));

  const loaded: LoadedDatasets = { datasets: {}, failed: [], diagnostics: [...repo.diagnostics] };
  const seen = new Set(loaded.diagnostics.map((d) => JSON.stringify(d)));
  shown.forEach((result, i) => {
    if (result.status === 'rejected') {
      loaded.failed.push({ fqn: fqns[i], message: (result.reason as Error).message });
      return;
    }
    const { object, diagnostics } = result.value;
    if (object.kind === 'dataset') loaded.datasets[object.fqn] = toDescriptor(object);
    diagnostics.forEach((d) => {
      const key = JSON.stringify(d);
      if (!seen.has(key)) {
        seen.add(key);
        loaded.diagnostics.push(d);
      }
    });
  });
  return loaded;
}
