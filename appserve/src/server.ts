// What the appserve frontend asks its server: the appserve backend's routes (/appserve/…), and the
// core API (/api) through the Anfra SDK's client, as any client of anfra does.
import { coreClient, loadDatasets } from 'anfra-sdk/api';
import type { DatasetDescriptor, User } from 'anfra-sdk/common';
import type { CatalogEntry } from './catalog';

export const api = coreClient('/api');

/** What the frontend starts from: the repo, who reads it, and whether it live-reloads. */
export interface Context {
  repo: { name: string };
  reader: User;
  watch: boolean;
}

async function json<T> (path: string): Promise<T> {
  const response = await fetch(path);
  if (!response.ok) throw new Error(`The server answered ${response.status}.`);
  return response.json() as Promise<T>;
}

export const context = () => json<Context>('/appserve/context');
export const catalog = () => json<CatalogEntry[]>('/appserve/apps');
/**
 * What each Data App is provisioned with: every dataset that could be loaded. With the problems
 * of the ones that couldn't, or that are incomplete, such as a data source the repo doesn't
 * configure; the files that do not compile are left to `problems`.
 */
export async function datasets (): Promise<{ datasets: Record<string, DatasetDescriptor>, problems: AmlProblem[] }> {
  const loaded = await loadDatasets(api);
  return {
    datasets: loaded.datasets,
    problems: loaded.diagnostics.filter((d) => !d.filePath).map((d) => ({ message: d.message })),
  };
}

/** A Data App definition, as written, and the base its relative URLs resolve against. */
export async function definition (path: string): Promise<{ html: string, baseHref: string }> {
  const file = path.split('/').map(encodeURIComponent).join('/');
  const response = await fetch(`/appserve/files/${file}`);
  if (!response.ok) throw new Error(`Couldn't read ${path}: the server answered ${response.status}.`);
  const dir = file.includes('/') ? file.slice(0, file.lastIndexOf('/') + 1) : '';
  return { html: await response.text(), baseHref: `/appserve/files/${dir}` };
}

/** Whether the server and its sidecars answer: core.status's state. */
export type Health = 'up' | 'down' | 'unreachable';

export async function health (): Promise<Health> {
  try {
    const { data } = await api.POST('/core.status', { body: {} });
    if (!data) return 'unreachable';
    return data.state === 'healthy' ? 'up' : 'down';
  } catch {
    return 'unreachable';
  }
}

/** One AML problem, as the problems banner shows it. */
export interface AmlProblem { file?: string, line?: number, column?: number, message: string }

/**
 * The repo's AML problems: the files that do not compile, and the validator suite's error
 * findings. Warnings are left out of the banner.
 */
export async function problems (): Promise<AmlProblem[]> {
  const { data } = await api.POST('/core.validate', { body: {} });
  if (!data) return [];
  return [
    ...(data.diagnostics ?? []).map((d) => ({ file: d.filePath, line: d.row, column: d.col, message: d.message })),
    ...(data.reports ?? []).filter((r) => r.severity === 'error').map((r) => ({ file: r.filePath, message: r.message })),
  ];
}

/** What changed in the repo, while live reload is on. */
export type RepoEvent = { type: 'apps', paths: string[] } | { type: 'aml' };
