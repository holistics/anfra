import { describe, expect, it } from 'vitest';
import { salesDataset, stubBackend, testUser } from '../app/testSupport';
import { mountDataApp } from './mount';

describe('mountDataApp', () => {
  const options = () => ({
    container: document.body,
    definition: '<head></head><body>hi</body>',
    baseHref: '/appserve/files/',
    datasets: { sales: salesDataset },
    user: testUser,
    backend: stubBackend().backend,
    frameScript: 'window.FRAME = 1;',
  });

  it('mounts the definition in a frame that may run scripts and nothing more, provisioned', () => {
    const app = mountDataApp(options());
    expect(app.frame.getAttribute('sandbox')).toBe('allow-scripts');
    expect(app.frame.parentElement).toBe(document.body);
    expect(app.frame.srcdoc).toContain('window.FRAME = 1;');
    expect(app.frame.srcdoc).toContain('"hostOrigin":"http://localhost:3000"');
    expect(app.frame.srcdoc).toContain('<body>hi</body>');
    app.unmount();
    expect(app.frame.isConnected).toBe(false);
  });

  it('tells the frame whether its Backend can export', () => {
    const without = mountDataApp(options());
    const withExport = mountDataApp({
      ...options(),
      backend: stubBackend({
        exportQuery: { url: 'u', filename: 'f.csv', format: 'csv', rowCount: 0, expiresAt: new Date() },
      }).backend,
    });
    expect(without.frame.srcdoc).toContain('"features":{"export":false,');
    expect(withExport.frame.srcdoc).toContain('"features":{"export":true,');
    without.unmount();
    withExport.unmount();
  });

  it('tells the frame whether its Backend can trace lineage', () => {
    const without = mountDataApp(options());
    const withLineage = mountDataApp({ ...options(), backend: stubBackend({ lineage: { results: [] } }).backend });
    expect(without.frame.srcdoc).toContain('"lineage":false}');
    expect(withLineage.frame.srcdoc).toContain('"lineage":true}');
    without.unmount();
    withLineage.unmount();
  });

  it('tells the frame it may render only when turned on and it has the renderer', () => {
    const on = mountDataApp({ ...options(), renderer: 'code', render: true });
    const unasked = mountDataApp({ ...options(), renderer: 'code' });
    const off = mountDataApp({ ...options(), renderer: 'code', render: false });
    const none = mountDataApp({ ...options(), render: true });
    expect(on.frame.srcdoc).toContain('"render":true,');
    expect(unasked.frame.srcdoc).toContain('"render":false,');
    expect(off.frame.srcdoc).toContain('"render":false,');
    expect(none.frame.srcdoc).toContain('"render":false,');
    [on, unasked, off, none].forEach((app) => app.unmount());
  });

  it('refuses to mount without its frame script', () => {
    expect(() => mountDataApp({ ...options(), frameScript: undefined })).toThrow(/built without its frame script/);
  });
});
