import { describe, expect, it, vi } from 'vitest';
import type { FrameKernel } from './kernel';
import { PROVISION_ELEMENT_ID, type Provision } from '../common/bridge';
import { bootstrap } from './bootstrap';
import { callOf } from '../common/runtimeErrors';
import { salesDataset, testUser } from './testSupport';

/** A frame window, provisioned by a host at `hostOrigin`, whose `message` listener the test drives. */
function frameFrom (hostOrigin: string, setup: (kernel: FrameKernel) => void = () => {}) {
  const provision: Provision = {
    datasets: { sales: salesDataset }, user: testUser, plugins: [{ name: 'probe', version: '1' }], hostOrigin,
  };
  const posted: unknown[] = [];
  const parent = { postMessage: (message: unknown) => { posted.push(message); } };
  let onMessage: ((event: Partial<MessageEvent>) => void) | undefined;
  const win = {
    parent,
    setTimeout,
    addEventListener: (type: string, listener: typeof onMessage) => { if (type === 'message') onMessage = listener; },
    document: {
      getElementById: (id: string) => (id === PROVISION_ELEMENT_ID ? { textContent: JSON.stringify(provision) } : null),
      querySelectorAll: () => [],
      documentElement: {},
    },
  } as unknown as Window;

  const received: unknown[] = [];
  bootstrap(win, [['probe', (kernel) => { kernel.on('ping', (payload) => received.push(payload)); setup(kernel); }]]);

  const send = (event: Partial<MessageEvent>, payload: unknown) => onMessage!({
    ...event, data: { type: 'anfra:event', name: 'probe.ping', payload },
  });
  const call = (name: string) => onMessage!({
    source: parent as unknown as Window, origin: hostOrigin, data: { type: 'anfra:call', id: 1, name, request: {} },
  });
  const errors = () => posted.filter((m) => (m as { name?: string }).name === 'anfra.error').map((m) => (m as { payload: unknown }).payload);
  /** Fail the frame's call `name` as the host would. */
  const fail = (name: string, error: unknown) => {
    const sent = posted.find((m) => (m as { type?: string, name?: string }).type === 'anfra:call' && (m as { name: string }).name === name) as { id: number };
    onMessage!({ source: parent as unknown as Window, origin: hostOrigin, data: { type: 'anfra:result', id: sent.id, ok: false, error } });
  };
  return { win, parent, send, received, call, errors, fail };
}

describe('bootstrap', () => {
  it("takes the host's messages from its parent", () => {
    const frame = frameFrom('https://host.test');

    frame.send({ source: frame.parent as unknown as Window, origin: 'https://host.test' }, 'from the parent');

    expect(frame.received).toEqual(['from the parent']);
  });

  // A host that mounts into a nested document of its own (an empty same-origin frame, for a page
  // whose own content policy would stop the Data App's scripts) posts from its page's window.
  it("takes the host's messages from another window of the host's origin", () => {
    const frame = frameFrom('https://host.test');

    frame.send({ source: {} as Window, origin: 'https://host.test' }, 'from the page');

    expect(frame.received).toEqual(['from the page']);
  });

  it('drops a message from any other window', () => {
    const frame = frameFrom('https://host.test');

    frame.send({ source: {} as Window, origin: 'https://elsewhere.test' }, 'another site');
    frame.send({ source: {} as Window, origin: 'null' }, 'a sandboxed frame');

    expect(frame.received).toEqual([]);
  });

  it("drops an opaque window's message even when the host is opaque too", () => {
    const frame = frameFrom('null');

    frame.send({ source: {} as Window, origin: 'null' }, 'a sandboxed frame');
    frame.send({ source: frame.parent as unknown as Window, origin: 'null' }, 'the parent');

    expect(frame.received).toEqual(['the parent']);
  });

  it('reports a plugin that failed to set up to the host', () => {
    const frame = frameFrom('https://host.test', () => { throw new TypeError('no chart lib'); });
    expect(frame.errors()).toEqual([expect.objectContaining({ kind: 'plugin.uncaught', message: 'no chart lib' })]);
  });

  it("names a plugin's call as the app made it", async () => {
    const frame = frameFrom('https://host.test', (kernel) => kernel.provide({ fetchAll: () => kernel.call('probe.remote', {}) }));
    const probe = (frame.win as unknown as { Anfra: { probe: { fetchAll: () => Promise<unknown> } } }).Anfra.probe;
    expect(probe.fetchAll).toBe(probe.fetchAll);
    const failed = probe.fetchAll().catch((err: unknown) => err);
    frame.fail('probe.remote', { code: 'forbidden', scope: 'user', message: 'No access.' });
    expect(callOf(await failed)).toBe('probe_fetch_all');
  });

  it("reports a frame call's failure the host is told only the shape of", async () => {
    const frame = frameFrom('https://host.test', (kernel) => kernel.handle('boom', () => { throw new RangeError('bad index'); }));
    frame.call('probe.boom');
    await vi.waitFor(() => expect(frame.errors()).toEqual([expect.objectContaining({ kind: 'plugin.uncaught', message: 'bad index' })]));
  });
});
