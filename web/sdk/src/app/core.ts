/**
 * `core`'s frame side, which the kernel is: the host's refresh, `core.execute`, run on every app in
 * the frame; and `core.results`, whether a result on screen came from a server's result cache, told
 * to the host each time the apps' results settle, so its Refresh command can say how old they are.
 * Nothing here is the definition's: it refreshes its own app with `app.refresh()`.
 */
import type { ExecuteOptions, ExecuteSummary } from '../common/types';
import type { FrameKernel } from './kernel';

/** What `core.results` says: whether a cached result shows, and since when (the oldest cached run). */
export interface ResultsStatus {
  cached: boolean;
  /** ISO time of the oldest run behind a cached result on screen. */
  cachedSince?: string;
}

function executeOptions (request: unknown, { invalidRequest }: FrameKernel['errors']): ExecuteOptions {
  const options = (request ?? {}) as Record<string, unknown>;
  if (typeof options !== 'object') throw invalidRequest('core.execute takes { force?, bustCache? }.');
  for (const key of ['force', 'bustCache'] as const) {
    if (options[key] !== undefined && typeof options[key] !== 'boolean') {
      throw invalidRequest(`core.execute's ${key} is true or false, not ${JSON.stringify(options[key])}.`);
    }
  }
  return {
    ...(options.force ? { force: true } : {}),
    ...(options.bustCache ? { bustCache: true } : {}),
  };
}

/** Whether a result on screen came from a cache, from the apps' queries as they are now. */
export function resultsStatus (kernel: FrameKernel): ResultsStatus {
  let cached = false;
  let oldest = Infinity;
  for (const app of kernel.sdk.apps) {
    for (const query of Object.values(app.queries)) {
      const debug = query.result?.debug;
      if (!debug?.fromCache) continue;
      // A cached result that does not say when it ran is cached all the same.
      cached = true;
      const at = debug.executedAt ? new Date(debug.executedAt).getTime() : NaN;
      if (at < oldest) oldest = at;
    }
  }
  if (!cached) return { cached: false };
  return Number.isFinite(oldest) ? { cached: true, cachedSince: new Date(oldest).toISOString() } : { cached: true };
}

/** Start answering the host's `core.*` calls in this frame; `kernel` is the kernel's, named `core`. */
export function installCore (kernel: FrameKernel): void {
  kernel.handle('execute', async (request: unknown): Promise<ExecuteSummary> => {
    const options = executeOptions(request, kernel.errors);
    const summaries = await Promise.all(kernel.sdk.apps.map((app) => app.execute(options)));
    return {
      succeeded: summaries.flatMap((summary) => summary.succeeded),
      failed: summaries.flatMap((summary) => summary.failed),
    };
  });

  let last: string | undefined;
  let timer: number | undefined;
  const report = () => {
    timer = undefined;
    const running = kernel.sdk.apps.some((app) => Object.values(app.queries).some((query) => query.state === 'executing'));
    if (running) return; // told again as each query finishes
    const status = resultsStatus(kernel);
    const key = JSON.stringify(status);
    if (key === last) return;
    last = key;
    kernel.emit('results', status);
  };
  kernel.onApp((app) => {
    app.subscribe(() => {
      timer ??= kernel.window.setTimeout(report, 0);
    });
  });
}
