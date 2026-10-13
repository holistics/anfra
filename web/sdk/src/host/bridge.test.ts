import {
  afterEach, beforeEach, describe, expect, it, vi,
} from 'vitest';
import type { FrameMessage, HostMessage } from '../common/bridge';
import { PermissionError, ValidationError } from '../common/errors';
import type { Backend } from '../common/types';
import { pictureFilename, serveBridge, type BridgeHandle } from './bridge';

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

let frame: HTMLIFrameElement;
let replies: HostMessage[];
let bridge: BridgeHandle | undefined;

beforeEach(() => {
  frame = document.createElement('iframe');
  document.body.appendChild(frame);
  replies = [];
  vi.spyOn(frame.contentWindow!, 'postMessage').mockImplementation((message: unknown) => { replies.push(message as HostMessage); });
});

afterEach(() => {
  bridge?.stop();
  frame.remove();
  vi.restoreAllMocks();
});

function fromFrame (message: FrameMessage, source: Window | null = frame.contentWindow) {
  window.dispatchEvent(new MessageEvent('message', { data: message, source }));
}

const settle = () => new Promise((resolve) => { setTimeout(resolve, 0); });
/** Until the frame has had `count` answers: reading a file's bytes takes more than a tick. */
const answered = (count: number) => vi.waitFor(() => { expect(replies.length).toBeGreaterThanOrEqual(count); });

function backend (overrides: Partial<Backend> = {}): Backend {
  return {
    submitQuery: vi.fn(async () => ({ columns: [], values: [[1]] })),
    fieldSuggestions: vi.fn(async () => ['a', 'b']),
    ...overrides,
  };
}

const query = { dataset: 'sales', aql: 'explore {}', input: { filters: [], conditions: [], sorts: [], dateDrills: [] } };

