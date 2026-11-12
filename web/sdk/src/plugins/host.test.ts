import {
  afterEach, beforeEach, describe, expect, it, vi,
} from 'vitest';
import type { BridgeMessage } from '../common/bridge';
import type { Backend } from '../common/types';
import { salesDataset, testUser } from '../app/testSupport';
import { createHost, type MountedDataApp } from '../host/runtime';
import type { Plugin } from '../host/plugin';
import { corePlugin } from './core';
import { exportPlugin } from './export';
import { lineagePlugin } from './lineage';
import { pictureFilename, renderPlugin } from './render';

// jsdom's Blob has no arrayBuffer(), which every browser the SDK runs in has.
if (!Blob.prototype.arrayBuffer) {
  Blob.prototype.arrayBuffer = function arrayBuffer (this: Blob) {
    return new Promise<ArrayBuffer>((resolve, reject) => {
      const reader = new FileReader();
      reader.onload = () => resolve(reader.result as ArrayBuffer);
      reader.onerror = () => reject(reader.error);
      reader.readAsArrayBuffer(this);
    });
  };
}

let replies: BridgeMessage[];
let mounted: MountedDataApp | undefined;

beforeEach(() => {
  replies = [];
});

afterEach(() => {
  mounted?.unmount();
  mounted = undefined;
  vi.restoreAllMocks();
});

function backend (overrides: Partial<Backend> = {}): Backend {
  return {
    submitQuery: vi.fn(async () => ({ columns: [], values: [] })),
    fieldSuggestions: vi.fn(async () => []),
    ...overrides,
  };
}

function mount (...plugins: Plugin[]) {
  const host = createHost({ plugins: [corePlugin(backend()), ...plugins], frameScript: 'x' });
  mounted = host.mount({
    container: document.body, definition: '', datasets: { sales: salesDataset }, user: testUser,
  });
  vi.spyOn(mounted.frame.contentWindow!, 'postMessage').mockImplementation((message: unknown) => { replies.push(message as BridgeMessage); });
  return mounted;
}

function fromFrame (message: BridgeMessage) {
  window.dispatchEvent(new MessageEvent('message', { data: message, source: mounted!.frame.contentWindow }));
}

/** Until the frame has had `count` answers: reading a file's bytes takes more than a tick. */
const answered = (count: number) => vi.waitFor(() => { expect(replies.length).toBeGreaterThanOrEqual(count); });

const query = { dataset: 'sales', aql: 'explore {}', input: { filters: [], conditions: [], sorts: [], dateDrills: [] } };

