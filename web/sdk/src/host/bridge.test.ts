import {
  afterEach, beforeEach, describe, expect, it, vi,
} from 'vitest';
import type { FrameMessage, HostMessage } from '../common/bridge';
import { PermissionError } from '../common/errors';
import type { Backend } from '../common/types';
import { serveBridge, type BridgeHandle } from './bridge';

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