describe('serveBridge', () => {
  it("answers the frame's Backend calls with the host's Backend", async () => {
    const b = backend();
    bridge = serveBridge(frame, b);
    fromFrame({ type: 'anfra:request', id: 1, method: 'submitQuery', request: query });
    fromFrame({ type: 'anfra:request', id: 2, method: 'fieldSuggestions', request: { dataset: 'sales', model: 'orders', field: 'status', q: '' } });
    await settle();
    expect(b.submitQuery).toHaveBeenCalledWith(query, expect.any(AbortSignal));
    expect(replies).toEqual([
      { type: 'anfra:response', id: 1, ok: true, result: { columns: [], values: [[1]] } },
      { type: 'anfra:response', id: 2, ok: true, result: ['a', 'b'] },
    ]);
  });

  it('sends a failure across as its error class by name', async () => {
    bridge = serveBridge(frame, backend({ submitQuery: async () => { throw new PermissionError('Not yours.'); } }));
    fromFrame({ type: 'anfra:request', id: 1, method: 'submitQuery', request: query });
    await settle();
    expect(replies).toEqual([{ type: 'anfra:response', id: 1, ok: false, error: { name: 'PermissionError', message: 'Not yours.' } }]);
  });

  describe('an export', () => {
    const link = {
      url: 'http://host/exports/download/t0k3n/revenue.csv', filename: 'revenue.csv', format: 'csv', rowCount: 3, expiresAt: new Date(0),
    };
    const exportRequest = { ...query, format: 'csv', formatOptions: { bom: true }, filename: 'revenue' };

    it('is made by the Backend, saved from the host page, then answered with its link', async () => {
      const clicked: { href: string, download: string, connected: boolean }[] = [];
      vi.spyOn(HTMLAnchorElement.prototype, 'click').mockImplementation(function (this: HTMLAnchorElement) {
        clicked.push({ href: this.href, download: this.download, connected: this.isConnected });
      });
      const exportQuery = vi.fn(async () => link);
      bridge = serveBridge(frame, backend({ exportQuery }));
      fromFrame({ type: 'anfra:request', id: 3, method: 'exportQuery', request: exportRequest });
      await settle();
      expect(exportQuery).toHaveBeenCalledWith(exportRequest, expect.any(AbortSignal));
      expect(clicked).toEqual([{ href: link.url, download: 'revenue.csv', connected: true }]);
      expect(document.querySelector('a[download]')).toBeNull();
      expect(replies).toEqual([{ type: 'anfra:response', id: 3, ok: true, result: link }]);
    });

    it('fails as a TransportError when the Backend cannot export, saving nothing', async () => {
      const click = vi.spyOn(HTMLAnchorElement.prototype, 'click');
      bridge = serveBridge(frame, backend());
      fromFrame({ type: 'anfra:request', id: 4, method: 'exportQuery', request: exportRequest });
      await settle();
      expect(click).not.toHaveBeenCalled();
      expect(replies).toEqual([{
        type: 'anfra:response', id: 4, ok: false, error: { name: 'TransportError', message: 'This host cannot make exports.' },
      }]);
    });
  });

  describe('renderings', () => {
    const rendering = { render: true, renderer: 'window.__anfraRenderer = {};' };

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

    const save = (id: number, blob: unknown, filename: unknown) => fromFrame(
      { type: 'anfra:request', id, method: 'saveFile', request: { blob, filename } } as FrameMessage,
    );

    it("sends the renderer's code when the frame asks, and saves the file it makes", async () => {
      bridge = serveBridge(frame, backend(), {}, rendering);
      fromFrame({ type: 'anfra:request', id: 1, method: 'renderer', request: null });
      save(2, png(), 'Sales.png');
      await answered(2);
      expect(replies).toEqual([
        { type: 'anfra:response', id: 1, ok: true, result: rendering.renderer },
        { type: 'anfra:response', id: 2, ok: true, result: null },
      ]);
      expect(clicked).toEqual([{ href: 'blob:1', download: 'Sales.png' }]);
      expect(saved[0].type).toBe('image/png');
    });

    it('saves a file as the picture its bytes are, under a name ending in its extension', async () => {
      bridge = serveBridge(frame, backend(), {}, rendering);
      save(1, pdf(), 'Q3 report.pdf.exe');
      await answered(1);
      expect(replies).toEqual([{ type: 'anfra:response', id: 1, ok: true, result: null }]);
      expect(clicked).toEqual([{ href: 'blob:1', download: 'Q3 report.pdf.exe.pdf' }]);
      expect(saved[0].type).toBe('application/pdf');
    });

    it('refuses to save what is not a PNG or a PDF, whatever it is called', async () => {
      bridge = serveBridge(frame, backend(), {}, rendering);
      save(1, new Blob(['MZ\x90\x00'], { type: 'image/png' }), 'Sales.png');
      await answered(1);
      save(2, 'not a blob', 'Sales.png');
      await answered(2);
      expect(replies).toEqual([
        { type: 'anfra:response', id: 1, ok: false, error: { name: 'ValidationError', message: 'Only a PNG or a PDF rendering can be saved.' } },
        { type: 'anfra:response', id: 2, ok: false, error: { name: 'ValidationError', message: 'A rendering must be saved as a file.' } },
      ]);
      expect(clicked).toEqual([]);
    });

    it('saves one file at a time, and a few in a window', async () => {
      vi.useFakeTimers({ toFake: ['Date'] });
      try {
        bridge = serveBridge(frame, backend(), {}, rendering);
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
        const busy = { name: 'ValidationError', message: 'Too many files saved at once: try again in a few seconds.' };
        expect(replies.map((r) => (r.type === 'anfra:response' && !r.ok ? [r.id, r.error] : r.type === 'anfra:response' && r.id))).toEqual([
          [2, busy], 1, 3, 4, 5, 6, [7, busy], 8,
        ]);
        expect(clicked.map((c) => c.download)).toEqual(['a.png', '3.png', '4.png', '5.png', '6.png', 'later.png']);
      } finally {
        vi.useRealTimers();
      }
    });

    it('asks the frame to render, and answers with what it rendered or why it could not', async () => {
      bridge = serveBridge(frame, backend(), {}, rendering);
      const done = bridge.render({ block: 'trend', format: 'png' });
      const failed = bridge.render({ block: 'nope', format: 'png' });
      expect(replies).toEqual([
        { type: 'anfra:render', id: 1, request: { block: 'trend', format: 'png' } },
        { type: 'anfra:render', id: 2, request: { block: 'nope', format: 'png' } },
      ]);
      const result = { filename: 'a.png', format: 'png' as const, width: 1, height: 1, blob: new Blob(), omitted: [] };
      fromFrame({ type: 'anfra:rendered', id: 1, ok: true, result });
      fromFrame({ type: 'anfra:rendered', id: 2, ok: false, error: { name: 'ValidationError', message: 'No element is marked data-anfra-block="nope".' } });
      await expect(done).resolves.toEqual(result);
      await expect(failed).rejects.toBeInstanceOf(ValidationError);
    });

    it('refuses all of it unless rendering is turned on', async () => {
      bridge = serveBridge(frame, backend(), {}, { renderer: 'code' });
      await expect(bridge.render({ format: 'png' })).rejects.toThrow(/does not render/);
      fromFrame({ type: 'anfra:request', id: 1, method: 'renderer', request: null });
      save(2, png(), 'x.png');
      await settle();
      expect(replies).toEqual([
        { type: 'anfra:response', id: 1, ok: false, error: { name: 'ValidationError', message: 'This host does not render Data Apps.' } },
        { type: 'anfra:response', id: 2, ok: false, error: { name: 'ValidationError', message: 'This host does not render Data Apps.' } },
      ]);
    });

    it('rejects a rendering still running when it stops', async () => {
      bridge = serveBridge(frame, backend(), {}, rendering);
      const pending = bridge.render({ format: 'pdf' });
      bridge.stop();
      bridge = undefined;
      await expect(pending).rejects.toThrow(/unmounted/);
    });
  });

  describe('pictureFilename', () => {
    it('keeps the words, drops what a file system or a reader would misread, and ends in the extension', () => {
      expect(pictureFilename('Sales - Revenue.png', 'png')).toBe('Sales - Revenue.png');
      expect(pictureFilename('Sales - Revenue.PNG', 'png')).toBe('Sales - Revenue.PNG');
      expect(pictureFilename('Sales 2026.10', 'pdf')).toBe('Sales 2026.10.pdf');
      expect(pictureFilename('../../etc/passwd', 'png')).toBe('-..-etc-passwd.png');
      expect(pictureFilename('invoice\u202egnp.exe', 'png')).toBe('invoicegnp.exe.png');
      expect(pictureFilename('a\nb\u0000c', 'png')).toBe('abc.png');
      expect(pictureFilename('   ...  ', 'png')).toBe('Rendering.png');
      expect(pictureFilename(42, 'pdf')).toBe('Rendering.pdf');
      expect(pictureFilename('x'.repeat(300), 'png')).toHaveLength(204);
    });
  });

  it("aborts a call the frame cancels, and answers nothing for it", async () => {
    let signal: AbortSignal | undefined;
    bridge = serveBridge(frame, backend({
      submitQuery: (_r, s) => new Promise((_resolve, reject) => {
        signal = s;
        s.addEventListener('abort', () => reject(Object.assign(new Error('aborted'), { name: 'AbortError' })));
      }),
    }));
    fromFrame({ type: 'anfra:request', id: 7, method: 'submitQuery', request: query });
    fromFrame({ type: 'anfra:cancel', id: 7 });
    await settle();
    expect(signal?.aborted).toBe(true);
    expect(replies).toEqual([]);
  });

  it('asks the frame to highlight, locate and pick, and expects no answer to any', () => {
    bridge = serveBridge(frame, backend());
    bridge.highlight('0.2');
    bridge.highlight(null);
    bridge.locate({ node: '0.2' });
    bridge.locate({ app: 0, kind: 'query', name: 'revenue' });
    bridge.setPicking(true);
    expect(replies).toEqual([
      { type: 'anfra:highlight', node: '0.2' },
      { type: 'anfra:highlight', node: null },
      { type: 'anfra:locate', target: { node: '0.2' } },
      { type: 'anfra:locate', target: { app: 0, kind: 'query', name: 'revenue' } },
      { type: 'anfra:pick-watch', on: true },
    ]);
  });

  it('serves its own frame only, and only the Backend', async () => {
    const b = backend();
    bridge = serveBridge(frame, b);
    fromFrame({ type: 'anfra:request', id: 1, method: 'submitQuery', request: query }, window);
    fromFrame({ type: 'anfra:request', id: 2, method: 'ingest', request: {} } as unknown as FrameMessage);
    await settle();
    expect(b.submitQuery).not.toHaveBeenCalled();
    expect(replies).toEqual([{ type: 'anfra:response', id: 2, ok: false, error: { name: 'Error', message: 'No such Backend method: ingest' } }]);
  });

  it('relays inspection and picks while asked to, and tells a reloaded frame to keep posting', () => {
    const onInspect = vi.fn();
    const onPick = vi.fn();
    bridge = serveBridge(frame, backend(), { onInspect, onPick });
    const structure = { nodes: [], usage: {} };
    fromFrame({ type: 'anfra:inspect', apps: [], structure });
    fromFrame({ type: 'anfra:picked', node: '0' });
    expect(onInspect).not.toHaveBeenCalled();
    expect(onPick).not.toHaveBeenCalled();

    bridge.setInspecting(true);
    expect(replies).toEqual([{ type: 'anfra:inspect-watch', open: true }]);
    fromFrame({ type: 'anfra:inspect', apps: [], structure });
    expect(onInspect).toHaveBeenCalledWith([], structure);
    fromFrame({ type: 'anfra:picked', node: '0' });
    expect(onPick).toHaveBeenCalledWith('0');
    fromFrame({ type: 'anfra:ready' });
    expect(replies).toHaveLength(2);
  });
});