describe('core', () => {
  /** The frame's `core.execute` calls, as the host made them. */
  const executes = () => replies.filter((message) => message.type === 'anfra:call' && message.name === 'core.execute') as { id: number, request: unknown }[];
  const answer = (id: number, result: unknown) => fromFrame({ type: 'anfra:result', id, ok: true, result });
  const results = (payload: unknown) => fromFrame({ type: 'anfra:event', name: 'core.results', payload });

  it('refreshes the frame: every query again, busting the cache unless told not to, answering its summary', async () => {
    const app = mount();
    const busting = app.call('core.refresh');
    await answered(1);
    expect(executes()[0].request).toEqual({ force: true, bustCache: true });
    answer(executes()[0].id, { succeeded: ['revenue'], failed: [] });
    await expect(busting).resolves.toEqual({ succeeded: ['revenue'], failed: [] });

    const keeping = app.call('core.refresh', { bustCache: false });
    await answered(2);
    expect(executes()[1].request).toEqual({ force: true, bustCache: false });
    answer(executes()[1].id, { succeeded: [], failed: ['orders'] });
    await expect(keeping).resolves.toEqual({ succeeded: [], failed: ['orders'] });
  });

  it('runs one refresh at a time, whatever it busts, and clears it once one ends however it ends', async () => {
    const app = mount();
    const first = app.call('core.refresh');
    await answered(1);
    await expect(app.call('core.refresh', { bustCache: false })).rejects.toThrow('A refresh is running already.');
    fromFrame({ type: 'anfra:result', id: executes()[0].id, ok: false, error: { name: 'DataAppError', code: 'call_failed', scope: 'server', message: 'gone' } });
    await expect(first).rejects.toThrow('gone');

    const second = app.call('core.refresh');
    await answered(2);
    answer(executes()[1].id, { succeeded: [], failed: [] });
    await second;

    const third = app.call('core.refresh');
    await answered(3);
    app.unmount();
    mounted = undefined;
    await expect(third).rejects.toThrow(/unmounted/);
    expect(app.commandState('core.refresh')).not.toHaveProperty('disabled');
  });

  it('is not the frame\'s to call: neither the refresh nor the frame\'s own execute', async () => {
    mount();
    fromFrame({ type: 'anfra:call', id: 1, name: 'core.refresh', request: null });
    fromFrame({ type: 'anfra:call', id: 2, name: 'core.execute', request: { force: true } });
    await answered(2);
    expect(replies).toEqual([
      { type: 'anfra:result', id: 1, ok: false, error: expect.objectContaining({ code: 'forbidden' }) },
      { type: 'anfra:result', id: 2, ok: false, error: expect.objectContaining({ code: 'invalid_request' }) },
    ]);
  });

  it('offers Refresh, disabled while a refresh runs and not while the frame runs its queries', async () => {
    const host = createHost({ plugins: [corePlugin(backend())], frameScript: 'x' });
    expect(host.commands).toEqual([expect.objectContaining({
      id: 'core.refresh', title: 'Refresh', icon: 'refresh', placement: 'primary', run: { call: 'core.refresh', request: { bustCache: true } },
    })]);
    // A button and a palette entry only: a key forwarded from the frame could not run a host-only call.
    expect(host.commands[0]!.shortcut).toBeUndefined();
    const app = mount();
    const states: unknown[] = [];
    app.onCommandsChange(() => states.push(app.commandState('core.refresh')));

    fromFrame({ type: 'anfra:call', id: 1, name: 'core.submitQuery', request: query });
    expect(app.commandState('core.refresh')).toEqual({});

    const run = app.run('core.refresh');
    await vi.waitFor(() => { expect(executes()).toHaveLength(1); });
    expect(app.commandState('core.refresh')).toEqual({ disabled: 'Refreshing…' });
    await expect(app.run('core.refresh')).rejects.toThrow('Refreshing…');
    answer(executes()[0].id, { succeeded: [], failed: [] });
    await run;
    expect(states).toEqual([{ disabled: 'Refreshing…' }, {}]);
  });

  it('says how old the figures are while a cached result shows, each minute, and nothing once none does', async () => {
    vi.useFakeTimers({ toFake: ['Date', 'setInterval', 'clearInterval'] });
    try {
      vi.setSystemTime(new Date('2026-11-12T09:00:00Z'));
      const app = mount();
      let changes = 0;
      app.onCommandsChange(() => { changes += 1; });

      results({ cached: false });
      expect(app.commandState('core.refresh')).toEqual({});
      results({ cached: true, cachedSince: '2026-11-12T08:55:00Z' });
      expect(app.commandState('core.refresh')).toEqual({ detail: 'Data from 5 minutes ago' });
      expect(changes).toBe(1);

      vi.advanceTimersByTime(60_000);
      expect(changes).toBe(2);
      expect(app.commandState('core.refresh')).toEqual({ detail: 'Data from 6 minutes ago' });

      results({ cached: false });
      expect(app.commandState('core.refresh')).toEqual({});
      vi.advanceTimersByTime(180_000);
      expect(changes).toBe(3);

      // What isn't a status is dropped.
      results({ cached: 'yes' });
      results({ cached: true, cachedSince: 'yesterday' });
      expect(app.commandState('core.refresh')).toEqual({});
      // A cache that doesn't say when it ran.
      results({ cached: true });
      expect(app.commandState('core.refresh')).toEqual({ detail: 'Data from a cache' });
    } finally {
      vi.useRealTimers();
    }
  });

  it('says it is refreshing, not how old the figures are, while a refresh runs', async () => {
    vi.useFakeTimers({ toFake: ['Date'] });
    try {
      vi.setSystemTime(new Date('2026-11-12T09:00:00Z'));
      const app = mount();
      results({ cached: true, cachedSince: '2026-11-12T07:00:00Z' });
      expect(app.commandState('core.refresh')).toEqual({ detail: 'Data from 2 hours ago' });
      app.call('core.refresh').catch(() => {});
      await vi.waitFor(() => { expect(executes()).toHaveLength(1); });
      expect(app.commandState('core.refresh')).toEqual({ disabled: 'Refreshing…' });
    } finally {
      vi.useRealTimers();
    }
  });
});

