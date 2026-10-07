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

  it('refuses to mount without its frame script', () => {
    expect(() => mountDataApp({ ...options(), frameScript: undefined })).toThrow(/built without its frame script/);
  });
});
