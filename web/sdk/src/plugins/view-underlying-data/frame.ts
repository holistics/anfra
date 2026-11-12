/**
 * `view-underlying-data`'s frame half: the "View underlying data" entry on a point's menu, and the
 * modal it opens. The entry shows for each measure of the point that resolves to a model (the one
 * the definition named with `query.menu`'s `measure`, or else each the query has); the modal lists
 * that model's rows behind the point, run as `core.submitQuery` and downloaded as `export.query`,
 * through the pipeline like any of the query's calls. The view is the plugin's body.
 */
import type { FrameKernel } from '../../app/kernel';
import type { App } from '../../app/app';
import type { Query } from '../../app/query';
import { defaultExportFilename } from '../../app/execution/submitQuery';
import type { MenuTarget, RowsTarget } from '../../common/menu';
import type { BackendExportRequest, BackendQueryResult, ExportResult } from '../../common/types';
import {
  candidates, PAGE_SIZE, plan, type Candidate,
} from './model';
import type { ViewDeps } from './view';

interface Body {
  view: (root: ShadowRoot, deps: ViewDeps) => () => void;
}

const LABEL = 'View underlying data';

export default function setup (kernel: FrameKernel): void {
  const located = (target: RowsTarget): { app: App, query: Query } | undefined => {
    const app = kernel.sdk.apps[target.app];
    const query = app?.queries[target.query];
    return app && query ? { app, query } : undefined;
  };

  async function open (target: RowsTarget, app: App, query: Query, candidate: Candidate): Promise<void> {
    const columns = query.result?.columns ?? [];
    const planned = plan(target, candidate, kernel.queryRequest(query), columns, query.dataset.descriptor);
    const body = await kernel.body<Body>();
    const canExport = kernel.sdk.plugins.has('export');
    const filename = `${defaultExportFilename(query, app.title)} - underlying data`;
    // The rows behind a point are the query's own requests, so inspect lists them under it.
    const stamp = { app: target.app, query: target.query };
    await kernel.modal({
      title: planned.title,
      size: 'lg',
      // Removing the query closes it: the rows it shows belong to a query that is gone.
      about: stamp,
      mount: (root) => body.view(root, {
        plan: planned,
        fetchPage: (page, signal) => kernel.call<BackendQueryResult>('core.submitQuery', { ...planned.request, input: { ...planned.request.input, pagination: { page, pageSize: PAGE_SIZE } } }, signal, stamp),
        ...(canExport
          ? {
            download: (signal: AbortSignal) => {
              const request: BackendExportRequest = {
                ...planned.request, format: 'csv', formatOptions: { bom: true }, filename,
              };
              return kernel.call<ExportResult>('export.query', request, signal, stamp);
            },
          }
          : {}),
      }),
    });
  }

  kernel.menuItem({
    id: 'open',
    target: 'rows',
    entries: (target: MenuTarget) => {
      if (target.kind !== 'rows') return [];
      const found = located(target);
      if (!found) return [];
      const { app, query } = found;
      const offered = candidates(target, query.result?.columns ?? [], query.dataset.descriptor);
      return offered.map((candidate) => ({
        label: offered.length > 1 ? `${LABEL}: ${candidate.column.label || candidate.column.name}` : LABEL,
        ...(candidate.disabled
          ? { disabled: candidate.disabled }
          : { run: () => open(target, app, query, candidate) }),
      }));
    },
  });
}