describe('export', () => {
  const link = {
    url: 'http://host/exports/download/t0k3n/revenue.csv', filename: 'revenue.csv', format: 'csv', rowCount: 3, expiresAt: new Date(0),
  };
  const request = { ...query, format: 'csv', formatOptions: { bom: true }, filename: 'revenue' };

  it('is made by the Backend, saved from the host page, then answered with its link', async () => {
    const clicked: { href: string, download: string, connected: boolean }[] = [];
    vi.spyOn(HTMLAnchorElement.prototype, 'click').mockImplementation(function (this: HTMLAnchorElement) {
      clicked.push({ href: this.href, download: this.download, connected: this.isConnected });
    });
    const exportQuery = vi.fn(async () => link);
    mount(exportPlugin({ exportQuery }));
    fromFrame({ type: 'anfra:call', id: 3, name: 'export.query', request });
    await answered(1);
    expect(exportQuery).toHaveBeenCalledWith(request, expect.any(AbortSignal));
    expect(clicked).toEqual([{ href: link.url, download: 'revenue.csv', connected: true }]);
    expect(document.querySelector('a[download]')).toBeNull();
    expect(replies).toEqual([{ type: 'anfra:result', id: 3, ok: true, result: link }]);
  });

  it('needs a Backend that can export, as lineage needs one that can trace', () => {
    expect(() => exportPlugin({})).toThrow(/no exportQuery/);
    expect(() => lineagePlugin({})).toThrow(/no lineage/);
  });
});

describe('lineage', () => {
  it("is answered by the host's Backend", async () => {
    const traced = { results: [] };
    const lineage = vi.fn(async () => traced);
    mount(lineagePlugin({ lineage }));
    const request = { targets: [{ parent: 'orders', child: 'amount' }], limit: 5 };
    fromFrame({ type: 'anfra:call', id: 1, name: 'lineage.trace', request });
    await answered(1);
    expect(lineage).toHaveBeenCalledWith(request, expect.any(AbortSignal));
    expect(replies).toEqual([{ type: 'anfra:result', id: 1, ok: true, result: traced }]);
  });
});

