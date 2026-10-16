/**
 * `export`: a query's whole result as a file (`query.export()`), made by the host's Backend and
 * saved for the reader from the host's page, since the frame has no `allow-downloads`.
 */
import type { Backend, BackendExportRequest, ExportResult } from '../../common/types';
import { definePlugin, type Plugin } from '../../host/plugin';
import { PLUGIN_VERSION } from '../version';
import setup from 'anfra-sdk:setup/export';

/**
 * Save an export's file for the reader, from the host's page: following a link is how a browser
 * writes a file to disk without holding it. A link to another origin (a presigned URL) ignores
 * `download`; its server's `Content-Disposition` names the file instead.
 */
function saveLink (doc: Document, link: ExportResult): void {
  const a = doc.createElement('a');
  a.href = link.url;
  a.download = link.filename;
  a.rel = 'noopener';
  a.style.display = 'none';
  doc.body.appendChild(a);
  a.click();
  a.remove();
}

/** `backend` must have `exportQuery` (`coreApiBackend` does). */
export function exportPlugin (backend: Pick<Backend, 'exportQuery'>): Plugin {
  const { exportQuery } = backend;
  if (typeof exportQuery !== 'function') throw new Error('exportPlugin: the Backend has no exportQuery.');
  return definePlugin({
    name: 'export',
    version: PLUGIN_VERSION,
    requires: ['core'],
    host: {
      calls: {
        /** Make the export, start saving it, then answer its link. */
        query: {
          frame: true,
          async handler (request: BackendExportRequest, { signal, mount }) {
            const link = await exportQuery.call(backend, request, signal);
            saveLink(mount?.document ?? document, link);
            return link;
          },
        },
      },
    },
    frame: { setup },
  });
}
