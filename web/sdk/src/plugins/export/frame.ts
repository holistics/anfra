/**
 * `export`'s frame half: `query.export()`. The request is the app model's (the query with what its
 * result on screen was last run with); the plugin sends it.
 */
import type { FrameKernel } from '../../app/kernel';
import type { Query } from '../../app/query';
import type { ExportOptions, ExportResult } from '../../common/types';

export default function setup (kernel: FrameKernel): void {
  kernel.provideOnQuery((query: Query) => (options: ExportOptions = {}): Promise<ExportResult> => {
    const app = kernel.sdk.apps.find((candidate) => candidate.queries[query.name] === query);
    if (!app) return Promise.reject(new kernel.errors.ValidationError(`Query '${query.name}' belongs to no app.`, query.name));
    return app.exportQuery(query, options, (request, signal) => kernel.call<ExportResult>('export.query', request, signal)
      .then((result) => {
        // Dates don't survive the bridge.
        result.expiresAt = new Date(result.expiresAt);
        return result;
      }));
  });
}