describe('render', () => {
  const png = () => new Blob([new Uint8Array([0x89, 0x50, 0x4e, 0x47, 0x0d, 0x0a, 0x1a, 0x0a, 0, 0])]);
  const pdf = () => new Blob(['%PDF-1.4\n']);
  let clicked: { href: string, download: string }[];
  let saved: Blob[];

  beforeEach(() => {
    clicked = [];
    saved = [];
    vi.spyOn(HTMLAnchorElement.prototype, 'click').mockImplementation(function (this: HTMLAnchorElement) {
      clicked.push({ href: this.href, download: this.download });
    });
    URL.createObjectURL = vi.fn((blob: Blob) => { saved.push(blob); return `blob:${saved.length}`; });
    URL.revokeObjectURL = vi.fn();
  });

  const save = (id: number, blob: unknown, filename: unknown) => fromFrame({
    type: 'anfra:call', id, name: 'render.save', request: { blob, filename },
  });

  it('offers the page as a PNG and a PDF, each drawn by the frame for the host', async () => {
    const app = mount(renderPlugin({ renderer: 'R' }));
    const host = createHost({ plugins: [corePlugin(backend()), renderPlugin({ renderer: 'R' })], frameScript: 'x' });
    expect(host.commands.map((command) => [command.id, command.title, command.placement])).toEqual([
      ['core.refresh', 'Refresh', 'primary'], ['render.png', 'Download as PNG', 'overflow'], ['render.pdf', 'Download as PDF', 'overflow'],
    ]);
    app.run('render.pdf').catch(() => {}); // the frame never answers: it is unmounted after
    await answered(1);
    expect(replies[0]).toMatchObject({ type: 'anfra:call', name: 'render.draw', request: { format: 'pdf' } });
  });

  it('shows its commands as disabled while the frame draws, and answers what it drew', async () => {
    const app = mount(renderPlugin({ renderer: 'R' }));
    const run = app.run('render.png');
    await answered(1);
    expect(app.commandState('render.pdf')).toEqual({ disabled: 'Drawing a picture…' });
    await expect(app.run('render.pdf')).rejects.toThrow('Drawing a picture…');
    const { id } = replies[0] as { id: number };
    fromFrame({ type: 'anfra:result', id, ok: true, result: { omitted: [] } });
    await expect(run).resolves.toEqual({ omitted: [] });
    expect(app.commandState('render.pdf')).toEqual({});
  });

  it('shows its commands as disabled while a picture saves', async () => {
    const app = mount(renderPlugin({ renderer: 'R' }));
    const states: unknown[] = [];
    app.onCommandsChange(() => states.push(app.commandState('render.png')));
    save(1, png(), 'Sales.png');
    await answered(1);
    expect(states).toEqual([{ disabled: 'Saving a picture…' }, {}]);
  });

  it('serves the renderer as its frame body, and saves the file the frame makes', async () => {
    mount(renderPlugin({ renderer: 'RENDERER' }));
    fromFrame({ type: 'anfra:call', id: 1, name: 'anfra.body', request: { plugin: 'render' } });
    save(2, png(), 'Sales.png');
    await answered(2);
    expect(replies).toEqual([
      { type: 'anfra:result', id: 1, ok: true, result: 'RENDERER' },
      { type: 'anfra:result', id: 2, ok: true, result: null },
    ]);
    expect(clicked).toEqual([{ href: 'blob:1', download: 'Sales.png' }]);
    expect(saved[0].type).toBe('image/png');
  });

  it('saves a file as the picture its bytes are, under a name ending in its extension', async () => {
    mount(renderPlugin({ renderer: 'R' }));
    save(1, pdf(), 'Q3 report.pdf.exe');
    await answered(1);
    expect(replies).toEqual([{ type: 'anfra:result', id: 1, ok: true, result: null }]);
    expect(clicked).toEqual([{ href: 'blob:1', download: 'Q3 report.pdf.exe.pdf' }]);
    expect(saved[0].type).toBe('application/pdf');
  });

  it('refuses to save what is not a PNG or a PDF, whatever it is called', async () => {
    mount(renderPlugin({ renderer: 'R' }));
    save(1, new Blob(['MZ\x90\x00'], { type: 'image/png' }), 'Sales.png');
    await answered(1);
    save(2, 'not a blob', 'Sales.png');
    await answered(2);
    expect(replies).toEqual([
      { type: 'anfra:result', id: 1, ok: false, error: { name: 'DataAppError', code: 'invalid_request', scope: 'client', message: 'Only a PNG or a PDF rendering can be saved.' } },
      { type: 'anfra:result', id: 2, ok: false, error: { name: 'DataAppError', code: 'invalid_request', scope: 'client', message: 'A rendering must be saved as a file.' } },
    ]);
    expect(clicked).toEqual([]);
  });

  it('saves one file at a time, and a few in a window, per frame', async () => {
    vi.useFakeTimers({ toFake: ['Date'] });
    try {
      mount(renderPlugin({ renderer: 'R' }));
      save(1, png(), 'a');
      save(2, png(), 'b');
      await answered(2);
      for (let id = 3; id <= 7; id++) {
        save(id, png(), String(id));
        await answered(id);
      }
      vi.setSystemTime(Date.now() + 10_000);
      save(8, png(), 'later');
      await answered(8);
      const busy = { name: 'DataAppError', code: 'invalid_request', scope: 'client', message: 'Too many files saved at once: try again in a few seconds.' };
      expect(replies.map((r) => (r.type === 'anfra:result' && !r.ok ? [r.id, r.error] : r.type === 'anfra:result' && r.id))).toEqual([
        [2, busy], 1, 3, 4, 5, 6, [7, busy], 8,
      ]);
      expect(clicked.map((c) => c.download)).toEqual(['a.png', '3.png', '4.png', '5.png', '6.png', 'later.png']);
    } finally {
      vi.useRealTimers();
    }
  });

  it('is off where it is not installed: nothing saves a file', async () => {
    mount();
    save(1, png(), 'x.png');
    await answered(1);
    expect(replies).toEqual([{ type: 'anfra:result', id: 1, ok: false, error: { name: 'DataAppError', code: 'invalid_request', scope: 'client', message: "Nothing on this host answers 'render.save'." } }]);
  });

  it('keeps the words of a name, drops what a file system or a reader would misread, and ends in the extension', () => {
    expect(pictureFilename('Sales - Revenue.png', 'png')).toBe('Sales - Revenue.png');
    expect(pictureFilename('Sales - Revenue.PNG', 'png')).toBe('Sales - Revenue.PNG');
    expect(pictureFilename('Sales 2026.10', 'pdf')).toBe('Sales 2026.10.pdf');
    expect(pictureFilename('../../etc/passwd', 'png')).toBe('-..-etc-passwd.png');
    expect(pictureFilename('invoice‮gnp.exe', 'png')).toBe('invoicegnp.exe.png');
    expect(pictureFilename('a\nb\u0000c', 'png')).toBe('abc.png');
    expect(pictureFilename('   ...  ', 'png')).toBe('Rendering.png');
    expect(pictureFilename(42, 'pdf')).toBe('Rendering.pdf');
    expect(pictureFilename('x'.repeat(300), 'png')).toHaveLength(204);
  });
});
